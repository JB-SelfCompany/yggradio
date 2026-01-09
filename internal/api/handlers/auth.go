package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/api/middleware"
	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/moderation"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// AuthHandler handles authentication-related API endpoints
type AuthHandler struct {
	db           *sql.DB
	userRepo     *models.UserRepository
	magicLinkMgr *security.MagicLinkManager
	powValidator *moderation.ProofOfWork
	rateLimiter  *middleware.RateLimiter
	auditLogger  *security.AuditLogger
	csrfMgr      *security.CSRFManager
	config       *config.Config
	yggAddress   string
	logger       *log.Logger
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(
	db *sql.DB,
	userRepo *models.UserRepository,
	magicLinkMgr *security.MagicLinkManager,
	powValidator *moderation.ProofOfWork,
	rateLimiter *middleware.RateLimiter,
	auditLogger *security.AuditLogger,
	csrfMgr *security.CSRFManager,
	config *config.Config,
	yggAddress string,
	logger *log.Logger,
) *AuthHandler {
	return &AuthHandler{
		db:           db,
		userRepo:     userRepo,
		magicLinkMgr: magicLinkMgr,
		powValidator: powValidator,
		rateLimiter:  rateLimiter,
		auditLogger:  auditLogger,
		csrfMgr:      csrfMgr,
		config:       config,
		yggAddress:   yggAddress,
		logger:       logger,
	}
}

// RegisterMagicLink handles POST /api/auth/register/magic-link
// Generates a new magic link for anonymous authentication
func (h *AuthHandler) RegisterMagicLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client IPv6
	clientIP := extractIPv6(r.RemoteAddr)

	// Rate limiting - 1 registration per IP per hour (enforced by middleware)

	// Parse request body
	var req struct {
		PoWHash      string `json:"pow_hash"`
		PoWNonce     string `json:"pow_nonce"`
		PoWTimestamp int64  `json:"pow_timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.auditLogger.Log(
			"magic_link_registration_invalid_json",
			security.SeverityMedium,
			"",
			"/api/auth/register/magic-link",
			fmt.Sprintf("Invalid JSON: %v", err),
		)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// SECURITY: Verify timestamp is within acceptable window (5 minutes)
	// This prevents replay attacks
	now := time.Now().Unix()
	if req.PoWTimestamp < now-300 || req.PoWTimestamp > now+300 {
		h.auditLogger.Log(
			"magic_link_registration_invalid_timestamp",
			security.SeverityHigh,
			"",
			"/api/auth/register/magic-link",
			fmt.Sprintf("Timestamp out of range: %d (current: %d)", req.PoWTimestamp, now),
		)
		http.Error(w, "Invalid timestamp", http.StatusBadRequest)
		return
	}

	// SECURITY: Verify Proof-of-Work (16 bits difficulty)
	// Challenge format: "register_magic_link_<timestamp>"
	// This prevents spam and rate limit bypass
	challengeData := fmt.Sprintf("register_magic_link_%d", req.PoWTimestamp)

	// Hash the challenge data to get the expected prefix
	hash := sha256.Sum256([]byte(challengeData + req.PoWNonce))
	hashHex := hex.EncodeToString(hash[:])

	// Verify the hash has required difficulty (16 leading zero bits = 4 hex zeros)
	difficulty := 16
	requiredZeros := difficulty / 4
	valid := true
	for i := 0; i < requiredZeros; i++ {
		if hashHex[i] != '0' {
			valid = false
			break
		}
	}

	if !valid || req.PoWHash != hashHex {
		h.auditLogger.Log(
			"magic_link_registration_invalid_pow",
			security.SeverityHigh,
			"",
			"/api/auth/register/magic-link",
			"Invalid Proof-of-Work",
		)
		http.Error(w, "Invalid Proof-of-Work", http.StatusBadRequest)
		return
	}

	// Create anonymous user in database (no pubkey)
	user, err := h.userRepo.CreateAnonymous()
	if err != nil {
		h.logger.Printf("ERROR: Failed to create anonymous user: %v", err)
		h.auditLogger.Log(
			"magic_link_registration_user_creation_failed",
			security.SeverityCritical,
			"",
			"/api/auth/register/magic-link",
			fmt.Sprintf("User creation failed: %v", err),
		)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Generate magic link token
	userAgent := r.Header.Get("User-Agent")
	token, err := h.magicLinkMgr.GenerateMagicLink(user.ID, clientIP, userAgent)
	if err != nil {
		h.logger.Printf("ERROR: Failed to generate magic link: %v", err)
		h.auditLogger.Log(
			"magic_link_generation_failed",
			security.SeverityCritical,
			"",
			"/api/auth/register/magic-link",
			fmt.Sprintf("Magic link generation failed: %v", err),
		)
		http.Error(w, "Failed to generate magic link", http.StatusInternalServerError)
		return
	}

	// Build magic link URL using Host header from request
	// This allows magic links to work with custom domains (e.g., wave.ygg)
	host := r.Host
	if host == "" {
		// Fallback to Yggdrasil IPv6 if Host header is missing
		host = fmt.Sprintf("[%s]:%d", h.yggAddress, h.config.Server.Port)
	}

	// Determine protocol (http or https)
	// NOTE: Yggdrasil provides TLS 1.3 encryption at the network layer,
	// so HTTPS is not required. All connections use http:// scheme.
	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}

	magicLinkURL := fmt.Sprintf("%s://%s/login?key=%s", protocol, host, token)

	// Success - log only to standard logger (no audit log to reduce noise)
	h.logger.Printf("Magic link registered for user %d", user.ID)

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"magic_link": magicLinkURL,
		"warning":    "Save this link in a secure location! You'll need it to access this account. Alternative: use Ed25519 keys instead.",
	})
}

// LoginWithMagicLink handles GET /login?key=<token>
// Validates magic link token, creates cookie, and redirects to homepage
func (h *AuthHandler) LoginWithMagicLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client IPv6
	clientIP := extractIPv6(r.RemoteAddr)

	// Rate limiting - 20 login attempts per IP per hour (enforced by middleware)

	// Extract token from query parameter
	token := r.URL.Query().Get("key")
	if token == "" {
		h.auditLogger.Log(
			"magic_link_login_missing_token",
			security.SeverityMedium,
			"",
			"/login",
			"Missing magic link token",
		)
		http.Error(w, "Missing magic link token", http.StatusBadRequest)
		return
	}

	// Validate magic link
	magicLinkID, userID, err := h.magicLinkMgr.ValidateMagicLink(token, clientIP)
	if err != nil {
		h.logger.Printf("Magic link validation failed: %v", err)
		h.auditLogger.Log(
			security.EventAuthFailure,
			security.SeverityHigh,
			"",
			"/login",
			fmt.Sprintf("Magic link validation failed: %v", err),
		)
		http.Error(w, "Invalid or expired magic link", http.StatusUnauthorized)
		return
	}

	// Create session cookie
	userAgent := r.Header.Get("User-Agent")
	cookieValue, expiresAt, err := h.magicLinkMgr.CreateCookie(magicLinkID, userID, clientIP, userAgent)
	if err != nil {
		h.logger.Printf("ERROR: Failed to create cookie: %v", err)
		h.auditLogger.Log(
			"cookie_creation_failed",
			security.SeverityCritical,
			"",
			"/login",
			fmt.Sprintf("Cookie creation failed: %v", err),
		)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// SECURITY: Set HttpOnly, Secure (if HTTPS), SameSite=Strict cookie
	// NOTE: Yggdrasil provides TLS 1.3 encryption at the network layer, so HTTPS is not used.
	// Secure flag will be false for HTTP connections (which is all connections in Yggdrasil).
	// This is intentional - the encryption is handled by Yggdrasil, not by HTTPS.
	isSecure := r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name:     "yggradio_auth",
		Value:    cookieValue,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   isSecure, // false for Yggdrasil (HTTP with Yggdrasil encryption)
		SameSite: http.SameSiteStrictMode,
	})

	// Success - log only to standard logger (no audit log to reduce noise)
	h.logger.Printf("Magic link login successful for user %d", userID)

	// Redirect to homepage with first_login flag and token so user can save it
	// Token is passed in URL so the frontend can display the full magic link for saving
	http.Redirect(w, r, fmt.Sprintf("/?first_login=true&key=%s", token), http.StatusSeeOther)
}

// Logout handles POST /api/auth/logout
// Logs out the current user (supports both Ed25519 and magic link)
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client IPv6
	clientIP := extractIPv6(r.RemoteAddr)

	// Get authentication method from context (set by AuthenticateAny middleware)
	authMethod, ok := middleware.GetAuthMethodFromContext(r.Context())
	if !ok {
		h.auditLogger.Log(
			"logout_no_auth_method",
			security.SeverityMedium,
			"",
			"/api/auth/logout",
			"No authentication method in context",
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user ID for logging
	userID, _ := middleware.GetUserIDFromContext(r.Context())

	// Handle logout based on auth method
	if authMethod == "magic_link" {
		// Magic link logout - delete cookie
		cookie, err := r.Cookie("yggradio_auth")
		if err == nil && cookie != nil {
			// Delete cookie from database
			if err := h.magicLinkMgr.DeleteCookie(cookie.Value, clientIP); err != nil {
				h.logger.Printf("WARNING: Failed to delete cookie from DB: %v", err)
				// Continue anyway - we'll expire the browser cookie
			}
		}

		// Expire cookie in browser
		// NOTE: Secure flag should match the original cookie (determined by TLS)
		// For Yggdrasil, this is always false (HTTP with Yggdrasil encryption)
		isSecure := r.TLS != nil
		http.SetCookie(w, &http.Cookie{
			Name:     "yggradio_auth",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   isSecure, // false for Yggdrasil
			SameSite: http.SameSiteStrictMode,
		})

		h.auditLogger.LogWithMetadata(
			"logout_magic_link",
			security.SeverityLow,
			"",
			"/api/auth/logout",
			"Magic link user logged out",
			map[string]string{
				"user_id": fmt.Sprintf("%d", userID),
			},
		)
	} else if authMethod == "ed25519" {
		// Ed25519 logout - invalidate CSRF tokens
		pubkey, _ := middleware.GetPubkeyFromContext(r.Context())
		h.csrfMgr.InvalidateUserTokens(pubkey)
		h.auditLogger.LogWithMetadata(
			"logout_ed25519",
			security.SeverityLow,
			pubkey,
			"/api/auth/logout",
			"Ed25519 user logged out",
			map[string]string{
				"user_id": fmt.Sprintf("%d", userID),
				"pubkey":  pubkey,
			},
		)
	}

	h.logger.Printf("User %d logged out (method: %s)", userID, authMethod)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// GetSessions handles GET /api/auth/sessions
// Returns list of active sessions for the current user (magic link only)
func (h *AuthHandler) GetSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client IPv6
	clientIP := extractIPv6(r.RemoteAddr)

	// Get authentication method from context
	authMethod, ok := middleware.GetAuthMethodFromContext(r.Context())
	if !ok {
		h.auditLogger.Log(
			"get_sessions_no_auth_method",
			security.SeverityMedium,
			"",
			"/api/auth/sessions",
			"No authentication method in context",
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only magic link users have sessions
	if authMethod != "magic_link" {
		h.auditLogger.Log(
			"get_sessions_wrong_auth_method",
			security.SeverityLow,
			"",
			"/api/auth/sessions",
			fmt.Sprintf("Wrong auth method: %s", authMethod),
		)
		http.Error(w, "Only available for magic link users", http.StatusBadRequest)
		return
	}

	// Get user ID
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		h.auditLogger.Log(
			"get_sessions_no_user_id",
			security.SeverityMedium,
			"",
			"/api/auth/sessions",
			"No user ID in context",
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get all active cookies for this user
	cookies, err := h.magicLinkMgr.GetActiveCookiesForUser(userID)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get active cookies for user %d: %v", userID, err)
		h.auditLogger.Log(
			"get_sessions_db_error",
			security.SeverityHigh,
			"",
			"/api/auth/sessions",
			fmt.Sprintf("Failed to get sessions: %v", err),
		)
		http.Error(w, "Failed to retrieve sessions", http.StatusInternalServerError)
		return
	}

	// Build response
	sessions := make([]map[string]interface{}, 0, len(cookies))
	for _, cookie := range cookies {
		// Determine if this is the current session
		// We can't compare cookie values directly (they're hashed in DB)
		// So we'll use a combination of IP and recent activity
		isCurrent := false
		if cookie.IPv6Address.Valid && cookie.IPv6Address.String == clientIP {
			// Check if last_used is very recent (within last minute)
			if time.Since(cookie.LastUsed) < time.Minute {
				isCurrent = true
			}
		}

		session := map[string]interface{}{
			"id":         cookie.ID,
			"created_at": cookie.CreatedAt.Format(time.RFC3339),
			"expires_at": cookie.ExpiresAt.Format(time.RFC3339),
			"last_used":  cookie.LastUsed.Format(time.RFC3339),
			"is_current": isCurrent,
		}

		// Add optional fields
		if cookie.IPv6Address.Valid {
			session["ipv6_address"] = cookie.IPv6Address.String
		}
		if cookie.UserAgent.Valid {
			session["user_agent"] = cookie.UserAgent.String
		}

		sessions = append(sessions, session)
	}

	h.logger.Printf("Retrieved %d active sessions for user %d", len(sessions), userID)

	// Return sessions list
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": sessions,
	})
}
