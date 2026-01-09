package middleware

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// Benchmark for rate limiter
func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := NewRateLimiter(1000, 100)
	ip := "2001:db8::1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.AllowAuth(ip)
	}
}

// Benchmark for concurrent rate limiting
func BenchmarkRateLimiterConcurrent(b *testing.B) {
	rl := NewRateLimiter(10000, 1000)

	b.RunParallel(func(pb *testing.PB) {
		ip := "2001:db8::1"
		for pb.Next() {
			rl.AllowAuth(ip)
		}
	})
}

// Benchmark for different IPs (simulates real scenario)
func BenchmarkRateLimiterMultipleIPs(b *testing.B) {
	rl := NewRateLimiter(1000, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := fmt.Sprintf("2001:db8::%x", i%1000)
		rl.AllowAuth(ip)
	}
}

// Benchmark for HTTP middleware
func BenchmarkRateLimiterMiddleware(b *testing.B) {
	rl := NewRateLimiter(10000, 1000)

	handler := rl.LimitAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[2001:db8::1]:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
	}
}

// Benchmark for authentication middleware (signature verification)
func BenchmarkAuthMiddlewareVerify(b *testing.B) {
	// Generate Ed25519 key pair
	pubkey, privkey, _ := ed25519.GenerateKey(rand.Reader)
	pubkeyHex := hex.EncodeToString(pubkey)

	// Create handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create request
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[2001:db8::1]:12345"

		// Create signature
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		message := fmt.Sprintf("%s%s%s", req.Method, req.URL.Path, timestamp)
		signature := ed25519.Sign(privkey, []byte(message))
		signatureHex := hex.EncodeToString(signature)

		// Set headers
		req.Header.Set("X-Pubkey", pubkeyHex)
		req.Header.Set("X-Signature", signatureHex)
		req.Header.Set("X-Timestamp", timestamp)

		// Execute (without full auth middleware to isolate verification)
		w := httptest.NewRecorder()

		// Just verify signature (core operation)
		valid := ed25519.Verify(pubkey, []byte(message), signature)
		if valid {
			handler.ServeHTTP(w, req)
		}
	}
}

// Benchmark for extractIPv6
func BenchmarkExtractIPv6(b *testing.B) {
	remoteAddrs := []string{
		"[2001:db8::1]:12345",
		"2001:db8::2:8080",
		"[::1]:80",
		"::1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addr := remoteAddrs[i%len(remoteAddrs)]
		_ = extractIPv6(addr)
	}
}

// Benchmark for upload rate limiting
func BenchmarkUploadRateLimit(b *testing.B) {
	rl := NewRateLimiter(1000, 100)
	pubkey := "test_pubkey_12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.AllowUpload(pubkey, 10)
	}
}

// Benchmark concurrent upload limiting
func BenchmarkUploadRateLimitConcurrent(b *testing.B) {
	rl := NewRateLimiter(1000, 100)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			pubkey := fmt.Sprintf("user_%d", i%100)
			rl.AllowUpload(pubkey, 10)
			i++
		}
	})
}

// Benchmark for getAPILimiter (lazy initialization)
func BenchmarkGetAPILimiter(b *testing.B) {
	rl := NewRateLimiter(1000, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := fmt.Sprintf("2001:db8::%x", i%100)
		_ = rl.getAPILimiter(ip)
	}
}

// Benchmark cleanup operation
func BenchmarkCleanup(b *testing.B) {
	rl := NewRateLimiter(1000, 100)

	// Populate with many limiters
	for i := 0; i < 10000; i++ {
		ip := fmt.Sprintf("2001:db8::%x", i)
		rl.getAPILimiter(ip)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate cleanup logic
		rl.apiLimiters.Range(func(key, value interface{}) bool {
			limiter := value.(*rate.Limiter)
			if limiter.Tokens() == float64(rl.apiBurst) {
				rl.apiLimiters.Delete(key)
			}
			return true
		})
	}
}

// Benchmark for realistic API scenario (rate limit + handler)
func BenchmarkRealisticAPIRequest(b *testing.B) {
	rl := NewRateLimiter(10000, 1000)

	// Simulate real handler with some work
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing
		time.Sleep(100 * time.Microsecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	limitedHandler := rl.LimitAPI(handler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/stations", nil)
		req.RemoteAddr = fmt.Sprintf("[2001:db8::%x]:12345", i%10)
		w := httptest.NewRecorder()

		limitedHandler.ServeHTTP(w, req)
	}
}
