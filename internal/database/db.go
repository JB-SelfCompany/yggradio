package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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

// New creates a new database connection and runs migrations
func New(dbPath string, logger *log.Logger) (*DB, error) {
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

	// Set connection pool settings
	sqlDB.SetMaxOpenConns(1) // SQLite only supports one writer
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
	if db.logger != nil {
		db.logger.Println("Closing database connection...")
	}
	return db.DB.Close()
}

// LogSecurityEvent logs a security event to the audit log
func (db *DB) LogSecurityEvent(eventType, severity, ipv6, pubkey, endpoint, details string) error {
	query := `
		INSERT INTO security_audit_log (event_type, severity, ipv6_address, pubkey, endpoint, details)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, eventType, severity, ipv6, pubkey, endpoint, details)
	if err != nil {
		return fmt.Errorf("failed to log security event: %w", err)
	}
	return nil
}

// CleanupExpiredSessions removes expired authentication sessions
func (db *DB) CleanupExpiredSessions() error {
	query := `DELETE FROM auth_sessions WHERE expires_at < CURRENT_TIMESTAMP`
	result, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		db.logger.Printf("Cleaned up %d expired sessions", rows)
	}

	return nil
}

// CleanupOldAuditLogs removes old audit logs based on retention period
func (db *DB) CleanupOldAuditLogs(retentionDays int) error {
	query := `
		DELETE FROM security_audit_log
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`
	result, err := db.Exec(query, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to cleanup old audit logs: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		db.logger.Printf("Cleaned up %d old audit log entries", rows)
	}

	return nil
}

// GetStationByMountpoint retrieves a station by its mountpoint
func (db *DB) GetStationByMountpoint(mountpoint string) (*Station, error) {
	query := `
		SELECT id, uuid, name, description, mountpoint, owner_pubkey,
			   created_at, updated_at, status, listeners_count,
			   metadata_title, content_type, bitrate, genre, pow_hash
		FROM stations WHERE mountpoint = ?
	`

	var s Station
	err := db.QueryRow(query, mountpoint).Scan(
		&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
		&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
		&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
		&s.Bitrate, &s.Genre, &s.PoWHash,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get station: %w", err)
	}

	return &s, nil
}

// UpdateStationStatus updates the station's online status
func (db *DB) UpdateStationStatus(mountpoint, status string) error {
	query := `
		UPDATE stations
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE mountpoint = ?
	`
	_, err := db.Exec(query, status, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update station status: %w", err)
	}
	return nil
}

// UpdateStationListenerCount updates the number of listeners for a station
func (db *DB) UpdateStationListenerCount(mountpoint string, count int) error {
	query := `
		UPDATE stations
		SET listeners_count = ?, updated_at = CURRENT_TIMESTAMP
		WHERE mountpoint = ?
	`
	_, err := db.Exec(query, count, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update listener count: %w", err)
	}
	return nil
}

// UpdateStationMetadata updates the current playing metadata
func (db *DB) UpdateStationMetadata(mountpoint, title string) error {
	query := `
		UPDATE stations
		SET metadata_title = ?, updated_at = CURRENT_TIMESTAMP
		WHERE mountpoint = ?
	`
	_, err := db.Exec(query, title, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update station metadata: %w", err)
	}
	return nil
}

// Station represents a radio station
type Station struct {
	ID             int64
	UUID           string
	Name           string
	Description    sql.NullString
	Mountpoint     string
	OwnerPubkey    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Status         string
	ListenersCount int
	MetadataTitle  sql.NullString
	ContentType    string
	Bitrate        sql.NullInt64
	Genre          sql.NullString
	PoWHash        string
}

// BeginTx starts a new transaction
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.DB.Begin()
}

// Ping checks if the database is accessible
func (db *DB) Ping() error {
	return db.DB.Ping()
}

// GetDatabaseStats returns database statistics
func (db *DB) GetDatabaseStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// ✅ SECURE - Use static queries for each table (prevents SQL injection)
	queries := map[string]string{
		"stations":  "SELECT COUNT(*) FROM stations",
		"users":     "SELECT COUNT(*) FROM users",
		"comments":  "SELECT COUNT(*) FROM comments",
	}

	for table, query := range queries {
		var count int
		if err := db.QueryRow(query).Scan(&count); err != nil {
			return nil, fmt.Errorf("failed to count %s: %w", table, err)
		}
		stats[table] = count
	}

	// Get database size
	var pageCount, pageSize int
	if err := db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return nil, fmt.Errorf("failed to get page size: %w", err)
	}
	stats["size_bytes"] = pageCount * pageSize

	return stats, nil
}

// FederatedStation represents a cached federated station
type FederatedStation struct {
	ID                int64
	UUID              string
	SourceNode        string
	SourceNodeAddress sql.NullString // Yggdrasil IPv6 address
	SourceNodeName    sql.NullString
	Name              string
	Description       sql.NullString
	Mountpoint        string
	OwnerPubkey       string
	Status            string
	ListenersCount    int
	ContentType       sql.NullString
	Bitrate           sql.NullInt64
	Genre             sql.NullString
	MetadataTitle     sql.NullString
	AverageRating     float64
	VoteCount         int
	CachedAt          time.Time
	LastUpdated       time.Time
}

// GetFederatedStationCache retrieves a cached federated station
func (db *DB) GetFederatedStationCache(sourceNode, mountpoint string) (*FederatedStation, error) {
	query := `
		SELECT id, uuid, source_node, source_node_address, source_node_name, name, description,
		       mountpoint, owner_pubkey, status, listeners_count,
		       content_type, bitrate, genre, metadata_title,
		       average_rating, vote_count, cached_at, last_updated
		FROM federated_station_cache
		WHERE source_node = ? AND mountpoint = ?
	`

	var fs FederatedStation
	err := db.QueryRow(query, sourceNode, mountpoint).Scan(
		&fs.ID, &fs.UUID, &fs.SourceNode, &fs.SourceNodeAddress, &fs.SourceNodeName,
		&fs.Name, &fs.Description, &fs.Mountpoint, &fs.OwnerPubkey,
		&fs.Status, &fs.ListenersCount, &fs.ContentType, &fs.Bitrate,
		&fs.Genre, &fs.MetadataTitle, &fs.AverageRating, &fs.VoteCount,
		&fs.CachedAt, &fs.LastUpdated,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get federated station cache: %w", err)
	}

	return &fs, nil
}

// UpsertFederatedStationCache inserts or updates a federated station in cache
func (db *DB) UpsertFederatedStationCache(station *FederatedStation) error {
	query := `
		INSERT INTO federated_station_cache (
			uuid, source_node, source_node_address, source_node_name, name, description,
			mountpoint, owner_pubkey, status, listeners_count,
			content_type, bitrate, genre, metadata_title,
			average_rating, vote_count, last_updated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(source_node, mountpoint) DO UPDATE SET
			uuid = excluded.uuid,
			source_node_address = excluded.source_node_address,
			source_node_name = excluded.source_node_name,
			name = excluded.name,
			description = excluded.description,
			owner_pubkey = excluded.owner_pubkey,
			status = excluded.status,
			listeners_count = excluded.listeners_count,
			content_type = excluded.content_type,
			bitrate = excluded.bitrate,
			genre = excluded.genre,
			metadata_title = excluded.metadata_title,
			average_rating = excluded.average_rating,
			vote_count = excluded.vote_count,
			last_updated = CURRENT_TIMESTAMP
	`

	_, err := db.Exec(query,
		station.UUID, station.SourceNode, station.SourceNodeAddress, station.SourceNodeName,
		station.Name, station.Description, station.Mountpoint,
		station.OwnerPubkey, station.Status, station.ListenersCount,
		station.ContentType, station.Bitrate, station.Genre,
		station.MetadataTitle, station.AverageRating, station.VoteCount,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert federated station cache: %w", err)
	}

	return nil
}

// UpsertFederatedStationCachePreserveMetadata inserts or updates a federated station in cache,
// but preserves the existing metadata_title field (doesn't overwrite it).
// This is used by the federation client when syncing with the server, to avoid
// overwriting real-time metadata updates from the proxy or metadata polling worker.
func (db *DB) UpsertFederatedStationCachePreserveMetadata(station *FederatedStation) error {
	query := `
		INSERT INTO federated_station_cache (
			uuid, source_node, source_node_address, source_node_name, name, description,
			mountpoint, owner_pubkey, status, listeners_count,
			content_type, bitrate, genre, metadata_title,
			average_rating, vote_count, last_updated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(source_node, mountpoint) DO UPDATE SET
			uuid = excluded.uuid,
			source_node_address = excluded.source_node_address,
			source_node_name = excluded.source_node_name,
			name = excluded.name,
			description = excluded.description,
			owner_pubkey = excluded.owner_pubkey,
			status = excluded.status,
			listeners_count = excluded.listeners_count,
			content_type = excluded.content_type,
			bitrate = excluded.bitrate,
			genre = excluded.genre,
			average_rating = excluded.average_rating,
			vote_count = excluded.vote_count,
			last_updated = CURRENT_TIMESTAMP
	`

	_, err := db.Exec(query,
		station.UUID, station.SourceNode, station.SourceNodeAddress, station.SourceNodeName,
		station.Name, station.Description, station.Mountpoint,
		station.OwnerPubkey, station.Status, station.ListenersCount,
		station.ContentType, station.Bitrate, station.Genre,
		station.MetadataTitle, station.AverageRating, station.VoteCount,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert federated station cache (preserve metadata): %w", err)
	}

	return nil
}

// ListFederatedStationCache retrieves all cached federated stations
func (db *DB) ListFederatedStationCache() ([]*FederatedStation, error) {
	query := `
		SELECT id, uuid, source_node, source_node_address, source_node_name, name, description,
		       mountpoint, owner_pubkey, status, listeners_count,
		       content_type, bitrate, genre, metadata_title,
		       average_rating, vote_count, cached_at, last_updated
		FROM federated_station_cache
		ORDER BY last_updated DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list federated station cache: %w", err)
	}
	defer rows.Close()

	var stations []*FederatedStation
	for rows.Next() {
		var fs FederatedStation
		err := rows.Scan(
			&fs.ID, &fs.UUID, &fs.SourceNode, &fs.SourceNodeAddress, &fs.SourceNodeName,
			&fs.Name, &fs.Description, &fs.Mountpoint, &fs.OwnerPubkey,
			&fs.Status, &fs.ListenersCount, &fs.ContentType, &fs.Bitrate,
			&fs.Genre, &fs.MetadataTitle, &fs.AverageRating, &fs.VoteCount,
			&fs.CachedAt, &fs.LastUpdated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan federated station: %w", err)
		}
		stations = append(stations, &fs)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating federated stations: %w", err)
	}

	return stations, nil
}

// ExpireFederatedStationCache removes cached stations older than the given time
func (db *DB) ExpireFederatedStationCache(olderThan time.Time) error {
	query := `DELETE FROM federated_station_cache WHERE last_updated < ?`
	result, err := db.Exec(query, olderThan)
	if err != nil {
		return fmt.Errorf("failed to expire federated station cache: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		db.logger.Printf("Expired %d old federated station cache entries", rows)
	}

	return nil
}

// GetFederatedStationCacheByAddress retrieves a cached federated station by IPv6 address
func (db *DB) GetFederatedStationCacheByAddress(nodeAddress, mountpoint string) (*FederatedStation, error) {
	query := `
		SELECT id, uuid, source_node, source_node_address, source_node_name, name, description,
		       mountpoint, owner_pubkey, status, listeners_count,
		       content_type, bitrate, genre, metadata_title,
		       average_rating, vote_count, cached_at, last_updated
		FROM federated_station_cache
		WHERE source_node_address = ? AND mountpoint = ?
	`

	var fs FederatedStation
	err := db.QueryRow(query, nodeAddress, mountpoint).Scan(
		&fs.ID, &fs.UUID, &fs.SourceNode, &fs.SourceNodeAddress, &fs.SourceNodeName,
		&fs.Name, &fs.Description, &fs.Mountpoint, &fs.OwnerPubkey,
		&fs.Status, &fs.ListenersCount, &fs.ContentType, &fs.Bitrate,
		&fs.Genre, &fs.MetadataTitle, &fs.AverageRating, &fs.VoteCount,
		&fs.CachedAt, &fs.LastUpdated,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get federated station cache by address: %w", err)
	}

	return &fs, nil
}

// ClearFederatedStationCache clears all entries from the federated station cache
func (db *DB) ClearFederatedStationCache() error {
	query := `DELETE FROM federated_station_cache`
	result, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to clear federated station cache: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		db.logger.Printf("Cleared %d federated station cache entries", rows)
	}

	return nil
}

// GetLastFederationServer retrieves the last used federation server address
// Returns empty string if no previous server address is stored
func (db *DB) GetLastFederationServer() string {
	query := `SELECT value FROM server_config WHERE key = 'last_federation_server'`
	var address string
	err := db.QueryRow(query).Scan(&address)
	if err == sql.ErrNoRows {
		return "" // No previous server configured
	}
	if err != nil {
		db.logger.Printf("WARNING: Failed to get last federation server: %v", err)
		return ""
	}
	return address
}

// SaveLastFederationServer saves the current federation server address
// Used to detect federation server changes on next startup
func (db *DB) SaveLastFederationServer(address string) error {
	query := `
		INSERT INTO server_config (key, value, updated_at)
		VALUES ('last_federation_server', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, address)
	if err != nil {
		return fmt.Errorf("failed to save federation server address: %w", err)
	}
	return nil
}

// UpdateFederatedStationMetadata updates only the metadata_title for a federated station
func (db *DB) UpdateFederatedStationMetadata(nodeAddress, mountpoint, metadata string) error {
	query := `
		UPDATE federated_station_cache
		SET metadata_title = ?,
		    last_updated = CURRENT_TIMESTAMP
		WHERE source_node_address = ? AND mountpoint = ?
	`

	result, err := db.Exec(query, metadata, nodeAddress, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update federated station metadata: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no federated station found with address %s and mountpoint %s", nodeAddress, mountpoint)
	}

	return nil
}

// DeleteFederatedStationsNotInList deletes cached stations whose UUIDs are not in the provided list
func (db *DB) DeleteFederatedStationsNotInList(uuids []string) error {
	if len(uuids) == 0 {
		// If list is empty, clear all
		return db.ClearFederatedStationCache()
	}

	// Build placeholders for SQL IN clause
	placeholders := make([]string, len(uuids))
	args := make([]interface{}, len(uuids))
	for i, uuid := range uuids {
		placeholders[i] = "?"
		args[i] = uuid
	}

	query := fmt.Sprintf(`DELETE FROM federated_station_cache WHERE uuid NOT IN (%s)`,
		strings.Join(placeholders, ", "))

	result, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete old federated stations: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		db.logger.Printf("Deleted %d stale federated station cache entries", rows)
	}

	return nil
}
