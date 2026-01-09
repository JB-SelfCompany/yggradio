package middleware

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// setupTestDatabase creates an in-memory SQLite database with schema
func setupTestDatabase(t *testing.T) *database.DB {
	t.Helper()

	// Create in-memory database
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Load schema from file
	schemaPath := filepath.Join("..", "..", "database", "schema.sql")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	// Execute schema
	_, err = sqlDB.Exec(string(schemaBytes))
	if err != nil {
		t.Fatalf("Failed to execute schema: %v", err)
	}

	// Wrap in database.DB
	db := &database.DB{DB: sqlDB}

	return db
}

// setupTestAuthMiddleware creates auth middleware with all dependencies
func setupTestAuthMiddleware(t *testing.T) (*AuthMiddleware, *database.DB, *security.MagicLinkManager) {
	t.Helper()

	db := setupTestDatabase(t)

	rateLimiter := NewRateLimiter(100, 50) // High limits for testing
	replayProtection := security.NewReplayProtection(5 * time.Minute)
	ipBlocker := security.NewIPBlocker(&security.IPBlockerConfig{
		AutoBlockThreshold: 10,
		BlockDuration:      1 * time.Hour,
	})

	tmpFile := filepath.Join(t.TempDir(), "test_audit.log")
	auditLogger, err := security.NewAuditLogger(&security.AuditConfig{
		LogPath: tmpFile,
	})
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	t.Cleanup(func() { auditLogger.Close() })

	authMw := NewAuthMiddleware(db, rateLimiter, replayProtection, ipBlocker, auditLogger)

	// Create magic link manager
	magicLinkMgr := security.NewMagicLinkManager(db.DB, auditLogger)
	authMw.SetMagicLinkManager(magicLinkMgr)

	return authMw, db, magicLinkMgr
}

// createTestUser creates a user and returns ID and Ed25519 keys
func createTestUser(t *testing.T, db *database.DB) (int64, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	// Generate Ed25519 keypair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	pubkeyHex := hex.EncodeToString(pubKey)

	// Create user
	userRepo := models.NewUserRepository(db.DB)
	user, err := userRepo.Create(pubkeyHex)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	return user.ID, pubKey, privKey
}

// createTestUserAnonymous creates anonymous user (no pubkey)
func createTestUserAnonymous(t *testing.T, db *database.DB) int64 {
	t.Helper()

	result, err := db.Exec("INSERT INTO users (pubkey, created_at) VALUES (NULL, CURRENT_TIMESTAMP)")
	if err != nil {
		t.Fatalf("Failed to create anonymous user: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get user ID: %v", err)
	}
	return userID
}

// signRequest creates Ed25519 signature for a request
func signRequest(method, path, timestamp string, privKey ed25519.PrivateKey) string {
	message := fmt.Sprintf("%s%s%s", method, path, timestamp)
	signature := ed25519.Sign(privKey, []byte(message))
	return hex.EncodeToString(signature)
}

func TestAuthenticateAny_Ed25519(t *testing.T) {
	authMw, db, _ := setupTestAuthMiddleware(t)
	defer db.Close()

	userID, pubKey, privKey := createTestUser(t, db)
	pubkeyHex := hex.EncodeToString(pubKey)

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication context was set
		authMethod, ok := GetAuthMethodFromContext(r.Context())
		if !ok || authMethod != "ed25519" {
			t.Errorf("Expected auth_method=ed25519, got %s", authMethod)
		}

		contextUserID, ok := GetUserIDFromContext(r.Context())
		if !ok || contextUserID != userID {
			t.Errorf("Expected user_id=%d, got %d", userID, contextUserID)
		}

		contextPubkey, ok := GetPubkeyFromContext(r.Context())
		if !ok || contextPubkey != pubkeyHex {
			t.Errorf("Expected pubkey=%s, got %s", pubkeyHex, contextPubkey)
		}

		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Valid Ed25519 signature succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		signature := signRequest("GET", "/api/test", timestamp, privKey)

		req.Header.Set("X-Pubkey", pubkeyHex)
		req.Header.Set("X-Timestamp", timestamp)
		req.Header.Set("X-Signature", signature)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Invalid signature fails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		invalidSignature := strings.Repeat("a", 128)

		req.Header.Set("X-Pubkey", pubkeyHex)
		req.Header.Set("X-Timestamp", timestamp)
		req.Header.Set("X-Signature", invalidSignature)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Missing headers fails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}

