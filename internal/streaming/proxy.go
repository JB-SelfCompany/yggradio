package streaming

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// ProxyHandler handles proxying of remote federated streams to local listeners
type ProxyHandler struct {
	db            *database.DB
	remoteClient  *RemoteClient
	validator     *security.Validator
	auditLogger   *security.AuditLogger
	logger        *log.Logger

	// Rate limiting and resource management
	activeProxies     map[string]*proxySession // key: node_address:mountpoint
	activeProxiesMu   sync.RWMutex
	maxProxiesPerIP   int32
	maxProxiesGlobal  int32
	currentProxies    int32

	// Reconnection settings
	maxReconnectAttempts int
	streamTimeout        time.Duration

	// Server configuration
	serverPort int // Port to use for connecting to remote nodes
}

// proxySession tracks an active proxy connection
type proxySession struct {
	nodeAddress    string
	mountpoint     string
	listeners      int32
	createdAt      time.Time
	lastActivity   time.Time
	lastMetadata   string
	streamReader   *StreamReader
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex
	streamError    error       // Tracks stream health
	lastReadTime   time.Time   // Last successful read from stream
}

// ProxyConfig contains configuration for the proxy handler
type ProxyConfig struct {
	MaxProxiesPerIP      int32
	MaxProxiesGlobal     int32
	MaxReconnectAttempts int
	StreamTimeout        time.Duration
	ServerPort           int // Port to use for connecting to remote nodes
}

// NewProxyHandler creates a new proxy handler for federated streams
func NewProxyHandler(
	db *database.DB,
	validator *security.Validator,
	auditLogger *security.AuditLogger,
	logger *log.Logger,
	config *ProxyConfig,
) *ProxyHandler {
	if config == nil {
		config = &ProxyConfig{
			MaxProxiesPerIP:      5,
			MaxProxiesGlobal:     100,
			MaxReconnectAttempts: 3,
			StreamTimeout:        60 * time.Minute, // 1 hour for continuous streaming
			ServerPort:           8080,             // Default port
		}
	}

	return &ProxyHandler{
		db:                   db,
		remoteClient:         NewRemoteClient(validator, logger),
		validator:            validator,
		auditLogger:          auditLogger,
		logger:               logger,
		activeProxies:        make(map[string]*proxySession),
		maxProxiesPerIP:      config.MaxProxiesPerIP,
		maxProxiesGlobal:     config.MaxProxiesGlobal,
		maxReconnectAttempts: config.MaxReconnectAttempts,
		streamTimeout:        config.StreamTimeout,
		serverPort:           config.ServerPort,
	}
}

