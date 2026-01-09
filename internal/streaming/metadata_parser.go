package streaming

import (
	"bytes"
	"io"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/JB-SelfCompany/yggradio/internal/database"
	"github.com/JB-SelfCompany/yggradio/internal/security"
)

// MetadataExtractor wraps an io.Reader and extracts metadata from audio streams
type MetadataExtractor struct {
	source      io.Reader
	db          *database.DB
	mountpoint  string
	sanitizer   *security.Sanitizer
	logger      *log.Logger
	buffer      []byte
	bufferSize  int
	lastMetadata string
}

// NewMetadataExtractor creates a new metadata extractor
func NewMetadataExtractor(
	source io.Reader,
	db *database.DB,
	mountpoint string,
	sanitizer *security.Sanitizer,
	logger *log.Logger,
) *MetadataExtractor {
	return &MetadataExtractor{
		source:     source,
		db:         db,
		mountpoint: mountpoint,
		sanitizer:  sanitizer,
		logger:     logger,
		buffer:     make([]byte, 0, 16384), // 16KB buffer
		bufferSize: 16384,
	}
}

// Read implements io.Reader and extracts metadata on-the-fly
func (m *MetadataExtractor) Read(p []byte) (n int, err error) {
	n, err = m.source.Read(p)
	if n > 0 {
		// Add to buffer for metadata detection
		m.buffer = append(m.buffer, p[:n]...)

		// Keep buffer size limited
		if len(m.buffer) > m.bufferSize*2 {
			m.buffer = m.buffer[len(m.buffer)-m.bufferSize:]
		}

		// Try to extract metadata periodically
		if len(m.buffer) >= m.bufferSize {
			m.tryExtractMetadata()
		}
	}
	return n, err
}

