package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/ratelimit"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// SummaryResponse contains both translated title and summary
type SummaryResponse struct {
	TranslatedTitle string `json:"translated_title"`
	Summary         string `json:"summary"`
}

// AISummarizer defines the interface for AI-based text summarization
type AISummarizer interface {
	// Summarize generates a TL;DR summary in Brazilian Portuguese
	Summarize(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error)
	// SummarizeInLanguage generates a TL;DR summary with translated title in the specified language
	SummarizeInLanguage(ctx context.Context, text string, originalTitle string, languageCode string) (*SummaryResponse, error)
}

// GeminiSummarizer implements AISummarizer using Google's Gemini API
type GeminiSummarizer struct {
	apiKey      string
	model       string
	prompt      string
	rateLimiter *ratelimit.Manager
}

// NewGeminiSummarizer creates a new Gemini-based summarizer
func NewGeminiSummarizer(apiKey string) *GeminiSummarizer {
	return &GeminiSummarizer{
		apiKey:      apiKey,
		model:       "gemini-2.5-flash",  // Using latest flash model (free tier)
		prompt:      "Crie um resumo informativo em Português Brasileiro (PT-BR) para desenvolvedores de jogos sobre o seguinte artigo do Godot Engine. O resumo deve ter 3-5 frases, destacando as principais novidades, melhorias ou mudanças importantes. Seja claro, técnico e objetivo. IMPORTANTE: NÃO inclua nenhum preâmbulo, introdução ou frase como 'Aqui está um resumo'. Comece DIRETAMENTE com o conteúdo do resumo:",
		rateLimiter: ratelimit.NewManager(ratelimit.DefaultConfig()),
	}
}

// NewGeminiSummarizerWithRateLimit creates a new Gemini-based summarizer with custom rate limiting
func NewGeminiSummarizerWithRateLimit(apiKey string, config ratelimit.Config) *GeminiSummarizer {
	return &GeminiSummarizer{
		apiKey:      apiKey,
		model:       "gemini-2.5-flash",
		prompt:      "Crie um resumo informativo em Português Brasileiro (PT-BR) para desenvolvedores de jogos sobre o seguinte artigo do Godot Engine. O resumo deve ter 3-5 frases, destacando as principais novidades, melhorias ou mudanças importantes. Seja claro, técnico e objetivo. IMPORTANTE: NÃO inclua nenhum preâmbulo, introdução ou frase como 'Aqui está um resumo'. Comece DIRETAMENTE com o conteúdo do resumo:",
		rateLimiter: ratelimit.NewManager(config),
	}
}

// ListAvailableModels returns available Gemini models (for debugging)
func (s *GeminiSummarizer) ListAvailableModels(ctx context.Context) ([]string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	var models []string
	iter := client.ListModels(ctx)
	for {
		model, err := iter.Next()
		if err != nil {
			break
		}
		models = append(models, model.Name)
	}
	return models, nil
}

// Summarize generates a TL;DR summary in English (default language) with rate limiting
func (s *GeminiSummarizer) Summarize(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
	// Default to English for backward compatibility
	return s.SummarizeInLanguage(ctx, text, originalTitle, "en")
}

// SummarizeDeprecated is the old implementation (kept for reference)
func (s *GeminiSummarizer) SummarizeDeprecated(ctx context.Context, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty text provided")
	}

	log.Printf("Starting summary generation (input length: %d chars)", len(text))

	// Create client for token counting
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(s.model)

	// Build full prompt
	fullPrompt := fmt.Sprintf("%s\n\n%s", s.prompt, text)

	// Count tokens before making request
	tokenResp, err := model.CountTokens(ctx, genai.Text(fullPrompt))
	if err != nil {
		log.Printf("WARNING: Failed to count tokens: %v (proceeding anyway)", err)
		// Estimate based on character count (rough: 1 token ≈ 4 chars)
		tokenResp = &genai.CountTokensResponse{
			TotalTokens: int32(len(fullPrompt) / 4),
		}
	}

	inputTokens := int(tokenResp.TotalTokens)
	estimatedOutputTokens := 1500 // Max output tokens configured
	estimatedTotal := inputTokens + estimatedOutputTokens

	log.Printf("Token estimate: input=%d, estimated_output=%d, total=%d", 
		inputTokens, estimatedOutputTokens, estimatedTotal)

	// Check rate limits before making request
	can, err := s.rateLimiter.CanMakeRequest(estimatedTotal)
	if !can {
		log.Printf("Rate limit check failed: %v", err)
		// Try waiting for capacity
		waitErr := s.rateLimiter.WaitForCapacity(ctx, estimatedTotal)
		if waitErr != nil {
			return "", fmt.Errorf("rate limit exceeded and wait failed: %w", err)
		}
		log.Printf("Rate limit capacity available after waiting")
	}

	// Retry logic with exponential backoff
	var summary string
	var lastErr error
	maxRetries := s.rateLimiter.GetConfig().RetryAttempts

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := s.rateLimiter.CalculateBackoff(attempt - 1)
			log.Printf("Retry attempt %d/%d after %v backoff", attempt, maxRetries, backoff)
			
			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		summary, lastErr = s.attemptSummarize(ctx, model, fullPrompt)
		
		if lastErr == nil {
			// Success - record request with actual token usage
			actualTokens := inputTokens + (len(summary) / 4) // Rough estimate of output tokens
			s.rateLimiter.RecordRequest(actualTokens)
			log.Printf("Request successful, recorded %d tokens", actualTokens)
			return summary, nil
		}

		// Record failure for circuit breaker
		s.rateLimiter.RecordFailure()
		log.Printf("Attempt %d failed: %v", attempt, lastErr)

		// Check if we should retry
		if !s.shouldRetry(lastErr) {
			log.Printf("Error is not retryable, aborting")
			return "", lastErr
		}
	}

	return "", fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
}

