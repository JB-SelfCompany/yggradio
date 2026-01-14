package main

import (
	"context"
	"flag"
	"fmt"
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
)

var (
	// Version is set at build time
	Version = "1.2.0"

	// Command line flags
	configPath = flag.String("config", "~/.yggradio/config.yaml", "Path to configuration file")
	version    = flag.Bool("version", false, "Show version information")
)

func main() {
	flag.Parse()

	// Show version and exit
	if *version {
		fmt.Printf("YggRadio version %s\n", Version)
		os.Exit(0)
	}

	// Setup logger
	logger := log.New(os.Stdout, "[yggradio] ", log.LstdFlags|log.Lshortfile)

	logger.Printf("Starting YggRadio v%s", Version)

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
	router := api.NewRouter(db, cfg, pubkey, privkey, yggAddr.String(), instanceURL, Version, logger, streamingServer)
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
