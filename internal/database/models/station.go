package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

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
	IsPrivate      bool
	// Playlist streaming fields
	PlaylistEnabled     bool
	PlaylistDirectory   sql.NullString
	PlaylistMode        sql.NullString // "sequential" or "shuffle"
	PlaylistLoop        bool
	PlaylistFilePattern sql.NullString
	// External stream fields (v1.1.0+)
	ExternalStreamURL  sql.NullString
	ExternalStreamType sql.NullString // "hls" or "direct"
	AutoStart          bool            // Controls automatic stream startup (v1.1.0+)
}

// StationRepository provides database operations for stations
type StationRepository struct {
	db *sql.DB
}

// NewStationRepository creates a new station repository
func NewStationRepository(db *sql.DB) *StationRepository {
	return &StationRepository{db: db}
}

// Create creates a new station
func (r *StationRepository) Create(s *Station) error {
	// Generate UUID if not set
	if s.UUID == "" {
		s.UUID = uuid.New().String()
	}

	query := `
		INSERT INTO stations (uuid, name, description, mountpoint, owner_pubkey, content_type, bitrate, genre, pow_hash, is_private,
			playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern, external_stream_url, external_stream_type, auto_start)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at, updated_at, status, listeners_count
	`

	// Default auto_start to true for new stations with external URLs
	if s.ExternalStreamURL.Valid && s.ExternalStreamURL.String != "" {
		s.AutoStart = true
	}

	err := r.db.QueryRow(
		query,
		s.UUID, s.Name, s.Description, s.Mountpoint,
		s.OwnerPubkey, s.ContentType, s.Bitrate, s.Genre, s.PoWHash, s.IsPrivate,
		s.PlaylistEnabled, s.PlaylistDirectory, s.PlaylistMode, s.PlaylistLoop, s.PlaylistFilePattern,
		s.ExternalStreamURL, s.ExternalStreamType, s.AutoStart,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt, &s.Status, &s.ListenersCount)

	if err != nil {
		return fmt.Errorf("failed to create station: %w", err)
	}

	return nil
}

// GetByID retrieves a station by ID
func (r *StationRepository) GetByID(id int64) (*Station, error) {
	query := `
		SELECT id, uuid, name, description, mountpoint, owner_pubkey,
			   created_at, updated_at, status, listeners_count,
			   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
			   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
			   external_stream_url, external_stream_type, COALESCE(auto_start, 1) as auto_start
		FROM stations WHERE id = ?
	`

	var s Station
	err := r.db.QueryRow(query, id).Scan(
		&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
		&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
		&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
		&s.Bitrate, &s.Genre, &s.PoWHash, &s.IsPrivate,
		&s.PlaylistEnabled, &s.PlaylistDirectory, &s.PlaylistMode, &s.PlaylistLoop, &s.PlaylistFilePattern,
		&s.ExternalStreamURL, &s.ExternalStreamType, &s.AutoStart,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get station: %w", err)
	}

	return &s, nil
}

// GetByMountpoint retrieves a station by mountpoint
func (r *StationRepository) GetByMountpoint(mountpoint string) (*Station, error) {
	query := `
		SELECT id, uuid, name, description, mountpoint, owner_pubkey,
			   created_at, updated_at, status, listeners_count,
			   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
			   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
			   external_stream_url, external_stream_type, COALESCE(auto_start, 1) as auto_start
		FROM stations WHERE mountpoint = ?
	`

	var s Station
	err := r.db.QueryRow(query, mountpoint).Scan(
		&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
		&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
		&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
		&s.Bitrate, &s.Genre, &s.PoWHash, &s.IsPrivate,
		&s.PlaylistEnabled, &s.PlaylistDirectory, &s.PlaylistMode, &s.PlaylistLoop, &s.PlaylistFilePattern,
		&s.ExternalStreamURL, &s.ExternalStreamType, &s.AutoStart,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get station: %w", err)
	}

	return &s, nil
}

