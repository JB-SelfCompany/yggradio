package security

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidLength     = errors.New("invalid length")
	ErrInvalidCharacters = errors.New("invalid characters")
	ErrInvalidFormat     = errors.New("invalid format")
)

// Validator provides input validation functions
type Validator struct{}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateName validates station name
func (v *Validator) ValidateName(name string) error {
	name = strings.TrimSpace(name)

	// Length: 3-100 characters
	if len(name) < 3 || len(name) > 100 {
		return ErrInvalidLength
	}

	// Check UTF-8
	if !utf8.ValidString(name) {
		return ErrInvalidFormat
	}

	return nil
}

// ValidateDescription validates description text
func (v *Validator) ValidateDescription(desc string) error {
	desc = strings.TrimSpace(desc)

	// Maximum 5000 characters
	if len(desc) > 5000 {
		return ErrInvalidLength
	}

	// Check UTF-8
	if !utf8.ValidString(desc) {
		return ErrInvalidFormat
	}

	return nil
}

// ValidateImageURL validates image URL
func (v *Validator) ValidateImageURL(urlStr string) error {
	if urlStr == "" {
		return nil // Optional field
	}

	// Parse URL
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return ErrInvalidFormat
	}

	// Only HTTP/HTTPS
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("only http/https URLs allowed")
	}

	// Maximum length
	if len(urlStr) > 2048 {
		return ErrInvalidLength
	}

	return nil
}

// ValidatePubkey validates Ed25519 public key
func (v *Validator) ValidatePubkey(pubkey string) error {
	// Ed25519 pubkey = 32 bytes = 64 hex characters
	if len(pubkey) != 64 {
		return ErrInvalidLength
	}

	matched, _ := regexp.MatchString(`^[a-fA-F0-9]{64}$`, pubkey)
	if !matched {
		return ErrInvalidCharacters
	}

	return nil
}

// ValidateMountpoint validates stream mountpoint path
func (v *Validator) ValidateMountpoint(path string) error {
	path = strings.TrimSpace(path)

	// Must start with /
	if !strings.HasPrefix(path, "/") {
		return errors.New("mountpoint must start with /")
	}

	// SECURITY: Length 2-64 characters (reduced from 256 for better sanity)
	// 64 chars is more than enough for any reasonable mountpoint name
	if len(path) < 2 || len(path) > 64 {
		return ErrInvalidLength
	}

	// Check for path traversal
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return errors.New("invalid path: path traversal detected")
	}

	// Only allow alphanumeric, dash, underscore, and slash
	matched, _ := regexp.MatchString(`^/[a-zA-Z0-9/_-]+$`, path)
	if !matched {
		return ErrInvalidCharacters
	}

	return nil
}

// ValidateContentType validates audio content type
func (v *Validator) ValidateContentType(contentType string, allowedTypes []string) error {
	contentType = strings.ToLower(strings.TrimSpace(contentType))

	for _, allowed := range allowedTypes {
		if contentType == strings.ToLower(allowed) {
			return nil
		}
	}

	return errors.New("unsupported content type")
}

// ValidateFileSize validates file size
func (v *Validator) ValidateFileSize(size, maxSize int64) error {
	if size <= 0 {
		return errors.New("file size must be positive")
	}

	if size > maxSize {
		return errors.New("file size exceeds maximum allowed")
	}

	return nil
}

// ValidateRating validates rating value
func (v *Validator) ValidateRating(rating int) error {
	if rating < 1 || rating > 5 {
		return errors.New("rating must be between 1 and 5")
	}
	return nil
}

// ValidateIPv6 validates IPv6 address format
func (v *Validator) ValidateIPv6(ip string) error {
	// Basic IPv6 format check
	matched, _ := regexp.MatchString(`^([0-9a-fA-F]{0,4}:){7}[0-9a-fA-F]{0,4}$`, ip)
	if !matched {
		// Try compressed format
		matched, _ = regexp.MatchString(`^(([0-9a-fA-F]{1,4}:){1,7}|:):([0-9a-fA-F]{1,4}:){0,6}[0-9a-fA-F]{1,4}$`, ip)
		if !matched {
			return ErrInvalidFormat
		}
	}
	return nil
}

