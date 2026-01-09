package federation_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// StationAggregator pulls station data from registered nodes
type StationAggregator struct {
	db         *DB
	config     *Config
	logger     *log.Logger
	httpClient *http.Client
}

// NewStationAggregator creates a new station aggregator
func NewStationAggregator(db *DB, config *Config, logger *log.Logger) *StationAggregator {
	// Create HTTP client with timeouts and IPv6 support
	httpClient := &http.Client{
		Timeout: time.Duration(config.Federation.NodeTimeout) * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &StationAggregator{
		db:         db,
		config:     config,
		logger:     logger,
		httpClient: httpClient,
	}
}

// PullFromNode pulls station data from a single node
func (sa *StationAggregator) PullFromNode(node *Node) error {
	startTime := time.Now()

	// Update pull attempt timestamp
	if err := sa.db.UpdateNodePullAttempt(node.UUID); err != nil {
		sa.logger.Printf("Failed to update pull attempt for node %s: %v", node.UUID, err)
	}

	// Construct URL for node's station provider endpoint
	// Use HTTP because Yggdrasil network already provides encryption at network layer
	url := fmt.Sprintf("http://[%s]:%d/api/federation/stations", node.Address, node.Port)

	sa.logger.Printf("Pulling stations from node %s (%s)", node.Name, url)

	// Create request with context for timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(sa.config.Federation.NodeTimeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return sa.handlePullError(node, startTime, fmt.Errorf("failed to create request: %w", err))
	}

	// Set headers
	req.Header.Set("User-Agent", "YggRadio-Federation-Server/1.0")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := sa.httpClient.Do(req)
	if err != nil {
		return sa.handlePullError(node, startTime, fmt.Errorf("failed to connect to node: %w", err))
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) // Read max 1KB for error message
		return sa.handlePullError(node, startTime, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body)))
	}

	// Read and parse response (limit to 10MB to prevent memory exhaustion)
	bodyReader := io.LimitReader(resp.Body, 10*1024*1024)
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return sa.handlePullError(node, startTime, fmt.Errorf("failed to read response: %w", err))
	}

	var pullResp PullResponse
	if err := json.Unmarshal(body, &pullResp); err != nil {
		return sa.handlePullError(node, startTime, fmt.Errorf("failed to parse response: %w", err))
	}

	// Validate response
	if !pullResp.Success {
		return sa.handlePullError(node, startTime, fmt.Errorf("node returned error response"))
	}

	// Verify pubkey matches registered node
	if pullResp.Pubkey != node.Pubkey {
		sa.logger.Printf("WARNING: Pubkey mismatch for node %s (expected %s, got %s)", node.UUID, node.Pubkey, pullResp.Pubkey)
		sa.db.LogSecurityEvent("pull_pubkey_mismatch", "high", "", node.Pubkey, url, fmt.Sprintf("expected: %s, got: %s", node.Pubkey, pullResp.Pubkey))
		return sa.handlePullError(node, startTime, fmt.Errorf("pubkey mismatch"))
	}

	// Check station count limit
	if len(pullResp.Stations) > sa.config.Federation.MaxStationsPerNode {
		sa.logger.Printf("Node %s exceeded station limit: %d > %d", node.UUID, len(pullResp.Stations), sa.config.Federation.MaxStationsPerNode)
		pullResp.Stations = pullResp.Stations[:sa.config.Federation.MaxStationsPerNode] // Truncate
	}

	// Process stations
	successCount := 0
	for _, station := range pullResp.Stations {
		if err := sa.processStation(node.UUID, &station); err != nil {
			sa.logger.Printf("Failed to process station %s from node %s: %v", station.UUID, node.UUID, err)
			continue
		}
		successCount++
	}

	// Update node's successful pull status
	if err := sa.db.UpdateNodePullSuccess(node.UUID, successCount); err != nil {
		sa.logger.Printf("Failed to update pull success for node %s: %v", node.UUID, err)
	}

	// Record successful pull in history
	duration := time.Since(startTime).Milliseconds()
	if err := sa.db.RecordPullHistory(node.UUID, true, successCount, "", duration); err != nil {
		sa.logger.Printf("Failed to record pull history for node %s: %v", node.UUID, err)
	}

	sa.logger.Printf("Successfully pulled %d stations from node %s (took %dms)", successCount, node.Name, duration)

	return nil
}

// processStation validates and upserts a single station
func (sa *StationAggregator) processStation(nodeUUID string, station *PulledStationData) error {
	// Validate station data
	if err := sa.validateStation(station); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Upsert station in database
	if err := sa.db.UpsertStation(nodeUUID, station); err != nil {
		return fmt.Errorf("failed to upsert station: %w", err)
	}

	return nil
}

