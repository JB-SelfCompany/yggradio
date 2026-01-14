package api

import (
	"crypto/ed25519"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/JB-SelfCompany/yggradio/internal/api/handlers"
	"github.com/JB-SelfCompany/yggradio/internal/api/middleware"
	"github.com/JB-SelfCompany/yggradio/internal/config"
	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/federation_client"
	"github.com/JB-SelfCompany/yggradio/internal/moderation"
	"github.com/JB-SelfCompany/yggradio/internal/security"
	"github.com/JB-SelfCompany/yggradio/internal/streaming"
	"github.com/JB-SelfCompany/yggradio/internal/web"
)

// Router represents the HTTP router
type Router struct {
	db                  *database.DB
	config              *config.Config
	version             string
	logger              *log.Logger
	rateLimiter         *middleware.RateLimiter
	authMw              *middleware.AuthMiddleware
	csrfMw              *middleware.CSRFMiddleware
	stationHandler      *handlers.StationHandler
	adminHandler        *handlers.AdminHandler
	userHandler         *handlers.UserHandler
	ratingHandler       *handlers.RatingHandler
	authHandler         *handlers.AuthHandler
	streamingServer     handlers.StreamingServer
	// Federation components
	federationClient    *federation_client.Client
	federationProvider  *federation_client.ProviderHandler
	proxyHandler        *streaming.ProxyHandler
}

