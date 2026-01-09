package streaming

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// ListenerHandler handles incoming listener connections (simplified for browser-only playback)
type ListenerHandler struct {
	db          *database.DB
	mpManager   *MountpointManager
	config      *config.StreamingConfig
	validator   *security.Validator
	sanitizer   *security.Sanitizer
	auditLogger *security.AuditLogger
	logger      *log.Logger
}

// NewListenerHandler creates a new listener handler
func NewListenerHandler(
	db *database.DB,
	mpManager *MountpointManager,
	cfg *config.StreamingConfig,
	validator *security.Validator,
	sanitizer *security.Sanitizer,
	auditLogger *security.AuditLogger,
	logger *log.Logger,
) *ListenerHandler {
	return &ListenerHandler{
		db:          db,
		mpManager:   mpManager,
		config:      cfg,
		validator:   validator,
		sanitizer:   sanitizer,
		auditLogger: auditLogger,
		logger:      logger,
	}
}

// HandleListener handles a listener connection request (simplified for browsers only)
func (h *ListenerHandler) HandleListener(w http.ResponseWriter, r *http.Request) {
	// 1. Extract mountpoint path
	mountpointPath := r.URL.Path
	if mountpointPath == "" || mountpointPath == "/" {
		http.Error(w, "Invalid mountpoint", http.StatusBadRequest)
		return
	}

	// 2. Get mountpoint
	mountpoint, exists := h.mpManager.Get(mountpointPath)
	if !exists {
		http.Error(w, "Mountpoint not found", http.StatusNotFound)
		return
	}

	// 3. Log connection (privacy: no IP logging)
	h.logger.Printf("New listener connection to %s", mountpointPath)

	// 4. Set HTTP headers for audio streaming
	h.setListenerHeaders(w, mountpoint)

	// 5. Send HTTP 200 OK
	w.WriteHeader(http.StatusOK)

	// Flush headers
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// 6. Add listener to mountpoint
	listener := mountpoint.AddListener()
	defer mountpoint.RemoveListener(listener)

	h.logger.Printf("Listener added to %s (ID: %s)", mountpointPath, listener.ID)

	// 7. Update listener count in database
	if err := h.db.UpdateStationListenerCount(mountpointPath, mountpoint.GetListenerCount()); err != nil {
		h.logger.Printf("Failed to update listener count: %v", err)
	}
	defer func() {
		if err := h.db.UpdateStationListenerCount(mountpointPath, mountpoint.GetListenerCount()); err != nil {
			h.logger.Printf("Failed to update listener count on disconnect: %v", err)
		}
	}()

	// 8. Stream audio (simple, no metadata injection)
	h.streamAudio(r.Context(), w, listener)

	h.logger.Printf("Listener disconnected from %s (duration: %v)",
		mountpointPath, time.Since(listener.ConnectedAt))
}

// streamAudio streams audio data to the listener (simplified, no ICY metadata)
func (h *ListenerHandler) streamAudio(ctx context.Context, w http.ResponseWriter, listener *Listener) {
	flusher, canFlush := w.(http.Flusher)
	chunkCount := 0

	for {
		select {
		case chunk, ok := <-listener.Chan:
			if !ok {
				// Channel closed, listener removed
				return
			}

			// Write audio chunk directly (no metadata injection)
			_, err := w.Write(chunk)
			if err != nil {
				h.logger.Printf("Write error: %v", err)
				return
			}

			// Flush every 5 chunks instead of every chunk
			// This reduces system calls and allows browser to buffer more efficiently
			// 5 chunks = ~20KB, good balance between latency and efficiency
			chunkCount++
			if canFlush && chunkCount%5 == 0 {
				flusher.Flush()
			}

		case <-ctx.Done():
			// Client disconnected
			return
		}
	}
}

