package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/moderation"
	"github.com/JB-SelfCompany/yggradio/internal/security"
	"github.com/JB-SelfCompany/yggradio/internal/streaming"
)

// StationHandler handles station-related API endpoints
type StationHandler struct {
	db              *sql.DB
	dbWrapper       *database.DB // For federated station queries
	stationRepo     *models.StationRepository
	ratingRepo      *models.RatingRepository
	powManager      *moderation.ProofOfWork
	validator       *security.Validator
	sanitizer       *security.Sanitizer
	logger          *log.Logger
	serverSecret    string
	streamingServer StreamingServer
	serverURL       string
}

// StreamingServer interface for streaming operations
type StreamingServer interface {
	DisconnectSource(mountpoint string) error
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	// External stream management (v1.1.0+)
	StopExternalStream(mountpoint string)
	StartExternalStream(mountpoint string) error
	IsExternalStreamActive(mountpoint string) bool
}

// NewStationHandler creates a new station handler
func NewStationHandler(
	db *sql.DB,
	dbWrapper *database.DB,
	stationRepo *models.StationRepository,
	ratingRepo *models.RatingRepository,
	powManager *moderation.ProofOfWork,
	validator *security.Validator,
	sanitizer *security.Sanitizer,
	logger *log.Logger,
	serverSecret string,
	streamingServer StreamingServer,
	serverURL string,
) *StationHandler {
	return &StationHandler{
		db:              db,
		dbWrapper:       dbWrapper,
		stationRepo:     stationRepo,
		ratingRepo:      ratingRepo,
		powManager:      powManager,
		validator:       validator,
		sanitizer:       sanitizer,
		logger:          logger,
		serverSecret:    serverSecret,
		streamingServer: streamingServer,
		serverURL:       serverURL,
	}
}

