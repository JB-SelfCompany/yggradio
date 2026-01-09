package middleware

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	db               *database.DB
	cache            *AuthCache
	rateLimiter      *RateLimiter
	replayProtection *security.ReplayProtection
	ipBlocker        *security.IPBlocker
	auditLogger      *security.AuditLogger
	magicLinkMgr     *security.MagicLinkManager
}

// AuthCache caches authentication results
type AuthCache struct {
	cache sync.Map
	ttl   time.Duration
}

// CacheEntry represents a cached auth entry
type CacheEntry struct {
	pubkey    string
	verified  bool
	expiresAt time.Time
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(db *database.DB, rateLimiter *RateLimiter, replayProtection *security.ReplayProtection, ipBlocker *security.IPBlocker, auditLogger *security.AuditLogger) *AuthMiddleware {
	return &AuthMiddleware{
		db:              db,
		rateLimiter:     rateLimiter,
		replayProtection: replayProtection,
		ipBlocker:       ipBlocker,
		auditLogger:     auditLogger,
		cache: &AuthCache{
			ttl: 5 * time.Minute,
		},
	}
}

// Authenticate validates Ed25519 signature-based authentication
func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractIPv6(r.RemoteAddr)

		// Check if IP is blocked
		if a.ipBlocker != nil {
			if blocked, reason := a.ipBlocker.IsBlocked(clientIP); blocked {
				a.logSecurityEvent(security.EventBlockedAccess, security.SeverityCritical, "", r.URL.Path,
					fmt.Sprintf("Blocked IP attempted access: %s", reason))
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
		}

		// Rate limiting on auth attempts
		if !a.rateLimiter.AllowAuth(clientIP) {
			a.logSecurityEvent(security.EventRateLimit, security.SeverityHigh, "", r.URL.Path,
				"Auth rate limit exceeded")

			// Record failure for potential auto-blocking
			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "rate limit exceeded")
			}

			http.Error(w, "Too many authentication attempts", http.StatusTooManyRequests)
			return
		}

		// Get signature headers
		signature := r.Header.Get("X-Signature")
		pubkeyHex := r.Header.Get("X-Pubkey")
		timestamp := r.Header.Get("X-Timestamp")

		if signature == "" || pubkeyHex == "" || timestamp == "" {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityLow, "", r.URL.Path,
				"Missing authentication headers")

			// Record failure
			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "missing auth headers")
			}

			http.Error(w, "Missing authentication headers", http.StatusUnauthorized)
			return
		}

		// Validate pubkey format (64 hex chars)
		if len(pubkeyHex) != 64 {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityMedium, "", r.URL.Path,
				"Invalid pubkey format")

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "invalid pubkey format")
			}

			http.Error(w, "Invalid pubkey format", http.StatusUnauthorized)
			return
		}

		// Verify timestamp (protection against replay attacks)
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityMedium, pubkeyHex, r.URL.Path,
				"Invalid timestamp format")

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "invalid timestamp")
			}

			http.Error(w, "Invalid timestamp", http.StatusUnauthorized)
			return
		}

		// Allow ±5 minutes window
		currentTime := time.Now().Unix()
		timeDiff := currentTime - ts
		if timeDiff > 300 || timeDiff < -300 {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityHigh, pubkeyHex, r.URL.Path,
				fmt.Sprintf("Timestamp outside valid window: %d seconds", timeDiff))

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "timestamp out of window")
			}

			http.Error(w, "Timestamp outside valid window", http.StatusUnauthorized)
			return
		}

		// Check for replay attack
		if a.replayProtection != nil {
			if err := a.replayProtection.CheckAndMarkNonce(pubkeyHex, timestamp, signature); err != nil {
				a.logSecurityEvent(security.EventReplayAttack, security.SeverityCritical, pubkeyHex, r.URL.Path,
					"Replay attack detected: "+err.Error())

				// Severe security violation - record multiple failures
				if a.ipBlocker != nil {
					for i := 0; i < 5; i++ {
						a.ipBlocker.RecordFailure(clientIP, "replay attack")
					}
				}

				http.Error(w, "Replay attack detected", http.StatusForbidden)
				return
			}
		}

		// Check cache
		cacheKey := fmt.Sprintf("%s:%s:%s", pubkeyHex, timestamp, signature)
		if cached, found := a.cache.Get(cacheKey); found {
			ctx := context.WithValue(r.Context(), "pubkey", cached.pubkey)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Reconstruct message to verify
		// Include query string if present
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path = path + "?" + r.URL.RawQuery
		}
		message := fmt.Sprintf("%s%s%s", r.Method, path, timestamp)

		// Decode keys
		pubkey, err := hex.DecodeString(pubkeyHex)
		if err != nil || len(pubkey) != ed25519.PublicKeySize {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityMedium, pubkeyHex, r.URL.Path,
				"Invalid pubkey encoding")

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "invalid pubkey encoding")
			}

			http.Error(w, "Invalid pubkey", http.StatusUnauthorized)
			return
		}

		sig, err := hex.DecodeString(signature)
		if err != nil || len(sig) != ed25519.SignatureSize {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityMedium, pubkeyHex, r.URL.Path,
				"Invalid signature encoding")

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "invalid signature encoding")
			}

			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}

		// Verify Ed25519 signature
		if !ed25519.Verify(pubkey, []byte(message), sig) {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityHigh, pubkeyHex, r.URL.Path,
				"Signature verification failed")

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "signature verification failed")
			}

			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}

		// Authentication successful - no logging (reduces noise)

		// Reset failure count for this IP on successful auth
		if a.ipBlocker != nil {
			a.ipBlocker.ResetFailures(clientIP)
		}

		// Save to cache
		a.cache.Set(cacheKey, pubkeyHex, 5*time.Minute)

		// Get or create user by pubkey to obtain user_id
		userRepo := models.NewUserRepository(a.db.DB)
		user, err := userRepo.GetOrCreate(pubkeyHex)
		if err != nil {
			// Failed to auto-register user
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityCritical, pubkeyHex, r.URL.Path,
				fmt.Sprintf("Failed to get or create user: %v", err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Auto-registration complete (no logging to reduce noise)

		// Add authentication context (Ed25519 method)
		ctx := context.WithValue(r.Context(), "auth_method", "ed25519")
		ctx = context.WithValue(ctx, "user_id", user.ID)
		ctx = context.WithValue(ctx, "pubkey", pubkeyHex)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SetMagicLinkManager sets the magic link manager for cookie-based authentication
func (a *AuthMiddleware) SetMagicLinkManager(mgr *security.MagicLinkManager) {
	a.magicLinkMgr = mgr
}

// AuthenticateAny validates either Ed25519 signature or Magic Link cookie authentication
// Checks authentication methods in this order:
// 1. Ed25519 headers (X-Signature, X-Pubkey) - uses existing Authenticate()
// 2. Cookie (yggradio_auth) - validates session cookie
// 3. Query param (?key=token) - validates magic link token
// Returns 401 if all methods fail
func (a *AuthMiddleware) AuthenticateAny(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractIPv6(r.RemoteAddr)

		// Check if IP is blocked (applies to all auth methods)
		if a.ipBlocker != nil {
			if blocked, reason := a.ipBlocker.IsBlocked(clientIP); blocked {
				a.logSecurityEvent(security.EventBlockedAccess, security.SeverityCritical, "", r.URL.Path,
					fmt.Sprintf("Blocked IP attempted access: %s", reason))
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
		}

		// 1. Check for Ed25519 headers first (highest security)
		if r.Header.Get("X-Signature") != "" && r.Header.Get("X-Pubkey") != "" {
			// Use existing Authenticate middleware
			a.Authenticate(next).ServeHTTP(w, r)
			return
		}

		// 2. Check for session cookie (magic link users)
		cookie, err := r.Cookie("yggradio_auth")
		if err == nil && cookie.Value != "" {
			// Rate limiting on cookie validation attempts
			if !a.rateLimiter.AllowAuth(clientIP) {
				a.logSecurityEvent(security.EventRateLimit, security.SeverityHigh, "", r.URL.Path,
					"Cookie validation rate limit exceeded")

				if a.ipBlocker != nil {
					a.ipBlocker.RecordFailure(clientIP, "rate limit exceeded")
				}

				http.Error(w, "Too many authentication attempts", http.StatusTooManyRequests)
				return
			}

			// Validate cookie
			if a.magicLinkMgr == nil {
				a.logSecurityEvent(security.EventAuthFailure, security.SeverityCritical, "", r.URL.Path,
					"MagicLinkManager not initialized")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			userID, err := a.magicLinkMgr.ValidateCookie(cookie.Value, clientIP)
			if err == nil {
				// Cookie is valid - authentication successful (no logging to reduce noise)

				// Reset failure count for this IP on successful auth
				if a.ipBlocker != nil {
					a.ipBlocker.ResetFailures(clientIP)
				}

				// Add authentication context (magic_link method)
				ctx := context.WithValue(r.Context(), "auth_method", "magic_link")
				ctx = context.WithValue(ctx, "user_id", userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Cookie validation failed - log and continue to next method
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityMedium, "", r.URL.Path,
				"Invalid or expired cookie")

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "invalid cookie")
			}
		}

		// 3. Check for magic link token in query param (?key=<token>)
		token := r.URL.Query().Get("key")
		if token != "" {
			// Rate limiting on magic link validation attempts
			if !a.rateLimiter.AllowAuth(clientIP) {
				a.logSecurityEvent(security.EventRateLimit, security.SeverityHigh, "", r.URL.Path,
					"Magic link validation rate limit exceeded")

				if a.ipBlocker != nil {
					a.ipBlocker.RecordFailure(clientIP, "rate limit exceeded")
				}

				http.Error(w, "Too many authentication attempts", http.StatusTooManyRequests)
				return
			}

			// Validate magic link token
			if a.magicLinkMgr == nil {
				a.logSecurityEvent(security.EventAuthFailure, security.SeverityCritical, "", r.URL.Path,
					"MagicLinkManager not initialized")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			magicLinkID, userID, err := a.magicLinkMgr.ValidateMagicLink(token, clientIP)
			if err == nil {
				// Magic link is valid - authentication successful (no logging to reduce noise)
				// Note: Cookie creation and redirect should be handled by the handler
				// Here we just validate the token and add to context

				// Reset failure count for this IP on successful auth
				if a.ipBlocker != nil {
					a.ipBlocker.ResetFailures(clientIP)
				}

				// Add authentication context (magic_link method)
				// Also add magic_link_id for handler to create cookie
				ctx := context.WithValue(r.Context(), "auth_method", "magic_link")
				ctx = context.WithValue(ctx, "user_id", userID)
				ctx = context.WithValue(ctx, "magic_link_id", magicLinkID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Magic link validation failed
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityHigh, "", r.URL.Path,
				"Invalid or expired magic link token")

			if a.ipBlocker != nil {
				a.ipBlocker.RecordFailure(clientIP, "invalid magic link")
			}
		}

		// All authentication methods failed (expected - user not authenticated, no logging to reduce noise)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// OptionalAuth provides optional authentication (doesn't fail if not authenticated)
func (a *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pubkeyHex := r.Header.Get("X-Pubkey")
		if pubkeyHex != "" {
			// Try to authenticate, but don't fail if it doesn't work
			a.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})).ServeHTTP(w, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

// RequireAdmin requires the user to be an admin
func (a *AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractIPv6(r.RemoteAddr)

		pubkey := r.Context().Value("pubkey")
		if pubkey == nil {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityMedium,
				"", r.URL.Path, "Admin access without authentication")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		pubkeyStr := pubkey.(string)

		// ✅ Check if user is admin in database
		userRepo := models.NewUserRepository(a.db.DB)
		isAdmin, err := userRepo.IsAdmin(pubkeyStr)
		if err != nil {
			a.logSecurityEvent(security.EventAuthFailure, security.SeverityHigh,
				pubkeyStr, r.URL.Path,
				"Database error during admin check: "+err.Error())
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !isAdmin {
			a.logSecurityEvent(security.EventBlockedAccess, security.SeverityCritical,
				pubkeyStr, r.URL.Path,
				"Non-admin attempted admin access")

			// Record multiple failures for attempted privilege escalation
			if a.ipBlocker != nil {
				for i := 0; i < 3; i++ {
					a.ipBlocker.RecordFailure(clientIP, "admin access attempt")
				}
			}

			http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
			return
		}

		// ✅ Admin verified - log successful admin access
		a.logSecurityEvent(security.EventAuthSuccess, security.SeverityLow,
			pubkeyStr, r.URL.Path, "Admin access granted")

		next.ServeHTTP(w, r)
	})
}

// Get retrieves a cached entry
func (c *AuthCache) Get(key string) (*CacheEntry, bool) {
	if val, ok := c.cache.Load(key); ok {
		entry := val.(*CacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry, true
		}
		c.cache.Delete(key)
	}
	return nil, false
}

// Set stores a cache entry
func (c *AuthCache) Set(key, pubkey string, ttl time.Duration) {
	entry := &CacheEntry{
		pubkey:    pubkey,
		verified:  true,
		expiresAt: time.Now().Add(ttl),
	}
	c.cache.Store(key, entry)
}

// logSecurityEvent logs a security event (privacy: no IP logging)
func (a *AuthMiddleware) logSecurityEvent(eventType, severity, pubkey, endpoint, details string) {
	// Log to database if available (without IP address for privacy)
	if a.db != nil {
		a.db.LogSecurityEvent(eventType, severity, "", pubkey, endpoint, details)
	}

	// Also log to audit logger (without IP address for privacy)
	if a.auditLogger != nil {
		a.auditLogger.Log(eventType, severity, pubkey, endpoint, details)
	}
}

// GetUserIDFromContext extracts the user ID from the request context
// Returns the user ID and true if present, or 0 and false if not found
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value("user_id").(int64)
	return userID, ok
}

// GetAuthMethodFromContext extracts the authentication method from the request context
// Returns either "ed25519" or "magic_link" and true if present, or empty string and false if not found
func GetAuthMethodFromContext(ctx context.Context) (string, bool) {
	method, ok := ctx.Value("auth_method").(string)
	return method, ok
}

// GetPubkeyFromContext extracts the Ed25519 public key from the request context
// This is only available for Ed25519 authenticated users
// Returns the pubkey and true if present, or empty string and false if not found
func GetPubkeyFromContext(ctx context.Context) (string, bool) {
	pubkey, ok := ctx.Value("pubkey").(string)
	return pubkey, ok
}

// GetMagicLinkIDFromContext extracts the magic link ID from the request context
// This is only available during magic link validation (before cookie is created)
// Returns the magic link ID and true if present, or 0 and false if not found
func GetMagicLinkIDFromContext(ctx context.Context) (int64, bool) {
	magicLinkID, ok := ctx.Value("magic_link_id").(int64)
	return magicLinkID, ok
}
