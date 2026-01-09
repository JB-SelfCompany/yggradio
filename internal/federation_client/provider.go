package federation_client

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/database/models"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// ProviderHandler handles the station provider endpoint for federation server
type ProviderHandler struct {
	db            *sql.DB
	stationRepo   *models.StationRepository
	ratingRepo    *models.RatingRepository
	validator     *security.Validator
	crypto        *security.CryptoUtil
	localPubkey   []byte
	localPrivkey  []byte
	logger        *log.Logger
}

// NewProviderHandler creates a new provider handler
func NewProviderHandler(
	db *sql.DB,
	stationRepo *models.StationRepository,
	ratingRepo *models.RatingRepository,
	validator *security.Validator,
	crypto *security.CryptoUtil,
	localPubkey []byte,
	localPrivkey []byte,
	logger *log.Logger,
) *ProviderHandler {
	return &ProviderHandler{
		db:           db,
		stationRepo:  stationRepo,
		ratingRepo:   ratingRepo,
		validator:    validator,
		crypto:       crypto,
		localPubkey:  localPubkey,
		localPrivkey: localPrivkey,
		logger:       logger,
	}
}

// StationProviderResponse represents a station in the provider response
type StationProviderResponse struct {
	UUID           string  `json:"uuid"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Mountpoint     string  `json:"mountpoint"`
	OwnerPubkey    string  `json:"owner_pubkey"`
	Status         string  `json:"status"`
	ListenersCount int     `json:"listeners_count"`
	ContentType    string  `json:"content_type"`
	Bitrate        int     `json:"bitrate"`
	Genre          string  `json:"genre"`
	MetadataTitle  string  `json:"metadata_title"`
	AverageRating  float64 `json:"average_rating"`
	VoteCount      int     `json:"vote_count"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// ProviderAPIResponse represents the full response with signature
type ProviderAPIResponse struct {
	Success   bool                       `json:"success"`
	Stations  []StationProviderResponse  `json:"stations"`
	Count     int                        `json:"count"`
	Pubkey    string                     `json:"pubkey"`
	Signature string                     `json:"signature"`
	Timestamp int64                      `json:"timestamp"`
}

// ServeHTTP handles GET /api/federation/stations - returns public stations with ratings
func (h *ProviderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all public stations (is_private = false)
	stations, err := h.stationRepo.ListPublic(100, 0) // Limit 100 for federation
	if err != nil {
		h.logger.Printf("ERROR: Failed to list public stations: %v", err)
		http.Error(w, "Failed to retrieve stations", http.StatusInternalServerError)
		return
	}

	// Build stations list with ratings
	stationsList := make([]StationProviderResponse, 0, len(stations))
	for _, station := range stations {
		// Get rating stats for this station
		stats, err := h.ratingRepo.GetStationRatings(station.ID, "")
		if err != nil {
			h.logger.Printf("WARNING: Failed to get ratings for station %d: %v", station.ID, err)
			// Continue with zero ratings instead of failing
			stats = &models.RatingStats{
				AverageRating: 0,
				VoteCount:     0,
			}
		}

		// Build response item
		item := StationProviderResponse{
			UUID:           station.UUID,
			Name:           station.Name,
			Description:    station.Description.String,
			Mountpoint:     station.Mountpoint,
			OwnerPubkey:    station.OwnerPubkey,
			Status:         station.Status,
			ListenersCount: station.ListenersCount,
			ContentType:    station.ContentType,
			Bitrate:        int(station.Bitrate.Int64),
			Genre:          station.Genre.String,
			MetadataTitle:  station.MetadataTitle.String,
			AverageRating:  stats.AverageRating,
			VoteCount:      stats.VoteCount,
			CreatedAt:      station.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:      station.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}

		stationsList = append(stationsList, item)
	}

	// Create timestamp
	timestamp := time.Now().Unix()

	// Build signature message: COUNT+TIMESTAMP
	signatureMessage := fmt.Sprintf("%d%d", len(stationsList), timestamp)
	signature := h.crypto.SignMessage(h.localPrivkey, signatureMessage)

	// Build full response with signature
	apiResponse := ProviderAPIResponse{
		Success:   true,
		Stations:  stationsList,
		Count:     len(stationsList),
		Pubkey:    hex.EncodeToString(h.localPubkey),
		Signature: signature,
		Timestamp: timestamp,
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(apiResponse); err != nil {
		h.logger.Printf("ERROR: Failed to encode response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	// Successfully provided stations (no logging to reduce noise)
}

// GetStationURL returns the full URL for accessing a station stream
func GetStationURL(serverAddress string, serverPort int, mountpoint string) string {
	return fmt.Sprintf("http://[%s]:%d%s", serverAddress, serverPort, mountpoint)
}