// NewRouter creates a new HTTP router
// streamingServer can be nil if streaming functionality is not needed yet
func NewRouter(db *database.DB, cfg *config.Config, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, yggAddr string, instanceURL string, version string, logger *log.Logger, streamingServer handlers.StreamingServer) *Router {
	rateLimiter := middleware.NewRateLimiter(
		cfg.RateLimit.APIRequestsPerMinute,
		cfg.RateLimit.AuthAttemptsPerMinute,
	)

	// Create audit logger first
	auditLogger, err := security.NewAuditLogger(&security.AuditConfig{
		LogPath:    cfg.Logging.SecurityEvents,
		EnableJSON: true,
		BufferSize: 1000,
	})
	if err != nil {
		logger.Printf("Warning: failed to create audit logger: %v", err)
	}

	// Create security components with audit logger
	replayProtection := security.NewReplayProtection(5 * time.Minute)
	ipBlocker := security.NewIPBlocker(&security.IPBlockerConfig{
		AutoBlockThreshold: 10,
		BlockDuration:      time.Hour,
		FailureWindow:      15 * time.Minute,
		AuditLogger:        auditLogger,
	})

	authMw := middleware.NewAuthMiddleware(db, rateLimiter, replayProtection, ipBlocker, auditLogger)

	// Initialize security utilities
	validator := security.NewValidator()
	sanitizer := security.NewSanitizer()
	csrfManager := security.NewCSRFManager(15 * time.Minute) // 15 minute TTL
	csrfMw := middleware.NewCSRFMiddleware(csrfManager, auditLogger)
	crypto := security.NewCryptoUtil()

	// Initialize MagicLinkManager for dual authentication
	magicLinkMgr := security.NewMagicLinkManager(db.DB, auditLogger)
	authMw.SetMagicLinkManager(magicLinkMgr)

	// Initialize repositories
	stationRepo := models.NewStationRepository(db.DB)
	userRepo := models.NewUserRepository(db.DB)
	modRepo := models.NewModerationRepository(db.DB)
	ratingRepo := models.NewRatingRepository(db.DB)

	// Initialize Proof-of-Work managers
	powStation := moderation.NewProofOfWork(cfg.Moderation.PoWDifficultyStation)
	powAccount := moderation.NewProofOfWork(cfg.Moderation.PoWDifficultyAccount)

	// Initialize handlers
	// Use instance URL for external access
	serverURL := instanceURL

	stationHandler := handlers.NewStationHandler(
		db.DB,
		db,
		stationRepo,
		ratingRepo,
		powStation,
		validator,
		sanitizer,
		logger,
		cfg.Streaming.ServerSecret,
		streamingServer,
		serverURL,
	)

	adminHandler := handlers.NewAdminHandler(
		db.DB,
		modRepo,
		powStation, // Using station PoW manager for admin operations
		logger,
		validator,
		sanitizer,
		csrfManager,
		auditLogger,
	)

	userHandler := handlers.NewUserHandler(
		db.DB,
		userRepo,
		validator,
		sanitizer,
		auditLogger,
		logger,
	)

	ratingHandler := handlers.NewRatingHandler(
		db.DB,
		db, // dbWrapper for federated station queries
		ratingRepo,
		stationRepo,
		validator,
		crypto,
		publicKey,
		privateKey,
		logger,
		cfg.Server.Port, // Server port for federated requests
	)

	// Initialize AuthHandler for dual authentication
	authHandler := handlers.NewAuthHandler(
		db.DB,
		userRepo,
		magicLinkMgr,
		powAccount,
		rateLimiter,
		auditLogger,
		csrfManager,
		cfg,
		yggAddr,
		logger,
	)

	// Initialize federation components (if enabled)
	var federationClient *federation_client.Client
	var federationProvider *federation_client.ProviderHandler
	var proxyHandler *streaming.ProxyHandler

	// Check federation status
	if !cfg.Federation.Enabled {
		// Federation is DISABLED - clean up any stale cache from previous runs
		logger.Println("Federation is disabled, cleaning up stale federated cache...")

		if err := db.ClearFederatedStationCache(); err != nil {
			logger.Printf("WARNING: Failed to clear federated station cache: %v", err)
			// Don't fail startup - continue without federation
		} else {
			logger.Println("Federated station cache cleared successfully")
		}

		// Don't initialize federation components
		federationClient = nil
		federationProvider = nil
		proxyHandler = nil

	} else {
		// Federation is ENABLED
		logger.Println("Initializing federation client...")

		// Check if federation server address changed since last run
		lastServerAddress := db.GetLastFederationServer()
		currentServerAddress := cfg.Federation.ServerAddress

		if lastServerAddress != "" && lastServerAddress != currentServerAddress {
			logger.Printf("Federation server changed (%s -> %s), clearing old cache...",
				lastServerAddress, currentServerAddress)

			if err := db.ClearFederatedStationCache(); err != nil {
				logger.Printf("WARNING: Failed to clear cache on server change: %v", err)
			} else {
				logger.Println("Old federated cache cleared due to server change")
			}
		}

		// Save current server address for next startup comparison
		if err := db.SaveLastFederationServer(currentServerAddress); err != nil {
			logger.Printf("WARNING: Failed to save federation server address: %v", err)
			// Continue anyway - not critical
		}

		// Generate node UUID (used for federation server tracking)
		nodeUUID := uuid.New().String()

		// Create federation client
		var err error
		federationClient, err = federation_client.New(
			&cfg.Federation,
			&cfg.Server,
			db,
			yggAddr,
			publicKey,
			privateKey,
			nodeUUID,
			"0.1.0", // YggRadio version
			logger,
		)
		if err != nil {
			logger.Printf("Failed to create federation client: %v", err)
		} else if federationClient != nil {
			// Start federation client background workers
			if err := federationClient.Start(); err != nil {
				logger.Printf("Failed to start federation client: %v", err)
			} else {
				logger.Println("Federation client started successfully")
			}

			// Create federation provider handler (for server to pull our stations)
			federationProvider = federation_client.NewProviderHandler(
				db.DB,
				stationRepo,
				ratingRepo,
				validator,
				crypto,
				publicKey,
				privateKey,
				logger,
			)

			// Create proxy handler (for streaming federated stations)
			proxyConfig := &streaming.ProxyConfig{
				MaxProxiesPerIP:      5,
				MaxProxiesGlobal:     100,
				MaxReconnectAttempts: 3,
				StreamTimeout:        5 * time.Minute,
				ServerPort:           cfg.Server.Port, // Use server port for connecting to remote nodes
			}
			proxyHandler = streaming.NewProxyHandler(
				db,
				validator,
				auditLogger,
				logger,
				proxyConfig,
			)

			logger.Println("Federation provider and proxy handlers initialized")
		}
	}

	return &Router{
		db:                 db,
		config:             cfg,
		version:            version,
		logger:             logger,
		rateLimiter:        rateLimiter,
		authMw:             authMw,
		csrfMw:             csrfMw,
		stationHandler:     stationHandler,
		adminHandler:       adminHandler,
		userHandler:        userHandler,
		ratingHandler:      ratingHandler,
		authHandler:        authHandler,
		streamingServer:    streamingServer,
		federationClient:   federationClient,
		federationProvider: federationProvider,
		proxyHandler:       proxyHandler,
	}
}

