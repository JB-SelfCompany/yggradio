package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// MagicLinkManager handles magic link and cookie-based authentication
type MagicLinkManager struct {
	db          *sql.DB
	auditLogger *AuditLogger
}

// NewMagicLinkManager creates a new magic link manager
func NewMagicLinkManager(db *sql.DB, auditLogger *AuditLogger) *MagicLinkManager {
	return &MagicLinkManager{
		db:          db,
		auditLogger: auditLogger,
	}
}

// GenerateMagicLink creates a new magic link token for a user
// Returns the plaintext token (48 hex chars) - caller must send this to user ONCE
// Stores SHA256 hash in database
func (m *MagicLinkManager) GenerateMagicLink(userID int64, ipv6, userAgent string) (string, error) {
	// SECURITY: Generate 24 random bytes (192 bits entropy)
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		m.auditLogger.Log(
			"magic_link_generation_failed",
			SeverityCritical,
			"",
			"",
			fmt.Sprintf("Failed to generate random token: %v", err),
		)
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Convert to hex (48 characters)
	token := hex.EncodeToString(tokenBytes)

	// SECURITY: Hash the token with SHA256 before storage
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Store hash in database
	query := `
		INSERT INTO magic_links (token_hash, user_id, ipv6_created, user_agent)
		VALUES (?, ?, ?, ?)
	`
	_, err := m.db.Exec(query, tokenHash, userID, ipv6, userAgent)
	if err != nil {
		m.auditLogger.Log(
			"magic_link_storage_failed",
			SeverityHigh,
			"",
			"",
			fmt.Sprintf("Failed to store magic link for user %d: %v", userID, err),
		)
		return "", fmt.Errorf("failed to store magic link: %w", err)
	}

	// Audit log success
	m.auditLogger.LogWithMetadata(
		"magic_link_created",
		SeverityMedium,
		"",
		"",
		fmt.Sprintf("Magic link created for user %d", userID),
		map[string]string{
			"user_id":    fmt.Sprintf("%d", userID),
			"user_agent": userAgent,
		},
	)

	// Return plaintext token (ONLY time it's returned)
	return token, nil
}

// ValidateMagicLink validates a magic link token and returns the associated magic link ID and user ID
// This updates the last_used timestamp
func (m *MagicLinkManager) ValidateMagicLink(token string, ipv6 string) (magicLinkID int64, userID int64, err error) {
	// SECURITY: Validate token format (must be 48 hex characters)
	if len(token) != 48 {
		m.auditLogger.Log(
			"magic_link_invalid_format",
			SeverityMedium,
			"",
			"",
			fmt.Sprintf("Invalid token format: length %d", len(token)),
		)
		return 0, 0, fmt.Errorf("invalid token format")
	}

	// Validate hex encoding
	if _, err := hex.DecodeString(token); err != nil {
		m.auditLogger.Log(
			"magic_link_invalid_encoding",
			SeverityMedium,
			"",
			"",
			"Invalid token encoding",
		)
		return 0, 0, fmt.Errorf("invalid token encoding")
	}

	// SECURITY: Hash the token to compare with stored hash
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Query database for active magic link
	query := `
		SELECT id, user_id, token_hash
		FROM magic_links
		WHERE token_hash = ? AND is_active = 1
	`

	var storedHash string
	err = m.db.QueryRow(query, tokenHash).Scan(&magicLinkID, &userID, &storedHash)
	if err == sql.ErrNoRows {
		m.auditLogger.Log(
			EventAuthFailure,
			SeverityHigh,
			"",
			"",
			"Magic link validation failed: token not found or inactive",
		)
		return 0, 0, fmt.Errorf("invalid or expired magic link")
	}
	if err != nil {
		m.auditLogger.Log(
			"magic_link_db_error",
			SeverityCritical,
			"",
			"",
			fmt.Sprintf("Database error during validation: %v", err),
		)
		return 0, 0, fmt.Errorf("failed to validate magic link: %w", err)
	}

	// SECURITY: Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(tokenHash), []byte(storedHash)) != 1 {
		m.auditLogger.Log(
			EventAuthFailure,
			SeverityCritical,
			"",
			"",
			"Magic link hash mismatch - possible attack",
		)
		return 0, 0, fmt.Errorf("invalid magic link")
	}

	// Update last_used timestamp
	updateQuery := `UPDATE magic_links SET last_used = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := m.db.Exec(updateQuery, magicLinkID); err != nil {
		// Log but don't fail - this is not critical
		m.auditLogger.Log(
			"magic_link_update_failed",
			SeverityLow,
			"",
			"",
			fmt.Sprintf("Failed to update last_used for magic link %d", magicLinkID),
		)
	}

	// Audit log successful validation
	m.auditLogger.LogWithMetadata(
		EventAuthSuccess,
		SeverityLow,
		"",
		"",
		fmt.Sprintf("Magic link validated for user %d", userID),
		map[string]string{
			"user_id":       fmt.Sprintf("%d", userID),
			"magic_link_id": fmt.Sprintf("%d", magicLinkID),
		},
	)

	return magicLinkID, userID, nil
}

// CreateCookie generates a session cookie for an authenticated user
// Returns plaintext cookie value (64 hex chars) and expiration time
// Stores SHA256 hash in database
func (m *MagicLinkManager) CreateCookie(magicLinkID, userID int64, ipv6, userAgent string) (string, time.Time, error) {
	// SECURITY: Generate 32 random bytes (256 bits entropy)
	cookieBytes := make([]byte, 32)
	if _, err := rand.Read(cookieBytes); err != nil {
		m.auditLogger.Log(
			"cookie_generation_failed",
			SeverityCritical,
			"",
			"",
			fmt.Sprintf("Failed to generate random cookie: %v", err),
		)
		return "", time.Time{}, fmt.Errorf("failed to generate random cookie: %w", err)
	}

	// Convert to hex (64 characters)
	cookieValue := hex.EncodeToString(cookieBytes)

	// SECURITY: Hash the cookie with SHA256 before storage
	hash := sha256.Sum256([]byte(cookieValue))
	cookieHash := hex.EncodeToString(hash[:])

	// Calculate expiration (1 week from now)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Store hash in database
	query := `
		INSERT INTO auth_cookies (cookie_hash, magic_link_id, user_id, expires_at, ipv6_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := m.db.Exec(query, cookieHash, magicLinkID, userID, expiresAt, ipv6, userAgent)
	if err != nil {
		m.auditLogger.Log(
			"cookie_storage_failed",
			SeverityHigh,
			"",
			"",
			fmt.Sprintf("Failed to store cookie for user %d: %v", userID, err),
		)
		return "", time.Time{}, fmt.Errorf("failed to store cookie: %w", err)
	}

	// Audit log success
	m.auditLogger.LogWithMetadata(
		"cookie_created",
		SeverityLow,
		"",
		"",
		fmt.Sprintf("Session cookie created for user %d", userID),
		map[string]string{
			"user_id":       fmt.Sprintf("%d", userID),
			"magic_link_id": fmt.Sprintf("%d", magicLinkID),
			"expires_at":    expiresAt.Format(time.RFC3339),
			"user_agent":    userAgent,
		},
	)

	// Return plaintext cookie and expiration
	return cookieValue, expiresAt, nil
}

