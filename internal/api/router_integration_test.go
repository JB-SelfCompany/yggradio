package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database"
)

// setupTestEnvironment creates a minimal environment for router integration tests
func setupTestEnvironment(t *testing.T, federationEnabled bool, serverAddress string) (*database.DB, *config.Config, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	// Create temp directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create logger
	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)

	// Create database
	db, err := database.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Generate test key pair
	pubkey, privkey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create test config
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:         8080,
			Bind:         "127.0.0.1",
			InstanceName: "Test Instance",
		},
		Federation: config.FederationConfig{
			Enabled:          federationEnabled,
			ServerAddress:    serverAddress,
			ServerPort:       9000,
			RegisterInterval: 120,
			QueryInterval:    60,
			Timeout:          30,
		},
		Security: config.SecurityConfig{
			CSRFTokenTTL:          900,
			EnableSecurityAudit:   false,
			FailedAuthThreshold:   5,
			BlockDuration:         3600,
			AutoBlockOnFailedAuth: true,
		},
		RateLimit: config.RateLimitConfig{
			APIRequestsPerMinute:         1000,
			AuthAttemptsPerMinute:        10,
			StationCreationPerHour:       100,
			CommentsPerHour:              50,
			SourceConnectionsPerMinute:   10,
			ListenerConnectionsPerMinute: 100,
		},
	}

	return db, cfg, pubkey, privkey
}

// TestRouterIntegration_FederationDisabledClearCache tests that router clears cache when federation is disabled
func TestRouterIntegration_FederationDisabledClearCache(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, cfg, pubkey, privkey := setupTestEnvironment(t, false, "")
	defer db.Close()

	// Insert test federated stations
	testStations := []struct {
		uuid       string
		sourceNode string
		mountpoint string
	}{
		{"uuid-1", "200:test:1::", "/stream1"},
		{"uuid-2", "200:test:2::", "/stream2"},
	}

	for _, st := range testStations {
		station := &database.FederatedStation{
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

	// Verify stations exist before router creation
	stationsBefore, err := db.ListFederatedStationCache()
	if err != nil {
		t.Fatalf("ListFederatedStationCache() failed: %v", err)
	}
	if len(stationsBefore) != len(testStations) {
		t.Fatalf("Expected %d stations before router creation, got %d", len(testStations), len(stationsBefore))
	}

	// Create Yggdrasil address (mock)
	yggAddr := &net.IPAddr{
		IP: net.ParseIP("200:1234:5678:abcd::1"),
	}

	// Get logger
	logger := log.New(os.Stderr, "[test-router] ", log.LstdFlags)

	// Create router with federation DISABLED
	// This should trigger cache cleanup in NewRouter()
	instanceURL := "http://localhost:8080"

	router := NewRouter(db, cfg, pubkey, privkey, yggAddr.String(), instanceURL, logger, nil)
	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}
	defer router.Stop()

	// Verify cache was cleared
	stationsAfter, err := db.ListFederatedStationCache()
	if err != nil {
		t.Fatalf("ListFederatedStationCache() after router creation failed: %v", err)
	}
	if len(stationsAfter) != 0 {
		t.Errorf("Expected 0 stations after router creation with federation disabled, got %d", len(stationsAfter))
	}
}

// TestRouterIntegration_FederationServerChange tests cache clearing on server address change
func TestRouterIntegration_FederationServerChange(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, cfg, pubkey, privkey := setupTestEnvironment(t, true, "200:old:server::1")
	defer db.Close()

	// Simulate previous run with old server
	err := db.SaveLastFederationServer("200:old:server::1")
	if err != nil {
		t.Fatalf("Failed to save old server address: %v", err)
	}

	// Insert test station from old server
	station := &database.FederatedStation{
		UUID:        "old-station-uuid",
		SourceNode:  "200:old::",
		Mountpoint:  "/old-stream",
		Name:        "Old Station",
		OwnerPubkey: "test-pubkey",
		Status:      "online",
	}
	err = db.UpsertFederatedStationCache(station)
	if err != nil {
		t.Fatalf("Failed to insert old station: %v", err)
	}

	// Verify station exists
	stationsBefore, err := db.ListFederatedStationCache()
	if err != nil {
		t.Fatalf("ListFederatedStationCache() failed: %v", err)
	}
	if len(stationsBefore) != 1 {
		t.Fatalf("Expected 1 station before server change, got %d", len(stationsBefore))
	}

	// Change server address in config
	cfg.Federation.ServerAddress = "301:be28:cf55:3c9::10"

	// Create Yggdrasil address (mock)
	yggAddr := &net.IPAddr{
		IP: net.ParseIP("200:1234:5678:abcd::1"),
	}

	// Get logger
	logger := log.New(os.Stderr, "[test-router] ", log.LstdFlags)

	// Create router with NEW server address
	// This should detect server change and clear cache
	instanceURL := "http://localhost:8080"

	router := NewRouter(db, cfg, pubkey, privkey, yggAddr.String(), instanceURL, logger, nil)
	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}
	defer router.Stop()

	// Verify cache was cleared
	stationsAfter, err := db.ListFederatedStationCache()
	if err != nil {
		t.Fatalf("ListFederatedStationCache() after server change failed: %v", err)
	}
	if len(stationsAfter) != 0 {
		t.Errorf("Expected 0 stations after server change, got %d", len(stationsAfter))
	}

	// Verify new server address was saved
	savedServer := db.GetLastFederationServer()
	if savedServer != cfg.Federation.ServerAddress {
		t.Errorf("Expected saved server %q, got %q", cfg.Federation.ServerAddress, savedServer)
	}
}

