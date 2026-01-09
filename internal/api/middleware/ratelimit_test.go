package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_LimitAPI(t *testing.T) {
	rl := NewRateLimiter(10, 5) // 10 API req/min, 5 auth req/min
	// Note: cleanup() is a blocking function, so we don't defer it in tests

	handler := rl.LimitAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 10 requests should succeed
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "[2001:db8::1]:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: got status %d, want %d", i, w.Code, http.StatusOK)
		}
	}

	// 11th request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[2001:db8::1]:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Rate limit: got status %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimiter_AllowAuth(t *testing.T) {
	rl := NewRateLimiter(100, 5) // 5 auth attempts/min

	ip := "2001:db8::1"

	// First 5 attempts should succeed
	for i := 0; i < 5; i++ {
		if !rl.AllowAuth(ip) {
			t.Errorf("Auth attempt %d should be allowed", i)
		}
	}

	// 6th attempt should be blocked
	if rl.AllowAuth(ip) {
		t.Error("6th auth attempt should be blocked")
	}
}

func TestRateLimiter_AllowUpload(t *testing.T) {
	rl := NewRateLimiter(100, 10)

	pubkey := "test_pubkey"
	maxPerHour := 3

	// First 3 uploads should succeed
	for i := 0; i < 3; i++ {
		if !rl.AllowUpload(pubkey, maxPerHour) {
			t.Errorf("Upload %d should be allowed", i)
		}
	}

	// 4th upload should be blocked
	if rl.AllowUpload(pubkey, maxPerHour) {
		t.Error("4th upload should be blocked")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, 2) // Low limits for testing

	handler := rl.LimitAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP1: Use up limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "[2001:db8::1]:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// IP2: Should still have its own limit
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[2001:db8::2]:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Different IP should have separate limit, got status %d", w.Code)
	}
}

func TestRateLimiter_UploadReset(t *testing.T) {
	rl := NewRateLimiter(100, 10)

	pubkey := "test_pubkey"
	maxPerHour := 2

	// Use up limit
	rl.AllowUpload(pubkey, maxPerHour)
	rl.AllowUpload(pubkey, maxPerHour)

	// Should be blocked
	if rl.AllowUpload(pubkey, maxPerHour) {
		t.Error("Upload should be blocked")
	}

	// Manually reset the tracker
	if tracker, ok := rl.uploadLimiters.Load(pubkey); ok {
		t := tracker.(*uploadTracker)
		t.mu.Lock()
		t.resetAt = time.Now().Add(-1 * time.Hour) // Force expiration
		t.mu.Unlock()
	}

	// Should be allowed after reset
	if !rl.AllowUpload(pubkey, maxPerHour) {
		t.Error("Upload should be allowed after reset")
	}
}

func TestExtractIPv6(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"bracketed IPv6", "[2001:db8::1]:12345", "2001:db8::1"},
		{"localhost bracketed", "[::1]:8080", "::1"},
		{"plain IPv6", "2001:db8::1:12345", "2001:db8::1"},
		{"full IPv6", "2001:0db8:0000:0000:0000:0000:0000:0001:12345", "2001:0db8:0000:0000:0000:0000:0000:0001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIPv6(tt.remoteAddr)
			if got != tt.want {
				t.Errorf("extractIPv6() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkRateLimiter_LimitAPI(b *testing.B) {
	rl := NewRateLimiter(1000000, 1000000) // High limits to avoid blocking

	handler := rl.LimitAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[2001:db8::1]:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkRateLimiter_AllowAuth(b *testing.B) {
	rl := NewRateLimiter(1000000, 1000000) // High limits

	ip := "2001:db8::1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.AllowAuth(ip)
	}
}

func BenchmarkRateLimiter_AllowUpload(b *testing.B) {
	rl := NewRateLimiter(1000000, 1000000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pubkey := fmt.Sprintf("pubkey%d", i)
		rl.AllowUpload(pubkey, 1000)
	}
}

func BenchmarkRateLimiter_ManyIPs(b *testing.B) {
	rl := NewRateLimiter(1000000, 1000000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := fmt.Sprintf("2001:db8::%d", i%1000)
		rl.AllowAuth(ip)
	}
}