// ValidateCookie validates a session cookie and returns the user ID
// This updates the last_used timestamp
func (m *MagicLinkManager) ValidateCookie(cookieValue string, ipv6 string) (int64, error) {
	// SECURITY: Validate cookie format (must be 64 hex characters)
	if len(cookieValue) != 64 {
		m.auditLogger.Log(
			"cookie_invalid_format",
			SeverityMedium,
			"",
			"",
			fmt.Sprintf("Invalid cookie format: length %d", len(cookieValue)),
		)
		return 0, fmt.Errorf("invalid cookie format")
	}

	// Validate hex encoding
	if _, err := hex.DecodeString(cookieValue); err != nil {
		m.auditLogger.Log(
			"cookie_invalid_encoding",
			SeverityMedium,
			"",
			"",
			"Invalid cookie encoding",
		)
		return 0, fmt.Errorf("invalid cookie encoding")
	}

	// SECURITY: Hash the cookie to compare with stored hash
	hash := sha256.Sum256([]byte(cookieValue))
	cookieHash := hex.EncodeToString(hash[:])

	// Query database for valid cookie
	query := `
		SELECT id, user_id, cookie_hash
		FROM auth_cookies
		WHERE cookie_hash = ? AND expires_at > CURRENT_TIMESTAMP
	`

	var cookieID int64
	var userID int64
	var storedHash string

	err := m.db.QueryRow(query, cookieHash).Scan(&cookieID, &userID, &storedHash)
	if err == sql.ErrNoRows {
		m.auditLogger.Log(
			EventAuthFailure,
			SeverityMedium,
			"",
			"",
			"Cookie validation failed: not found or expired",
		)
		return 0, fmt.Errorf("invalid or expired cookie")
	}
	if err != nil {
		m.auditLogger.Log(
			"cookie_db_error",
			SeverityCritical,
			"",
			"",
			fmt.Sprintf("Database error during cookie validation: %v", err),
		)
		return 0, fmt.Errorf("failed to validate cookie: %w", err)
	}

	// SECURITY: Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(cookieHash), []byte(storedHash)) != 1 {
		m.auditLogger.Log(
			EventAuthFailure,
			SeverityCritical,
			"",
			"",
			"Cookie hash mismatch - possible attack",
		)
		return 0, fmt.Errorf("invalid cookie")
	}

	// Update last_used timestamp
	updateQuery := `UPDATE auth_cookies SET last_used = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := m.db.Exec(updateQuery, cookieID); err != nil {
		// Log but don't fail - this is not critical
		m.auditLogger.Log(
			"cookie_update_failed",
			SeverityLow,
			"",
			"",
			fmt.Sprintf("Failed to update last_used for cookie %d", cookieID),
		)
	}

	return userID, nil
}

// DeleteCookie deletes a session cookie (logout)
func (m *MagicLinkManager) DeleteCookie(cookieValue string, ipv6 string) error {
	// SECURITY: Hash the cookie value
	hash := sha256.Sum256([]byte(cookieValue))
	cookieHash := hex.EncodeToString(hash[:])

	// Delete from database
	query := `DELETE FROM auth_cookies WHERE cookie_hash = ?`
	result, err := m.db.Exec(query, cookieHash)
	if err != nil {
		m.auditLogger.Log(
			"cookie_delete_failed",
			SeverityMedium,
			"",
			"",
			fmt.Sprintf("Failed to delete cookie: %v", err),
		)
		return fmt.Errorf("failed to delete cookie: %w", err)
	}

	rows, _ := result.RowsAffected()
	m.auditLogger.LogWithMetadata(
		"cookie_deleted",
		SeverityLow,
		"",
		"",
		"Session cookie deleted (logout)",
		map[string]string{
			"rows_affected": fmt.Sprintf("%d", rows),
		},
	)

	return nil
}

// CleanupExpiredCookies removes expired cookies from the database
// This should be called periodically (e.g., daily cron job)
func (m *MagicLinkManager) CleanupExpiredCookies() error {
	query := `DELETE FROM auth_cookies WHERE expires_at < CURRENT_TIMESTAMP`
	result, err := m.db.Exec(query)
	if err != nil {
		m.auditLogger.Log(
			"cookie_cleanup_failed",
			SeverityMedium,
			"",
			"",
			fmt.Sprintf("Failed to cleanup expired cookies: %v", err),
		)
		return fmt.Errorf("failed to cleanup expired cookies: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		m.auditLogger.LogWithMetadata(
			"cookie_cleanup_completed",
			SeverityLow,
			"",
			"",
			"Expired cookies cleaned up",
			map[string]string{
				"cookies_deleted": fmt.Sprintf("%d", rows),
			},
		)
	}

	return nil
}

// DeactivateMagicLink deactivates a magic link (prevents future use)
func (m *MagicLinkManager) DeactivateMagicLink(magicLinkID int64, ipv6 string) error {
	query := `UPDATE magic_links SET is_active = 0 WHERE id = ?`
	_, err := m.db.Exec(query, magicLinkID)
	if err != nil {
		m.auditLogger.Log(
			"magic_link_deactivate_failed",
			SeverityMedium,
			"",
			"",
			fmt.Sprintf("Failed to deactivate magic link %d: %v", magicLinkID, err),
		)
		return fmt.Errorf("failed to deactivate magic link: %w", err)
	}

	m.auditLogger.LogWithMetadata(
		"magic_link_deactivated",
		SeverityMedium,
		"",
		"",
		"Magic link deactivated",
		map[string]string{
			"magic_link_id": fmt.Sprintf("%d", magicLinkID),
		},
	)

	return nil
}

// GetActiveMagicLinksForUser returns all active magic links for a user
func (m *MagicLinkManager) GetActiveMagicLinksForUser(userID int64) ([]MagicLinkInfo, error) {
	query := `
		SELECT id, created_at, last_used, ipv6_created, user_agent
		FROM magic_links
		WHERE user_id = ? AND is_active = 1
		ORDER BY created_at DESC
	`

	rows, err := m.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query magic links: %w", err)
	}
	defer rows.Close()

	var links []MagicLinkInfo
	for rows.Next() {
		var link MagicLinkInfo
		err := rows.Scan(&link.ID, &link.CreatedAt, &link.LastUsed, &link.IPv6Created, &link.UserAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to scan magic link: %w", err)
		}
		links = append(links, link)
	}

	return links, nil
}

// GetActiveCookiesForUser returns all active cookies for a user
func (m *MagicLinkManager) GetActiveCookiesForUser(userID int64) ([]CookieInfo, error) {
	query := `
		SELECT id, created_at, expires_at, last_used, ipv6_address, user_agent
		FROM auth_cookies
		WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC
	`

	rows, err := m.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query cookies: %w", err)
	}
	defer rows.Close()

	var cookies []CookieInfo
	for rows.Next() {
		var cookie CookieInfo
		err := rows.Scan(&cookie.ID, &cookie.CreatedAt, &cookie.ExpiresAt, &cookie.LastUsed, &cookie.IPv6Address, &cookie.UserAgent)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cookie: %w", err)
		}
		cookies = append(cookies, cookie)
	}

	return cookies, nil
}

// MagicLinkInfo contains information about a magic link (without the token)
type MagicLinkInfo struct {
	ID          int64
	CreatedAt   time.Time
	LastUsed    time.Time
	IPv6Created sql.NullString
	UserAgent   sql.NullString
}

// CookieInfo contains information about an auth cookie (without the cookie value)
type CookieInfo struct {
	ID          int64
	CreatedAt   time.Time
	ExpiresAt   time.Time
	LastUsed    time.Time
	IPv6Address sql.NullString
	UserAgent   sql.NullString
}
