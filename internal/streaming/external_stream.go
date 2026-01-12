package streaming

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// ExternalStreamManager manages external stream sources (HLS and direct)
type ExternalStreamManager struct {
	hlsParser  *HLSParser
	httpClient *http.Client
	logger     *log.Logger

	// Track active streams to prevent duplicates
	activeStreams map[string]context.CancelFunc
	mu            sync.Mutex
}

// NewExternalStreamManager creates a new external stream manager
func NewExternalStreamManager(logger *log.Logger, timeout time.Duration) *ExternalStreamManager {
	// For streaming connections, we need different timeout strategies:
	// - DialTimeout: timeout for establishing TCP connection
	// - TLSHandshakeTimeout: timeout for TLS handshake
	// - ResponseHeaderTimeout: timeout for receiving response headers
	// - NO Client.Timeout: allows infinite streaming (body read never times out)
	// This prevents "context deadline exceeded" errors during long-running streams
	return &ExternalStreamManager{
		hlsParser: NewHLSParser(timeout),
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second, // Timeout for establishing connection
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second, // Timeout for TLS handshake
				ResponseHeaderTimeout: timeout,          // Timeout for receiving headers
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   10,
			},
			// NO Timeout set here - allows infinite streaming
		},
		logger:        logger,
		activeStreams: make(map[string]context.CancelFunc),
	}
}

