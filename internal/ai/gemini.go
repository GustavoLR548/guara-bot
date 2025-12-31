package ai

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// AISummarizer defines the interface for AI-based text summarization
type AISummarizer interface {
	// Summarize generates a TL;DR summary in Brazilian Portuguese
	Summarize(ctx context.Context, text string) (string, error)
}

// GeminiSummarizer implements AISummarizer using Google's Gemini API
type GeminiSummarizer struct {
	apiKey string
	model  string
	prompt string
}

// NewGeminiSummarizer creates a new Gemini-based summarizer
func NewGeminiSummarizer(apiKey string) *GeminiSummarizer {
	return &GeminiSummarizer{
		apiKey: apiKey,
		model:  "gemini-2.5-flash",  // Using latest flash model (free tier)
		prompt: "Crie um resumo informativo em Português Brasileiro (PT-BR) para desenvolvedores de jogos sobre o seguinte artigo do Godot Engine. O resumo deve ter 3-5 frases, destacando as principais novidades, melhorias ou mudanças importantes. Seja claro, técnico e objetivo. IMPORTANTE: NÃO inclua nenhum preâmbulo, introdução ou frase como 'Aqui está um resumo'. Comece DIRETAMENTE com o conteúdo do resumo:",
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

// Summarize generates a TL;DR summary in Brazilian Portuguese
func (s *GeminiSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty text provided")
	}

	log.Printf("Starting summary generation (input length: %d chars)", len(text))

	// Create client
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	// Create model
	model := client.GenerativeModel(s.model)

	// Configure model for informative summaries
	model.SetTemperature(0.7)
	model.SetMaxOutputTokens(1500) // Increased for longer summaries
	model.SetTopP(0.95)              // Better response diversity
	model.SetTopK(40)                // Balance between creativity and accuracy

	// Build full prompt
	fullPrompt := fmt.Sprintf("%s\n\n%s", s.prompt, text)
	log.Printf("Sending request to Gemini API (model: %s)", s.model)

	// Generate content with timing
	startTime := time.Now()
	resp, err := model.GenerateContent(ctx, genai.Text(fullPrompt))
	duration := time.Since(startTime)
	
	if err != nil {
		log.Printf("Gemini API error after %v: %v", duration, err)
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}
	
	log.Printf("Gemini API responded in %v", duration)

	// Extract text from response
	if len(resp.Candidates) == 0 {
		log.Printf("WARNING: No candidates in response")
		return "", fmt.Errorf("no summary generated")
	}

	candidate := resp.Candidates[0]
	
	// Log finish reason for debugging
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

// SetPrompt allows customizing the summarization prompt
func (s *GeminiSummarizer) SetPrompt(prompt string) {
	s.prompt = prompt
}

// SetModel allows changing the Gemini model
func (s *GeminiSummarizer) SetModel(model string) {
	s.model = model
}
