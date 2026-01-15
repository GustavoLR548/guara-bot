package ai

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/github"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// EstimatePRBatchTokens estimates the total tokens needed for a batch of PRs
func EstimatePRBatchTokens(prs []github.PullRequest, languageCode string) int {
	// Rough estimation: 1 token ≈ 4 characters
	totalChars := 0
	
	// Base prompt overhead (instructions, formatting)
	totalChars += 800
	
	// Language-specific instructions
	langInfo := GetLanguageInfo(languageCode)
	totalChars += len(langInfo.Instructions) + len(langInfo.Name)
	
	// Per-PR content
	for _, pr := range prs {
		// PR metadata
		totalChars += len(pr.Title) + len(pr.Author) + len(pr.HTMLURL)
		
		// PR body (truncate estimate at 500 chars per PR)
		bodyChars := len(pr.Body)
		if bodyChars > 500 {
			bodyChars = 500
		}
		totalChars += bodyChars
		
		// Labels
		for _, label := range pr.Labels {
			totalChars += len(label.Name)
		}
		
		// Formatting overhead (~150 chars per PR)
		totalChars += 150
	}
	
	// Output tokens estimate (roughly 100 tokens per PR + 200 base)
	outputTokens := (len(prs) * 100) + 200
	
	inputTokens := totalChars / 4
	return inputTokens + outputTokens
}

// FitPRsWithinTokenLimit returns the maximum number of PRs that fit within token limit
func FitPRsWithinTokenLimit(prs []github.PullRequest, languageCode string, maxTokens int) int {
	if len(prs) == 0 {
		return 0
	}
	
	// Try all PRs first
	if EstimatePRBatchTokens(prs, languageCode) <= maxTokens {
		return len(prs)
	}
	
	// Binary search for optimal batch size
	left, right := 1, len(prs)
	result := 0
	
	for left <= right {
		mid := (left + right) / 2
		estimate := EstimatePRBatchTokens(prs[:mid], languageCode)
		
		if estimate <= maxTokens {
			result = mid
			left = mid + 1
		} else {
			right = mid - 1
		}
	}
	
	log.Printf("Token optimization: %d PRs fit within %d token limit (estimated: %d tokens)", 
		result, maxTokens, EstimatePRBatchTokens(prs[:result], languageCode))
	
	return result
}

// CollectAllParts collects all parts from a Gemini response
func CollectAllParts(candidate *genai.Candidate) string {
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return ""
	}
	var result strings.Builder
	for _, part := range candidate.Content.Parts {
		result.WriteString(fmt.Sprintf("%v", part))
	}
	return result.String()
}

// StripPreamble removes common AI preambles from responses
func StripPreamble(text string) string {
	// Common preamble patterns to remove (only at the very start)
	preambles := []string{
		"Here is a summary of the",
		"Here's a summary of the",
		"Voici un résumé des",
		"Voilà un résumé des",
		"Here is the summary:",
		"Here's the summary:",
		"Voici le résumé :",
		"Summary:",
		"Résumé :",
	}
	
	// Trim any leading/trailing whitespace first
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	
	for _, preamble := range preambles {
		if strings.HasPrefix(lower, strings.ToLower(preamble)) {
			// Find where the preamble ends
			// Look for first newline after preamble
			idx := strings.Index(text, "\n")
			if idx > 0 && idx < 150 { // Within reasonable preamble length
				text = text[idx+1:]
				log.Printf("Stripped preamble (ended at newline)")
			} else {
				// No newline, try to find colon
				idx = strings.Index(text, ":")
				if idx > 0 && idx < 100 {
					text = text[idx+1:]
					log.Printf("Stripped preamble (ended at colon)")
				}
			}
			break
		}
	}
	
	// Remove leading "---" separator if present (but only at the very start)
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "---") {
		text = strings.TrimPrefix(text, "---")
		text = strings.TrimSpace(text)
	}
	
	return strings.TrimSpace(text)
}

