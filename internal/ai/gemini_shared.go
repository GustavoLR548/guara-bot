package ai

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/ratelimit"
)

// LanguageInfo contains information about a supported language
type LanguageInfo struct {
	Code         string
	Name         string
	NativeName   string
	Instructions string
}

// GetLanguageInfo returns language configuration for a given code
func GetLanguageInfo(code string) LanguageInfo {
	languages := map[string]LanguageInfo{
		"pt-BR": {
			Code:         "pt-BR",
			Name:         "Português Brasileiro",
			NativeName:   "Português (Brasil)",
			Instructions: "Use linguagem técnica mas acessível. Evite anglicismos desnecessários.",
		},
		"en": {
			Code:         "en",
			Name:         "English",
			NativeName:   "English",
			Instructions: "Use clear technical language. Be concise and professional.",
		},
		"es": {
			Code:         "es",
			Name:         "Español",
			NativeName:   "Español",
			Instructions: "Usa lenguaje técnico pero accesible. Sé claro y profesional.",
		},
		"fr": {
			Code:         "fr",
			Name:         "Français",
			NativeName:   "Français",
			Instructions: "Utilisez un langage technique mais accessible. Soyez clair et professionnel.",
		},
		"de": {
			Code:         "de",
			Name:         "Deutsch",
			NativeName:   "Deutsch",
			Instructions: "Verwenden Sie klare technische Sprache. Seien Sie präzise und professionnel.",
		},
		"ja": {
			Code:         "ja",
			Name:         "日本語 (Japanese)",
			NativeName:   "日本語",
			Instructions: "技術的でありながら分かりやすい言語を使用してください。明確でプロフェッショナルに。",
		},
	}

	if info, ok := languages[code]; ok {
		return info
	}

	// Default to English if unknown language
	log.Printf("Unknown language code: %s, defaulting to English", code)
	return languages["en"]
}

// GetSupportedLanguages returns a list of all supported language codes
func GetSupportedLanguages() []string {
	return []string{"pt-BR", "en", "es", "fr", "de", "ja"}
}

// GetLanguageName returns the display name for a language code
func GetLanguageName(code string) string {
	return GetLanguageInfo(code).NativeName
}

// ParseJSONResponse parses the JSON response from Gemini and validates it
func ParseJSONResponse(rawResponse string, originalTitle string, languageCode string) (*SummaryResponse, error) {
	// Clean up response - remove markdown code blocks if present
	cleanedResponse := strings.TrimSpace(rawResponse)
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```")
	cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")
	cleanedResponse = strings.TrimSpace(cleanedResponse)
	
	log.Printf("Parsing JSON response for %s (length: %d)", languageCode, len(cleanedResponse))
	
	var response SummaryResponse
	if err := json.Unmarshal([]byte(cleanedResponse), &response); err != nil {
		log.Printf("ERROR: Failed to parse JSON for %s: %v", languageCode, err)
		log.Printf("Raw response (first 200 chars): %s", TruncateString(cleanedResponse, 200))
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}
	
	// Validate response
	if response.TranslatedTitle == "" {
		log.Printf("WARNING: Empty translated title for %s, using original", languageCode)
		response.TranslatedTitle = originalTitle
	}
	
	if response.Summary == "" {
		log.Printf("ERROR: Empty summary for %s", languageCode)
		return nil, fmt.Errorf("empty summary in response")
	}
	
	// Truncate title if too long (max 256 chars for Discord embed)
	if len(response.TranslatedTitle) > 256 {
		log.Printf("WARNING: Title too long for %s (%d chars), truncating", languageCode, len(response.TranslatedTitle))
		response.TranslatedTitle = response.TranslatedTitle[:253] + "..."
	}
	
	return &response, nil
}

// TruncateString truncates a string to maxLength characters
func TruncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}

// ExtractSummaryFromBrokenJSON attempts to extract summary text from malformed JSON
// This is a fallback when proper JSON parsing fails
func ExtractSummaryFromBrokenJSON(rawResponse string) string {
	// Try to extract the summary field value even if JSON is incomplete/malformed
	// Look for "summary": "..." pattern
	summaryStart := strings.Index(rawResponse, `"summary"`)
	if summaryStart == -1 {
		// No summary field found, return cleaned response
		return strings.TrimSpace(rawResponse)
	}
	
	// Find the opening quote after "summary":
	valueStart := strings.Index(rawResponse[summaryStart:], `":`)
	if valueStart == -1 {
		return strings.TrimSpace(rawResponse)
	}
	valueStart += summaryStart + 2 // Move past ":"
	
	// Skip whitespace and opening quote
	for valueStart < len(rawResponse) && (rawResponse[valueStart] == ' ' || rawResponse[valueStart] == '"') {
		valueStart++
	}
	
	// Find the closing quote (handle escaped quotes)
	valueEnd := valueStart
	for valueEnd < len(rawResponse) {
		if rawResponse[valueEnd] == '"' && (valueEnd == 0 || rawResponse[valueEnd-1] != '\\') {
			break
		}
		valueEnd++
	}
	
	if valueEnd > valueStart && valueEnd < len(rawResponse) {
		summary := rawResponse[valueStart:valueEnd]
		// Unescape common JSON escapes
		summary = strings.ReplaceAll(summary, `\"`, `"`)
		summary = strings.ReplaceAll(summary, `\\`, `\`)
		summary = strings.ReplaceAll(summary, `\n`, "\n")
		return strings.TrimSpace(summary)
	}
	
	// Fallback: return cleaned response
	return strings.TrimSpace(rawResponse)
}

// ShouldRetry determines if an error is retryable
func ShouldRetry(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	
	// Retryable errors (temporary issues)
	retryableErrors := []string{
		"429",                              // Rate limit
		"503",                              // Service unavailable
		"timeout",                          // Timeout
		"deadline exceeded",                // Context deadline
		"temporary",                        // Temporary network issues
		"connection reset",                 // Connection issues
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	// Non-retryable errors (permanent failures)
	nonRetryableErrors := []string{
		"400",          // Bad request
		"401",          // Unauthorized
		"403",          // Forbidden
		"404",          // Not found
		"invalid",      // Invalid input
	}

	for _, nonRetryable := range nonRetryableErrors {
		if strings.Contains(errStr, nonRetryable) {
			return false
		}
	}

	// Default: retry on unknown errors
	return true
}

// CalculateBackoff calculates exponential backoff duration for retry attempts
func CalculateBackoff(rateLimiter *ratelimit.Manager, attempt int) time.Duration {
	return rateLimiter.CalculateBackoff(attempt)
}
