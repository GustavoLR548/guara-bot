package ai

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockAISummarizer is a mock implementation of AISummarizer for testing
type MockAISummarizer struct {
	SummarizeFunc          func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error)
	SummarizeInLanguageFunc func(ctx context.Context, text string, originalTitle string, languageCode string) (*SummaryResponse, error)
}

func (m *MockAISummarizer) Summarize(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
	if m.SummarizeFunc != nil {
		return m.SummarizeFunc(ctx, text, originalTitle)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *MockAISummarizer) SummarizeInLanguage(ctx context.Context, text string, originalTitle string, languageCode string) (*SummaryResponse, error) {
	if m.SummarizeInLanguageFunc != nil {
		return m.SummarizeInLanguageFunc(ctx, text, originalTitle, languageCode)
	}
	return nil, fmt.Errorf("not implemented")
}

// TestMockAISummarizer_Summarize tests the mock implementation
func TestMockAISummarizer_Summarize(t *testing.T) {
	tests := []struct {
		name               string
		inputText          string
		inputTitle         string
		mockResponse       *SummaryResponse
		mockError          error
		expectError        bool
		expectedTitle      string
		expectedSummary    string
	}{
		{
			name:       "successful summarization",
			inputText:  "Long article text here",
			inputTitle: "Godot 4.3 Released",
			mockResponse: &SummaryResponse{
				TranslatedTitle: "Godot 4.3 Released",
				Summary:         "TL;DR: Summary in English",
			},
			mockError:       nil,
			expectError:     false,
			expectedTitle:   "Godot 4.3 Released",
			expectedSummary: "TL;DR: Summary in English",
		},
		{
			name:        "error during summarization",
			inputText:   "Some text",
			inputTitle:  "Test Title",
			mockError:   fmt.Errorf("API error"),
			expectError: true,
		},
		{
			name:       "empty summary response",
			inputText:  "Text to summarize",
			inputTitle: "Another Title",
			mockResponse: &SummaryResponse{
				TranslatedTitle: "Another Title",
				Summary:         "",
			},
			expectError:     false,
			expectedTitle:   "Another Title",
			expectedSummary: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockAISummarizer{
				SummarizeFunc: func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
					assert.Equal(t, tt.inputText, text)
					assert.Equal(t, tt.inputTitle, originalTitle)
					return tt.mockResponse, tt.mockError
				},
			}

			ctx := context.Background()
			result, err := mock.Summarize(ctx, tt.inputText, tt.inputTitle)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if result != nil {
					assert.Equal(t, tt.expectedTitle, result.TranslatedTitle)
					assert.Equal(t, tt.expectedSummary, result.Summary)
				}
			}
		})
	}
}

// TestGeminiSummarizer_Summarize_EmptyInput tests validation
func TestGeminiSummarizer_Summarize_EmptyInput(t *testing.T) {
	// We don't need a real API key for this test
	summarizer := NewGeminiSummarizer("fake-api-key")

	ctx := context.Background()
	_, err := summarizer.Summarize(ctx, "", "Test Title")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty text")
}

// TestGeminiSummarizer_SetPrompt tests custom prompt setting
func TestGeminiSummarizer_SetPrompt(t *testing.T) {
	summarizer := NewGeminiSummarizer("fake-api-key")

	customPrompt := "Custom prompt for testing"
	summarizer.SetPrompt(customPrompt)

	assert.Equal(t, customPrompt, summarizer.prompt)
}

// TestGeminiSummarizer_SetModel tests model configuration
func TestGeminiSummarizer_SetModel(t *testing.T) {
	summarizer := NewGeminiSummarizer("fake-api-key")

	customModel := "gemini-pro"
	summarizer.SetModel(customModel)

	assert.Equal(t, customModel, summarizer.model)
}

