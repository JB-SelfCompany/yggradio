package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CryptoUtil provides cryptographic utility functions
type CryptoUtil struct{}

// NewCryptoUtil creates a new crypto utility
func NewCryptoUtil() *CryptoUtil {
	return &CryptoUtil{}
}

// GenerateKeyPair generates a new Ed25519 key pair
func (c *CryptoUtil) GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pubkey, privkey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	return pubkey, privkey, nil
}

// SignMessage signs a message with a private key
func (c *CryptoUtil) SignMessage(privateKey ed25519.PrivateKey, message string) string {
	signature := ed25519.Sign(privateKey, []byte(message))
	return hex.EncodeToString(signature)
}

// VerifySignature verifies a signature
func (c *CryptoUtil) VerifySignature(pubkeyHex, message, signatureHex string) bool {
	// Decode public key
	pubkey, err := hex.DecodeString(pubkeyHex)
	if err != nil || len(pubkey) != ed25519.PublicKeySize {
		return false
	}

	// Decode signature
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}

	// Verify
	return ed25519.Verify(pubkey, []byte(message), signature)
}

// HashPassword creates a SHA256 hash of a password (for basic auth)
func (c *CryptoUtil) HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// VerifyPassword verifies a password against a hash using constant-time comparison
// SECURITY: Uses subtle.ConstantTimeCompare to prevent timing attacks
func (c *CryptoUtil) VerifyPassword(password, hash string) bool {
	computedHash := c.HashPassword(password)

	// SECURITY: Use constant-time comparison to prevent timing attacks
	// Even if hashes are different lengths, we compare safely
	return subtle.ConstantTimeCompare([]byte(computedHash), []byte(hash)) == 1
}

// GenerateRandomToken generates a random hex token
func (c *CryptoUtil) GenerateRandomToken(byteLength int) (string, error) {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}

// HashFile calculates SHA256 hash of a file
func (c *CryptoUtil) HashFile(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// HashBytes calculates SHA256 hash of bytes
func (c *CryptoUtil) HashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// LoadOrGenerateKeyPair loads a key pair from file or generates a new one
func (c *CryptoUtil) LoadOrGenerateKeyPair(keyPath string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	// Try to load existing key
	if _, err := os.Stat(keyPath); err == nil {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read key file: %w", err)
		}

		// Key file contains hex-encoded private key
		privkeyBytes, err := hex.DecodeString(string(data))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode key: %w", err)
		}

		if len(privkeyBytes) != ed25519.PrivateKeySize {
			return nil, nil, errors.New("invalid key size")
		}

		privkey := ed25519.PrivateKey(privkeyBytes)
		pubkey := privkey.Public().(ed25519.PublicKey)

		return pubkey, privkey, nil
	}

	// Generate new key pair
	pubkey, privkey, err := c.GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}

	// Ensure directory exists
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create key directory: %w", err)
	}

	// Save private key
	keyData := hex.EncodeToString(privkey)
	if err := os.WriteFile(keyPath, []byte(keyData), 0600); err != nil {
		return nil, nil, fmt.Errorf("failed to save key: %w", err)
	}

	return pubkey, privkey, nil
}

// PubkeyToHex converts a public key to hex string
func (c *CryptoUtil) PubkeyToHex(pubkey ed25519.PublicKey) string {
	return hex.EncodeToString(pubkey)
}

// HexToPubkey converts a hex string to public key
func (c *CryptoUtil) HexToPubkey(hexStr string) (ed25519.PublicKey, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex: %w", err)
	}

	if len(bytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid public key size")
	}

	return ed25519.PublicKey(bytes), nil
}

// ConstantTimeCompare performs constant-time comparison of two strings
func (c *CryptoUtil) ConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}

	return result == 0
}