// Stop gracefully stops the router and its components
func (rt *Router) Stop() {
	rt.logger.Println("Stopping router and federation components...")

	// Stop federation client workers
	if rt.federationClient != nil {
		rt.logger.Println("Stopping federation client workers...")
		if err := rt.federationClient.Stop(); err != nil {
			rt.logger.Printf("ERROR: Failed to stop federation client: %v", err)
		} else {
			rt.logger.Println("Federation client stopped successfully")
		}
	}

	// Close active proxy connections
	if rt.proxyHandler != nil {
		rt.logger.Println("Closing active proxy connections...")
		if err := rt.proxyHandler.Close(); err != nil {
			rt.logger.Printf("ERROR: Failed to close proxy handler: %v", err)
		} else {
			rt.logger.Println("Proxy handler closed successfully")
		}
	}

	rt.logger.Println("Router stopped")
}

// Setup sets up all routes
func (rt *Router) Setup() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/health", rt.handleHealth)
	mux.HandleFunc("/api/info", rt.handleInfo)
	mux.HandleFunc("/api/stats", rt.handleStats)

	// Authentication routes (public - no auth required)
	mux.HandleFunc("/api/auth/register/magic-link", rt.authHandler.RegisterMagicLink)
	mux.HandleFunc("/login", rt.authHandler.LoginWithMagicLink)

	// Authentication routes (protected - requires AuthenticateAny)
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		rt.authMw.AuthenticateAny(http.HandlerFunc(rt.authHandler.Logout)).ServeHTTP(w, r)
	})
	mux.HandleFunc("/api/auth/sessions", func(w http.ResponseWriter, r *http.Request) {
		rt.authMw.AuthenticateAny(http.HandlerFunc(rt.authHandler.GetSessions)).ServeHTTP(w, r)
	})

	// CSRF token endpoint (requires authentication - any method)
	mux.HandleFunc("/api/csrf-token", func(w http.ResponseWriter, r *http.Request) {
		rt.authMw.AuthenticateAny(http.HandlerFunc(rt.csrfMw.GenerateToken)).ServeHTTP(w, r)
	})

	// Station routes
	mux.HandleFunc("/api/stations", rt.handleStations)
	mux.HandleFunc("/api/stations/", rt.handleStationDetail)

	// User routes
	mux.HandleFunc("/api/user/profile", rt.handleUserProfile)

	// Admin/Management routes - Available to all authenticated users
	mux.HandleFunc("/api/admin/stats", rt.handleAdminStats)
	mux.HandleFunc("/api/admin/", rt.handleAdmin)

	// Federation routes
	mux.HandleFunc("/api/federation/stations", rt.handleFederationStationProvider)
	mux.HandleFunc("/api/federation/ratings", rt.handleFederationRatingReceiver)
	mux.HandleFunc("/api/federated/stations/", rt.handleFederatedStationRating)

	// Create web handler for serving frontend
	webHandler, err := web.NewHandler(rt.logger)
	if err != nil {
		rt.logger.Printf("Warning: failed to create web handler: %v", err)
	}

	// Create a custom handler that routes between API, streaming, and frontend
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// API routes and special endpoints go to the API mux
		if strings.HasPrefix(path, "/api/") || path == "/login" {
			mux.ServeHTTP(w, r)
			return
		}

		// Streaming source requests (PUT/SOURCE method)
		if rt.streamingServer != nil && (r.Method == http.MethodPut || r.Method == "SOURCE") {
			rt.streamingServer.ServeHTTP(w, r)
			return
		}

		// Federated stream proxy requests
		if strings.HasPrefix(path, "/stream/federated/") {
			if rt.proxyHandler != nil {
				rt.proxyHandler.ServeHTTP(w, r)
			} else {
				http.Error(w, "Federation not enabled", http.StatusServiceUnavailable)
			}
			return
		}

		// Station URL proxy: /stations/:mountpoint -> /:mountpoint
		// This provides user-friendly URLs that proxy to the actual stream mountpoint
		// Works with media players like VLC that don't follow redirects properly
		if strings.HasPrefix(path, "/stations/") && r.Method == http.MethodGet {
			mountpoint := strings.TrimPrefix(path, "/stations")
			// Only proxy if mountpoint is not empty (not just "/" or "")
			if mountpoint != "" && mountpoint != "/" {
				// Check if this is a federated stream request
				// Federated streams use /stream/federated/{uuid} format
				if strings.HasPrefix(mountpoint, "/stream/federated/") {
					if rt.proxyHandler != nil {
						// Rewrite the request path to the federated stream path
						r.URL.Path = mountpoint
						rt.proxyHandler.ServeHTTP(w, r)
					} else {
						http.Error(w, "Federation not enabled", http.StatusServiceUnavailable)
					}
					return
				}

				// Local stream - forward to streaming server
				if rt.streamingServer != nil {
					r.URL.Path = mountpoint
					rt.streamingServer.ServeHTTP(w, r)
					return
				}
			}
		}

		// Frontend routes: /, /assets/*, frontend files, known SPA routes
		// Note: We explicitly check for common frontend extensions to avoid catching audio streams
		isFrontendFile := strings.HasSuffix(path, ".js") ||
			strings.HasSuffix(path, ".css") ||
			strings.HasSuffix(path, ".html") ||
			strings.HasSuffix(path, ".svg") ||
			strings.HasSuffix(path, ".png") ||
			strings.HasSuffix(path, ".jpg") ||
			strings.HasSuffix(path, ".ico") ||
			strings.HasSuffix(path, ".json") ||
			strings.HasSuffix(path, ".woff") ||
			strings.HasSuffix(path, ".woff2") ||
			strings.HasSuffix(path, ".ttf") ||
			strings.HasSuffix(path, ".eot")

		isFrontendRoute := path == "/" ||
			strings.HasPrefix(path, "/assets/") ||
			isFrontendFile ||
			strings.HasPrefix(path, "/stations") ||
			strings.HasPrefix(path, "/profile") ||
			strings.HasPrefix(path, "/admin") ||
			strings.HasPrefix(path, "/federation")

		if isFrontendRoute {
			if webHandler != nil {
				webHandler.ServeHTTP(w, r)
			} else {
				http.Error(w, "Frontend not available", http.StatusServiceUnavailable)
			}
			return
		}

		// Everything else is treated as a potential mountpoint (streaming)
		// This includes: /rd, /stream, etc. (with or without audio extensions like .mp3)
		if rt.streamingServer != nil {
			rt.streamingServer.ServeHTTP(w, r)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	})

	// Apply middleware in reverse order (they wrap around each other)
	var handler http.Handler = finalHandler

	// Recovery middleware (outermost)
	handler = middleware.RecoverPanic(handler)

	// Logging
	handler = middleware.LogRequest(handler)

	// Security headers
	handler = middleware.SecurityHeaders(handler)

	// CORS
	handler = middleware.CORS([]string{"*"})(handler)

	// Gzip compression
	handler = middleware.Gzip(handler)

	// Rate limiting
	handler = rt.rateLimiter.LimitAPI(handler)

	return handler
}

