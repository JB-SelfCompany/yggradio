package federation_server

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// DB represents the database connection
type DB struct {
	*sql.DB
	logger *log.Logger
}

// NewDB creates a new database connection and runs migrations
func NewDB(dbPath string, logger *log.Logger) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings (SQLite only supports one writer)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	db := &DB{
		DB:     sqlDB,
		logger: logger,
	}

	// Run migrations
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	db.logger.Println("Database initialized successfully")
	return db, nil
}

// migrate runs database migrations
func (db *DB) migrate() error {
	db.logger.Println("Running database migrations...")

	// Execute schema
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	db.logger.Println("Database migrations completed")
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.logger.Println("Closing database connection...")
	return db.DB.Close()
}

// === Node Repository Methods ===

// RegisterNode creates or updates a node registration
func (db *DB) RegisterNode(req *RegistrationRequest) (*Node, error) {
	// Check if node already exists by pubkey
	var existingNode Node
	err := db.QueryRow(`
		SELECT id, uuid, pubkey, status FROM federation_nodes WHERE pubkey = ?
	`, req.Pubkey).Scan(&existingNode.ID, &existingNode.UUID, &existingNode.Pubkey, &existingNode.Status)

	now := time.Now()

	if err == sql.ErrNoRows {
		// New node - insert
		query := `
			INSERT INTO federation_nodes (uuid, address, port, pubkey, name, description, version, last_seen, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active')
		`
		result, err := db.Exec(query, req.UUID, req.Address, req.Port, req.Pubkey, req.Name, req.Description, req.Version, now)
		if err != nil {
			return nil, fmt.Errorf("failed to insert node: %w", err)
		}

		id, _ := result.LastInsertId()

		return &Node{
			ID:              id,
			UUID:            req.UUID,
			Address:         req.Address,
			Port:            req.Port,
			Pubkey:          req.Pubkey,
			Name:            req.Name,
			FirstRegistered: now,
			LastSeen:        now,
			Status:          "active",
		}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to query existing node: %w", err)
	}

	// Existing node - update
	query := `
		UPDATE federation_nodes
		SET address = ?, port = ?, name = ?, description = ?, version = ?,
		    last_seen = ?, status = 'active', consecutive_failures = 0
		WHERE pubkey = ?
	`
	_, err = db.Exec(query, req.Address, req.Port, req.Name, req.Description, req.Version, now, req.Pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to update node: %w", err)
	}

	// Return updated node
	return db.GetNodeByPubkey(req.Pubkey)
}

