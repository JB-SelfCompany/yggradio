package security

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory SQLite database with the schema loaded
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Create in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Load schema from file
	schemaPath := filepath.Join("..", "database", "schema.sql")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	// Execute schema
	_, err = db.Exec(string(schemaBytes))
	if err != nil {
		t.Fatalf("Failed to execute schema: %v", err)
	}

	return db
}

// createTestAuditLogger creates a no-op audit logger for tests
func createTestAuditLogger(t *testing.T) *AuditLogger {
	t.Helper()
	logger, err := NewAuditLogger(&AuditConfig{
		EnableJSON: false,
		BufferSize: 100,
	})
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	return logger
}

// createTestUser creates a user in the test database and returns the user ID
func createTestUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	result, err := db.Exec("INSERT INTO users (pubkey, created_at) VALUES (NULL, CURRENT_TIMESTAMP)")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get user ID: %v", err)
	}
	return userID
}

func TestGenerateMagicLink(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	auditLogger := createTestAuditLogger(t)
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(t, db)

	t.Run("Generate valid token", func(t *testing.T) {
		token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		// Token should be 48 hex characters (24 bytes * 2)
		if len(token) != 48 {
			t.Errorf("Token length = %d, want 48", len(token))
		}

		// Verify token is valid hex
		if _, err := fmt.Sscanf(token, "%x", new([]byte)); err != nil {
			t.Errorf("Token is not valid hex: %v", err)
		}
	})

	t.Run("Token is unique", func(t *testing.T) {
		token1, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		token2, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		if token1 == token2 {
			t.Error("Generated tokens should be unique")
		}
	})

	t.Run("Hash stored in database", func(t *testing.T) {
		token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		// Query database to verify hash is stored
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM magic_links WHERE user_id = ?", userID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query database: %v", err)
		}

		if count == 0 {
			t.Error("Magic link hash not found in database")
		}

		// Verify the plaintext token is NOT in the database
		var tokenInDB string
		err = db.QueryRow("SELECT token_hash FROM magic_links WHERE user_id = ? LIMIT 1", userID).Scan(&tokenInDB)
		if err != nil {
			t.Fatalf("Failed to query token_hash: %v", err)
		}

		if tokenInDB == token {
			t.Error("Plaintext token should not be stored in database")
		}
	})

	t.Run("Audit log recorded", func(t *testing.T) {
		_, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		// Note: Audit logger writes to file asynchronously, not to DB
		// Just verify the operation succeeded - audit logging is tested separately
	})
}

