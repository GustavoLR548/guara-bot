package ratelimit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	
	assert.Equal(t, 10, config.MaxRequestsPerMinute)
	assert.Equal(t, 200000, config.MaxTokensPerMinute)
	assert.Equal(t, 4000, config.MaxTokensPerRequest)
	assert.Equal(t, 5, config.CircuitBreakerThreshold)
	assert.Equal(t, 5*time.Minute, config.CircuitBreakerTimeout)
	assert.Equal(t, 3, config.RetryAttempts)
}

func TestNewManager(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)
	
	assert.NotNil(t, manager)
	assert.Equal(t, config, manager.config)
	assert.Equal(t, 0, manager.requestCount)
	assert.Equal(t, 0, manager.tokenCount)
	assert.False(t, manager.circuitOpen)
}

func TestManager_EstimateTokens(t *testing.T) {
	tests := []struct {
		name                string
		inputTokens         int
		estimatedOutput     int
		expectedTotal       int
		expectedExceeds     bool
	}{
		{
			name:            "small request",
			inputTokens:     100,
			estimatedOutput: 200,
			expectedTotal:   300,
			expectedExceeds: false,
		},
		{
			name:            "large request exceeds limit",
			inputTokens:     3000,
			estimatedOutput: 2000,
			expectedTotal:   5000,
			expectedExceeds: true,
		},
		{
			name:            "at limit boundary",
			inputTokens:     2000,
			estimatedOutput: 2000,
			expectedTotal:   4000,
			expectedExceeds: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(DefaultConfig())
			
			estimate := manager.EstimateTokens(tt.inputTokens, tt.estimatedOutput)
			
			assert.Equal(t, tt.inputTokens, estimate.InputTokens)
			assert.Equal(t, tt.estimatedOutput, estimate.EstimatedOutput)
			assert.Equal(t, tt.expectedTotal, estimate.Total)
			assert.Equal(t, tt.expectedExceeds, estimate.ExceedsLimit)
		})
	}
}

