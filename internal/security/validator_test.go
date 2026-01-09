package security

import (
	"strings"
	"testing"
)

func TestValidator_ValidateName(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "My Radio Station", false},
		{"valid Russian", "Моя Радиостанция", false},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 101), true},
		{"empty", "", true},
		{"minimum length", "abc", false},
		{"maximum length", strings.Repeat("a", 100), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateDescription(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid description", "This is a description", false},
		{"empty valid", "", false},
		{"too long", strings.Repeat("a", 5001), true},
		{"maximum length", strings.Repeat("a", 5000), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateDescription(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDescription() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidatePubkey(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid pubkey", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
		{"uppercase valid", "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF", false},
		{"too short", "0123456789abcdef", true},
		{"too long", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00", true},
		{"invalid chars", "0123456789abcdefg123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatePubkey(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePubkey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateMountpoint(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid mountpoint", "/radio/rock", false},
		{"valid simple", "/test", false},
		{"path traversal", "/radio/../admin", true},
		{"double slash", "/radio//test", true},
		{"no leading slash", "radio", true},
		{"too long", "/" + strings.Repeat("a", 256), true},
		{"invalid chars", "/radio/<script>", true},
		{"maximum length", "/" + strings.Repeat("a", 62), false},  // Max 64 chars total
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateMountpoint(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMountpoint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateRating(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   int
		wantErr bool
	}{
		{"valid 1", 1, false},
		{"valid 3", 3, false},
		{"valid 5", 5, false},
		{"too low", 0, true},
		{"too high", 6, true},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateRating(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRating() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateContentType(t *testing.T) {
	v := NewValidator()
	allowedTypes := []string{"audio/mpeg", "audio/ogg", "audio/opus"}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid mpeg", "audio/mpeg", false},
		{"valid ogg", "audio/ogg", false},
		{"valid opus", "audio/opus", false},
		{"invalid type", "audio/wav", true},
		{"invalid format", "invalid", true},
		{"empty", "", true},
		{"case insensitive", "AUDIO/MPEG", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateContentType(tt.input, allowedTypes)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContentType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateFileSize(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		size    int64
		maxSize int64
		wantErr bool
	}{
		{"valid size", 1000, 5000, false},
		{"at maximum", 5000, 5000, false},
		{"exceeds maximum", 6000, 5000, true},
		{"zero size", 0, 5000, true},
		{"negative size", -100, 5000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateFileSize(tt.size, tt.maxSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileSize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
