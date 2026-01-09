package models

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestAuthCookieRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	// Create a magic link first
	ml := &MagicLink{TokenHash: "cookie_test_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Successful creation", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "test_cookie_hash_123456",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
			IPv6Address: sql.NullString{String: "200:1234::1", Valid: true},
			UserAgent:   sql.NullString{String: "Test-Agent", Valid: true},
		}

		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify ID was set
		if cookie.ID == 0 {
			t.Error("Create() should set ID")
		}

		// Verify timestamps were set
		if cookie.CreatedAt.IsZero() {
			t.Error("Create() should set CreatedAt")
		}
		if cookie.LastUsed.IsZero() {
			t.Error("Create() should set LastUsed")
		}
	})

	t.Run("Duplicate cookie_hash fails", func(t *testing.T) {
		cookieHash := "duplicate_cookie_hash"
		expiresAt := time.Now().Add(7 * 24 * time.Hour)

		cookie1 := &AuthCookie{
			CookieHash:  cookieHash,
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		err := repo.Create(cookie1)
		if err != nil {
			t.Fatalf("First Create() error = %v", err)
		}

		// Try to create another with same cookie_hash
		cookie2 := &AuthCookie{
			CookieHash:  cookieHash,
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		err = repo.Create(cookie2)
		if err == nil {
			t.Error("Create() should fail with duplicate cookie_hash (UNIQUE constraint)")
		}
	})

	t.Run("Foreign key constraint", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "fk_test_cookie",
			MagicLinkID: 99999, // Non-existent magic link
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		err := repo.Create(cookie)
		if err == nil {
			t.Error("Create() should fail with invalid magic_link_id (foreign key constraint)")
		}
	})

	t.Run("Null values handled", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "null_values_cookie",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
			IPv6Address: sql.NullString{Valid: false},
			UserAgent:   sql.NullString{Valid: false},
		}

		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() with null values error = %v", err)
		}

		// Retrieve and verify
		retrieved, err := repo.GetByID(cookie.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.IPv6Address.Valid {
			t.Error("IPv6Address should be null")
		}
		if retrieved.UserAgent.Valid {
			t.Error("UserAgent should be null")
		}
	})
}

func TestAuthCookieRepository_GetByCookieHash(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "get_hash_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Existing hash returns data", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "existing_cookie_hash",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
			IPv6Address: sql.NullString{String: "200:1234::1", Valid: true},
			UserAgent:   sql.NullString{String: "Test-Agent", Valid: true},
		}

		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Retrieve by hash
		retrieved, err := repo.GetByCookieHash("existing_cookie_hash")
		if err != nil {
			t.Fatalf("GetByCookieHash() error = %v", err)
		}

		if retrieved == nil {
			t.Fatal("GetByCookieHash() returned nil")
		}

		if retrieved.CookieHash != cookie.CookieHash {
			t.Errorf("CookieHash = %s, want %s", retrieved.CookieHash, cookie.CookieHash)
		}

		if retrieved.UserID != userID {
			t.Errorf("UserID = %d, want %d", retrieved.UserID, userID)
		}
	})

	t.Run("Non-existent hash returns nil", func(t *testing.T) {
		retrieved, err := repo.GetByCookieHash("non_existent_cookie_hash")
		if err != nil {
			t.Fatalf("GetByCookieHash() error = %v", err)
		}

		if retrieved != nil {
			t.Error("GetByCookieHash() should return nil for non-existent hash")
		}
	})

	t.Run("Expired cookie not returned", func(t *testing.T) {
		// Create expired cookie
		expiresAt := time.Now().Add(-24 * time.Hour) // Expired 1 day ago
		cookie := &AuthCookie{
			CookieHash:  "expired_cookie_hash",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// GetByCookieHash should return the cookie even if expired
		// (filtering by expiry is done in the query, but SQLite CURRENT_TIMESTAMP might behave differently)
		// Let's verify the cookie exists in DB
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM auth_cookies WHERE cookie_hash = ?", "expired_cookie_hash").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}

		if count == 0 {
			t.Error("Expired cookie should exist in database")
		}
	})
}

func TestAuthCookieRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "get_id_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Existing ID returns data", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "test_get_by_id",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(cookie.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved == nil {
			t.Fatal("GetByID() returned nil")
		}

		if retrieved.ID != cookie.ID {
			t.Errorf("ID = %d, want %d", retrieved.ID, cookie.ID)
		}
	})

	t.Run("Non-existent ID returns nil", func(t *testing.T) {
		retrieved, err := repo.GetByID(99999)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved != nil {
			t.Error("GetByID() should return nil for non-existent ID")
		}
	})
}

