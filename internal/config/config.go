package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/utils"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Streaming  StreamingConfig  `yaml:"streaming"`
	Moderation ModerationConfig `yaml:"moderation"`
	Federation FederationConfig `yaml:"federation"`
	RateLimit  RateLimitConfig  `yaml:"rate_limit"`
	Security   SecurityConfig   `yaml:"security"`
	Logging LoggingConfig `yaml:"logging"`
}


// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Port                int           `yaml:"port"`
	Bind                string        `yaml:"bind"`
	InstanceName        string        `yaml:"instance_name"`
	InstanceDescription string        `yaml:"instance_description"`
	ReadTimeout         time.Duration `yaml:"read_timeout"`
	WriteTimeout        time.Duration `yaml:"write_timeout"`
	IdleTimeout         time.Duration `yaml:"idle_timeout"`
}

// DatabaseConfig contains database settings
type DatabaseConfig struct {
	Path               string        `yaml:"path"`
	MaxConnections     int           `yaml:"max_connections"`
	ConnectionLifetime time.Duration `yaml:"connection_lifetime"`
}


// StreamingConfig contains streaming server settings
type StreamingConfig struct {
	MaxListenersPerStation int           `yaml:"max_listeners_per_station"`
	MaxSourceClients       int           `yaml:"max_source_clients"`
	BufferSize             int           `yaml:"buffer_size"`
	MetadataInterval       int           `yaml:"metadata_interval"`
	SlowClientTimeout      time.Duration `yaml:"slow_client_timeout"`
	ServerSecret           string        `yaml:"server_secret"` // HMAC secret for source password generation
}

// ModerationConfig contains moderation settings
type ModerationConfig struct {
	PoWDifficultyAccount int `yaml:"pow_difficulty_account"`
	PoWDifficultyStation int `yaml:"pow_difficulty_station"`
}

// FederationConfig contains federation settings (Phase 2: Centralized federation)
type FederationConfig struct {
	Enabled          bool   `yaml:"enabled"`
	ServerAddress    string `yaml:"server_address"`
	ServerPort       int    `yaml:"server_port"`
	RegisterInterval int    `yaml:"register_interval"` // Seconds between registration heartbeats
	QueryInterval    int    `yaml:"query_interval"`    // Seconds between station queries
	Timeout          int    `yaml:"timeout"`           // Request timeout in seconds
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
	APIRequestsPerMinute          int `yaml:"api_requests_per_minute"`
	AuthAttemptsPerMinute         int `yaml:"auth_attempts_per_minute"`
	StationCreationPerHour        int `yaml:"station_creation_per_hour"`
	CommentsPerHour               int `yaml:"comments_per_hour"`
	SourceConnectionsPerMinute    int `yaml:"source_connections_per_minute"`
	ListenerConnectionsPerMinute  int `yaml:"listener_connections_per_minute"`
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	CSRFTokenTTL            int  `yaml:"csrf_token_ttl"`
	EnableSecurityAudit     bool `yaml:"enable_security_audit"`
	AuditLogRetention       int  `yaml:"audit_log_retention"`
	AutoBlockOnFailedAuth   bool `yaml:"auto_block_on_failed_auth"`
	FailedAuthThreshold     int  `yaml:"failed_auth_threshold"`
	BlockDuration           int  `yaml:"block_duration"`
	CSPEnabled              bool `yaml:"csp_enabled"`

	// Magic link authentication settings
	MagicLinkEnabled       bool   `yaml:"magic_link_enabled"`
	MagicLinkTokenLength   int    `yaml:"magic_link_token_length"`
	MagicLinkCookieTTL     int    `yaml:"magic_link_cookie_ttl"`
	MagicLinkCookieName    string `yaml:"magic_link_cookie_name"`
	MagicLinkRequirePoW    bool   `yaml:"magic_link_require_pow"`
	MagicLinkPoWDifficulty int    `yaml:"magic_link_pow_difficulty"`
}

// AdminConfig contains admin settings
// DEPRECATED: All authenticated users can manage their own resources
// type AdminConfig struct {
// 	Enabled bool     `yaml:"enabled"`
// 	Pubkeys []string `yaml:"pubkeys"`
// }

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level          string `yaml:"level"`
	Format         string `yaml:"format"`
	Output         string `yaml:"output"`
	SecurityEvents string `yaml:"security_events"`
}

