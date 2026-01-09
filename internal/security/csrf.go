package security

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// CSRFManager manages CSRF tokens
type CSRFManager struct {
	tokens sync.Map // map[string]*csrfToken
	ttl    time.Duration
}

// csrfToken represents a CSRF token
type csrfToken struct {
	value     string
	pubkey    string
	createdAt time.Time
}

// NewCSRFManager creates a new CSRF manager
func NewCSRFManager(ttl time.Duration) *CSRFManager {
	cm := &CSRFManager{
		ttl: ttl,
	}

	// Start cleanup goroutine
	go cm.cleanup()

	return cm
}

// GenerateToken generates a new CSRF token for a user
func (cm *CSRFManager) GenerateToken(pubkey string) (string, error) {
	// Generate random 32 bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	tokenValue := hex.EncodeToString(randomBytes)

	token := &csrfToken{
		value:     tokenValue,
		pubkey:    pubkey,
		createdAt: time.Now(),
	}

	cm.tokens.Store(tokenValue, token)

	return tokenValue, nil
}

// VerifyToken verifies a CSRF token
func (cm *CSRFManager) VerifyToken(tokenValue, pubkey string) error {
	tokenInterface, exists := cm.tokens.Load(tokenValue)
	if !exists {
		return errors.New("invalid token")
	}

	token := tokenInterface.(*csrfToken)

	// Check if token belongs to this user
	if token.pubkey != pubkey {
		return errors.New("token does not belong to this user")
	}

	// Check if token is expired
	if time.Since(token.createdAt) > cm.ttl {
		cm.tokens.Delete(tokenValue)
		return errors.New("token expired")
	}

	// One-time use token - delete after verification
	cm.tokens.Delete(tokenValue)

	return nil
}

// cleanup periodically removes expired tokens
func (cm *CSRFManager) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		cm.tokens.Range(func(key, value interface{}) bool {
			token := value.(*csrfToken)
			if now.Sub(token.createdAt) > cm.ttl {
				cm.tokens.Delete(key)
			}
			return true
		})
	}
}

// InvalidateUserTokens invalidates all tokens for a specific user
func (cm *CSRFManager) InvalidateUserTokens(pubkey string) {
	cm.tokens.Range(func(key, value interface{}) bool {
		token := value.(*csrfToken)
		if token.pubkey == pubkey {
			cm.tokens.Delete(key)
		}
		return true
	})
}

// Count returns the number of active tokens
func (cm *CSRFManager) Count() int {
	count := 0
	cm.tokens.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Close is a no-op for CSRFManager (included for consistency with other managers)
func (cm *CSRFManager) Close() {
	// No cleanup needed for CSRFManager
	// Cleanup goroutine will stop naturally when the program exits
}