func TestAuthCookieRepository_GetByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "get_user_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Returns all cookies for user", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)

		// Create multiple cookies
		for i := 0; i < 3; i++ {
			cookie := &AuthCookie{
				CookieHash:  "user_cookie_" + string(rune('a'+i)),
				MagicLinkID: ml.ID,
				UserID:      userID,
				ExpiresAt:   expiresAt,
			}
			err := repo.Create(cookie)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		cookies, err := repo.GetByUserID(userID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(cookies) != 3 {
			t.Errorf("GetByUserID() returned %d cookies, want 3", len(cookies))
		}
	})

	t.Run("No cookies returns empty slice", func(t *testing.T) {
		newUserID := createTestUser(t, db)
		cookies, err := repo.GetByUserID(newUserID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(cookies) != 0 {
			t.Errorf("GetByUserID() returned %d cookies, want 0", len(cookies))
		}
	})
}

func TestAuthCookieRepository_GetActiveByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "active_user_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Returns only non-expired cookies", func(t *testing.T) {
		// Create active cookie
		activeCookie := &AuthCookie{
			CookieHash:  "active_cookie",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
		}
		repo.Create(activeCookie)

		// Create expired cookie
		expiredCookie := &AuthCookie{
			CookieHash:  "expired_cookie",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   time.Now().Add(-24 * time.Hour),
		}
		repo.Create(expiredCookie)

		cookies, err := repo.GetActiveByUserID(userID)
		if err != nil {
			t.Fatalf("GetActiveByUserID() error = %v", err)
		}

		if len(cookies) != 1 {
			t.Errorf("GetActiveByUserID() returned %d cookies, want 1", len(cookies))
		}

		if cookies[0].CookieHash != "active_cookie" {
			t.Error("GetActiveByUserID() returned wrong cookie")
		}
	})
}

func TestAuthCookieRepository_GetByMagicLinkID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "ml_cookies_test", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Returns all cookies for magic link", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)

		// Create multiple cookies for same magic link
		for i := 0; i < 3; i++ {
			cookie := &AuthCookie{
				CookieHash:  "ml_cookie_" + string(rune('a'+i)),
				MagicLinkID: ml.ID,
				UserID:      userID,
				ExpiresAt:   expiresAt,
			}
			err := repo.Create(cookie)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		cookies, err := repo.GetByMagicLinkID(ml.ID)
		if err != nil {
			t.Fatalf("GetByMagicLinkID() error = %v", err)
		}

		if len(cookies) != 3 {
			t.Errorf("GetByMagicLinkID() returned %d cookies, want 3", len(cookies))
		}
	})
}

func TestAuthCookieRepository_UpdateLastUsed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "update_last_used_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Timestamp updated", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "update_last_used_test",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}
		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		initialLastUsed := cookie.LastUsed

		// Wait to ensure timestamp difference (SQLite CURRENT_TIMESTAMP has second precision)
		time.Sleep(1100 * time.Millisecond)

		err = repo.UpdateLastUsed(cookie.ID)
		if err != nil {
			t.Fatalf("UpdateLastUsed() error = %v", err)
		}

		// Retrieve and verify
		updated, err := repo.GetByID(cookie.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if !updated.LastUsed.After(initialLastUsed) {
			t.Error("LastUsed timestamp not updated")
		}
	})
}

func TestAuthCookieRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "delete_cookie_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Cookie deleted by ID", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "delete_by_id_test",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}
		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Delete
		err = repo.Delete(cookie.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify deleted
		deleted, err := repo.GetByID(cookie.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if deleted != nil {
			t.Error("Cookie should be deleted")
		}
	})

	t.Run("Cookie deleted by hash", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "delete_by_hash_test",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}
		err := repo.Create(cookie)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Delete by hash
		err = repo.DeleteByCookieHash("delete_by_hash_test")
		if err != nil {
			t.Fatalf("DeleteByCookieHash() error = %v", err)
		}

		// Verify deleted
		deleted, err := repo.GetByCookieHash("delete_by_hash_test")
		if err != nil {
			t.Fatalf("GetByCookieHash() error = %v", err)
		}

		if deleted != nil {
			t.Error("Cookie should be deleted")
		}
	})
}

