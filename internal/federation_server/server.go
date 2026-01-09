package federation_server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Server represents the federation HTTP server
type Server struct {
	config      *Config
	db          *DB
	logger      *log.Logger
	httpServer  *http.Server
	scheduler   *Scheduler
	handlers    *APIHandlers
	rateLimiter *RateLimiter
	version     string
}

// NewServer creates a new federation server instance
func NewServer(config *Config, db *DB, logger *log.Logger, version string) *Server {
	return &Server{
		config:  config,
		db:      db,
		logger:  logger,
		version: version,
	}
}

// Start starts the HTTP server and background scheduler
func (s *Server) Start() error {
	s.logger.Println("Initializing federation server components...")

	// Initialize rate limiter
	s.rateLimiter = NewRateLimiter(s.config, s.db, s.logger)

	// Initialize node manager
	nodeManager := NewNodeManager(s.db, s.config, s.logger)

	// Initialize station aggregator
	aggregator := NewStationAggregator(s.db, s.config, s.logger)

	// Initialize API handlers
	s.handlers = NewAPIHandlers(s.db, s.config, s.logger, nodeManager, s.version)

	// Initialize scheduler
	s.scheduler = NewScheduler(s.db, s.config, s.logger, aggregator, nodeManager)

	// Setup HTTP router
	mux := s.setupRouter()

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", s.config.Server.Bind, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  s.config.Server.ReadTimeout,
		WriteTimeout: s.config.Server.WriteTimeout,
		IdleTimeout:  s.config.Server.IdleTimeout,
		ErrorLog:     s.logger,
	}

	// Start scheduler
	if err := s.scheduler.Start(); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Start HTTP server in goroutine
	go func() {
		s.logger.Printf("Starting HTTP server on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatalf("HTTP server error: %v", err)
		}
	}()

	s.logger.Printf("Federation server started successfully on %s", addr)
	return nil
}

// Stop stops the HTTP server and background scheduler gracefully
func (s *Server) Stop(timeout time.Duration) error {
	s.logger.Println("Stopping federation server...")

	// Stop scheduler first
	if s.scheduler != nil {
		if err := s.scheduler.Stop(); err != nil {
			s.logger.Printf("Error stopping scheduler: %v", err)
		}
	}

	// Stop HTTP server
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.logger.Printf("HTTP server shutdown error: %v", err)
			return err
		}
	}

	s.logger.Println("Federation server stopped successfully")
	return nil
}

// setupRouter configures HTTP routes and middleware
func (s *Server) setupRouter() http.Handler {
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("/", s.handlers.HandleRoot)
	mux.HandleFunc("/api/health", s.handlers.HandleHealth)

	// Rate-limited endpoints
	mux.HandleFunc("/api/federation/register", s.withRateLimit(s.handlers.HandleRegister, "registration"))
	mux.HandleFunc("/api/stations", s.withRateLimit(s.handlers.HandleStations, "query"))
	mux.HandleFunc("/api/nodes", s.withRateLimit(s.handlers.HandleNodes, "query"))

	// Wrap with security headers and logging middleware
	handler := s.securityHeadersMiddleware(mux)
	handler = s.loggingMiddleware(handler)

	return handler
}

// === Middleware ===

// loggingMiddleware logs all HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		s.logger.Printf("%s %s from %s - %d (%v)",
			r.Method, r.URL.Path, getClientIP(r), wrapped.statusCode, duration)
	})
}

// securityHeadersMiddleware adds security headers to all responses
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'none'; object-src 'none'")

		// CORS headers (for API access from YggRadio instances)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Signature, X-Pubkey, X-Timestamp")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withRateLimit wraps a handler with rate limiting
func (s *Server) withRateLimit(handler http.HandlerFunc, limitType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)

		// Check rate limit
		allowed, err := s.rateLimiter.Allow(clientIP, limitType)
		if err != nil {
			s.logger.Printf("Rate limiter error for %s: %v", clientIP, err)
			// Allow on error to prevent disruption
			handler(w, r)
			return
		}

		if !allowed {
			s.logger.Printf("Rate limit exceeded for %s on %s", clientIP, limitType)
			s.db.LogSecurityEvent("rate_limit_exceeded", "medium", "", "", r.URL.Path, fmt.Sprintf("type: %s", limitType))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"success": false, "error": "Rate limit exceeded"}`))
			return
		}

		handler(w, r)
	}
}

// === Response Writer Wrapper ===

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// === Rate Limiter ===

// RateLimiter implements rate limiting for the federation server
type RateLimiter struct {
	config    *Config
	db        *DB
	logger    *log.Logger
	limiters  map[string]*rateLimiterEntry
	mu        sync.RWMutex
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// rateLimiterEntry holds rate limiter state for a client
type rateLimiterEntry struct {
	registrationLimiter *rate.Limiter
	queryLimiter        *rate.Limiter
	lastSeen            time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *Config, db *DB, logger *log.Logger) *RateLimiter {
	rl := &RateLimiter{
		config:      config,
		db:          db,
		logger:      logger,
		limiters:    make(map[string]*rateLimiterEntry),
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine to remove stale entries
	rl.cleanupTicker = time.NewTicker(5 * time.Minute)
	go rl.cleanupLoop()

	return rl
}

// Allow checks if a request is allowed under rate limits
func (rl *RateLimiter) Allow(clientIP, limitType string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Get or create limiter entry for client
	entry, exists := rl.limiters[clientIP]
	if !exists {
		entry = &rateLimiterEntry{
			// Registration: N per hour
			registrationLimiter: rate.NewLimiter(
				rate.Limit(float64(rl.config.RateLimit.RegistrationsPerHour)/3600.0),
				rl.config.RateLimit.RegistrationsPerHour,
			),
			// Query: N per minute
			queryLimiter: rate.NewLimiter(
				rate.Limit(float64(rl.config.RateLimit.QueriesPerMinute)/60.0),
				rl.config.RateLimit.QueriesPerMinute,
			),
			lastSeen: time.Now(),
		}
		rl.limiters[clientIP] = entry
	}

	entry.lastSeen = time.Now()

	// Check appropriate limiter based on type
	switch limitType {
	case "registration":
		return entry.registrationLimiter.Allow(), nil
	case "query":
		return entry.queryLimiter.Allow(), nil
	default:
		return true, nil // Unknown type - allow by default
	}
}

// cleanupLoop periodically removes stale rate limiter entries
func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			rl.cleanupTicker.Stop()
			return
		}
	}
}

// cleanup removes rate limiter entries not seen in last hour
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	threshold := time.Now().Add(-1 * time.Hour)
	removed := 0

	for ip, entry := range rl.limiters {
		if entry.lastSeen.Before(threshold) {
			delete(rl.limiters, ip)
			removed++
		}
	}

	if removed > 0 {
		rl.logger.Printf("Cleaned up %d stale rate limiter entries", removed)
	}
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}