// TestGeminiSummarizer_DefaultConfiguration tests default settings
func TestGeminiSummarizer_DefaultConfiguration(t *testing.T) {
	apiKey := "test-api-key-123"
	summarizer := NewGeminiSummarizer(apiKey)

	assert.Equal(t, apiKey, summarizer.apiKey)
	assert.Equal(t, "gemini-2.5-flash", summarizer.model)
	assert.Contains(t, summarizer.prompt, "resumo")
	assert.Contains(t, summarizer.prompt, "PT-BR")
	assert.Contains(t, summarizer.prompt, "desenvolvedores")
}

// TestGeminiSummarizer_ContextCancellation tests context handling
func TestGeminiSummarizer_ContextCancellation(t *testing.T) {
	summarizer := NewGeminiSummarizer("fake-api-key")

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := summarizer.Summarize(ctx, "Some text to summarize", "Test Title")

	// Should fail because context is cancelled
	require.Error(t, err)
}

// TestAISummarizerInterface_Compliance tests that implementations comply with interface
func TestAISummarizerInterface_Compliance(t *testing.T) {
	tests := []struct {
		name        string
		summarizer  AISummarizer
		description string
	}{
		{
			name: "GeminiSummarizer complies",
			summarizer: NewGeminiSummarizer("test-key"),
			description: "GeminiSummarizer should implement AISummarizer",
		},
		{
			name: "MockAISummarizer complies",
			summarizer: &MockAISummarizer{
				SummarizeFunc: func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
					return &SummaryResponse{
						TranslatedTitle: originalTitle,
						Summary:         "mock summary",
					}, nil
				},
			},
			description: "MockAISummarizer should implement AISummarizer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test just verifies interface compliance at compile time
			var _ AISummarizer = tt.summarizer
			assert.NotNil(t, tt.summarizer, tt.description)
		})
	}
}

// TestMockAISummarizer_TableDriven tests various mock scenarios
func TestMockAISummarizer_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func() *MockAISummarizer
		inputText      string
		inputTitle     string
		expectError    bool
		validateResult func(*testing.T, *SummaryResponse)
	}{
		{
			name: "returns English summary",
			setupMock: func() *MockAISummarizer {
				return &MockAISummarizer{
					SummarizeFunc: func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
						return &SummaryResponse{
							TranslatedTitle: originalTitle,
							Summary:         "TL;DR: This is a summary in English",
						}, nil
					},
				}
			},
			inputText:   "Article in English",
			inputTitle:  "Test Article",
			expectError: false,
			validateResult: func(t *testing.T, result *SummaryResponse) {
				assert.Contains(t, result.Summary, "TL;DR")
				assert.Contains(t, result.Summary, "English")
				assert.Equal(t, "Test Article", result.TranslatedTitle)
			},
		},
		{
			name: "handles long text",
			setupMock: func() *MockAISummarizer {
				return &MockAISummarizer{
					SummarizeFunc: func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
						assert.Greater(t, len(text), 100)
						return &SummaryResponse{
							TranslatedTitle: originalTitle,
							Summary:         "Short summary",
						}, nil
					},
				}
			},
			inputText:   string(make([]byte, 500)), // Long text
			inputTitle:  "Long Article",
			expectError: false,
		},
		{
			name: "simulates API rate limit error",
			setupMock: func() *MockAISummarizer {
				return &MockAISummarizer{
					SummarizeFunc: func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
						return nil, fmt.Errorf("API rate limit exceeded")
					},
				}
			},
			inputText:   "Some text",
			inputTitle:  "Rate Limited",
			expectError: true,
		},
		{
			name: "simulates network timeout",
			setupMock: func() *MockAISummarizer{
				return &MockAISummarizer{
					SummarizeFunc: func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
						return nil, fmt.Errorf("context deadline exceeded")
					},
				}
			},
			inputText:   "Article content",
			inputTitle:  "Timeout Test",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			ctx := context.Background()

			result, err := mock.Summarize(ctx, tt.inputText, tt.inputTitle)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

