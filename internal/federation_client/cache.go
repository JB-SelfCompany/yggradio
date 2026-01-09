package federation_client

import (
	"fmt"
	"log"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/database"
)

// CacheManager manages the federated station cache
type CacheManager struct {
	db     *database.DB
	logger *log.Logger
}

// NewCacheManager creates a new cache manager
func NewCacheManager(db *database.DB, logger *log.Logger) (*CacheManager, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &CacheManager{
		db:     db,
		logger: logger,
	}, nil
}

// Get retrieves a cached federated station
func (cm *CacheManager) Get(sourceNode, mountpoint string) (*database.FederatedStation, error) {
	station, err := cm.db.GetFederatedStationCache(sourceNode, mountpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached station: %w", err)
	}
	return station, nil
}

// List retrieves all cached federated stations
func (cm *CacheManager) List() ([]*database.FederatedStation, error) {
	stations, err := cm.db.ListFederatedStationCache()
	if err != nil {
		return nil, fmt.Errorf("failed to list cached stations: %w", err)
	}
	return stations, nil
}

// Upsert inserts or updates a federated station in cache
func (cm *CacheManager) Upsert(station *database.FederatedStation) error {
	if err := cm.db.UpsertFederatedStationCache(station); err != nil {
		return fmt.Errorf("failed to upsert cached station: %w", err)
	}
	return nil
}

// ExpireOldEntries removes cached stations that haven't been updated within the given duration
func (cm *CacheManager) ExpireOldEntries(duration time.Duration) error {
	cutoffTime := time.Now().Add(-duration)

	if err := cm.db.ExpireFederatedStationCache(cutoffTime); err != nil {
		return fmt.Errorf("failed to expire old cache entries: %w", err)
	}

	cm.logger.Printf("Expired cache entries older than %s", duration)
	return nil
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats() (map[string]interface{}, error) {
	stations, err := cm.db.ListFederatedStationCache()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache stats: %w", err)
	}

	// Count by status
	onlineCount := 0
	offlineCount := 0
	for _, station := range stations {
		if station.Status == "online" {
			onlineCount++
		} else {
			offlineCount++
		}
	}

	stats := map[string]interface{}{
		"total_cached":   len(stations),
		"online_stations": onlineCount,
		"offline_stations": offlineCount,
	}

	return stats, nil
}
