package security

import (
	"fmt"
	"sync"
	"time"
)

// IPBlocker manages IP address blocking
type IPBlocker struct {
	blocked        sync.Map // map[string]*BlockEntry
	failureTracker sync.Map // map[string]*FailureTracker
	autoBlockThreshold int
	blockDuration      time.Duration
	stopChan           chan struct{}
	auditLogger        *AuditLogger
}

// BlockEntry represents a blocked IP
type BlockEntry struct {
	IP        string
	Reason    string
	BlockedAt time.Time
	ExpiresAt time.Time
	Permanent bool
}

// FailureTracker tracks failed attempts from an IP
type FailureTracker struct {
	IP            string
	Failures      int
	FirstFailure  time.Time
	LastFailure   time.Time
	mu            sync.Mutex
}

// IPBlockerConfig configures the IP blocker
type IPBlockerConfig struct {
	AutoBlockThreshold int           // Number of failures before auto-block
	BlockDuration      time.Duration // Duration of automatic blocks
	FailureWindow      time.Duration // Time window for counting failures
	AuditLogger        *AuditLogger  // Optional audit logger
}

// NewIPBlocker creates a new IP blocker
func NewIPBlocker(cfg *IPBlockerConfig) *IPBlocker {
	if cfg.AutoBlockThreshold == 0 {
		cfg.AutoBlockThreshold = 10
	}
	if cfg.BlockDuration == 0 {
		cfg.BlockDuration = 1 * time.Hour
	}
	if cfg.FailureWindow == 0 {
		cfg.FailureWindow = 15 * time.Minute
	}

	blocker := &IPBlocker{
		autoBlockThreshold: cfg.AutoBlockThreshold,
		blockDuration:      cfg.BlockDuration,
		stopChan:           make(chan struct{}),
		auditLogger:        cfg.AuditLogger,
	}

	// Start cleanup goroutine
	go blocker.cleanup()

	return blocker
}

// IsBlocked checks if an IP is blocked
func (ib *IPBlocker) IsBlocked(ip string) (bool, string) {
	if val, exists := ib.blocked.Load(ip); exists {
		entry := val.(*BlockEntry)

		// Check if temporary block has expired
		if !entry.Permanent && time.Now().After(entry.ExpiresAt) {
			ib.Unblock(ip)
			return false, ""
		}

		return true, entry.Reason
	}

	return false, ""
}

// Block blocks an IP address
func (ib *IPBlocker) Block(ip, reason string, duration time.Duration) {
	var expiresAt time.Time
	permanent := false

	if duration == 0 {
		// Permanent block
		permanent = true
		expiresAt = time.Now().Add(100 * 365 * 24 * time.Hour) // 100 years
	} else {
		expiresAt = time.Now().Add(duration)
	}

	entry := &BlockEntry{
		IP:        ip,
		Reason:    reason,
		BlockedAt: time.Now(),
		ExpiresAt: expiresAt,
		Permanent: permanent,
	}

	ib.blocked.Store(ip, entry)

	// Log the block
	if ib.auditLogger != nil {
		durationStr := "permanent"
		if !permanent {
			durationStr = duration.String()
		}
		ib.auditLogger.Log(
			EventIPBlocked,
			SeverityHigh,
			"",
			"",
			fmt.Sprintf("IP blocked: %s (duration: %s)", reason, durationStr),
		)
	}
}

// Unblock removes a block on an IP
func (ib *IPBlocker) Unblock(ip string) {
	ib.blocked.Delete(ip)
	ib.failureTracker.Delete(ip)
}

