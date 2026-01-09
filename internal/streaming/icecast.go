package streaming

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/api/middleware"
	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// Server represents the Icecast-compatible streaming server
type Server struct {
	db              *database.DB
	config          *config.StreamingConfig
	rateLimitConfig *config.RateLimitConfig
	mpManager       *MountpointManager
	sourceHandler   *SourceHandler
	listenerHandler *ListenerHandler
	rateLimiter     *middleware.RateLimiter
	validator       *security.Validator
	sanitizer       *security.Sanitizer
	auditLogger     *security.AuditLogger
	logger          *log.Logger

	// Server state
	running bool
	mu      sync.RWMutex
}

// NewServer creates a new streaming server
func NewServer(
	db *database.DB,
	cfg *config.StreamingConfig,
	rateLimitCfg *config.RateLimitConfig,
	auditLogger *security.AuditLogger,
	logger *log.Logger,
	serverPort int,
) *Server {
	// Create mountpoint manager
	mpManager := NewMountpointManager(logger)

	// Create validator and sanitizer
	validator := security.NewValidator()
	sanitizer := security.NewSanitizer()

	// Create rate limiter
	rateLimiter := middleware.NewRateLimiter(
		rateLimitCfg.APIRequestsPerMinute,
		rateLimitCfg.AuthAttemptsPerMinute,
	)

	// Create source handler
	sourceHandler := NewSourceHandler(
		db,
		mpManager,
		cfg,
		validator,
		sanitizer,
		auditLogger,
		logger,
	)

	// Create listener handler
	listenerHandler := NewListenerHandler(
		db,
		mpManager,
		cfg,
		validator,
		sanitizer,
		auditLogger,
		logger,
	)

	return &Server{
		db:              db,
		config:          cfg,
		rateLimitConfig: rateLimitCfg,
		mpManager:       mpManager,
		sourceHandler:   sourceHandler,
		listenerHandler: listenerHandler,
		rateLimiter:     rateLimiter,
		validator:       validator,
		sanitizer:       sanitizer,
		auditLogger:     auditLogger,
		logger:          logger,
		running:         false,
	}
}