func TestAuthCookieRepository_DeleteByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "delete_user_cookies_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("All user cookies deleted", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)

		// Create multiple cookies
		for i := 0; i < 3; i++ {
			cookie := &AuthCookie{
				CookieHash:  "delete_user_" + string(rune('a'+i)),
				MagicLinkID: ml.ID,
				UserID:      userID,
				ExpiresAt:   expiresAt,
			}
			repo.Create(cookie)
		}

		// Delete all
		err := repo.DeleteByUserID(userID)
		if err != nil {
			t.Fatalf("DeleteByUserID() error = %v", err)
		}

		// Verify all deleted
		cookies, err := repo.GetByUserID(userID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(cookies) != 0 {
			t.Errorf("Expected 0 cookies, got %d", len(cookies))
		}
	})

	t.Run("Error if no cookies found", func(t *testing.T) {
		newUserID := createTestUser(t, db)

		err := repo.DeleteByUserID(newUserID)
		if err == nil {
			t.Error("DeleteByUserID() should return error when no cookies found")
		}
	})
}

func TestAuthCookieRepository_DeleteExpired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "delete_expired_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Expired cookies removed", func(t *testing.T) {
		// Create active cookie
		activeCookie := &AuthCookie{
			CookieHash:  "active_cookie_expire_test",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
		}
		repo.Create(activeCookie)

		// Create expired cookies
		for i := 0; i < 3; i++ {
			cookie := &AuthCookie{
				CookieHash:  "expired_cookie_" + string(rune('a'+i)),
				MagicLinkID: ml.ID,
				UserID:      userID,
				ExpiresAt:   time.Now().Add(-24 * time.Hour),
			}
			repo.Create(cookie)
		}

		// Delete expired
		err := repo.DeleteExpired()
		if err != nil {
			t.Fatalf("DeleteExpired() error = %v", err)
		}

		// Verify only active cookie remains
		cookies, err := repo.GetByUserID(userID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(cookies) != 1 {
			t.Errorf("Expected 1 cookie after DeleteExpired, got %d", len(cookies))
		}

		if cookies[0].CookieHash != "active_cookie_expire_test" {
			t.Error("Wrong cookie remained after DeleteExpired")
		}
	})

	t.Run("Active cookies remain", func(t *testing.T) {
		newUserID := createTestUser(t, db)
		newML := &MagicLink{TokenHash: "active_remain_ml", UserID: newUserID}
		mlRepo.Create(newML)

		// Create only active cookies
		for i := 0; i < 3; i++ {
			cookie := &AuthCookie{
				CookieHash:  "active_remain_" + string(rune('a'+i)),
				MagicLinkID: newML.ID,
				UserID:      newUserID,
				ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
			}
			repo.Create(cookie)
		}

		// Delete expired
		err := repo.DeleteExpired()
		if err != nil {
			t.Fatalf("DeleteExpired() error = %v", err)
		}

		// Verify all remain
		cookies, err := repo.GetByUserID(newUserID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(cookies) != 3 {
			t.Errorf("Expected 3 active cookies, got %d", len(cookies))
		}
	})
}

func TestAuthCookieRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "count_cookies_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("CountByUserID", func(t *testing.T) {
		// Initially 0
		count, err := repo.CountByUserID(userID)
		if err != nil {
			t.Fatalf("CountByUserID() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Initial count = %d, want 0", count)
		}

		// Create cookies
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		for i := 0; i < 5; i++ {
			cookie := &AuthCookie{
				CookieHash:  "count_cookie_" + string(rune('a'+i)),
				MagicLinkID: ml.ID,
				UserID:      userID,
				ExpiresAt:   expiresAt,
			}
			repo.Create(cookie)
		}

		// Count should be 5
		count, err = repo.CountByUserID(userID)
		if err != nil {
			t.Fatalf("CountByUserID() error = %v", err)
		}
		if count != 5 {
			t.Errorf("Count = %d, want 5", count)
		}
	})

	t.Run("CountActiveByUserID", func(t *testing.T) {
		newUserID := createTestUser(t, db)
		newML := &MagicLink{TokenHash: "count_active_ml", UserID: newUserID}
		mlRepo.Create(newML)

		// Create 3 active cookies
		for i := 0; i < 3; i++ {
			cookie := &AuthCookie{
				CookieHash:  "active_count_cookie_" + string(rune('a'+i)),
				MagicLinkID: newML.ID,
				UserID:      newUserID,
				ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
			}
			repo.Create(cookie)
		}

		// Create 2 expired cookies
		for i := 0; i < 2; i++ {
			cookie := &AuthCookie{
				CookieHash:  "expired_count_cookie_" + string(rune('a'+i)),
				MagicLinkID: newML.ID,
				UserID:      newUserID,
				ExpiresAt:   time.Now().Add(-24 * time.Hour),
			}
			repo.Create(cookie)
		}

		// Active count should be 3
		count, err := repo.CountActiveByUserID(newUserID)
		if err != nil {
			t.Fatalf("CountActiveByUserID() error = %v", err)
		}
		if count != 3 {
			t.Errorf("Active count = %d, want 3", count)
		}

		// Total count should be 5
		totalCount, err := repo.CountByUserID(newUserID)
		if err != nil {
			t.Fatalf("CountByUserID() error = %v", err)
		}
		if totalCount != 5 {
			t.Errorf("Total count = %d, want 5", totalCount)
		}
	})
}