func TestManager_CanMakeRequest(t *testing.T) {
	tests := []struct {
		name            string
		setupManager    func(*Manager)
		estimatedTokens int
		expectAllow     bool
		expectError     bool
	}{
		{
			name: "first request should be allowed",
			setupManager: func(m *Manager) {
				// Fresh manager, no setup needed
			},
			estimatedTokens: 1000,
			expectAllow:     true,
			expectError:     false,
		},
		{
			name: "request exceeding per-request limit should fail",
			setupManager: func(m *Manager) {
				// No setup needed
			},
			estimatedTokens: 5000, // Exceeds MaxTokensPerRequest (4000)
			expectAllow:     false,
			expectError:     true,
		},
		{
			name: "request when circuit is open should fail",
			setupManager: func(m *Manager) {
				// Trigger circuit breaker
				for i := 0; i < 5; i++ {
					m.RecordFailure()
				}
			},
			estimatedTokens: 1000,
			expectAllow:     false,
			expectError:     true,
		},
		{
			name: "request exceeding token limit should fail",
			setupManager: func(m *Manager) {
				// Use up most of the token budget
				m.RecordRequest(195000)
			},
			estimatedTokens: 10000, // Would exceed 200k limit
			expectAllow:     false,
			expectError:     true,
		},
		{
			name: "request exceeding request limit should fail",
			setupManager: func(m *Manager) {
				// Max out request count
				for i := 0; i < 10; i++ {
					m.RecordRequest(100)
				}
			},
			estimatedTokens: 1000,
			expectAllow:     false,
			expectError:     true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(DefaultConfig())
			tt.setupManager(manager)
			
			can, err := manager.CanMakeRequest(tt.estimatedTokens)
			
			assert.Equal(t, tt.expectAllow, can)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_RecordRequest(t *testing.T) {
	manager := NewManager(DefaultConfig())
	
	// Record first request
	manager.RecordRequest(1000)
	
	stats := manager.GetStatistics()
	assert.Equal(t, 1, stats.CurrentWindowRequests)
	assert.Equal(t, 1000, stats.CurrentWindowTokens)
	assert.Equal(t, int64(1), stats.TotalRequests)
	assert.Equal(t, int64(1000), stats.TotalTokens)
	
	// Record second request
	manager.RecordRequest(500)
	
	stats = manager.GetStatistics()
	assert.Equal(t, 2, stats.CurrentWindowRequests)
	assert.Equal(t, 1500, stats.CurrentWindowTokens)
	assert.Equal(t, int64(2), stats.TotalRequests)
	assert.Equal(t, int64(1500), stats.TotalTokens)
}

func TestManager_RecordFailure(t *testing.T) {
	manager := NewManager(DefaultConfig())
	
	// Record failures below threshold
	for i := 0; i < 4; i++ {
		manager.RecordFailure()
	}
	assert.False(t, manager.IsCircuitOpen(), "Circuit should remain closed below threshold")
	
	// Record one more to trigger circuit breaker
	manager.RecordFailure()
	assert.True(t, manager.IsCircuitOpen(), "Circuit should open after threshold")
	
	stats := manager.GetStatistics()
	assert.Equal(t, int64(5), stats.TotalFailures)
}

func TestManager_CircuitBreaker(t *testing.T) {
	config := DefaultConfig()
	config.CircuitBreakerTimeout = 100 * time.Millisecond // Short timeout for testing
	manager := NewManager(config)
	
	// Trigger circuit breaker
	for i := 0; i < 5; i++ {
		manager.RecordFailure()
	}
	
	// Circuit should be open
	assert.True(t, manager.IsCircuitOpen())
	can, err := manager.CanMakeRequest(1000)
	assert.False(t, can)
	assert.Error(t, err)
	
	// Wait for timeout
	time.Sleep(150 * time.Millisecond)
	
	// Circuit should close after timeout
	assert.False(t, manager.IsCircuitOpen())
	can, err = manager.CanMakeRequest(1000)
	assert.True(t, can)
	assert.NoError(t, err)
}

func TestManager_WindowReset(t *testing.T) {
	config := DefaultConfig()
	config.MaxRequestsPerMinute = 2
	manager := NewManager(config)
	
	// Fill up the window
	manager.RecordRequest(1000)
	manager.RecordRequest(1000)
	
	// Third request should fail
	can, err := manager.CanMakeRequest(1000)
	assert.False(t, can)
	assert.Error(t, err)
	
	// Manually reset window (simulate time passing)
	manager.mu.Lock()
	manager.windowStart = time.Now().Add(-61 * time.Second) // Move window start to past
	manager.mu.Unlock()
	
	// Request should succeed after window reset
	can, err = manager.CanMakeRequest(1000)
	assert.True(t, can)
	assert.NoError(t, err)
}

func TestManager_WaitForCapacity(t *testing.T) {
	config := DefaultConfig()
	config.MaxRequestsPerMinute = 1
	manager := NewManager(config)
	
	// First request succeeds immediately
	ctx := context.Background()
	err := manager.WaitForCapacity(ctx, 1000)
	assert.NoError(t, err)
	manager.RecordRequest(1000)
	
	// Second request should wait (but we'll cancel it)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	err = manager.WaitForCapacity(ctx, 1000)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestManager_Reset(t *testing.T) {
	manager := NewManager(DefaultConfig())
	
	// Add some state
	manager.RecordRequest(1000)
	manager.RecordRequest(500)
	manager.RecordFailure()
	
	stats := manager.GetStatistics()
	assert.Greater(t, stats.TotalRequests, int64(0))
	assert.Greater(t, stats.TotalTokens, int64(0))
	
	// Reset
	manager.Reset()
	
	stats = manager.GetStatistics()
	assert.Equal(t, 0, stats.CurrentWindowRequests)
	assert.Equal(t, 0, stats.CurrentWindowTokens)
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.TotalTokens)
	assert.Equal(t, int64(0), stats.TotalFailures)
	assert.False(t, stats.CircuitOpen)
}

func TestManager_CalculateBackoff(t *testing.T) {
	config := DefaultConfig()
	config.RetryBackoffBase = 1 * time.Second
	manager := NewManager(config)
	
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},  // 2^0 = 1
		{1, 2 * time.Second},  // 2^1 = 2
		{2, 4 * time.Second},  // 2^2 = 4
		{3, 8 * time.Second},  // 2^3 = 8
		{10, 60 * time.Second}, // Capped at 1 minute
	}
	
	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			backoff := manager.CalculateBackoff(tt.attempt)
			assert.Equal(t, tt.expected, backoff)
		})
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	manager := NewManager(DefaultConfig())
	
	// Simulate concurrent requests
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				manager.CanMakeRequest(100)
				manager.RecordRequest(100)
				manager.GetStatistics()
			}
			done <- true
		}()
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	stats := manager.GetStatistics()
	assert.Equal(t, int64(1000), stats.TotalRequests)
}

