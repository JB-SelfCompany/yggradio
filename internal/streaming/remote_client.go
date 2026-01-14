package streaming

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// RemoteClient handles HTTP streaming connections to remote federated nodes
type RemoteClient struct {
	httpClient *http.Client
	validator  *security.Validator
	logger     *log.Logger
}

// RemoteStreamConfig contains configuration for a remote stream connection
type RemoteStreamConfig struct {
	NodeAddress string // Yggdrasil IPv6 address
	NodePort    int    // Remote node's port
	Mountpoint  string // Stream mountpoint path
	Timeout     time.Duration
}

// NewRemoteClient creates a new remote streaming client for Yggdrasil
func NewRemoteClient(validator *security.Validator, logger *log.Logger) *RemoteClient {
	// Create HTTP client optimized for Yggdrasil mesh network
	// Note: Yggdrasil provides end-to-end encryption at the network layer, so HTTPS is not required
	transport := &http.Transport{
		// Dial settings optimized for high-latency mesh network
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second, // Increased for Yggdrasil latency
			KeepAlive: 60 * time.Second,
		}).DialContext,

		// Connection pooling - increased for streaming
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   5, // Increased for multiple listeners
		IdleConnTimeout:       120 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,

		// Enable keep-alives for better performance
		DisableKeepAlives: false,

		// Response header timeout - increased for Yggdrasil latency
		ResponseHeaderTimeout: 30 * time.Second,

		// Disable compression to avoid re-encoding audio streams
		DisableCompression: true,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   0, // No global timeout - use context for per-request timeouts
		// Do not follow redirects - security measure
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &RemoteClient{
		httpClient: httpClient,
		validator:  validator,
		logger:     logger,
	}
}

// StreamReader represents an active stream connection to a remote node
type StreamReader struct {
	response    *http.Response
	body        io.ReadCloser
	contentType string
	metadata    string // Current ICY metadata (StreamTitle)
}

// Read implements io.Reader interface
func (sr *StreamReader) Read(p []byte) (n int, err error) {
	return sr.body.Read(p)
}

// Close closes the stream connection
func (sr *StreamReader) Close() error {
	if sr.body != nil {
		return sr.body.Close()
	}
	return nil
}

// ContentType returns the content type of the stream
func (sr *StreamReader) ContentType() string {
	return sr.contentType
}

// GetMetadata returns the current ICY metadata (StreamTitle)
func (sr *StreamReader) GetMetadata() string {
	return sr.metadata
}

// SetMetadata updates the current ICY metadata (StreamTitle)
func (sr *StreamReader) SetMetadata(metadata string) {
	sr.metadata = metadata
}

// Connect establishes a streaming connection to a remote federated station
// Returns a StreamReader for reading audio data, or an error
func (rc *RemoteClient) Connect(ctx context.Context, config *RemoteStreamConfig) (*StreamReader, error) {
	// Security validation: ensure node address is a valid Yggdrasil IPv6
	if err := rc.validator.ValidateYggdrasilIPv6(config.NodeAddress); err != nil {
		rc.logger.Printf("SECURITY: Invalid Yggdrasil IPv6 address rejected: %s - %v", config.NodeAddress, err)
		return nil, fmt.Errorf("invalid node address: %w", err)
	}

	// Security validation: ensure mountpoint is safe (prevent path traversal)
	if err := rc.validator.ValidateMountpoint(config.Mountpoint); err != nil {
		rc.logger.Printf("SECURITY: Invalid mountpoint rejected: %s - %v", config.Mountpoint, err)
		return nil, fmt.Errorf("invalid mountpoint: %w", err)
	}

	// Validate port range
	if config.NodePort < 1 || config.NodePort > 65535 {
		return nil, fmt.Errorf("invalid port: %d", config.NodePort)
	}

	// Construct URL with IPv6 address in brackets
	// Use HTTP instead of HTTPS for Yggdrasil mesh network (encryption handled by Yggdrasil layer)
	url := fmt.Sprintf("http://[%s]:%d%s", config.NodeAddress, config.NodePort, config.Mountpoint)

	rc.logger.Printf("Connecting to remote stream: %s", url)

	// Create request with context for timeout control
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers for streaming
	req.Header.Set("User-Agent", "YggRadio-Proxy/1.0")
	req.Header.Set("Accept", "audio/mpeg, audio/ogg, audio/opus, audio/aac, audio/flac, */*")
	req.Header.Set("Icy-MetaData", "1") // Request ICY metadata if available

	// Execute request
	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote stream: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("remote stream returned HTTP %d", resp.StatusCode)
	}

	// Get content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg" // Default to MP3 if not specified
	}

	// Parse ICY metadata from headers
	icyName := resp.Header.Get("icy-name")
	icyDescription := resp.Header.Get("icy-description")

	// Construct metadata string (will be updated by proxy as stream progresses)
	initialMetadata := icyName
	if initialMetadata == "" {
		initialMetadata = icyDescription
	}

	rc.logger.Printf("Connected to remote stream: %s (Content-Type: %s, ICY-Name: %s)", url, contentType, icyName)

	return &StreamReader{
		response:    resp,
		body:        resp.Body,
		contentType: contentType,
		metadata:    initialMetadata,
	}, nil
}

// Reconnect attempts to reconnect to a remote stream after disconnection
// Implements exponential backoff for retry logic
func (rc *RemoteClient) Reconnect(ctx context.Context, config *RemoteStreamConfig, maxRetries int) (*StreamReader, error) {
	var lastErr error
	backoff := 1 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			rc.logger.Printf("Reconnection attempt %d/%d after %v", attempt+1, maxRetries, backoff)

			// Wait with exponential backoff
			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Exponential backoff (max 30 seconds)
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}

		// Attempt connection
		stream, err := rc.Connect(ctx, config)
		if err == nil {
			rc.logger.Printf("Reconnection successful after %d attempts", attempt+1)
			return stream, nil
		}

		lastErr = err
		rc.logger.Printf("Reconnection attempt %d failed: %v", attempt+1, err)
	}

	return nil, fmt.Errorf("reconnection failed after %d attempts: %w", maxRetries, lastErr)
}

// Close closes the HTTP client and cleans up resources
func (rc *RemoteClient) Close() error {
	// Close idle connections
	rc.httpClient.CloseIdleConnections()
	return nil
}
