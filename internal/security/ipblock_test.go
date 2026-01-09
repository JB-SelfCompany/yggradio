package security

import (
	"testing"
	"time"
)

func TestIPBlocker_BlockAndIsBlocked(t *testing.T) {
	cfg := &IPBlockerConfig{
		AutoBlockThreshold: 10,
		BlockDuration:      1 * time.Hour,
	}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"
	reason := "test block"

	// Initially not blocked
	if blocked, _ := blocker.IsBlocked(ip); blocked {
		t.Error("IP should not be blocked initially")
	}

	// Block the IP
	blocker.Block(ip, reason, 1*time.Hour)

	// Should be blocked now
	if blocked, blockReason := blocker.IsBlocked(ip); !blocked {
		t.Error("IP should be blocked")
	} else if blockReason != reason {
		t.Errorf("Block reason = %s, want %s", blockReason, reason)
	}
}

func TestIPBlocker_Unblock(t *testing.T) {
	cfg := &IPBlockerConfig{}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"

	// Block then unblock
	blocker.Block(ip, "test", 1*time.Hour)
	blocker.Unblock(ip)

	// Should not be blocked
	if blocked, _ := blocker.IsBlocked(ip); blocked {
		t.Error("IP should not be blocked after unblock")
	}
}

func TestIPBlocker_PermanentBlock(t *testing.T) {
	cfg := &IPBlockerConfig{}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"

	// Permanent block (duration = 0)
	blocker.Block(ip, "permanent ban", 0)

	blocked, _ := blocker.IsBlocked(ip)
	if !blocked {
		t.Error("IP should be permanently blocked")
	}

	// Check entry
	entries := blocker.GetBlockedIPs()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 blocked IP, got %d", len(entries))
	}

	if !entries[0].Permanent {
		t.Error("Block should be marked as permanent")
	}
}

