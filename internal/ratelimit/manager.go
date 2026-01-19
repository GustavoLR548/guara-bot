package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Config holds rate limiting configuration
type Config struct {
	MaxRequestsPerMinute int           // Maximum API requests per minute
	MaxTokensPerMinute   int           // Maximum tokens (input + output) per minute
	MaxTokensPerRequest  int           // Maximum tokens per single request
	CircuitBreakerThreshold int        // Failures before opening circuit
	CircuitBreakerTimeout time.Duration // Time to wait before retrying after circuit opens
	RetryAttempts        int           // Number of retry attempts
	RetryBackoffBase     time.Duration // Base duration for exponential backoff
}

// DefaultConfig returns conservative rate limiting configuration for Gemini free tier
func DefaultConfig() Config {
	return Config{
		MaxRequestsPerMinute:    10,              // Conservative: well below 15 RPM limit
		MaxTokensPerMinute:      200000,          // Conservative: below 250k TPM limit
		MaxTokensPerRequest:     4000,            // Safe per-request limit
		CircuitBreakerThreshold: 5,               // Open circuit after 5 consecutive failures
		CircuitBreakerTimeout:   5 * time.Minute, // Wait 5 minutes before retrying
		RetryAttempts:           3,               // Retry up to 3 times
		RetryBackoffBase:        1 * time.Second, // Start with 1s backoff
	}
}

// Manager handles rate limiting, token counting, and circuit breaking
type Manager struct {
	config Config
	mu     sync.RWMutex
	
	// Rate limiting state
	requestCount      int
	tokenCount        int
	windowStart       time.Time
	
	// Circuit breaker state
	circuitOpen       bool
	failureCount      int
	lastFailureTime   time.Time
	
	// Statistics
	totalRequests     int64
	totalTokens       int64
	totalFailures     int64
}

// NewManager creates a new rate limit manager
func NewManager(config Config) *Manager {
	return &Manager{
		config:      config,
		windowStart: time.Now(),
	}
}

// GetConfig returns the current rate limiting configuration
func (m *Manager) GetConfig() Config {
	return m.config
}

// TokenEstimate represents estimated token usage
type TokenEstimate struct {
	InputTokens      int
	EstimatedOutput  int
	Total            int
	ExceedsLimit     bool
	RemainingInWindow int
}

// EstimateTokens calculates estimated token usage for a request
func (m *Manager) EstimateTokens(inputTokens, estimatedOutputTokens int) TokenEstimate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	total := inputTokens + estimatedOutputTokens
	remaining := m.config.MaxTokensPerMinute - m.tokenCount
	
	return TokenEstimate{
		InputTokens:      inputTokens,
		EstimatedOutput:  estimatedOutputTokens,
		Total:            total,
		ExceedsLimit:     total > m.config.MaxTokensPerRequest,
		RemainingInWindow: remaining,
	}
}

// CanMakeRequest checks if a request can be made within rate limits
func (m *Manager) CanMakeRequest(estimatedTokens int) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Check circuit breaker
	if m.circuitOpen {
		if time.Since(m.lastFailureTime) < m.config.CircuitBreakerTimeout {
			return false, fmt.Errorf("circuit breaker open: waiting %v before retry", 
				m.config.CircuitBreakerTimeout - time.Since(m.lastFailureTime))
		}
	}
	
	// Reset window if needed
	if time.Since(m.windowStart) >= time.Minute {
		return true, nil // Will reset in recordRequest
	}
	
	// Check request limit
	if m.requestCount >= m.config.MaxRequestsPerMinute {
		waitTime := time.Minute - time.Since(m.windowStart)
		return false, fmt.Errorf("request rate limit exceeded: wait %v", waitTime)
	}
	
	// Check token limit
	if m.tokenCount + estimatedTokens > m.config.MaxTokensPerMinute {
		waitTime := time.Minute - time.Since(m.windowStart)
		return false, fmt.Errorf("token rate limit exceeded: wait %v", waitTime)
	}
	
	// Check per-request token limit
	if estimatedTokens > m.config.MaxTokensPerRequest {
		return false, fmt.Errorf("request exceeds max tokens per request (%d > %d)", 
			estimatedTokens, m.config.MaxTokensPerRequest)
	}
	
	return true, nil
}