// StartHLSStream starts streaming an HLS playlist to a mountpoint
// Returns a cancel function to stop the stream
func (m *ExternalStreamManager) StartHLSStream(ctx context.Context, playlistURL, mountpoint string, server *Server) error {
	m.mu.Lock()
	// Check if stream is already running for this mountpoint
	if cancel, exists := m.activeStreams[mountpoint]; exists {
		m.mu.Unlock()
		// Stop existing stream first
		cancel()
		// Wait a bit for cleanup
		time.Sleep(100 * time.Millisecond)
		m.mu.Lock()
	}

	// Create cancellable context for this stream
	streamCtx, cancel := context.WithCancel(ctx)
	m.activeStreams[mountpoint] = cancel
	m.mu.Unlock()

	// Start streaming in goroutine
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.activeStreams, mountpoint)
			m.mu.Unlock()
			m.logger.Printf("[HLS] Stopped streaming %s to %s", playlistURL, mountpoint)
		}()

		m.logger.Printf("[HLS] Starting stream from %s to %s", playlistURL, mountpoint)

		// Track seen segments to avoid re-downloading
		// Use segment filename (not full URI) as key since query params change
		seenSegmentFiles := make(map[string]bool)
		var lastMediaSequence int
		var detectedContentType string
		var initSegmentSent bool

		// Refresh interval (default 1x target duration, min 3 seconds)
		refreshInterval := 3 * time.Second

		// Get or create mountpoint (will update content type after first segment)
		mp := server.mpManager.GetOrCreate(mountpoint, 0, "", "", "audio/aac", 0)

		// Create pipe for streaming segments (ONCE, not per iteration)
		pr, pw := io.Pipe()

		// Set pipe as source (content type will be updated after first segment)
		success, err := mp.TrySetSource(pr, "audio/aac")
		if !success || err != nil {
			m.logger.Printf("[HLS] Could not set source for mountpoint %s: %v", mountpoint, err)
			pw.Close()
			pr.Close()
			return
		}

		// Update station status to online
		if err := server.db.UpdateStationStatus(mountpoint, "online"); err != nil {
			m.logger.Printf("[HLS] Warning: Could not update station status to online: %v", err)
		} else {
			m.logger.Printf("[HLS] Station %s is now ONLINE", mountpoint)
		}

		// Cleanup pipe on exit
		defer func() {
			pw.Close()
			// Update station status to offline when stream stops
			if err := server.db.UpdateStationStatus(mountpoint, "offline"); err != nil {
				m.logger.Printf("[HLS] Warning: Could not update station status to offline: %v", err)
			} else {
				m.logger.Printf("[HLS] Station %s is now OFFLINE", mountpoint)
			}
		}()

		for {
			select {
			case <-streamCtx.Done():
				return
			default:
			}

			// Fetch playlist
			playlist, err := m.hlsParser.FetchPlaylist(streamCtx, playlistURL)
			if err != nil {
				m.logger.Printf("[HLS] Error fetching playlist %s: %v", playlistURL, err)
				// Wait before retry
				time.Sleep(refreshInterval)
				continue
			}

			// Update refresh interval based on target duration
			if playlist.TargetDuration > 0 {
				refreshInterval = playlist.TargetDuration
				if refreshInterval < 3*time.Second {
					refreshInterval = 3 * time.Second
				}
			}

			// Download and store initialization segment (for fMP4)
			if !initSegmentSent && playlist.InitSegmentURI != "" {
				m.logger.Printf("[HLS] Downloading initialization segment: %s", playlist.InitSegmentURI)
				initData, err := m.hlsParser.FetchSegment(streamCtx, playlist.InitSegmentURI)
				if err != nil {
					m.logger.Printf("[HLS] Error fetching init segment: %v", err)
				} else {
					m.logger.Printf("[HLS] Downloaded init segment - %d bytes", len(initData))
					// Detect content type from init segment (fMP4 format)
					detectedContentType = "audio/mp4"
					m.logger.Printf("[HLS] Detected fMP4 format, using content type: %s", detectedContentType)
					mp.SetContentType(detectedContentType)

					// Store init segment in mountpoint (will be sent to every new listener)
					mp.SetInitSegment(initData)
					initSegmentSent = true
					m.logger.Printf("[HLS] Initialization segment stored for late-joining listeners")
				}
			}

			// Download and stream new segments
			newSegmentCount := 0
			for _, segment := range playlist.Segments {
				// Check context before each segment
				select {
				case <-streamCtx.Done():
					return
				default:
				}

				// Extract filename from segment URI (ignore query params)
				// Example: http://host/path/segment.aac?param=value -> segment.aac
				segmentFilename := path.Base(segment.URI)
				// Remove query string if present
				if idx := strings.Index(segmentFilename, "?"); idx != -1 {
					segmentFilename = segmentFilename[:idx]
				}

				// Skip already seen segments
				if seenSegmentFiles[segmentFilename] {
					continue
				}

				// Skip old segments (before last known sequence)
				if segment.Sequence < lastMediaSequence {
					continue
				}

				// Download segment
				data, err := m.hlsParser.FetchSegment(streamCtx, segment.URI)
				if err != nil {
					m.logger.Printf("[HLS] Error fetching segment %s: %v", segment.URI, err)
					continue
				}

				m.logger.Printf("[HLS] Downloaded segment %d (%s) - %d bytes", segment.Sequence, segmentFilename, len(data))

				// Detect content type from first segment if not yet detected
				// Don't override if already detected from init segment (fMP4)
				if detectedContentType == "" {
					ext := strings.ToLower(path.Ext(segmentFilename))
					switch ext {
					case ".m4s", ".m4a":
						// fMP4 segment - should have init segment
						detectedContentType = "audio/mp4"
					case ".aac":
						detectedContentType = "audio/aac"
					case ".mp3":
						detectedContentType = "audio/mpeg"
					case ".ts":
						detectedContentType = "video/mp2t"
					default:
						detectedContentType = "audio/aac" // Default to AAC
					}
					m.logger.Printf("[HLS] Detected content type: %s (from extension: %s)", detectedContentType, ext)
					// Update mountpoint content type
					mp.SetContentType(detectedContentType)
				}

				// Write segment data to pipe
				_, err = pw.Write(data)
				if err != nil {
					m.logger.Printf("[HLS] Error writing segment to pipe: %v", err)
					return
				}

				// Mark segment as seen (by filename)
				seenSegmentFiles[segmentFilename] = true
				lastMediaSequence = segment.Sequence
				newSegmentCount++

				// Cleanup old segments from memory (keep last 1000)
				if len(seenSegmentFiles) > 1000 {
					// Remove oldest entries (simple cleanup strategy)
					for k := range seenSegmentFiles {
						delete(seenSegmentFiles, k)
						if len(seenSegmentFiles) <= 500 {
							break
						}
					}
				}
			}

			// Log only if new segments were processed
			if newSegmentCount > 0 {
				m.logger.Printf("[HLS] Processed %d new segments", newSegmentCount)
			}

			// If VOD (not live), exit after all segments
			if playlist.EndList {
				m.logger.Printf("[HLS] VOD playlist complete for %s", mountpoint)
				return
			}

			// Wait before refreshing playlist
			time.Sleep(refreshInterval)
		}
	}()

	return nil
}

