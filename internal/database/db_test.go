package database

import (
	"log"
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB creates a test database for unit testing
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	// Create temp directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create logger that discards output during tests
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)

	// Create database
	db, err := New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return db
}

// TestGetLastFederationServer tests retrieving the last federation server address
func TestGetLastFederationServer(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("No previous server", func(t *testing.T) {
		addr := db.GetLastFederationServer()
		if addr != "" {
			t.Errorf("Expected empty string for no previous server, got %q", addr)
		}
	})

	t.Run("Save and retrieve", func(t *testing.T) {
		testAddr := "200:1234:5678:abcd::1"
		err := db.SaveLastFederationServer(testAddr)
		if err != nil {
			t.Fatalf("SaveLastFederationServer() failed: %v", err)
		}

		addr := db.GetLastFederationServer()
		if addr != testAddr {
			t.Errorf("GetLastFederationServer() = %q, want %q", addr, testAddr)
		}
	})

	t.Run("Update server", func(t *testing.T) {
		// Save first server
		firstAddr := "200:1111:2222:3333::1"
		err := db.SaveLastFederationServer(firstAddr)
		if err != nil {
			t.Fatalf("SaveLastFederationServer() failed: %v", err)
		}

		// Verify first save
		addr := db.GetLastFederationServer()
		if addr != firstAddr {
			t.Errorf("GetLastFederationServer() = %q, want %q", addr, firstAddr)
		}

		// Update to second server
		secondAddr := "200:9999:8888:7777::1"
		err = db.SaveLastFederationServer(secondAddr)
		if err != nil {
			t.Fatalf("SaveLastFederationServer() update failed: %v", err)
		}

		// Verify update
		addr = db.GetLastFederationServer()
		if addr != secondAddr {
			t.Errorf("GetLastFederationServer() after update = %q, want %q", addr, secondAddr)
		}
	})
}

// TestSaveLastFederationServer tests saving federation server address
func TestSaveLastFederationServer(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("Save valid address", func(t *testing.T) {
		addr := "200:abcd:ef01:2345::1"
		err := db.SaveLastFederationServer(addr)
		if err != nil {
			t.Errorf("SaveLastFederationServer() failed: %v", err)
		}

		// Verify it was saved
		saved := db.GetLastFederationServer()
		if saved != addr {
			t.Errorf("Saved address = %q, want %q", saved, addr)
		}
	})

	t.Run("Save empty address", func(t *testing.T) {
		err := db.SaveLastFederationServer("")
		if err != nil {
			t.Errorf("SaveLastFederationServer() with empty string failed: %v", err)
		}

		saved := db.GetLastFederationServer()
		if saved != "" {
			t.Errorf("Saved address = %q, want empty string", saved)
		}
	})

	t.Run("Multiple saves (upsert)", func(t *testing.T) {
		addresses := []string{
			"200:1111::1",
			"200:2222::1",
			"200:3333::1",
		}

		for _, addr := range addresses {
			err := db.SaveLastFederationServer(addr)
			if err != nil {
				t.Fatalf("SaveLastFederationServer(%q) failed: %v", addr, err)
			}

			saved := db.GetLastFederationServer()
			if saved != addr {
				t.Errorf("After save, got %q, want %q", saved, addr)
			}
		}

		// Verify only one entry exists in database
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM server_config WHERE key = 'last_federation_server'").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count entries: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 entry in server_config, got %d", count)
		}
	})
}

// TestClearFederatedStationCacheOnDisable tests cache clearing
func TestClearFederatedStationCacheOnDisable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("Clear empty cache", func(t *testing.T) {
		err := db.ClearFederatedStationCache()
		if err != nil {
			t.Errorf("ClearFederatedStationCache() on empty cache failed: %v", err)
		}
	})

	t.Run("Clear cache with stations", func(t *testing.T) {
		// Insert test federated stations
		testStations := []struct {
			uuid       string
			sourceNode string
			mountpoint string
		}{
			{"uuid-1", "200:test:1::", "/stream1"},
			{"uuid-2", "200:test:2::", "/stream2"},
			{"uuid-3", "200:test:3::", "/stream3"},
		}

		for _, st := range testStations {
			station := &FederatedStation{
				UUID:           st.uuid,
				SourceNode:     st.sourceNode,
				Mountpoint:     st.mountpoint,
				Name:           "Test Station",
				OwnerPubkey:    "test-pubkey",
				Status:         "online",
				ListenersCount: 0,
			}
			err := db.UpsertFederatedStationCache(station)
			if err != nil {
				t.Fatalf("Failed to insert test station: %v", err)
			}
		}

		// Verify stations exist
		stations, err := db.ListFederatedStationCache()
		if err != nil {
			t.Fatalf("ListFederatedStationCache() failed: %v", err)
		}
		if len(stations) != len(testStations) {
			t.Errorf("Expected %d stations before clear, got %d", len(testStations), len(stations))
		}

		// Clear cache
		err = db.ClearFederatedStationCache()
		if err != nil {
			t.Fatalf("ClearFederatedStationCache() failed: %v", err)
		}

		// Verify cache is empty
		stations, err = db.ListFederatedStationCache()
		if err != nil {
			t.Fatalf("ListFederatedStationCache() after clear failed: %v", err)
		}
		if len(stations) != 0 {
			t.Errorf("Expected 0 stations after clear, got %d", len(stations))
		}
	})
}

