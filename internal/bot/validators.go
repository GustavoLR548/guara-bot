package bot

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Validation helper functions for command input validation

// isValidURL validates that a string is a valid HTTP/HTTPS URL
func isValidURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must use http or https protocol")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a valid host")
	}

	return nil
}

// isValidFeedID validates feed identifier format
func isValidFeedID(feedID string) error {
	if feedID == "" {
		return fmt.Errorf("feed ID cannot be empty")
	}

	if len(feedID) > 50 {
		return fmt.Errorf("feed ID too long (max 50 characters)")
	}

	// Allow alphanumeric, hyphens, underscores
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPattern.MatchString(feedID) {
		return fmt.Errorf("feed ID can only contain letters, numbers, hyphens, and underscores")
	}

	return nil
}

// isValidRepoID validates repository identifier format
func isValidRepoID(repoID string) error {
	if repoID == "" {
		return fmt.Errorf("repository ID cannot be empty")
	}

	if len(repoID) > 50 {
		return fmt.Errorf("repository ID too long (max 50 characters)")
	}

	// Allow alphanumeric, hyphens, underscores
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPattern.MatchString(repoID) {
		return fmt.Errorf("repository ID can only contain letters, numbers, hyphens, and underscores")
	}

	return nil
}

// isValidGitHubName validates GitHub owner/repo name format
func isValidGitHubName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if len(name) > 100 {
		return fmt.Errorf("name too long (max 100 characters)")
	}

	// GitHub allows alphanumeric, hyphens, underscores, dots
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !validPattern.MatchString(name) {
		return fmt.Errorf("invalid GitHub name format")
	}

	// Cannot start or end with special characters
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "_") {
		return fmt.Errorf("name cannot start with special characters")
	}

	return nil
}

// isValidTimeFormat validates time in HH:MM format (24-hour)
func isValidTimeFormat(timeStr string) error {
	if timeStr == "" {
		return fmt.Errorf("time cannot be empty")
	}

	// Strict HH:MM format validation
	timePattern := regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	if !timePattern.MatchString(timeStr) {
		return fmt.Errorf("invalid time format, use HH:MM (24-hour format, e.g., 09:00, 13:30)")
	}

	return nil
}

// validateScheduleTimes validates a list of schedule times
func validateScheduleTimes(times []string) error {
	if len(times) == 0 {
		return nil // Empty schedule is valid (uses fallback interval)
	}

	if len(times) > 24 {
		return fmt.Errorf("too many schedule times (max 24)")
	}

	seen := make(map[string]bool)
	for _, timeStr := range times {
		// Validate format
		if err := isValidTimeFormat(timeStr); err != nil {
			return err
		}

		// Check for duplicates
		if seen[timeStr] {
			return fmt.Errorf("duplicate time in schedule: %s", timeStr)
		}
		seen[timeStr] = true
	}

	return nil
}

// isValidLanguageCode validates language code against supported languages
func isValidLanguageCode(code string) error {
	validLanguages := map[string]bool{
		"pt-BR": true,
		"en":    true,
		"es":    true,
		"fr":    true,
		"de":    true,
		"ja":    true,
	}

	if !validLanguages[code] {
		return fmt.Errorf("unsupported language code. Supported: pt-BR, en, es, fr, de, ja")
	}

	return nil
}

// truncateMessage truncates a message to Discord's limit with ellipsis
func truncateMessage(message string, maxLength int) string {
	if len(message) <= maxLength {
		return message
	}

	if maxLength < 3 {
		return message[:maxLength]
	}

	return message[:maxLength-3] + "..."
}

// sanitizeRedisKey removes characters that could cause issues in Redis keys
func sanitizeRedisKey(key string) string {
	// Remove or replace problematic characters
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, ":", "_")
	key = strings.ReplaceAll(key, "*", "_")
	key = strings.ReplaceAll(key, "?", "_")
	return key
}

// RateLimiter tracks command cooldowns per user
type RateLimiter struct {
	cooldowns map[string]time.Time
	duration  time.Duration
}

// NewRateLimiter creates a new rate limiter with specified cooldown duration
func NewRateLimiter(cooldown time.Duration) *RateLimiter {
	return &RateLimiter{
		cooldowns: make(map[string]time.Time),
		duration:  cooldown,
	}
}

// Check returns true if the user is rate limited, false otherwise
func (rl *RateLimiter) Check(userID string) (bool, time.Duration) {
	if lastUse, exists := rl.cooldowns[userID]; exists {
		elapsed := time.Since(lastUse)
		if elapsed < rl.duration {
			remaining := rl.duration - elapsed
			return true, remaining
		}
	}
	return false, 0
}

// Record records a command use for a user
func (rl *RateLimiter) Record(userID string) {
	rl.cooldowns[userID] = time.Now()
}

// Cleanup removes expired cooldowns (should be called periodically)
func (rl *RateLimiter) Cleanup() {
	now := time.Now()
	for userID, lastUse := range rl.cooldowns {
		if now.Sub(lastUse) > rl.duration*2 {
			delete(rl.cooldowns, userID)
		}
	}
}