func TestManager_GetStatistics(t *testing.T) {
	manager := NewManager(DefaultConfig())
	
	// Initial state
	stats := manager.GetStatistics()
	assert.Equal(t, 0, stats.CurrentWindowRequests)
	assert.Equal(t, 0, stats.CurrentWindowTokens)
	assert.False(t, stats.CircuitOpen)
	
	// After some requests
	manager.RecordRequest(1000)
	manager.RecordRequest(2000)
	
	stats = manager.GetStatistics()
	assert.Equal(t, 2, stats.CurrentWindowRequests)
	assert.Equal(t, 3000, stats.CurrentWindowTokens)
	assert.Equal(t, int64(2), stats.TotalRequests)
	assert.Equal(t, int64(3000), stats.TotalTokens)
	
	// After failures
	for i := 0; i < 5; i++ {
		manager.RecordFailure()
	}
	
	stats = manager.GetStatistics()
	assert.True(t, stats.CircuitOpen)
	assert.Equal(t, int64(5), stats.TotalFailures)
}

func TestManager_TokenLimitScenarios(t *testing.T) {
	tests := []struct {
		name        string
		maxTPM      int
		requests    []int
		shouldPass  []bool
	}{
		{
			name:   "within limits",
			maxTPM: 10000,
			requests: []int{2000, 2000, 2000},
			shouldPass: []bool{true, true, true},
		},
		{
			name:   "exceeds on third request",
			maxTPM: 5000,
			requests: []int{2000, 2000, 2000},
			shouldPass: []bool{true, true, false},
		},
		{
			name:   "single large request",
			maxTPM: 10000,
			requests: []int{15000},
			shouldPass: []bool{false},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MaxTokensPerMinute = tt.maxTPM
			config.MaxTokensPerRequest = 10000
			manager := NewManager(config)
			
			for i, tokens := range tt.requests {
				can, _ := manager.CanMakeRequest(tokens)
				assert.Equal(t, tt.shouldPass[i], can, 
					"Request %d with %d tokens", i, tokens)
				
				if can {
					manager.RecordRequest(tokens)
				}
			}
		})
	}
}

func TestManager_CircuitBreakerRecovery(t *testing.T) {
	manager := NewManager(DefaultConfig())
	
	// Trigger circuit breaker
	for i := 0; i < 5; i++ {
		manager.RecordFailure()
	}
	assert.True(t, manager.IsCircuitOpen())
	
	// Successful request should reset failure count
	manager.RecordRequest(1000)
	assert.False(t, manager.IsCircuitOpen())
	
	stats := manager.GetStatistics()
	require.NotNil(t, stats)
	assert.False(t, stats.CircuitOpen)
}