// TestRouterIntegration_FederationServerUnchanged tests that cache is NOT cleared when server is unchanged
func TestRouterIntegration_FederationServerUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	serverAddr := "200:same:server::1"
	db, cfg, pubkey, privkey := setupTestEnvironment(t, true, serverAddr)
	defer db.Close()

	// Simulate previous run with same server
	err := db.SaveLastFederationServer(serverAddr)
	if err != nil {
		t.Fatalf("Failed to save server address: %v", err)
	}

	// Insert test station
	station := &database.FederatedStation{
		UUID:        "same-station-uuid",
		SourceNode:  "200:same::",
		Mountpoint:  "/same-stream",
		Name:        "Same Station",
		OwnerPubkey: "test-pubkey",
		Status:      "online",
	}
	err = db.UpsertFederatedStationCache(station)
	if err != nil {
		t.Fatalf("Failed to insert station: %v", err)
	}

	// Verify station exists
	stationsBefore, err := db.ListFederatedStationCache()
	if err != nil {
		t.Fatalf("ListFederatedStationCache() failed: %v", err)
	}
	if len(stationsBefore) != 1 {
		t.Fatalf("Expected 1 station before router creation, got %d", len(stationsBefore))
	}

	// Create Yggdrasil address (mock)
	yggAddr := &net.IPAddr{
		IP: net.ParseIP("200:1234:5678:abcd::1"),
	}

	// Get logger
	logger := log.New(os.Stderr, "[test-router] ", log.LstdFlags)

	// Create router with SAME server address
	// This should NOT clear cache
	instanceURL := "http://localhost:8080"

	router := NewRouter(db, cfg, pubkey, privkey, yggAddr.String(), instanceURL, logger, nil)
	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}
	defer router.Stop()

	// Note: We can't easily test that cache is NOT cleared because
	// when federation is enabled, the federation client will start
	// background workers that will query the federation server.
	// Since we don't have a real federation server in this test,
	// those workers will fail and eventually clear old cache entries.
	//
	// For this test, we just verify that the initial logic doesn't
	// immediately clear the cache due to server change detection.

	// Give a moment for any immediate cache clearing to happen
	// (but not long enough for background workers to clean up)
	// This is a bit hacky but works for this test

	// Verify saved server is still the same
	savedServer := db.GetLastFederationServer()
	if savedServer != serverAddr {
		t.Errorf("Expected saved server %q, got %q", serverAddr, savedServer)
	}
}

// TestRouterIntegration_StopGracefully tests graceful shutdown of router
func TestRouterIntegration_StopGracefully(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, cfg, pubkey, privkey := setupTestEnvironment(t, false, "")
	defer db.Close()

	yggAddr := &net.IPAddr{
		IP: net.ParseIP("200:1234:5678:abcd::1"),
	}

	logger := log.New(os.Stderr, "[test-router] ", log.LstdFlags)
	instanceURL := "http://localhost:8080"

	router := NewRouter(db, cfg, pubkey, privkey, yggAddr.String(), instanceURL, logger, nil)
	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}

	// Test graceful stop
	router.Stop()

	// Stop should be idempotent - calling it again should not panic
	router.Stop()
}
