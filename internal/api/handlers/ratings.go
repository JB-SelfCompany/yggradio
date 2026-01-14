package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// RatingHandler handles rating-related API endpoints
type RatingHandler struct {
	db            *sql.DB
	dbWrapper     *database.DB // For federated station queries
	ratingRepo    *models.RatingRepository
	stationRepo   *models.StationRepository
	validator     *security.Validator
	crypto        *security.CryptoUtil
	localPubkey   []byte
	localPrivkey  []byte
	logger        *log.Logger
	httpClient    *http.Client
	serverPort    int // Server port for federated requests
}

// NewRatingHandler creates a new rating handler
func NewRatingHandler(
	db *sql.DB,
	dbWrapper *database.DB,
	ratingRepo *models.RatingRepository,
	stationRepo *models.StationRepository,
	validator *security.Validator,
	crypto *security.CryptoUtil,
	localPubkey []byte,
	localPrivkey []byte,
	logger *log.Logger,
	serverPort int,
) *RatingHandler {
	return &RatingHandler{
		db:           db,
		dbWrapper:    dbWrapper,
		ratingRepo:   ratingRepo,
		stationRepo:  stationRepo,
		validator:    validator,
		crypto:       crypto,
		localPubkey:  localPubkey,
		localPrivkey: localPrivkey,
		logger:       logger,
		serverPort:   serverPort,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SubmitRating handles POST /api/stations/{id}/rate
// Requires authentication and CSRF token
func (h *RatingHandler) SubmitRating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var userPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		h.logger.Printf("ERROR: Missing user_id in context for rating submission")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get user pubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			userPubkey = pubkey
		} else {
			h.logger.Printf("ERROR: Ed25519 auth but no pubkey in context")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		// Format: "magiclink:<user_id>" (64 hex chars padded)
		userPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		h.logger.Printf("ERROR: Unknown auth_method for rating: %s", authMethod)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract IPv6 address from request
	userIPv6 := extractIPv6(r.RemoteAddr)
	if userIPv6 == "" {
		h.logger.Printf("ERROR: Failed to extract IPv6 from RemoteAddr: %s", r.RemoteAddr)
		http.Error(w, "Failed to determine client address", http.StatusBadRequest)
		return
	}

	// Extract IPv6 subnet (/64)
	userIPv6Subnet := extractIPv6Subnet(userIPv6)
	if userIPv6Subnet == "" {
		h.logger.Printf("ERROR: Failed to extract subnet from IPv6: %s", userIPv6)
		http.Error(w, "Invalid IPv6 address", http.StatusBadRequest)
		return
	}

	// Extract station ID from URL path
	// URL format: /api/stations/{id}/rate
	stationID, err := h.extractStationID(r.URL.Path)
	if err != nil {
		h.logger.Printf("ERROR: Invalid station ID in path: %v", err)
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Verify station exists
	station, err := h.stationRepo.GetByID(stationID)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get station %d: %v", stationID, err)
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}
	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Check if station owner is trying to rate their own station
	if station.OwnerPubkey == userPubkey {
		h.logger.Printf("SECURITY: Owner %s attempted to rate own station %d", userPubkey, stationID)
		http.Error(w, "You cannot rate your own station", http.StatusForbidden)
		return
	}

	// Parse request body
	var req struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Printf("ERROR: Failed to decode rating request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate rating is in range [1, 5]
	if req.Rating < 1 || req.Rating > 5 {
		h.logger.Printf("ERROR: Invalid rating value %d from user %s", req.Rating, userPubkey)
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	// SPAM PROTECTION: Check for duplicate ratings from same IPv6 address
	isDuplicateIPv6, err := h.ratingRepo.CheckIPv6Duplicate("station", stationID, userPubkey, userIPv6)
	if err != nil {
		h.logger.Printf("ERROR: Failed to check IPv6 duplicate: %v", err)
		http.Error(w, "Failed to validate request", http.StatusInternalServerError)
		return
	}
	if isDuplicateIPv6 {
		h.logger.Printf("SECURITY: Duplicate rating attempt from IPv6 %s for station %d (pubkey %s)", userIPv6, stationID, userPubkey)
		http.Error(w, "You can rate this station only once", http.StatusForbidden)
		return
	}

	// SPAM PROTECTION: Check for duplicate ratings from same subnet
	isDuplicateSubnet, err := h.ratingRepo.CheckSubnetDuplicate("station", stationID, userPubkey, userIPv6Subnet)
	if err != nil {
		h.logger.Printf("ERROR: Failed to check subnet duplicate: %v", err)
		http.Error(w, "Failed to validate request", http.StatusInternalServerError)
		return
	}
	if isDuplicateSubnet {
		h.logger.Printf("SECURITY: Duplicate rating attempt from subnet %s for station %d (pubkey %s)", userIPv6Subnet, stationID, userPubkey)
		http.Error(w, "You can rate this station only once", http.StatusForbidden)
		return
	}

	// Upsert rating (creates new or updates existing)
	if err := h.ratingRepo.UpsertRating("station", stationID, userPubkey, userIPv6, userIPv6Subnet, req.Rating); err != nil {
		h.logger.Printf("ERROR: Failed to upsert rating for station %d by user %s: %v", stationID, userPubkey, err)
		http.Error(w, "Failed to save rating", http.StatusInternalServerError)
		return
	}

	// Get updated statistics
	stats, err := h.ratingRepo.GetStationRatings(stationID, userPubkey)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get updated stats for station %d: %v", stationID, err)
		http.Error(w, "Failed to retrieve updated statistics", http.StatusInternalServerError)
		return
	}

	h.logger.Printf("Rating submitted: station=%d, user=%s, ipv6=%s, subnet=%s, rating=%d", stationID, userPubkey, userIPv6, userIPv6Subnet, req.Rating)

	// Return success with updated stats
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"rating":         req.Rating,
		"average_rating": stats.AverageRating,
		"vote_count":     stats.VoteCount,
	})
}