// ListStations returns a paginated list of stations
func (h *StationHandler) ListStations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Parse sorting parameters (v1.1.0+)
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort_by")))
	sortOrder := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort_order")))

	// Validate and set defaults
	if sortBy == "" {
		sortBy = "created" // Default sort by creation date
	}
	if sortOrder == "" {
		sortOrder = "desc" // Default descending order
	}

	// Whitelist validation (additional safety, already validated in repository)
	validSortBy := map[string]bool{"listeners": true, "rating": true, "created": true}
	if !validSortBy[sortBy] {
		http.Error(w, "Invalid sort_by parameter. Must be: listeners, rating, or created", http.StatusBadRequest)
		return
	}

	validSortOrder := map[string]bool{"asc": true, "desc": true}
	if !validSortOrder[sortOrder] {
		http.Error(w, "Invalid sort_order parameter. Must be: asc or desc", http.StatusBadRequest)
		return
	}

	// Extract optional pubkey from context
	ownerPubkey := ""
	if pubkey, ok := r.Context().Value("pubkey").(string); ok {
		ownerPubkey = pubkey
	}

	// Get LOCAL stations from database with privacy filter and sorting
	stations, err := h.stationRepo.ListWithPrivacyFilterSorted(limit, offset, ownerPubkey, sortBy, sortOrder)
	if err != nil {
		h.logger.Printf("ERROR: Failed to list stations: %v", err)
		http.Error(w, "Failed to retrieve stations", http.StatusInternalServerError)
		return
	}

	// Convert local stations to API response format
	response := make([]map[string]interface{}, 0, len(stations))
	for _, station := range stations {
		// Get rating stats for this station
		var averageRating float64
		var voteCount int
		if h.ratingRepo != nil {
			stats, err := h.ratingRepo.GetStationRatings(station.ID, "")
			if err != nil {
				h.logger.Printf("WARNING: Failed to get ratings for station %d: %v", station.ID, err)
				// Continue with zero ratings instead of failing
			} else if stats != nil {
				averageRating = stats.AverageRating
				voteCount = stats.VoteCount
			}
		}

		item := map[string]interface{}{
			"id":              station.ID,
			"uuid":            station.UUID,
			"name":            station.Name,
			"mountpoint":      station.Mountpoint,
			"owner_pubkey":    station.OwnerPubkey,
			"created_at":      station.CreatedAt,
			"updated_at":      station.UpdatedAt,
			"status":          station.Status,
			"listeners_count": station.ListenersCount,
			"content_type":    station.ContentType,
			"is_private":      station.IsPrivate,
			"is_federated":    false, // Local station
			"average_rating":  averageRating,
			"vote_count":      voteCount,
		}

		if station.Description.Valid {
			item["description"] = station.Description.String
		}
		if station.MetadataTitle.Valid {
			item["metadata_title"] = station.MetadataTitle.String
		}
		if station.Bitrate.Valid {
			item["bitrate"] = station.Bitrate.Int64
		}

		// External stream fields (v1.1.0+)
		if station.ExternalStreamURL.Valid {
			item["external_stream_url"] = station.ExternalStreamURL.String
		}
		if station.ExternalStreamType.Valid {
			item["external_stream_type"] = station.ExternalStreamType.String
			// For HLS streams, provide proxy URL for browser playback
			if station.ExternalStreamType.String == "hls" {
				item["hls_proxy_url"] = fmt.Sprintf("/proxy/hls%s/playlist.m3u8", station.Mountpoint)
			}
		}

		response = append(response, item)
	}

	// Get FEDERATED stations from cache
	if h.dbWrapper != nil {
		federatedStations, err := h.dbWrapper.ListFederatedStationCache()
		if err != nil {
			h.logger.Printf("WARNING: Failed to list federated stations: %v", err)
			// Continue without federated stations instead of failing
		} else {
			// Add federated stations to response
			for _, fs := range federatedStations {
				// Use simplified URL format with UUID for easier routing
				item := map[string]interface{}{
					"id":              0, // No local ID for federated stations
					"uuid":            fs.UUID,
					"name":            fs.Name,
					"mountpoint":      fmt.Sprintf("/stream/federated/%s", fs.UUID), // Simplified URL format
					"owner_pubkey":    fs.OwnerPubkey,
					"status":          fs.Status,
					"listeners_count": fs.ListenersCount,
					"is_private":      false, // Federated stations are always public
					"is_federated":    true,
					"source_node":     fs.SourceNode,
					"average_rating":  fs.AverageRating,
					"vote_count":      fs.VoteCount,
				}

				if fs.Description.Valid {
					item["description"] = fs.Description.String
				}
				if fs.SourceNodeName.Valid {
					item["source_node_name"] = fs.SourceNodeName.String
				}
				if fs.ContentType.Valid {
					item["content_type"] = fs.ContentType.String
				}
				if fs.Bitrate.Valid {
					item["bitrate"] = fs.Bitrate.Int64
				}
				if fs.Genre.Valid {
					item["genre"] = fs.Genre.String
				}
				if fs.MetadataTitle.Valid {
					item["metadata_title"] = fs.MetadataTitle.String
				}

				response = append(response, item)
			}
		}
	}

	// Sort combined results (local + federated) by requested field (v1.1.0+)
	// Note: Local stations are already sorted by DB, but federated need sorting
	if len(response) > 0 {
		sortStationsByField(response, sortBy, sortOrder)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stations": response,
		"limit":    limit,
		"offset":   offset,
	})
}

