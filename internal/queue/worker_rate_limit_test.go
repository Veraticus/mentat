package queue

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// Test mocks for rate limit testing

type rateLimitTestLLM struct {
	queryFunc func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error)
}

func (m *rateLimitTestLLM) Query(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, prompt, sessionID)
	}
	return &claude.LLMResponse{Message: "mock response"}, nil
}

type rateLimitTestMessenger struct {
	sendFunc func(ctx context.Context, recipient, message string) error
}

func (m *rateLimitTestMessenger) Send(ctx context.Context, recipient, message string) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, recipient, message)
	}
	return nil
}

func (m *rateLimitTestMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

func (m *rateLimitTestMessenger) Listen(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	return ch, nil
}

func (m *rateLimitTestMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	return ch, nil
}

// TestRateLimitDetection tests the rate limit error detection without needing worker internals.
func TestRateLimitDetection(t *testing.T) {
	tests := []struct {
		name            string
		llmError        error
		maxAttempts     int
		currentAttempts int
		expectRetry     bool
		isRateLimit     bool
	}{
		{
			name:            "rate limit error with retries remaining",
			llmError:        errors.New("HTTP 429: Too Many Requests"),
			maxAttempts:     3,
			currentAttempts: 1,
			expectRetry:     true,
			isRateLimit:     true,
		},
		{
			name:            "rate limit error at max attempts",
			llmError:        errors.New("rate limit exceeded"),
			maxAttempts:     3,
			currentAttempts: 3,
			expectRetry:     false,
			isRateLimit:     true,
		},
		{
			name:            "RateLimitError type",
			llmError:        &RateLimitError{Message: "quota exceeded", RetryAfter: 30 * time.Second},
			maxAttempts:     3,
			currentAttempts: 1,
			expectRetry:     true,
			isRateLimit:     true,
		},
		{
			name:            "non-rate-limit error with retries",
			llmError:        errors.New("connection timeout"),
			maxAttempts:     3,
			currentAttempts: 1,
			expectRetry:     true,
			isRateLimit:     false,
		},
		{
			name:            "non-rate-limit error at max attempts",
			llmError:        errors.New("internal server error"),
			maxAttempts:     3,
			currentAttempts: 3,
			expectRetry:     false,
			isRateLimit:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test rate limit detection
			if IsRateLimitError(tt.llmError) != tt.isRateLimit {
				t.Errorf("IsRateLimitError(%v) = %v, want %v", tt.llmError, !tt.isRateLimit, tt.isRateLimit)
			}

			// Test retry logic
			msg := NewMessage("test-123", "conv-123", "user123", "+1234567890", "test message")
			msg.Attempts = tt.currentAttempts
			msg.MaxAttempts = tt.maxAttempts

			canRetry := msg.CanRetry()
			if canRetry != tt.expectRetry {
				t.Errorf("CanRetry() = %v, want %v", canRetry, tt.expectRetry)
			}
		})
	}
}

func TestCalculateRateLimitRetryDelay(t *testing.T) {
	tests := []struct {
		name        string
		attempts    int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{
			name:        "first attempt",
			attempts:    1,
			minExpected: 30 * time.Second,  // base delay
			maxExpected: 90 * time.Second,  // 2x base with jitter
		},
		{
			name:        "second attempt",
			attempts:    2,
			minExpected: 60 * time.Second,  // 2x base
			maxExpected: 150 * time.Second, // with jitter
		},
		{
			name:        "third attempt",
			attempts:    3,
			minExpected: 120 * time.Second, // 4x base
			maxExpected: 300 * time.Second, // with jitter
		},
		{
			name:        "max delay cap",
			attempts:    10,
			minExpected: 8 * time.Minute,  // Should be capped
			maxExpected: 10 * time.Minute, // at max delay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := calculateRateLimitRetryDelay(tt.attempts)
			
			if delay < tt.minExpected {
				t.Errorf("Delay %v is less than minimum expected %v", delay, tt.minExpected)
			}
			if delay > tt.maxExpected {
				t.Errorf("Delay %v is greater than maximum expected %v", delay, tt.maxExpected)
			}
		})
	}
}

// TestRateLimitIntegration tests the full rate limit handling flow.
func TestRateLimitIntegration(t *testing.T) {
	ctx := context.Background()
	
	// Track retry attempts using atomic operations for thread safety
	var retryCount int64
	
	// Create a mock LLM that fails with rate limit on first attempt
	mockLLM := &rateLimitTestLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			atomic.AddInt64(&retryCount, 1)
			return nil, fmt.Errorf("HTTP 429: Too Many Requests")
		},
	}
	
	// Create coordinator and worker pool
	coordinator := NewCoordinator(ctx)
	defer func() {
		if err := coordinator.Stop(); err != nil {
			t.Logf("Failed to stop coordinator: %v", err)
		}
	}()
	
	mockMessenger := &rateLimitTestMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	
	rateLimiter := NewRateLimiter(10, 1, time.Second)
	pool := NewWorkerPool(1, mockLLM, mockMessenger, coordinator.manager, rateLimiter)
	
	// Start worker pool
	poolCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	err := pool.Start(poolCtx)
	if err != nil {
		t.Fatalf("Failed to start worker pool: %v", err)
	}
	
	// Submit a message
	msg := signal.IncomingMessage{
		From:       "user123",
		FromNumber: "+1234567890",
		Text:       "test message",
		Timestamp:  time.Now(),
	}
	err = coordinator.Enqueue(msg)
	if err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}
	
	// Wait for initial processing attempt
	time.Sleep(200 * time.Millisecond)
	
	// Verify the message was rate limited and scheduled for retry
	count := atomic.LoadInt64(&retryCount)
	if count != 1 {
		t.Errorf("Expected 1 attempt, got %d", count)
	}
	
	// Check that message is queued for retry with appropriate delay
	stats := coordinator.Stats()
	if stats.TotalQueued != 1 {
		t.Errorf("Expected 1 queued message (for retry), got %d", stats.TotalQueued)
	}
	
	// Find the internal message and verify it has NextRetryAt set
	var foundMsg *Message
	coordinator.messages.Range(func(_, value any) bool {
		if msg, ok := value.(*Message); ok {
			foundMsg = msg
			return false // Stop iteration
		}
		return true
	})
	
	if foundMsg == nil {
		t.Fatal("Could not find message")
	}
	
	nextRetryAt := foundMsg.GetNextRetryAt()
	if nextRetryAt == nil {
		t.Fatal("NextRetryAt not set on retrying message")
	}
	
	// Verify retry delay is appropriate for rate limit (should be at least 30 seconds)
	delay := time.Until(*nextRetryAt)
	if delay < 25*time.Second { // Allow some margin
		t.Errorf("Retry delay too short for rate limit: %v", delay)
	}
	
	t.Logf("Message scheduled for retry at %v (delay: %v)", 
		nextRetryAt.Format(time.RFC3339), delay)
}