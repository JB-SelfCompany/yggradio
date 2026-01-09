package models

import (
	"database/sql"
	"time"
)

// ModerationRepository provides database operations for moderation
type ModerationRepository struct {
	db *sql.DB
}

// NewModerationRepository creates a new moderation repository
func NewModerationRepository(db *sql.DB) *ModerationRepository {
	return &ModerationRepository{db: db}
}

// Helper functions for creating nullable types

// NewNullString creates a sql.NullString
func NewNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// NewNullTime creates a sql.NullTime
func NewNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}

// NewNullInt64 creates a sql.NullInt64
func NewNullInt64(n int64) sql.NullInt64 {
	return sql.NullInt64{Int64: n, Valid: true}
}
