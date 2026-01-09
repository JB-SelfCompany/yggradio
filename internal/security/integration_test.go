package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

// TestSecurityIntegration tests the integration of all security components
func TestSecurityIntegration_FullAuthFlow(t *testing.T) {
	// Setup all security components
	auditLogger, err := NewAuditLogger(&AuditConfig{
		EnableJSON: false,
		BufferSize: 100,
	})
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	replayProtection := NewReplayProtection(5 * time.Minute)
	defer replayProtection.Close()

	ipBlocker := NewIPBlocker(&IPBlockerConfig{
		AutoBlockThreshold: 3,
		BlockDuration:      1 * time.Hour,
		AuditLogger:        auditLogger,
	})
	defer ipBlocker.Close()

	csrfManager := NewCSRFManager(15 * time.Minute)
	defer csrfManager.Close()

	// Generate test key pair
	pubkey, privkey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}
	pubkeyHex := hex.EncodeToString(pubkey)

	// Test 1: Successful authentication
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	message := "GET/test" + timestamp
	signature := ed25519.Sign(privkey, []byte(message))
	signatureHex := hex.EncodeToString(signature)

	// Check replay protection
	err = replayProtection.CheckAndMarkNonce(pubkeyHex, timestamp, signatureHex)
	if err != nil {
		t.Errorf("First auth should succeed: %v", err)
	}

	// Log successful auth
	auditLogger.Log(EventAuthSuccess, SeverityLow, "2001:db8::1", pubkeyHex, "/test", "Auth successful")

	// Test 2: Replay attack detection
	err = replayProtection.CheckAndMarkNonce(pubkeyHex, timestamp, signatureHex)
	if err == nil {
		t.Error("Replay attack should be detected")
	}

	// Log replay attack
	auditLogger.Log(EventReplayAttack, SeverityCritical, "2001:db8::1", pubkeyHex, "/test", "Replay detected")

	// Test 3: IP blocking after failures
	ip := "2001:db8::bad"
	for i := 0; i < 3; i++ {
		blocked := ipBlocker.RecordFailure(ip, "auth failure")
		if i < 2 && blocked {
			t.Error("Should not be blocked before threshold")
		}
		if i == 2 && !blocked {
			t.Error("Should be blocked at threshold")
		}
	}

	// Verify IP is blocked
	if blocked, _ := ipBlocker.IsBlocked(ip); !blocked {
		t.Error("IP should be blocked after failures")
	}

	// Test 4: CSRF token generation and validation
	csrfToken, err := csrfManager.GenerateToken(pubkeyHex)
	if err != nil {
		t.Fatalf("Failed to generate CSRF token: %v", err)
	}

	err = csrfManager.VerifyToken(csrfToken, pubkeyHex)
	if err != nil {
		t.Errorf("CSRF token verification failed: %v", err)
	}

	// Test 5: Verify audit log stats
	stats := auditLogger.GetStats()
	if pendingEvents, ok := stats["pending_events"].(int); !ok || pendingEvents < 0 {
		t.Errorf("Unexpected pending events: %v", pendingEvents)
	}

	// Test 6: Verify IP blocker stats
	ipStats := ipBlocker.GetStats()
	if blockedIPs, ok := ipStats["blocked_ips"].(int); !ok || blockedIPs < 1 {
		t.Errorf("Expected at least 1 blocked IP, got %v", blockedIPs)
	}

	// Test 7: Verify replay protection stats
	rpStats := replayProtection.GetStats()
	if activeNonces, ok := rpStats["active_nonces"].(int); !ok || activeNonces < 1 {
		t.Errorf("Expected at least 1 active nonce, got %v", activeNonces)
	}
}