// attemptSummarize performs a single summarization attempt
func (s *GeminiSummarizer) attemptSummarize(ctx context.Context, model *genai.GenerativeModel, prompt string) (string, error) {
	// Configure model for informative summaries
	model.SetTemperature(0.7)
	model.SetMaxOutputTokens(1500)
	model.SetTopP(0.95)
	model.SetTopK(40)

	log.Printf("Sending request to Gemini API (model: %s)", s.model)

	// Generate content with timing
	startTime := time.Now()
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	duration := time.Since(startTime)
	
	if err != nil {
		log.Printf("Gemini API error after %v: %v", duration, err)
		return "", fmt.Errorf("API request failed: %w", err)
	}
	
	log.Printf("Gemini API responded in %v", duration)

	// Extract text from response
	if len(resp.Candidates) == 0 {
		log.Printf("WARNING: No candidates in response")
		return "", fmt.Errorf("no summary generated")
	}

	candidate := resp.Candidates[0]
	log.Printf("Response finish reason: %v", candidate.FinishReason)
	
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		log.Printf("WARNING: Empty content in response")
		return "", fmt.Errorf("empty response from Gemini")
	}

	// Extract text from all parts
	var summary string
	for i, part := range candidate.Content.Parts {
		partText := fmt.Sprintf("%v", part)
		summary += partText
		log.Printf("Part %d length: %d chars", i+1, len(partText))
	}

	if summary == "" {
		log.Printf("WARNING: No text content extracted")
		return "", fmt.Errorf("no text content in response")
	}

	// Trim whitespace
	summary = strings.TrimSpace(summary)
	log.Printf("Summary generated successfully (output length: %d chars)", len(summary))
	
	// Validate summary isn't cut off mid-sentence
	if !strings.HasSuffix(summary, ".") && !strings.HasSuffix(summary, "!") && !strings.HasSuffix(summary, "?") {
		log.Printf("WARNING: Summary may be incomplete (doesn't end with punctuation)")
	}

	return summary, nil
}

// LanguageInfo contains information about a supported language
type LanguageInfo struct {
	Code         string
	Name         string
	NativeName   string
	Instructions string
}

// getLanguageInfo returns language configuration for a given code
func getLanguageInfo(code string) LanguageInfo {
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
			Instructions: "Verwenden Sie klare technische Sprache. Seien Sie präzise und professionell.",
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
	return getLanguageInfo(code).NativeName
}

