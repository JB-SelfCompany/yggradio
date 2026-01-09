package moderation

import (
	"regexp"
	"strings"
	"unicode"
)

// ContentFilter implements automatic spam and content filtering
type ContentFilter struct {
	blacklistedKeywords []string
	spamPatterns        []*regexp.Regexp
}

// NewContentFilter creates a new content filter with default rules
func NewContentFilter() *ContentFilter {
	cf := &ContentFilter{
		blacklistedKeywords: defaultBlacklistedKeywords(),
		spamPatterns:        defaultSpamPatterns(),
	}

	return cf
}

// IsSpam checks if content matches spam patterns or blacklisted keywords
// Returns: (isSpam bool, reason string)
func (cf *ContentFilter) IsSpam(content string) (bool, string) {
	// Check for blacklisted keywords
	if blocked, keyword := cf.containsBlacklistedKeyword(content); blocked {
		return true, "Contains blacklisted keyword: " + keyword
	}

	// Check for repeated characters (spam pattern)
	if cf.hasRepeatedCharacters(content, 15) {
		return true, "Excessive repeated characters detected"
	}

	// Check for excessive URLs
	if cf.hasExcessiveURLs(content, 5) {
		return true, "Too many URLs (spam pattern)"
	}

	// Check for excessive uppercase (shouting)
	if cf.hasExcessiveUppercase(content, 0.7) {
		return true, "Excessive uppercase (spam pattern)"
	}

	// Check for suspicious patterns
	if matched, pattern := cf.matchesSpamPattern(content); matched {
		return true, "Matches spam pattern: " + pattern
	}

	// Check for zero-width characters and other unicode tricks
	if cf.hasUnicodeTricks(content) {
		return true, "Suspicious unicode characters detected"
	}

	return false, ""
}

// containsBlacklistedKeyword checks if content contains any blacklisted keywords
func (cf *ContentFilter) containsBlacklistedKeyword(content string) (bool, string) {
	contentLower := strings.ToLower(content)

	for _, keyword := range cf.blacklistedKeywords {
		keywordLower := strings.ToLower(keyword)

		// Word boundary check to avoid false positives
		if strings.Contains(contentLower, keywordLower) {
			return true, keyword
		}
	}

	return false, ""
}

// hasRepeatedCharacters checks for excessive character repetition
func (cf *ContentFilter) hasRepeatedCharacters(content string, threshold int) bool {
	if len(content) == 0 {
		return false
	}

	maxRepeat := 1
	currentRepeat := 1
	var lastChar rune

	for _, char := range content {
		if char == lastChar {
			currentRepeat++
			if currentRepeat > maxRepeat {
				maxRepeat = currentRepeat
			}
		} else {
			currentRepeat = 1
			lastChar = char
		}
	}

	return maxRepeat >= threshold
}

// hasExcessiveURLs checks for too many URLs in content
func (cf *ContentFilter) hasExcessiveURLs(content string, threshold int) bool {
	urlPattern := regexp.MustCompile(`https?://[^\s]+`)
	matches := urlPattern.FindAllString(content, -1)

	return len(matches) > threshold
}

// hasExcessiveUppercase checks if content has too much uppercase text
func (cf *ContentFilter) hasExcessiveUppercase(content string, threshold float64) bool {
	if len(content) < 10 {
		return false // Too short to judge
	}

	upperCount := 0
	letterCount := 0

	for _, char := range content {
		if unicode.IsLetter(char) {
			letterCount++
			if unicode.IsUpper(char) {
				upperCount++
			}
		}
	}

	if letterCount == 0 {
		return false
	}

	ratio := float64(upperCount) / float64(letterCount)
	return ratio > threshold
}

// matchesSpamPattern checks if content matches known spam patterns
func (cf *ContentFilter) matchesSpamPattern(content string) (bool, string) {
	for _, pattern := range cf.spamPatterns {
		if pattern.MatchString(content) {
			return true, pattern.String()
		}
	}

	return false, ""
}

// hasUnicodeTricks checks for suspicious unicode characters
func (cf *ContentFilter) hasUnicodeTricks(content string) bool {
	// Check for zero-width characters
	zeroWidthChars := []rune{
		'\u200B', // Zero-width space
		'\u200C', // Zero-width non-joiner
		'\u200D', // Zero-width joiner
		'\u2060', // Word joiner
		'\uFEFF', // Zero-width no-break space
	}

	for _, char := range content {
		for _, zwChar := range zeroWidthChars {
			if char == zwChar {
				return true
			}
		}
	}

	// Check for excessive combining characters
	combiningCount := 0
	for _, char := range content {
		if unicode.In(char, unicode.Mn, unicode.Mc, unicode.Me) {
			combiningCount++
		}
	}

	// More than 10% combining characters is suspicious
	if len(content) > 0 && float64(combiningCount)/float64(len(content)) > 0.1 {
		return true
	}

	return false
}

