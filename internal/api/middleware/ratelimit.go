package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	apiLimiters    sync.Map
	authLimiters   sync.Map
	uploadLimiters sync.Map

	apiRate   rate.Limit
	apiBurst  int
	authRate  rate.Limit
	authBurst int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(apiPerMin, authPerMin int) *RateLimiter {
	rl := &RateLimiter{
		// API: requests per minute
		apiRate:  rate.Every(time.Minute / time.Duration(apiPerMin)),
		apiBurst: apiPerMin,

		// Auth: attempts per minute
		authRate:  rate.Every(time.Minute / time.Duration(authPerMin)),
		authBurst: authPerMin,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// LimitAPI rate limits API requests
func (rl *RateLimiter) LimitAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIPv6(r.RemoteAddr)

		limiter := rl.getAPILimiter(ip)
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// AllowAuth checks if an auth attempt is allowed
func (rl *RateLimiter) AllowAuth(ip string) bool {
	limiter := rl.getAuthLimiter(ip)
	return limiter.Allow()
}

// AllowSource checks if a source connection is allowed
func (rl *RateLimiter) AllowSource(ip string) bool {
	limiter := rl.getAPILimiter(ip)
	return limiter.Allow()
}

// AllowListener checks if a listener connection is allowed
func (rl *RateLimiter) AllowListener(ip string) bool {
	limiter := rl.getAPILimiter(ip)
	return limiter.Allow()
}

// getAPILimiter gets or creates a rate limiter for an IP
func (rl *RateLimiter) getAPILimiter(ip string) *rate.Limiter {
	if limiter, exists := rl.apiLimiters.Load(ip); exists {
		return limiter.(*rate.Limiter)
	}

	limiter := rate.NewLimiter(rl.apiRate, rl.apiBurst)
	rl.apiLimiters.Store(ip, limiter)
	return limiter
}

// getAuthLimiter gets or creates an auth rate limiter for an IP
func (rl *RateLimiter) getAuthLimiter(ip string) *rate.Limiter {
	if limiter, exists := rl.authLimiters.Load(ip); exists {
		return limiter.(*rate.Limiter)
	}

	limiter := rate.NewLimiter(rl.authRate, rl.authBurst)
	rl.authLimiters.Store(ip, limiter)
	return limiter
}

// uploadTracker tracks upload rate limits per user
type uploadTracker struct {
	count   int
	resetAt time.Time
	mu      sync.Mutex
}

// AllowUpload checks if an upload is allowed for a user
func (rl *RateLimiter) AllowUpload(pubkey string, maxPerHour int) bool {
	tracker := rl.getUploadTracker(pubkey)

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	now := time.Now()

	// Reset if hour passed
	if now.After(tracker.resetAt) {
		tracker.count = 0
		tracker.resetAt = now.Add(time.Hour)
	}

	// Check limit
	if tracker.count >= maxPerHour {
		return false
	}

	tracker.count++
	return true
}

// getUploadTracker gets or creates an upload tracker
func (rl *RateLimiter) getUploadTracker(pubkey string) *uploadTracker {
	if tracker, exists := rl.uploadLimiters.Load(pubkey); exists {
		return tracker.(*uploadTracker)
	}

	tracker := &uploadTracker{
		count:   0,
		resetAt: time.Now().Add(time.Hour),
	}
	rl.uploadLimiters.Store(pubkey, tracker)
	return tracker
}

// cleanup periodically removes inactive limiters
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Clean API limiters
		rl.apiLimiters.Range(func(key, value interface{}) bool {
			limiter := value.(*rate.Limiter)
			if limiter.Tokens() == float64(rl.apiBurst) {
				rl.apiLimiters.Delete(key)
			}
			return true
		})

		// Clean auth limiters
		rl.authLimiters.Range(func(key, value interface{}) bool {
			limiter := value.(*rate.Limiter)
			if limiter.Tokens() == float64(rl.authBurst) {
				rl.authLimiters.Delete(key)
			}
			return true
		})

		// Clean old upload trackers
		now := time.Now()
		rl.uploadLimiters.Range(func(key, value interface{}) bool {
			tracker := value.(*uploadTracker)
			tracker.mu.Lock()
			if now.After(tracker.resetAt.Add(24 * time.Hour)) {
				rl.uploadLimiters.Delete(key)
			}
			tracker.mu.Unlock()
			return true
		})
	}
}

// extractIPv6 extracts IPv6 address from RemoteAddr
func extractIPv6(remoteAddr string) string {
	// Format is typically "[::1]:port" or "::1:port"
	if strings.Contains(remoteAddr, "[") {
		// Extract from [address]:port
		parts := strings.Split(remoteAddr, "]")
		if len(parts) > 0 {
			return strings.TrimPrefix(parts[0], "[")
		}
	}

	// Extract from address:port
	parts := strings.Split(remoteAddr, ":")
	if len(parts) >= 2 {
		// Rejoin all but last part (port)
		return strings.Join(parts[:len(parts)-1], ":")
	}

	return remoteAddr
}
