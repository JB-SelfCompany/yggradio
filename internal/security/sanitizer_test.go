package security

import (
	"strings"
	"testing"
)

func TestSanitizer_SanitizeHTML(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "removes script tags",
			input:  `<script>alert('XSS')</script>Hello`,
			expect: "Hello",
		},
		{
			name:   "removes all HTML",
			input:  `<p>Hello <strong>World</strong></p>`,
			expect: "Hello World",
		},
		{
			name:   "removes onclick handlers",
			input:  `<div onclick="evil()">Click</div>`,
			expect: "Click",
		},
		{
			name:   "handles iframe attacks",
			input:  `<iframe src="http://evil.com"></iframe>`,
			expect: "",
		},
		{
			name:   "plain text unchanged",
			input:  "Just plain text",
			expect: "Just plain text",
		},
		{
			name:   "handles SVG XSS",
			input:  `<svg><script>alert(1)</script></svg>`,
			expect: "",
		},
		{
			name:   "removes javascript: protocol",
			input:  `<a href="javascript:alert(1)">link</a>`,
			expect: "link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeHTML(tt.input)
			if result != tt.expect {
				t.Errorf("SanitizeHTML() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSanitizer_SanitizeString(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "escapes HTML entities",
			input:  `<script>alert("XSS")</script>`,
			expect: "&lt;script&gt;alert(&#34;XSS&#34;)&lt;/script&gt;",
		},
		{
			name:   "trims whitespace",
			input:  "  hello world  ",
			expect: "hello world",
		},
		{
			name:   "escapes ampersand",
			input:  "Tom & Jerry",
			expect: "Tom &amp; Jerry",
		},
		{
			name:   "handles quotes",
			input:  `He said "hello"`,
			expect: "He said &#34;hello&#34;",
		},
		{
			name:   "plain text unchanged",
			input:  "Plain text",
			expect: "Plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeString(tt.input)
			if result != tt.expect {
				t.Errorf("SanitizeString() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSanitizer_SanitizeLikePattern(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "escapes percent",
			input:  "test%value",
			expect: `test\%value`,
		},
		{
			name:   "escapes underscore",
			input:  "test_value",
			expect: `test\_value`,
		},
		{
			name:   "escapes backslash",
			input:  `test\value`,
			expect: `test\\value`,
		},
		{
			name:   "escapes all special chars",
			input:  `test\%_value`,
			expect: `test\\\%\_value`,
		},
		{
			name:   "plain text unchanged",
			input:  "testvalue",
			expect: "testvalue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeLikePattern(tt.input)
			if result != tt.expect {
				t.Errorf("SanitizeLikePattern() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSanitizer_SanitizeMountpointPath(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "valid path",
			input:  "/radio/stream",
			expect: "/radio/stream",
		},
		{
			name:   "adds leading slash",
			input:  "radio/stream",
			expect: "/radio/stream",
		},
		{
			name:   "path traversal blocked",
			input:  "/radio/../admin",
			expect: "",
		},
		{
			name:   "double slash blocked",
			input:  "/radio//stream",
			expect: "",
		},
		{
			name:   "removes invalid characters",
			input:  "/radio/<script>",
			expect: "/radio/script",  // Slash is preserved as it's a valid character
		},
		{
			name:   "too long rejected",
			input:  "/" + strings.Repeat("a", 300),
			expect: "",
		},
		{
			name:   "trims whitespace",
			input:  "  /radio/stream  ",
			expect: "/radio/stream",
		},
		{
			name:   "allows alphanumeric and dash underscore",
			input:  "/radio_station-1",
			expect: "/radio_station-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeMountpointPath(tt.input)
			if result != tt.expect {
				t.Errorf("SanitizeMountpointPath() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSanitizer_StripControlCharacters(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "removes null bytes",
			input:  "hello\x00world",
			expect: "helloworld",
		},
		{
			name:   "removes control characters",
			input:  "hello\x01\x02\x03world",
			expect: "helloworld",
		},
		{
			name:   "preserves newlines",
			input:  "hello\nworld",
			expect: "hello\nworld",
		},
		{
			name:   "preserves tabs",
			input:  "hello\tworld",
			expect: "hello\tworld",
		},
		{
			name:   "plain text unchanged",
			input:  "hello world",
			expect: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.StripControlCharacters(tt.input)
			if result != tt.expect {
				t.Errorf("StripControlCharacters() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSanitizer_TruncateString(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		maxLen int
		expect string
	}{
		{
			name:   "no truncation needed",
			input:  "short",
			maxLen: 10,
			expect: "short",
		},
		{
			name:   "truncates long string",
			input:  "this is a long string",
			maxLen: 10,
			expect: "this is a ",
		},
		{
			name:   "handles UTF-8 correctly",
			input:  "hello 世界",
			maxLen: 7,
			expect: "hello 世",
		},
		{
			name:   "at exact length",
			input:  "exact",
			maxLen: 5,
			expect: "exact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.TruncateString(tt.input, tt.maxLen)
			if result != tt.expect {
				t.Errorf("TruncateString() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSanitizer_NormalizeWhitespace(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "replaces tabs",
			input:  "hello\tworld",
			expect: "hello world",
		},
		{
			name:   "normalizes line endings",
			input:  "hello\r\nworld",
			expect: "hello\nworld",
		},
		{
			name:   "removes multiple spaces",
			input:  "hello    world",
			expect: "hello world",
		},
		{
			name:   "trims whitespace",
			input:  "  hello world  ",
			expect: "hello world",
		},
		{
			name:   "handles all together",
			input:  "  hello\t\t\r\nworld    test  ",
			expect: "hello \nworld test",  // Space before \n is preserved
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.NormalizeWhitespace(tt.input)
			if result != tt.expect {
				t.Errorf("NormalizeWhitespace() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSanitizer_SanitizeFilename(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "removes path separators",
			input:  "../../etc/passwd",
			expect: "____etc_passwd", 
		},
		{
			name:   "removes null bytes",
			input:  "file\x00name.txt",
			expect: "filename.txt",
		},
		{
			name:   "truncates long filenames",
			input:  strings.Repeat("a", 300),
			expect: strings.Repeat("a", 255),
		},
		{
			name:   "removes backslashes",
			input:  `path\to\file.txt`,
			expect: "path_to_file.txt",
		},
		{
			name:   "safe filename unchanged",
			input:  "myfile.txt",
			expect: "myfile.txt",
		},
		{
			name:   "handles path traversal",
			input:  "../../../etc/shadow",
			expect: "______etc_shadow", 
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeFilename(tt.input)
			if result != tt.expect {
				t.Errorf("SanitizeFilename() = %q, want %q", result, tt.expect)
			}
		})
	}
}

// Security-focused tests
func TestSanitizer_XSSPrevention(t *testing.T) {
	s := NewSanitizer()

	xssVectors := []string{
		`<script>alert('XSS')</script>`,
		`<img src=x onerror=alert('XSS')>`,
		`<svg/onload=alert('XSS')>`,
		`<iframe src="javascript:alert('XSS')">`,
		`<body onload=alert('XSS')>`,
		`<input onfocus=alert('XSS') autofocus>`,
		`<select onfocus=alert('XSS') autofocus>`,
		`<textarea onfocus=alert('XSS') autofocus>`,
		`<object data="javascript:alert('XSS')">`,
		`<embed src="data:text/html,<script>alert('XSS')</script>">`,
	}

	for _, vector := range xssVectors {
		t.Run("XSS_vector", func(t *testing.T) {
			result := s.SanitizeHTML(vector)
			// After sanitization, should not contain dangerous patterns
			if strings.Contains(strings.ToLower(result), "<script") ||
				strings.Contains(strings.ToLower(result), "javascript:") ||
				strings.Contains(strings.ToLower(result), "onerror") ||
				strings.Contains(strings.ToLower(result), "onload") {
				t.Errorf("XSS vector not properly sanitized: %q -> %q", vector, result)
			}
		})
	}
}

func TestSanitizer_PathTraversalPrevention(t *testing.T) {
	s := NewSanitizer()

	pathTraversalVectors := []string{
		"../../etc/passwd",
		"..\\..\\windows\\system32",
		"/radio/../admin",
		"/radio/./../../admin",
		"/radio//admin",
	}

	for _, vector := range pathTraversalVectors {
		t.Run("path_traversal", func(t *testing.T) {
			result := s.SanitizeMountpointPath(vector)
			// Should either be empty or not contain traversal patterns
			if result != "" && (strings.Contains(result, "..") || strings.Contains(result, "//")) {
				t.Errorf("Path traversal not prevented: %q -> %q", vector, result)
			}
		})
	}
}

func TestSanitizer_SQLInjectionPrevention(t *testing.T) {
	s := NewSanitizer()

	// Test LIKE pattern escaping
	sqlPatterns := []string{
		"%' OR '1'='1",
		"_' UNION SELECT",
		"\\%test",
	}

	for _, pattern := range sqlPatterns {
		t.Run("sql_injection", func(t *testing.T) {
			result := s.SanitizeLikePattern(pattern)
			// Should escape all wildcards
			if !strings.Contains(result, `\%`) && strings.Contains(pattern, "%") {
				t.Errorf("SQL wildcard not escaped: %q -> %q", pattern, result)
			}
		})
	}
}