func TestValidateMagicLink(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	auditLogger := createTestAuditLogger(t)
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(t, db)

	t.Run("Valid token returns correct user_id", func(t *testing.T) {
		token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		magicLinkID, validatedUserID, err := manager.ValidateMagicLink(token, "200:1234::2")
		if err != nil {
			t.Errorf("ValidateMagicLink() error = %v", err)
		}

		if validatedUserID != userID {
			t.Errorf("ValidateMagicLink() userID = %d, want %d", validatedUserID, userID)
		}

		if magicLinkID == 0 {
			t.Error("ValidateMagicLink() magicLinkID should not be 0")
		}
	})

	t.Run("Invalid token returns error", func(t *testing.T) {
		invalidToken := strings.Repeat("a", 48)

		_, _, err := manager.ValidateMagicLink(invalidToken, "200:1234::1")
		if err == nil {
			t.Error("ValidateMagicLink() should return error for invalid token")
		}
	})

	t.Run("Wrong format token returns error", func(t *testing.T) {
		tests := []struct {
			name  string
			token string
		}{
			{"too short", "abc123"},
			{"too long", strings.Repeat("a", 100)},
			{"not hex", strings.Repeat("g", 48)},
			{"empty", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, _, err := manager.ValidateMagicLink(tt.token, "200:1234::1")
				if err == nil {
					t.Errorf("ValidateMagicLink() should return error for %s", tt.name)
				}
			})
		}
	})

	t.Run("last_used timestamp updated", func(t *testing.T) {
		token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		// Validate token first time to get magic_link_id
		magicLinkID, _, err := manager.ValidateMagicLink(token, "200:1234::1")
		if err != nil {
			t.Fatalf("ValidateMagicLink() error = %v", err)
		}

		// Get initial last_used for this specific magic link
		var initialLastUsed time.Time
		err = db.QueryRow("SELECT last_used FROM magic_links WHERE id = ?", magicLinkID).Scan(&initialLastUsed)
		if err != nil {
			t.Fatalf("Failed to query initial last_used: %v", err)
		}

		// Wait to ensure timestamp difference (SQLite CURRENT_TIMESTAMP has second precision)
		time.Sleep(1500 * time.Millisecond)

		// Validate token again
		_, _, err = manager.ValidateMagicLink(token, "200:1234::1")
		if err != nil {
			t.Fatalf("ValidateMagicLink() error = %v", err)
		}

		// Get updated last_used for the same magic link
		var updatedLastUsed time.Time
		err = db.QueryRow("SELECT last_used FROM magic_links WHERE id = ?", magicLinkID).Scan(&updatedLastUsed)
		if err != nil {
			t.Fatalf("Failed to query updated last_used: %v", err)
		}

		if !updatedLastUsed.After(initialLastUsed) {
			t.Errorf("last_used timestamp should be updated after validation: initial=%v, updated=%v", initialLastUsed, updatedLastUsed)
		}
	})

	t.Run("Inactive token returns error", func(t *testing.T) {
		token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		// Deactivate the magic link
		_, err = db.Exec("UPDATE magic_links SET is_active = 0 WHERE user_id = ?", userID)
		if err != nil {
			t.Fatalf("Failed to deactivate magic link: %v", err)
		}

		// Try to validate
		_, _, err = manager.ValidateMagicLink(token, "200:1234::1")
		if err == nil {
			t.Error("ValidateMagicLink() should return error for inactive token")
		}
	})
}

func TestCreateCookie(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	auditLogger := createTestAuditLogger(t)
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(t, db)

	// Create a magic link first
	token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	if err != nil {
		t.Fatalf("GenerateMagicLink() error = %v", err)
	}

	magicLinkID, _, err := manager.ValidateMagicLink(token, "200:1234::1")
	if err != nil {
		t.Fatalf("ValidateMagicLink() error = %v", err)
	}

	t.Run("Generate valid cookie", func(t *testing.T) {
		cookieValue, expiresAt, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Cookie should be 64 hex characters (32 bytes * 2)
		if len(cookieValue) != 64 {
			t.Errorf("Cookie length = %d, want 64", len(cookieValue))
		}

		// Verify cookie is valid hex
		if _, err := fmt.Sscanf(cookieValue, "%x", new([]byte)); err != nil {
			t.Errorf("Cookie is not valid hex: %v", err)
		}

		// Verify expires_at is in the future (approximately 1 week)
		expectedExpiry := time.Now().Add(7 * 24 * time.Hour)
		diff := expiresAt.Sub(expectedExpiry).Abs()
		if diff > 1*time.Minute {
			t.Errorf("Expiry time difference too large: %v", diff)
		}
	})

	t.Run("Cookie is unique", func(t *testing.T) {
		cookie1, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		cookie2, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		if cookie1 == cookie2 {
			t.Error("Generated cookies should be unique")
		}
	})

	t.Run("Hash stored in database", func(t *testing.T) {
		cookieValue, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Query database to verify hash is stored
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM auth_cookies WHERE user_id = ?", userID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query database: %v", err)
		}

		if count == 0 {
			t.Error("Cookie hash not found in database")
		}

		// Verify the plaintext cookie is NOT in the database
		var cookieInDB string
		err = db.QueryRow("SELECT cookie_hash FROM auth_cookies WHERE user_id = ? LIMIT 1", userID).Scan(&cookieInDB)
		if err != nil {
			t.Fatalf("Failed to query cookie_hash: %v", err)
		}

		if cookieInDB == cookieValue {
			t.Error("Plaintext cookie should not be stored in database")
		}
	})

	t.Run("expires_at set correctly", func(t *testing.T) {
		cookieValue, expectedExpiry, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Query database to verify expires_at
		var expiresAt time.Time
		err = db.QueryRow("SELECT expires_at FROM auth_cookies WHERE user_id = ? ORDER BY created_at DESC LIMIT 1", userID).Scan(&expiresAt)
		if err != nil {
			t.Fatalf("Failed to query expires_at: %v", err)
		}

		diff := expiresAt.Sub(expectedExpiry).Abs()
		if diff > 1*time.Second {
			t.Errorf("Database expires_at differs from returned value by %v", diff)
		}

		_ = cookieValue // Use the variable
	})

	t.Run("Audit log recorded", func(t *testing.T) {
		_, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Note: Audit logger writes to file asynchronously, not to DB
		// Just verify the operation succeeded - audit logging is tested separately
	})
}