func TestAuthenticateAny_MagicLinkCookie(t *testing.T) {
	authMw, db, magicLinkMgr := setupTestAuthMiddleware(t)
	defer db.Close()

	userID := createTestUserAnonymous(t, db)

	// Generate magic link and cookie
	token, err := magicLinkMgr.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	if err != nil {
		t.Fatalf("Failed to generate magic link: %v", err)
	}

	magicLinkID, _, err := magicLinkMgr.ValidateMagicLink(token, "200:1234::1")
	if err != nil {
		t.Fatalf("Failed to validate magic link: %v", err)
	}

	cookieValue, expiresAt, err := magicLinkMgr.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
	if err != nil {
		t.Fatalf("Failed to create cookie: %v", err)
	}

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication context was set
		authMethod, ok := GetAuthMethodFromContext(r.Context())
		if !ok || authMethod != "magic_link" {
			t.Errorf("Expected auth_method=magic_link, got %s", authMethod)
		}

		contextUserID, ok := GetUserIDFromContext(r.Context())
		if !ok || contextUserID != userID {
			t.Errorf("Expected user_id=%d, got %d", userID, contextUserID)
		}

		// Pubkey should not be set for magic link users
		_, hasPubkey := GetPubkeyFromContext(r.Context())
		if hasPubkey {
			t.Error("Magic link user should not have pubkey in context")
		}

		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Valid cookie succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		req.AddCookie(&http.Cookie{
			Name:    "yggradio_auth",
			Value:   cookieValue,
			Expires: expiresAt,
		})

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Invalid cookie fails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		req.AddCookie(&http.Cookie{
			Name:  "yggradio_auth",
			Value: strings.Repeat("b", 64), // Invalid cookie
		})

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Expired cookie fails", func(t *testing.T) {
		// Create expired cookie
		expiredCookie, _, err := magicLinkMgr.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("Failed to create cookie: %v", err)
		}

		// Manually expire it in database
		hash := sha256.Sum256([]byte(expiredCookie))
		cookieHash := hex.EncodeToString(hash[:])
		_, err = db.Exec("UPDATE auth_cookies SET expires_at = datetime('now', '-1 day') WHERE cookie_hash = ?", cookieHash)
		if err != nil {
			t.Fatalf("Failed to expire cookie: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		req.AddCookie(&http.Cookie{
			Name:  "yggradio_auth",
			Value: expiredCookie,
		})

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for expired cookie, got %d", w.Code)
		}
	})
}

func TestAuthenticateAny_MagicLinkToken(t *testing.T) {
	authMw, db, magicLinkMgr := setupTestAuthMiddleware(t)
	defer db.Close()

	userID := createTestUserAnonymous(t, db)

	// Generate magic link
	token, err := magicLinkMgr.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	if err != nil {
		t.Fatalf("Failed to generate magic link: %v", err)
	}

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication context was set
		authMethod, ok := GetAuthMethodFromContext(r.Context())
		if !ok || authMethod != "magic_link" {
			t.Errorf("Expected auth_method=magic_link, got %s", authMethod)
		}

		contextUserID, ok := GetUserIDFromContext(r.Context())
		if !ok || contextUserID != userID {
			t.Errorf("Expected user_id=%d, got %d", userID, contextUserID)
		}

		// Magic link ID should be available
		magicLinkID, ok := GetMagicLinkIDFromContext(r.Context())
		if !ok || magicLinkID == 0 {
			t.Error("Magic link ID should be in context")
		}

		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Valid token in query param succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/login?key="+token, nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Invalid token fails", func(t *testing.T) {
		invalidToken := strings.Repeat("c", 48)
		req := httptest.NewRequest("GET", "/login?key="+invalidToken, nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Inactive token fails", func(t *testing.T) {
		// Create and immediately deactivate
		inactiveToken, err := magicLinkMgr.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("Failed to generate magic link: %v", err)
		}

		// Deactivate
		hash := sha256.Sum256([]byte(inactiveToken))
		tokenHash := hex.EncodeToString(hash[:])
		_, err = db.Exec("UPDATE magic_links SET is_active = 0 WHERE token_hash = ?", tokenHash)
		if err != nil {
			t.Fatalf("Failed to deactivate token: %v", err)
		}

		req := httptest.NewRequest("GET", "/login?key="+inactiveToken, nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for inactive token, got %d", w.Code)
		}
	})
}