// StartDirectStream starts streaming a direct audio stream to a mountpoint
// Returns a cancel function to stop the stream
func (m *ExternalStreamManager) StartDirectStream(ctx context.Context, streamURL, mountpoint, contentType string, server *Server) error {
	m.mu.Lock()
	// Check if stream is already running for this mountpoint
	if cancel, exists := m.activeStreams[mountpoint]; exists {
		m.mu.Unlock()
		// Stop existing stream first
		cancel()
		// Wait a bit for cleanup
		time.Sleep(100 * time.Millisecond)
		m.mu.Lock()
	}

	// Create cancellable context for this stream
	streamCtx, cancel := context.WithCancel(ctx)
	m.activeStreams[mountpoint] = cancel
	m.mu.Unlock()

	// Start streaming in goroutine
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.activeStreams, mountpoint)
			m.mu.Unlock()

			// Update station status to offline when stream stops
			if err := server.db.UpdateStationStatus(mountpoint, "offline"); err != nil {
				m.logger.Printf("[DirectStream] Warning: Could not update station status to offline: %v", err)
			} else {
				m.logger.Printf("[DirectStream] Station %s is now OFFLINE", mountpoint)
			}

			m.logger.Printf("[DirectStream] Stopped streaming %s to %s", streamURL, mountpoint)
		}()

		m.logger.Printf("[DirectStream] Starting stream from %s to %s", streamURL, mountpoint)

		// Retry loop with exponential backoff
		maxRetries := 3
		retryDelay := 5 * time.Second

		for retry := 0; retry < maxRetries; retry++ {
			select {
			case <-streamCtx.Done():
				return
			default:
			}

			// Create HTTP request
			req, err := http.NewRequestWithContext(streamCtx, "GET", streamURL, nil)
			if err != nil {
				m.logger.Printf("[DirectStream] Error creating request: %v", err)
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
				continue
			}

			// Request Icecast metadata
			req.Header.Set("Icy-MetaData", "1")

			// Execute request
			resp, err := m.httpClient.Do(req)
			if err != nil {
				m.logger.Printf("[DirectStream] Error connecting to %s: %v", streamURL, err)
				time.Sleep(retryDelay)
				retryDelay *= 2
				continue
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				m.logger.Printf("[DirectStream] Unexpected status code %d from %s", resp.StatusCode, streamURL)
				time.Sleep(retryDelay)
				retryDelay *= 2
				continue
			}

			// Get or create mountpoint with content type
			mp := server.mpManager.GetOrCreate(mountpoint, 0, "", "", contentType, 0)

			// Create pipe for streaming
			pr, pw := io.Pipe()

			// Set pipe as source
			success, err := mp.TrySetSource(pr, contentType)
			if !success || err != nil {
				resp.Body.Close()
				pw.Close()
				pr.Close()
				m.logger.Printf("[DirectStream] Could not set source for mountpoint %s: %v", mountpoint, err)
				time.Sleep(retryDelay)
				retryDelay *= 2
				continue
			}

			// Update station status to online
			if err := server.db.UpdateStationStatus(mountpoint, "online"); err != nil {
				m.logger.Printf("[DirectStream] Warning: Could not update station status to online: %v", err)
			} else {
				m.logger.Printf("[DirectStream] Station %s is now ONLINE", mountpoint)
			}

			// Stream data from HTTP response to pipe
			_, err = io.Copy(pw, resp.Body)

			// Cleanup
			resp.Body.Close()
			pw.Close()

			// Check for errors
			if err != nil {
				if streamCtx.Err() != nil {
					// Context cancelled, exit gracefully
					return
				}
				m.logger.Printf("[DirectStream] Stream error: %v, retrying...", err)
				time.Sleep(retryDelay)
				retryDelay *= 2
				continue
			}

			// Stream ended normally (shouldn't happen for live streams)
			m.logger.Printf("[DirectStream] Stream %s ended", streamURL)
			return
		}

		m.logger.Printf("[DirectStream] Max retries exceeded for %s", streamURL)
	}()

	return nil
}

// StopStream stops an active external stream for a mountpoint
func (m *ExternalStreamManager) StopStream(mountpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.activeStreams[mountpoint]; exists {
		cancel()
		delete(m.activeStreams, mountpoint)
		m.logger.Printf("[ExternalStream] Stopped stream for %s", mountpoint)
	}
}

// IsStreamActive checks if a stream is currently active for a mountpoint
func (m *ExternalStreamManager) IsStreamActive(mountpoint string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.activeStreams[mountpoint]
	return exists
}

// StopAll stops all active external streams
func (m *ExternalStreamManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for mountpoint, cancel := range m.activeStreams {
		cancel()
		m.logger.Printf("[ExternalStream] Stopped stream for %s", mountpoint)
	}
	m.activeStreams = make(map[string]context.CancelFunc)
}