// sortStationsByField sorts stations by the specified field and order
func sortStationsByField(stations []map[string]interface{}, sortBy, sortOrder string) {
	// Simple bubble sort for small lists (pagination limits to max 100)
	switch sortBy {
	case "listeners":
		if sortOrder == "asc" {
			// Ascending order
			for i := 0; i < len(stations)-1; i++ {
				for j := i + 1; j < len(stations); j++ {
					count1, _ := stations[i]["listeners_count"].(int)
					count2, _ := stations[j]["listeners_count"].(int)
					if count1 > count2 {
						stations[i], stations[j] = stations[j], stations[i]
					}
				}
			}
		} else {
			// Descending order
			for i := 0; i < len(stations)-1; i++ {
				for j := i + 1; j < len(stations); j++ {
					count1, _ := stations[i]["listeners_count"].(int)
					count2, _ := stations[j]["listeners_count"].(int)
					if count1 < count2 {
						stations[i], stations[j] = stations[j], stations[i]
					}
				}
			}
		}
	case "rating":
		if sortOrder == "asc" {
			for i := 0; i < len(stations)-1; i++ {
				for j := i + 1; j < len(stations); j++ {
					rating1, _ := stations[i]["average_rating"].(float64)
					rating2, _ := stations[j]["average_rating"].(float64)
					if rating1 > rating2 {
						stations[i], stations[j] = stations[j], stations[i]
					}
				}
			}
		} else {
			for i := 0; i < len(stations)-1; i++ {
				for j := i + 1; j < len(stations); j++ {
					rating1, _ := stations[i]["average_rating"].(float64)
					rating2, _ := stations[j]["average_rating"].(float64)
					if rating1 < rating2 {
						stations[i], stations[j] = stations[j], stations[i]
					}
				}
			}
		}
	// For "created", stations are already sorted by DB query
	}
}