func TestAuthenticateAny_NoAuth(t *testing.T) {
	authMw, db, _ := setupTestAuthMiddleware(t)
	defer db.Close()

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without authentication")
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("No authentication returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}

func TestAuthenticateAny_PriorityOrder(t *testing.T) {
	authMw, db, magicLinkMgr := setupTestAuthMiddleware(t)
	defer db.Close()

	// Create Ed25519 user
	ed25519UserID, pubKey, privKey := createTestUser(t, db)
	pubkeyHex := hex.EncodeToString(pubKey)

	// Create magic link user
	magicLinkUserID := createTestUserAnonymous(t, db)
	token, _ := magicLinkMgr.GenerateMagicLink(magicLinkUserID, "200:1234::1", "Test-Agent")
	magicLinkID, _, _ := magicLinkMgr.ValidateMagicLink(token, "200:1234::1")
	cookieValue, expiresAt, _ := magicLinkMgr.CreateCookie(magicLinkID, magicLinkUserID, "200:1234::1", "Test-Agent")

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authMethod, _ := GetAuthMethodFromContext(r.Context())
		userID, _ := GetUserIDFromContext(r.Context())

		// Return which method and user was authenticated
		w.Header().Set("X-Auth-Method", authMethod)
		w.Header().Set("X-User-ID", fmt.Sprintf("%d", userID))
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Ed25519 has priority over cookie", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		// Add both Ed25519 headers and cookie
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		signature := signRequest("GET", "/api/test", timestamp, privKey)

		req.Header.Set("X-Pubkey", pubkeyHex)
		req.Header.Set("X-Timestamp", timestamp)
		req.Header.Set("X-Signature", signature)

		req.AddCookie(&http.Cookie{
			Name:    "yggradio_auth",
			Value:   cookieValue,
			Expires: expiresAt,
		})

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}

		// Should use Ed25519, not cookie
		if w.Header().Get("X-Auth-Method") != "ed25519" {
			t.Errorf("Expected ed25519, got %s", w.Header().Get("X-Auth-Method"))
		}

		if w.Header().Get("X-User-ID") != fmt.Sprintf("%d", ed25519UserID) {
			t.Errorf("Expected Ed25519 user ID %d, got %s", ed25519UserID, w.Header().Get("X-User-ID"))
		}
	})

	t.Run("Cookie has priority over query token", func(t *testing.T) {
		// Create a new token for testing
		newToken, _ := magicLinkMgr.GenerateMagicLink(magicLinkUserID, "200:1234::1", "Test-Agent")

		req := httptest.NewRequest("GET", "/api/test?key="+newToken, nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		req.AddCookie(&http.Cookie{
			Name:    "yggradio_auth",
			Value:   cookieValue,
			Expires: expiresAt,
		})

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}

		// Should use cookie, not query token
		// (Cookie is validated first and returns immediately)
		if w.Header().Get("X-Auth-Method") != "magic_link" {
			t.Errorf("Expected magic_link, got %s", w.Header().Get("X-Auth-Method"))
		}
	})
}

func TestAuthenticateAny_BlockedIP(t *testing.T) {
	authMw, db, _ := setupTestAuthMiddleware(t)
	defer db.Close()

	blockedIP := "200:9999::1"

	// Block the IP
	for i := 0; i < 10; i++ {
		authMw.ipBlocker.RecordFailure(blockedIP, "test")
	}

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for blocked IP")
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Blocked IP returns 403", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = fmt.Sprintf("[%s]:12345", blockedIP)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", w.Code)
		}
	})
}