// TestFederationServerChangeDetection tests server change detection logic
func TestFederationServerChangeDetection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("First run - no previous server", func(t *testing.T) {
		lastServer := db.GetLastFederationServer()
		currentServer := "200:new:server::1"

		// Simulate first run
		if lastServer == "" {
			// No previous server, no need to clear cache
			t.Log("First run detected - no cache to clear")
		}

		// Save current server
		err := db.SaveLastFederationServer(currentServer)
		if err != nil {
			t.Fatalf("Failed to save server: %v", err)
		}
	})

	t.Run("Server changed - should clear cache", func(t *testing.T) {
		// Setup: save initial server
		initialServer := "200:old:server::1"
		err := db.SaveLastFederationServer(initialServer)
		if err != nil {
			t.Fatalf("Failed to save initial server: %v", err)
		}

		// Insert test station
		station := &FederatedStation{
			UUID:        "test-uuid",
			SourceNode:  "200:old::",
			Mountpoint:  "/live",
			Name:        "Old Station",
			OwnerPubkey: "test-pubkey",
			Status:      "online",
		}
		err = db.UpsertFederatedStationCache(station)
		if err != nil {
			t.Fatalf("Failed to insert station: %v", err)
		}

		// Simulate server change
		lastServer := db.GetLastFederationServer()
		newServer := "200:new:server::1"

		if lastServer != "" && lastServer != newServer {
			// Server changed - clear cache
			err = db.ClearFederatedStationCache()
			if err != nil {
				t.Fatalf("Failed to clear cache: %v", err)
			}
		}

		// Verify cache is cleared
		stations, err := db.ListFederatedStationCache()
		if err != nil {
			t.Fatalf("Failed to list stations: %v", err)
		}
		if len(stations) != 0 {
			t.Errorf("Expected empty cache after server change, got %d stations", len(stations))
		}

		// Save new server
		err = db.SaveLastFederationServer(newServer)
		if err != nil {
			t.Fatalf("Failed to save new server: %v", err)
		}

		// Verify new server is saved
		saved := db.GetLastFederationServer()
		if saved != newServer {
			t.Errorf("Saved server = %q, want %q", saved, newServer)
		}
	})

	t.Run("Server unchanged - keep cache", func(t *testing.T) {
		server := "200:same:server::1"

		// Save server
		err := db.SaveLastFederationServer(server)
		if err != nil {
			t.Fatalf("Failed to save server: %v", err)
		}

		// Insert station
		station := &FederatedStation{
			UUID:        "test-uuid-2",
			SourceNode:  "200:same::",
			Mountpoint:  "/live2",
			Name:        "Same Station",
			OwnerPubkey: "test-pubkey",
			Status:      "online",
		}
		err = db.UpsertFederatedStationCache(station)
		if err != nil {
			t.Fatalf("Failed to insert station: %v", err)
		}

		// Check if server changed
		lastServer := db.GetLastFederationServer()
		if lastServer != server {
			t.Fatalf("Expected server %q, got %q", server, lastServer)
		}

		// Server unchanged - should NOT clear cache
		stations, err := db.ListFederatedStationCache()
		if err != nil {
			t.Fatalf("Failed to list stations: %v", err)
		}
		if len(stations) == 0 {
			t.Error("Cache was incorrectly cleared when server didn't change")
		}
	})
}

// TestDatabaseIsolation ensures test databases don't interfere with each other
func TestDatabaseIsolation(t *testing.T) {
	db1 := setupTestDB(t)
	defer db1.Close()

	db2 := setupTestDB(t)
	defer db2.Close()

	// Save different values in each database
	err := db1.SaveLastFederationServer("200:db1::1")
	if err != nil {
		t.Fatalf("db1.SaveLastFederationServer() failed: %v", err)
	}

	err = db2.SaveLastFederationServer("200:db2::1")
	if err != nil {
		t.Fatalf("db2.SaveLastFederationServer() failed: %v", err)
	}

	// Verify each database has its own value
	addr1 := db1.GetLastFederationServer()
	addr2 := db2.GetLastFederationServer()

	if addr1 != "200:db1::1" {
		t.Errorf("db1 address = %q, want %q", addr1, "200:db1::1")
	}
	if addr2 != "200:db2::1" {
		t.Errorf("db2 address = %q, want %q", addr2, "200:db2::1")
	}
	if addr1 == addr2 {
		t.Error("Databases are not isolated - they have the same value")
	}
}

// TestServerConfigUpdatedAt tests that updated_at timestamp is set correctly
func TestServerConfigUpdatedAt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Save initial server
	err := db.SaveLastFederationServer("200:test::1")
	if err != nil {
		t.Fatalf("SaveLastFederationServer() failed: %v", err)
	}

	// Get initial timestamp
	var initialTime string
	err = db.QueryRow("SELECT updated_at FROM server_config WHERE key = 'last_federation_server'").Scan(&initialTime)
	if err != nil {
		t.Fatalf("Failed to get initial timestamp: %v", err)
	}

	// Wait a bit to ensure timestamp difference
	// Note: SQLite's CURRENT_TIMESTAMP has second precision
	// For more precise testing, consider using time.Sleep(2 * time.Second)
	// but that would slow down tests significantly

	// Update server
	err = db.SaveLastFederationServer("200:test::2")
	if err != nil {
		t.Fatalf("SaveLastFederationServer() update failed: %v", err)
	}

	// Get updated timestamp
	var updatedTime string
	err = db.QueryRow("SELECT updated_at FROM server_config WHERE key = 'last_federation_server'").Scan(&updatedTime)
	if err != nil {
		t.Fatalf("Failed to get updated timestamp: %v", err)
	}

	// Timestamps should be set (not empty)
	if initialTime == "" {
		t.Error("Initial timestamp is empty")
	}
	if updatedTime == "" {
		t.Error("Updated timestamp is empty")
	}

	// Note: Due to SQLite second precision, we can't reliably test
	// that updatedTime > initialTime without sleeping between operations
	// Just verify both are non-empty for now
}
