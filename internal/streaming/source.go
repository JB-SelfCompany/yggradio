package streaming

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// SourceHandler handles incoming source client connections (broadcasters)
type SourceHandler struct {
	db          *database.DB
	mpManager   *MountpointManager
	config      *config.StreamingConfig
	validator   *security.Validator
	sanitizer   *security.Sanitizer
	auditLogger *security.AuditLogger
	logger      *log.Logger
}

// NewSourceHandler creates a new source handler
func NewSourceHandler(
	db *database.DB,
	mpManager *MountpointManager,
	cfg *config.StreamingConfig,
	validator *security.Validator,
	sanitizer *security.Sanitizer,
	auditLogger *security.AuditLogger,
	logger *log.Logger,
) *SourceHandler {
	return &SourceHandler{
		db:          db,
		mpManager:   mpManager,
		config:      cfg,
		validator:   validator,
		sanitizer:   sanitizer,
		auditLogger: auditLogger,
		logger:      logger,
	}
}

// HandleSource handles a source client connection (PUT or SOURCE method)
func (h *SourceHandler) HandleSource(w http.ResponseWriter, r *http.Request) {
	// 1. Validate mountpoint path (protect against path traversal)
	mountpointPath := h.sanitizer.SanitizeMountpointPath(r.URL.Path)
	if mountpointPath == "" {
		h.auditLogger.Log(
			security.EventInvalidInput,
			security.SeverityMedium,
			"",
			r.URL.Path,
			"Invalid mountpoint path - possible path traversal attempt",
		)
		http.Error(w, "Invalid mountpoint", http.StatusBadRequest)
		return
	}

	// 2. Validate mountpoint path format
	if err := h.validator.ValidateMountpoint(mountpointPath); err != nil {
		h.auditLogger.Log(
			security.EventInvalidInput,
			security.SeverityMedium,
			"",
			mountpointPath,
			fmt.Sprintf("Mountpoint validation failed: %v", err),
		)
		http.Error(w, "Invalid mountpoint format", http.StatusBadRequest)
		return
	}

	// 3. Authenticate source client (HTTP Basic Auth)
	username, password, ok := r.BasicAuth()
	if !ok {
		h.auditLogger.Log(
			security.EventAuthFailure,
			security.SeverityLow,
			"",
			mountpointPath,
			"Missing Basic Authentication",
		)
		w.Header().Set("WWW-Authenticate", `Basic realm="YggRadio Source"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 4. Validate credentials and get station
	station, err := h.authenticateSource(username, password, mountpointPath)
	if err != nil {
		h.auditLogger.Log(
			security.EventAuthFailure,
			security.SeverityMedium,
			"",
			mountpointPath,
			fmt.Sprintf("Authentication failed: %v", err),
		)
		w.Header().Set("WWW-Authenticate", `Basic realm="YggRadio Source"`)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// 5. Validate Content-Type header
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		// Default to MP3 if not specified (common for legacy clients)
		contentType = "audio/mpeg"
	}

	allowedTypes := []string{
		"audio/mpeg",
		"audio/ogg",
		"audio/opus",
		"audio/aac",
		"audio/flac",
		"application/ogg",
		"audio/mp4",
	}

	if err := h.validator.ValidateContentType(contentType, allowedTypes); err != nil {
		h.auditLogger.Log(
			security.EventInvalidInput,
			security.SeverityLow,
			station.OwnerPubkey,
			mountpointPath,
			fmt.Sprintf("Unsupported content type: %s", contentType),
		)
		http.Error(w, "Content-Type not supported", http.StatusUnsupportedMediaType)
		return
	}

	// SECURITY FIX: Validate actual content using magic bytes to prevent content-type spoofing
	// This prevents attackers from sending non-audio data disguised as audio
	bufferedReader := bufio.NewReaderSize(r.Body, 32768) // 32KB buffer for reading magic bytes
	magicBytes, err := bufferedReader.Peek(512)           // Read first 512 bytes to check magic bytes
	if err != nil && err != io.EOF {
		h.auditLogger.Log(
			security.EventInvalidInput,
			security.SeverityMedium,
			station.OwnerPubkey,
			mountpointPath,
			fmt.Sprintf("Failed to read stream data for validation: %v", err),
		)
		http.Error(w, "Failed to read stream data", http.StatusBadRequest)
		return
	}

	// Validate magic bytes match an audio format
	detectedType, err := h.validator.ValidateAudioMagicBytes(magicBytes)
	if err != nil {
		h.auditLogger.Log(
			security.EventInvalidInput,
			security.SeverityHigh,
			station.OwnerPubkey,
			mountpointPath,
			fmt.Sprintf("Magic bytes validation failed: %v (claimed type: %s)", err, contentType),
		)
		http.Error(w, "Stream data is not a valid audio format", http.StatusUnsupportedMediaType)
		return
	}

	// Log if detected type differs from claimed type (informational, not blocking)
	if detectedType != contentType {
		h.logger.Printf("Content-Type mismatch for %s: header claims %s but magic bytes indicate %s (using detected type)",
			mountpointPath, contentType, detectedType)
		contentType = detectedType // Use the detected type for accuracy
	}

	// 6. Parse bitrate from headers (if provided)
	bitrate := 0
	if icyBr := r.Header.Get("ice-bitrate"); icyBr != "" {
		fmt.Sscanf(icyBr, "%d", &bitrate)
	} else if icyBr := r.Header.Get("icy-br"); icyBr != "" {
		fmt.Sscanf(icyBr, "%d", &bitrate)
	}

	// 7. Extract metadata from ICY headers (if provided)
	metadata := ""
	if icyTitle := r.Header.Get("ice-title"); icyTitle != "" {
		metadata = h.sanitizer.SanitizeString(icyTitle)
	} else if icyTitle := r.Header.Get("icy-title"); icyTitle != "" {
		metadata = h.sanitizer.SanitizeString(icyTitle)
	} else if icyName := r.Header.Get("ice-name"); icyName != "" {
		metadata = h.sanitizer.SanitizeString(icyName)
	} else if icyName := r.Header.Get("icy-name"); icyName != "" {
		metadata = h.sanitizer.SanitizeString(icyName)
	}

	// Update metadata in database if provided
	if metadata != "" {
		if err := h.db.UpdateStationMetadata(mountpointPath, metadata); err != nil {
			h.logger.Printf("Failed to update initial metadata: %v", err)
		} else {
			h.logger.Printf("Initial metadata set for %s: %s", mountpointPath, metadata)
		}
	}

	// 8. Get or create mountpoint
	genre := ""
	if station.Genre.Valid {
		genre = station.Genre.String
	}
	mountpoint := h.mpManager.GetOrCreate(
		mountpointPath,
		station.ID,
		station.Name,
		genre,
		contentType,
		bitrate,
	)

	// 9. Set security headers (before any status responses)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	// 10. Set up source stream with metadata extraction
	// Wrap the buffered reader with metadata extractor for automatic metadata parsing
	// SECURITY: Use bufferedReader instead of r.Body (already validated magic bytes)
	metadataExtractor := NewMetadataExtractor(
		bufferedReader,
		h.db,
		mountpointPath,
		h.sanitizer,
		h.logger,
	)

	// 11. Atomically check and set source to prevent race condition
	// SECURITY: TrySetSource performs check and set in single atomic operation
	ok, err = mountpoint.TrySetSource(metadataExtractor, contentType)
	if !ok {
		// Mountpoint already has active source
		h.auditLogger.Log(
			security.EventInvalidInput,
			security.SeverityMedium,
			station.OwnerPubkey,
			mountpointPath,
			fmt.Sprintf("Mountpoint already has active source: %v", err),
		)
		http.Error(w, "Mountpoint already in use", http.StatusConflict)
		return
	}

	// 12. Send HTTP 200 OK to begin streaming (after successful source assignment)
	w.WriteHeader(http.StatusOK)

	// Flush headers
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	h.logger.Printf("Source connected: %s (owner: %s, type: %s, bitrate: %d)",
		mountpointPath, station.OwnerPubkey[:16]+"...", contentType, bitrate)

	// 13. Update station status to online
	if err := h.db.UpdateStationStatus(mountpointPath, "online"); err != nil {
		h.logger.Printf("Failed to update station status: %v", err)
	}

	h.logger.Printf("Metadata auto-extraction enabled for %s", mountpointPath)

	// 14. Keep connection open until source disconnects or context is cancelled
	<-r.Context().Done()

	// 15. Cleanup on disconnect
	mountpoint.RemoveSource()

	// 16. Update station status to offline
	if err := h.db.UpdateStationStatus(mountpointPath, "offline"); err != nil {
		h.logger.Printf("Failed to update station status: %v", err)
	}

	h.logger.Printf("Source disconnected: %s", mountpointPath)
}

// authenticateSource validates source credentials against the database
func (h *SourceHandler) authenticateSource(username, password, mountpoint string) (*database.Station, error) {
	// Get station by mountpoint
	station, err := h.db.GetStationByMountpoint(mountpoint)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	if station == nil {
		return nil, fmt.Errorf("station not found")
	}

	// For source authentication, we expect:
	// Username: "source" (standard Icecast convention)
	// Password: station-specific password (derived from owner's pubkey or configured separately)
	//
	// In this implementation, we use a simple scheme:
	// Password = base64(SHA256(owner_pubkey + mountpoint))
	// This allows the station owner to derive the password from their pubkey

	// Validate username (should be "source")
	if username != "source" {
		return nil, fmt.Errorf("invalid username (expected 'source')")
	}

	// Generate expected password from owner's pubkey
	expectedPassword := h.generateSourcePassword(station.OwnerPubkey, mountpoint)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) != 1 {
		return nil, fmt.Errorf("invalid password")
	}

	return station, nil
}

// generateSourcePassword generates a cryptographically secure source password
// from owner pubkey and mountpoint using HMAC-SHA256
func (h *SourceHandler) generateSourcePassword(ownerPubkey, mountpoint string) string {
	// Use HMAC-SHA256 with a server-specific secret key from config
	// This secret should be unique per instance and rotated periodically
	secret := []byte(h.config.ServerSecret)

	// Create HMAC hash
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ownerPubkey + ":" + mountpoint))
	hash := mac.Sum(nil)

	// Return first 24 characters of base64-encoded hash (strong enough for source auth)
	return base64.URLEncoding.EncodeToString(hash)[:24]
}

// HandleMetadataUpdate handles metadata updates from source clients
func (h *SourceHandler) HandleMetadataUpdate(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests for metadata updates
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get mountpoint from query parameter
	mountpoint := r.URL.Query().Get("mount")
	if mountpoint == "" {
		mountpoint = r.URL.Path
	}

	// Sanitize mountpoint
	mountpoint = h.sanitizer.SanitizeMountpointPath(mountpoint)
	if mountpoint == "" {
		http.Error(w, "Invalid mountpoint", http.StatusBadRequest)
		return
	}

	// Get metadata from query parameters (Icecast format)
	song := r.URL.Query().Get("song")
	if song == "" {
		// Try alternative parameter names
		song = r.URL.Query().Get("title")
	}
	if song == "" {
		http.Error(w, "Missing song parameter", http.StatusBadRequest)
		return
	}

	// SECURITY: Truncate BEFORE sanitization to prevent buffer overflow
	// Sanitizer may expand the string (HTML entity encoding), so we limit input size first
	const maxMetadataLength = 200
	if len(song) > maxMetadataLength {
		song = song[:maxMetadataLength]
	}

	// Sanitize metadata after truncation
	song = h.sanitizer.SanitizeString(song)

	// SECURITY: Ensure valid UTF-8 after sanitization
	// This prevents corrupted multibyte sequences from truncation
	if !utf8.ValidString(song) {
		h.logger.Printf("Warning: Invalid UTF-8 in metadata after sanitization, cleaning")
		song = strings.ToValidUTF8(song, "")
	}

	// Get mountpoint
	mp, exists := h.mpManager.Get(mountpoint)
	if !exists {
		http.Error(w, "Mountpoint not found", http.StatusNotFound)
		return
	}

	// Update metadata
	mp.UpdateMetadata(song)

	// Update database
	if err := h.db.UpdateStationMetadata(mountpoint, song); err != nil {
		h.logger.Printf("Failed to update metadata in database: %v", err)
	}

	h.logger.Printf("Metadata updated for %s: %s", mountpoint, song)

	// Send success response
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}

// GetSourceStats returns statistics about source connections
func (h *SourceHandler) GetSourceStats() map[string]interface{} {
	activeSources := 0
	sourceInfo := make([]map[string]interface{}, 0)

	for _, mp := range h.mpManager.List() {
		if mp.IsOnline() {
			activeSources++
			sourceInfo = append(sourceInfo, map[string]interface{}{
				"mountpoint":   mp.GetPath(),
				"station_id":   mp.GetStationID(),
				"content_type": mp.ContentType(),
				"bitrate":      mp.Bitrate(),
				"listeners":    mp.GetListenerCount(),
			})
		}
	}

	return map[string]interface{}{
		"active_sources": activeSources,
		"sources":        sourceInfo,
	}
}

// DisconnectSource forcefully disconnects a source from a mountpoint
func (h *SourceHandler) DisconnectSource(mountpoint string) error {
	mp, exists := h.mpManager.Get(mountpoint)
	if !exists {
		return fmt.Errorf("mountpoint not found: %s", mountpoint)
	}

	if !mp.IsOnline() {
		return fmt.Errorf("no active source on mountpoint: %s", mountpoint)
	}

	mp.RemoveSource()

	// Update database
	if err := h.db.UpdateStationStatus(mountpoint, "offline"); err != nil {
		h.logger.Printf("Failed to update station status: %v", err)
	}

	h.logger.Printf("Forcefully disconnected source from: %s", mountpoint)
	return nil
}

// ValidateSourceRequest performs additional validation on source requests
func (h *SourceHandler) ValidateSourceRequest(r *http.Request) error {
	// Check for required headers
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return fmt.Errorf("missing Content-Type header")
	}

	// Check for suspicious Content-Length
	if r.ContentLength > 0 && r.ContentLength < 1000000 {
		// Source streams should not have a Content-Length (they're continuous)
		// or it should be very large
		h.logger.Printf("Warning: Source request has small Content-Length: %d", r.ContentLength)
	}

	// Check for ICY headers (compatibility check)
	if r.Header.Get("ice-name") != "" || r.Header.Get("icy-name") != "" {
		h.logger.Printf("ICY headers detected from %s", r.RemoteAddr)
	}

	return nil
}

// SourceConnectionTimeout sets a maximum duration for source connections
func (h *SourceHandler) SourceConnectionTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if timeout <= 0 {
		// No timeout
		return ctx
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)

	// Cancel will be called automatically when timeout expires
	go func() {
		<-timeoutCtx.Done()
		if timeoutCtx.Err() == context.DeadlineExceeded {
			h.logger.Printf("Source connection timed out after %v", timeout)
		}
		cancel()
	}()

	return timeoutCtx
}

// MonitorSourceHealth monitors source connection health
func (h *SourceHandler) MonitorSourceHealth(mountpoint string, checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		mp, exists := h.mpManager.Get(mountpoint)
		if !exists {
			return // Mountpoint removed
		}

		if !mp.IsOnline() {
			return // Source disconnected
		}

		// Check if any data is flowing
		listenerCount := mp.GetListenerCount()
		h.logger.Printf("Health check - %s: online, %d listeners", mountpoint, listenerCount)

		// Could add more sophisticated health checks here:
		// - Check if data was received in last N seconds
		// - Monitor bitrate consistency
		// - Check for errors
	}
}

// ParseIcyHeaders parses ICY-specific headers from source request
func (h *SourceHandler) ParseIcyHeaders(r *http.Request) map[string]string {
	icyHeaders := make(map[string]string)

	// Common ICY headers
	headerNames := []string{
		"ice-name", "icy-name",
		"ice-description", "icy-description",
		"ice-genre", "icy-genre",
		"ice-url", "icy-url",
		"ice-bitrate", "icy-br",
		"ice-public", "icy-pub",
	}

	for _, name := range headerNames {
		if value := r.Header.Get(name); value != "" {
			// Sanitize value
			value = h.sanitizer.SanitizeString(value)
			icyHeaders[strings.TrimPrefix(name, "ice-")] = value
		}
	}

	return icyHeaders
}