// handleHealth returns health status
func (rt *Router) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check database connectivity
	if err := rt.db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "unhealthy",
			"error":  "database unavailable",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"uptime": "ok",
	})
}

// handleInfo returns instance information
func (rt *Router) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":        rt.config.Server.InstanceName,
		"description": rt.config.Server.InstanceDescription,
		"version":     rt.version,
		"port":        rt.config.Server.Port,
		"features": map[string]bool{
			"streaming":  true,
			"federation": rt.config.Federation.Enabled,
		},
		"pow_difficulty": map[string]int{
			"account": rt.config.Moderation.PoWDifficultyAccount,
			"station": rt.config.Moderation.PoWDifficultyStation,
		},
	})
}

// handleStats returns public statistics
func (rt *Router) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := rt.db.GetDatabaseStats()
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Station handlers

func (rt *Router) handleStations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rt.stationHandler.ListStations(w, r)
	case http.MethodPost:
		// SECURITY: Requires authentication (Ed25519 or Magic Link) + CSRF protection
		rt.authMw.AuthenticateAny(
			rt.csrfMw.Protect(http.HandlerFunc(rt.stationHandler.CreateStation)),
		).ServeHTTP(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (rt *Router) handleStationDetail(w http.ResponseWriter, r *http.Request) {
	// Check if this is a credentials request (GET - no CSRF needed)
	if strings.HasSuffix(r.URL.Path, "/credentials") {
		rt.authMw.AuthenticateAny(http.HandlerFunc(rt.stationHandler.GetStreamingCredentials)).ServeHTTP(w, r)
		return
	}

	// Check if this is a stop broadcast request (POST/PUT - needs CSRF)
	if strings.HasSuffix(r.URL.Path, "/stop") {
		rt.authMw.AuthenticateAny(
			rt.csrfMw.Protect(http.HandlerFunc(rt.stationHandler.StopBroadcast)),
		).ServeHTTP(w, r)
		return
	}

	// Check if this is a start broadcast request (POST/PUT - needs CSRF) - v1.1.0+
	if strings.HasSuffix(r.URL.Path, "/start") {
		rt.authMw.AuthenticateAny(
			rt.csrfMw.Protect(http.HandlerFunc(rt.stationHandler.StartBroadcast)),
		).ServeHTTP(w, r)
		return
	}

	// Check if this is a playlist script request (GET - no CSRF needed)
	if strings.Contains(r.URL.Path, "/playlist-script") {
		rt.authMw.AuthenticateAny(http.HandlerFunc(rt.stationHandler.GetPlaylistScript)).ServeHTTP(w, r)
		return
	}

	// Check if this is a playlist config request (PUT/PATCH - needs CSRF)
	if strings.Contains(r.URL.Path, "/playlist-config") {
		rt.authMw.AuthenticateAny(
			rt.csrfMw.Protect(http.HandlerFunc(rt.stationHandler.UpdatePlaylistConfig)),
		).ServeHTTP(w, r)
		return
	}

	// Rating endpoints
	if strings.Contains(r.URL.Path, "/rate") {
		if r.Method == http.MethodPost {
			// SECURITY: CSRF protection for rating submissions
			// Use AuthenticateAny to support both Ed25519 and magic link users
			rt.authMw.AuthenticateAny(
				rt.csrfMw.Protect(http.HandlerFunc(rt.ratingHandler.SubmitRating)),
			).ServeHTTP(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if strings.Contains(r.URL.Path, "/ratings") {
		rt.ratingHandler.GetRatingStats(w, r) // No auth required
		return
	}

	if strings.Contains(r.URL.Path, "/my-rating") {
		rt.authMw.AuthenticateAny(http.HandlerFunc(rt.ratingHandler.GetMyRating)).ServeHTTP(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rt.stationHandler.GetStation(w, r)
	case http.MethodPut, http.MethodPatch:
		// SECURITY: CSRF protection for station updates (supports both Ed25519 and Magic Link)
		rt.authMw.AuthenticateAny(
			rt.csrfMw.Protect(http.HandlerFunc(rt.stationHandler.UpdateStation)),
		).ServeHTTP(w, r)
	case http.MethodDelete:
		// SECURITY: CSRF protection for station deletion (supports both Ed25519 and Magic Link)
		rt.authMw.AuthenticateAny(
			rt.csrfMw.Protect(http.HandlerFunc(rt.stationHandler.DeleteStation)),
		).ServeHTTP(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// User profile handler

func (rt *Router) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	rt.authMw.AuthenticateAny(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rt.userHandler.GetProfile(w, r)
		default:
			// Note: Profile updates removed - users are now anonymous (privacy-first)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})).ServeHTTP(w, r)
}

// Admin handlers

func (rt *Router) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	rt.authMw.AuthenticateAny(http.HandlerFunc(rt.adminHandler.GetInstanceStats)).ServeHTTP(w, r)
}

func (rt *Router) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if strings.HasPrefix(path, "/api/admin/audit") {
		rt.authMw.AuthenticateAny(http.HandlerFunc(rt.adminHandler.GetAuditLog)).ServeHTTP(w, r)
		return
	}

	// Default admin endpoint
	http.Error(w, "Admin endpoint - specify action", http.StatusBadRequest)
}

// Federation handler

// handleFederationStationProvider provides public station list to federation server
func (rt *Router) handleFederationStationProvider(w http.ResponseWriter, r *http.Request) {
	if rt.federationProvider == nil {
		http.Error(w, "Federation not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// This endpoint is called by federation server to pull our stations
	// Returns all public stations with ratings
	rt.federationProvider.ServeHTTP(w, r)
}

// handleFederationRatingReceiver receives ratings from other federated nodes
func (rt *Router) handleFederationRatingReceiver(w http.ResponseWriter, r *http.Request) {
	// This endpoint receives ratings from other nodes for our local stations
	// No authentication middleware - signature is verified in the handler
	rt.ratingHandler.ReceiveFederatedRating(w, r)
}

// handleFederatedStationRating handles rating submission for federated stations
func (rt *Router) handleFederatedStationRating(w http.ResponseWriter, r *http.Request) {
	// This endpoint allows authenticated users to rate federated stations
	// Ratings are forwarded to the source node
	if strings.Contains(r.URL.Path, "/rate") && r.Method == http.MethodPost {
		// SECURITY: CSRF protection for federated rating submissions
		// Use AuthenticateAny to support both Ed25519 and magic link users
		rt.authMw.AuthenticateAny(
			rt.csrfMw.Protect(http.HandlerFunc(rt.ratingHandler.SubmitFederatedRating)),
		).ServeHTTP(w, r)
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}
