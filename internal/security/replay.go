package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ReplayProtection provides protection against replay attacks
type ReplayProtection struct {
	nonces   sync.Map // map[string]time.Time
	window   time.Duration
	stopChan chan struct{}
}

// NonceEntry represents a nonce entry with expiration
type NonceEntry struct {
	timestamp time.Time
	used      bool
}

// NewReplayProtection creates a new replay protection system
func NewReplayProtection(window time.Duration) *ReplayProtection {
	if window == 0 {
		window = 5 * time.Minute // Default 5 minute window
	}

	rp := &ReplayProtection{
		window:   window,
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	go rp.cleanup()

	return rp
}

// CheckAndMarkNonce checks if a nonce has been used and marks it as used
func (rp *ReplayProtection) CheckAndMarkNonce(pubkey, timestamp, signature string) error {
	// Create unique nonce identifier
	nonceID := rp.createNonceID(pubkey, timestamp, signature)

	// Check if nonce exists
	if val, exists := rp.nonces.Load(nonceID); exists {
		entry := val.(*NonceEntry)
		if entry.used {
			return fmt.Errorf("replay attack detected: nonce already used")
		}
	}

	// Mark as used
	entry := &NonceEntry{
		timestamp: time.Now(),
		used:      true,
	}
	rp.nonces.Store(nonceID, entry)

	return nil
}

// IsNonceUsed checks if a nonce has been used without marking it
func (rp *ReplayProtection) IsNonceUsed(pubkey, timestamp, signature string) bool {
	nonceID := rp.createNonceID(pubkey, timestamp, signature)

	if val, exists := rp.nonces.Load(nonceID); exists {
		entry := val.(*NonceEntry)
		return entry.used
	}

	return false
}

// createNonceID creates a unique identifier for a nonce
func (rp *ReplayProtection) createNonceID(pubkey, timestamp, signature string) string {
	// Combine pubkey, timestamp, and signature to create unique ID
	data := fmt.Sprintf("%s:%s:%s", pubkey, timestamp, signature)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// cleanup periodically removes expired nonces
func (rp *ReplayProtection) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rp.cleanupExpired()
		case <-rp.stopChan:
			return
		}
	}
}

// cleanupExpired removes nonces older than the window
func (rp *ReplayProtection) cleanupExpired() {
	now := time.Now()
	cutoff := now.Add(-rp.window)

	rp.nonces.Range(func(key, value interface{}) bool {
		entry := value.(*NonceEntry)
		if entry.timestamp.Before(cutoff) {
			rp.nonces.Delete(key)
		}
		return true
	})
}

// Close stops the cleanup goroutine
func (rp *ReplayProtection) Close() {
	close(rp.stopChan)
}

// GetStats returns statistics about the replay protection
func (rp *ReplayProtection) GetStats() map[string]interface{} {
	count := 0
	rp.nonces.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	return map[string]interface{}{
		"active_nonces": count,
		"window_seconds": rp.window.Seconds(),
	}
}

// Clear removes all nonces (for testing purposes)
func (rp *ReplayProtection) Clear() {
	rp.nonces.Range(func(key, value interface{}) bool {
		rp.nonces.Delete(key)
		return true
	})
}

// TimestampValidator validates request timestamps
type TimestampValidator struct {
	window time.Duration
}

// NewTimestampValidator creates a new timestamp validator
func NewTimestampValidator(window time.Duration) *TimestampValidator {
	if window == 0 {
		window = 5 * time.Minute
	}

	return &TimestampValidator{
		window: window,
	}
}

// ValidateTimestamp validates that a timestamp is within the acceptable window
func (tv *TimestampValidator) ValidateTimestamp(timestamp int64) error {
	now := time.Now().Unix()
	diff := now - timestamp

	// Check if timestamp is within window (past or future)
	if diff > int64(tv.window.Seconds()) {
		return fmt.Errorf("timestamp too old: %d seconds", diff)
	}

	if diff < -int64(tv.window.Seconds()) {
		return fmt.Errorf("timestamp too far in future: %d seconds", -diff)
	}

	return nil
}

// GetTimestampDrift returns the drift between request timestamp and server time
func (tv *TimestampValidator) GetTimestampDrift(timestamp int64) time.Duration {
	now := time.Now().Unix()
	diff := now - timestamp
	return time.Duration(diff) * time.Second
}