func TestSecurityIntegration_ValidatorAndSanitizer(t *testing.T) {
	validator := NewValidator()
	sanitizer := NewSanitizer()

	// Test coordinated validation and sanitization
	testCases := []struct {
		name        string
		input       string
		shouldValid bool
		sanitized   string
	}{
		{
			name:        "clean input",
			input:       "Normal Name",
			shouldValid: true,
			sanitized:   "Normal Name",
		},
		{
			name:        "HTML injection attempt",
			input:       "Name<script>alert('xss')</script>",
			shouldValid: true,
			sanitized:   "Name",
		},
		{
			name:        "SQL injection attempt",
			input:       "Name'; DROP TABLE users--",
			shouldValid: true,
			sanitized:   "Name&#39;; DROP TABLE users--",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Validate
			err := validator.ValidateName(tc.input)
			if (err == nil) != tc.shouldValid {
				t.Errorf("Validation result mismatch: err=%v, shouldValid=%v", err, tc.shouldValid)
			}

			// Sanitize
			clean := sanitizer.SanitizeHTML(tc.input)
			if clean != tc.sanitized {
				t.Errorf("Sanitized = %q, want %q", clean, tc.sanitized)
			}
		})
	}
}

func TestSecurityIntegration_CryptoUtils(t *testing.T) {
	crypto := NewCryptoUtil()

	// Test full crypto flow
	// 1. Generate key pair
	pubkey, privkey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// 2. Sign message
	message := "test message"
	signature := crypto.SignMessage(privkey, message)

	// 3. Verify signature
	pubkeyHex := crypto.PubkeyToHex(pubkey)
	if !crypto.VerifySignature(pubkeyHex, message, signature) {
		t.Error("Signature verification failed")
	}

	// 4. Test wrong message
	if crypto.VerifySignature(pubkeyHex, "wrong message", signature) {
		t.Error("Verification should fail for wrong message")
	}

	// 5. Test password hashing
	password := "secure_password_123"
	hash := crypto.HashPassword(password)

	if !crypto.VerifyPassword(password, hash) {
		t.Error("Password verification failed")
	}

	if crypto.VerifyPassword("wrong_password", hash) {
		t.Error("Wrong password should not verify")
	}

	// 6. Test random token generation
	token, err := crypto.GenerateRandomToken(32)
	if err != nil {
		t.Fatalf("Failed to generate random token: %v", err)
	}

	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("Token length = %d, want 64", len(token))
	}
}

func TestSecurityIntegration_HeadersManager(t *testing.T) {
	// Test headers manager with custom config
	cfg := &HeadersConfig{
		CSPDefaultSrc:     []string{"'self'"},
		CSPScriptSrc:      []string{"'self'", "'unsafe-inline'"},
		HSTSMaxAge:        31536000,
		ReferrerPolicy:    "strict-origin-when-cross-origin",
		PermissionsPolicy: map[string][]string{
			"geolocation": {},
			"microphone":  {},
		},
		EnableXFrameOptions:     true,
		EnableXContentTypeOptions: true,
		EnableXXSSProtection:    true,
	}

	hm := NewHeadersManager(cfg)

	// Verify CSP is built
	if hm.csp == "" {
		t.Error("CSP should not be empty")
	}

	// Verify HSTS is built
	if hm.hsts == "" {
		t.Error("HSTS should not be empty")
	}

	// Verify Permissions Policy is built
	if hm.pp == "" {
		t.Error("Permissions Policy should not be empty")
	}

	// Test that headers can be applied (just check no panic)
	// In real usage, this would be tested with actual HTTP response writer
}

// BenchmarkSecurityIntegration_FullAuthFlow benchmarks the complete auth flow
func BenchmarkSecurityIntegration_FullAuthFlow(b *testing.B) {
	auditLogger, _ := NewAuditLogger(&AuditConfig{BufferSize: 10000})
	defer auditLogger.Close()

	replayProtection := NewReplayProtection(5 * time.Minute)
	defer replayProtection.Close()

	ipBlocker := NewIPBlocker(&IPBlockerConfig{
		AutoBlockThreshold: 1000,
		AuditLogger:        auditLogger,
	})
	defer ipBlocker.Close()

	pubkey, privkey, _ := ed25519.GenerateKey(rand.Reader)
	pubkeyHex := hex.EncodeToString(pubkey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timestamp := fmt.Sprintf("%d_%d", time.Now().Unix(), i)
		message := "GET/test" + timestamp
		signature := ed25519.Sign(privkey, []byte(message))
		signatureHex := hex.EncodeToString(signature)

		// Full auth flow
		replayProtection.CheckAndMarkNonce(pubkeyHex, timestamp, signatureHex)
		auditLogger.Log(EventAuthSuccess, SeverityLow, "2001:db8::1", pubkeyHex, "/test", "Auth")
	}
}