// GetByOwner retrieves all stations owned by a user
func (r *StationRepository) GetByOwner(ownerPubkey string) ([]*Station, error) {
	query := `
		SELECT id, uuid, name, description, mountpoint, owner_pubkey,
			   created_at, updated_at, status, listeners_count,
			   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
			   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
			   external_stream_url, external_stream_type
		FROM stations WHERE owner_pubkey = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, ownerPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to get stations: %w", err)
	}
	defer rows.Close()

	var stations []*Station
	for rows.Next() {
		var s Station
		err := rows.Scan(
			&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
			&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
			&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
			&s.Bitrate, &s.Genre, &s.PoWHash, &s.IsPrivate,
			&s.PlaylistEnabled, &s.PlaylistDirectory, &s.PlaylistMode, &s.PlaylistLoop, &s.PlaylistFilePattern,
			&s.ExternalStreamURL, &s.ExternalStreamType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan station: %w", err)
		}
		stations = append(stations, &s)
	}

	return stations, nil
}

// List retrieves all stations with pagination
func (r *StationRepository) List(limit, offset int) ([]*Station, error) {
	// SECURITY: Validate pagination parameters to prevent SQL injection
	const (
		maxLimit      = 1000 // Maximum records per query
		defaultLimit  = 50   // Default if invalid limit provided
	)

	// Validate and sanitize limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	// Validate offset (must be non-negative)
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, uuid, name, description, mountpoint, owner_pubkey,
			   created_at, updated_at, status, listeners_count,
			   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
			   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
			   external_stream_url, external_stream_type
		FROM stations
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list stations: %w", err)
	}
	defer rows.Close()

	var stations []*Station
	for rows.Next() {
		var s Station
		err := rows.Scan(
			&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
			&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
			&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
			&s.Bitrate, &s.Genre, &s.PoWHash, &s.IsPrivate,
			&s.PlaylistEnabled, &s.PlaylistDirectory, &s.PlaylistMode, &s.PlaylistLoop, &s.PlaylistFilePattern,
			&s.ExternalStreamURL, &s.ExternalStreamType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan station: %w", err)
		}
		stations = append(stations, &s)
	}

	return stations, nil
}

// ListPublic retrieves only public stations (for federation)
func (r *StationRepository) ListPublic(limit, offset int) ([]*Station, error) {
	// SECURITY: Validate pagination parameters to prevent SQL injection
	const (
		maxLimit      = 1000
		defaultLimit  = 50
	)

	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, uuid, name, description, mountpoint, owner_pubkey,
			   created_at, updated_at, status, listeners_count,
			   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
			   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
			   external_stream_url, external_stream_type
		FROM stations
		WHERE is_private = 0
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list public stations: %w", err)
	}
	defer rows.Close()

	var stations []*Station
	for rows.Next() {
		var s Station
		err := rows.Scan(
			&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
			&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
			&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
			&s.Bitrate, &s.Genre, &s.PoWHash, &s.IsPrivate,
			&s.PlaylistEnabled, &s.PlaylistDirectory, &s.PlaylistMode, &s.PlaylistLoop, &s.PlaylistFilePattern,
			&s.ExternalStreamURL, &s.ExternalStreamType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan station: %w", err)
		}
		stations = append(stations, &s)
	}

	return stations, nil
}

// ListWithPrivacyFilter retrieves stations with privacy filtering
// If ownerPubkey is empty, only returns public stations
// If ownerPubkey is provided, returns public stations + user's private stations
func (r *StationRepository) ListWithPrivacyFilter(limit, offset int, ownerPubkey string) ([]*Station, error) {
	// SECURITY: Validate pagination parameters to prevent SQL injection
	const (
		maxLimit      = 1000
		defaultLimit  = 50
	)

	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	var query string
	var args []interface{}

	if ownerPubkey == "" {
		// Unauthenticated users: only public stations
		query = `
			SELECT id, uuid, name, description, mountpoint, owner_pubkey,
				   created_at, updated_at, status, listeners_count,
				   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
				   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
				   external_stream_url, external_stream_type
			FROM stations
			WHERE is_private = 0
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
		args = []interface{}{limit, offset}
	} else {
		// Authenticated users: public stations + their own private stations
		query = `
			SELECT id, uuid, name, description, mountpoint, owner_pubkey,
				   created_at, updated_at, status, listeners_count,
				   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
				   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
				   external_stream_url, external_stream_type
			FROM stations
			WHERE is_private = 0 OR owner_pubkey = ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
		args = []interface{}{ownerPubkey, limit, offset}
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list stations: %w", err)
	}
	defer rows.Close()

	var stations []*Station
	for rows.Next() {
		var s Station
		err := rows.Scan(
			&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
			&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
			&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
			&s.Bitrate, &s.Genre, &s.PoWHash, &s.IsPrivate,
			&s.PlaylistEnabled, &s.PlaylistDirectory, &s.PlaylistMode, &s.PlaylistLoop, &s.PlaylistFilePattern,
			&s.ExternalStreamURL, &s.ExternalStreamType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan station: %w", err)
		}
		stations = append(stations, &s)
	}

	return stations, nil
}

// Update updates station information
func (r *StationRepository) Update(s *Station) error {
	query := `
		UPDATE stations
		SET name = ?, description = ?, is_private = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := r.db.Exec(query, s.Name, s.Description, s.IsPrivate, s.ID)
	if err != nil {
		return fmt.Errorf("failed to update station: %w", err)
	}

	return nil
}