// RecordFailure records a failed attempt from an IP
func (ib *IPBlocker) RecordFailure(ip, reason string) bool {
	// Get or create failure tracker
	var tracker *FailureTracker
	if val, exists := ib.failureTracker.Load(ip); exists {
		tracker = val.(*FailureTracker)
	} else {
		tracker = &FailureTracker{
			IP:           ip,
			Failures:     0,
			FirstFailure: time.Now(),
		}
		ib.failureTracker.Store(ip, tracker)
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	// Reset if failure window expired
	if time.Since(tracker.FirstFailure) > 15*time.Minute {
		tracker.Failures = 0
		tracker.FirstFailure = time.Now()
	}

	tracker.Failures++
	tracker.LastFailure = time.Now()

	// Check if auto-block threshold reached
	if tracker.Failures >= ib.autoBlockThreshold {
		ib.Block(ip, fmt.Sprintf("Auto-blocked: %s (threshold: %d failures)", reason, ib.autoBlockThreshold), ib.blockDuration)

		// Log brute force detection
		if ib.auditLogger != nil {
			ib.auditLogger.Log(
				EventBruteForce,
				SeverityCritical,
				"",
				"",
				fmt.Sprintf("Brute force detected: %d failures in %v", tracker.Failures, time.Since(tracker.FirstFailure)),
			)
		}

		return true
	}

	return false
}

// GetFailureCount returns the number of failures for an IP
func (ib *IPBlocker) GetFailureCount(ip string) int {
	if val, exists := ib.failureTracker.Load(ip); exists {
		tracker := val.(*FailureTracker)
		tracker.mu.Lock()
		defer tracker.mu.Unlock()
		return tracker.Failures
	}
	return 0
}

// ResetFailures resets the failure count for an IP
func (ib *IPBlocker) ResetFailures(ip string) {
	ib.failureTracker.Delete(ip)
}

// cleanup periodically removes expired blocks and old failure trackers
func (ib *IPBlocker) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ib.cleanupExpired()
		case <-ib.stopChan:
			return
		}
	}
}

// cleanupExpired removes expired blocks and old failure trackers
func (ib *IPBlocker) cleanupExpired() {
	now := time.Now()

	// Remove expired blocks
	ib.blocked.Range(func(key, value interface{}) bool {
		entry := value.(*BlockEntry)
		if !entry.Permanent && now.After(entry.ExpiresAt) {
			ib.blocked.Delete(key)
		}
		return true
	})

	// Remove old failure trackers (>1 hour old)
	ib.failureTracker.Range(func(key, value interface{}) bool {
		tracker := value.(*FailureTracker)
		tracker.mu.Lock()
		if now.Sub(tracker.LastFailure) > 1*time.Hour {
			ib.failureTracker.Delete(key)
		}
		tracker.mu.Unlock()
		return true
	})
}

// Close stops the cleanup goroutine
func (ib *IPBlocker) Close() {
	close(ib.stopChan)
}

// GetStats returns statistics about the IP blocker
func (ib *IPBlocker) GetStats() map[string]interface{} {
	blockedCount := 0
	permanentCount := 0
	trackedCount := 0

	ib.blocked.Range(func(key, value interface{}) bool {
		blockedCount++
		entry := value.(*BlockEntry)
		if entry.Permanent {
			permanentCount++
		}
		return true
	})

	ib.failureTracker.Range(func(key, value interface{}) bool {
		trackedCount++
		return true
	})

	return map[string]interface{}{
		"blocked_ips":       blockedCount,
		"permanent_blocks":  permanentCount,
		"tracked_ips":       trackedCount,
		"auto_block_threshold": ib.autoBlockThreshold,
		"block_duration_seconds": ib.blockDuration.Seconds(),
	}
}

// GetBlockedIPs returns a list of all blocked IPs
func (ib *IPBlocker) GetBlockedIPs() []*BlockEntry {
	var entries []*BlockEntry

	ib.blocked.Range(func(key, value interface{}) bool {
		entry := value.(*BlockEntry)
		entries = append(entries, entry)
		return true
	})

	return entries
}

// GetTrackedIPs returns a list of all IPs being tracked for failures
func (ib *IPBlocker) GetTrackedIPs() []*FailureTracker {
	var trackers []*FailureTracker

	ib.failureTracker.Range(func(key, value interface{}) bool {
		tracker := value.(*FailureTracker)
		trackers = append(trackers, tracker)
		return true
	})

	return trackers
}
