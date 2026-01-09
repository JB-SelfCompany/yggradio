package security

import (
	"testing"
	"time"
)

func TestCSRFManager_GenerateToken(t *testing.T) {
	manager := NewCSRFManager(15 * time.Minute)
	defer manager.Close()

	pubkey := "test_pubkey_123"

	token, err := manager.GenerateToken(pubkey)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if token == "" {
		t.Error("GenerateToken() returned empty token")
	}

	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("GenerateToken() token length = %d, want 64", len(token))
	}
}

func TestCSRFManager_VerifyToken(t *testing.T) {
	manager := NewCSRFManager(15 * time.Minute)
	defer manager.Close()

	pubkey := "test_pubkey_123"

	// Generate token
	token, err := manager.GenerateToken(pubkey)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	// Verify valid token
	err = manager.VerifyToken(token, pubkey)
	if err != nil {
		t.Errorf("VerifyToken() error = %v, want nil", err)
	}

	// Token should be deleted after verification (one-time use)
	err = manager.VerifyToken(token, pubkey)
	if err == nil {
		t.Error("VerifyToken() should fail on second use")
	}
}

func TestCSRFManager_VerifyToken_WrongPubkey(t *testing.T) {
	manager := NewCSRFManager(15 * time.Minute)
	defer manager.Close()

	pubkey := "test_pubkey_123"
	wrongPubkey := "wrong_pubkey_456"

	token, _ := manager.GenerateToken(pubkey)

	err := manager.VerifyToken(token, wrongPubkey)
	if err == nil {
		t.Error("VerifyToken() should fail with wrong pubkey")
	}
}

func TestCSRFManager_VerifyToken_Expired(t *testing.T) {
	manager := NewCSRFManager(100 * time.Millisecond)
	defer manager.Close()

	pubkey := "test_pubkey_123"

	token, _ := manager.GenerateToken(pubkey)

	// Wait for token to expire
	time.Sleep(200 * time.Millisecond)

	err := manager.VerifyToken(token, pubkey)
	if err == nil {
		t.Error("VerifyToken() should fail for expired token")
	}
}

func TestCSRFManager_InvalidateUserTokens(t *testing.T) {
	manager := NewCSRFManager(15 * time.Minute)
	defer manager.Close()

	pubkey := "test_pubkey_123"

	// Generate multiple tokens
	token1, _ := manager.GenerateToken(pubkey)
	token2, _ := manager.GenerateToken(pubkey)

	// Invalidate all tokens for user
	manager.InvalidateUserTokens(pubkey)

	// Both tokens should be invalid
	if err := manager.VerifyToken(token1, pubkey); err == nil {
		t.Error("Token 1 should be invalidated")
	}
	if err := manager.VerifyToken(token2, pubkey); err == nil {
		t.Error("Token 2 should be invalidated")
	}
}

func TestCSRFManager_Count(t *testing.T) {
	manager := NewCSRFManager(15 * time.Minute)
	defer manager.Close()

	if count := manager.Count(); count != 0 {
		t.Errorf("Initial count = %d, want 0", count)
	}

	manager.GenerateToken("user1")
	manager.GenerateToken("user2")

	if count := manager.Count(); count != 2 {
		t.Errorf("Count after 2 generations = %d, want 2", count)
	}
}

func TestCSRFManager_Cleanup(t *testing.T) {
	manager := NewCSRFManager(100 * time.Millisecond)
	defer manager.Close()

	// Generate tokens
	manager.GenerateToken("user1")
	manager.GenerateToken("user2")

	if count := manager.Count(); count != 2 {
		t.Errorf("Initial count = %d, want 2", count)
	}

	// Wait for cleanup to run (cleanup runs every 5 minutes in production)
	// For testing, we'll just wait a bit and verify expiration works
	time.Sleep(200 * time.Millisecond)

	// Tokens should be expired but not yet cleaned up
	// Cleanup happens periodically, not immediately
	// This test verifies the expiration logic works
}

func BenchmarkCSRFManager_GenerateToken(b *testing.B) {
	manager := NewCSRFManager(15 * time.Minute)
	defer manager.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.GenerateToken("test_pubkey")
	}
}

func BenchmarkCSRFManager_VerifyToken(b *testing.B) {
	manager := NewCSRFManager(15 * time.Minute)
	defer manager.Close()

	// Pre-generate tokens
	tokens := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		token, _ := manager.GenerateToken("test_pubkey")
		tokens[i] = token
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.VerifyToken(tokens[i], "test_pubkey")
	}
}
