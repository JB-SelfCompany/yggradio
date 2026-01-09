package security

import (
	"html"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Sanitizer provides input sanitization functions
type Sanitizer struct {
	policy *bluemonday.Policy
}

// NewSanitizer creates a new sanitizer
func NewSanitizer() *Sanitizer {
	// Strict policy - removes all HTML tags
	policy := bluemonday.StrictPolicy()

	return &Sanitizer{
		policy: policy,
	}
}

// SanitizeHTML removes all HTML tags from input
func (s *Sanitizer) SanitizeHTML(input string) string {
	return s.policy.Sanitize(input)
}

// SanitizeString escapes HTML entities and trims whitespace
func (s *Sanitizer) SanitizeString(input string) string {
	// HTML escape
	escaped := html.EscapeString(input)

	// Trim whitespace
	return strings.TrimSpace(escaped)
}

// SanitizeLikePattern escapes special characters for SQL LIKE queries
func (s *Sanitizer) SanitizeLikePattern(input string) string {
	// Escape backslashes first
	input = strings.ReplaceAll(input, `\`, `\\`)

	// Escape SQL LIKE wildcards
	input = strings.ReplaceAll(input, `%`, `\%`)
	input = strings.ReplaceAll(input, `_`, `\_`)

	return input
}

// SanitizeMountpointPath sanitizes and validates mountpoint paths
func (s *Sanitizer) SanitizeMountpointPath(path string) string {
	// Trim whitespace
	path = strings.TrimSpace(path)

	// Check for path traversal attempts
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return ""
	}

	// Ensure starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Length limit
	if len(path) > 256 {
		return ""
	}

	// Remove any characters that aren't alphanumeric, dash, underscore, or slash
	// This is a whitelist approach for security
	var sanitized strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '/' {
			sanitized.WriteRune(r)
		}
	}

	result := sanitized.String()

	// Ensure it still starts with / after sanitization
	if !strings.HasPrefix(result, "/") {
		return ""
	}

	return result
}

// StripControlCharacters removes control characters from string
func (s *Sanitizer) StripControlCharacters(input string) string {
	var result strings.Builder

	for _, r := range input {
		// Keep only printable characters and common whitespace
		if r == '\n' || r == '\r' || r == '\t' || r >= ' ' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// TruncateString truncates a string to maximum length
func (s *Sanitizer) TruncateString(input string, maxLen int) string {
	if len(input) <= maxLen {
		return input
	}

	// Ensure we don't cut in the middle of a UTF-8 character
	runes := []rune(input)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}

	return input
}

// NormalizeWhitespace normalizes whitespace in text
func (s *Sanitizer) NormalizeWhitespace(input string) string {
	// Replace multiple spaces with single space
	re := strings.NewReplacer(
		"\t", " ",
		"\r\n", "\n",
		"\r", "\n",
	)

	normalized := re.Replace(input)

	// Replace multiple consecutive spaces
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}

	return strings.TrimSpace(normalized)
}

// SanitizeFilename sanitizes filename for safe storage
func (s *Sanitizer) SanitizeFilename(filename string) string {
	// Remove any path components
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")

	// Remove dangerous characters
	filename = strings.ReplaceAll(filename, "..", "_")
	filename = strings.ReplaceAll(filename, "\x00", "")

	// Limit length
	if len(filename) > 255 {
		filename = filename[:255]
	}

	return filename
}
