package federation_server

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/utils"
	"gopkg.in/yaml.v3"
)

// Config represents the federation server configuration
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Federation FederationConfig `yaml:"federation"`
	RateLimit  RateLimitConfig  `yaml:"rate_limit"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Port         int           `yaml:"port"`
	Bind         string        `yaml:"bind"`
	InstanceName string        `yaml:"instance_name"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// DatabaseConfig contains database settings
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// FederationConfig contains federation-specific settings
type FederationConfig struct {
	PullInterval           int `yaml:"pull_interval"`            // Seconds between pulls from nodes
	NodeTimeout            int `yaml:"node_timeout"`             // Seconds before marking node offline
	MaxConsecutiveFailures int `yaml:"max_consecutive_failures"` // Mark offline after N failures
	StationTTL             int `yaml:"station_ttl"`              // Seconds before removing stale stations
	MaxStationsPerNode     int `yaml:"max_stations_per_node"`    // Maximum stations per node
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
	RegistrationsPerHour int `yaml:"registrations_per_hour"`
	QueriesPerMinute     int `yaml:"queries_per_minute"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	Output     string `yaml:"output"`
	LogFile    string `yaml:"log_file"` // Path to log file
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
			// Expand paths before saving and returning
			cfg.ExpandPaths()
			if err := Save(cfg, configPath); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
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
	return `# YggRadio Federation Server Configuration

# HTTP Server Configuration
server:
  port: ` + fmt.Sprintf("%d", cfg.Server.Port) + `
  bind: "` + cfg.Server.Bind + `"  # Listen on all IPv6 interfaces (includes Yggdrasil)
  instance_name: "` + cfg.Server.InstanceName + `"
  read_timeout: ` + cfg.Server.ReadTimeout.String() + `
  write_timeout: ` + cfg.Server.WriteTimeout.String() + `
  idle_timeout: ` + cfg.Server.IdleTimeout.String() + `

# Database Configuration
database:
  # Database will be created automatically if it doesn't exist
  path: "` + cfg.Database.Path + `"

# Federation Configuration
federation:
  # Pull interval: How often to pull station lists from registered nodes (seconds)
  pull_interval: ` + fmt.Sprintf("%d", cfg.Federation.PullInterval) + `  # 5 minutes

  # Node timeout: Mark nodes offline if not seen within this time (seconds)
  node_timeout: ` + fmt.Sprintf("%d", cfg.Federation.NodeTimeout) + `  # 1 minute

  # Max consecutive failures: Mark node offline after N failed pull attempts
  max_consecutive_failures: ` + fmt.Sprintf("%d", cfg.Federation.MaxConsecutiveFailures) + `

  # Station TTL: Remove stations not seen within this time (seconds)
  station_ttl: ` + fmt.Sprintf("%d", cfg.Federation.StationTTL) + `  # 1 hour

  # Max stations per node: Limit stations per node to prevent abuse
  max_stations_per_node: ` + fmt.Sprintf("%d", cfg.Federation.MaxStationsPerNode) + `

# Rate Limiting Configuration
rate_limit:
  # Registrations per hour: Maximum node registrations per IP per hour
  registrations_per_hour: ` + fmt.Sprintf("%d", cfg.RateLimit.RegistrationsPerHour) + `

  # Queries per minute: Maximum station/node queries per IP per minute
  queries_per_minute: ` + fmt.Sprintf("%d", cfg.RateLimit.QueriesPerMinute) + `

# Logging Configuration
logging:
  level: "` + cfg.Logging.Level + `"  # debug, info, warn, error
  format: "` + cfg.Logging.Format + `"
  output: "` + cfg.Logging.Output + `"
  log_file: "` + cfg.Logging.LogFile + `"
`
}

// ExpandPaths expands ~ in file paths to home directory
func (c *Config) ExpandPaths() {
	c.Database.Path = utils.ExpandPath(c.Database.Path)
	c.Logging.LogFile = utils.ExpandPath(c.Logging.LogFile)
}

// Validate validates configuration values
func (c *Config) Validate() error {
	// Validate server port
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	// Validate pull interval
	if c.Federation.PullInterval < 60 {
		return fmt.Errorf("pull_interval must be at least 60 seconds")
	}

	// Validate node timeout
	if c.Federation.NodeTimeout < 10 {
		return fmt.Errorf("node_timeout must be at least 10 seconds")
	}

	// Validate max consecutive failures
	if c.Federation.MaxConsecutiveFailures < 1 {
		return fmt.Errorf("max_consecutive_failures must be at least 1")
	}

	// Validate station TTL
	if c.Federation.StationTTL < 300 {
		return fmt.Errorf("station_ttl must be at least 300 seconds")
	}

	// Validate max stations per node
	if c.Federation.MaxStationsPerNode < 1 || c.Federation.MaxStationsPerNode > 1000 {
		return fmt.Errorf("max_stations_per_node must be between 1 and 1000")
	}

	// Validate rate limits
	if c.RateLimit.RegistrationsPerHour < 1 {
		return fmt.Errorf("registrations_per_hour must be at least 1")
	}
	if c.RateLimit.QueriesPerMinute < 1 {
		return fmt.Errorf("queries_per_minute must be at least 1")
	}

	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         9000,
			Bind:         "[::]",
			InstanceName: "YggRadio Federation Server",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Database: DatabaseConfig{
			Path: "~/.yggradio-federation/federation.db",
		},
		Federation: FederationConfig{
			PullInterval:           300,  // 5 minutes
			NodeTimeout:            60,   // 1 minute
			MaxConsecutiveFailures: 3,
			StationTTL:             3600, // 1 hour
			MaxStationsPerNode:     100,
		},
		RateLimit: RateLimitConfig{
			RegistrationsPerHour: 10,
			QueriesPerMinute:     60,
		},
		Logging: LoggingConfig{
			Level:   "info",
			Format:  "json",
			Output:  "stdout",
			LogFile: "~/.yggradio-federation/federation.log",
		},
	}
}
