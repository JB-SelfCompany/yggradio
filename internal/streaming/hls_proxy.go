package streaming

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HLSProxy proxies HLS streams for browser playback
// Instead of concatenating segments, we proxy the original HLS stream
// This allows browsers to use native HLS support or hls.js
type HLSProxy struct {
	httpClient *http.Client
	logger     *log.Logger

	// Track original stream URLs by mountpoint
	streamURLs map[string]string
	mu         sync.RWMutex
}

// NewHLSProxy creates a new HLS proxy
func NewHLSProxy(logger *log.Logger, timeout time.Duration) *HLSProxy {
	return &HLSProxy{
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		logger:     logger,
		streamURLs: make(map[string]string),
	}
}

// RegisterStream registers a stream URL for a mountpoint
func (p *HLSProxy) RegisterStream(mountpoint, streamURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streamURLs[mountpoint] = streamURL
	p.logger.Printf("[HLS Proxy] Registered stream for %s: %s", mountpoint, streamURL)
}

// UnregisterStream removes a stream registration
func (p *HLSProxy) UnregisterStream(mountpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.streamURLs, mountpoint)
	p.logger.Printf("[HLS Proxy] Unregistered stream for %s", mountpoint)
}

// GetStreamURL gets the stream URL for a mountpoint
func (p *HLSProxy) GetStreamURL(mountpoint string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	url, exists := p.streamURLs[mountpoint]
	return url, exists
}

// ProxyPlaylist proxies the HLS playlist and rewrites segment URLs
func (p *HLSProxy) ProxyPlaylist(w http.ResponseWriter, r *http.Request, mountpoint string) {
	// Get original stream URL
	streamURL, exists := p.GetStreamURL(mountpoint)
	if !exists {
		p.logger.Printf("[HLS Proxy] Stream not found for mountpoint: %s", mountpoint)
		http.Error(w, "Stream not found", http.StatusNotFound)
		return
	}

	// Append query parameters from client request (important for hlssid)
	upstreamURL := streamURL
	if r.URL.RawQuery != "" {
		separator := "?"
		if strings.Contains(streamURL, "?") {
			separator = "&"
		}
		upstreamURL = streamURL + separator + r.URL.RawQuery
	}

	// Fetch original playlist
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", upstreamURL, nil)
	if err != nil {
		p.logger.Printf("[HLS Proxy] Error creating request: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Printf("[HLS Proxy] Error fetching playlist: %v", err)
		http.Error(w, "Failed to fetch playlist", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		p.logger.Printf("[HLS Proxy] Unexpected status code: %d", resp.StatusCode)
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}

	// Parse and rewrite playlist
	baseURL, err := url.Parse(streamURL)
	if err != nil {
		p.logger.Printf("[HLS Proxy] Error parsing base URL: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Read and rewrite playlist
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		// Check if this is a master playlist variant (contains .m3u8)
		if !strings.HasPrefix(line, "#") && strings.Contains(line, ".m3u8") {
			// This is a variant URL in master playlist
			// Resolve it to absolute URL
			variantURL, err := baseURL.Parse(line)
			if err != nil {
				p.logger.Printf("[HLS Proxy] Error parsing variant URL '%s': %v", line, err)
				fmt.Fprintf(w, "%s\n", line)
				continue
			}

			// Rewrite to proxy through our server
			// The variant will include hlssid in query string
			proxyURL := fmt.Sprintf("/proxy/hls%s/playlist.m3u8?%s",
				mountpoint,
				variantURL.RawQuery)
			fmt.Fprintf(w, "%s\n", proxyURL)
			continue
		}

		// Check if this is a segment URI (.aac, .ts, .m4s, etc.)
		if !strings.HasPrefix(line, "#") && line != "" {
			// Resolve relative URL
			segmentURL, err := baseURL.Parse(line)
			if err != nil {
				p.logger.Printf("[HLS Proxy] Error parsing segment URL '%s': %v", line, err)
				fmt.Fprintf(w, "%s\n", line)
				continue
			}

			// Rewrite to proxy through our server
			// Format: /proxy/hls/{mountpoint}/segment?url={encoded_url}
			proxyURL := fmt.Sprintf("/proxy/hls%s/segment?url=%s",
				mountpoint,
				url.QueryEscape(segmentURL.String()))
			fmt.Fprintf(w, "%s\n", proxyURL)
			continue
		}

		// Write other lines as-is (tags, comments, etc.)
		fmt.Fprintf(w, "%s\n", line)
	}

	if err := scanner.Err(); err != nil {
		p.logger.Printf("[HLS Proxy] Error reading playlist: %v", err)
	}
}

// ProxySegment proxies a single HLS segment
func (p *HLSProxy) ProxySegment(w http.ResponseWriter, r *http.Request) {
	// Get segment URL from query parameter
	segmentURL := r.URL.Query().Get("url")
	if segmentURL == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	// Fetch segment
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", segmentURL, nil)
	if err != nil {
		p.logger.Printf("[HLS Proxy] Error creating segment request: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Printf("[HLS Proxy] Error fetching segment: %v", err)
		http.Error(w, "Failed to fetch segment", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		p.logger.Printf("[HLS Proxy] Segment fetch status: %d", resp.StatusCode)
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}

	// Add CORS headers first (before copying other headers)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")

	// Copy important headers from upstream
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}

	// Stream segment to client
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		p.logger.Printf("[HLS Proxy] Error streaming segment: %v", err)
	}
}