func TestAuthenticateAny_RateLimiting(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.Close()

	// Create rate limiter with low limits for testing
	rateLimiter := NewRateLimiter(100, 2) // Only 2 auth attempts
	replayProtection := security.NewReplayProtection(5 * time.Minute)
	ipBlocker := security.NewIPBlocker(&security.IPBlockerConfig{
		AutoBlockThreshold: 10,
		BlockDuration:      1 * time.Hour,
	})

	tmpFile := filepath.Join(t.TempDir(), "test_audit.log")
	auditLogger, err := security.NewAuditLogger(&security.AuditConfig{
		LogPath: tmpFile,
	})
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	t.Cleanup(func() { auditLogger.Close() })

	authMw := NewAuthMiddleware(db, rateLimiter, replayProtection, ipBlocker, auditLogger)
	magicLinkMgr := security.NewMagicLinkManager(db.DB, auditLogger)
	authMw.SetMagicLinkManager(magicLinkMgr)

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	testIP := "200:rate::limit"

	t.Run("Rate limit exceeded returns 429", func(t *testing.T) {
		// First 2 attempts with invalid cookie
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/api/test", nil)
			req.RemoteAddr = fmt.Sprintf("[%s]:12345", testIP)
			req.AddCookie(&http.Cookie{
				Name:  "yggradio_auth",
				Value: strings.Repeat("x", 64),
			})

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}

		// 3rd attempt should be rate limited
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = fmt.Sprintf("[%s]:12345", testIP)
		req.AddCookie(&http.Cookie{
			Name:  "yggradio_auth",
			Value: strings.Repeat("y", 64),
		})

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("Expected status 429, got %d", w.Code)
		}
	})
}

func TestContextHelpers(t *testing.T) {
	t.Run("GetUserIDFromContext", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "user_id", int64(123))

		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			t.Error("Expected to find user_id in context")
		}
		if userID != 123 {
			t.Errorf("Expected user_id=123, got %d", userID)
		}
	})

	t.Run("GetAuthMethodFromContext", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "auth_method", "ed25519")

		method, ok := GetAuthMethodFromContext(ctx)
		if !ok {
			t.Error("Expected to find auth_method in context")
		}
		if method != "ed25519" {
			t.Errorf("Expected auth_method=ed25519, got %s", method)
		}
	})

	t.Run("GetPubkeyFromContext", func(t *testing.T) {
		testPubkey := strings.Repeat("a", 64)
		ctx := context.WithValue(context.Background(), "pubkey", testPubkey)

		pubkey, ok := GetPubkeyFromContext(ctx)
		if !ok {
			t.Error("Expected to find pubkey in context")
		}
		if pubkey != testPubkey {
			t.Errorf("Expected pubkey=%s, got %s", testPubkey, pubkey)
		}
	})

	t.Run("GetMagicLinkIDFromContext", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "magic_link_id", int64(456))

		magicLinkID, ok := GetMagicLinkIDFromContext(ctx)
		if !ok {
			t.Error("Expected to find magic_link_id in context")
		}
		if magicLinkID != 456 {
			t.Errorf("Expected magic_link_id=456, got %d", magicLinkID)
		}
	})

	t.Run("Missing values return false", func(t *testing.T) {
		ctx := context.Background()

		_, ok := GetUserIDFromContext(ctx)
		if ok {
			t.Error("Should not find user_id in empty context")
		}

		_, ok = GetAuthMethodFromContext(ctx)
		if ok {
			t.Error("Should not find auth_method in empty context")
		}

		_, ok = GetPubkeyFromContext(ctx)
		if ok {
			t.Error("Should not find pubkey in empty context")
		}

		_, ok = GetMagicLinkIDFromContext(ctx)
		if ok {
			t.Error("Should not find magic_link_id in empty context")
		}
	})
}

// Benchmark tests
func BenchmarkAuthenticateAny_Ed25519(b *testing.B) {
	authMw, db, _ := setupTestAuthMiddleware(&testing.T{})
	defer db.Close()

	_, pubKey, privKey := createTestUser(&testing.T{}, db)
	pubkeyHex := hex.EncodeToString(pubKey)

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		signature := signRequest("GET", "/api/test", timestamp, privKey)

		req.Header.Set("X-Pubkey", pubkeyHex)
		req.Header.Set("X-Timestamp", timestamp)
		req.Header.Set("X-Signature", signature)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkAuthenticateAny_Cookie(b *testing.B) {
	authMw, db, magicLinkMgr := setupTestAuthMiddleware(&testing.T{})
	defer db.Close()

	userID := createTestUserAnonymous(&testing.T{}, db)

	token, _ := magicLinkMgr.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	magicLinkID, _, _ := magicLinkMgr.ValidateMagicLink(token, "200:1234::1")
	cookieValue, expiresAt, _ := magicLinkMgr.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")

	handler := authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "[200:1234::1]:12345"

		req.AddCookie(&http.Cookie{
			Name:    "yggradio_auth",
			Value:   cookieValue,
			Expires: expiresAt,
		})

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}