// Load loads configuration from a YAML file
func Load(configPath string) (*Config, error) {
	// Expand home directory if present
	if len(configPath) > 0 && configPath[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, configPath[1:])
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		// If config doesn't exist, create default
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			// Auto-generate secure ServerSecret for new installations
			if err := cfg.GenerateServerSecret(); err != nil {
				return nil, fmt.Errorf("failed to generate server secret: %w", err)
			}
			// Expand paths before saving and returning
			cfg.ExpandPaths()
			if err := Save(cfg, configPath); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// SECURITY: Verify and fix config file permissions (must be 0600 or more restrictive)
	// This prevents other users from reading ServerSecret and other sensitive data
	if err := ensureConfigFilePermissions(configPath); err != nil {
		return nil, fmt.Errorf("failed to secure config file permissions: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// SECURITY: Auto-generate ServerSecret if it's empty or set to the insecure default
	// This protects users who upgraded from old configs, used example config, or manually edited the file
	// IMPORTANT: Only update the file IN-PLACE to preserve comments and formatting
	if cfg.Streaming.ServerSecret == "" || cfg.Streaming.ServerSecret == "CHANGE-ME-TO-RANDOM-SECRET" {
		if err := cfg.GenerateServerSecret(); err != nil {
			return nil, fmt.Errorf("failed to generate server secret: %w", err)
		}
		// Update ONLY the server_secret field in the existing file (preserves comments and formatting)
		if err := updateServerSecretInPlace(configPath, cfg.Streaming.ServerSecret); err != nil {
			return nil, fmt.Errorf("failed to update server secret in config: %w", err)
		}
	}

	// Expand paths
	cfg.ExpandPaths()

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Save saves configuration to a YAML file with detailed comments
func Save(cfg *Config, configPath string) error {
	// Expand home directory if present
	if len(configPath) > 0 && configPath[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, configPath[1:])
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Generate YAML with comments
	data := generateConfigYAML(cfg)

	// Write to file
	if err := os.WriteFile(configPath, []byte(data), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// generateConfigYAML generates YAML configuration with detailed comments
func generateConfigYAML(cfg *Config) string {
	return `# YggRadio Configuration

# Yggdrasil Network Configuration
# YggRadio requires a running Yggdrasil daemon on the local machine.
# No additional configuration needed - the application automatically
# detects the Yggdrasil IPv6 address from network interfaces.
#
# Install Yggdrasil:
#   Linux:   apt install yggdrasil  (or download from yggdrasil-network.github.io)
#   macOS:   brew install yggdrasil-go
#   Windows: https://github.com/yggdrasil-network/yggdrasil-go/releases
#
# Start Yggdrasil:
#   sudo systemctl start yggdrasil
#   # or manually: yggdrasil -useconffile /etc/yggdrasil.conf
#
# Configure Yggdrasil peers in /etc/yggdrasil.conf (not here)
# For best streaming performance, choose peers with low latency (<100ms)
# Find peers: https://github.com/yggdrasil-network/public-peers

# HTTP Server Configuration
server:
  port: ` + fmt.Sprintf("%d", cfg.Server.Port) + `
  bind: "` + cfg.Server.Bind + `"  # Empty or "[::]" = auto-detect and bind to Yggdrasil IPv6 address (tun0 interface)
            # You can specify a custom IPv6 address if needed
  instance_name: "` + cfg.Server.InstanceName + `"
  instance_description: "` + cfg.Server.InstanceDescription + `"
  read_timeout: ` + cfg.Server.ReadTimeout.String() + `
  write_timeout: ` + cfg.Server.WriteTimeout.String() + `
  idle_timeout: ` + cfg.Server.IdleTimeout.String() + `

# Database Configuration
database:
  path: "` + cfg.Database.Path + `"
  max_connections: ` + fmt.Sprintf("%d", cfg.Database.MaxConnections) + `  # SQLite limitation
  connection_lifetime: ` + cfg.Database.ConnectionLifetime.String() + `

# Streaming Server Configuration
streaming:
  max_listeners_per_station: ` + fmt.Sprintf("%d", cfg.Streaming.MaxListenersPerStation) + `
  max_source_clients: ` + fmt.Sprintf("%d", cfg.Streaming.MaxSourceClients) + `

  # These settings minimize bufferbloat and reduce latency
  buffer_size: ` + fmt.Sprintf("%d", cfg.Streaming.BufferSize) + `  # 32KB (reduced from 64KB for lower latency)
  metadata_interval: ` + fmt.Sprintf("%d", cfg.Streaming.MetadataInterval) + `  # 16KB - optimal for ICY metadata
  slow_client_timeout: ` + cfg.Streaming.SlowClientTimeout.String() + `  # Quickly disconnect laggy clients

  # Server secret for source authentication
  # IMPORTANT: Change this to a random secret in production!
  server_secret: "` + cfg.Streaming.ServerSecret + `"

# Moderation Configuration
moderation:
  # Proof-of-Work difficulty (4-20 bits)
  # Higher values = more spam protection, but longer wait time
  # On modern CPU: difficulty N ≈ 2^N attempts
  pow_difficulty_account: ` + fmt.Sprintf("%d", cfg.Moderation.PoWDifficultyAccount) + `  # ~16384 attempts (~2 sec) - account creation is rare
  pow_difficulty_station: ` + fmt.Sprintf("%d", cfg.Moderation.PoWDifficultyStation) + `  # ~262144 attempts (~10-30 sec) - station creation is very important

# Federation Configuration
federation:
  enabled: ` + fmt.Sprintf("%t", cfg.Federation.Enabled) + `  # Default: disabled
  server_address: "` + cfg.Federation.ServerAddress + `"  # IPv6 address of federation server (default: 301:be28:cf55:3c9::10)
  server_port: ` + fmt.Sprintf("%d", cfg.Federation.ServerPort) + `  # Port of federation server
  register_interval: ` + fmt.Sprintf("%d", cfg.Federation.RegisterInterval) + `  # Seconds between registration heartbeats
  query_interval: ` + fmt.Sprintf("%d", cfg.Federation.QueryInterval) + `  # Seconds between station queries
  timeout: ` + fmt.Sprintf("%d", cfg.Federation.Timeout) + `  # Request timeout in seconds

# Rate Limiting Configuration
rate_limit:
  # API requests per minute (per IP)
  api_requests_per_minute: ` + fmt.Sprintf("%d", cfg.RateLimit.APIRequestsPerMinute) + `

  # Authentication attempts per minute (per IP)
  auth_attempts_per_minute: ` + fmt.Sprintf("%d", cfg.RateLimit.AuthAttemptsPerMinute) + `

  # Station creation per hour (per pubkey)
  station_creation_per_hour: ` + fmt.Sprintf("%d", cfg.RateLimit.StationCreationPerHour) + `

  # Comments per hour (per pubkey)
  comments_per_hour: ` + fmt.Sprintf("%d", cfg.RateLimit.CommentsPerHour) + `

  # Source connections per minute (per IP)
  source_connections_per_minute: ` + fmt.Sprintf("%d", cfg.RateLimit.SourceConnectionsPerMinute) + `

  # Listener connections per minute (per IP)
  listener_connections_per_minute: ` + fmt.Sprintf("%d", cfg.RateLimit.ListenerConnectionsPerMinute) + `

# Security Configuration
security:
  # CSRF token TTL (seconds)
  csrf_token_ttl: ` + fmt.Sprintf("%d", cfg.Security.CSRFTokenTTL) + `  # 15 minutes

  # Enable security audit logging
  enable_security_audit: ` + fmt.Sprintf("%t", cfg.Security.EnableSecurityAudit) + `

  # Audit log retention (seconds)
  audit_log_retention: ` + fmt.Sprintf("%d", cfg.Security.AuditLogRetention) + `  # 30 days

  # Auto-block on failed authentication
  auto_block_on_failed_auth: ` + fmt.Sprintf("%t", cfg.Security.AutoBlockOnFailedAuth) + `
  failed_auth_threshold: ` + fmt.Sprintf("%d", cfg.Security.FailedAuthThreshold) + `
  block_duration: ` + fmt.Sprintf("%d", cfg.Security.BlockDuration) + `  # 1 hour

  # Content Security Policy
  csp_enabled: ` + fmt.Sprintf("%t", cfg.Security.CSPEnabled) + `

  # Magic Link Authentication
  magic_link_enabled: ` + fmt.Sprintf("%t", cfg.Security.MagicLinkEnabled) + `
  magic_link_token_length: ` + fmt.Sprintf("%d", cfg.Security.MagicLinkTokenLength) + `  # bytes
  magic_link_cookie_ttl: ` + fmt.Sprintf("%d", cfg.Security.MagicLinkCookieTTL) + `  # seconds
  magic_link_cookie_name: "` + cfg.Security.MagicLinkCookieName + `"
  magic_link_require_pow: ` + fmt.Sprintf("%t", cfg.Security.MagicLinkRequirePoW) + `
  magic_link_pow_difficulty: ` + fmt.Sprintf("%d", cfg.Security.MagicLinkPoWDifficulty) + `

# Logging Configuration
logging:
  level: "` + cfg.Logging.Level + `"  # debug, info, warn, error
  format: "` + cfg.Logging.Format + `"
  output: "` + cfg.Logging.Output + `"
  security_events: "` + cfg.Logging.SecurityEvents + `"
`
}

// ExpandPaths expands ~ in file paths to home directory
func (c *Config) ExpandPaths() {
	c.Database.Path = utils.ExpandPath(c.Database.Path)
	c.Logging.SecurityEvents = utils.ExpandPath(c.Logging.SecurityEvents)
}

// Validate validates configuration values
func (c *Config) Validate() error {
	// No Yggdrasil validation needed - external daemon is required

	// Validate server port
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	// SECURITY: Validate ServerSecret is not set to the insecure default and has proper length
	// Note: Empty string is allowed - it will be auto-generated during Load()
	if c.Streaming.ServerSecret == "CHANGE-ME-TO-RANDOM-SECRET" {
		return fmt.Errorf("ServerSecret must be changed from default value")
	}

	// SECURITY: Validate ServerSecret has minimum length (32 characters for 128-bit security)
	// Skip length check if empty (will be auto-generated)
	if c.Streaming.ServerSecret != "" && len(c.Streaming.ServerSecret) < 32 {
		return fmt.Errorf("ServerSecret must be at least 32 characters long (got %d)", len(c.Streaming.ServerSecret))
	}

	// Validate PoW difficulty ranges
	if c.Moderation.PoWDifficultyAccount < 4 || c.Moderation.PoWDifficultyAccount > 20 {
		return fmt.Errorf("invalid PoW difficulty for accounts: %d (must be 4-20)", c.Moderation.PoWDifficultyAccount)
	}
	if c.Moderation.PoWDifficultyStation < 4 || c.Moderation.PoWDifficultyStation > 20 {
		return fmt.Errorf("invalid PoW difficulty for stations: %d (must be 4-20)", c.Moderation.PoWDifficultyStation)
	}

	// SECURITY: Validate timeouts (1 second to 10 minutes for read, 1 second to 30 minutes for write)
	if c.Server.ReadTimeout < 1*time.Second || c.Server.ReadTimeout > 10*time.Minute {
		return fmt.Errorf("ReadTimeout must be between 1s and 10m, got: %v", c.Server.ReadTimeout)
	}
	if c.Server.WriteTimeout < 1*time.Second || c.Server.WriteTimeout > 30*time.Minute {
		return fmt.Errorf("WriteTimeout must be between 1s and 30m, got: %v", c.Server.WriteTimeout)
	}
	if c.Server.IdleTimeout < 1*time.Second || c.Server.IdleTimeout > 2*time.Hour {
		return fmt.Errorf("IdleTimeout must be between 1s and 2h, got: %v", c.Server.IdleTimeout)
	}

	// SECURITY: Validate buffer sizes (4KB to 1MB to prevent memory exhaustion)
	if c.Streaming.BufferSize < 4096 || c.Streaming.BufferSize > 1048576 {
		return fmt.Errorf("BufferSize must be between 4KB and 1MB, got: %d", c.Streaming.BufferSize)
	}

	// SECURITY: Validate connection limits (prevent resource exhaustion)
	if c.Streaming.MaxListenersPerStation < 1 || c.Streaming.MaxListenersPerStation > 10000 {
		return fmt.Errorf("MaxListenersPerStation must be between 1 and 10000, got: %d", c.Streaming.MaxListenersPerStation)
	}
	if c.Streaming.MaxSourceClients < 1 || c.Streaming.MaxSourceClients > 100 {
		return fmt.Errorf("MaxSourceClients must be between 1 and 100, got: %d", c.Streaming.MaxSourceClients)
	}

	// SECURITY: Validate block duration (1 minute to 7 days to prevent permanent bans from misconfiguration)
	if c.Security.BlockDuration < 60 || c.Security.BlockDuration > 604800 {
		return fmt.Errorf("BlockDuration must be between 1 minute (60s) and 7 days (604800s), got: %d", c.Security.BlockDuration)
	}

	// SECURITY: Validate CSRF token TTL (1 minute to 1 hour for security)
	if c.Security.CSRFTokenTTL < 60 || c.Security.CSRFTokenTTL > 3600 {
		return fmt.Errorf("CSRFTokenTTL must be between 1 minute (60s) and 1 hour (3600s), got: %d", c.Security.CSRFTokenTTL)
	}

	// SECURITY: Validate rate limits are positive and reasonable
	if c.RateLimit.APIRequestsPerMinute < 1 || c.RateLimit.APIRequestsPerMinute > 10000 {
		return fmt.Errorf("APIRequestsPerMinute must be between 1 and 10000, got: %d", c.RateLimit.APIRequestsPerMinute)
	}
	if c.RateLimit.AuthAttemptsPerMinute < 1 || c.RateLimit.AuthAttemptsPerMinute > 100 {
		return fmt.Errorf("AuthAttemptsPerMinute must be between 1 and 100, got: %d", c.RateLimit.AuthAttemptsPerMinute)
	}
	if c.RateLimit.StationCreationPerHour < 1 || c.RateLimit.StationCreationPerHour > 100 {
		return fmt.Errorf("StationCreationPerHour must be between 1 and 100, got: %d", c.RateLimit.StationCreationPerHour)
	}

	// Validate metadata interval is reasonable (1KB to 1MB)
	if c.Streaming.MetadataInterval < 1024 || c.Streaming.MetadataInterval > 1048576 {
		return fmt.Errorf("MetadataInterval must be between 1KB and 1MB, got: %d", c.Streaming.MetadataInterval)
	}

	// SECURITY: Validate magic link settings
	if c.Security.MagicLinkEnabled {
		// Validate token length (16-32 bytes for security)
		if c.Security.MagicLinkTokenLength < 16 || c.Security.MagicLinkTokenLength > 32 {
			return fmt.Errorf("MagicLinkTokenLength must be between 16 and 32 bytes, got: %d", c.Security.MagicLinkTokenLength)
		}

		// Validate cookie TTL (1 hour to 30 days)
		if c.Security.MagicLinkCookieTTL < 3600 || c.Security.MagicLinkCookieTTL > 2592000 {
			return fmt.Errorf("MagicLinkCookieTTL must be between 1 hour (3600s) and 30 days (2592000s), got: %d", c.Security.MagicLinkCookieTTL)
		}

		// Validate cookie name is not empty
		if c.Security.MagicLinkCookieName == "" {
			return fmt.Errorf("MagicLinkCookieName cannot be empty")
		}

		// Validate PoW difficulty if enabled
		if c.Security.MagicLinkRequirePoW {
			if c.Security.MagicLinkPoWDifficulty < 4 || c.Security.MagicLinkPoWDifficulty > 20 {
				return fmt.Errorf("MagicLinkPoWDifficulty must be between 4 and 20, got: %d", c.Security.MagicLinkPoWDifficulty)
			}
		}
	}

	return nil
}

// GenerateServerSecret generates a cryptographically secure random server secret
// This is used for HMAC-based source password generation
func (c *Config) GenerateServerSecret() error {
	// Generate 64 random bytes (512 bits) for high security
	const secretLength = 64
	randomBytes := make([]byte, secretLength)

	// Use crypto/rand for cryptographically secure random generation
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 for safe storage in YAML (86 characters for 64 bytes)
	c.Streaming.ServerSecret = base64.StdEncoding.EncodeToString(randomBytes)

	return nil
}

// updateServerSecretInPlace updates only the server_secret field in the config file
// This preserves all comments, formatting, and other fields exactly as they were
func updateServerSecretInPlace(configPath string, newSecret string) error {
	// Read the entire file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Split into lines
	lines := strings.Split(string(data), "\n")
	updated := false

	// Find and update the server_secret line
	for i, line := range lines {
		// Look for server_secret field (with any amount of whitespace)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "server_secret:") {
			// Preserve indentation
			indent := ""
			for _, ch := range line {
				if ch == ' ' || ch == '\t' {
					indent += string(ch)
				} else {
					break
				}
			}
			// Update the line with new secret, preserving indentation
			lines[i] = fmt.Sprintf(`%sserver_secret: "%s"`, indent, newSecret)
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("server_secret field not found in config file")
	}

	// Join lines back and write to file
	newData := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(newData), 0600); err != nil {
		return fmt.Errorf("failed to write updated config: %w", err)
	}

	return nil
}

// ensureConfigFilePermissions checks and automatically fixes config file permissions
// Sets permissions to 0600 if they are too open (readable by group or others)
func ensureConfigFilePermissions(configPath string) error {
	info, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	mode := info.Mode()
	perm := mode.Perm()

	// SECURITY: Config file must not be readable by group or others (max permissions: 0600)
	// This prevents exposure of ServerSecret and other sensitive configuration
	const groupOtherReadable = 0o077 // Mask for group/other permissions

	if perm&groupOtherReadable != 0 {
		// Permissions are too open - automatically fix them
		if err := os.Chmod(configPath, 0600); err != nil {
			return fmt.Errorf(
				"config file has insecure permissions %o, failed to fix: %w. "+
					"Please run: chmod 0600 %s",
				perm, err, configPath,
			)
		}
		// Successfully fixed permissions - no error needed, this is normal when copying config.example.yaml
	}

	return nil
}