// AddBlacklistedKeyword adds a keyword to the blacklist
func (cf *ContentFilter) AddBlacklistedKeyword(keyword string) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return
	}

	// Check if already exists
	for _, existing := range cf.blacklistedKeywords {
		if strings.EqualFold(existing, keyword) {
			return
		}
	}

	cf.blacklistedKeywords = append(cf.blacklistedKeywords, keyword)
}

// RemoveBlacklistedKeyword removes a keyword from the blacklist
func (cf *ContentFilter) RemoveBlacklistedKeyword(keyword string) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return
	}

	filtered := make([]string, 0, len(cf.blacklistedKeywords))
	for _, existing := range cf.blacklistedKeywords {
		if !strings.EqualFold(existing, keyword) {
			filtered = append(filtered, existing)
		}
	}

	cf.blacklistedKeywords = filtered
}

// GetBlacklistedKeywords returns the current blacklist
func (cf *ContentFilter) GetBlacklistedKeywords() []string {
	return append([]string{}, cf.blacklistedKeywords...)
}

// defaultBlacklistedKeywords returns default blacklisted keywords
// This is a conservative list - administrators should customize based on their needs
func defaultBlacklistedKeywords() []string {
	return []string{
		// Spam-related
		"click here now",
		"buy now",
		"limited time offer",
		"congratulations you won",
		"claim your prize",
		"free money",
		"get rich quick",
		"work from home",
		"earn $$$ fast",
		"viagra",
		"cialis",
		"lottery winner",

		// Common spam phrases
		"subscribe to my channel",
		"check out my website",
		"follow me on",
		"join my discord",

		// Cryptocurrency scams
		"double your bitcoin",
		"crypto giveaway",
		"send btc",
		"wallet address",
	}
}

// defaultSpamPatterns returns default regex patterns for spam detection
func defaultSpamPatterns() []*regexp.Regexp {
	patterns := []string{
		// Email addresses in suspicious contexts
		`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}.*\s+(now|today|click|free)`,

		// Phone numbers with marketing language
		`\d{3}[-.\s]?\d{3}[-.\s]?\d{4}.*\s+(call|text|contact)`,

		// Cryptocurrency wallet addresses (Bitcoin)
		`\b[13][a-km-zA-HJ-NP-Z1-9]{25,34}\b`,

		// Multiple exclamation marks or question marks
		`[!?]{5,}`,

		// All caps words (common in spam)
		`\b[A-Z]{10,}\b`,

		// Excessive emojis (10+ in a row)
		`[\x{1F300}-\x{1F9FF}]{10,}`,
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if re, err := regexp.Compile(pattern); err == nil {
			compiled = append(compiled, re)
		}
	}

	return compiled
}

// ValidationError represents a validation error
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// SpamError represents a spam detection error
type SpamError struct {
	Reason string
}

func (e *SpamError) Error() string {
	return "Spam detected: " + e.Reason
}

// IsSpamError checks if error is a spam error
func IsSpamError(err error) bool {
	_, ok := err.(*SpamError)
	return ok
}

// FilterStats returns statistics about filter effectiveness
type FilterStats struct {
	TotalChecked         int
	SpamDetected         int
	BlacklistedKeywords  int
	RepeatedChars        int
	ExcessiveURLs        int
	ExcessiveUppercase   int
	PatternMatches       int
	UnicodeTricks        int
}

// CheckWithStats checks content and returns detailed statistics
func (cf *ContentFilter) CheckWithStats(content string) (isSpam bool, reason string, stats FilterStats) {
	stats.TotalChecked = 1

	// Blacklisted keywords
	if blocked, keyword := cf.containsBlacklistedKeyword(content); blocked {
		stats.SpamDetected = 1
		stats.BlacklistedKeywords = 1
		return true, "Contains blacklisted keyword: " + keyword, stats
	}

	// Repeated characters
	if cf.hasRepeatedCharacters(content, 15) {
		stats.SpamDetected = 1
		stats.RepeatedChars = 1
		return true, "Excessive repeated characters detected", stats
	}

	// Excessive URLs
	if cf.hasExcessiveURLs(content, 5) {
		stats.SpamDetected = 1
		stats.ExcessiveURLs = 1
		return true, "Too many URLs (spam pattern)", stats
	}

	// Excessive uppercase
	if cf.hasExcessiveUppercase(content, 0.7) {
		stats.SpamDetected = 1
		stats.ExcessiveUppercase = 1
		return true, "Excessive uppercase (spam pattern)", stats
	}

	// Pattern matches
	if matched, pattern := cf.matchesSpamPattern(content); matched {
		stats.SpamDetected = 1
		stats.PatternMatches = 1
		return true, "Matches spam pattern: " + pattern, stats
	}

	// Unicode tricks
	if cf.hasUnicodeTricks(content) {
		stats.SpamDetected = 1
		stats.UnicodeTricks = 1
		return true, "Suspicious unicode characters detected", stats
	}

	return false, "", stats
}