// ValidateYggdrasilIPv6 validates that the IPv6 address is within Yggdrasil's 200::/7 range
// This prevents SSRF attacks by only allowing connections to Yggdrasil mesh network addresses
// SECURITY: Uses proper CIDR range checking instead of string prefix matching to prevent
// accepting non-Yggdrasil IPv6 addresses like 2001:db8:: or 2002::
func (v *Validator) ValidateYggdrasilIPv6(ip string) error {
	// Parse as net.IP for proper validation
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return errors.New("invalid IPv6 address format")
	}

	// Ensure it's IPv6 (not IPv4)
	if parsedIP.To4() != nil {
		return errors.New("IPv4 addresses not allowed")
	}

	// Yggdrasil uses 200::/7 range
	// This means first 7 bits must be: 0000010 (binary)
	// In hex: 0x0200 to 0x03FF
	_, yggdrasilNet, err := net.ParseCIDR("200::/7")
	if err != nil {
		// This should never happen with hardcoded valid CIDR
		return errors.New("internal error: invalid Yggdrasil CIDR")
	}

	if !yggdrasilNet.Contains(parsedIP) {
		return errors.New("not a Yggdrasil IPv6 address (must be in 200::/7 range)")
	}

	// SECURITY: Reject localhost
	if parsedIP.IsLoopback() {
		return errors.New("localhost addresses not allowed")
	}

	// SECURITY: Reject unspecified address (::)
	if parsedIP.IsUnspecified() {
		return errors.New("unspecified addresses not allowed")
	}

	return nil
}

// GetIPv6Subnet extracts the /64 subnet from an IPv6 address for rate limiting
// SECURITY: Returns standardized subnet representation to prevent rate limit bypasses
// Yggdrasil typically uses /64 subnets, so we use this for grouping rate limits
func (v *Validator) GetIPv6Subnet(ip string) (string, error) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", errors.New("invalid IPv6 address")
	}

	// Ensure it's IPv6
	if parsedIP.To4() != nil {
		return "", errors.New("IPv4 addresses not supported for subnet extraction")
	}

	// Use /64 subnet for Yggdrasil (standard subnet size)
	mask := net.CIDRMask(64, 128)
	subnet := parsedIP.Mask(mask)

	return subnet.String(), nil
}

// ValidateUUID validates UUID v4 format
// SECURITY: Ensures only valid UUIDs are passed to database queries
func (v *Validator) ValidateUUID(uuid string) error {
	if uuid == "" {
		return errors.New("UUID cannot be empty")
	}

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// where y is one of [8, 9, a, b]
	matched, _ := regexp.MatchString(
		`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
		strings.ToLower(uuid),
	)
	if !matched {
		return ErrInvalidFormat
	}

	return nil
}

// ValidateDirectoryPath validates a directory path for security issues
// SECURITY: Prevents path traversal attacks in playlist configuration
func (v *Validator) ValidateDirectoryPath(path string) error {
	if path == "" {
		return nil // Optional field
	}

	path = strings.TrimSpace(path)

	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return errors.New("path traversal detected: '..' not allowed")
	}

	// Check for null bytes (used in some path traversal attacks)
	if strings.Contains(path, "\x00") {
		return errors.New("null bytes not allowed in path")
	}

	// Path must be absolute (starts with / on Unix or drive letter on Windows)
	if !strings.HasPrefix(path, "/") && !regexp.MustCompile(`^[a-zA-Z]:\\`).MatchString(path) {
		return errors.New("path must be absolute")
	}

	// Maximum length check
	if len(path) > 4096 {
		return ErrInvalidLength
	}

	return nil
}

// ValidateFilePattern validates a file pattern (glob) for security issues
// SECURITY: Prevents malicious patterns in playlist configuration
func (v *Validator) ValidateFilePattern(pattern string) error {
	if pattern == "" {
		return nil // Optional field
	}

	pattern = strings.TrimSpace(pattern)

	// Check for path traversal in pattern
	if strings.Contains(pattern, "..") {
		return errors.New("path traversal detected in pattern: '..' not allowed")
	}

	// Check for null bytes
	if strings.Contains(pattern, "\x00") {
		return errors.New("null bytes not allowed in pattern")
	}

	// Pattern should not contain absolute paths
	if strings.HasPrefix(pattern, "/") || regexp.MustCompile(`^[a-zA-Z]:\\`).MatchString(pattern) {
		return errors.New("pattern must be relative (no absolute paths)")
	}

	// Maximum length check
	if len(pattern) > 256 {
		return ErrInvalidLength
	}

	// Only allow safe characters in pattern: alphanumeric, wildcards, and common file chars
	// Include braces {} and comma for glob patterns like *.{mp3,ogg,opus}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9*?._,{}\[\]-]+$`, pattern)
	if !matched {
		return ErrInvalidCharacters
	}

	return nil
}

