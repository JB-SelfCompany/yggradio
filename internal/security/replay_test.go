package security

import (
	"fmt"
	"testing"
	"time"
)

func TestReplayProtection_CheckAndMarkNonce(t *testing.T) {
	rp := NewReplayProtection(5 * time.Minute)
	defer rp.Close()

	pubkey := "test_pubkey"
	timestamp := "1234567890"
	signature := "test_signature"

	// First use should succeed
	err := rp.CheckAndMarkNonce(pubkey, timestamp, signature)
	if err != nil {
		t.Errorf("First CheckAndMarkNonce() error = %v, want nil", err)
	}

	// Second use should fail (replay attack)
	err = rp.CheckAndMarkNonce(pubkey, timestamp, signature)
	if err == nil {
		t.Error("Second CheckAndMarkNonce() should fail (replay attack)")
	}
}

func TestReplayProtection_IsNonceUsed(t *testing.T) {
	rp := NewReplayProtection(5 * time.Minute)
	defer rp.Close()

	pubkey := "test_pubkey"
	timestamp := "1234567890"
	signature := "test_signature"

	// Initially not used
	if rp.IsNonceUsed(pubkey, timestamp, signature) {
		t.Error("Nonce should not be used initially")
	}

	// Mark as used
	rp.CheckAndMarkNonce(pubkey, timestamp, signature)

	// Should be used now
	if !rp.IsNonceUsed(pubkey, timestamp, signature) {
		t.Error("Nonce should be used after marking")
	}
}

func TestReplayProtection_DifferentNonces(t *testing.T) {
	rp := NewReplayProtection(5 * time.Minute)
	defer rp.Close()

	pubkey := "test_pubkey"

	// Different timestamps should work
	err1 := rp.CheckAndMarkNonce(pubkey, "1234567890", "sig1")
	err2 := rp.CheckAndMarkNonce(pubkey, "1234567891", "sig2")

	if err1 != nil || err2 != nil {
		t.Error("Different nonces should both succeed")
	}

	// Same timestamp but different signature should work
	err3 := rp.CheckAndMarkNonce(pubkey, "1234567890", "sig3")
	if err3 != nil {
		t.Error("Same timestamp with different signature should succeed")
	}
}

func TestReplayProtection_Cleanup(t *testing.T) {
	// Use short window for testing
	rp := NewReplayProtection(100 * time.Millisecond)
	defer rp.Close()

	pubkey := "test_pubkey"
	timestamp := "1234567890"
	signature := "test_signature"

	// Mark nonce
	rp.CheckAndMarkNonce(pubkey, timestamp, signature)

	// Should be used
	if !rp.IsNonceUsed(pubkey, timestamp, signature) {
		t.Error("Nonce should be used")
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Force cleanup
	rp.cleanupExpired()

	stats := rp.GetStats()
	activeNonces := stats["active_nonces"].(int)

	// Should be cleaned up
	if activeNonces > 0 {
		t.Errorf("Active nonces after cleanup = %d, want 0", activeNonces)
	}
}

func TestReplayProtection_GetStats(t *testing.T) {
	rp := NewReplayProtection(5 * time.Minute)
	defer rp.Close()

	// Mark some nonces
	for i := 0; i < 5; i++ {
		rp.CheckAndMarkNonce("pubkey", fmt.Sprintf("%d", i), fmt.Sprintf("sig%d", i))
	}

	stats := rp.GetStats()

	if activeNonces, ok := stats["active_nonces"].(int); !ok || activeNonces != 5 {
		t.Errorf("GetStats() active_nonces = %v, want 5", activeNonces)
	}

	if window, ok := stats["window_seconds"].(float64); !ok || window != 300 {
		t.Errorf("GetStats() window_seconds = %v, want 300", window)
	}
}

func TestReplayProtection_Clear(t *testing.T) {
	rp := NewReplayProtection(5 * time.Minute)
	defer rp.Close()

	// Mark nonces
	rp.CheckAndMarkNonce("pubkey1", "ts1", "sig1")
	rp.CheckAndMarkNonce("pubkey2", "ts2", "sig2")

	// Clear all
	rp.Clear()

	stats := rp.GetStats()
	if activeNonces := stats["active_nonces"].(int); activeNonces != 0 {
		t.Errorf("After Clear() active_nonces = %d, want 0", activeNonces)
	}
}

func TestTimestampValidator_ValidateTimestamp(t *testing.T) {
	tv := NewTimestampValidator(5 * time.Minute)

	now := time.Now().Unix()

	tests := []struct {
		name      string
		timestamp int64
		wantErr   bool
	}{
		{"current time", now, false},
		{"1 minute ago", now - 60, false},
		{"4 minutes ago", now - 240, false},
		{"6 minutes ago", now - 360, true},
		{"1 minute future", now + 60, false},
		{"6 minutes future", now + 360, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tv.ValidateTimestamp(tt.timestamp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimestamp() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTimestampValidator_GetTimestampDrift(t *testing.T) {
	tv := NewTimestampValidator(5 * time.Minute)

	now := time.Now().Unix()
	pastTimestamp := now - 10 // 10 seconds ago

	drift := tv.GetTimestampDrift(pastTimestamp)

	if drift < 9*time.Second || drift > 11*time.Second {
		t.Errorf("GetTimestampDrift() = %v, want ~10s", drift)
	}
}

func BenchmarkReplayProtection_CheckAndMarkNonce(b *testing.B) {
	rp := NewReplayProtection(5 * time.Minute)
	defer rp.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pubkey := fmt.Sprintf("pubkey%d", i)
		timestamp := fmt.Sprintf("%d", i)
		signature := fmt.Sprintf("sig%d", i)
		rp.CheckAndMarkNonce(pubkey, timestamp, signature)
	}
}

func BenchmarkReplayProtection_IsNonceUsed(b *testing.B) {
	rp := NewReplayProtection(5 * time.Minute)
	defer rp.Close()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		rp.CheckAndMarkNonce(fmt.Sprintf("pubkey%d", i), fmt.Sprintf("%d", i), fmt.Sprintf("sig%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % 1000
		rp.IsNonceUsed(fmt.Sprintf("pubkey%d", idx), fmt.Sprintf("%d", idx), fmt.Sprintf("sig%d", idx))
	}
}