// setListenerHeaders sets HTTP headers for listener response (simplified, no ICY headers)
func (h *ListenerHandler) setListenerHeaders(w http.ResponseWriter, mp *Mountpoint) {
	// Set content type from mountpoint
	contentType := mp.ContentType()
	if contentType == "" {
		contentType = "audio/mpeg" // Default to MP3
	}
	w.Header().Set("Content-Type", contentType)

	// Standard HTTP headers
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Accept-Ranges", "none")

	// CORS headers for browser playback
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type, Content-Length")
}

// GetListenerStats returns statistics about active listeners
func (h *ListenerHandler) GetListenerStats() map[string]interface{} {
	stats := make(map[string]interface{})

	allMountpoints := h.mpManager.List()
	totalListeners := 0

	for _, mp := range allMountpoints {
		totalListeners += mp.GetListenerCount()
	}

	stats["total_listeners"] = totalListeners
	stats["mountpoints_with_listeners"] = len(allMountpoints)

	return stats
}

// GetActiveListeners returns a list of all active listeners across all mountpoints
func (h *ListenerHandler) GetActiveListeners() []ListenerInfo {
	var listeners []ListenerInfo

	allMountpoints := h.mpManager.List()

	for _, mp := range allMountpoints {
		mp.listenersMu.RLock()
		for _, listener := range mp.listeners {
			listeners = append(listeners, ListenerInfo{
				ID:          listener.ID,
				Mountpoint:  mp.GetPath(),
				ConnectedAt: listener.ConnectedAt,
				BytesSent:   listener.BytesSent,
			})
		}
		mp.listenersMu.RUnlock()
	}

	return listeners
}

// ListenerInfo contains information about an active listener
type ListenerInfo struct {
	ID          string
	Mountpoint  string
	ConnectedAt time.Time
	BytesSent   int64
}

// DisconnectListener disconnects a listener by ID
func (h *ListenerHandler) DisconnectListener(listenerID string) error {
	allMountpoints := h.mpManager.List()

	for _, mp := range allMountpoints {
		mp.listenersMu.Lock()
		listener, exists := mp.listeners[listenerID]
		if exists {
			// SECURITY FIX: Remove from map BEFORE closing channel to prevent race condition
			// This ensures no goroutine can send to the channel after we close it
			delete(mp.listeners, listenerID)
			mp.listenersMu.Unlock()

			// Now safe to close the channel - no one has access to it anymore
			close(listener.Chan)
			h.logger.Printf("Manually disconnected listener %s", listenerID)
			return nil
		}
		mp.listenersMu.Unlock()
	}

	return fmt.Errorf("listener %s not found", listenerID)
}

// BroadcastShutdown broadcasts a shutdown message to all listeners
func (h *ListenerHandler) BroadcastShutdown(gracePeriod time.Duration) {
	h.logger.Printf("Broadcasting shutdown to all listeners (grace period: %v)", gracePeriod)

	allMountpoints := h.mpManager.List()

	for _, mp := range allMountpoints {
		mp.listenersMu.RLock()
		listenerCount := len(mp.listeners)
		mp.listenersMu.RUnlock()

		if listenerCount > 0 {
			h.logger.Printf("Notifying %d listeners on mountpoint %s", listenerCount, mp.GetPath())
		}
	}

	// Wait for grace period
	time.Sleep(gracePeriod)
}

// ValidateRequest validates an incoming listener request
func (h *ListenerHandler) ValidateRequest(r *http.Request) error {
	// Basic validation
	if r.Method != http.MethodGet {
		return fmt.Errorf("invalid method: %s", r.Method)
	}

	return nil
}

// extractIPv6 extracts IPv6 address from RemoteAddr
func extractIPv6(remoteAddr string) string {
	// RemoteAddr format: "[ipv6]:port"
	if strings.HasPrefix(remoteAddr, "[") {
		// IPv6 format: [2001:db8::1]:12345
		endBracket := strings.Index(remoteAddr, "]")
		if endBracket > 0 {
			return remoteAddr[1:endBracket]
		}
	}

	// IPv4 format or invalid format
	colonIdx := strings.LastIndex(remoteAddr, ":")
	if colonIdx > 0 {
		return remoteAddr[:colonIdx]
	}

	return remoteAddr
}