// ValidatePlaylistMode validates playlist playback mode
func (v *Validator) ValidatePlaylistMode(mode string) error {
	if mode == "" {
		return nil // Optional field
	}

	mode = strings.ToLower(strings.TrimSpace(mode))

	// Only allow specific modes
	allowedModes := []string{"sequential", "shuffle"}
	for _, allowed := range allowedModes {
		if mode == allowed {
			return nil
		}
	}

	return errors.New("invalid playlist mode (must be 'sequential' or 'shuffle')")
}

// ValidateAudioMagicBytes validates that data starts with audio format magic bytes
// SECURITY: Prevents content-type spoofing attacks by checking actual file content
// Returns the detected content type or error if not a valid audio format
func (v *Validator) ValidateAudioMagicBytes(data []byte) (string, error) {
	if len(data) < 12 {
		return "", errors.New("insufficient data to determine file type (need at least 12 bytes)")
	}

	// MP3 magic bytes
	// ID3v2 tag: "ID3"
	if len(data) >= 3 && data[0] == 0x49 && data[1] == 0x44 && data[2] == 0x33 {
		return "audio/mpeg", nil
	}
	// MP3 frame sync: 0xFF 0xFB or 0xFF 0xFA or 0xFF 0xF3 or 0xFF 0xF2
	if len(data) >= 2 && data[0] == 0xFF && (data[1]&0xE0) == 0xE0 {
		return "audio/mpeg", nil
	}

	// Ogg Vorbis/Opus magic bytes: "OggS"
	if len(data) >= 4 && data[0] == 0x4F && data[1] == 0x67 && data[2] == 0x67 && data[3] == 0x53 {
		// Further check for Opus vs Vorbis in the stream
		// For now, return generic Ogg
		if len(data) >= 28 && data[28] == 0x4F && data[29] == 0x70 && data[30] == 0x75 && data[31] == 0x73 {
			return "audio/opus", nil
		}
		return "application/ogg", nil
	}

	// FLAC magic bytes: "fLaC"
	if len(data) >= 4 && data[0] == 0x66 && data[1] == 0x4C && data[2] == 0x61 && data[3] == 0x43 {
		return "audio/flac", nil
	}

	// AAC ADTS frame: 0xFF 0xFx (where x >= 0xF0)
	if len(data) >= 2 && data[0] == 0xFF && (data[1]&0xF0) == 0xF0 {
		return "audio/aac", nil
	}

	// M4A/AAC in MP4 container: starts with ftyp
	if len(data) >= 8 && data[4] == 0x66 && data[5] == 0x74 && data[6] == 0x79 && data[7] == 0x70 {
		return "audio/mp4", nil
	}

	return "", errors.New("unrecognized audio format (magic bytes don't match any supported format)")
}

// SanitizeError converts internal errors to safe, generic messages for client responses
// SECURITY: Prevents information disclosure through detailed error messages
// Internal errors are logged server-side, safe messages returned to clients
func SanitizeError(err error) string {
	if err == nil {
		return "Unknown error"
	}

	errStr := strings.ToLower(err.Error())

	// Database errors - don't expose internal structure
	if strings.Contains(errStr, "database") ||
		strings.Contains(errStr, "sql") ||
		strings.Contains(errStr, "sqlite") ||
		strings.Contains(errStr, "query") {
		return "Database operation failed"
	}

	// File system errors - don't expose paths
	if strings.Contains(errStr, "no such file") ||
		strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "access denied") ||
		strings.Contains(errStr, "file") ||
		strings.Contains(errStr, "directory") {
		return "File access error"
	}

	// Network errors - don't expose internal topology
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dial") {
		return "Network error"
	}

	// Authentication/Authorization - keep vague
	if strings.Contains(errStr, "auth") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "token") ||
		strings.Contains(errStr, "signature") {
		return "Authentication failed"
	}

	// Validation errors - these are safe to pass through as they're user-facing
	if strings.Contains(errStr, "invalid") ||
		strings.Contains(errStr, "required") ||
		strings.Contains(errStr, "must be") ||
		strings.Contains(errStr, "expected") {
		// For validation errors, return the actual message but sanitize it
		// Remove any potential sensitive data like paths or IPs
		sanitized := strings.ReplaceAll(err.Error(), "/home/", "[path]/")
		sanitized = strings.ReplaceAll(sanitized, "/root/", "[path]/")
		sanitized = strings.ReplaceAll(sanitized, "/etc/", "[path]/")
		return sanitized
	}

	// Generic safe message for anything else
	return "Operation failed"
}