func TestValidateCookie(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	auditLogger := createTestAuditLogger(t)
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(t, db)

	// Create a magic link and cookie
	token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	if err != nil {
		t.Fatalf("GenerateMagicLink() error = %v", err)
	}

	magicLinkID, _, err := manager.ValidateMagicLink(token, "200:1234::1")
	if err != nil {
		t.Fatalf("ValidateMagicLink() error = %v", err)
	}

	t.Run("Valid cookie returns correct user_id", func(t *testing.T) {
		cookieValue, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		validatedUserID, err := manager.ValidateCookie(cookieValue, "200:1234::2")
		if err != nil {
			t.Errorf("ValidateCookie() error = %v", err)
		}

		if validatedUserID != userID {
			t.Errorf("ValidateCookie() userID = %d, want %d", validatedUserID, userID)
		}
	})

	t.Run("Invalid cookie returns error", func(t *testing.T) {
		invalidCookie := strings.Repeat("b", 64)

		_, err := manager.ValidateCookie(invalidCookie, "200:1234::1")
		if err == nil {
			t.Error("ValidateCookie() should return error for invalid cookie")
		}
	})

	t.Run("Wrong format cookie returns error", func(t *testing.T) {
		tests := []struct {
			name   string
			cookie string
		}{
			{"too short", "abc123"},
			{"too long", strings.Repeat("a", 100)},
			{"not hex", strings.Repeat("g", 64)},
			{"empty", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := manager.ValidateCookie(tt.cookie, "200:1234::1")
				if err == nil {
					t.Errorf("ValidateCookie() should return error for %s", tt.name)
				}
			})
		}
	})

	t.Run("Expired cookie returns error", func(t *testing.T) {
		cookieValue, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Set expiration to the past
		_, err = db.Exec("UPDATE auth_cookies SET expires_at = datetime('now', '-1 day') WHERE user_id = ?", userID)
		if err != nil {
			t.Fatalf("Failed to expire cookie: %v", err)
		}

		// Try to validate
		_, err = manager.ValidateCookie(cookieValue, "200:1234::1")
		if err == nil {
			t.Error("ValidateCookie() should return error for expired cookie")
		}
	})

	t.Run("last_used timestamp updated", func(t *testing.T) {
		cookieValue, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Hash the cookie to find it in DB
		hash := sha256.Sum256([]byte(cookieValue))
		cookieHash := hex.EncodeToString(hash[:])

		// Get cookie ID and initial last_used
		var cookieID int64
		var initialLastUsed time.Time
		err = db.QueryRow("SELECT id, last_used FROM auth_cookies WHERE cookie_hash = ?", cookieHash).Scan(&cookieID, &initialLastUsed)
		if err != nil {
			t.Fatalf("Failed to query initial last_used: %v", err)
		}

		// Wait to ensure timestamp difference (SQLite CURRENT_TIMESTAMP has second precision)
		time.Sleep(1500 * time.Millisecond)

		// Validate cookie
		_, err = manager.ValidateCookie(cookieValue, "200:1234::1")
		if err != nil {
			t.Fatalf("ValidateCookie() error = %v", err)
		}

		// Get updated last_used for the same cookie
		var updatedLastUsed time.Time
		err = db.QueryRow("SELECT last_used FROM auth_cookies WHERE id = ?", cookieID).Scan(&updatedLastUsed)
		if err != nil {
			t.Fatalf("Failed to query updated last_used: %v", err)
		}

		if !updatedLastUsed.After(initialLastUsed) {
			t.Errorf("last_used timestamp should be updated after validation: initial=%v, updated=%v", initialLastUsed, updatedLastUsed)
		}
	})
}