// Example of how to use the mock in other tests
func ExampleMockAISummarizer() {
	// Create a mock that returns a predefined summary
	mock := &MockAISummarizer{
		SummarizeFunc: func(ctx context.Context, text string, originalTitle string) (*SummaryResponse, error) {
			return &SummaryResponse{
				TranslatedTitle: "Godot 4.3 Released",
				Summary:         "TL;DR: Godot 4.3 was released with new features!",
			}, nil
		},
	}

	ctx := context.Background()
	response, err := mock.Summarize(ctx, "Long article about Godot 4.3 release...", "Godot 4.3 Release")
	
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(response.TranslatedTitle)
	fmt.Println(response.Summary)
	// Output: Godot 4.3 Released
	// TL;DR: Godot 4.3 was released with new features!
}

// TestGeminiSummarizer_RateLimiting tests rate limiting integration
func TestGeminiSummarizer_RateLimiting(t *testing.T) {
	summarizer := NewGeminiSummarizer("fake-api-key")
	
	// Get initial statistics
	stats := summarizer.GetRateLimitStatistics()
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.False(t, stats.CircuitOpen)
}

// TestGeminiSummarizer_WithCustomRateLimit tests custom rate limiting configuration
func TestGeminiSummarizer_WithCustomRateLimit(t *testing.T) {
	config := ratelimit.Config{
		MaxRequestsPerMinute:    5,
		MaxTokensPerMinute:      100000,
		MaxTokensPerRequest:     2000,
		CircuitBreakerThreshold: 3,
		CircuitBreakerTimeout:   1 * time.Minute,
		RetryAttempts:           2,
		RetryBackoffBase:        500 * time.Millisecond,
	}
	
	summarizer := NewGeminiSummarizerWithRateLimit("fake-api-key", config)
	assert.NotNil(t, summarizer)
	assert.Equal(t, "gemini-2.5-flash", summarizer.model)
}

// TestGeminiSummarizer_ResetRateLimits tests rate limit reset
func TestGeminiSummarizer_ResetRateLimits(t *testing.T) {
	summarizer := NewGeminiSummarizer("fake-api-key")
	
	// Simulate some usage (via recording directly for testing)
	summarizer.rateLimiter.RecordRequest(1000)
	summarizer.rateLimiter.RecordRequest(2000)
	
	stats := summarizer.GetRateLimitStatistics()
	assert.Greater(t, stats.TotalRequests, int64(0))
	
	// Reset
	summarizer.ResetRateLimits()
	
	stats = summarizer.GetRateLimitStatistics()
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.TotalTokens)
}

// TestGeminiSummarizer_ShouldRetry tests retry logic
func TestGeminiSummarizer_ShouldRetry(t *testing.T) {
	summarizer := NewGeminiSummarizer("fake-api-key")
	
	tests := []struct {
		name         string
		err          error
		shouldRetry  bool
	}{
		{
			name:        "nil error",
			err:         nil,
			shouldRetry: false,
		},
		{
			name:        "rate limit error",
			err:         fmt.Errorf("429 rate limit exceeded"),
			shouldRetry: true,
		},
		{
			name:        "service unavailable",
			err:         fmt.Errorf("503 service temporarily unavailable"),
			shouldRetry: true,
		},
		{
			name:        "timeout error",
			err:         fmt.Errorf("context deadline exceeded"),
			shouldRetry: true,
		},
		{
			name:        "bad request",
			err:         fmt.Errorf("400 bad request"),
			shouldRetry: false,
		},
		{
			name:        "unauthorized",
			err:         fmt.Errorf("401 unauthorized"),
			shouldRetry: false,
		},
		{
			name:        "not found",
			err:         fmt.Errorf("404 model not found"),
			shouldRetry: false,
		},
		{
			name:        "connection reset",
			err:         fmt.Errorf("connection reset by peer"),
			shouldRetry: true,
		},
		{
			name:        "unknown error",
			err:         fmt.Errorf("unknown error occurred"),
			shouldRetry: true, // Default to retry
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizer.shouldRetry(tt.err)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}