// UpdateStatus updates the station status
func (r *StationRepository) UpdateStatus(mountpoint, status string) error {
	query := `
		UPDATE stations
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE mountpoint = ?
	`

	_, err := r.db.Exec(query, status, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update station status: %w", err)
	}

	return nil
}

// UpdateAutoStart updates the auto_start flag for a station
func (r *StationRepository) UpdateAutoStart(mountpoint string, autoStart bool) error {
	query := `
		UPDATE stations
		SET auto_start = ?, updated_at = CURRENT_TIMESTAMP
		WHERE mountpoint = ?
	`

	_, err := r.db.Exec(query, autoStart, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update station auto_start: %w", err)
	}

	return nil
}

// UpdateListenerCount updates the listener count
func (r *StationRepository) UpdateListenerCount(mountpoint string, count int) error {
	query := `
		UPDATE stations
		SET listeners_count = ?, updated_at = CURRENT_TIMESTAMP
		WHERE mountpoint = ?
	`

	_, err := r.db.Exec(query, count, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update listener count: %w", err)
	}

	return nil
}

// UpdateMetadata updates the current playing metadata
func (r *StationRepository) UpdateMetadata(mountpoint, title string) error {
	query := `
		UPDATE stations
		SET metadata_title = ?, updated_at = CURRENT_TIMESTAMP
		WHERE mountpoint = ?
	`

	_, err := r.db.Exec(query, title, mountpoint)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// Delete deletes a station
func (r *StationRepository) Delete(id int64) error {
	query := `DELETE FROM stations WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete station: %w", err)
	}

	return nil
}

// Count returns the total number of stations
func (r *StationRepository) Count() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM stations`
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count stations: %w", err)
	}
	return count, nil
}