// GetRatingStats handles GET /api/stations/{id}/ratings
// Public endpoint - no authentication required
func (h *RatingHandler) GetRatingStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract station ID from URL path
	// URL format: /api/stations/{id}/ratings
	stationID, err := h.extractStationID(r.URL.Path)
	if err != nil {
		h.logger.Printf("ERROR: Invalid station ID in path: %v", err)
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Verify station exists
	station, err := h.stationRepo.GetByID(stationID)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get station %d: %v", stationID, err)
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}
	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Get rating statistics
	stats, err := h.ratingRepo.GetRatingStats("station", stationID)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get rating stats for station %d: %v", stationID, err)
		http.Error(w, "Failed to retrieve rating statistics", http.StatusInternalServerError)
		return
	}

	// Return statistics
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"average_rating": stats.AverageRating,
		"vote_count":     stats.VoteCount,
	})
}

// GetMyRating handles GET /api/stations/{id}/my-rating
// Requires authentication
func (h *RatingHandler) GetMyRating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var userPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		h.logger.Printf("ERROR: Missing user_id in context for getting user rating")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get user pubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			userPubkey = pubkey
		} else {
			h.logger.Printf("ERROR: Ed25519 auth but no pubkey in context")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		userPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		h.logger.Printf("ERROR: Unknown auth_method for getting user rating: %s", authMethod)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station ID from URL path
	// URL format: /api/stations/{id}/my-rating
	stationID, err := h.extractStationID(r.URL.Path)
	if err != nil {
		h.logger.Printf("ERROR: Invalid station ID in path: %v", err)
		http.Error(w, "Invalid station ID", http.StatusBadRequest)
		return
	}

	// Verify station exists
	station, err := h.stationRepo.GetByID(stationID)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get station %d: %v", stationID, err)
		http.Error(w, "Failed to retrieve station", http.StatusInternalServerError)
		return
	}
	if station == nil {
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Get user's rating
	userRating, err := h.ratingRepo.GetUserRating("station", stationID, userPubkey)
	if err != nil {
		h.logger.Printf("ERROR: Failed to get user rating for station %d: %v", stationID, err)
		http.Error(w, "Failed to retrieve user rating", http.StatusInternalServerError)
		return
	}

	// Return rating (null if user hasn't rated)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rating": userRating,
	})
}

// extractStationID extracts station ID from URL path
// Handles paths like /api/stations/{id}/rate, /api/stations/{id}/ratings, etc.
func (h *RatingHandler) extractStationID(path string) (int64, error) {
	// Remove prefix /api/stations/
	path = strings.TrimPrefix(path, "/api/stations/")

	// Split by / and get first part (the ID)
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		return 0, http.ErrMissingFile
	}

	// Parse ID
	stationID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	return stationID, nil
}