// ServeHTTP implements http.Handler interface
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set security headers for all responses
	s.setSecurityHeaders(w)

	// Note: Source connections are logged in source.go, listener connections don't need logging
	// to reduce log noise. Only log unusual methods.
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPut && r.Method != "SOURCE" {
		s.logger.Printf("Streaming request: %s %s", r.Method, r.URL.Path)
	}

	// Handle different request types based on HTTP method
	switch r.Method {
	case http.MethodPut, "SOURCE":
		// Source client connection (broadcaster)
		s.handleSourceRequest(w, r)

	case http.MethodGet:
		// Check if this is a playlist request - NOT SUPPORTED
		if strings.HasSuffix(r.URL.Path, ".m3u") || strings.HasSuffix(r.URL.Path, ".m3u8") || strings.HasSuffix(r.URL.Path, ".pls") {
			http.Error(w, "Playlist files not supported. Use web player.", http.StatusNotFound)
			return
		}

		// Check if this is a metadata update request
		if r.URL.Query().Get("mode") == "updinfo" || r.URL.Query().Get("song") != "" {
			s.sourceHandler.HandleMetadataUpdate(w, r)
			return
		}

		// Regular listener connection
		s.handleListenerRequest(w, r)

	case http.MethodHead:
		// HEAD request - return stream info without data
		s.handleHeadRequest(w, r)

	case http.MethodOptions:
		// CORS preflight
		s.handleOptionsRequest(w, r)

	default:
		s.auditLogger.Log(
			security.EventInvalidInput,
			security.SeverityLow,
			"",
			r.URL.Path,
			fmt.Sprintf("Unsupported HTTP method: %s", r.Method),
		)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSourceRequest handles source client (broadcaster) connections
func (s *Server) handleSourceRequest(w http.ResponseWriter, r *http.Request) {
	clientIP := extractIPv6(r.RemoteAddr)

	// Rate limiting for source connections
	if !s.rateLimiter.AllowSource(clientIP) {
		s.auditLogger.Log(
			security.EventRateLimit,
			security.SeverityMedium,
			"",
			r.URL.Path,
			"Source connection rate limit exceeded",
		)
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// Check total source connections limit
	sourceStats := s.sourceHandler.GetSourceStats()
	activeSources := sourceStats["active_sources"].(int)
	if activeSources >= s.config.MaxSourceClients {
		s.auditLogger.Log(
			security.EventRateLimit,
			security.SeverityMedium,
			"",
			r.URL.Path,
			fmt.Sprintf("Max source clients reached (%d)", s.config.MaxSourceClients),
		)
		http.Error(w, "Maximum source connections reached", http.StatusServiceUnavailable)
		return
	}

	// Additional validation
	if err := s.sourceHandler.ValidateSourceRequest(r); err != nil {
		s.logger.Printf("Source request validation failed: %v", err)
	}

	// Handle source connection
	s.sourceHandler.HandleSource(w, r)
}

// handleListenerRequest handles listener (client) connections
func (s *Server) handleListenerRequest(w http.ResponseWriter, r *http.Request) {
	clientIP := extractIPv6(r.RemoteAddr)

	// Rate limiting for listener connections
	if !s.rateLimiter.AllowListener(clientIP) {
		s.auditLogger.Log(
			security.EventRateLimit,
			security.SeverityLow,
			"",
			r.URL.Path,
			"Listener connection rate limit exceeded",
		)
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// Additional validation
	if err := s.listenerHandler.ValidateRequest(r); err != nil {
		s.logger.Printf("Listener request validation warning: %v", err)
	}

	// Handle listener connection
	s.listenerHandler.HandleListener(w, r)
}

// handleHeadRequest handles HEAD requests for stream information
func (s *Server) handleHeadRequest(w http.ResponseWriter, r *http.Request) {
	mountpointPath := s.sanitizer.SanitizeMountpointPath(r.URL.Path)
	if mountpointPath == "" {
		http.Error(w, "Invalid mountpoint", http.StatusBadRequest)
		return
	}

	// Get mountpoint
	mp, exists := s.mpManager.Get(mountpointPath)
	if !exists {
		http.Error(w, "Mountpoint not found", http.StatusNotFound)
		return
	}

	// Set headers with stream information (includes CORS headers)
	s.listenerHandler.setListenerHeaders(w, mp)

	// Add additional info headers
	w.Header().Set("X-Stream-Status", formatStatus(mp.IsOnline()))
	w.Header().Set("X-Listener-Count", fmt.Sprintf("%d", mp.GetListenerCount()))

	w.WriteHeader(http.StatusOK)
}

// handleOptionsRequest handles CORS preflight requests (simplified, no ICY headers)
func (s *Server) handleOptionsRequest(w http.ResponseWriter, r *http.Request) {
	// CORS headers for browser playback
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Max-Age", "86400")

	// Expose standard headers
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type, Content-Length")

	w.WriteHeader(http.StatusNoContent)
}

// setSecurityHeaders sets security headers for all responses
func (s *Server) setSecurityHeaders(w http.ResponseWriter) {
	// Prevent clickjacking
	w.Header().Set("X-Frame-Options", "DENY")

	// XSS protection
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	// Referrer policy
	w.Header().Set("Referrer-Policy", "no-referrer")

	// Don't send server version
	w.Header().Set("Server", "YggRadio")
}

// Start starts the streaming server
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	s.running = true
	s.logger.Println("Streaming server started")

	// Start background cleanup tasks
	go s.runCleanupTasks()

	return nil
}

// Stop stops the streaming server
func (s *Server) Stop(gracePeriod time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("server not running")
	}

	s.logger.Printf("Stopping streaming server (grace period: %v)...", gracePeriod)

	// Broadcast shutdown to listeners
	s.listenerHandler.BroadcastShutdown(gracePeriod)

	// Close all mountpoints
	s.mpManager.CloseAll()

	s.running = false
	s.logger.Println("Streaming server stopped")

	return nil
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// runCleanupTasks runs periodic cleanup tasks
func (s *Server) runCleanupTasks() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()

		if !running {
			return
		}

		// Cleanup tasks
		s.cleanupStaleConnections()
		s.updateStationStatistics()
	}
}

// cleanupStaleConnections removes stale connections
func (s *Server) cleanupStaleConnections() {
	// This could check for listeners that haven't received data in a while
	// or sources that haven't sent data
	// Removed log to reduce noise

	// Update offline status for mountpoints without sources
	for _, mp := range s.mpManager.List() {
		if !mp.IsOnline() {
			if err := s.db.UpdateStationStatus(mp.GetPath(), "offline"); err != nil {
				s.logger.Printf("Error updating station status: %v", err)
			}
		}
	}
}

// updateStationStatistics updates station statistics in the database
func (s *Server) updateStationStatistics() {
	for _, mp := range s.mpManager.List() {
		// Update listener count
		count := mp.GetListenerCount()
		if err := s.db.UpdateStationListenerCount(mp.GetPath(), count); err != nil {
			s.logger.Printf("Error updating listener count for %s: %v", mp.GetPath(), err)
		}
	}
}

// GetStats returns comprehensive server statistics
func (s *Server) GetStats() map[string]interface{} {
	sourceStats := s.sourceHandler.GetSourceStats()
	listenerStats := s.listenerHandler.GetListenerStats()

	return map[string]interface{}{
		"running":         s.IsRunning(),
		"total_sources":   sourceStats["active_sources"],
		"total_listeners": listenerStats["total_listeners"],
		"mountpoints":     len(s.mpManager.List()),
		"sources":         sourceStats["sources"],
		"listener_stats":  listenerStats,
	}
}

