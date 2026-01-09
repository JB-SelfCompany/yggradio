package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// Benchmark for Validator.ValidateName
func BenchmarkValidatorValidateName(b *testing.B) {
	v := NewValidator()
	validName := "Test Station Name"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.ValidateName(validName)
	}
}

// Benchmark for Validator.ValidatePubkey
func BenchmarkValidatorValidatePubkey(b *testing.B) {
	v := NewValidator()
	pubkey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.ValidatePubkey(pubkey)
	}
}

// Benchmark for Sanitizer.SanitizeHTML
func BenchmarkSanitizerSanitizeHTML(b *testing.B) {
	s := NewSanitizer()
	html := "<p>This is some <b>HTML</b> content with <script>alert('xss')</script> tags.</p>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.SanitizeHTML(html)
	}
}

// Benchmark for Sanitizer.SanitizeString
func BenchmarkSanitizerSanitizeString(b *testing.B) {
	s := NewSanitizer()
	str := "  This is a test string with <tags> that need sanitizing  "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.SanitizeString(str)
	}
}

// Benchmark for CSRF token generation
func BenchmarkCSRFGenerateToken(b *testing.B) {
	cm := NewCSRFManager(15 * time.Minute)
	pubkey := "test_pubkey_12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cm.GenerateToken(pubkey)
	}
}

// Benchmark for CSRF token verification
func BenchmarkCSRFVerifyToken(b *testing.B) {
	cm := NewCSRFManager(15 * time.Minute)
	pubkey := "test_pubkey_12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Generate new token for each iteration
		token, _ := cm.GenerateToken(pubkey)
		_ = cm.VerifyToken(token, pubkey)
	}
}

// Benchmark for ReplayProtection check
func BenchmarkReplayProtectionCheck(b *testing.B) {
	rp := NewReplayProtection(10 * time.Minute)
	pubkey := "test_pubkey_12345"
	timestamp := "1234567890"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nonce := string(rune(i))
		_ = rp.CheckAndMarkNonce(pubkey, timestamp, nonce)
	}
}

// Benchmark for IPBlocker.IsBlocked
func BenchmarkIPBlockerIsBlocked(b *testing.B) {
	ipb := NewIPBlocker(&IPBlockerConfig{
		AutoBlockThreshold: 10,
		BlockDuration:      1 * time.Hour,
		FailureWindow:      5 * time.Minute,
		AuditLogger:        nil,
	})
	ip := "2001:db8::1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ipb.IsBlocked(ip)
	}
}

// Benchmark for IPBlocker.RecordFailure
func BenchmarkIPBlockerRecordFailure(b *testing.B) {
	ipb := NewIPBlocker(&IPBlockerConfig{
		AutoBlockThreshold: 100,
		BlockDuration:      1 * time.Hour,
		FailureWindow:      5 * time.Minute,
		AuditLogger:        nil,
	})
	ip := "2001:db8::2"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ipb.RecordFailure(ip, "test failure")
	}
}

// Benchmark for Ed25519 signature verification (for comparison)
func BenchmarkEd25519Verify(b *testing.B) {
	// Generate key pair
	pubkey, privkey, _ := ed25519.GenerateKey(rand.Reader)

	// Message to sign
	message := []byte("GET/api/stations1234567890")

	// Generate signature
	signature := ed25519.Sign(privkey, message)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ed25519.Verify(pubkey, message, signature)
	}
}

// Benchmark for Ed25519 signing (for comparison)
func BenchmarkEd25519Sign(b *testing.B) {
	// Generate key pair
	_, privkey, _ := ed25519.GenerateKey(rand.Reader)

	// Message to sign
	message := []byte("GET/api/stations1234567890")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ed25519.Sign(privkey, message)
	}
}

// Benchmark concurrent CSRF operations
func BenchmarkCSRFConcurrent(b *testing.B) {
	cm := NewCSRFManager(15 * time.Minute)

	b.RunParallel(func(pb *testing.PB) {
		pubkey := "test_pubkey"
		for pb.Next() {
			token, _ := cm.GenerateToken(pubkey)
			_ = cm.VerifyToken(token, pubkey)
		}
	})
}

// Benchmark concurrent replay protection
func BenchmarkReplayProtectionConcurrent(b *testing.B) {
	rp := NewReplayProtection(10 * time.Minute)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			pubkey := "test_pubkey"
			timestamp := "1234567890"
			nonce := string(rune(i))
			_ = rp.CheckAndMarkNonce(pubkey, timestamp, nonce)
			i++
		}
	})
}

// Benchmark for AuditLogger.Log
func BenchmarkAuditLoggerLog(b *testing.B) {
	al, err := NewAuditLogger(&AuditConfig{
		LogPath:    "", // Empty path for benchmark (no file I/O)
		EnableJSON: false,
		BufferSize: 100,
	})
	if err != nil {
		b.Fatalf("Failed to create audit logger: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		al.Log("auth_failure", "medium", "2001:db8::1", "pubkey123", "/api/test", "test details")
	}
}
