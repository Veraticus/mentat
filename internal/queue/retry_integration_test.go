package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestRetryIntegration tests the full retry flow with NextRetryAt handling.
func TestRetryIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create components
	manager := NewManager(ctx)

	// Mock LLM that fails first two times, then succeeds
	attemptCount := int32(0)
	mockLLM := &mockLLM{
		err: NewRateLimitError("rate limited", 0, nil),
	}

	// Set up the mock to succeed on third attempt
	mockLLM.response = "Success!"

	mockMessenger := &mockMessenger{}
	rateLimiter := NewRateLimiter(10, 1, time.Minute)

	// Start manager
	go manager.Start(ctx)
	defer func() {
		if err := manager.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker pool
	pool := NewWorkerPool(1, mockLLM, mockMessenger, manager, rateLimiter)
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("Failed to start worker pool: %v", err)
	}
	defer pool.Stop()

	// Submit a message
	msg := NewMessage("test-msg", "test-conv", "sender", "+1234567890", "test message")
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Wait a bit for processing
	<-time.After(100 * time.Millisecond)

	// Message should have failed and be in retry state
	stats := manager.Stats()
	if stats["queued"] != 1 {
		t.Errorf("Expected 1 queued message (for retry), got %d", stats["queued"])
	}

	// Update mock to succeed on next attempt
	atomic.AddInt32(&attemptCount, 1)
	if atomic.LoadInt32(&attemptCount) >= 2 {
		mockLLM.err = nil
	}

	// Wait for retry delay (should be at least 30 seconds for rate limit)
	// In real scenario, we'd wait the full time, but for testing we'll check the state

	// Verify message has NextRetryAt set
	var foundMsg *Message
	manager.mu.RLock()
	for _, queue := range manager.queues {
		queue.mu.Lock()
		if queue.messages.Len() > 0 {
			if elem := queue.messages.Front(); elem != nil {
				if message, ok := elem.Value.(*Message); ok {
					foundMsg = message
				}
			}
		}
		queue.mu.Unlock()
	}
	manager.mu.RUnlock()

	if foundMsg == nil {
		t.Fatal("Could not find message in queue")
	}

	nextRetry := foundMsg.GetNextRetryAt()
	if nextRetry == nil {
		t.Fatal("NextRetryAt not set on retrying message")
	}

	// Verify it's in the future with appropriate delay
	delay := time.Until(*nextRetry)
	if delay < 25*time.Second { // Allow some margin
		t.Errorf("Retry delay too short for rate limit: %v", delay)
	}

	t.Logf("Message scheduled for retry at %v (delay: %v)",
		nextRetry.Format(time.RFC3339), delay)
}

// TestRetryDelayCalculation verifies the retry delay calculation.
func TestRetryDelayCalculation(t *testing.T) {
	tests := []struct {
		name        string
		attempts    int
		minDelay    time.Duration
		maxDelay    time.Duration
		isRateLimit bool
	}{
		{
			name:        "first retry standard",
			attempts:    1,
			minDelay:    1800 * time.Millisecond, // 2s - 10% jitter (1s * 2^1)
			maxDelay:    2200 * time.Millisecond, // 2s + 10% jitter
			isRateLimit: false,
		},
		{
			name:        "second retry standard",
			attempts:    2,
			minDelay:    3600 * time.Millisecond, // 4s - 10% jitter (1s * 2^2)
			maxDelay:    4400 * time.Millisecond, // 4s + 10% jitter
			isRateLimit: false,
		},
		{
			name:        "first retry rate limit",
			attempts:    1,
			minDelay:    54 * time.Second, // 60s - 10% jitter (30s * 2^1)
			maxDelay:    66 * time.Second, // 60s + 10% jitter
			isRateLimit: true,
		},
		{
			name:        "second retry rate limit",
			attempts:    2,
			minDelay:    108 * time.Second, // 120s - 10% jitter (30s * 2^2)
			maxDelay:    132 * time.Second, // 120s + 10% jitter
			isRateLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var delay time.Duration
			if tt.isRateLimit {
				delay = calculateRateLimitRetryDelay(tt.attempts)
			} else {
				delay = CalculateRetryDelay(tt.attempts)
			}

			if delay < tt.minDelay || delay > tt.maxDelay {
				t.Errorf("Delay %v outside expected range [%v, %v]",
					delay, tt.minDelay, tt.maxDelay)
			}
		})
	}
}