// PRSummarizer defines the interface for summarizing GitHub PRs
type PRSummarizer interface {
	// SummarizePRBatch generates a summary for a batch of PRs
	SummarizePRBatch(ctx context.Context, repoName string, prs []github.PullRequest, languageCode string) (string, error)
}

// GeminiPRSummarizer implements PRSummarizer using the existing Gemini client
type GeminiPRSummarizer struct {
	summarizer *GeminiSummarizer
}

// NewGeminiPRSummarizer creates a new Gemini-based PR summarizer
func NewGeminiPRSummarizer(summarizer *GeminiSummarizer) *GeminiPRSummarizer {
	return &GeminiPRSummarizer{
		summarizer: summarizer,
	}
}

// SummarizePRBatch generates a categorized summary for a batch of PRs
func (s *GeminiPRSummarizer) SummarizePRBatch(ctx context.Context, repoName string, prs []github.PullRequest, languageCode string) (string, error) {
	if len(prs) == 0 {
		return "", fmt.Errorf("no PRs to summarize")
	}
	
	// Validate token limits before processing
	maxTokens := 30000 // Conservative limit for Gemini 2.5 Flash
	estimatedTokens := EstimatePRBatchTokens(prs, languageCode)
	
	if estimatedTokens > maxTokens {
		return "", fmt.Errorf("batch too large: estimated %d tokens exceeds limit of %d (use FitPRsWithinTokenLimit to reduce batch size)", estimatedTokens, maxTokens)
	}
	
	log.Printf("Generating summary for %d PRs from %s in language %s (estimated: %d tokens)", len(prs), repoName, languageCode, estimatedTokens)
	
	// Get language info
	langInfo := GetLanguageInfo(languageCode)
	
	// Build PR list with categorization
	var prList strings.Builder
	categorized := make(map[string][]github.PullRequest)
	
	for _, pr := range prs {
		category := github.CategorizePR(pr)
		categorized[category] = append(categorized[category], pr)
	}
	
	// Format PRs by category
	for category, categoryPRs := range categorized {
		prList.WriteString(fmt.Sprintf("\n## %s:\n", category))
		for _, pr := range categoryPRs {
			// Get label names
			labelNames := make([]string, len(pr.Labels))
			for i, label := range pr.Labels {
				labelNames[i] = label.Name
			}
			
			prList.WriteString(fmt.Sprintf("- PR #%d: %s\n", pr.Number, pr.Title))
			prList.WriteString(fmt.Sprintf("  Author: %s\n", pr.Author))
			prList.WriteString(fmt.Sprintf("  Labels: %s\n", strings.Join(labelNames, ", ")))
			prList.WriteString(fmt.Sprintf("  URL: %s\n", pr.HTMLURL))
			if pr.Body != "" {
				// Truncate body to first 200 chars
				body := pr.Body
				if len(body) > 200 {
					body = body[:200] + "..."
				}
				prList.WriteString(fmt.Sprintf("  Description: %s\n", body))
			}
			prList.WriteString("\n")
		}
	}
	
	// Build prompt for Gemini
	prompt := fmt.Sprintf(`You are a technical news summarizer for the repository "%s". Analyze the following %d merged pull requests and create a concise, developer-focused summary in %s.

%s

CRITICAL: Start DIRECTLY with the categorized content. Do NOT include any preamble, introduction, or phrases like "Here is a summary" or "Voici un résumé". Begin immediately with the first category header.

REQUIREMENTS:
1. Group changes by category (Features, Bugfixes, Performance, UI/UX, Security, etc.)
2. For each significant PR, provide:
   - A brief "Why it matters" explanation (1 sentence)
   - The direct link to the PR
3. Keep it concise and scannable - developers should understand the key changes in under 2 minutes
4. Maintain a professional, technical tone
5. Format for Discord embed (use markdown)

OUTPUT FORMAT (example):
**🚀 Features**
• **[PR #123](url)**: Added new caching system - Improves performance by 40%% for repeated queries
• **[PR #456](url)**: Implemented dark mode - Enhances user experience with system theme support

**🐛 Bugfixes**
• **[PR #789](url)**: Fixed memory leak in worker pool - Prevents crashes during high load

**⚡ Performance**
• **[PR #234](url)**: Optimized database queries - Reduces API response time by 30%%

Repository: %s
Pull Requests:
%s`,
		repoName,
		len(prs),
		langInfo.Name,
		langInfo.Instructions,
		repoName,
		prList.String(),
	)
	
	// Check rate limits (reuse the estimated tokens from earlier)
	can, err := s.summarizer.rateLimiter.CanMakeRequest(estimatedTokens)
	if !can {
		log.Printf("Rate limit check failed: %v", err)
		waitErr := s.summarizer.rateLimiter.WaitForCapacity(ctx, estimatedTokens)
		if waitErr != nil {
			return "", fmt.Errorf("rate limit exceeded: %w", err)
		}
	}
	
	// Generate summary with retry logic
	var summary string
	var lastErr error
	maxRetries := s.summarizer.rateLimiter.GetConfig().RetryAttempts
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := CalculateBackoff(s.summarizer.rateLimiter, attempt-1)
			log.Printf("Retry attempt %d/%d after %v", attempt, maxRetries, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		
		// Call Gemini API
		summary, lastErr = s.callGemini(ctx, prompt)
		if lastErr == nil {
			// Success
			actualTokens := estimatedTokens + (len(summary) / 4)
			s.summarizer.rateLimiter.RecordRequest(actualTokens)
			log.Printf("PR summary generated successfully (%d chars)", len(summary))
			return summary, nil
		}
		
		s.summarizer.rateLimiter.RecordFailure()
		log.Printf("Attempt %d failed: %v", attempt, lastErr)
	}
	
	return "", fmt.Errorf("failed to generate PR summary after %d attempts: %w", maxRetries+1, lastErr)
}