// SummarizeInLanguage generates a TL;DR summary with translated title in the specified language with rate limiting
func (s *GeminiSummarizer) SummarizeInLanguage(ctx context.Context, text string, originalTitle string, languageCode string) (*SummaryResponse, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text provided")
	}

	log.Printf("Starting summary generation in language %s (input length: %d chars)", languageCode, len(text))

	// Get language info
	langInfo := getLanguageInfo(languageCode)

	// Create client
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(s.model)

	// Build language-specific prompt with JSON response format
	fullPrompt := fmt.Sprintf(`You are a technical news summarizer. Analyze the following article and provide:
1. A translated title in %s (keep it concise, under 100 characters)
2. A 3-5 sentence technical summary in %s highlighting key updates, improvements, or changes

%s

IMPORTANT:
- Respond ONLY with valid JSON
- Use this exact format: {"translated_title": "...", "summary": "..."}
- Do NOT include any preamble, explanation, markdown formatting, or code blocks
- Start directly with the JSON object
- The summary should be clear, technical, and professional
- If the title is already in %s, you can keep it similar but ensure it's natural

Original Title: %s

Article Content:
%s`,
		langInfo.Name,
		langInfo.Name,
		langInfo.Instructions,
		langInfo.Name,
		originalTitle,
		text,
	)

	// Count tokens before making request
	tokenResp, err := model.CountTokens(ctx, genai.Text(fullPrompt))
	if err != nil {
		log.Printf("WARNING: Failed to count tokens: %v (proceeding anyway)", err)
		tokenResp = &genai.CountTokensResponse{
			TotalTokens: int32(len(fullPrompt) / 4),
		}
	}

	inputTokens := int(tokenResp.TotalTokens)
	estimatedOutputTokens := 1500
	estimatedTotal := inputTokens + estimatedOutputTokens

	log.Printf("Token estimate for %s: input=%d, estimated_output=%d, total=%d", 
		languageCode, inputTokens, estimatedOutputTokens, estimatedTotal)

	// Check rate limits before making request
	can, err := s.rateLimiter.CanMakeRequest(estimatedTotal)
	if !can {
		log.Printf("Rate limit check failed for %s: %v", languageCode, err)
		// Try waiting for capacity
		waitErr := s.rateLimiter.WaitForCapacity(ctx, estimatedTotal)
		if waitErr != nil {
			return nil, fmt.Errorf("rate limit exceeded and wait failed: %w", err)
		}
		log.Printf("Rate limit capacity available after waiting for %s", languageCode)
	}

	// Retry logic with exponential backoff
	var rawResponse string
	var lastErr error
	maxRetries := s.rateLimiter.GetConfig().RetryAttempts

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := s.rateLimiter.CalculateBackoff(attempt - 1)
			log.Printf("Retry attempt %d/%d for %s after %v backoff", attempt, maxRetries, languageCode, backoff)
			
			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		rawResponse, lastErr = s.attemptSummarize(ctx, model, fullPrompt)
		
		if lastErr == nil {
			// Success - parse JSON response
			response, parseErr := s.parseJSONResponse(rawResponse, originalTitle, languageCode)
			if parseErr != nil {
				log.Printf("WARNING: JSON parse error for %s: %v", languageCode, parseErr)
				// Fallback: treat entire response as summary, keep original title
				response = &SummaryResponse{
					TranslatedTitle: originalTitle,
					Summary:         rawResponse,
				}
			}
			
			// Record request with actual token usage
			actualTokens := inputTokens + (len(rawResponse) / 4)
			s.rateLimiter.RecordRequest(actualTokens)
			log.Printf("Request successful for %s, recorded %d tokens", languageCode, actualTokens)
			log.Printf("Translated title (%s): %s", languageCode, response.TranslatedTitle)
			return response, nil
		}

		// Record failure for circuit breaker
		s.rateLimiter.RecordFailure()
		log.Printf("Attempt %d failed for %s: %v", attempt, languageCode, lastErr)

		// Check if we should retry
		if !s.shouldRetry(lastErr) {
			log.Printf("Error is not retryable for %s, aborting", languageCode)
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("failed after %d attempts for %s: %w", maxRetries+1, languageCode, lastErr)
}

// parseJSONResponse parses the JSON response from Gemini and validates it
func (s *GeminiSummarizer) parseJSONResponse(rawResponse string, originalTitle string, languageCode string) (*SummaryResponse, error) {
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
		log.Printf("Raw response (first 200 chars): %s", truncateString(cleanedResponse, 200))
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

// truncateString truncates a string to maxLength characters
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}

// shouldRetry determines if an error is retryable
func (s *GeminiSummarizer) shouldRetry(err error) bool {
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

// SetPrompt allows customizing the summarization prompt
func (s *GeminiSummarizer) SetPrompt(prompt string) {
	s.prompt = prompt
}

// SetModel allows changing the Gemini model
func (s *GeminiSummarizer) SetModel(model string) {
	s.model = model
}

// GetRateLimitStatistics returns current rate limiting statistics
func (s *GeminiSummarizer) GetRateLimitStatistics() ratelimit.Statistics {
	return s.rateLimiter.GetStatistics()
}

// ResetRateLimits resets rate limiting counters (useful for testing)
func (s *GeminiSummarizer) ResetRateLimits() {
	s.rateLimiter.Reset()
}