// GetStation returns a single station by ID or mountpoint
func (h *StationHandler) GetStation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	// URL format: /api/stations/{id} or /api/stations/mount/{mountpoint}
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	parts := strings.Split(path, "/")

	var station *models.Station
	var err error

	if len(parts) >= 2 && parts[0] == "mount" {
		// Get by mountpoint
		mountpoint := "/" + strings.Join(parts[1:], "/")
		station, err = h.stationRepo.GetByMountpoint(mountpoint)
	} else {
		// Get by ID
		id, parseErr := strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil {
			http.Error(w, "Invalid station ID", http.StatusBadRequest)
			return
		}
		station, err = h.stationRepo.GetByID(id)
	}

	if err != nil {
		h.logger.Printf("ERROR: Failed to get station: %v", err)
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Check privacy - if private, only owner can access
	if station.IsPrivate {
		requestPubkey := ""
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			requestPubkey = pubkey
		}
		if requestPubkey != station.OwnerPubkey {
			http.Error(w, "Station not found", http.StatusNotFound)
			return
		}
	}

	// Build response
	response := map[string]interface{}{
		"id":              station.ID,
		"uuid":            station.UUID,
		"name":            station.Name,
		"mountpoint":      station.Mountpoint,
		"owner_pubkey":    station.OwnerPubkey,
		"created_at":      station.CreatedAt,
		"updated_at":      station.UpdatedAt,
		"status":          station.Status,
		"listeners_count": station.ListenersCount,
		"content_type":    station.ContentType,
		"is_private":      station.IsPrivate,
	}

	if station.Description.Valid {
		response["description"] = station.Description.String
	}
	if station.MetadataTitle.Valid {
		response["metadata_title"] = station.MetadataTitle.String
	}
	if station.Bitrate.Valid {
		response["bitrate"] = station.Bitrate.Int64
	}

	// External stream fields (v1.1.0+)
	if station.ExternalStreamURL.Valid {
		response["external_stream_url"] = station.ExternalStreamURL.String
	}
	if station.ExternalStreamType.Valid {
		response["external_stream_type"] = station.ExternalStreamType.String
		// For HLS streams, provide proxy URL for browser playback
		if station.ExternalStreamType.String == "hls" {
			response["hls_proxy_url"] = fmt.Sprintf("/proxy/hls%s/playlist.m3u8", station.Mountpoint)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CreateStation creates a new station
func (h *StationHandler) CreateStation(w http.ResponseWriter, r *http.Request) {
	h.logger.Printf("CreateStation: Request received from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
		h.logger.Printf("CreateStation: Got user_id=%d from context", userID)
	} else {
		h.logger.Printf("CreateStation: ERROR - No user_id in context")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)
	h.logger.Printf("CreateStation: auth_method=%s", authMethod)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
			h.logger.Printf("CreateStation: Using Ed25519 pubkey (len=%d)", len(pubkey))
		} else {
			h.logger.Printf("CreateStation: ERROR - Ed25519 auth but no pubkey in context")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		// Format: "magiclink:<user_id>" (64 hex chars padded)
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
		h.logger.Printf("CreateStation: Generated synthetic pubkey for magic_link user: %s", ownerPubkey)
	} else {
		h.logger.Printf("CreateStation: ERROR - Unknown auth_method: '%s'", authMethod)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request
	var req struct {
		Name               string `json:"name"`
		Description        string `json:"description"`
		Mountpoint         string `json:"mountpoint"`
		ContentType        string `json:"content_type"`
		Bitrate            *int64 `json:"bitrate"`
		IsPrivate          bool   `json:"is_private"`
		PoWNonce           string `json:"pow_nonce"`
		PoWTimestamp       int64  `json:"pow_timestamp"`
		ExternalStreamURL  string `json:"external_stream_url"`  // v1.1.0+
		ExternalStreamType string `json:"external_stream_type"` // v1.1.0+ ("hls" or "direct")
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Printf("ERROR: Failed to decode request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate and sanitize input
	req.Name = h.sanitizer.SanitizeString(req.Name)
	req.Description = h.sanitizer.SanitizeString(req.Description)
	req.Mountpoint = h.sanitizer.SanitizeString(req.Mountpoint)

	if err := h.validator.ValidateName(req.Name); err != nil {
		h.logger.Printf("ERROR: Invalid station name '%s': %v", req.Name, err)
		http.Error(w, fmt.Sprintf("Invalid name: %v", err), http.StatusBadRequest)
		return
	}

	if req.Description != "" {
		if err := h.validator.ValidateDescription(req.Description); err != nil {
			h.logger.Printf("ERROR: Invalid description: %v", err)
			http.Error(w, fmt.Sprintf("Invalid description: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Validate mountpoint format
	if !strings.HasPrefix(req.Mountpoint, "/") {
		h.logger.Printf("ERROR: Mountpoint '%s' doesn't start with /", req.Mountpoint)
		http.Error(w, "Mountpoint must start with /", http.StatusBadRequest)
		return
	}

	// Validate external stream URL and type (v1.1.0+)
	if req.ExternalStreamURL != "" {
		// Validate URL with SSRF protection
		if err := h.validator.ValidateExternalStreamURL(req.ExternalStreamURL); err != nil {
			h.logger.Printf("ERROR: Invalid external_stream_url '%s': %v", req.ExternalStreamURL, err)
			http.Error(w, fmt.Sprintf("Invalid external_stream_url: %v", err), http.StatusBadRequest)
			return
		}

		// Validate stream type
		if err := h.validator.ValidateExternalStreamType(req.ExternalStreamType); err != nil {
			h.logger.Printf("ERROR: Invalid external_stream_type '%s': %v", req.ExternalStreamType, err)
			http.Error(w, fmt.Sprintf("Invalid external_stream_type: %v", err), http.StatusBadRequest)
			return
		}

		h.logger.Printf("External stream configured: type=%s, url=%s", req.ExternalStreamType, req.ExternalStreamURL)
	}

	// Verify Proof-of-Work
	challenge := &moderation.Challenge{
		Pubkey:     ownerPubkey,
		Timestamp:  req.PoWTimestamp,
		Difficulty: h.powManager.GetDifficulty(),
	}

	if !h.powManager.Verify(challenge, req.PoWNonce) {
		h.logger.Printf("ERROR: Invalid Proof-of-Work - Pubkey: %s, Nonce: %s, Timestamp: %d",
			ownerPubkey, req.PoWNonce, req.PoWTimestamp)
		http.Error(w, "Invalid Proof-of-Work", http.StatusBadRequest)
		return
	}

	// Create station
	station := &models.Station{
		Name:        req.Name,
		Mountpoint:  req.Mountpoint,
		OwnerPubkey: ownerPubkey,
		ContentType: req.ContentType,
		IsPrivate:   req.IsPrivate,
		PoWHash:     fmt.Sprintf("%s:%d:%s", ownerPubkey, req.PoWTimestamp, req.PoWNonce),
	}

	if req.Description != "" {
		station.Description = sql.NullString{String: req.Description, Valid: true}
	}

	if req.Bitrate != nil {
		station.Bitrate = sql.NullInt64{Int64: *req.Bitrate, Valid: true}
	}

	// Add external stream fields (v1.1.0+)
	if req.ExternalStreamURL != "" {
		station.ExternalStreamURL = sql.NullString{String: req.ExternalStreamURL, Valid: true}
		station.ExternalStreamType = sql.NullString{String: req.ExternalStreamType, Valid: true}
	}

	// Save to database
	if err := h.stationRepo.Create(station); err != nil {
		h.logger.Printf("ERROR: Failed to create station: %v", err)
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "Station with this mountpoint already exists", http.StatusConflict)
		} else {
			http.Error(w, "Failed to create station", http.StatusInternalServerError)
		}
		return
	}

	h.logger.Printf("Station created: %s (mountpoint: %s) by %s (auth: %s)", station.Name, station.Mountpoint, ownerPubkey, authMethod)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         station.ID,
		"uuid":       station.UUID,
		"name":       station.Name,
		"mountpoint": station.Mountpoint,
		"status":     "created",
	})
}

// UpdateStation updates an existing station
func (h *StationHandler) UpdateStation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Get existing station
	station, err := h.stationRepo.GetByID(id)
	if err != nil {
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if station.OwnerPubkey != ownerPubkey {
		http.Error(w, "Forbidden: you don't own this station", http.StatusForbidden)
		return
	}

	// Parse update request
	var req struct {
		Name               *string `json:"name"`
		Description        *string `json:"description"`
		IsPrivate          *bool   `json:"is_private"`
		ExternalStreamURL  *string `json:"external_stream_url"`  // v1.1.0+
		ExternalStreamType *string `json:"external_stream_type"` // v1.1.0+
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update fields if provided
	if req.Name != nil {
		name := h.sanitizer.SanitizeString(*req.Name)
		if err := h.validator.ValidateName(name); err != nil {
			http.Error(w, fmt.Sprintf("Invalid name: %v", err), http.StatusBadRequest)
			return
		}
		station.Name = name
	}

	if req.Description != nil {
		desc := h.sanitizer.SanitizeString(*req.Description)
		if desc != "" {
			if err := h.validator.ValidateDescription(desc); err != nil {
				http.Error(w, fmt.Sprintf("Invalid description: %v", err), http.StatusBadRequest)
				return
			}
			station.Description = sql.NullString{String: desc, Valid: true}
		} else {
			station.Description = sql.NullString{Valid: false}
		}
	}

	if req.IsPrivate != nil {
		station.IsPrivate = *req.IsPrivate
	}

	// Update external stream fields (v1.1.0+)
	if req.ExternalStreamURL != nil {
		url := strings.TrimSpace(*req.ExternalStreamURL)
		if url != "" {
			// Validate URL with SSRF protection
			if err := h.validator.ValidateExternalStreamURL(url); err != nil {
				http.Error(w, fmt.Sprintf("Invalid external_stream_url: %v", err), http.StatusBadRequest)
				return
			}

			// Validate stream type (required when URL is set)
			if req.ExternalStreamType == nil || *req.ExternalStreamType == "" {
				http.Error(w, "external_stream_type required when external_stream_url is set", http.StatusBadRequest)
				return
			}

			streamType := strings.ToLower(strings.TrimSpace(*req.ExternalStreamType))
			if err := h.validator.ValidateExternalStreamType(streamType); err != nil {
				http.Error(w, fmt.Sprintf("Invalid external_stream_type: %v", err), http.StatusBadRequest)
				return
			}

			station.ExternalStreamURL = sql.NullString{String: url, Valid: true}
			station.ExternalStreamType = sql.NullString{String: streamType, Valid: true}
			h.logger.Printf("Updated external stream: type=%s, url=%s", streamType, url)
		} else {
			// Empty URL - clear external stream settings
			station.ExternalStreamURL = sql.NullString{Valid: false}
			station.ExternalStreamType = sql.NullString{Valid: false}
			h.logger.Printf("Cleared external stream settings for station %d", station.ID)
		}
	}

	// Save changes
	if err := h.stationRepo.Update(station); err != nil {
		h.logger.Printf("ERROR: Failed to update station: %v", err)
		http.Error(w, "Failed to update station", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Station updated successfully",
	})
}

// DeleteStation deletes a station
func (h *StationHandler) DeleteStation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Get existing station
	station, err := h.stationRepo.GetByID(id)
	if err != nil {
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if station.OwnerPubkey != ownerPubkey {
		http.Error(w, "Forbidden: you don't own this station", http.StatusForbidden)
		return
	}

	// Delete station
	if err := h.stationRepo.Delete(id); err != nil {
		h.logger.Printf("ERROR: Failed to delete station: %v", err)
		http.Error(w, "Failed to delete station", http.StatusInternalServerError)
		return
	}

	h.logger.Printf("Station deleted: ID=%d by %s", id, ownerPubkey)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Station deleted successfully",
	})
}

// GetStreamingCredentials returns streaming credentials for station owner
func (h *StationHandler) GetStreamingCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Get station
	station, err := h.stationRepo.GetByID(id)
	if err != nil {
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if station.OwnerPubkey != ownerPubkey {
		http.Error(w, "Forbidden: you don't own this station", http.StatusForbidden)
		return
	}

	// Generate source password using same algorithm as streaming server
	password := h.generateSourcePassword(station.OwnerPubkey, station.Mountpoint)

	// Build streaming URLs
	streamURL := fmt.Sprintf("http://[yggdrasil-address]:port%s", station.Mountpoint)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"username":    "source",
		"password":    password,
		"mountpoint":  station.Mountpoint,
		"stream_url":  streamURL,
		"method":      "PUT",
	})
}

// StopBroadcast forcefully stops a station's broadcast
func (h *StationHandler) StopBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Get station
	station, err := h.stationRepo.GetByID(id)
	if err != nil {
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if station.OwnerPubkey != ownerPubkey {
		http.Error(w, "Forbidden: you don't own this station", http.StatusForbidden)
		return
	}

	// Stop broadcast (different handling for external vs normal stations)
	if h.streamingServer != nil {
		// Check if this is an external stream (HLS or Direct)
		if station.ExternalStreamURL.Valid && station.ExternalStreamURL.String != "" {
			// Stop external stream (HLS or Direct)
			h.streamingServer.StopExternalStream(station.Mountpoint)
			h.logger.Printf("External stream stopped for station %s by owner %s", station.Mountpoint, ownerPubkey)

			// Update station status to offline immediately in DB
			if err := h.stationRepo.UpdateStatus(station.Mountpoint, "offline"); err != nil {
				h.logger.Printf("WARNING: Failed to update station status to offline: %v", err)
			}

			// Disable auto_start to prevent monitor from restarting the stream
			if err := h.stationRepo.UpdateAutoStart(station.Mountpoint, false); err != nil {
				h.logger.Printf("WARNING: Failed to update auto_start flag: %v", err)
			}
		} else {
			// Normal station - disconnect source client
			if err := h.streamingServer.DisconnectSource(station.Mountpoint); err != nil {
				h.logger.Printf("ERROR: Failed to disconnect source for %s: %v", station.Mountpoint, err)
				http.Error(w, "Failed to stop broadcast", http.StatusInternalServerError)
				return
			}
			h.logger.Printf("Broadcast stopped for station %s by owner %s", station.Mountpoint, ownerPubkey)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Broadcast stopped successfully",
	})
}

// StartBroadcast starts or resumes a station's broadcast (external streams only)
func (h *StationHandler) StartBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Get station
	station, err := h.stationRepo.GetByID(id)
	if err != nil {
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if station.OwnerPubkey != ownerPubkey {
		http.Error(w, "Forbidden: you don't own this station", http.StatusForbidden)
		return
	}

	// Only works for external streams
	if !station.ExternalStreamURL.Valid || station.ExternalStreamURL.String == "" {
		http.Error(w, "This endpoint only works for external streams (HLS/Direct)", http.StatusBadRequest)
		return
	}

	// Start external stream
	if h.streamingServer != nil {
		if err := h.streamingServer.StartExternalStream(station.Mountpoint); err != nil {
			h.logger.Printf("ERROR: Failed to start external stream for %s: %v", station.Mountpoint, err)
			http.Error(w, fmt.Sprintf("Failed to start broadcast: %v", err), http.StatusInternalServerError)
			return
		}

		// Enable auto_start so monitor will keep the stream running
		if err := h.stationRepo.UpdateAutoStart(station.Mountpoint, true); err != nil {
			h.logger.Printf("WARNING: Failed to update auto_start flag: %v", err)
		}
	}

	h.logger.Printf("External stream started for station %s by owner %s", station.Mountpoint, ownerPubkey)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Broadcast started successfully",
	})
}

