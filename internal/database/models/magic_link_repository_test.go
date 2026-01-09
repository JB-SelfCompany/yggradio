package models

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory SQLite database with schema
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Create in-memory database
	db, err := sql.Open("sqlite", ":memory:")
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
	_, err = db.Exec(string(schemaBytes))
	if err != nil {
		t.Fatalf("Failed to execute schema: %v", err)
	}

	return db
}

// createTestUser creates a user for testing and returns the ID
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

func TestMagicLinkRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Successful creation", func(t *testing.T) {
		ml := &MagicLink{
			TokenHash:   "test_token_hash_123456",
			UserID:      userID,
			IPv6Created: sql.NullString{String: "200:1234::1", Valid: true},
			UserAgent:   sql.NullString{String: "Test-Agent", Valid: true},
		}

		err := repo.Create(ml)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify ID was set
		if ml.ID == 0 {
			t.Error("Create() should set ID")
		}

		// Verify timestamps were set
		if ml.CreatedAt.IsZero() {
			t.Error("Create() should set CreatedAt")
		}
		if ml.LastUsed.IsZero() {
			t.Error("Create() should set LastUsed")
		}

		// Verify is_active is true
		if !ml.IsActive {
			t.Error("Create() should set IsActive to true")
		}
	})

	t.Run("Duplicate token_hash fails", func(t *testing.T) {
		tokenHash := "duplicate_hash_test"

		ml1 := &MagicLink{
			TokenHash: tokenHash,
			UserID:    userID,
		}

		err := repo.Create(ml1)
		if err != nil {
			t.Fatalf("First Create() error = %v", err)
		}

		// Try to create another with same token_hash
		ml2 := &MagicLink{
			TokenHash: tokenHash,
			UserID:    userID,
		}

		err = repo.Create(ml2)
		if err == nil {
			t.Error("Create() should fail with duplicate token_hash")
		}
	})

	t.Run("Null values handled", func(t *testing.T) {
		ml := &MagicLink{
			TokenHash:   "test_null_values",
			UserID:      userID,
			IPv6Created: sql.NullString{Valid: false},
			UserAgent:   sql.NullString{Valid: false},
		}

		err := repo.Create(ml)
		if err != nil {
			t.Fatalf("Create() with null values error = %v", err)
		}

		// Retrieve and verify
		retrieved, err := repo.GetByID(ml.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.IPv6Created.Valid {
			t.Error("IPv6Created should be null")
		}
		if retrieved.UserAgent.Valid {
			t.Error("UserAgent should be null")
		}
	})
}

func TestMagicLinkRepository_GetByTokenHash(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Existing hash returns data", func(t *testing.T) {
		ml := &MagicLink{
			TokenHash:   "existing_hash_123",
			UserID:      userID,
			IPv6Created: sql.NullString{String: "200:1234::1", Valid: true},
			UserAgent:   sql.NullString{String: "Test-Agent", Valid: true},
		}

		err := repo.Create(ml)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Retrieve by hash
		retrieved, err := repo.GetByTokenHash("existing_hash_123")
		if err != nil {
			t.Fatalf("GetByTokenHash() error = %v", err)
		}

		if retrieved == nil {
			t.Fatal("GetByTokenHash() returned nil")
		}

		if retrieved.TokenHash != ml.TokenHash {
			t.Errorf("TokenHash = %s, want %s", retrieved.TokenHash, ml.TokenHash)
		}

		if retrieved.UserID != userID {
			t.Errorf("UserID = %d, want %d", retrieved.UserID, userID)
		}

		if retrieved.IPv6Created.String != "200:1234::1" {
			t.Errorf("IPv6Created = %s, want 200:1234::1", retrieved.IPv6Created.String)
		}
	})

	t.Run("Non-existent hash returns nil", func(t *testing.T) {
		retrieved, err := repo.GetByTokenHash("non_existent_hash")
		if err != nil {
			t.Fatalf("GetByTokenHash() error = %v", err)
		}

		if retrieved != nil {
			t.Error("GetByTokenHash() should return nil for non-existent hash")
		}
	})
}

func TestMagicLinkRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Existing ID returns data", func(t *testing.T) {
		ml := &MagicLink{
			TokenHash: "test_get_by_id",
			UserID:    userID,
		}

		err := repo.Create(ml)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(ml.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved == nil {
			t.Fatal("GetByID() returned nil")
		}

		if retrieved.ID != ml.ID {
			t.Errorf("ID = %d, want %d", retrieved.ID, ml.ID)
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

func TestMagicLinkRepository_GetByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Returns all magic links for user", func(t *testing.T) {
		// Create multiple magic links
		for i := 0; i < 3; i++ {
			ml := &MagicLink{
				TokenHash: "user_link_" + string(rune('a'+i)),
				UserID:    userID,
			}
			err := repo.Create(ml)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		links, err := repo.GetByUserID(userID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(links) != 3 {
			t.Errorf("GetByUserID() returned %d links, want 3", len(links))
		}
	})

	t.Run("No links returns empty slice", func(t *testing.T) {
		newUserID := createTestUser(t, db)
		links, err := repo.GetByUserID(newUserID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(links) != 0 {
			t.Errorf("GetByUserID() returned %d links, want 0", len(links))
		}
	})

	t.Run("Links ordered by created_at DESC", func(t *testing.T) {
		newUserID := createTestUser(t, db)

		// Create links with slight delay
		ml1 := &MagicLink{TokenHash: "order_test_1", UserID: newUserID}
		repo.Create(ml1)

		time.Sleep(10 * time.Millisecond)

		ml2 := &MagicLink{TokenHash: "order_test_2", UserID: newUserID}
		repo.Create(ml2)

		links, err := repo.GetByUserID(newUserID)
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}

		if len(links) < 2 {
			t.Fatal("Not enough links created")
		}

		// First link should be the most recent
		if !links[0].CreatedAt.After(links[1].CreatedAt) && !links[0].CreatedAt.Equal(links[1].CreatedAt) {
			t.Error("Links not ordered by created_at DESC")
		}
	})
}

func TestMagicLinkRepository_GetActiveByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Returns only active links", func(t *testing.T) {
		// Create active link
		active := &MagicLink{TokenHash: "active_link", UserID: userID}
		repo.Create(active)

		// Create inactive link
		inactive := &MagicLink{TokenHash: "inactive_link", UserID: userID}
		repo.Create(inactive)
		repo.Deactivate(inactive.ID)

		links, err := repo.GetActiveByUserID(userID)
		if err != nil {
			t.Fatalf("GetActiveByUserID() error = %v", err)
		}

		if len(links) != 1 {
			t.Errorf("GetActiveByUserID() returned %d links, want 1", len(links))
		}

		if links[0].TokenHash != "active_link" {
			t.Error("GetActiveByUserID() returned wrong link")
		}
	})
}

func TestMagicLinkRepository_UpdateLastUsed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Timestamp updated", func(t *testing.T) {
		ml := &MagicLink{TokenHash: "update_test", UserID: userID}
		err := repo.Create(ml)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		initialLastUsed := ml.LastUsed

		// Wait to ensure timestamp difference (SQLite CURRENT_TIMESTAMP has second precision)
		time.Sleep(1100 * time.Millisecond)

		err = repo.UpdateLastUsed(ml.ID)
		if err != nil {
			t.Fatalf("UpdateLastUsed() error = %v", err)
		}

		// Retrieve and verify
		updated, err := repo.GetByID(ml.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if !updated.LastUsed.After(initialLastUsed) {
			t.Error("LastUsed timestamp not updated")
		}
	})
}

func TestMagicLinkRepository_Deactivate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Link deactivated", func(t *testing.T) {
		ml := &MagicLink{TokenHash: "deactivate_test", UserID: userID}
		err := repo.Create(ml)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify initially active
		if !ml.IsActive {
			t.Fatal("Magic link should be active initially")
		}

		// Deactivate
		err = repo.Deactivate(ml.ID)
		if err != nil {
			t.Fatalf("Deactivate() error = %v", err)
		}

		// Verify deactivated
		deactivated, err := repo.GetByID(ml.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if deactivated.IsActive {
			t.Error("Magic link should be inactive after deactivation")
		}
	})
}

func TestMagicLinkRepository_DeactivateByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("All user links deactivated", func(t *testing.T) {
		// Create multiple links
		for i := 0; i < 3; i++ {
			ml := &MagicLink{
				TokenHash: "deactivate_user_" + string(rune('a'+i)),
				UserID:    userID,
			}
			repo.Create(ml)
		}

		// Deactivate all
		err := repo.DeactivateByUserID(userID)
		if err != nil {
			t.Fatalf("DeactivateByUserID() error = %v", err)
		}

		// Verify all deactivated
		links, err := repo.GetActiveByUserID(userID)
		if err != nil {
			t.Fatalf("GetActiveByUserID() error = %v", err)
		}

		if len(links) != 0 {
			t.Errorf("Expected 0 active links, got %d", len(links))
		}
	})

	t.Run("Error if no links found", func(t *testing.T) {
		newUserID := createTestUser(t, db)

		err := repo.DeactivateByUserID(newUserID)
		if err == nil {
			t.Error("DeactivateByUserID() should return error when no links found")
		}
	})
}

func TestMagicLinkRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Link deleted", func(t *testing.T) {
		ml := &MagicLink{TokenHash: "delete_test", UserID: userID}
		err := repo.Create(ml)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Delete
		err = repo.Delete(ml.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify deleted
		deleted, err := repo.GetByID(ml.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if deleted != nil {
			t.Error("Magic link should be deleted")
		}
	})
}

func TestMagicLinkRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("CountByUserID", func(t *testing.T) {
		// Initially 0
		count, err := repo.CountByUserID(userID)
		if err != nil {
			t.Fatalf("CountByUserID() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Initial count = %d, want 0", count)
		}

		// Create links
		for i := 0; i < 5; i++ {
			ml := &MagicLink{
				TokenHash: "count_test_" + string(rune('a'+i)),
				UserID:    userID,
			}
			repo.Create(ml)
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

		// Create 3 active links
		for i := 0; i < 3; i++ {
			ml := &MagicLink{
				TokenHash: "active_count_" + string(rune('a'+i)),
				UserID:    newUserID,
			}
			repo.Create(ml)
		}

		// Create 2 inactive links
		for i := 0; i < 2; i++ {
			ml := &MagicLink{
				TokenHash: "inactive_count_" + string(rune('a'+i)),
				UserID:    newUserID,
			}
			repo.Create(ml)
			repo.Deactivate(ml.ID)
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

func TestMagicLinkRepository_SQLInjection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("SQL injection in token_hash", func(t *testing.T) {
		maliciousHash := "'; DROP TABLE magic_links; --"

		ml := &MagicLink{
			TokenHash: maliciousHash,
			UserID:    userID,
		}

		// This should not cause SQL injection
		err := repo.Create(ml)
		if err != nil {
			// May fail due to length or other constraints, but should not execute SQL
			t.Logf("Create with malicious input failed (expected): %v", err)
		}

		// Verify table still exists
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM magic_links").Scan(&count)
		if err != nil {
			t.Fatalf("Table magic_links should still exist: %v", err)
		}
	})

	t.Run("SQL injection in GetByTokenHash", func(t *testing.T) {
		maliciousHash := "' OR '1'='1"

		// This should not return unauthorized data
		link, err := repo.GetByTokenHash(maliciousHash)
		if err != nil {
			t.Fatalf("GetByTokenHash() error = %v", err)
		}

		if link != nil {
			t.Error("SQL injection attempt should not return data")
		}
	})
}

func TestMagicLinkRepository_EdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(t, db)

	t.Run("Empty token hash", func(t *testing.T) {
		ml := &MagicLink{
			TokenHash: "",
			UserID:    userID,
		}

		err := repo.Create(ml)
		// Should handle gracefully (may succeed or fail based on constraints)
		_ = err
	})

	t.Run("Very long token hash", func(t *testing.T) {
		ml := &MagicLink{
			TokenHash: strings.Repeat("a", 10000),
			UserID:    userID,
		}

		err := repo.Create(ml)
		// Should handle gracefully
		_ = err
	})

	t.Run("Update non-existent link", func(t *testing.T) {
		err := repo.UpdateLastUsed(99999)
		// Should not error (UPDATE with 0 rows affected is valid)
		if err != nil {
			t.Logf("UpdateLastUsed with non-existent ID: %v", err)
		}
	})

	t.Run("Delete non-existent link", func(t *testing.T) {
		err := repo.Delete(99999)
		// Should not error (DELETE with 0 rows affected is valid)
		if err != nil {
			t.Logf("Delete with non-existent ID: %v", err)
		}
	})
}

// Benchmark tests
func BenchmarkMagicLinkRepository_Create(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(&testing.T{}, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ml := &MagicLink{
			TokenHash: "bench_token_" + string(rune(i)),
			UserID:    userID,
		}
		_ = repo.Create(ml)
	}
}

func BenchmarkMagicLinkRepository_GetByTokenHash(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer db.Close()

	repo := NewMagicLinkRepository(db)
	userID := createTestUser(&testing.T{}, db)

	// Pre-create magic links
	hashes := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		hash := "bench_get_" + string(rune(i))
		hashes[i] = hash
		ml := &MagicLink{
			TokenHash: hash,
			UserID:    userID,
		}
		repo.Create(ml)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetByTokenHash(hashes[i])
	}
}