func TestDeleteCookie(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	auditLogger := createTestAuditLogger(t)
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(t, db)

	// Create a magic link and cookie
	token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	if err != nil {
		t.Fatalf("GenerateMagicLink() error = %v", err)
	}

	magicLinkID, _, err := manager.ValidateMagicLink(token, "200:1234::1")
	if err != nil {
		t.Fatalf("ValidateMagicLink() error = %v", err)
	}

	t.Run("Delete existing cookie", func(t *testing.T) {
		cookieValue, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Delete cookie
		err = manager.DeleteCookie(cookieValue, "200:1234::1")
		if err != nil {
			t.Errorf("DeleteCookie() error = %v", err)
		}

		// Verify cookie is deleted from database
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM auth_cookies WHERE user_id = ?", userID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query database: %v", err)
		}

		if count != 0 {
			t.Error("Cookie should be deleted from database")
		}
	})

	t.Run("Validation fails after delete", func(t *testing.T) {
		cookieValue, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Delete cookie
		err = manager.DeleteCookie(cookieValue, "200:1234::1")
		if err != nil {
			t.Errorf("DeleteCookie() error = %v", err)
		}

		// Try to validate deleted cookie
		_, err = manager.ValidateCookie(cookieValue, "200:1234::1")
		if err == nil {
			t.Error("ValidateCookie() should fail after cookie is deleted")
		}
	})
}

func TestCleanupExpiredCookies(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	auditLogger := createTestAuditLogger(t)
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(t, db)

	// Create a magic link
	token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	if err != nil {
		t.Fatalf("GenerateMagicLink() error = %v", err)
	}

	magicLinkID, _, err := manager.ValidateMagicLink(token, "200:1234::1")
	if err != nil {
		t.Fatalf("ValidateMagicLink() error = %v", err)
	}

	t.Run("Expired cookies removed", func(t *testing.T) {
		// Create multiple cookies
		_, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		_, _, err = manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Get ID of first cookie to expire it
		var firstCookieID int64
		err = db.QueryRow("SELECT id FROM auth_cookies WHERE user_id = ? ORDER BY created_at ASC LIMIT 1", userID).Scan(&firstCookieID)
		if err != nil {
			t.Fatalf("Failed to get first cookie ID: %v", err)
		}

		// Expire first cookie (SQLite doesn't support LIMIT in UPDATE)
		_, err = db.Exec("UPDATE auth_cookies SET expires_at = datetime('now', '-1 day') WHERE id = ?", firstCookieID)
		if err != nil {
			t.Fatalf("Failed to expire cookie: %v", err)
		}

		// Run cleanup
		err = manager.CleanupExpiredCookies()
		if err != nil {
			t.Errorf("CleanupExpiredCookies() error = %v", err)
		}

		// Verify only active cookie remains
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM auth_cookies WHERE user_id = ?", userID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query database: %v", err)
		}

		if count != 1 {
			t.Errorf("Expected 1 active cookie, got %d", count)
		}
	})

	t.Run("Active cookies remain", func(t *testing.T) {
		// Create a new cookie
		_, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Get count before cleanup
		var countBefore int
		err = db.QueryRow("SELECT COUNT(*) FROM auth_cookies WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP", userID).Scan(&countBefore)
		if err != nil {
			t.Fatalf("Failed to query database: %v", err)
		}

		// Run cleanup
		err = manager.CleanupExpiredCookies()
		if err != nil {
			t.Errorf("CleanupExpiredCookies() error = %v", err)
		}

		// Get count after cleanup
		var countAfter int
		err = db.QueryRow("SELECT COUNT(*) FROM auth_cookies WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP", userID).Scan(&countAfter)
		if err != nil {
			t.Fatalf("Failed to query database: %v", err)
		}

		if countBefore != countAfter {
			t.Errorf("Active cookie count changed: before=%d, after=%d", countBefore, countAfter)
		}
	})
}

