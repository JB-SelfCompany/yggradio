package models

import (
	"database/sql"
	"fmt"
	"time"
)

// Rating represents a user rating for a target (station, etc.)
type Rating struct {
	ID             int64
	TargetType     string
	TargetID       int64
	UserPubkey     string
	UserIPv6       string
	UserIPv6Subnet string
	Rating         int
	CreatedAt      time.Time
}

// RatingStats represents aggregated rating statistics for a target
type RatingStats struct {
	AverageRating float64
	VoteCount     int
	UserRating    *int // Nullable - only set when user has rated
}

// RatingRepository provides database operations for ratings
type RatingRepository struct {
	db *sql.DB
}

// NewRatingRepository creates a new rating repository
func NewRatingRepository(db *sql.DB) *RatingRepository {
	return &RatingRepository{db: db}
}

// UpsertRating creates or updates a rating for a target
// Uses INSERT OR REPLACE to handle both new ratings and updates
// Includes IPv6 address and subnet for spam prevention
func (r *RatingRepository) UpsertRating(targetType string, targetID int64, userPubkey, userIPv6, userIPv6Subnet string, rating int) error {
	// Validate rating range (1-5)
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}

	if userIPv6 == "" || userIPv6Subnet == "" {
		return fmt.Errorf("IPv6 address and subnet are required")
	}

	query := `
		INSERT OR REPLACE INTO ratings (target_type, target_id, user_pubkey, user_ipv6, user_ipv6_subnet, rating, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`

	_, err := r.db.Exec(query, targetType, targetID, userPubkey, userIPv6, userIPv6Subnet, rating)
	if err != nil {
		return fmt.Errorf("failed to upsert rating: %w", err)
	}

	return nil
}

// GetRatingStats retrieves aggregated rating statistics for a target
// Returns average rating and total vote count
func (r *RatingRepository) GetRatingStats(targetType string, targetID int64) (*RatingStats, error) {
	query := `
		SELECT COALESCE(AVG(rating), 0) as avg_rating, COUNT(*) as vote_count
		FROM ratings
		WHERE target_type = ? AND target_id = ?
	`

	var stats RatingStats
	err := r.db.QueryRow(query, targetType, targetID).Scan(&stats.AverageRating, &stats.VoteCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get rating stats: %w", err)
	}

	return &stats, nil
}

// GetUserRating retrieves a specific user's rating for a target
// Returns nil if the user hasn't rated the target
func (r *RatingRepository) GetUserRating(targetType string, targetID int64, userPubkey string) (*int, error) {
	query := `
		SELECT rating
		FROM ratings
		WHERE target_type = ? AND target_id = ? AND user_pubkey = ?
	`

	var rating int
	err := r.db.QueryRow(query, targetType, targetID, userPubkey).Scan(&rating)
	if err == sql.ErrNoRows {
		return nil, nil // User hasn't rated this target
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user rating: %w", err)
	}

	return &rating, nil
}

// GetStationRatings is a convenience method to get rating stats for a station
// Returns stats with optional user rating if userPubkey is provided
func (r *RatingRepository) GetStationRatings(stationID int64, userPubkey string) (*RatingStats, error) {
	// Get aggregated stats
	stats, err := r.GetRatingStats("station", stationID)
	if err != nil {
		return nil, err
	}

	// If user pubkey provided, get their rating
	if userPubkey != "" {
		userRating, err := r.GetUserRating("station", stationID, userPubkey)
		if err != nil {
			return nil, err
		}
		stats.UserRating = userRating
	}

	return stats, nil
}

// CheckIPv6Duplicate checks if a rating already exists from this IPv6 address
// but with a different pubkey (spam prevention)
// Returns true if a duplicate is found
func (r *RatingRepository) CheckIPv6Duplicate(targetType string, targetID int64, userPubkey, userIPv6 string) (bool, error) {
	query := `
		SELECT COUNT(*) FROM ratings
		WHERE target_type = ? AND target_id = ?
		AND user_ipv6 = ? AND user_pubkey != ?
	`

	var count int
	err := r.db.QueryRow(query, targetType, targetID, userIPv6, userPubkey).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check IPv6 duplicate: %w", err)
	}

	return count > 0, nil
}

// CheckSubnetDuplicate checks if a rating already exists from this IPv6 subnet
// but with a different pubkey (spam prevention)
// Returns true if a duplicate is found
func (r *RatingRepository) CheckSubnetDuplicate(targetType string, targetID int64, userPubkey, userIPv6Subnet string) (bool, error) {
	query := `
		SELECT COUNT(*) FROM ratings
		WHERE target_type = ? AND target_id = ?
		AND user_ipv6_subnet = ? AND user_pubkey != ?
	`

	var count int
	err := r.db.QueryRow(query, targetType, targetID, userIPv6Subnet, userPubkey).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check subnet duplicate: %w", err)
	}

	return count > 0, nil
}
