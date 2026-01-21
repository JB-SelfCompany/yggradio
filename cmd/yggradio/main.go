package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/api"
	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/security"
	"github.com/JB-SelfCompany/yggradio/internal/streaming"
	"github.com/JB-SelfCompany/yggradio/internal/utils"
	"github.com/JB-SelfCompany/yggradio/internal/version"
)

var (
	// Command line flags
	configPath   = flag.String("config", "~/.yggradio/config.yaml", "Path to configuration file")
	showVersion  = flag.Bool("version", false, "Show version information")
	listStations = flag.Bool("list-stations", false, "List all stations with details")
	delStation   = flag.Int64("del-station", 0, "Delete station by ID")
	dbPath       = flag.String("db", "", "Direct path to database file (overrides config)")
)

func main() {
	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("YggRadio version %s\n", version.Version)
		os.Exit(0)
	}

	// Handle CLI commands (list-stations, del-station)
	if *listStations || *delStation > 0 {
		runCLICommands()
		return
	}

	// Setup logger
	logger := log.New(os.Stdout, "[yggradio] ", log.LstdFlags|log.Lshortfile)

	logger.Printf("Starting YggRadio v%s", version.Version)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := database.New(cfg.Database.Path, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Apply database migrations
	if err := database.RunMigrations(db.DB, logger); err != nil {
		logger.Fatalf("Failed to run migrations: %v", err)
	}

	// Load or generate Ed25519 keypair for authentication
	crypto := security.NewCryptoUtil()
	keyPath := utils.ExpandPath("~/.yggradio/yggradio.key")
	pubkey, privkey, err := crypto.LoadOrGenerateKeyPair(keyPath)
	if err != nil {
		logger.Fatalf("Failed to load/generate keypair: %v", err)
	}

	// Find available Yggdrasil IPv6 address with free port

	// Determine bind address and instance URL
	var bindAddr string
	var instanceURL string
	var yggAddr net.IP

	if cfg.Server.Bind != "" && cfg.Server.Bind != "[::]" && cfg.Server.Bind != "0.0.0.0" {
		// Use custom bind address from config (no port check)
		bindAddr = cfg.Server.Bind
		instanceURL = fmt.Sprintf("http://%s:%d", cfg.Server.Bind, cfg.Server.Port)

		// For federation, use the actual bind address that server listens on
		// Extract IP from bind address (remove brackets if present)
		bindIP := strings.Trim(cfg.Server.Bind, "[]")
		yggAddr = net.ParseIP(bindIP)
		if yggAddr == nil {
			logger.Fatalf("Failed to parse bind address '%s' as IP", cfg.Server.Bind)
		}
	} else {
		// Find available Yggdrasil IPv6 address:port
		// Priority: 200::/7 addresses first, then 300::/7
		var err error
		yggAddr, err = utils.FindAvailableAddress(cfg.Server.Port)
		if err != nil {
			logger.Fatalf("Failed to find available Yggdrasil address: %v", err)
		}

		bindAddr = fmt.Sprintf("[%s]", yggAddr.String())
		instanceURL = fmt.Sprintf("http://[%s]:%d", yggAddr.String(), cfg.Server.Port)
	}

	// Initialize audit logger for streaming
	auditLogger, err := security.NewAuditLogger(&security.AuditConfig{
		LogPath:    cfg.Logging.SecurityEvents,
		EnableJSON: true,
		BufferSize: 1000,
	})
	if err != nil {
		logger.Fatalf("Failed to initialize audit logger: %v", err)
	}

	// Initialize streaming server
	streamingServer := streaming.NewServer(
		db,
		&cfg.Streaming,
		&cfg.RateLimit,
		auditLogger,
		logger,
		cfg.Server.Port, // Server port for playlist URL generation
	)
	if err := streamingServer.Start(); err != nil {
		logger.Fatalf("Failed to start streaming server: %v", err)
	}
	defer streamingServer.Stop(30 * time.Second)

	// Start external stream monitor (v1.1.0+)
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go streamingServer.StartExternalStreamMonitor(monitorCtx, 60*time.Second)

	// Initialize HTTP router
	router := api.NewRouter(db, cfg, pubkey, privkey, yggAddr.String(), instanceURL, version.Version, logger, streamingServer)
	handler := router.Setup()
	defer router.Stop() // Stop federation components on shutdown

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", bindAddr, cfg.Server.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
		// For streaming, we need very long (or no) timeouts
		// ReadTimeout: 0 means no timeout
		// WriteTimeout: 0 means no timeout
		// This is necessary for long-lived streaming connections
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  cfg.Server.IdleTimeout,
		ErrorLog:     logger,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("HTTP server error: %v", err)
		}
	}()

	logger.Printf("YggRadio started at %s", instanceURL)

	// Start background maintenance tasks
	go startMaintenanceTasks(db, logger, cfg)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Println("Shutdown signal received, shutting down gracefully...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("Server shutdown error: %v", err)
	}

	logger.Println("YggRadio stopped")
}

// startMaintenanceTasks starts background maintenance tasks
func startMaintenanceTasks(db *database.DB, logger *log.Logger, cfg *config.Config) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Cleanup expired sessions
		if err := db.CleanupExpiredSessions(); err != nil {
			logger.Printf("Maintenance error (sessions): %v", err)
		}

		// Cleanup old audit logs
		retentionDays := cfg.Security.AuditLogRetention / 86400 // Convert seconds to days
		if err := db.CleanupOldAuditLogs(retentionDays); err != nil {
			logger.Printf("Maintenance error (audit logs): %v", err)
		}
	}
}

