package models

import (
	"database/sql"
	"fmt"
	"time"
)

// MagicLink represents a magic link authentication token
type MagicLink struct {
	ID          int64
	TokenHash   string
	UserID      int64
	CreatedAt   time.Time
	LastUsed    time.Time
	IsActive    bool
	IPv6Created sql.NullString
	UserAgent   sql.NullString
}

// MagicLinkRepository provides database operations for magic links
type MagicLinkRepository struct {
	db *sql.DB
}

// NewMagicLinkRepository creates a new magic link repository
func NewMagicLinkRepository(db *sql.DB) *MagicLinkRepository {
	return &MagicLinkRepository{db: db}
}

// Create creates a new magic link
func (r *MagicLinkRepository) Create(ml *MagicLink) error {
	query := `
		INSERT INTO magic_links (token_hash, user_id, ipv6_created, user_agent)
		VALUES (?, ?, ?, ?)
		RETURNING id, created_at, last_used, is_active
	`

	err := r.db.QueryRow(
		query,
		ml.TokenHash,
		ml.UserID,
		ml.IPv6Created,
		ml.UserAgent,
	).Scan(&ml.ID, &ml.CreatedAt, &ml.LastUsed, &ml.IsActive)

	if err != nil {
		return fmt.Errorf("failed to create magic link: %w", err)
	}

	return nil
}

// GetByTokenHash retrieves a magic link by its token hash
func (r *MagicLinkRepository) GetByTokenHash(tokenHash string) (*MagicLink, error) {
	query := `
		SELECT id, token_hash, user_id, created_at, last_used, is_active, ipv6_created, user_agent
		FROM magic_links
		WHERE token_hash = ?
	`

	var ml MagicLink
	err := r.db.QueryRow(query, tokenHash).Scan(
		&ml.ID,
		&ml.TokenHash,
		&ml.UserID,
		&ml.CreatedAt,
		&ml.LastUsed,
		&ml.IsActive,
		&ml.IPv6Created,
		&ml.UserAgent,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get magic link: %w", err)
	}

	return &ml, nil
}

// GetByID retrieves a magic link by its ID
func (r *MagicLinkRepository) GetByID(id int64) (*MagicLink, error) {
	query := `
		SELECT id, token_hash, user_id, created_at, last_used, is_active, ipv6_created, user_agent
		FROM magic_links
		WHERE id = ?
	`

	var ml MagicLink
	err := r.db.QueryRow(query, id).Scan(
		&ml.ID,
		&ml.TokenHash,
		&ml.UserID,
		&ml.CreatedAt,
		&ml.LastUsed,
		&ml.IsActive,
		&ml.IPv6Created,
		&ml.UserAgent,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get magic link: %w", err)
	}

	return &ml, nil
}

// GetByUserID retrieves all magic links for a user
func (r *MagicLinkRepository) GetByUserID(userID int64) ([]*MagicLink, error) {
	query := `
		SELECT id, token_hash, user_id, created_at, last_used, is_active, ipv6_created, user_agent
		FROM magic_links
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query magic links: %w", err)
	}
	defer rows.Close()

	var links []*MagicLink
	for rows.Next() {
		var ml MagicLink
		err := rows.Scan(
			&ml.ID,
			&ml.TokenHash,
			&ml.UserID,
			&ml.CreatedAt,
			&ml.LastUsed,
			&ml.IsActive,
			&ml.IPv6Created,
			&ml.UserAgent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan magic link: %w", err)
		}
		links = append(links, &ml)
	}

	return links, nil
}

// GetActiveByUserID retrieves all active magic links for a user
func (r *MagicLinkRepository) GetActiveByUserID(userID int64) ([]*MagicLink, error) {
	query := `
		SELECT id, token_hash, user_id, created_at, last_used, is_active, ipv6_created, user_agent
		FROM magic_links
		WHERE user_id = ? AND is_active = 1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query active magic links: %w", err)
	}
	defer rows.Close()

	var links []*MagicLink
	for rows.Next() {
		var ml MagicLink
		err := rows.Scan(
			&ml.ID,
			&ml.TokenHash,
			&ml.UserID,
			&ml.CreatedAt,
			&ml.LastUsed,
			&ml.IsActive,
			&ml.IPv6Created,
			&ml.UserAgent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan magic link: %w", err)
		}
		links = append(links, &ml)
	}

	return links, nil
}

// UpdateLastUsed updates the last_used timestamp for a magic link
func (r *MagicLinkRepository) UpdateLastUsed(id int64) error {
	query := `
		UPDATE magic_links
		SET last_used = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to update last_used: %w", err)
	}

	return nil
}

// Deactivate deactivates a magic link (sets is_active to false)
func (r *MagicLinkRepository) Deactivate(id int64) error {
	query := `
		UPDATE magic_links
		SET is_active = 0
		WHERE id = ?
	`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to deactivate magic link: %w", err)
	}

	return nil
}

// DeactivateByUserID deactivates all magic links for a user
func (r *MagicLinkRepository) DeactivateByUserID(userID int64) error {
	query := `
		UPDATE magic_links
		SET is_active = 0
		WHERE user_id = ?
	`

	result, err := r.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to deactivate magic links: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no magic links found for user %d", userID)
	}

	return nil
}

// Delete permanently deletes a magic link
func (r *MagicLinkRepository) Delete(id int64) error {
	query := `DELETE FROM magic_links WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete magic link: %w", err)
	}

	return nil
}

// CountByUserID returns the number of magic links for a user
func (r *MagicLinkRepository) CountByUserID(userID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM magic_links WHERE user_id = ?`
	err := r.db.QueryRow(query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count magic links: %w", err)
	}
	return count, nil
}

// CountActiveByUserID returns the number of active magic links for a user
func (r *MagicLinkRepository) CountActiveByUserID(userID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM magic_links WHERE user_id = ? AND is_active = 1`
	err := r.db.QueryRow(query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active magic links: %w", err)
	}
	return count, nil
}
