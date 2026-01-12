package streaming

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HLSPlaylist represents a parsed M3U8 playlist
type HLSPlaylist struct {
	IsLive          bool          // true for live, false for VOD
	TargetDuration  time.Duration // Target segment duration
	MediaSequence   int           // Current media sequence number
	Segments        []HLSSegment  // List of media segments
	EndList         bool          // true if #EXT-X-ENDLIST is present (VOD)
	Version         int           // HLS protocol version
	InitSegmentURI  string        // Initialization segment URI (for fMP4)
}

// HLSSegment represents a single media segment
type HLSSegment struct {
	URI      string        // Segment URI (absolute or relative)
	Duration time.Duration // Segment duration
	Sequence int           // Segment sequence number
}

// HLSParser parses HLS playlists
type HLSParser struct {
	httpClient *http.Client
}

// NewHLSParser creates a new HLS parser
func NewHLSParser(timeout time.Duration) *HLSParser {
	return &HLSParser{
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Limit redirect hops to prevent redirect loops
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// FetchPlaylist fetches and parses an HLS playlist from a URL
func (p *HLSParser) FetchPlaylist(ctx context.Context, playlistURL string) (*HLSPlaylist, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", playlistURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Fetch playlist
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse playlist
	playlist, err := p.ParsePlaylist(resp.Body, playlistURL)
	if err != nil {
		// Check if this is a master playlist error
		if strings.HasPrefix(err.Error(), "master_playlist:") {
			// Extract variant URL
			variantURL := strings.TrimPrefix(err.Error(), "master_playlist:")
			// Recursively fetch the media playlist
			return p.FetchPlaylist(ctx, variantURL)
		}
		return nil, fmt.Errorf("failed to parse playlist: %w", err)
	}

	return playlist, nil
}

// ParsePlaylist parses an M3U8 playlist from a reader
func (p *HLSParser) ParsePlaylist(r io.Reader, baseURL string) (*HLSPlaylist, error) {
	playlist := &HLSPlaylist{
		IsLive:   true, // Default to live
		Version:  1,    // Default HLS version
		Segments: make([]HLSSegment, 0),
	}

	scanner := bufio.NewScanner(r)
	var currentDuration time.Duration
	sequence := 0
	var masterPlaylistVariant string // Store first variant URL from master playlist
	var isNextLineVariant bool       // Flag indicating next line is a variant URL

	// Parse base URL for resolving relative URLs
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments (except HLS tags)
		if line == "" || (strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#EXT")) {
			continue
		}

		// Parse HLS tags
		if strings.HasPrefix(line, "#EXTM3U") {
			// Playlist header - required
			continue
		}

		// Check for master playlist variant (HLS adaptive streaming)
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			// This is a master playlist, next line will be a variant URL
			isNextLineVariant = true
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-VERSION:") {
			version, err := strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-VERSION:"))
			if err == nil {
				playlist.Version = version
			}
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			duration, err := strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"))
			if err == nil {
				playlist.TargetDuration = time.Duration(duration) * time.Second
			}
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			seq, err := strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"))
			if err == nil {
				playlist.MediaSequence = seq
				sequence = seq
			}
			continue
		}

		if strings.HasPrefix(line, "#EXTINF:") {
			// Parse segment duration
			parts := strings.Split(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if len(parts) > 0 {
				durationStr := strings.TrimSpace(parts[0])
				duration, err := strconv.ParseFloat(durationStr, 64)
				if err == nil {
					currentDuration = time.Duration(duration * float64(time.Second))
				}
			}
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-ENDLIST") {
			// VOD playlist - has a defined end
			playlist.EndList = true
			playlist.IsLive = false
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-MAP:") {
			// Parse initialization segment (fMP4)
			// Format: #EXT-X-MAP:URI="init.mp4"
			mapTag := strings.TrimPrefix(line, "#EXT-X-MAP:")
			// Extract URI value
			if strings.Contains(mapTag, "URI=\"") {
				start := strings.Index(mapTag, "URI=\"") + len("URI=\"")
				end := strings.Index(mapTag[start:], "\"")
				if end > 0 {
					initURI := mapTag[start : start+end]
					// Resolve relative URL
					initURL, err := base.Parse(initURI)
					if err == nil {
						playlist.InitSegmentURI = initURL.String()
					}
				}
			}
			continue
		}

		// If line doesn't start with #, it's either a segment URI or a variant URL
		if !strings.HasPrefix(line, "#") {
			// Check if this is a variant URL from master playlist
			if isNextLineVariant {
				// Resolve variant URL and store it
				variantURL, err := base.Parse(line)
				if err == nil && masterPlaylistVariant == "" {
					// Store first variant (we'll use it later)
					masterPlaylistVariant = variantURL.String()
				}
				isNextLineVariant = false
				continue
			}

			// Otherwise it's a segment URI
			// Resolve relative URLs
			segmentURL, err := base.Parse(line)
			if err != nil {
				// Skip invalid URLs
				continue
			}

			segment := HLSSegment{
				URI:      segmentURL.String(),
				Duration: currentDuration,
				Sequence: sequence,
			}
			playlist.Segments = append(playlist.Segments, segment)
			sequence++
			currentDuration = 0 // Reset for next segment
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading playlist: %w", err)
	}

	// Check if this is a master playlist (has variants but no segments)
	if len(playlist.Segments) == 0 && masterPlaylistVariant != "" {
		// This is a master playlist, return the variant URL for the caller to fetch
		// We'll handle this in FetchPlaylist by setting a special error
		return nil, fmt.Errorf("master_playlist:%s", masterPlaylistVariant)
	}

	// Validate playlist
	if len(playlist.Segments) == 0 {
		return nil, fmt.Errorf("playlist contains no segments")
	}

	return playlist, nil
}

// FetchSegment fetches a single media segment
func (p *HLSParser) FetchSegment(ctx context.Context, segmentURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", segmentURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch segment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read segment data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read segment: %w", err)
	}

	return data, nil
}

// StreamSegment streams a segment directly to a writer (more memory-efficient)
func (p *HLSParser) StreamSegment(ctx context.Context, segmentURL string, w io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", segmentURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch segment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Stream segment data
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("failed to stream segment: %w", err)
	}

	return n, nil
}