// runCLICommands handles CLI commands without starting the server
func runCLICommands() {
	// Silent logger for CLI operations
	silentLogger := log.New(os.Stderr, "", 0)

	// Determine database path
	var databasePath string
	if *dbPath != "" {
		// Use direct --db flag (highest priority)
		databasePath = utils.ExpandPath(*dbPath)
	} else {
		// Load from config
		cfg, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load configuration: %v\n", err)
			fmt.Fprintf(os.Stderr, "Config path: %s\n", *configPath)
			fmt.Fprintf(os.Stderr, "Hint: Use --db /path/to/yggradio.db to specify database directly\n")
			os.Exit(1)
		}
		databasePath = cfg.Database.Path
	}

	// Initialize database (silent mode)
	quietLogger := log.New(io.Discard, "", 0)
	db, err := database.New(databasePath, quietLogger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to database: %v\n", err)
		fmt.Fprintf(os.Stderr, "Database path: %s\n", databasePath)
		os.Exit(1)
	}
	defer db.Close()

	// Apply migrations silently
	if err := database.RunMigrations(db.DB, quietLogger); err != nil {
		silentLogger.Printf("Warning: migration error: %v\n", err)
	}

	// Execute CLI command
	if *listStations {
		fmt.Fprintf(os.Stderr, "Using database: %s\n", databasePath)
		if err := cliListStations(db); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else if *delStation > 0 {
		if err := cliDeleteStation(db, *delStation); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

// cliListStations lists all stations in a formatted table
func cliListStations(db *database.DB) error {
	query := `
		SELECT id, name, mountpoint, owner_pubkey, status, listeners_count, created_at
		FROM stations
		ORDER BY id ASC
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query stations: %w", err)
	}
	defer rows.Close()

	type stationInfo struct {
		ID         int64
		Name       string
		Mountpoint string
		Owner      string
		Status     string
		Listeners  int
		CreatedAt  time.Time
	}

	var stations []stationInfo
	maxNameLen := 4       // "Name"
	maxMountLen := 10     // "Mountpoint"
	maxOwnerLen := 5      // "Owner"

	for rows.Next() {
		var s stationInfo
		if err := rows.Scan(&s.ID, &s.Name, &s.Mountpoint, &s.Owner, &s.Status, &s.Listeners, &s.CreatedAt); err != nil {
			return fmt.Errorf("failed to scan station: %w", err)
		}

		// Truncate owner pubkey for display
		if len(s.Owner) > 16 {
			s.Owner = s.Owner[:8] + "..." + s.Owner[len(s.Owner)-4:]
		}

		if len(s.Name) > maxNameLen {
			maxNameLen = len(s.Name)
		}
		if len(s.Mountpoint) > maxMountLen {
			maxMountLen = len(s.Mountpoint)
		}
		if len(s.Owner) > maxOwnerLen {
			maxOwnerLen = len(s.Owner)
		}

		stations = append(stations, s)
	}

	if len(stations) == 0 {
		fmt.Println("No stations found.")
		return nil
	}

	// Print header
	fmt.Println()
	fmt.Printf("  %-4s │ %-*s │ %-*s │ %-*s │ %-8s │ %-9s │ %s\n",
		"ID", maxNameLen, "Name", maxMountLen, "Mountpoint", maxOwnerLen, "Owner", "Status", "Listeners", "Created")
	fmt.Printf("──────┼─%s─┼─%s─┼─%s─┼──────────┼───────────┼─────────────────────\n",
		strings.Repeat("─", maxNameLen), strings.Repeat("─", maxMountLen), strings.Repeat("─", maxOwnerLen))

	// Print stations
	for _, s := range stations {
		statusIcon := "●"
		if s.Status == "online" {
			statusIcon = "\033[32m●\033[0m" // Green
		} else {
			statusIcon = "\033[31m●\033[0m" // Red
		}

		fmt.Printf("  %-4d │ %-*s │ %-*s │ %-*s │ %s %-6s │ %-9d │ %s\n",
			s.ID, maxNameLen, s.Name, maxMountLen, s.Mountpoint, maxOwnerLen, s.Owner,
			statusIcon, s.Status, s.Listeners,
			s.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	fmt.Printf("\nTotal: %d station(s)\n", len(stations))
	return nil
}

// cliDeleteStation deletes a station by ID
func cliDeleteStation(db *database.DB, id int64) error {
	// First check if station exists and get its name
	var name string
	var mountpoint string
	err := db.QueryRow("SELECT name, mountpoint FROM stations WHERE id = ?", id).Scan(&name, &mountpoint)
	if err != nil {
		return fmt.Errorf("station with ID %d not found", id)
	}

	// Delete the station
	result, err := db.Exec("DELETE FROM stations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete station: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("station with ID %d not found", id)
	}

	fmt.Printf("Station deleted successfully:\n")
	fmt.Printf("  ID:         %d\n", id)
	fmt.Printf("  Name:       %s\n", name)
	fmt.Printf("  Mountpoint: %s\n", mountpoint)

	return nil
}
