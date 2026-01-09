package federation_client

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// Client represents a federation client that registers with and queries a federation server
type Client struct {
	config         *config.FederationConfig
	serverConfig   *config.ServerConfig
	db             *database.DB
	logger         *log.Logger
	httpClient     *http.Client
	crypto         *security.CryptoUtil
	localAddress   string // Hostname or IP address
	localPubkey    ed25519.PublicKey
	localPrivkey   ed25519.PrivateKey
	nodeUUID       string
	version        string
	serverPort     int // Port for connecting to remote YggRadio nodes

	// Background workers
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup

	// Cache manager
	cacheManager   *CacheManager
}

// New creates a new federation client
func New(
	cfg *config.FederationConfig,
	serverCfg *config.ServerConfig,
	db *database.DB,
	localAddr string,
	pubkey ed25519.PublicKey,
	privkey ed25519.PrivateKey,
	nodeUUID string,
	version string,
	logger *log.Logger,
) (*Client, error) {
	if !cfg.Enabled {
		logger.Println("Federation client is disabled")
		return nil, nil
	}

	// Validate configuration
	if cfg.ServerAddress == "" {
		return nil, fmt.Errorf("federation server address is required")
	}
	if cfg.ServerPort <= 0 || cfg.ServerPort > 65535 {
		return nil, fmt.Errorf("invalid federation server port: %d", cfg.ServerPort)
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
			DisableKeepAlives:   false,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		config:       cfg,
		serverConfig: serverCfg,
		db:           db,
		logger:       logger,
		httpClient:   httpClient,
		crypto:       security.NewCryptoUtil(),
		localAddress: localAddr,
		localPubkey:  pubkey,
		localPrivkey: privkey,
		nodeUUID:     nodeUUID,
		version:      version,
		serverPort:   serverCfg.Port, // Use local server port for connecting to remote nodes
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize cache manager
	cacheManager, err := NewCacheManager(db, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create cache manager: %w", err)
	}
	client.cacheManager = cacheManager

	logger.Printf("Federation client initialized (server: %s:%d)", cfg.ServerAddress, cfg.ServerPort)

	return client, nil
}

// Start starts the federation client background workers
func (c *Client) Start() error {
	if c == nil {
		return nil
	}

	c.logger.Println("Starting federation client...")

	// Start registration worker
	c.wg.Add(1)
	go c.registrationWorker()

	// Start query worker
	c.wg.Add(1)
	go c.queryWorker()

	// Start cache cleanup worker
	c.wg.Add(1)
	go c.cacheCleanupWorker()

	// Start metadata polling worker
	c.wg.Add(1)
	go c.metadataPollingWorker()

	c.logger.Println("Federation client started successfully")
	return nil
}

// Stop stops the federation client and waits for all workers to finish
func (c *Client) Stop() error {
	if c == nil {
		return nil
	}

	c.logger.Println("Stopping federation client...")

	// Cancel context to signal workers to stop
	c.cancel()

	// Wait for all workers to finish
	c.wg.Wait()

	c.logger.Println("Federation client stopped")
	return nil
}

// registrationWorker periodically registers with the federation server
func (c *Client) registrationWorker() {
	defer c.wg.Done()

	// Register immediately on startup
	if err := c.register(); err != nil {
		c.logger.Printf("ERROR: Initial registration failed: %v", err)
	}

	// Create ticker for periodic registration
	ticker := time.NewTicker(time.Duration(c.config.RegisterInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Println("Registration worker stopped")
			return
		case <-ticker.C:
			if err := c.register(); err != nil {
				c.logger.Printf("ERROR: Registration failed: %v", err)
			}
		}
	}
}

// queryWorker periodically queries federated stations from the server
func (c *Client) queryWorker() {
	defer c.wg.Done()

	// Query immediately on startup
	if err := c.queryStations(); err != nil {
		c.logger.Printf("ERROR: Initial query failed: %v", err)
	}

	// Create ticker for periodic queries
	ticker := time.NewTicker(time.Duration(c.config.QueryInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Println("Query worker stopped")
			return
		case <-ticker.C:
			if err := c.queryStations(); err != nil {
				c.logger.Printf("ERROR: Station query failed: %v", err)
			}
		}
	}
}

// cacheCleanupWorker periodically cleans up old cache entries
func (c *Client) cacheCleanupWorker() {
	defer c.wg.Done()

	// Clean up every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Println("Cache cleanup worker stopped")
			return
		case <-ticker.C:
			// Expire entries older than 2x query interval
			expireAfter := time.Duration(c.config.QueryInterval*2) * time.Second
			if err := c.cacheManager.ExpireOldEntries(expireAfter); err != nil {
				c.logger.Printf("ERROR: Cache cleanup failed: %v", err)
			}
		}
	}
}

// GetServerURL returns the federation server URL
func (c *Client) GetServerURL() string {
	if c == nil {
		return ""
	}

	// Check if address is IPv6 (contains colons but not a port-like pattern)
	// IPv6 addresses need to be wrapped in brackets for URLs
	// Examples: "301:be28:cf55:3c9::10" (IPv6), "localhost" (hostname), "127.0.0.1" (IPv4)
	if strings.Contains(c.config.ServerAddress, ":") {
		// This is an IPv6 address - wrap in brackets
		return fmt.Sprintf("http://[%s]:%d", c.config.ServerAddress, c.config.ServerPort)
	}

	// Regular hostname or IPv4 - no brackets needed
	return fmt.Sprintf("http://%s:%d", c.config.ServerAddress, c.config.ServerPort)
}

// GetCacheManager returns the cache manager
func (c *Client) GetCacheManager() *CacheManager {
	if c == nil {
		return nil
	}
	return c.cacheManager
}