// ServeHTTP handles GET /stream/federated/{node_address}/{mountpoint}
func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client IP for rate limiting
	clientIP := extractIPv6(r.RemoteAddr)

	// Parse URL path: /stream/federated/{node_address}/{mountpoint}
	// Example: /stream/federated/200:1234:5678::/live.mp3
	path := strings.TrimPrefix(r.URL.Path, "/stream/federated/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) != 2 {
		ph.auditLogger.Log("proxy_invalid_path", "low", "", r.URL.Path, "Invalid proxy path format")
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	nodeAddress := parts[0]
	mountpoint := "/" + parts[1] // Add leading slash for mountpoint

	// Security validation: validate Yggdrasil IPv6 address
	if err := ph.validator.ValidateYggdrasilIPv6(nodeAddress); err != nil {
		ph.auditLogger.Log("proxy_invalid_address", "medium", "", nodeAddress, fmt.Sprintf("Invalid Yggdrasil address: %v", err))
		ph.logger.Printf("SECURITY: Rejected invalid Yggdrasil address: %s - %v", nodeAddress, err)
		http.Error(w, "Invalid node address", http.StatusBadRequest)
		return
	}

	// Security validation: validate mountpoint
	if err := ph.validator.ValidateMountpoint(mountpoint); err != nil {
		ph.auditLogger.Log("proxy_invalid_mountpoint", "medium", "", mountpoint, fmt.Sprintf("Invalid mountpoint: %v", err))
		ph.logger.Printf("SECURITY: Rejected invalid mountpoint: %s - %v", mountpoint, err)
		http.Error(w, "Invalid mountpoint", http.StatusBadRequest)
		return
	}

	// Check global proxy limit
	currentCount := atomic.LoadInt32(&ph.currentProxies)
	if currentCount >= ph.maxProxiesGlobal {
		ph.auditLogger.Log("proxy_global_limit", "medium", "", "", fmt.Sprintf("Global proxy limit reached: %d", currentCount))
		ph.logger.Printf("SECURITY: Global proxy limit reached (%d)", currentCount)
		http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	// Check per-IP limit
	if err := ph.checkIPLimit(clientIP); err != nil {
		ph.auditLogger.Log("proxy_ip_limit", "medium", "", "", err.Error())
		ph.logger.Printf("SECURITY: IP proxy limit exceeded: %v", err)
		http.Error(w, "Too many concurrent proxy connections", http.StatusTooManyRequests)
		return
	}

	// Lookup station in federated cache by IPv6 address
	station, err := ph.db.GetFederatedStationCacheByAddress(nodeAddress, mountpoint)
	if err == sql.ErrNoRows || station == nil {
		ph.auditLogger.Log("proxy_station_not_found", "low", "", fmt.Sprintf("%s%s", nodeAddress, mountpoint), "Station not in federated cache")
		ph.logger.Printf("Station not found in federated cache: %s%s", nodeAddress, mountpoint)
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}
	if err != nil {
		ph.logger.Printf("ERROR: Failed to lookup federated station: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check if station is online
	if station.Status != "online" {
		http.Error(w, "Station is offline", http.StatusServiceUnavailable)
		return
	}

	// Get or create proxy session
	session, err := ph.getOrCreateSession(nodeAddress, mountpoint, ph.serverPort)
	if err != nil {
		ph.logger.Printf("ERROR: Failed to create proxy session: %v", err)
		http.Error(w, "Failed to connect to remote stream", http.StatusBadGateway)
		return
	}

	// Increment global and session listener counts
	atomic.AddInt32(&ph.currentProxies, 1)
	atomic.AddInt32(&session.listeners, 1)
	defer func() {
		atomic.AddInt32(&ph.currentProxies, -1)
		atomic.AddInt32(&session.listeners, -1)

		// Clean up session if no more listeners
		if atomic.LoadInt32(&session.listeners) == 0 {
			ph.cleanupSession(nodeAddress, mountpoint)
		}
	}()

	// Log successful proxy connection (audit only - no console log to reduce noise)
	ph.auditLogger.Log("proxy_connected", "info", "", fmt.Sprintf("%s%s", nodeAddress, mountpoint), "Proxy connection established")

	// Stream audio data to client
	ph.streamToClient(w, r, session, station)
}

// getOrCreateSession retrieves an existing proxy session or creates a new one
func (ph *ProxyHandler) getOrCreateSession(nodeAddress, mountpoint string, port int) (*proxySession, error) {
	key := fmt.Sprintf("%s%s", nodeAddress, mountpoint)

	// Check if session already exists
	ph.activeProxiesMu.RLock()
	session, exists := ph.activeProxies[key]
	ph.activeProxiesMu.RUnlock()

	if exists {
		// Update last activity time
		session.mu.Lock()
		session.lastActivity = time.Now()
		session.mu.Unlock()
		return session, nil
	}

	// Create new session
	ph.activeProxiesMu.Lock()
	defer ph.activeProxiesMu.Unlock()

	// Double-check after acquiring write lock
	if session, exists := ph.activeProxies[key]; exists {
		return session, nil
	}

	// Create context with cancel for long-lived stream (no timeout)
	// Timeout was causing streams to hang after the specified duration
	ctx, cancel := context.WithCancel(context.Background())

	// Connect to remote stream
	streamConfig := &RemoteStreamConfig{
		NodeAddress: nodeAddress,
		NodePort:    port,
		Mountpoint:  mountpoint,
		Timeout:     ph.streamTimeout,
	}

	streamReader, err := ph.remoteClient.Connect(ctx, streamConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to remote stream: %w", err)
	}

	// Create new session
	now := time.Now()
	session = &proxySession{
		nodeAddress:  nodeAddress,
		mountpoint:   mountpoint,
		listeners:    0,
		createdAt:    now,
		lastActivity: now,
		lastReadTime: now,
		lastMetadata: streamReader.GetMetadata(),
		streamReader: streamReader,
		ctx:          ctx,
		cancel:       cancel,
		streamError:  nil,
	}

	ph.activeProxies[key] = session

	ph.logger.Printf("Created new proxy session: %s%s", nodeAddress, mountpoint)

	// Start metadata update worker
	go ph.metadataUpdateWorker(session, nodeAddress, mountpoint)

	// Start health monitor
	go ph.healthMonitor(session, nodeAddress, mountpoint)

	return session, nil
}

// streamToClient streams audio data from remote source to local client
func (ph *ProxyHandler) streamToClient(w http.ResponseWriter, r *http.Request, session *proxySession, station *database.FederatedStation) {
	// Set streaming headers
	w.Header().Set("Content-Type", session.streamReader.ContentType())
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "close")

	// Add station metadata headers (from cache)
	w.Header().Set("icy-name", station.Name)
	if station.Description.Valid {
		w.Header().Set("icy-description", station.Description.String)
	}
	if station.Genre.Valid {
		w.Header().Set("icy-genre", station.Genre.String)
	}
	if station.Bitrate.Valid {
		w.Header().Set("icy-br", fmt.Sprintf("%d", station.Bitrate.Int64))
	}

	// Add current ICY metadata from stream (if available)
	if currentMetadata := session.streamReader.GetMetadata(); currentMetadata != "" {
		w.Header().Set("icy-title", currentMetadata)
	}

	// Write headers
	w.WriteHeader(http.StatusOK)

	// Flush headers immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Create buffer for streaming
	buffer := make([]byte, 8192) // 8KB buffer for audio chunks

	// Stream data from remote to client
	for {
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			ph.logger.Printf("Client disconnected from proxy stream: %s%s", session.nodeAddress, session.mountpoint)
			return
		case <-session.ctx.Done():
			ph.logger.Printf("Proxy session timeout: %s%s", session.nodeAddress, session.mountpoint)
			http.Error(w, "Stream timeout", http.StatusGatewayTimeout)
			return
		default:
		}

		// Read from remote stream
		n, err := session.streamReader.Read(buffer)
		if err != nil {
			// Update session error state
			session.mu.Lock()
			session.streamError = err
			session.mu.Unlock()

			if err == io.EOF {
				// Remote stream ended normally
				ph.logger.Printf("Remote stream ended: %s%s", session.nodeAddress, session.mountpoint)
				return
			}
			// Only log unexpected errors (not timeout/context deadline which are normal)
			if !strings.Contains(err.Error(), "context deadline exceeded") &&
			   !strings.Contains(err.Error(), "context canceled") {
				ph.logger.Printf("ERROR: Failed to read from remote stream: %v", err)
			}
			return
		}

		// Write to client
		if n > 0 {
			_, writeErr := w.Write(buffer[:n])
			if writeErr != nil {
				// Only log unexpected errors (not broken pipe which is normal on client disconnect)
				if !strings.Contains(writeErr.Error(), "broken pipe") &&
				   !strings.Contains(writeErr.Error(), "connection reset") &&
				   !strings.Contains(writeErr.Error(), "forcibly closed") {
					ph.logger.Printf("ERROR: Failed to write to client: %v", writeErr)
				}
				return
			}

			// Flush data to client
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

			// Update last activity and last read time
			now := time.Now()
			session.mu.Lock()
			session.lastActivity = now
			session.lastReadTime = now
			session.streamError = nil // Clear error on successful read
			session.mu.Unlock()
		}
	}
}

// cleanupSession removes and closes a proxy session
func (ph *ProxyHandler) cleanupSession(nodeAddress, mountpoint string) {
	key := fmt.Sprintf("%s%s", nodeAddress, mountpoint)

	ph.activeProxiesMu.Lock()
	defer ph.activeProxiesMu.Unlock()

	session, exists := ph.activeProxies[key]
	if !exists {
		return
	}

	// Close stream reader
	if session.streamReader != nil {
		session.streamReader.Close()
	}

	// Cancel context
	if session.cancel != nil {
		session.cancel()
	}

	// Remove from active sessions
	delete(ph.activeProxies, key)

	ph.logger.Printf("Cleaned up proxy session: %s%s", nodeAddress, mountpoint)
}

// checkIPLimit checks if the client IP has exceeded the per-IP proxy limit
func (ph *ProxyHandler) checkIPLimit(clientIP string) error {
	ph.activeProxiesMu.RLock()
	defer ph.activeProxiesMu.RUnlock()

	// Count active proxies for this IP
	// Note: This is a simplified check. In production, you might want to track
	// listeners per IP more precisely
	count := int32(0)
	for _, session := range ph.activeProxies {
		listeners := atomic.LoadInt32(&session.listeners)
		if listeners > 0 {
			count++
		}
	}

	if count >= ph.maxProxiesPerIP {
		return fmt.Errorf("IP has %d active proxy connections (limit: %d)", count, ph.maxProxiesPerIP)
	}

	return nil
}

// GetStats returns statistics about active proxy sessions
func (ph *ProxyHandler) GetStats() map[string]interface{} {
	ph.activeProxiesMu.RLock()
	defer ph.activeProxiesMu.RUnlock()

	totalListeners := int32(0)
	for _, session := range ph.activeProxies {
		totalListeners += atomic.LoadInt32(&session.listeners)
	}

	return map[string]interface{}{
		"active_sessions":  len(ph.activeProxies),
		"total_listeners":  totalListeners,
		"current_proxies":  atomic.LoadInt32(&ph.currentProxies),
		"max_proxies":      ph.maxProxiesGlobal,
	}
}

// metadataUpdateWorker periodically updates metadata in the cache database
func (ph *ProxyHandler) metadataUpdateWorker(session *proxySession, nodeAddress, mountpoint string) {
	ticker := time.NewTicker(2 * time.Second) // Update every 2 seconds for faster metadata refresh
	defer ticker.Stop()

	consecutiveFailures := 0
	const maxFailures = 5

	for {
		select {
		case <-session.ctx.Done():
			return
		case <-ticker.C:
			// Check if stream is still healthy
			session.mu.RLock()
			streamErr := session.streamError
			lastRead := session.lastReadTime
			session.mu.RUnlock()

			// If stream has errors or hasn't received data in 30 seconds, stop updating
			if streamErr != nil || time.Since(lastRead) > 30*time.Second {
				ph.logger.Printf("WARNING: Stream unhealthy for %s%s, stopping metadata updates", nodeAddress, mountpoint)
				return
			}

			// Fetch fresh metadata from the remote station's API
			freshMetadata := ph.fetchRemoteMetadata(nodeAddress, mountpoint)

			// If we got fresh metadata, update it
			if freshMetadata != "" {
				consecutiveFailures = 0 // Reset failure counter on success

				session.mu.Lock()
				changed := freshMetadata != session.lastMetadata
				if changed {
					session.lastMetadata = freshMetadata
					// Also update the stream reader's metadata
					session.streamReader.SetMetadata(freshMetadata)
				}
				session.mu.Unlock()

				// Update in database if changed
				if changed {
					// Try to update in federated_station_cache
					station, err := ph.db.GetFederatedStationCacheByAddress(nodeAddress, mountpoint)
					if err == nil && station != nil {
						// Update metadata field
						station.MetadataTitle.String = freshMetadata
						station.MetadataTitle.Valid = true

						// Upsert back to cache
						if err := ph.db.UpsertFederatedStationCache(station); err != nil {
							ph.logger.Printf("WARNING: Failed to update metadata in cache: %v", err)
						} else {
							ph.logger.Printf("Updated metadata for %s%s: %s", nodeAddress, mountpoint, freshMetadata)
						}
					}
				}
			} else {
				// Track consecutive failures
				consecutiveFailures++
				if consecutiveFailures >= maxFailures {
					ph.logger.Printf("WARNING: Too many metadata fetch failures for %s%s, stopping updates", nodeAddress, mountpoint)
					return
				}
			}
		}
	}
}

// fetchRemoteMetadata fetches current metadata from the remote station's API
func (ph *ProxyHandler) fetchRemoteMetadata(nodeAddress, mountpoint string) string {
	// Get station info from local cache first
	station, err := ph.db.GetFederatedStationCacheByAddress(nodeAddress, mountpoint)
	if err != nil || station == nil {
		return ""
	}

	// Make a quick HEAD request to the remote stream to get current ICY headers
	// This is much faster than fetching from federation server
	url := fmt.Sprintf("http://[%s]:%d/api/stations", nodeAddress, ph.serverPort)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		// Fallback to cached metadata
		if station.MetadataTitle.Valid {
			return station.MetadataTitle.String
		}
		return ""
	}

	// Create a temporary HTTP client for the metadata request
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Fallback to cached metadata
		if station.MetadataTitle.Valid {
			return station.MetadataTitle.String
		}
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fallback to cached metadata
		if station.MetadataTitle.Valid {
			return station.MetadataTitle.String
		}
		return ""
	}

	// Parse the JSON response to find our station
	var stationsResp struct {
		Stations []struct {
			Mountpoint    string `json:"mountpoint"`
			MetadataTitle string `json:"metadata_title"`
		} `json:"stations"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&stationsResp); err != nil {
		// Fallback to cached metadata
		if station.MetadataTitle.Valid {
			return station.MetadataTitle.String
		}
		return ""
	}

	// Find our station in the response
	for _, s := range stationsResp.Stations {
		if s.Mountpoint == mountpoint && s.MetadataTitle != "" {
			return s.MetadataTitle
		}
	}

	// Fallback to cached metadata
	if station.MetadataTitle.Valid {
		return station.MetadataTitle.String
	}

	return ""
}

// healthMonitor monitors the health of a proxy session and cleans up if unhealthy
func (ph *ProxyHandler) healthMonitor(session *proxySession, nodeAddress, mountpoint string) {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	const maxIdleTime = 2 * time.Minute // Max time without data before considering stream dead

	for {
		select {
		case <-session.ctx.Done():
			return
		case <-ticker.C:
			session.mu.RLock()
			lastRead := session.lastReadTime
			listeners := atomic.LoadInt32(&session.listeners)
			streamErr := session.streamError
			session.mu.RUnlock()

			// If no listeners and idle for more than 1 minute, clean up
			if listeners == 0 && time.Since(lastRead) > time.Minute {
				ph.logger.Printf("Cleaning up idle proxy session with no listeners: %s%s", nodeAddress, mountpoint)
				ph.cleanupSession(nodeAddress, mountpoint)
				return
			}

			// If stream hasn't received data in maxIdleTime, consider it dead
			if time.Since(lastRead) > maxIdleTime {
				ph.logger.Printf("ERROR: Stream dead (no data for %v): %s%s", maxIdleTime, nodeAddress, mountpoint)
				ph.cleanupSession(nodeAddress, mountpoint)
				return
			}

			// If stream has persistent errors, clean up
			if streamErr != nil && time.Since(lastRead) > 30*time.Second {
				ph.logger.Printf("ERROR: Stream unhealthy (error: %v): %s%s", streamErr, nodeAddress, mountpoint)
				ph.cleanupSession(nodeAddress, mountpoint)
				return
			}
		}
	}
}

// Close shuts down the proxy handler and closes all active sessions
func (ph *ProxyHandler) Close() error {
	ph.logger.Println("Shutting down proxy handler...")

	ph.activeProxiesMu.Lock()
	defer ph.activeProxiesMu.Unlock()

	sessionCount := len(ph.activeProxies)
	if sessionCount > 0 {
		ph.logger.Printf("Closing %d active proxy sessions...", sessionCount)
	}

	// Close all active sessions
	for key, session := range ph.activeProxies {
		if session.streamReader != nil {
			session.streamReader.Close()
		}
		if session.cancel != nil {
			session.cancel()
		}
		delete(ph.activeProxies, key)
	}

	// Close remote client
	if ph.remoteClient != nil {
		ph.remoteClient.Close()
	}

	ph.logger.Printf("Proxy handler closed (%d sessions terminated)", sessionCount)
	return nil
}