// validateStation validates station data
func (sa *StationAggregator) validateStation(station *PulledStationData) error {
	// Validate UUID
	if !isValidUUID(station.UUID) {
		return fmt.Errorf("invalid UUID")
	}

	// Validate name
	if len(station.Name) < 3 || len(station.Name) > 100 {
		return fmt.Errorf("invalid name length")
	}

	// Validate description
	if len(station.Description) > 5000 {
		return fmt.Errorf("description too long")
	}

	// Validate mountpoint
	if len(station.Mountpoint) < 1 || len(station.Mountpoint) > 256 {
		return fmt.Errorf("invalid mountpoint length")
	}

	// Validate owner pubkey (supports both Ed25519 and magic link synthetic keys)
	// Ed25519: 64 hex characters
	// Magic link: "magiclink:" prefix + 54 digits (total 64 chars)
	if len(station.OwnerPubkey) != 64 {
		return fmt.Errorf("invalid owner pubkey length: %d", len(station.OwnerPubkey))
	}

	// Check if it's a magic link synthetic key or Ed25519 hex key
	isMagicLink := len(station.OwnerPubkey) >= 10 && station.OwnerPubkey[:10] == "magiclink:"
	if !isMagicLink && !isHexString(station.OwnerPubkey) {
		return fmt.Errorf("invalid owner pubkey format")
	}

	// Validate status
	if station.Status != "online" && station.Status != "offline" {
		return fmt.Errorf("invalid status: %s", station.Status)
	}

	// Validate rating values
	if station.AverageRating < 0 || station.AverageRating > 5 {
		return fmt.Errorf("invalid average_rating: %f", station.AverageRating)
	}
	if station.VoteCount < 0 {
		return fmt.Errorf("invalid vote_count: %d", station.VoteCount)
	}

	// Validate bitrate (if provided)
	if station.Bitrate < 0 || station.Bitrate > 320 {
		return fmt.Errorf("invalid bitrate: %d", station.Bitrate)
	}

	// Validate listeners count
	if station.ListenersCount < 0 {
		return fmt.Errorf("invalid listeners_count: %d", station.ListenersCount)
	}

	return nil
}

// handlePullError handles a pull failure
func (sa *StationAggregator) handlePullError(node *Node, startTime time.Time, err error) error {
	duration := time.Since(startTime).Milliseconds()

	// Update node failure count
	if dbErr := sa.db.UpdateNodePullFailure(node.UUID, sa.config.Federation.MaxConsecutiveFailures); dbErr != nil {
		sa.logger.Printf("Failed to update pull failure for node %s: %v", node.UUID, dbErr)
	}

	// Record failed pull in history
	if dbErr := sa.db.RecordPullHistory(node.UUID, false, 0, err.Error(), duration); dbErr != nil {
		sa.logger.Printf("Failed to record pull history for node %s: %v", node.UUID, dbErr)
	}

	sa.logger.Printf("Pull failed for node %s: %v (took %dms)", node.Name, err, duration)

	return err
}

// PullFromAllNodes pulls station data from all active nodes
func (sa *StationAggregator) PullFromAllNodes() error {
	nodes, err := sa.db.GetActiveNodes()
	if err != nil {
		return fmt.Errorf("failed to get active nodes: %w", err)
	}

	// No logging if no nodes - this is normal during initial setup
	if len(nodes) == 0 {
		return nil
	}

	// Only log when actually pulling from nodes (reduces noise when idle)
	sa.logger.Printf("Pulling from %d active node(s)", len(nodes))

	successCount := 0
	failCount := 0

	for _, node := range nodes {
		if err := sa.PullFromNode(node); err != nil {
			failCount++
			// Continue with other nodes even if one fails
			continue
		}
		successCount++
	}

	// Only log completion if there were failures (successful pulls are silent)
	if failCount > 0 {
		sa.logger.Printf("Pull completed: %d successful, %d failed (total: %d nodes)", successCount, failCount, len(nodes))
	}

	return nil
}

// CleanupStaleStations removes stations that haven't been seen within the TTL
func (sa *StationAggregator) CleanupStaleStations() error {
	removed, err := sa.db.RemoveStaleStations(sa.config.Federation.StationTTL)
	if err != nil {
		return fmt.Errorf("failed to cleanup stale stations: %w", err)
	}

	if removed > 0 {
		sa.logger.Printf("Removed %d stale stations (TTL: %ds)", removed, sa.config.Federation.StationTTL)
	}

	return nil
}
