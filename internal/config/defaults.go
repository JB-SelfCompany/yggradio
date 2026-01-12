package config

import "time"

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:                8080,
			Bind:                "", // Empty = auto-bind to Yggdrasil IPv6 address (tun0)
			InstanceName:        "YggRadio Instance",
			InstanceDescription: "Decentralized radio platform on Yggdrasil",
			ReadTimeout:         30 * time.Second,
			WriteTimeout:        180 * time.Second, // Increased for streaming over Yggdrasil
			IdleTimeout:         120 * time.Second,
		},
		Database: DatabaseConfig{
			Path:               "~/.yggradio/yggradio.db",
			MaxConnections:     1,
			ConnectionLifetime: 3600 * time.Second,
		},
		Streaming: StreamingConfig{
			MaxListenersPerStation: 100,
			MaxSourceClients:       10,
			BufferSize:             32768,             // Reduced for lower latency (32KB)
			MetadataInterval:       16000,             // 16KB - good balance
			SlowClientTimeout:      10 * time.Second,  // Reduced to quickly disconnect laggy clients
			ServerSecret:           "CHANGE-ME-TO-RANDOM-SECRET", // Should be changed in production
		},
		Moderation: ModerationConfig{
			// SECURITY: Increased from 14->16 and 18->20 for better spam protection
			// Modern GPUs can compute lower difficulties too quickly
			PoWDifficultyAccount: 16, // ~65536 attempts (~4-8 sec on CPU, harder for GPUs)
			PoWDifficultyStation: 20, // ~1048576 attempts (~1-2 min on CPU, significantly harder)
		},
		Federation: FederationConfig{
			Enabled:          false, // Default: disabled (must configure server first)
			ServerAddress:    "301:be28:cf55:3c9::10",
			ServerPort:       9000,
			RegisterInterval: 450,  // 7.5 minutes (server allows 10 registrations/hour, 450s = 8/hour with safety margin)
			QueryInterval:    60,   // 1 minute (station list updates)
			Timeout:          30,   // 30 seconds
		},
		RateLimit: RateLimitConfig{
			APIRequestsPerMinute:         100,
			AuthAttemptsPerMinute:        10,
			StationCreationPerHour:       1,
			CommentsPerHour:              30,
			SourceConnectionsPerMinute:   5,
			ListenerConnectionsPerMinute: 20,
		},
		Security: SecurityConfig{
			CSRFTokenTTL:          900,  // 15 minutes
			EnableSecurityAudit:   true,
			AuditLogRetention:     2592000, // 30 days
			AutoBlockOnFailedAuth: true,
			FailedAuthThreshold:   10,
			BlockDuration:         3600, // 1 hour
			CSPEnabled:            true,

			// Magic link authentication defaults
			MagicLinkEnabled:       true,
			MagicLinkTokenLength:   24,              // 24 bytes = 48 hex chars (192 bits)
			MagicLinkCookieTTL:     604800,          // 1 week
			MagicLinkCookieName:    "yggradio_auth",
			MagicLinkRequirePoW:    true,
			MagicLinkPoWDifficulty: 16,              // Same as account creation
		},
		Logging: LoggingConfig{
			Level:          "info",
			Format:         "json",
			Output:         "stdout",
			SecurityEvents: "~/.yggradio/security.log",
		},
	}
}