// GetNodeByPubkey retrieves a node by its public key
func (db *DB) GetNodeByPubkey(pubkey string) (*Node, error) {
	query := `
		SELECT id, uuid, address, port, pubkey, name, description, version,
		       first_registered, last_seen, last_pull_attempt, last_pull_success,
		       status, consecutive_failures, total_pulls, total_stations
		FROM federation_nodes WHERE pubkey = ?
	`

	var node Node
	err := db.QueryRow(query, pubkey).Scan(
		&node.ID, &node.UUID, &node.Address, &node.Port, &node.Pubkey,
		&node.Name, &node.Description, &node.Version,
		&node.FirstRegistered, &node.LastSeen, &node.LastPullAttempt, &node.LastPullSuccess,
		&node.Status, &node.ConsecutiveFailures, &node.TotalPulls, &node.TotalStations,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return &node, nil
}

// GetNodeByUUID retrieves a node by its UUID
func (db *DB) GetNodeByUUID(uuid string) (*Node, error) {
	query := `
		SELECT id, uuid, address, port, pubkey, name, description, version,
		       first_registered, last_seen, last_pull_attempt, last_pull_success,
		       status, consecutive_failures, total_pulls, total_stations
		FROM federation_nodes WHERE uuid = ?
	`

	var node Node
	err := db.QueryRow(query, uuid).Scan(
		&node.ID, &node.UUID, &node.Address, &node.Port, &node.Pubkey,
		&node.Name, &node.Description, &node.Version,
		&node.FirstRegistered, &node.LastSeen, &node.LastPullAttempt, &node.LastPullSuccess,
		&node.Status, &node.ConsecutiveFailures, &node.TotalPulls, &node.TotalStations,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return &node, nil
}

// GetActiveNodes retrieves all active nodes
func (db *DB) GetActiveNodes() ([]*Node, error) {
	query := `
		SELECT id, uuid, address, port, pubkey, name, description, version,
		       first_registered, last_seen, last_pull_attempt, last_pull_success,
		       status, consecutive_failures, total_pulls, total_stations
		FROM federation_nodes WHERE status = 'active'
		ORDER BY last_seen DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var node Node
		err := rows.Scan(
			&node.ID, &node.UUID, &node.Address, &node.Port, &node.Pubkey,
			&node.Name, &node.Description, &node.Version,
			&node.FirstRegistered, &node.LastSeen, &node.LastPullAttempt, &node.LastPullSuccess,
			&node.Status, &node.ConsecutiveFailures, &node.TotalPulls, &node.TotalStations,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node: %w", err)
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

// GetAllNodes retrieves all nodes
func (db *DB) GetAllNodes() ([]*Node, error) {
	query := `
		SELECT id, uuid, address, port, pubkey, name, description, version,
		       first_registered, last_seen, last_pull_attempt, last_pull_success,
		       status, consecutive_failures, total_pulls, total_stations
		FROM federation_nodes
		ORDER BY last_seen DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var node Node
		err := rows.Scan(
			&node.ID, &node.UUID, &node.Address, &node.Port, &node.Pubkey,
			&node.Name, &node.Description, &node.Version,
			&node.FirstRegistered, &node.LastSeen, &node.LastPullAttempt, &node.LastPullSuccess,
			&node.Status, &node.ConsecutiveFailures, &node.TotalPulls, &node.TotalStations,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node: %w", err)
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

// UpdateNodePullAttempt updates the node's last pull attempt timestamp
func (db *DB) UpdateNodePullAttempt(nodeUUID string) error {
	query := `
		UPDATE federation_nodes
		SET last_pull_attempt = ?, total_pulls = total_pulls + 1
		WHERE uuid = ?
	`
	_, err := db.Exec(query, time.Now(), nodeUUID)
	return err
}

// UpdateNodePullSuccess updates the node's last successful pull
func (db *DB) UpdateNodePullSuccess(nodeUUID string, stationCount int) error {
	query := `
		UPDATE federation_nodes
		SET last_pull_success = ?, consecutive_failures = 0,
		    total_stations = ?, status = 'active'
		WHERE uuid = ?
	`
	_, err := db.Exec(query, time.Now(), stationCount, nodeUUID)
	return err
}

// UpdateNodePullFailure increments failure count and marks offline if threshold exceeded
func (db *DB) UpdateNodePullFailure(nodeUUID string, maxFailures int) error {
	query := `
		UPDATE federation_nodes
		SET consecutive_failures = consecutive_failures + 1,
		    status = CASE
		        WHEN consecutive_failures + 1 >= ? THEN 'offline'
		        ELSE status
		    END
		WHERE uuid = ?
	`
	_, err := db.Exec(query, maxFailures, nodeUUID)
	return err
}

// === Station Repository Methods ===

// UpsertStation creates or updates a federated station
func (db *DB) UpsertStation(nodeUUID string, station *PulledStationData) error {
	query := `
		INSERT INTO federated_stations (
			uuid, node_uuid, name, description, mountpoint, owner_pubkey,
			status, listeners_count, content_type, bitrate, genre, metadata_title,
			average_rating, vote_count, created_at, updated_at, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_uuid, mountpoint) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			status = excluded.status,
			listeners_count = excluded.listeners_count,
			content_type = excluded.content_type,
			bitrate = excluded.bitrate,
			genre = excluded.genre,
			metadata_title = excluded.metadata_title,
			average_rating = excluded.average_rating,
			vote_count = excluded.vote_count,
			updated_at = excluded.updated_at,
			last_seen = excluded.last_seen
	`

	now := time.Now()

	// Parse timestamps from station data
	var createdAt, updatedAt time.Time
	if station.CreatedAt != "" {
		createdAt, _ = time.Parse(time.RFC3339, station.CreatedAt)
	} else {
		createdAt = now
	}
	if station.UpdatedAt != "" {
		updatedAt, _ = time.Parse(time.RFC3339, station.UpdatedAt)
	} else {
		updatedAt = now
	}

	_, err := db.Exec(query,
		station.UUID, nodeUUID, station.Name, station.Description,
		station.Mountpoint, station.OwnerPubkey, station.Status,
		station.ListenersCount, station.ContentType, station.Bitrate,
		station.Genre, station.MetadataTitle, station.AverageRating,
		station.VoteCount, createdAt, updatedAt, now,
	)

	return err
}

// GetStationsByNode retrieves all stations for a specific node
func (db *DB) GetStationsByNode(nodeUUID string) ([]*Station, error) {
	query := `
		SELECT id, uuid, node_uuid, name, description, mountpoint, owner_pubkey,
		       status, listeners_count, content_type, bitrate, genre, metadata_title,
		       average_rating, vote_count, created_at, updated_at, first_seen, last_seen
		FROM federated_stations WHERE node_uuid = ?
		ORDER BY last_seen DESC
	`

	rows, err := db.Query(query, nodeUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to query stations: %w", err)
	}
	defer rows.Close()

	return db.scanStations(rows)
}

// GetAllStations retrieves all federated stations
func (db *DB) GetAllStations() ([]*Station, error) {
	query := `
		SELECT id, uuid, node_uuid, name, description, mountpoint, owner_pubkey,
		       status, listeners_count, content_type, bitrate, genre, metadata_title,
		       average_rating, vote_count, created_at, updated_at, first_seen, last_seen
		FROM federated_stations
		ORDER BY last_seen DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all stations: %w", err)
	}
	defer rows.Close()

	return db.scanStations(rows)
}

// GetOnlineStations retrieves all online stations
func (db *DB) GetOnlineStations() ([]*Station, error) {
	query := `
		SELECT id, uuid, node_uuid, name, description, mountpoint, owner_pubkey,
		       status, listeners_count, content_type, bitrate, genre, metadata_title,
		       average_rating, vote_count, created_at, updated_at, first_seen, last_seen
		FROM federated_stations
		WHERE status = 'online'
		ORDER BY last_seen DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query online stations: %w", err)
	}
	defer rows.Close()

	return db.scanStations(rows)
}

// scanStations is a helper to scan station rows
func (db *DB) scanStations(rows *sql.Rows) ([]*Station, error) {
	var stations []*Station
	for rows.Next() {
		var station Station
		err := rows.Scan(
			&station.ID, &station.UUID, &station.NodeUUID, &station.Name,
			&station.Description, &station.Mountpoint, &station.OwnerPubkey,
			&station.Status, &station.ListenersCount, &station.ContentType,
			&station.Bitrate, &station.Genre, &station.MetadataTitle,
			&station.AverageRating, &station.VoteCount,
			&station.CreatedAt, &station.UpdatedAt, &station.FirstSeen, &station.LastSeen,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan station: %w", err)
		}
		stations = append(stations, &station)
	}

	return stations, nil
}

// RemoveStaleStations removes stations not seen within the TTL
func (db *DB) RemoveStaleStations(ttlSeconds int) (int64, error) {
	query := `
		DELETE FROM federated_stations
		WHERE last_seen < datetime('now', '-' || ? || ' seconds')
	`
	result, err := db.Exec(query, ttlSeconds)
	if err != nil {
		return 0, fmt.Errorf("failed to remove stale stations: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows, nil
}

// === Pull History Methods ===

// RecordPullHistory records a pull operation in the audit trail
func (db *DB) RecordPullHistory(nodeUUID string, success bool, stationsPulled int, errorMsg string, durationMs int64) error {
	query := `
		INSERT INTO pull_history (node_uuid, success, stations_pulled, error_message, duration_ms)
		VALUES (?, ?, ?, ?, ?)
	`

	var errMsgPtr *string
	if errorMsg != "" {
		errMsgPtr = &errorMsg
	}

	_, err := db.Exec(query, nodeUUID, success, stationsPulled, errMsgPtr, durationMs)
	return err
}

// === Security Audit Methods ===

// LogSecurityEvent logs a security event
func (db *DB) LogSecurityEvent(eventType, severity, ipv6, pubkey, endpoint, details string) error {
	query := `
		INSERT INTO security_audit_log (event_type, severity, ipv6_address, pubkey, endpoint, details)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	var ipv6Ptr, pubkeyPtr, endpointPtr, detailsPtr *string
	if ipv6 != "" {
		ipv6Ptr = &ipv6
	}
	if pubkey != "" {
		pubkeyPtr = &pubkey
	}
	if endpoint != "" {
		endpointPtr = &endpoint
	}
	if details != "" {
		detailsPtr = &details
	}

	_, err := db.Exec(query, eventType, severity, ipv6Ptr, pubkeyPtr, endpointPtr, detailsPtr)
	if err != nil {
		return fmt.Errorf("failed to log security event: %w", err)
	}

	return nil
}

// GetStats returns database statistics
func (db *DB) GetStats() (map[string]int, error) {
	stats := make(map[string]int)

	// Count nodes by status
	rows, err := db.Query(`
		SELECT status, COUNT(*) FROM federation_nodes GROUP BY status
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err == nil {
				stats["nodes_"+status] = count
			}
		}
	}

	// Count total stations
	var totalStations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM federated_stations`).Scan(&totalStations); err == nil {
		stats["total_stations"] = totalStations
	}

	// Count online stations
	var onlineStations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM federated_stations WHERE status = 'online'`).Scan(&onlineStations); err == nil {
		stats["online_stations"] = onlineStations
	}

	// Count recent pulls (last hour)
	var recentPulls int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM pull_history
		WHERE pull_timestamp > datetime('now', '-1 hour')
	`).Scan(&recentPulls); err == nil {
		stats["recent_pulls"] = recentPulls
	}

	return stats, nil
}