// UpdatePlaylistConfig updates the playlist configuration for a station
// SECURITY: Caller MUST validate PlaylistDirectory and PlaylistFilePattern before calling
// Use security.Validator.ValidateDirectoryPath() and ValidateFilePattern()
func (r *StationRepository) UpdatePlaylistConfig(s *Station) error {
	// SECURITY NOTE: Path validation MUST be done by the caller (API handler)
	// This is a repository layer - it trusts the caller has validated inputs
	// See internal/api/handlers/station.go UpdatePlaylistConfig handler

	query := `
		UPDATE stations
		SET playlist_enabled = ?,
			playlist_directory = ?,
			playlist_mode = ?,
			playlist_loop = ?,
			playlist_file_pattern = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := r.db.Exec(
		query,
		s.PlaylistEnabled,
		s.PlaylistDirectory,
		s.PlaylistMode,
		s.PlaylistLoop,
		s.PlaylistFilePattern,
		s.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update playlist config: %w", err)
	}

	return nil
}

// ListWithPrivacyFilterSorted retrieves stations with privacy filtering and sorting
// If ownerPubkey is empty, only returns public stations
// If ownerPubkey is provided, returns public stations + user's private stations
// sortBy: "listeners", "rating", or "created" (default: "created")
// sortOrder: "asc" or "desc" (default: "desc")
func (r *StationRepository) ListWithPrivacyFilterSorted(limit, offset int, ownerPubkey, sortBy, sortOrder string) ([]*Station, error) {
	// SECURITY: Validate pagination parameters
	const (
		maxLimit     = 1000
		defaultLimit = 50
	)

	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	// SECURITY: Whitelist for sortBy to prevent SQL injection
	var orderByClause string
	switch sortBy {
	case "listeners":
		orderByClause = "listeners_count"
	case "rating":
		// Use subquery to calculate average rating
		orderByClause = "(SELECT COALESCE(AVG(rating), 0) FROM ratings WHERE target_type = 'station' AND target_id = stations.id)"
	case "created":
		orderByClause = "created_at"
	default:
		// Default to created
		orderByClause = "created_at"
	}

	// SECURITY: Whitelist for sortOrder
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc" // Default to descending
	}

	var query string
	var args []interface{}

	if ownerPubkey == "" {
		// Unauthenticated users: only public stations
		query = fmt.Sprintf(`
			SELECT id, uuid, name, description, mountpoint, owner_pubkey,
				   created_at, updated_at, status, listeners_count,
				   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
				   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
				   external_stream_url, external_stream_type, COALESCE(auto_start, 1) as auto_start
			FROM stations
			WHERE is_private = 0
			ORDER BY %s %s
			LIMIT ? OFFSET ?
		`, orderByClause, sortOrder)
		args = []interface{}{limit, offset}
	} else {
		// Authenticated users: public stations + their own private stations
		query = fmt.Sprintf(`
			SELECT id, uuid, name, description, mountpoint, owner_pubkey,
				   created_at, updated_at, status, listeners_count,
				   metadata_title, content_type, bitrate, genre, pow_hash, is_private,
				   playlist_enabled, playlist_directory, playlist_mode, playlist_loop, playlist_file_pattern,
				   external_stream_url, external_stream_type, COALESCE(auto_start, 1) as auto_start
			FROM stations
			WHERE is_private = 0 OR owner_pubkey = ?
			ORDER BY %s %s
			LIMIT ? OFFSET ?
		`, orderByClause, sortOrder)
		args = []interface{}{ownerPubkey, limit, offset}
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list stations: %w", err)
	}
	defer rows.Close()

	var stations []*Station
	for rows.Next() {
		var s Station
		err := rows.Scan(
			&s.ID, &s.UUID, &s.Name, &s.Description, &s.Mountpoint,
			&s.OwnerPubkey, &s.CreatedAt, &s.UpdatedAt, &s.Status,
			&s.ListenersCount, &s.MetadataTitle, &s.ContentType,
			&s.Bitrate, &s.Genre, &s.PoWHash, &s.IsPrivate,
			&s.PlaylistEnabled, &s.PlaylistDirectory, &s.PlaylistMode, &s.PlaylistLoop, &s.PlaylistFilePattern,
			&s.ExternalStreamURL, &s.ExternalStreamType, &s.AutoStart,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan station: %w", err)
		}
		stations = append(stations, &s)
	}

	return stations, nil
}