// SubmitFederatedRating handles POST /api/federated/stations/{uuid}/rate
// Forwards rating to the source node of a federated station
func (h *RatingHandler) SubmitFederatedRating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user - supports both Ed25519 and Magic Link authentication
	var userPubkey string
	var userID int64

	// Try to get user_id first (works for both auth methods)
	if uid, ok := r.Context().Value("user_id").(int64); ok {
		userID = uid
	} else {
		h.logger.Printf("ERROR: Missing user_id in context for federated rating submission")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check auth method to determine how to get user pubkey
	authMethod, _ := r.Context().Value("auth_method").(string)

	if authMethod == "ed25519" {
		// Ed25519 users: use their actual public key
		if pubkey, ok := r.Context().Value("pubkey").(string); ok {
			userPubkey = pubkey
		} else {
			h.logger.Printf("ERROR: Ed25519 auth but no pubkey in context")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else if authMethod == "magic_link" {
		// Magic link users: generate synthetic pubkey from user_id
		userPubkey = fmt.Sprintf("magiclink:%054d", userID)
	} else {
		h.logger.Printf("ERROR: Unknown auth_method for federated rating: %s", authMethod)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract station UUID from URL path
	// URL format: /api/federated/stations/{uuid}/rate
	stationUUID := h.extractFederatedStationUUID(r.URL.Path)
	if stationUUID == "" {
		h.logger.Printf("ERROR: Invalid station UUID in path")
		http.Error(w, "Invalid station UUID", http.StatusBadRequest)
		return
	}

	// Check if dbWrapper is available
	if h.dbWrapper == nil {
		h.logger.Printf("ERROR: Federation not configured")
		http.Error(w, "Federation not configured", http.StatusServiceUnavailable)
		return
	}

	// Find federated station in cache
	var federatedStation *database.FederatedStation
	stations, err := h.dbWrapper.ListFederatedStationCache()
	if err != nil {
		h.logger.Printf("ERROR: Failed to list federated stations: %v", err)
		http.Error(w, "Failed to retrieve federated stations", http.StatusInternalServerError)
		return
	}

	for _, fs := range stations {
		if fs.UUID == stationUUID {
			federatedStation = fs
			break
		}
	}

	if federatedStation == nil {
		http.Error(w, "Federated station not found", http.StatusNotFound)
		return
	}

	// Check if user is trying to rate their own station
	if federatedStation.OwnerPubkey == userPubkey {
		h.logger.Printf("SECURITY: Owner %s attempted to rate own federated station %s", userPubkey, stationUUID)
		http.Error(w, "You cannot rate your own station", http.StatusForbidden)
		return
	}

	// Parse request body
	var req struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Printf("ERROR: Failed to decode federated rating request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate rating is in range [1, 5]
	if req.Rating < 1 || req.Rating > 5 {
		h.logger.Printf("ERROR: Invalid rating value %d from user %s", req.Rating, userPubkey)
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	// Get source node address
	if !federatedStation.SourceNodeAddress.Valid || federatedStation.SourceNodeAddress.String == "" {
		h.logger.Printf("ERROR: Federated station %s has no source node address", stationUUID)
		http.Error(w, "Source node address not available", http.StatusServiceUnavailable)
		return
	}

	nodeAddress := federatedStation.SourceNodeAddress.String

	// Get source node port (use local server port as fallback)
	nodePort := h.serverPort
	if federatedStation.SourceNodePort.Valid && federatedStation.SourceNodePort.Int64 > 0 {
		nodePort = int(federatedStation.SourceNodePort.Int64)
	}

	// Forward rating to source node
	if err := h.forwardRatingToSourceNode(nodeAddress, nodePort, stationUUID, userPubkey, req.Rating); err != nil {
		h.logger.Printf("ERROR: Failed to forward rating to source node %s: %v", nodeAddress, err)
		http.Error(w, "Failed to submit rating to source node", http.StatusBadGateway)
		return
	}

	h.logger.Printf("Federated rating forwarded: station=%s, user=%s, node=%s, rating=%d", stationUUID, userPubkey, nodeAddress, req.Rating)

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Rating submitted to source node successfully",
		"rating":  req.Rating,
	})
}

// forwardRatingToSourceNode sends rating to the source node via HTTP
func (h *RatingHandler) forwardRatingToSourceNode(nodeAddress string, nodePort int, stationUUID, voterPubkey string, rating int) error {
	// Build request URL
	url := fmt.Sprintf("http://[%s]:%d/api/federation/ratings", nodeAddress, nodePort)

	// Create timestamp for signature
	timestamp := time.Now().Unix()

	// Build signature message: UUID+PUBKEY+RATING+TIMESTAMP
	signatureMessage := fmt.Sprintf("%s%s%d%d", stationUUID, voterPubkey, rating, timestamp)
	signature := h.crypto.SignMessage(h.localPrivkey, signatureMessage)

	// Build request payload
	payload := map[string]interface{}{
		"station_uuid": stationUUID,
		"voter_pubkey": voterPubkey,
		"rating":       rating,
		"from_node":    hex.EncodeToString(h.localPubkey),
		"signature":    signature,
		"timestamp":    timestamp,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HTTP request with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("source node returned status %d", resp.StatusCode)
	}

	return nil
}

// extractFederatedStationUUID extracts station UUID from URL path
// Handles paths like /api/federated/stations/{uuid}/rate
func (h *RatingHandler) extractFederatedStationUUID(path string) string {
	// Remove prefix /api/federated/stations/
	path = strings.TrimPrefix(path, "/api/federated/stations/")

	// Split by / and get first part (the UUID)
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		return ""
	}

	return parts[0]
}

// ReceiveFederatedRating handles POST /api/federation/ratings
// Receives ratings from other federated nodes and saves them locally
func (h *RatingHandler) ReceiveFederatedRating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		StationUUID string `json:"station_uuid"`
		VoterPubkey string `json:"voter_pubkey"`
		Rating      int    `json:"rating"`
		FromNode    string `json:"from_node"`
		Signature   string `json:"signature"`
		Timestamp   int64  `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Printf("ERROR: Failed to decode federated rating request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate timestamp (within 5 minutes)
	now := time.Now().Unix()
	if now-req.Timestamp > 300 || now-req.Timestamp < -300 {
		h.logger.Printf("SECURITY: Federated rating rejected - timestamp out of range: %d", req.Timestamp)
		http.Error(w, "Invalid timestamp", http.StatusBadRequest)
		return
	}

	// Validate signature
	signatureMessage := fmt.Sprintf("%s%s%d%d", req.StationUUID, req.VoterPubkey, req.Rating, req.Timestamp)
	if !h.crypto.VerifySignature(req.FromNode, signatureMessage, req.Signature) {
		h.logger.Printf("SECURITY: Invalid signature for federated rating from node %s", req.FromNode)
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Validate rating range
	if req.Rating < 1 || req.Rating > 5 {
		h.logger.Printf("ERROR: Invalid rating value %d", req.Rating)
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	// Find local station by UUID
	stations, err := h.stationRepo.List(1000, 0) // Get all stations
	if err != nil {
		h.logger.Printf("ERROR: Failed to list stations: %v", err)
		http.Error(w, "Failed to retrieve stations", http.StatusInternalServerError)
		return
	}

	var targetStation *models.Station
	for _, station := range stations {
		if station.UUID == req.StationUUID {
			targetStation = station
			break
		}
	}

	if targetStation == nil {
		h.logger.Printf("ERROR: Station with UUID %s not found", req.StationUUID)
		http.Error(w, "Station not found", http.StatusNotFound)
		return
	}

	// Check if voter is trying to rate their own station
	if targetStation.OwnerPubkey == req.VoterPubkey {
		h.logger.Printf("SECURITY: Federated rating rejected - owner cannot rate own station: %s", req.VoterPubkey)
		http.Error(w, "Owner cannot rate own station", http.StatusForbidden)
		return
	}

	// Extract IPv6 address from remote node (use from_node as placeholder for IPv6)
	// In federated context, we use the source node's pubkey as identifier
	userIPv6 := req.FromNode // Use node pubkey as "IPv6" for federation
	userIPv6Subnet := req.FromNode[:16] // Use prefix as "subnet"

	// Check for duplicate ratings (same voter + station)
	isDuplicate, err := h.ratingRepo.CheckIPv6Duplicate("station", targetStation.ID, req.VoterPubkey, userIPv6)
	if err != nil {
		h.logger.Printf("ERROR: Failed to check duplicate: %v", err)
		http.Error(w, "Failed to validate request", http.StatusInternalServerError)
		return
	}
	if isDuplicate {
		h.logger.Printf("INFO: Duplicate federated rating from %s for station %d - updating", req.VoterPubkey, targetStation.ID)
		// Allow update instead of rejecting
	}

	// Save rating locally
	if err := h.ratingRepo.UpsertRating("station", targetStation.ID, req.VoterPubkey, userIPv6, userIPv6Subnet, req.Rating); err != nil {
		h.logger.Printf("ERROR: Failed to save federated rating: %v", err)
		http.Error(w, "Failed to save rating", http.StatusInternalServerError)
		return
	}

	h.logger.Printf("Federated rating received and saved: station=%s (ID=%d), voter=%s, from_node=%s, rating=%d",
		req.StationUUID, targetStation.ID, req.VoterPubkey, req.FromNode, req.Rating)

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Rating received and saved successfully",
	})
}
