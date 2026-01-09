package security

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	Timestamp  time.Time         `json:"timestamp"`
	EventType  string            `json:"event_type"`
	Severity   string            `json:"severity"`
	Pubkey     string            `json:"pubkey,omitempty"`
	Endpoint   string            `json:"endpoint,omitempty"`
	Details    string            `json:"details,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Severity levels
const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// Event types
const (
	EventAuthFailure      = "auth_failure"
	EventAuthSuccess      = "auth_success"
	EventRateLimit        = "rate_limit"
	EventBlockedAccess    = "blocked_access"
	EventInvalidInput     = "invalid_input"
	EventPoWFailure       = "pow_failure"
	EventCSRFFailure      = "csrf_failure"
	EventReplayAttack     = "replay_attack"
	EventSuspiciousActivity = "suspicious_activity"
	EventIPBlocked        = "ip_blocked"
	EventBruteForce       = "brute_force"
)

// AuditLogger provides security event logging
type AuditLogger struct {
	writer     io.Writer
	file       *os.File
	mu         sync.Mutex
	buffer     chan *SecurityEvent
	stopChan   chan struct{}
	enableJSON bool
}

// AuditConfig configures the audit logger
type AuditConfig struct {
	LogPath    string
	EnableJSON bool
	BufferSize int
}

// NewAuditLogger creates a new security audit logger
func NewAuditLogger(cfg *AuditConfig) (*AuditLogger, error) {
	var writer io.Writer
	var file *os.File
	var err error

	if cfg.LogPath != "" {
		// Open log file with append mode
		file, err = os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to open audit log file: %w", err)
		}
		writer = file
	} else {
		// Use stdout if no log path specified
		writer = os.Stdout
	}

	bufferSize := cfg.BufferSize
	if bufferSize == 0 {
		bufferSize = 1000
	}

	logger := &AuditLogger{
		writer:     writer,
		file:       file,
		buffer:     make(chan *SecurityEvent, bufferSize),
		stopChan:   make(chan struct{}),
		enableJSON: cfg.EnableJSON,
	}

	// Start background writer
	go logger.processEvents()

	return logger, nil
}

// LogEvent logs a security event
func (a *AuditLogger) LogEvent(event *SecurityEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Non-blocking send to buffer
	select {
	case a.buffer <- event:
	default:
		// Buffer full - log to stderr
		fmt.Fprintf(os.Stderr, "WARNING: Audit log buffer full, dropping event: %s\n", event.EventType)
	}
}

// Log is a convenience method for logging events
func (a *AuditLogger) Log(eventType, severity, pubkey, endpoint, details string) {
	event := &SecurityEvent{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Pubkey:    pubkey,
		Endpoint:  endpoint,
		Details:   details,
	}
	a.LogEvent(event)
}

// LogWithMetadata logs an event with additional metadata
func (a *AuditLogger) LogWithMetadata(eventType, severity, pubkey, endpoint, details string, metadata map[string]string) {
	event := &SecurityEvent{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Pubkey:    pubkey,
		Endpoint:  endpoint,
		Details:   details,
		Metadata:  metadata,
	}
	a.LogEvent(event)
}

// processEvents processes events from the buffer
func (a *AuditLogger) processEvents() {
	for {
		select {
		case event := <-a.buffer:
			a.writeEvent(event)
		case <-a.stopChan:
			// Drain remaining events
			for len(a.buffer) > 0 {
				event := <-a.buffer
				a.writeEvent(event)
			}
			return
		}
	}
}

// writeEvent writes a single event to the log
func (a *AuditLogger) writeEvent(event *SecurityEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var output string

	if a.enableJSON {
		// JSON format
		data, err := json.Marshal(event)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to marshal audit event: %v\n", err)
			return
		}
		output = string(data) + "\n"
	} else {
		// Human-readable format
		output = fmt.Sprintf("[%s] [%s] %s - %s",
			event.Timestamp.Format(time.RFC3339),
			event.Severity,
			event.EventType,
			event.Details,
		)

		if event.Pubkey != "" {
			output += fmt.Sprintf(" | Pubkey: %s", event.Pubkey[:16]+"...")
		}
		if event.Endpoint != "" {
			output += fmt.Sprintf(" | Endpoint: %s", event.Endpoint)
		}
		if len(event.Metadata) > 0 {
			metaJSON, _ := json.Marshal(event.Metadata)
			output += fmt.Sprintf(" | Metadata: %s", metaJSON)
		}
		output += "\n"
	}

	if _, err := a.writer.Write([]byte(output)); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to write audit event: %v\n", err)
	}
}

// Close gracefully shuts down the logger
func (a *AuditLogger) Close() error {
	close(a.stopChan)

	// Wait a bit for buffer to drain
	time.Sleep(100 * time.Millisecond)

	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// GetStats returns statistics about the audit logger
func (a *AuditLogger) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"buffer_size":     cap(a.buffer),
		"pending_events":  len(a.buffer),
		"buffer_usage_pct": float64(len(a.buffer)) / float64(cap(a.buffer)) * 100,
	}
}

// SecurityEventFilter helps filter security events by severity
type SecurityEventFilter struct {
	minSeverity string
}

// NewSecurityEventFilter creates a new event filter
func NewSecurityEventFilter(minSeverity string) *SecurityEventFilter {
	return &SecurityEventFilter{
		minSeverity: minSeverity,
	}
}

// ShouldLog determines if an event should be logged based on severity
func (f *SecurityEventFilter) ShouldLog(severity string) bool {
	severityLevels := map[string]int{
		SeverityLow:      1,
		SeverityMedium:   2,
		SeverityHigh:     3,
		SeverityCritical: 4,
	}

	minLevel := severityLevels[f.minSeverity]
	eventLevel := severityLevels[severity]

	return eventLevel >= minLevel
}
