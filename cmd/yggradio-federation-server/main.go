package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/federation_server"
	"github.com/JB-SelfCompany/yggradio/internal/utils"
)

var (
	// Version is set at build time
	Version = "1.0.0"

	// Command line flags
	configPath = flag.String("config", "~/.yggradio-federation/config.yaml", "Path to configuration file")
	version    = flag.Bool("version", false, "Show version information")
)

func main() {
	flag.Parse()

	// Show version and exit
	if *version {
		fmt.Printf("YggRadio Federation Server version %s\n", Version)
		os.Exit(0)
	}

	// Setup logger (initially to stdout only)
	logger := log.New(os.Stdout, "[federation-server] ", log.LstdFlags|log.Lshortfile)

	logger.Printf("Starting YggRadio Federation Server v%s", Version)

	// Load configuration
	cfg, err := federation_server.Load(*configPath)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Setup file logging if configured
	if cfg.Logging.LogFile != "" {
		// Create log directory if it doesn't exist
		logDir := filepath.Dir(cfg.Logging.LogFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			logger.Printf("Warning: Failed to create log directory: %v", err)
		} else {
			// Open log file
			logFile, err := os.OpenFile(cfg.Logging.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				logger.Printf("Warning: Failed to open log file: %v", err)
			} else {
				// Use MultiWriter to write to both stdout and file
				multiWriter := io.MultiWriter(os.Stdout, logFile)
				logger.SetOutput(multiWriter)
				// Note: logFile will be closed when program exits
			}
		}
	}

	// Initialize database
	db, err := federation_server.NewDB(cfg.Database.Path, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Check port availability on Yggdrasil addresses if bind is [::]
	if cfg.Server.Bind == "" || cfg.Server.Bind == "[::]" {
		// Find available address with free port
		yggAddr, err := utils.FindAvailableAddress(cfg.Server.Port)
		if err != nil {
			logger.Fatalf("Failed to find available Yggdrasil address: %v", err)
		}

		// Update bind to use the specific address
		cfg.Server.Bind = fmt.Sprintf("[%s]", yggAddr.String())
	} else {
		// Custom bind address - check if specified port is available
		// Extract IP from bind address (could be "[ipv6]:port" or "ipv6" or "::")
		bindHost := cfg.Server.Bind
		if bindHost[0] == '[' {
			// Remove brackets for IPv6
			bindHost = bindHost[1 : len(bindHost)-1]
		}

		if bindHost != "::" && bindHost != "0.0.0.0" {
			// Parse and check specific address
			ip := net.ParseIP(bindHost)
			if ip != nil && !utils.IsPortAvailable(ip, cfg.Server.Port) {
				logger.Fatalf("Port %d is not available on %s", cfg.Server.Port, bindHost)
			}
		}
	}

	// Create and start server
	server := federation_server.NewServer(cfg, db, logger, Version)

	if err := server.Start(); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}

	logger.Printf("Federation server started at %s:%d", cfg.Server.Bind, cfg.Server.Port)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Println("Shutdown signal received, shutting down gracefully...")

	// Graceful shutdown with 30 second timeout
	if err := server.Stop(30 * time.Second); err != nil {
		logger.Printf("Server shutdown error: %v", err)
	}

	logger.Println("YggRadio Federation Server stopped")
}
