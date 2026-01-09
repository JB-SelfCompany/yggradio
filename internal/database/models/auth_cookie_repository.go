package models

import (
	"database/sql"
	"fmt"
	"time"
)

// AuthCookie represents a session cookie for magic link authentication
type AuthCookie struct {
	ID          int64
	CookieHash  string
	MagicLinkID int64
	UserID      int64
	CreatedAt   time.Time
	ExpiresAt   time.Time
	LastUsed    time.Time
	IPv6Address sql.NullString
	UserAgent   sql.NullString
}

// AuthCookieRepository provides database operations for auth cookies
type AuthCookieRepository struct {
	db *sql.DB
}

// NewAuthCookieRepository creates a new auth cookie repository
func NewAuthCookieRepository(db *sql.DB) *AuthCookieRepository {
	return &AuthCookieRepository{db: db}
}

// Create creates a new auth cookie
func (r *AuthCookieRepository) Create(cookie *AuthCookie) error {
	query := `
		INSERT INTO auth_cookies (cookie_hash, magic_link_id, user_id, expires_at, ipv6_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, created_at, last_used
	`

	err := r.db.QueryRow(
		query,
		cookie.CookieHash,
		cookie.MagicLinkID,
		cookie.UserID,
		cookie.ExpiresAt,
		cookie.IPv6Address,
		cookie.UserAgent,
	).Scan(&cookie.ID, &cookie.CreatedAt, &cookie.LastUsed)

	if err != nil {
		return fmt.Errorf("failed to create auth cookie: %w", err)
	}

	return nil
}

// GetByCookieHash retrieves an auth cookie by its hash
func (r *AuthCookieRepository) GetByCookieHash(cookieHash string) (*AuthCookie, error) {
	query := `
		SELECT id, cookie_hash, magic_link_id, user_id, created_at, expires_at, last_used, ipv6_address, user_agent
		FROM auth_cookies
		WHERE cookie_hash = ?
	`

	var cookie AuthCookie
	err := r.db.QueryRow(query, cookieHash).Scan(
		&cookie.ID,
		&cookie.CookieHash,
		&cookie.MagicLinkID,
		&cookie.UserID,
		&cookie.CreatedAt,
		&cookie.ExpiresAt,
		&cookie.LastUsed,
		&cookie.IPv6Address,
		&cookie.UserAgent,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get auth cookie: %w", err)
	}

	return &cookie, nil
}

// GetByID retrieves an auth cookie by its ID
func (r *AuthCookieRepository) GetByID(id int64) (*AuthCookie, error) {
	query := `
		SELECT id, cookie_hash, magic_link_id, user_id, created_at, expires_at, last_used, ipv6_address, user_agent
		FROM auth_cookies
		WHERE id = ?
	`

	var cookie AuthCookie
	err := r.db.QueryRow(query, id).Scan(
		&cookie.ID,
		&cookie.CookieHash,
		&cookie.MagicLinkID,
		&cookie.UserID,
		&cookie.CreatedAt,
		&cookie.ExpiresAt,
		&cookie.LastUsed,
		&cookie.IPv6Address,
		&cookie.UserAgent,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get auth cookie: %w", err)
	}

	return &cookie, nil
}

// GetByUserID retrieves all auth cookies for a user
func (r *AuthCookieRepository) GetByUserID(userID int64) ([]*AuthCookie, error) {
	query := `
		SELECT id, cookie_hash, magic_link_id, user_id, created_at, expires_at, last_used, ipv6_address, user_agent
		FROM auth_cookies
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query auth cookies: %w", err)
	}
	defer rows.Close()

	var cookies []*AuthCookie
	for rows.Next() {
		var cookie AuthCookie
		err := rows.Scan(
			&cookie.ID,
			&cookie.CookieHash,
			&cookie.MagicLinkID,
			&cookie.UserID,
			&cookie.CreatedAt,
			&cookie.ExpiresAt,
			&cookie.LastUsed,
			&cookie.IPv6Address,
			&cookie.UserAgent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan auth cookie: %w", err)
		}
		cookies = append(cookies, &cookie)
	}

	return cookies, nil
}

// GetActiveByUserID retrieves all non-expired cookies for a user
func (r *AuthCookieRepository) GetActiveByUserID(userID int64) ([]*AuthCookie, error) {
	query := `
		SELECT id, cookie_hash, magic_link_id, user_id, created_at, expires_at, last_used, ipv6_address, user_agent
		FROM auth_cookies
		WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query active auth cookies: %w", err)
	}
	defer rows.Close()

	var cookies []*AuthCookie
	for rows.Next() {
		var cookie AuthCookie
		err := rows.Scan(
			&cookie.ID,
			&cookie.CookieHash,
			&cookie.MagicLinkID,
			&cookie.UserID,
			&cookie.CreatedAt,
			&cookie.ExpiresAt,
			&cookie.LastUsed,
			&cookie.IPv6Address,
			&cookie.UserAgent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan auth cookie: %w", err)
		}
		cookies = append(cookies, &cookie)
	}

	return cookies, nil
}

