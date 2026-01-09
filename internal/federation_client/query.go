package federation_client

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/JB-SelfCompany/yggradio/internal/database"
)

// FederatedStationResponse represents a station in the federation server response
type FederatedStationResponse struct {
	UUID           string  `json:"uuid"`
	NodeUUID       string  `json:"node_uuid"`        // Changed from source_node to match server
	NodeAddress    string  `json:"node_address"`     // Yggdrasil IPv6 address
	NodeName       string  `json:"node_name"`        // Changed from source_node_name to match server
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
}

// StationQueryResponse represents the response from GET /api/stations
type StationQueryResponse struct {
	Success  bool                         `json:"success"`
	Stations []FederatedStationResponse   `json:"stations"`
	Count    int                          `json:"count"`
}

// queryStations queries the federation server for all federated stations
func (c *Client) queryStations() error {
	// Send request to federation server (logging removed to reduce noise)
	url := fmt.Sprintf("%s/api/stations", c.GetServerURL())
	httpReq, err := http.NewRequestWithContext(c.ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send query request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("query failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var queryResp StationQueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return fmt.Errorf("failed to parse query response: %w", err)
	}

	if !queryResp.Success {
		return fmt.Errorf("query failed: server returned success=false")
	}

	// Filter out our own stations (to avoid duplication)
	// Use NodeAddress instead of NodeUUID because NodeUUID is randomly generated on each restart
	filteredStations := make([]FederatedStationResponse, 0)
	for _, station := range queryResp.Stations {
		// Skip stations from our own node by comparing Yggdrasil addresses
		if station.NodeAddress == c.localAddress {
			continue
		}
		filteredStations = append(filteredStations, station)
	}

	// Before updating cache, remove stale stations that are no longer in the list
	currentUUIDs := make([]string, len(filteredStations))
	for i, station := range filteredStations {
		currentUUIDs[i] = station.UUID
	}

	// Delete stations not in the current list (prevents duplicates on node restart)
	if err := c.db.DeleteFederatedStationsNotInList(currentUUIDs); err != nil {
		c.logger.Printf("WARNING: Failed to clean up stale stations: %v", err)
		// Continue anyway - not a critical error
	}

	// Update local cache with current stations
	if err := c.updateCache(filteredStations); err != nil {
		return fmt.Errorf("failed to update cache: %w", err)
	}

	// Successfully queried and cached stations (no logging to reduce noise)
	return nil
}

// updateCache updates the local cache with federated stations
func (c *Client) updateCache(stations []FederatedStationResponse) error {
	for _, station := range stations {
		// Convert to database model
		dbStation := &database.FederatedStation{
			UUID:       station.UUID,
			SourceNode: station.NodeUUID, // Map NodeUUID to SourceNode (field names differ)
			SourceNodeAddress: sql.NullString{
				String: station.NodeAddress, // Map NodeAddress to SourceNodeAddress
				Valid:  station.NodeAddress != "",
			},
			SourceNodeName: sql.NullString{
				String: station.NodeName, // Map NodeName to SourceNodeName
				Valid:  station.NodeName != "",
			},
			Name: station.Name,
			Description: sql.NullString{
				String: station.Description,
				Valid:  station.Description != "",
			},
			Mountpoint:     station.Mountpoint,
			OwnerPubkey:    station.OwnerPubkey,
			Status:         station.Status,
			ListenersCount: station.ListenersCount,
			ContentType: sql.NullString{
				String: station.ContentType,
				Valid:  station.ContentType != "",
			},
			Bitrate: sql.NullInt64{
				Int64: int64(station.Bitrate),
				Valid: station.Bitrate > 0,
			},
			Genre: sql.NullString{
				String: station.Genre,
				Valid:  station.Genre != "",
			},
			MetadataTitle: sql.NullString{
				String: station.MetadataTitle,
				Valid:  station.MetadataTitle != "",
			},
			AverageRating: station.AverageRating,
			VoteCount:     station.VoteCount,
		}

		// Upsert into cache (preserve metadata_title to avoid overwriting real-time updates)
		// The proxy and metadata polling worker update metadata_title more frequently,
		// so we preserve existing values when syncing with federation server
		if err := c.db.UpsertFederatedStationCachePreserveMetadata(dbStation); err != nil {
			c.logger.Printf("WARNING: Failed to cache station %s: %v", station.UUID, err)
			continue
		}
	}

	return nil
}
