package federation_server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// APIHandlers contains HTTP handlers for the federation server API
type APIHandlers struct {
	db          *DB
	config      *Config
	logger      *log.Logger
	nodeManager *NodeManager
	startTime   time.Time
	version     string
}

// NewAPIHandlers creates a new API handlers instance
func NewAPIHandlers(
	db *DB,
	config *Config,
	logger *log.Logger,
	nodeManager *NodeManager,
	version string,
) *APIHandlers {
	return &APIHandlers{
		db:          db,
		config:      config,
		logger:      logger,
		nodeManager: nodeManager,
		startTime:   time.Now(),
		version:     version,
	}
}

// HandleRegister handles node registration requests
// POST /api/federation/register
func (h *APIHandlers) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Printf("Registration request parse error: %v", err)
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get client IP for logging
	clientIP := getClientIP(r)

	// Process registration
	node, err := h.nodeManager.RegisterNode(&req)
	if err != nil {
		h.logger.Printf("Registration failed from %s: %v", clientIP, err)
		// Don't leak detailed error to client
		h.sendError(w, "Registration failed", http.StatusBadRequest)
		return
	}

	// Send success response
	resp := RegistrationResponse{
		Success: true,
		Message: "Node registered successfully",
		NodeID:  node.UUID,
	}

	h.sendJSON(w, resp, http.StatusOK)
}

// HandleStations handles federated stations query requests
// GET /api/stations
func (h *APIHandlers) HandleStations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get filter parameters from query string
	filterOnline := r.URL.Query().Get("status") == "online"

	// Retrieve stations from database
	var stations []*Station
	var err error

	if filterOnline {
		stations, err = h.db.GetOnlineStations()
	} else {
		stations, err = h.db.GetAllStations()
	}

	if err != nil {
		h.logger.Printf("Failed to retrieve stations: %v", err)
		h.sendError(w, "Failed to retrieve stations", http.StatusInternalServerError)
		return
	}

	// Get node information for enrichment
	nodes, err := h.db.GetAllNodes()
	if err != nil {
		h.logger.Printf("Failed to retrieve nodes: %v", err)
		// Continue with empty node list - will show stations without node names
		nodes = []*Node{}
	}

	// Create node lookup map
	nodeMap := make(map[string]*Node)
	for _, node := range nodes {
		nodeMap[node.UUID] = node
	}

	// Convert to public format
	publicStations := make([]StationPublic, 0, len(stations))
	for _, station := range stations {
		nodeName := ""
		nodeAddress := ""
		nodePort := 8080 // Default port
		if node, ok := nodeMap[station.NodeUUID]; ok {
			nodeName = node.Name
			nodeAddress = node.Address
			nodePort = node.Port
		}

		publicStation := StationPublic{
			UUID:           station.UUID,
			NodeUUID:       station.NodeUUID,
			NodeAddress:    nodeAddress,
			NodePort:       nodePort,
			NodeName:       nodeName,
			Name:           station.Name,
			Description:    station.Description.String,
			Mountpoint:     station.Mountpoint,
			OwnerPubkey:    station.OwnerPubkey,
			Status:         station.Status,
			ListenersCount: station.ListenersCount,
			ContentType:    station.ContentType.String,
			Bitrate:        int(station.Bitrate.Int64),
			Genre:          station.Genre.String,
			MetadataTitle:  station.MetadataTitle.String,
			AverageRating:  station.AverageRating,
			VoteCount:      station.VoteCount,
			LastSeen:       station.LastSeen.Format(time.RFC3339),
		}

		publicStations = append(publicStations, publicStation)
	}

	// Send response
	resp := StationListResponse{
		Success:  true,
		Stations: publicStations,
		Count:    len(publicStations),
		Metadata: ResponseMetadata{
			Timestamp: time.Now().Format(time.RFC3339),
			Version:   h.version,
		},
	}

	h.sendJSON(w, resp, http.StatusOK)
}