// generateSourcePassword generates source password (same as streaming server)
func (h *StationHandler) generateSourcePassword(ownerPubkey, mountpoint string) string {
	secret := []byte(h.serverSecret)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ownerPubkey + ":" + mountpoint))
	hash := mac.Sum(nil)
	return base64.URLEncoding.EncodeToString(hash)[:24]
}

// UpdatePlaylistConfig updates the playlist configuration for a station
func (h *StationHandler) UpdatePlaylistConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL path
	// URL format: /api/stations/{id}/playlist-config
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Get existing station
	station, err := h.stationRepo.GetByID(id)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get station: %v", err)
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if station.OwnerPubkey != ownerPubkey {
		http.Error(w, "Forbidden: you don't own this station", http.StatusForbidden)
		return
	}

	// Parse request body
	var req struct {
		PlaylistEnabled     bool   `json:"playlist_enabled"`
		PlaylistDirectory   string `json:"playlist_directory"`
		PlaylistMode        string `json:"playlist_mode"`
		PlaylistLoop        bool   `json:"playlist_loop"`
		PlaylistFilePattern string `json:"playlist_file_pattern"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// SECURITY FIX: Use dedicated validators to prevent path traversal attacks
	// Validate playlist mode
	if err := h.validator.ValidatePlaylistMode(req.PlaylistMode); err != nil {
		http.Error(w, fmt.Sprintf("Invalid playlist mode: %v", err), http.StatusBadRequest)
		return
	}

	// SECURITY: Validate directory path against path traversal attacks
	// This prevents attackers from accessing files outside intended directories
	if req.PlaylistDirectory != "" {
		if err := h.validator.ValidateDirectoryPath(req.PlaylistDirectory); err != nil {
			http.Error(w, fmt.Sprintf("Invalid directory path: %v", err), http.StatusBadRequest)
			return
		}
	}

	// SECURITY: Validate file pattern to prevent malicious glob patterns
	if req.PlaylistFilePattern != "" {
		if err := h.validator.ValidateFilePattern(req.PlaylistFilePattern); err != nil {
			http.Error(w, fmt.Sprintf("Invalid file pattern: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Update station playlist fields
	station.PlaylistEnabled = req.PlaylistEnabled

	if req.PlaylistDirectory != "" {
		station.PlaylistDirectory = sql.NullString{String: req.PlaylistDirectory, Valid: true}
	} else {
		station.PlaylistDirectory = sql.NullString{Valid: false}
	}

	if req.PlaylistMode != "" {
		station.PlaylistMode = sql.NullString{String: req.PlaylistMode, Valid: true}
	} else {
		station.PlaylistMode = sql.NullString{Valid: false}
	}

	station.PlaylistLoop = req.PlaylistLoop

	if req.PlaylistFilePattern != "" {
		station.PlaylistFilePattern = sql.NullString{String: req.PlaylistFilePattern, Valid: true}
	} else {
		station.PlaylistFilePattern = sql.NullString{Valid: false}
	}

	// Save to database
	if err := h.stationRepo.UpdatePlaylistConfig(station); err != nil {
		h.logger.Printf("ERROR: Failed to update playlist config: %v", err)
		http.Error(w, "Failed to update playlist configuration", http.StatusInternalServerError)
		return
	}

	h.logger.Printf("Playlist config updated for station %d by %s", id, ownerPubkey)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Playlist configuration updated successfully",
	})
}

// GetPlaylistScript generates and returns a playlist streaming script
func (h *StationHandler) GetPlaylistScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var ownerPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get ownerPubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			ownerPubkey = pubkey
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		ownerPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL path
	// URL format: /api/stations/{id}/playlist-script?type={bash|powershell|systemd}
	path := strings.TrimPrefix(r.URL.Path, "/api/stations/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Get script type from query parameter
	scriptType := r.URL.Query().Get("type")
	if scriptType == "" {
		scriptType = "bash" // Default
	}

	// Validate script type
	if scriptType != "bash" && scriptType != "powershell" && scriptType != "systemd" {
		http.Error(w, "Invalid script type: must be 'bash', 'powershell', or 'systemd'", http.StatusBadRequest)
		return
	}

	// Get station
	station, err := h.stationRepo.GetByID(id)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get station: %v", err)
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}

	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if station.OwnerPubkey != ownerPubkey {
		http.Error(w, "Forbidden: you don't own this station", http.StatusForbidden)
		return
	}

	// Check if playlist is enabled
	if !station.PlaylistEnabled {
		http.Error(w, "Playlist streaming is not enabled for this station", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if !station.PlaylistDirectory.Valid || station.PlaylistDirectory.String == "" {
		http.Error(w, "Playlist directory not configured", http.StatusBadRequest)
		return
	}

	// Generate source password
	password := h.generateSourcePassword(station.OwnerPubkey, station.Mountpoint)

	// Build stream URL
	streamURL := fmt.Sprintf("%s%s", h.serverURL, station.Mountpoint)

	// Get playlist mode (default to sequential)
	mode := "sequential"
	if station.PlaylistMode.Valid {
		mode = station.PlaylistMode.String
	}

	// Get file pattern (default to all common audio formats)
	filePattern := "*.{mp3,ogg,opus,flac,m4a,aac,wav,wma,ape}"
	if station.PlaylistFilePattern.Valid && station.PlaylistFilePattern.String != "" {
		filePattern = station.PlaylistFilePattern.String
	}

	// Get bitrate (default to 128)
	bitrate := 128
	if station.Bitrate.Valid {
		bitrate = int(station.Bitrate.Int64)
	}

	// Create generator
	generator := streaming.NewPlaylistScriptGenerator(h.serverURL)

	var script string
	var filename string

	switch scriptType {
	case "bash":
		config := streaming.BashScriptConfig{
			PlaylistDir: station.PlaylistDirectory.String,
			FilePattern: filePattern,
			Mode:        mode,
			Loop:        station.PlaylistLoop,
			StreamURL:   streamURL,
			Username:    "source",
			Password:    password,
			Bitrate:     bitrate,
		}
		script, err = generator.GenerateBashScript(config)
		filename = "yggradio-stream.sh"

	case "powershell":
		config := streaming.PowerShellScriptConfig{
			PlaylistDir: station.PlaylistDirectory.String,
			FilePattern: filePattern,
			Mode:        mode,
			Loop:        station.PlaylistLoop,
			StreamURL:   streamURL,
			Username:    "source",
			Password:    password,
			Bitrate:     bitrate,
		}
		script, err = generator.GeneratePowerShellScript(config)
		filename = "yggradio-stream.ps1"

	case "systemd":
		// For systemd, generate a unit file
		serviceName := "yggradio-stream"
		config := streaming.SystemdUnitConfig{
			ServiceName: serviceName,
			Description: fmt.Sprintf("YggRadio Playlist Stream: %s", station.Name),
			ScriptPath:  "/usr/local/bin/yggradio-stream.sh",
			WorkingDir:  station.PlaylistDirectory.String,
		}
		script, err = generator.GenerateSystemdUnit(config)
		filename = "yggradio-stream.service"
	}

	if err != nil {
		h.logger.Printf("ERROR: Failed to generate %s script: %v", scriptType, err)
		http.Error(w, "Failed to generate script", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Return script
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))

	h.logger.Printf("Generated %s script for station %d (owner: %s)", scriptType, id, ownerPubkey)
}