// callGemini makes the actual API call to Gemini
func (s *GeminiPRSummarizer) callGemini(ctx context.Context, prompt string) (string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.summarizer.apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()
	
	model := client.GenerativeModel(s.summarizer.model)
	model.SetTemperature(0.7)
	model.SetMaxOutputTokens(8000) // Increased for large PR batches (was 2000)
	model.SetTopP(0.95)
	model.SetTopK(40)
	
	log.Printf("Sending PR summary request to Gemini API")
	startTime := time.Now()
	
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	duration := time.Since(startTime)
	
	if err != nil {
		log.Printf("Gemini API error after %v: %v", duration, err)
		return "", fmt.Errorf("API request failed: %w", err)
	}
	
	log.Printf("Gemini API responded in %v", duration)
	
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no summary generated")
	}
	
	candidate := resp.Candidates[0]
	
	// Check finish reason
	if candidate.FinishReason == genai.FinishReasonMaxTokens {
		log.Printf("ERROR: PR summary truncated due to max tokens limit - this batch is too large")
		return "", fmt.Errorf("response truncated: batch too large for token limit (try reducing batch size)")
	}
	
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}
	
	// Collect all parts (wait for complete response)
	summary := CollectAllParts(candidate)
	log.Printf("Collected summary: %d characters (before preamble strip)", len(summary))
	
	// Strip common preambles
	summary = StripPreamble(summary)
	log.Printf("After preamble strip: %d characters", len(summary))
	summary = strings.TrimSpace(summary)
	
	if summary == "" {
		return "", fmt.Errorf("no text content in response")
	}
	
	// Sanity check: summary should be reasonable length for the number of PRs
	if len(summary) < 100 {
		log.Printf("WARNING: Summary suspiciously short (%d chars) for this batch", len(summary))
	}
	
	return summary, nil
}