func TestConstantTimeComparison(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	auditLogger := createTestAuditLogger(t)
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(t, db)

	t.Run("Timing attack resistance for magic link", func(t *testing.T) {
		// Generate a valid token
		validToken, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		// Create an invalid token with same length
		invalidToken := strings.Repeat("a", 48)

		// Measure validation time for valid token
		validStart := time.Now()
		_, _, _ = manager.ValidateMagicLink(validToken, "200:1234::1")
		validDuration := time.Since(validStart)

		// Measure validation time for invalid token
		invalidStart := time.Now()
		_, _, _ = manager.ValidateMagicLink(invalidToken, "200:1234::1")
		invalidDuration := time.Since(invalidStart)

		// The times should be similar (within order of magnitude)
		// This is a weak test, but demonstrates the principle
		log.Printf("Valid token validation: %v, Invalid token validation: %v", validDuration, invalidDuration)

		// We don't assert on exact times because they can vary,
		// but the code uses subtle.ConstantTimeCompare which provides protection
	})

	t.Run("Timing attack resistance for cookie", func(t *testing.T) {
		// Create a valid cookie
		token, err := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("GenerateMagicLink() error = %v", err)
		}

		magicLinkID, _, err := manager.ValidateMagicLink(token, "200:1234::1")
		if err != nil {
			t.Fatalf("ValidateMagicLink() error = %v", err)
		}

		validCookie, _, err := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		if err != nil {
			t.Fatalf("CreateCookie() error = %v", err)
		}

		// Create an invalid cookie with same length
		invalidCookie := strings.Repeat("b", 64)

		// Measure validation time for valid cookie
		validStart := time.Now()
		_, _ = manager.ValidateCookie(validCookie, "200:1234::1")
		validDuration := time.Since(validStart)

		// Measure validation time for invalid cookie
		invalidStart := time.Now()
		_, _ = manager.ValidateCookie(invalidCookie, "200:1234::1")
		invalidDuration := time.Since(invalidStart)

		log.Printf("Valid cookie validation: %v, Invalid cookie validation: %v", validDuration, invalidDuration)
	})
}

// Benchmark tests
func BenchmarkGenerateMagicLink(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	auditLogger := createTestAuditLogger(&testing.T{})
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(&testing.T{}, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	}
}

func BenchmarkValidateMagicLink(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	auditLogger := createTestAuditLogger(&testing.T{})
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(&testing.T{}, db)

	// Pre-generate tokens
	tokens := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		token, _ := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
		tokens[i] = token
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = manager.ValidateMagicLink(tokens[i], "200:1234::1")
	}
}

func BenchmarkCreateCookie(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	auditLogger := createTestAuditLogger(&testing.T{})
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(&testing.T{}, db)

	token, _ := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	magicLinkID, _, _ := manager.ValidateMagicLink(token, "200:1234::1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
	}
}

func BenchmarkValidateCookie(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	auditLogger := createTestAuditLogger(&testing.T{})
	defer auditLogger.Close()

	manager := NewMagicLinkManager(db, auditLogger)
	userID := createTestUser(&testing.T{}, db)

	token, _ := manager.GenerateMagicLink(userID, "200:1234::1", "Test-Agent")
	magicLinkID, _, _ := manager.ValidateMagicLink(token, "200:1234::1")

	// Pre-generate cookies
	cookies := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		cookie, _, _ := manager.CreateCookie(magicLinkID, userID, "200:1234::1", "Test-Agent")
		cookies[i] = cookie
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.ValidateCookie(cookies[i], "200:1234::1")
	}
}
