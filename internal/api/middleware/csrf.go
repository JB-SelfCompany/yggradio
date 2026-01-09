package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// CSRFMiddleware provides CSRF protection middleware
type CSRFMiddleware struct {
	manager     *security.CSRFManager
	auditLogger *security.AuditLogger
}

// NewCSRFMiddleware creates a new CSRF middleware
func NewCSRFMiddleware(manager *security.CSRFManager, auditLogger *security.AuditLogger) *CSRFMiddleware {
	return &CSRFMiddleware{
		manager:     manager,
		auditLogger: auditLogger,
	}
}

// Protect provides CSRF protection for state-changing requests
func (cm *CSRFMiddleware) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only protect state-changing methods
		if !isStateMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Get user identifier (pubkey for Ed25519, synthetic pubkey for magic link)
		var userIdentifier string
		authMethod, _ := r.Context().Value("auth_method").(string)

		if authMethod == "ed25519" {
			// Ed25519 users: use actual pubkey
			pubkey, ok := r.Context().Value("pubkey").(string)
			if !ok || pubkey == "" {
				// No authentication, skip CSRF
				next.ServeHTTP(w, r)
				return
			}
			userIdentifier = pubkey
		} else if authMethod == "magic_link" {
			// Magic link users: generate synthetic pubkey from user_id
			userID, ok := r.Context().Value("user_id").(int64)
			if !ok {
				// No authentication, skip CSRF
				next.ServeHTTP(w, r)
				return
			}
			userIdentifier = fmt.Sprintf("magiclink:%054d", userID)
		} else {
			// No authentication, skip CSRF
			next.ServeHTTP(w, r)
			return
		}

		// Get CSRF token from header or form
		csrfToken := r.Header.Get("X-CSRF-Token")
		if csrfToken == "" {
			csrfToken = r.FormValue("csrf_token")
		}

		if csrfToken == "" {
			cm.logFailure(r, userIdentifier, "Missing CSRF token")
			http.Error(w, "Missing CSRF token", http.StatusForbidden)
			return
		}

		// Verify token
		if err := cm.manager.VerifyToken(csrfToken, userIdentifier); err != nil {
			cm.logFailure(r, userIdentifier, "Invalid CSRF token: "+err.Error())
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateToken generates a new CSRF token for the authenticated user
func (cm *CSRFMiddleware) GenerateToken(w http.ResponseWriter, r *http.Request) {
	// Get user identifier (pubkey for Ed25519, synthetic pubkey for magic link)
	var userIdentifier string
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use actual pubkey
		pubkey, ok := r.Context().Value("pubkey").(string)
		if !ok || pubkey == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userIdentifier = pubkey
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		userID, ok := r.Context().Value("user_id").(int64)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userIdentifier = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	token, err := cm.manager.GenerateToken(userIdentifier)
	if err != nil {
		http.Error(w, "Failed to generate CSRF token", http.StatusInternalServerError)
		return
	}

	// Return token in response
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"csrf_token":"` + token + `"}`))
}

// InjectToken injects CSRF token into the context for templates
func (cm *CSRFMiddleware) InjectToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get user identifier (pubkey for Ed25519, synthetic pubkey for magic link)
		var userIdentifier string
		authMethod, _ := r.Context().Value("auth_method").(string)

		if authMethod == "ed25519" {
			// Ed25519 users: use actual pubkey
			pubkey, ok := r.Context().Value("pubkey").(string)
			if ok && pubkey != "" {
				userIdentifier = pubkey
			}
		} else if authMethod == "magic_link" {
			// Magic link users: generate synthetic pubkey from user_id
			userID, ok := r.Context().Value("user_id").(int64)
			if ok {
				userIdentifier = fmt.Sprintf("magiclink:%054d", userID)
			}
		}

		if userIdentifier != "" {
			// Generate token for authenticated users
			token, err := cm.manager.GenerateToken(userIdentifier)
			if err == nil {
				// Add token to context
				ctx := context.WithValue(r.Context(), "csrf_token", token)
				r = r.WithContext(ctx)

				// Also set in header for convenience
				w.Header().Set("X-CSRF-Token", token)
			}
		}

		next.ServeHTTP(w, r)
	})
}

// logFailure logs a CSRF failure
func (cm *CSRFMiddleware) logFailure(r *http.Request, pubkey, details string) {
	if cm.auditLogger != nil {
		cm.auditLogger.Log(
			security.EventCSRFFailure,
			security.SeverityMedium,
			pubkey,
			r.URL.Path,
			details,
		)
	}
}

// isStateMutatingMethod checks if an HTTP method can mutate state
func isStateMutatingMethod(method string) bool {
	method = strings.ToUpper(method)
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

// extractClientIP extracts IPv6 address from remoteAddr
func extractClientIP(remoteAddr string) string {
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
	if len(parts) > 1 {
		// Rejoin all but last part (port)
		return strings.Join(parts[:len(parts)-1], ":")
	}

	return remoteAddr
}