// WaitForCapacity blocks until capacity is available or context is cancelled
func (m *Manager) WaitForCapacity(ctx context.Context, estimatedTokens int) error {
	for {
		can, err := m.CanMakeRequest(estimatedTokens)
		if can {
			return nil
		}
		
		// Extract wait time from error message if available
		waitTime := 1 * time.Second
		if err != nil {
			// Parse wait time from error or use default
			waitTime = m.getWaitTime()
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue loop to check again
		}
	}
}

// getWaitTime calculates how long to wait before retrying
func (m *Manager) getWaitTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if time.Since(m.windowStart) >= time.Minute {
		return 0
	}
	
	return time.Minute - time.Since(m.windowStart)
}

// RecordRequest records a successful request and its token usage
func (m *Manager) RecordRequest(tokensUsed int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Reset window if needed
	if time.Since(m.windowStart) >= time.Minute {
		m.requestCount = 0
		m.tokenCount = 0
		m.windowStart = time.Now()
	}
	
	m.requestCount++
	m.tokenCount += tokensUsed
	m.totalRequests++
	m.totalTokens += int64(tokensUsed)
	
	// Reset circuit breaker on success
	m.failureCount = 0
	if m.circuitOpen {
		m.circuitOpen = false
	}
}

// RecordFailure records a failed request and updates circuit breaker
func (m *Manager) RecordFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.failureCount++
	m.totalFailures++
	m.lastFailureTime = time.Now()
	
	// Open circuit if threshold reached
	if m.failureCount >= m.config.CircuitBreakerThreshold {
		m.circuitOpen = true
	}
}

// IsCircuitOpen returns whether the circuit breaker is currently open
func (m *Manager) IsCircuitOpen() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.circuitOpen {
		return false
	}
	
	// Check if timeout has passed
	if time.Since(m.lastFailureTime) >= m.config.CircuitBreakerTimeout {
		return false
	}
	
	return true
}

// GetStatistics returns current rate limiting statistics
func (m *Manager) GetStatistics() Statistics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return Statistics{
		CurrentWindowRequests: m.requestCount,
		CurrentWindowTokens:   m.tokenCount,
		WindowTimeRemaining:   time.Minute - time.Since(m.windowStart),
		CircuitOpen:           m.circuitOpen,
		TotalRequests:         m.totalRequests,
		TotalTokens:           m.totalTokens,
		TotalFailures:         m.totalFailures,
	}
}

// Statistics contains rate limiting metrics
type Statistics struct {
	CurrentWindowRequests int
	CurrentWindowTokens   int
	WindowTimeRemaining   time.Duration
	CircuitOpen           bool
	TotalRequests         int64
	TotalTokens           int64
	TotalFailures         int64
}

// Reset resets all rate limiting counters (useful for testing)
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.requestCount = 0
	m.tokenCount = 0
	m.windowStart = time.Now()
	m.circuitOpen = false
	m.failureCount = 0
	m.totalRequests = 0
	m.totalTokens = 0
	m.totalFailures = 0
}

// CalculateBackoff returns exponential backoff duration for retry attempt
func (m *Manager) CalculateBackoff(attempt int) time.Duration {
	base := m.config.RetryBackoffBase
	// Prevent integer overflow by capping attempt value
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 30 { // 2^30 seconds is already > 34 years
		attempt = 30
	}
	// Exponential: 1s, 2s, 4s, 8s, etc.
	backoff := base * time.Duration(1<<uint(attempt))
	
	// Cap at 1 minute
	if backoff > time.Minute {
		backoff = time.Minute
	}
	
	return backoff
}