// GetMountpointStats returns statistics for a specific mountpoint
func (s *Server) GetMountpointStats(mountpoint string) (map[string]interface{}, error) {
	mp, exists := s.mpManager.Get(mountpoint)
	if !exists {
		return nil, fmt.Errorf("mountpoint not found: %s", mountpoint)
	}

	return mp.GetStats(), nil
}

// ListMountpoints returns a list of all mountpoints
func (s *Server) ListMountpoints() []string {
	var paths []string
	for _, mp := range s.mpManager.List() {
		paths = append(paths, mp.GetPath())
	}
	return paths
}

// DisconnectSource forcefully disconnects a source
func (s *Server) DisconnectSource(mountpoint string) error {
	return s.sourceHandler.DisconnectSource(mountpoint)
}

// DisconnectListener forcefully disconnects a listener
func (s *Server) DisconnectListener(listenerID string) error {
	return s.listenerHandler.DisconnectListener(listenerID)
}

// GetActiveListeners returns information about active listeners
func (s *Server) GetActiveListeners() []ListenerInfo {
	return s.listenerHandler.GetActiveListeners()
}

// UpdateMetadata updates metadata for a mountpoint
func (s *Server) UpdateMetadata(mountpoint, metadata string) error {
	mp, exists := s.mpManager.Get(mountpoint)
	if !exists {
		return fmt.Errorf("mountpoint not found: %s", mountpoint)
	}

	// Sanitize metadata
	metadata = s.sanitizer.SanitizeString(metadata)

	// Update in memory
	mp.UpdateMetadata(metadata)

	// Update database
	if err := s.db.UpdateStationMetadata(mountpoint, metadata); err != nil {
		return fmt.Errorf("failed to update metadata in database: %w", err)
	}

	s.logger.Printf("Metadata updated for %s: %s", mountpoint, metadata)
	return nil
}

// HealthCheck performs a health check on the streaming server
func (s *Server) HealthCheck() error {
	if !s.IsRunning() {
		return fmt.Errorf("server not running")
	}

	// Check database connectivity
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Println("Initiating graceful shutdown...")

	// Determine grace period from context deadline
	gracePeriod := 30 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		gracePeriod = time.Until(deadline)
	}

	// Stop the server
	if err := s.Stop(gracePeriod); err != nil {
		return err
	}

	s.logger.Println("Shutdown complete")
	return nil
}

// GetMountpointManager returns the mountpoint manager (for advanced usage)
func (s *Server) GetMountpointManager() *MountpointManager {
	return s.mpManager
}

// formatStatus formats a boolean status as a string
func formatStatus(online bool) string {
	if online {
		return "online"
	}
	return "offline"
}

// StreamingStats represents comprehensive streaming statistics
type StreamingStats struct {
	TotalSources   int                      `json:"total_sources"`
	TotalListeners int                      `json:"total_listeners"`
	Mountpoints    []MountpointStats        `json:"mountpoints"`
	Uptime         time.Duration            `json:"uptime"`
	ServerStatus   string                   `json:"server_status"`
}

// MountpointStats represents statistics for a single mountpoint
type MountpointStats struct {
	Path         string `json:"path"`
	StationID    int64  `json:"station_id"`
	Online       bool   `json:"online"`
	Listeners    int    `json:"listeners"`
	ContentType  string `json:"content_type"`
	Bitrate      int    `json:"bitrate"`
	CurrentTrack string `json:"current_track"`
}

// GetDetailedStats returns detailed statistics in a structured format
func (s *Server) GetDetailedStats() *StreamingStats {
	var totalListeners int
	var totalSources int
	var mountpointStats []MountpointStats

	for _, mp := range s.mpManager.List() {
		stats := mp.GetStats()

		mpStat := MountpointStats{
			Path:         stats["path"].(string),
			StationID:    stats["station_id"].(int64),
			Online:       stats["online"].(bool),
			Listeners:    stats["listeners"].(int),
			ContentType:  stats["content_type"].(string),
			Bitrate:      stats["bitrate"].(int),
			CurrentTrack: stats["current_track"].(string),
		}

		totalListeners += mpStat.Listeners
		if mpStat.Online {
			totalSources++
		}

		mountpointStats = append(mountpointStats, mpStat)
	}

	status := "running"
	if !s.IsRunning() {
		status = "stopped"
	}

	return &StreamingStats{
		TotalSources:   totalSources,
		TotalListeners: totalListeners,
		Mountpoints:    mountpointStats,
		ServerStatus:   status,
	}
}