// GetByMagicLinkID retrieves all cookies associated with a magic link
func (r *AuthCookieRepository) GetByMagicLinkID(magicLinkID int64) ([]*AuthCookie, error) {
	query := `
		SELECT id, cookie_hash, magic_link_id, user_id, created_at, expires_at, last_used, ipv6_address, user_agent
		FROM auth_cookies
		WHERE magic_link_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, magicLinkID)
	if err != nil {
		return nil, fmt.Errorf("failed to query auth cookies by magic link: %w", err)
	}
	defer rows.Close()

	var cookies []*AuthCookie
	for rows.Next() {
		var cookie AuthCookie
		err := rows.Scan(
			&cookie.ID,
			&cookie.CookieHash,
			&cookie.MagicLinkID,
			&cookie.UserID,
			&cookie.CreatedAt,
			&cookie.ExpiresAt,
			&cookie.LastUsed,
			&cookie.IPv6Address,
			&cookie.UserAgent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan auth cookie: %w", err)
		}
		cookies = append(cookies, &cookie)
	}

	return cookies, nil
}

// UpdateLastUsed updates the last_used timestamp for a cookie
func (r *AuthCookieRepository) UpdateLastUsed(id int64) error {
	query := `
		UPDATE auth_cookies
		SET last_used = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to update last_used: %w", err)
	}

	return nil
}

// Delete deletes an auth cookie by ID
func (r *AuthCookieRepository) Delete(id int64) error {
	query := `DELETE FROM auth_cookies WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete auth cookie: %w", err)
	}

	return nil
}

// DeleteByCookieHash deletes an auth cookie by its hash
func (r *AuthCookieRepository) DeleteByCookieHash(cookieHash string) error {
	query := `DELETE FROM auth_cookies WHERE cookie_hash = ?`

	_, err := r.db.Exec(query, cookieHash)
	if err != nil {
		return fmt.Errorf("failed to delete auth cookie: %w", err)
	}

	return nil
}

// DeleteByUserID deletes all cookies for a user
func (r *AuthCookieRepository) DeleteByUserID(userID int64) error {
	query := `DELETE FROM auth_cookies WHERE user_id = ?`

	result, err := r.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete auth cookies: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no auth cookies found for user %d", userID)
	}

	return nil
}

// DeleteByMagicLinkID deletes all cookies associated with a magic link
func (r *AuthCookieRepository) DeleteByMagicLinkID(magicLinkID int64) error {
	query := `DELETE FROM auth_cookies WHERE magic_link_id = ?`

	_, err := r.db.Exec(query, magicLinkID)
	if err != nil {
		return fmt.Errorf("failed to delete auth cookies by magic link: %w", err)
	}

	return nil
}

// DeleteExpired removes all expired cookies
func (r *AuthCookieRepository) DeleteExpired() error {
	query := `DELETE FROM auth_cookies WHERE expires_at < CURRENT_TIMESTAMP`

	result, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to delete expired cookies: %w", err)
	}

	rows, _ := result.RowsAffected()
	// Return count of deleted rows (can be 0)
	_ = rows

	return nil
}

// CountByUserID returns the number of cookies for a user
func (r *AuthCookieRepository) CountByUserID(userID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM auth_cookies WHERE user_id = ?`
	err := r.db.QueryRow(query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count auth cookies: %w", err)
	}
	return count, nil
}

// CountActiveByUserID returns the number of non-expired cookies for a user
func (r *AuthCookieRepository) CountActiveByUserID(userID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM auth_cookies WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP`
	err := r.db.QueryRow(query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active auth cookies: %w", err)
	}
	return count, nil
}

// IsExpired checks if a cookie is expired
func (r *AuthCookieRepository) IsExpired(id int64) (bool, error) {
	var expiresAt time.Time
	query := `SELECT expires_at FROM auth_cookies WHERE id = ?`
	err := r.db.QueryRow(query, id).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return true, fmt.Errorf("cookie not found")
	}
	if err != nil {
		return true, fmt.Errorf("failed to check expiration: %w", err)
	}

	return time.Now().After(expiresAt), nil
}