func TestIPBlocker_TemporaryBlockExpiration(t *testing.T) {
	cfg := &IPBlockerConfig{}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"

	// Block for very short duration
	blocker.Block(ip, "test", 100*time.Millisecond)

	// Should be blocked
	if blocked, _ := blocker.IsBlocked(ip); !blocked {
		t.Error("IP should be blocked initially")
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Should be unblocked automatically
	if blocked, _ := blocker.IsBlocked(ip); blocked {
		t.Error("IP should be unblocked after expiration")
	}
}

func TestIPBlocker_RecordFailure(t *testing.T) {
	cfg := &IPBlockerConfig{
		AutoBlockThreshold: 3,
		BlockDuration:      1 * time.Hour,
	}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"

	// Record failures
	blocker.RecordFailure(ip, "auth failure")
	blocker.RecordFailure(ip, "auth failure")

	// Should not be blocked yet
	if blocked, _ := blocker.IsBlocked(ip); blocked {
		t.Error("IP should not be blocked before threshold")
	}

	// Third failure should trigger auto-block
	autoBlocked := blocker.RecordFailure(ip, "auth failure")
	if !autoBlocked {
		t.Error("IP should be auto-blocked on third failure")
	}

	// Should be blocked now
	if blocked, _ := blocker.IsBlocked(ip); !blocked {
		t.Error("IP should be blocked after reaching threshold")
	}
}

func TestIPBlocker_GetFailureCount(t *testing.T) {
	cfg := &IPBlockerConfig{
		AutoBlockThreshold: 10,
	}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"

	// Record some failures
	for i := 0; i < 5; i++ {
		blocker.RecordFailure(ip, "test")
	}

	count := blocker.GetFailureCount(ip)
	if count != 5 {
		t.Errorf("Failure count = %d, want 5", count)
	}
}

func TestIPBlocker_ResetFailures(t *testing.T) {
	cfg := &IPBlockerConfig{}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"

	// Record failures
	blocker.RecordFailure(ip, "test")
	blocker.RecordFailure(ip, "test")

	// Reset
	blocker.ResetFailures(ip)

	// Count should be 0
	if count := blocker.GetFailureCount(ip); count != 0 {
		t.Errorf("Failure count after reset = %d, want 0", count)
	}
}

func TestIPBlocker_FailureWindowReset(t *testing.T) {
	cfg := &IPBlockerConfig{
		FailureWindow:      100 * time.Millisecond,
		AutoBlockThreshold: 100, // High threshold to avoid auto-blocking
	}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	ip := "2001:db8::1"

	// Record failures
	blocker.RecordFailure(ip, "test")
	blocker.RecordFailure(ip, "test")

	// Should have 2 failures
	if count := blocker.GetFailureCount(ip); count != 2 {
		t.Errorf("Initial failure count = %d, want 2", count)
	}

	// Wait for window to expire (note: the failure window is 15 minutes by default in RecordFailure,
	// not the FailureWindow config, so we need to manually manipulate the tracker)
	tracker, _ := blocker.failureTracker.Load(ip)
	ft := tracker.(*FailureTracker)
	ft.mu.Lock()
	ft.FirstFailure = time.Now().Add(-20 * time.Minute) // Force expiration
	ft.mu.Unlock()

	// Record another failure - should reset counter
	blocker.RecordFailure(ip, "test")

	// Should only have 1 failure (counter was reset)
	if count := blocker.GetFailureCount(ip); count != 1 {
		t.Errorf("Failure count after window reset = %d, want 1", count)
	}
}

func TestIPBlocker_GetStats(t *testing.T) {
	cfg := &IPBlockerConfig{
		AutoBlockThreshold: 5,
		BlockDuration:      1 * time.Hour,
	}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	// Block some IPs
	blocker.Block("2001:db8::1", "test1", 1*time.Hour)
	blocker.Block("2001:db8::2", "test2", 0) // permanent

	// Track some IPs
	blocker.RecordFailure("2001:db8::3", "test")

	stats := blocker.GetStats()

	if blockedIPs := stats["blocked_ips"].(int); blockedIPs != 2 {
		t.Errorf("blocked_ips = %d, want 2", blockedIPs)
	}

	if permanentBlocks := stats["permanent_blocks"].(int); permanentBlocks != 1 {
		t.Errorf("permanent_blocks = %d, want 1", permanentBlocks)
	}

	if trackedIPs := stats["tracked_ips"].(int); trackedIPs != 1 {
		t.Errorf("tracked_ips = %d, want 1", trackedIPs)
	}
}

func TestIPBlocker_GetBlockedIPs(t *testing.T) {
	cfg := &IPBlockerConfig{}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	// Block some IPs
	blocker.Block("2001:db8::1", "reason1", 1*time.Hour)
	blocker.Block("2001:db8::2", "reason2", 1*time.Hour)

	entries := blocker.GetBlockedIPs()

	if len(entries) != 2 {
		t.Errorf("GetBlockedIPs() length = %d, want 2", len(entries))
	}
}

func TestIPBlocker_Cleanup(t *testing.T) {
	cfg := &IPBlockerConfig{}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	// Block with short duration
	blocker.Block("2001:db8::1", "test", 100*time.Millisecond)

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Force cleanup
	blocker.cleanupExpired()

	stats := blocker.GetStats()
	if blockedIPs := stats["blocked_ips"].(int); blockedIPs != 0 {
		t.Errorf("After cleanup blocked_ips = %d, want 0", blockedIPs)
	}
}

func BenchmarkIPBlocker_RecordFailure(b *testing.B) {
	cfg := &IPBlockerConfig{
		AutoBlockThreshold: 1000, // High threshold to avoid auto-blocking during benchmark
	}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blocker.RecordFailure("2001:db8::1", "test")
	}
}

func BenchmarkIPBlocker_IsBlocked(b *testing.B) {
	cfg := &IPBlockerConfig{}
	blocker := NewIPBlocker(cfg)
	defer blocker.Close()

	// Pre-populate with some blocks
	for i := 0; i < 1000; i++ {
		blocker.Block("2001:db8::"+string(rune(i)), "test", 1*time.Hour)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blocker.IsBlocked("2001:db8::500")
	}
}
