package federation_client

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// metadataPollingWorker periodically polls remote federated stations for metadata updates
func (c *Client) metadataPollingWorker() {
	defer c.wg.Done()

	// Poll every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Println("Metadata polling worker stopped")
			return
		case <-ticker.C:
			if err := c.pollAllMetadata(); err != nil {
				// Log error but continue (non-critical)
				c.logger.Printf("WARNING: Metadata polling failed: %v", err)
			}
		}
	}
}

// pollAllMetadata polls metadata for all cached federated stations
func (c *Client) pollAllMetadata() error {
	// Get all cached federated stations
	stations, err := c.db.ListFederatedStationCache()
	if err != nil {
		return fmt.Errorf("failed to list federated stations: %w", err)
	}

	// Poll metadata for each station (in parallel, but with limited concurrency)
	for _, station := range stations {
		// Skip stations without address or mountpoint
		if !station.SourceNodeAddress.Valid || station.Mountpoint == "" {
			continue
		}

		// Skip offline stations
		if station.Status != "online" {
			continue
		}

		// Poll metadata asynchronously (don't block on single failures)
		go c.pollStationMetadata(station.SourceNodeAddress.String, station.Mountpoint)
	}

	return nil
}

// pollStationMetadata polls metadata from a single remote station
func (c *Client) pollStationMetadata(nodeAddress, mountpoint string) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	// Build stream URL using server port from config
	streamURL := fmt.Sprintf("http://[%s]:%d%s", nodeAddress, c.serverPort, mountpoint)

	// Create HEAD request to get metadata without downloading stream
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, streamURL, nil)
	if err != nil {
		c.logger.Printf("DEBUG: Failed to create metadata request for %s%s: %v", nodeAddress, mountpoint, err)
		return
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Don't log errors - stations may be temporarily offline
		return
	}
	defer resp.Body.Close()

	// Check if stream is available
	if resp.StatusCode != http.StatusOK {
		return
	}

	// Extract metadata from ICY headers
	icyName := resp.Header.Get("icy-name")
	icyDescription := resp.Header.Get("icy-description")

	// Use icy-name as metadata title, fallback to icy-description
	metadata := icyName
	if metadata == "" {
		metadata = icyDescription
	}

	// If no ICY metadata, try to extract from response
	if metadata == "" {
		return // No metadata to update
	}

	// Update metadata in cache (suppress errors for non-existent stations)
	if err := c.db.UpdateFederatedStationMetadata(nodeAddress, mountpoint, metadata); err != nil {
		// Suppress "no station found" errors (station may have been removed from cache)
		// Other errors are logged
		if err.Error() != fmt.Sprintf("no federated station found with address %s and mountpoint %s", nodeAddress, mountpoint) {
			c.logger.Printf("DEBUG: Failed to update metadata for %s%s: %v", nodeAddress, mountpoint, err)
		}
	}
}