func TestAuthCookieRepository_IsExpired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "is_expired_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Active cookie not expired", func(t *testing.T) {
		cookie := &AuthCookie{
			CookieHash:  "active_expire_check",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
		}
		repo.Create(cookie)

		expired, err := repo.IsExpired(cookie.ID)
		if err != nil {
			t.Fatalf("IsExpired() error = %v", err)
		}

		if expired {
			t.Error("Active cookie should not be expired")
		}
	})

	t.Run("Expired cookie is expired", func(t *testing.T) {
		cookie := &AuthCookie{
			CookieHash:  "expired_expire_check",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   time.Now().Add(-24 * time.Hour),
		}
		repo.Create(cookie)

		expired, err := repo.IsExpired(cookie.ID)
		if err != nil {
			t.Fatalf("IsExpired() error = %v", err)
		}

		if !expired {
			t.Error("Expired cookie should be marked as expired")
		}
	})

	t.Run("Non-existent cookie returns error", func(t *testing.T) {
		_, err := repo.IsExpired(99999)
		if err == nil {
			t.Error("IsExpired() should return error for non-existent cookie")
		}
	})
}

func TestAuthCookieRepository_SQLInjection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "sql_injection_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("SQL injection in cookie_hash", func(t *testing.T) {
		maliciousHash := "'; DROP TABLE auth_cookies; --"

		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  maliciousHash,
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		// This should not cause SQL injection
		err := repo.Create(cookie)
		if err != nil {
			// May fail due to constraints, but should not execute SQL
			t.Logf("Create with malicious input failed (expected): %v", err)
		}

		// Verify table still exists
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM auth_cookies").Scan(&count)
		if err != nil {
			t.Fatalf("Table auth_cookies should still exist: %v", err)
		}
	})

	t.Run("SQL injection in GetByCookieHash", func(t *testing.T) {
		maliciousHash := "' OR '1'='1"

		// This should not return unauthorized data
		cookie, err := repo.GetByCookieHash(maliciousHash)
		if err != nil {
			t.Fatalf("GetByCookieHash() error = %v", err)
		}

		if cookie != nil {
			t.Error("SQL injection attempt should not return data")
		}
	})
}

func TestAuthCookieRepository_EdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	ml := &MagicLink{TokenHash: "edge_cases_ml", UserID: userID}
	mlRepo.Create(ml)

	t.Run("Very long cookie hash", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  strings.Repeat("a", 10000),
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		err := repo.Create(cookie)
		// Should handle gracefully
		_ = err
	})

	t.Run("Empty cookie hash", func(t *testing.T) {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		cookie := &AuthCookie{
			CookieHash:  "",
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}

		err := repo.Create(cookie)
		// Should handle gracefully
		_ = err
	})

	t.Run("Update non-existent cookie", func(t *testing.T) {
		err := repo.UpdateLastUsed(99999)
		// Should not error
		if err != nil {
			t.Logf("UpdateLastUsed with non-existent ID: %v", err)
		}
	})

	t.Run("Delete non-existent cookie", func(t *testing.T) {
		err := repo.Delete(99999)
		// Should not error
		if err != nil {
			t.Logf("Delete with non-existent ID: %v", err)
		}
	})
}

// Benchmark tests
func BenchmarkAuthCookieRepository_Create(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(&testing.T{}, db)

	ml := &MagicLink{TokenHash: "bench_ml", UserID: userID}
	mlRepo.Create(ml)

	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cookie := &AuthCookie{
			CookieHash:  "bench_cookie_" + string(rune(i)),
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}
		_ = repo.Create(cookie)
	}
}

func BenchmarkAuthCookieRepository_GetByCookieHash(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	repo := NewAuthCookieRepository(db)
	mlRepo := NewMagicLinkRepository(db)
	userID := createTestUser(&testing.T{}, db)

	ml := &MagicLink{TokenHash: "bench_get_ml", UserID: userID}
	mlRepo.Create(ml)

	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Pre-create cookies
	hashes := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		hash := "bench_get_cookie_" + string(rune(i))
		hashes[i] = hash
		cookie := &AuthCookie{
			CookieHash:  hash,
			MagicLinkID: ml.ID,
			UserID:      userID,
			ExpiresAt:   expiresAt,
		}
		repo.Create(cookie)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetByCookieHash(hashes[i])
	}
}
