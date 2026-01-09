package moderation

import (
	"strings"
	"testing"
)

func TestContentFilter_BlacklistedKeywords(t *testing.T) {
	filter := NewContentFilter()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Clean content",
			content:  "This is a normal comment",
			expected: false,
		},
		{
			name:     "Contains spam keyword",
			content:  "Click here now to win!",
			expected: true,
		},
		{
			name:     "Contains buy now",
			content:  "Buy now and save money!",
			expected: true,
		},
		{
			name:     "Case insensitive",
			content:  "CLICK HERE NOW",
			expected: true,
		},
		{
			name:     "Partial match",
			content:  "Check out this click here now offer",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSpam, _ := filter.IsSpam(tt.content)
			if isSpam != tt.expected {
				t.Errorf("Expected spam=%v, got spam=%v for content: %s", tt.expected, isSpam, tt.content)
			}
		})
	}
}

func TestContentFilter_RepeatedCharacters(t *testing.T) {
	filter := NewContentFilter()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Normal text",
			content:  "Hello world",
			expected: false,
		},
		{
			name:     "Some repetition (acceptable)",
			content:  "Looooove this!!!",
			expected: false,
		},
		{
			name:     "Excessive repetition",
			content:  "Hellooooooooooooooooo",
			expected: true,
		},
		{
			name:     "Spam pattern",
			content:  "aaaaaaaaaaaaaaaaaaaa",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSpam, _ := filter.IsSpam(tt.content)
			if isSpam != tt.expected {
				t.Errorf("Expected spam=%v, got spam=%v for content: %s", tt.expected, isSpam, tt.content)
			}
		})
	}
}

func TestContentFilter_ExcessiveURLs(t *testing.T) {
	filter := NewContentFilter()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "No URLs",
			content:  "This is a comment without links",
			expected: false,
		},
		{
			name:     "One URL (acceptable)",
			content:  "Check this out: https://example.com",
			expected: false,
		},
		{
			name:     "Two URLs (acceptable)",
			content:  "Visit https://example.com and https://example.org",
			expected: false,
		},
		{
			name:     "Excessive URLs (spam)",
			content:  "https://spam1.com https://spam2.com https://spam3.com https://spam4.com https://spam5.com https://spam6.com",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSpam, _ := filter.IsSpam(tt.content)
			if isSpam != tt.expected {
				t.Errorf("Expected spam=%v, got spam=%v for content: %s", tt.expected, isSpam, tt.content)
			}
		})
	}
}

func TestContentFilter_ExcessiveUppercase(t *testing.T) {
	filter := NewContentFilter()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Normal case",
			content:  "This Is A Normal Comment",
			expected: false,
		},
		{
			name:     "Some uppercase (acceptable)",
			content:  "WOW this is AMAZING!",
			expected: false,
		},
		{
			name:     "Excessive uppercase (shouting/spam)",
			content:  "BUY NOW THIS IS THE BEST OFFER EVER!!!",
			expected: true,
		},
		{
			name:     "All caps (spam)",
			content:  "CLICK HERE TO CLAIM YOUR PRIZE",
			expected: true,
		},
		{
			name:     "Short all caps (acceptable)",
			content:  "LOL",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSpam, _ := filter.IsSpam(tt.content)
			if isSpam != tt.expected {
				t.Errorf("Expected spam=%v, got spam=%v for content: %s", tt.expected, isSpam, tt.content)
			}
		})
	}
}

func TestContentFilter_UnicodeTricks(t *testing.T) {
	filter := NewContentFilter()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Normal text",
			content:  "Hello world",
			expected: false,
		},
		{
			name:     "Zero-width space",
			content:  "Hello\u200Bworld",
			expected: true,
		},
		{
			name:     "Zero-width joiner",
			content:  "Hello\u200Dworld",
			expected: true,
		},
		{
			name:     "Zero-width non-joiner",
			content:  "Hello\u200Cworld",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSpam, _ := filter.IsSpam(tt.content)
			if isSpam != tt.expected {
				t.Errorf("Expected spam=%v, got spam=%v for content: %s", tt.expected, isSpam, tt.content)
			}
		})
	}
}