// Close implements io.Closer
func (m *MetadataExtractor) Close() error {
	// If source implements io.Closer, close it
	if closer, ok := m.source.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// tryExtractMetadata attempts to extract metadata from the buffer
func (m *MetadataExtractor) tryExtractMetadata() {
	// Try ID3v2 tags (MP3)
	if metadata := m.extractID3v2(); metadata != "" {
		m.updateMetadata(metadata)
		return
	}

	// Try Vorbis comments (OGG/Opus)
	if metadata := m.extractVorbisComment(); metadata != "" {
		m.updateMetadata(metadata)
		return
	}

	// Try ID3v1 tags (MP3, legacy)
	if metadata := m.extractID3v1(); metadata != "" {
		m.updateMetadata(metadata)
	}
}

// extractID3v2 extracts ID3v2 tags from MP3 stream
func (m *MetadataExtractor) extractID3v2() string {
	// ID3v2 starts with "ID3"
	idx := bytes.Index(m.buffer, []byte("ID3"))
	if idx == -1 {
		return ""
	}

	// Check if we have enough data for header
	if len(m.buffer[idx:]) < 10 {
		return ""
	}

	// Parse ID3v2 header
	header := m.buffer[idx : idx+10]
	if string(header[0:3]) != "ID3" {
		return ""
	}

	// Get version (major.minor)
	majorVersion := header[3]

	// Calculate tag size (synchsafe integer)
	size := int(header[6])<<21 | int(header[7])<<14 | int(header[8])<<7 | int(header[9])

	// Check if we have the full tag
	if len(m.buffer[idx:]) < 10+size {
		return ""
	}

	tagData := m.buffer[idx+10 : idx+10+size]

	// Try to find TIT2 (Title) and TPE1 (Artist) frames
	title := m.findID3Frame(tagData, "TIT2", majorVersion)
	artist := m.findID3Frame(tagData, "TPE1", majorVersion)

	if artist != "" && title != "" {
		return artist + " - " + title
	} else if title != "" {
		return title
	}

	return ""
}

// findID3Frame finds and extracts a specific ID3v2 frame
func (m *MetadataExtractor) findID3Frame(data []byte, frameID string, majorVersion byte) string {
	idx := bytes.Index(data, []byte(frameID))
	if idx == -1 {
		return ""
	}

	// Check if we have enough data for frame header (10 bytes)
	if len(data[idx:]) < 10 {
		return ""
	}

	// Parse frame size
	var frameSize int
	if majorVersion >= 4 {
		// ID3v2.4 uses synchsafe integers for frame size
		frameSize = int(data[idx+4]&0x7F)<<21 | int(data[idx+5]&0x7F)<<14 | int(data[idx+6]&0x7F)<<7 | int(data[idx+7]&0x7F)
	} else {
		// ID3v2.3 and earlier use normal integers
		frameSize = int(data[idx+4])<<24 | int(data[idx+5])<<16 | int(data[idx+6])<<8 | int(data[idx+7])
	}

	// Sanity check frame size
	if frameSize <= 0 || frameSize > 10000 {
		return ""
	}

	// Check if we have enough data for frame content
	if len(data[idx:]) < 10+frameSize {
		return ""
	}

	// Frame data starts after 10-byte header
	frameData := data[idx+10 : idx+10+frameSize]

	// First byte is encoding
	if len(frameData) < 1 {
		return ""
	}

	encoding := frameData[0]
	textData := frameData[1:]

	// Decode based on encoding
	var result string
	switch encoding {
	case 0: // ISO-8859-1
		result = string(textData)
	case 1: // UTF-16 with BOM
		result = m.decodeUTF16(textData)
	case 2: // UTF-16BE without BOM
		result = m.decodeUTF16BE(textData)
	case 3: // UTF-8
		result = string(textData)
	default:
		// Unknown encoding, try as UTF-8
		result = string(textData)
	}

	// Clean up the result
	result = strings.TrimSpace(string(bytes.TrimRight([]byte(result), "\x00")))

	// Validate that result contains printable characters
	if !m.isPrintableText(result) {
		return ""
	}

	return result
}

// decodeUTF16 decodes UTF-16 text with BOM
func (m *MetadataExtractor) decodeUTF16(data []byte) string {
	if len(data) < 2 {
		return ""
	}

	// Check BOM
	if data[0] == 0xFF && data[1] == 0xFE {
		// Little-endian
		return m.decodeUTF16LE(data[2:])
	} else if data[0] == 0xFE && data[1] == 0xFF {
		// Big-endian
		return m.decodeUTF16BE(data[2:])
	}

	// No BOM, assume big-endian (ID3v2 default)
	return m.decodeUTF16BE(data)
}

// decodeUTF16LE decodes UTF-16 little-endian
func (m *MetadataExtractor) decodeUTF16LE(data []byte) string {
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}

	result := make([]rune, 0, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		if i+1 >= len(data) {
			break
		}
		r := rune(data[i]) | rune(data[i+1])<<8
		if r == 0 {
			break
		}
		result = append(result, r)
	}
	return string(result)
}

// decodeUTF16BE decodes UTF-16 big-endian
func (m *MetadataExtractor) decodeUTF16BE(data []byte) string {
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}

	result := make([]rune, 0, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		if i+1 >= len(data) {
			break
		}
		r := rune(data[i])<<8 | rune(data[i+1])
		if r == 0 {
			break
		}
		result = append(result, r)
	}
	return string(result)
}

// isPrintableText validates that a string contains mostly printable characters
func (m *MetadataExtractor) isPrintableText(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Must be valid UTF-8
	if !utf8.ValidString(s) {
		return false
	}

	printable := 0
	total := 0

	for _, r := range s {
		total++
		// Count printable characters (letters, digits, punctuation, spaces)
		if r >= 32 && r < 127 || r >= 160 {
			printable++
		}
	}

	// At least 80% of characters must be printable
	return total > 0 && float64(printable)/float64(total) >= 0.8
}