// HandleNodes handles registered nodes query requests
// GET /api/nodes
func (h *APIHandlers) HandleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Retrieve nodes from database
	nodes, err := h.db.GetAllNodes()
	if err != nil {
		h.logger.Printf("Failed to retrieve nodes: %v", err)
		h.sendError(w, "Failed to retrieve nodes", http.StatusInternalServerError)
		return
	}

	// Convert to public format
	publicNodes := make([]NodePublic, 0, len(nodes))
	for _, node := range nodes {
		publicNode := NodePublic{
			UUID:            node.UUID,
			Address:         node.Address,
			Port:            node.Port,
			Name:            node.Name,
			Description:     node.Description.String,
			Version:         node.Version.String,
			Status:          node.Status,
			TotalStations:   node.TotalStations,
			LastSeen:        node.LastSeen.Format(time.RFC3339),
			FirstRegistered: node.FirstRegistered.Format(time.RFC3339),
		}

		publicNodes = append(publicNodes, publicNode)
	}

	// Send response
	resp := NodeListResponse{
		Success: true,
		Nodes:   publicNodes,
		Count:   len(publicNodes),
		Metadata: ResponseMetadata{
			Timestamp: time.Now().Format(time.RFC3339),
			Version:   h.version,
		},
	}

	h.sendJSON(w, resp, http.StatusOK)
}

// HandleHealth handles health check requests
// GET /api/health
func (h *APIHandlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get database statistics
	stats, err := h.db.GetStats()
	if err != nil {
		h.logger.Printf("Failed to get stats: %v", err)
		stats = make(map[string]int)
	}

	// Calculate uptime
	uptime := time.Since(h.startTime)

	// Send response
	resp := HealthResponse{
		Status:  "healthy",
		Version: h.version,
		Uptime:  uptime.String(),
		Stats:   stats,
	}

	h.sendJSON(w, resp, http.StatusOK)
}

// === Helper Methods ===

// sendJSON sends a JSON response
func (h *APIHandlers) sendJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Printf("Failed to encode JSON response: %v", err)
	}
}

// sendError sends an error response
func (h *APIHandlers) sendError(w http.ResponseWriter, message string, statusCode int) {
	resp := ErrorResponse{
		Success: false,
		Error:   message,
	}

	h.sendJSON(w, resp, statusCode)
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For header first (if behind proxy)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}

	// Try X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// logRequest logs an incoming HTTP request
func (h *APIHandlers) logRequest(r *http.Request) {
	h.logger.Printf("%s %s from %s", r.Method, r.URL.Path, getClientIP(r))
}

// HandleNotFound handles 404 errors
func (h *APIHandlers) HandleNotFound(w http.ResponseWriter, r *http.Request) {
	h.logger.Printf("404 Not Found: %s %s from %s", r.Method, r.URL.Path, getClientIP(r))
	h.sendError(w, "Not found", http.StatusNotFound)
}

// HandleMethodNotAllowed handles 405 errors
func (h *APIHandlers) HandleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	h.logger.Printf("405 Method Not Allowed: %s %s from %s", r.Method, r.URL.Path, getClientIP(r))
	h.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// HandleRoot handles root path requests
func (h *APIHandlers) HandleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		h.HandleNotFound(w, r)
		return
	}

	response := map[string]interface{}{
		"service":     "YggRadio Federation Server",
		"version":     h.version,
		"instance":    h.config.Server.InstanceName,
		"uptime":      time.Since(h.startTime).String(),
		"endpoints": []string{
			"POST /api/federation/register - Node registration",
			"GET /api/stations - Query federated stations",
			"GET /api/nodes - List registered nodes",
			"GET /api/health - Health check",
		},
	}

	h.sendJSON(w, response, http.StatusOK)
}

// ValidateStationQuery validates query parameters for station listing
func (h *APIHandlers) ValidateStationQuery(r *http.Request) error {
	// Validate status parameter if provided
	status := r.URL.Query().Get("status")
	if status != "" && status != "online" && status != "offline" {
		return fmt.Errorf("invalid status parameter: must be 'online' or 'offline'")
	}

	// Add more query parameter validations as needed

	return nil
}
