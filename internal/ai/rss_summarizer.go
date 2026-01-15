package ai

import (
	"context"
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

// GeminiSummarizer implements AISummarizer using Google's Gemini API for RSS feeds
type GeminiSummarizer struct {
	apiKey      string
	model       string
	prompt      string
	rateLimiter *ratelimit.Manager
}

// NewGeminiSummarizer creates a new Gemini-based summarizer for RSS feeds
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

// SummarizeInLanguage generates a TL;DR summary with translated title in the specified language with rate limiting
func (s *GeminiSummarizer) SummarizeInLanguage(ctx context.Context, text string, originalTitle string, languageCode string) (*SummaryResponse, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text provided")
	}

	log.Printf("Starting RSS summary generation in language %s (input length: %d chars)", languageCode, len(text))

	// Get language info
	langInfo := GetLanguageInfo(languageCode)

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
			backoff := CalculateBackoff(s.rateLimiter, attempt-1)
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
			response, parseErr := ParseJSONResponse(rawResponse, originalTitle, languageCode)
			if parseErr != nil {
				log.Printf("WARNING: JSON parse error for %s: %v", languageCode, parseErr)
				log.Printf("Attempting to extract summary from malformed JSON...")
				
				// Smarter fallback: try to extract summary even from broken JSON
				extractedSummary := ExtractSummaryFromBrokenJSON(rawResponse)
				
				response = &SummaryResponse{
					TranslatedTitle: originalTitle,
					Summary:         extractedSummary,
				}
				
				log.Printf("Extracted summary (%d chars) from malformed JSON", len(extractedSummary))
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
		if !ShouldRetry(lastErr) {
			log.Printf("Error is not retryable for %s, aborting", languageCode)
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("failed after %d attempts for %s: %w", maxRetries+1, languageCode, lastErr)
}

// attemptSummarize performs a single summarization attempt
func (s *GeminiSummarizer) attemptSummarize(ctx context.Context, model *genai.GenerativeModel, prompt string) (string, error) {
	// Configure model for informative summaries
	model.SetTemperature(0.7)
	model.SetMaxOutputTokens(1500)
	model.SetTopP(0.95)
	model.SetTopK(40)

	log.Printf("Sending RSS summary request to Gemini API (model: %s)", s.model)

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
	log.Printf("RSS summary generated successfully (output length: %d chars)", len(summary))
	
	// Validate summary isn't cut off mid-sentence
	if !strings.HasSuffix(summary, ".") && !strings.HasSuffix(summary, "!") && !strings.HasSuffix(summary, "?") {
		log.Printf("WARNING: Summary may be incomplete (doesn't end with punctuation)")
	}

	return summary, nil
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
