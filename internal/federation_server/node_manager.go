package federation_server

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"time"
)

// NodeManager handles node registration and health tracking
type NodeManager struct {
	db     *DB
	config *Config
	logger *log.Logger
}

// NewNodeManager creates a new node manager
func NewNodeManager(db *DB, config *Config, logger *log.Logger) *NodeManager {
	return &NodeManager{
		db:     db,
		config: config,
		logger: logger,
	}
}

// RegisterNode processes a node registration request with Ed25519 signature verification
func (nm *NodeManager) RegisterNode(req *RegistrationRequest) (*Node, error) {
	// 1. Validate input fields
	if err := nm.validateRegistrationRequest(req); err != nil {
		nm.logger.Printf("Registration validation failed: %v", err)
		nm.db.LogSecurityEvent("node_registration_failed", "medium", "", req.Pubkey, "/api/federation/register", fmt.Sprintf("validation: %v", err))
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 2. Verify Ed25519 signature
	if err := nm.verifySignature(req); err != nil {
		nm.logger.Printf("Registration signature verification failed: %v", err)
		nm.db.LogSecurityEvent("node_registration_failed", "high", "", req.Pubkey, "/api/federation/register", "invalid signature")
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	// 3. Check timestamp (replay protection)
	now := time.Now().Unix()
	timestampDiff := now - req.Timestamp
	if timestampDiff < 0 {
		timestampDiff = -timestampDiff
	}
	if timestampDiff > 300 { // 5 minute window
		nm.logger.Printf("Registration timestamp out of window: %d seconds difference", timestampDiff)
		nm.db.LogSecurityEvent("node_registration_failed", "medium", "", req.Pubkey, "/api/federation/register", "timestamp out of window")
		return nil, fmt.Errorf("timestamp out of valid window")
	}

	// 4. Register node in database
	node, err := nm.db.RegisterNode(req)
	if err != nil {
		nm.logger.Printf("Failed to register node in database: %v", err)
		nm.db.LogSecurityEvent("node_registration_failed", "low", "", req.Pubkey, "/api/federation/register", fmt.Sprintf("database error: %v", err))
		return nil, fmt.Errorf("failed to register node: %w", err)
	}

	nm.logger.Printf("Node registered successfully: %s (pubkey: %s...)", req.Name, req.Pubkey[:16])
	nm.db.LogSecurityEvent("node_registered", "low", "", req.Pubkey, "/api/federation/register", fmt.Sprintf("name: %s, uuid: %s", req.Name, req.UUID))

	return node, nil
}

// validateRegistrationRequest validates all input fields
func (nm *NodeManager) validateRegistrationRequest(req *RegistrationRequest) error {
	// Validate UUID format
	if !isValidUUID(req.UUID) {
		return fmt.Errorf("invalid UUID format")
	}

	// Validate IPv6 address
	if !isValidIPv6(req.Address) {
		return fmt.Errorf("invalid IPv6 address")
	}

	// Validate port
	if req.Port < 1 || req.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", req.Port)
	}

	// Validate pubkey (Ed25519 = 32 bytes = 64 hex chars)
	if len(req.Pubkey) != 64 {
		return fmt.Errorf("invalid pubkey length")
	}
	if !isHexString(req.Pubkey) {
		return fmt.Errorf("pubkey must be hex-encoded")
	}

	// Validate name
	if len(req.Name) < 3 || len(req.Name) > 100 {
		return fmt.Errorf("name must be 3-100 characters")
	}

	// Validate description (optional, max 5000 chars)
	if len(req.Description) > 5000 {
		return fmt.Errorf("description too long")
	}

	// Validate version (optional, max 50 chars)
	if len(req.Version) > 50 {
		return fmt.Errorf("version string too long")
	}

	// Validate signature
	if len(req.Signature) != 128 { // Ed25519 signature = 64 bytes = 128 hex chars
		return fmt.Errorf("invalid signature length")
	}
	if !isHexString(req.Signature) {
		return fmt.Errorf("signature must be hex-encoded")
	}

	// Validate timestamp
	if req.Timestamp <= 0 {
		return fmt.Errorf("invalid timestamp")
	}

	return nil
}

// verifySignature verifies the Ed25519 signature on the registration request
func (nm *NodeManager) verifySignature(req *RegistrationRequest) error {
	// Decode public key
	pubkeyBytes, err := hex.DecodeString(req.Pubkey)
	if err != nil {
		return fmt.Errorf("failed to decode pubkey: %w", err)
	}
	if len(pubkeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid pubkey size")
	}

	// Decode signature
	signatureBytes, err := hex.DecodeString(req.Signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}
	if len(signatureBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size")
	}

	// Construct message to verify (same format as YggRadio: UUID+ADDRESS+PORT+TIMESTAMP)
	message := fmt.Sprintf("%s%s%d%d", req.UUID, req.Address, req.Port, req.Timestamp)

	// Verify signature using constant-time comparison
	pubkey := ed25519.PublicKey(pubkeyBytes)
	valid := ed25519.Verify(pubkey, []byte(message), signatureBytes)

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte{boolToByte(valid)}, []byte{1}) != 1 {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// MarkNodeOffline marks nodes as offline if they haven't been seen recently
func (nm *NodeManager) MarkNodeOffline(nodeUUID string) error {
	query := `
		UPDATE federation_nodes
		SET status = 'offline'
		WHERE uuid = ? AND status != 'blocked'
	`
	_, err := nm.db.Exec(query, nodeUUID)
	if err != nil {
		return fmt.Errorf("failed to mark node offline: %w", err)
	}

	nm.logger.Printf("Marked node offline: %s", nodeUUID)
	nm.db.LogSecurityEvent("node_marked_offline", "low", "", "", "", fmt.Sprintf("node_uuid: %s", nodeUUID))

	return nil
}

// MarkStaleNodesOffline marks nodes as offline if they haven't been seen within timeout
func (nm *NodeManager) MarkStaleNodesOffline() error {
	timeout := nm.config.Federation.NodeTimeout

	query := `
		UPDATE federation_nodes
		SET status = 'offline'
		WHERE status = 'active'
		  AND last_seen < datetime('now', '-' || ? || ' seconds')
		  AND status != 'blocked'
	`

	result, err := nm.db.Exec(query, timeout)
	if err != nil {
		return fmt.Errorf("failed to mark stale nodes offline: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		nm.logger.Printf("Marked %d stale nodes as offline (timeout: %ds)", rows, timeout)
	}

	return nil
}

// BlockNode blocks a node (prevents further registration)
func (nm *NodeManager) BlockNode(nodeUUID string, reason string) error {
	query := `
		UPDATE federation_nodes
		SET status = 'blocked'
		WHERE uuid = ?
	`
	_, err := nm.db.Exec(query, nodeUUID)
	if err != nil {
		return fmt.Errorf("failed to block node: %w", err)
	}

	nm.logger.Printf("Blocked node: %s (reason: %s)", nodeUUID, reason)
	nm.db.LogSecurityEvent("node_blocked", "high", "", "", "", fmt.Sprintf("node_uuid: %s, reason: %s", nodeUUID, reason))

	return nil
}

// UnblockNode unblocks a previously blocked node
func (nm *NodeManager) UnblockNode(nodeUUID string) error {
	query := `
		UPDATE federation_nodes
		SET status = 'pending'
		WHERE uuid = ?
	`
	_, err := nm.db.Exec(query, nodeUUID)
	if err != nil {
		return fmt.Errorf("failed to unblock node: %w", err)
	}

	nm.logger.Printf("Unblocked node: %s", nodeUUID)
	nm.db.LogSecurityEvent("node_unblocked", "low", "", "", "", fmt.Sprintf("node_uuid: %s", nodeUUID))

	return nil
}

// === Helper Functions ===

// isValidUUID validates UUID format (simple check)
func isValidUUID(uuid string) bool {
	if len(uuid) != 36 {
		return false
	}
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	pattern := regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	return pattern.MatchString(uuid)
}

// isValidIPv6 validates IPv6 address format (Yggdrasil range: 200::/7)
func isValidIPv6(addr string) bool {
	// Basic validation - should start with Yggdrasil prefix
	// Yggdrasil addresses are in 200::/7 range (200: to 3ff:)
	if len(addr) < 3 {
		return false
	}

	// Accept full IPv6 format or compressed format
	// This is a simple check - actual validation happens when connecting
	pattern := regexp.MustCompile(`^[0-9a-fA-F:]+$`)
	return pattern.MatchString(addr)
}

// isHexString checks if a string contains only hex characters
func isHexString(s string) bool {
	pattern := regexp.MustCompile(`^[a-fA-F0-9]+$`)
	return pattern.MatchString(s)
}

// boolToByte converts bool to byte (1 or 0) for constant-time comparison
func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