// extractVorbisComment extracts Vorbis comments from OGG/Opus stream
func (m *MetadataExtractor) extractVorbisComment() string {
	// Look for "OggS" header
	idx := bytes.Index(m.buffer, []byte("OggS"))
	if idx == -1 {
		return ""
	}

	// Simple extraction - look for TITLE and ARTIST tags
	titleIdx := bytes.Index(m.buffer[idx:], []byte("TITLE="))
	artistIdx := bytes.Index(m.buffer[idx:], []byte("ARTIST="))

	var title, artist string

	if titleIdx != -1 {
		titleStart := idx + titleIdx + 6
		// Look for null byte or newline as delimiter
		remaining := m.buffer[titleStart:]
		titleEnd := bytes.IndexAny(remaining, "\x00\n")
		if titleEnd == -1 {
			// Limit to 200 characters if no delimiter found
			titleEnd = 200
			if titleEnd > len(remaining) {
				titleEnd = len(remaining)
			}
		}
		if titleEnd > 0 && titleEnd <= 200 {
			candidate := string(remaining[:titleEnd])
			if m.isPrintableText(candidate) {
				title = strings.TrimSpace(candidate)
			}
		}
	}

	if artistIdx != -1 {
		artistStart := idx + artistIdx + 7
		// Look for null byte or newline as delimiter
		remaining := m.buffer[artistStart:]
		artistEnd := bytes.IndexAny(remaining, "\x00\n")
		if artistEnd == -1 {
			// Limit to 200 characters if no delimiter found
			artistEnd = 200
			if artistEnd > len(remaining) {
				artistEnd = len(remaining)
			}
		}
		if artistEnd > 0 && artistEnd <= 200 {
			candidate := string(remaining[:artistEnd])
			if m.isPrintableText(candidate) {
				artist = strings.TrimSpace(candidate)
			}
		}
	}

	if artist != "" && title != "" {
		return artist + " - " + title
	} else if title != "" {
		return title
	}

	return ""
}

// extractID3v1 extracts ID3v1 tags from MP3 stream (legacy)
func (m *MetadataExtractor) extractID3v1() string {
	// ID3v1 is at the end of file, look for "TAG" marker
	idx := bytes.Index(m.buffer, []byte("TAG"))
	if idx == -1 {
		return ""
	}

	// Check if we have 128 bytes (ID3v1 tag size)
	if len(m.buffer[idx:]) < 128 {
		return ""
	}

	tag := m.buffer[idx : idx+128]
	if string(tag[0:3]) != "TAG" {
		return ""
	}

	// Extract title (bytes 3-32) and artist (bytes 33-62)
	titleBytes := bytes.TrimRight(tag[3:33], "\x00")
	artistBytes := bytes.TrimRight(tag[33:63], "\x00")

	title := strings.TrimSpace(string(titleBytes))
	artist := strings.TrimSpace(string(artistBytes))

	// Validate extracted text
	if title != "" && !m.isPrintableText(title) {
		title = ""
	}
	if artist != "" && !m.isPrintableText(artist) {
		artist = ""
	}

	if artist != "" && title != "" {
		return artist + " - " + title
	} else if title != "" {
		return title
	}

	return ""
}

// updateMetadata updates metadata in the database if it changed
func (m *MetadataExtractor) updateMetadata(metadata string) {
	// Sanitize metadata
	metadata = m.sanitizer.SanitizeString(metadata)

	// Only update if different from last metadata
	if metadata == m.lastMetadata || metadata == "" {
		return
	}

	m.lastMetadata = metadata

	// Update in database
	if err := m.db.UpdateStationMetadata(m.mountpoint, metadata); err != nil {
		m.logger.Printf("Failed to auto-update metadata: %v", err)
		return
	}

	m.logger.Printf("Auto-extracted metadata for %s: %s", m.mountpoint, metadata)
}

// ParseStreamTitle extracts the stream title from ICY metadata string
// Format: StreamTitle='Artist - Song';
func ParseStreamTitle(metadata string) string {
	// Look for StreamTitle=' ... '
	start := strings.Index(metadata, "StreamTitle='")
	if start == -1 {
		return ""
	}

	// Move past "StreamTitle='"
	start += len("StreamTitle='")

	// Find closing quote
	end := strings.Index(metadata[start:], "'")
	if end == -1 {
		// No closing quote found
		return ""
	}

	// Extract title
	title := metadata[start : start+end]
	return strings.TrimSpace(title)
}
