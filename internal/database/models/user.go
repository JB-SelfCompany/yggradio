package models

import (
	"database/sql"
	"fmt"
	"time"
)

// User represents a user in the system (privacy-first, anonymous)
type User struct {
	ID        int64
	Pubkey    sql.NullString // Nullable for magic link users
	CreatedAt time.Time
	IsAdmin   bool
}

// UserRepository provides database operations for users
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByPubkey retrieves a user by their public key
func (r *UserRepository) GetByPubkey(pubkey string) (*User, error) {
	query := `
		SELECT id, pubkey, created_at, is_admin
		FROM users WHERE pubkey = ?
	`

	var u User
	err := r.db.QueryRow(query, pubkey).Scan(
		&u.ID, &u.Pubkey, &u.CreatedAt, &u.IsAdmin,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &u, nil
}

// Create creates a new user with a public key (Ed25519 authentication)
func (r *UserRepository) Create(pubkey string) (*User, error) {
	query := `
		INSERT INTO users (pubkey)
		VALUES (?)
		RETURNING id, pubkey, created_at, is_admin
	`

	var u User
	err := r.db.QueryRow(query, pubkey).Scan(
		&u.ID, &u.Pubkey, &u.CreatedAt, &u.IsAdmin,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &u, nil
}

// IsAdmin checks if a user is an admin
func (r *UserRepository) IsAdmin(pubkey string) (bool, error) {
	var isAdmin bool
	query := `SELECT is_admin FROM users WHERE pubkey = ?`
	err := r.db.QueryRow(query, pubkey).Scan(&isAdmin)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check admin status: %w", err)
	}

	return isAdmin, nil
}

// GetOrCreate gets an existing user or creates a new one
func (r *UserRepository) GetOrCreate(pubkey string) (*User, error) {
	user, err := r.GetByPubkey(pubkey)
	if err != nil {
		return nil, err
	}

	if user != nil {
		return user, nil
	}

	return r.Create(pubkey)
}

// GetByID retrieves a user by their ID
func (r *UserRepository) GetByID(id int64) (*User, error) {
	query := `
		SELECT id, pubkey, created_at, is_admin
		FROM users WHERE id = ?
	`

	var u User
	err := r.db.QueryRow(query, id).Scan(
		&u.ID, &u.Pubkey, &u.CreatedAt, &u.IsAdmin,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	return &u, nil
}

// CreateAnonymous creates a new user without a public key (for magic link authentication)
func (r *UserRepository) CreateAnonymous() (*User, error) {
	query := `
		INSERT INTO users (pubkey)
		VALUES (NULL)
		RETURNING id, pubkey, created_at, is_admin
	`

	var u User
	err := r.db.QueryRow(query).Scan(
		&u.ID, &u.Pubkey, &u.CreatedAt, &u.IsAdmin,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create anonymous user: %w", err)
	}

	return &u, nil
}