func TestContentFilter_AddRemoveKeyword(t *testing.T) {
	filter := NewContentFilter()

	// Add custom keyword
	filter.AddBlacklistedKeyword("badword")

	// Test that it's detected
	isSpam, reason := filter.IsSpam("This contains badword")
	if !isSpam {
		t.Error("Expected custom keyword to be detected")
	}
	if !strings.Contains(reason, "badword") {
		t.Errorf("Expected reason to mention 'badword', got: %s", reason)
	}

	// Remove keyword
	filter.RemoveBlacklistedKeyword("badword")

	// Test that it's no longer detected
	isSpam, _ = filter.IsSpam("This contains badword")
	if isSpam {
		t.Error("Expected removed keyword to not be detected")
	}
}

func TestContentFilter_GetKeywords(t *testing.T) {
	filter := NewContentFilter()

	// Get default keywords
	keywords := filter.GetBlacklistedKeywords()
	if len(keywords) == 0 {
		t.Error("Expected default keywords to be present")
	}

	// Add custom keyword
	filter.AddBlacklistedKeyword("custom")
	keywords = filter.GetBlacklistedKeywords()

	found := false
	for _, kw := range keywords {
		if kw == "custom" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected custom keyword to be in list")
	}
}

func TestContentFilter_CheckWithStats(t *testing.T) {
	filter := NewContentFilter()

	// Test clean content
	isSpam, reason, stats := filter.CheckWithStats("Normal comment")
	if isSpam {
		t.Error("Expected clean content to not be spam")
	}
	if stats.TotalChecked != 1 {
		t.Errorf("Expected TotalChecked=1, got %d", stats.TotalChecked)
	}
	if stats.SpamDetected != 0 {
		t.Errorf("Expected SpamDetected=0, got %d", stats.SpamDetected)
	}

	// Test blacklisted keyword
	isSpam, reason, stats = filter.CheckWithStats("Click here now")
	if !isSpam {
		t.Error("Expected spam to be detected")
	}
	if stats.SpamDetected != 1 {
		t.Errorf("Expected SpamDetected=1, got %d", stats.SpamDetected)
	}
	if stats.BlacklistedKeywords != 1 {
		t.Errorf("Expected BlacklistedKeywords=1, got %d", stats.BlacklistedKeywords)
	}
	if !strings.Contains(reason, "keyword") {
		t.Errorf("Expected reason to mention keyword, got: %s", reason)
	}

	// Test repeated characters
	isSpam, reason, stats = filter.CheckWithStats("aaaaaaaaaaaaaaaaaaa")
	if !isSpam {
		t.Error("Expected spam to be detected")
	}
	if stats.RepeatedChars != 1 {
		t.Errorf("Expected RepeatedChars=1, got %d", stats.RepeatedChars)
	}
	if !strings.Contains(reason, "repeated") {
		t.Errorf("Expected reason to mention repeated chars, got: %s", reason)
	}

	// Test excessive URLs
	manyURLs := "https://a.com https://b.com https://c.com https://d.com https://e.com https://f.com"
	isSpam, reason, stats = filter.CheckWithStats(manyURLs)
	if !isSpam {
		t.Error("Expected spam to be detected")
	}
	if stats.ExcessiveURLs != 1 {
		t.Errorf("Expected ExcessiveURLs=1, got %d", stats.ExcessiveURLs)
	}
	if !strings.Contains(reason, "URL") {
		t.Errorf("Expected reason to mention URLs, got: %s", reason)
	}
}

func BenchmarkContentFilter_IsSpam(b *testing.B) {
	filter := NewContentFilter()
	content := "This is a normal comment without any spam indicators"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.IsSpam(content)
	}
}

func BenchmarkContentFilter_IsSpam_WithURLs(b *testing.B) {
	filter := NewContentFilter()
	content := "Check out https://example.com and https://example.org for more info"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.IsSpam(content)
	}
}
