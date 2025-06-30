package queue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// TestQueueSystemIntegration tests the complete integration of all queue components.
func TestQueueSystemIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock dependencies
	mockLLM := &mockLLM{
		response: "Test response",
	}
	mockMessenger := &mockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	// Create system configuration
	config := SystemConfig{
		WorkerPoolSize:     2,
		MinWorkers:         1,
		MaxWorkers:         5,
		RateLimitPerMinute: 10,
		BurstLimit:         3,
		LLM:                mockLLM,
		Messenger:          mockMessenger,
	}

	// Create the integrated queue system
	system, err := NewSystem(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create queue system: %v", err)
	}
	defer func() {
		if err := system.Stop(); err != nil {
			t.Errorf("Failed to stop system: %v", err)
		}
	}()

	// Start the system
	if err := system.Start(); err != nil {
		t.Fatalf("Failed to start queue system: %v", err)
	}

	// Test 1: Simple message flow
	t.Run("SimpleMessageFlow", func(t *testing.T) {
		msg := signal.IncomingMessage{
			From:       "test-user",
			FromNumber: "+1234567890",
			Text:       "Hello",
			Timestamp:  time.Now(),
		}

		// Enqueue the message
		if err := system.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}

		// Log initial state
		t.Logf("Message enqueued, initial stats: %+v", system.Stats())

		// Wait for processing with timeout
		deadline := time.Now().Add(5 * time.Second) // Increased timeout
		for time.Now().Before(deadline) {
			stats := system.Stats()
			t.Logf("Current stats: queued=%d, processing=%d, completed=%d, failed=%d", 
				stats.TotalQueued, stats.TotalProcessing, stats.TotalCompleted, stats.TotalFailed)
			if stats.TotalCompleted >= 1 {
				// Success!
				return
			}
			time.Sleep(100 * time.Millisecond)
		}

		// If we get here, the message wasn't processed in time
		stats := system.Stats()
		t.Errorf("Expected 1 completed message, got %d (queued=%d, processing=%d, failed=%d)", 
			stats.TotalCompleted, stats.TotalQueued, stats.TotalProcessing, stats.TotalFailed)
	})

	// Test 2: Multiple messages from same conversation
	t.Run("ConversationOrdering", func(t *testing.T) {
		// Reset mock messenger to track order
		var receivedMessages []string
		var mu sync.Mutex
		mockMessenger.mu.Lock()
		mockMessenger.sendFunc = func(_ context.Context, _, message string) error {
			mu.Lock()
			receivedMessages = append(receivedMessages, message)
			mu.Unlock()
			return nil
		}
		mockMessenger.mu.Unlock()
		
		defer func() {
			mockMessenger.mu.Lock()
			mockMessenger.sendFunc = nil
			mockMessenger.mu.Unlock()
		}()

		// Reset LLM to return message-specific responses
		mockLLM.queryFunc = func(_ context.Context, message, _ string) (*claude.LLMResponse, error) {
			return &claude.LLMResponse{
				Message: fmt.Sprintf("Response to: %s", message),
			}, nil
		}

		// Send multiple messages from same user
		for i := 1; i <= 3; i++ {
			msg := signal.IncomingMessage{
				From:       "conversation-test-user",
				FromNumber: "+1234567890",
				Text:       fmt.Sprintf("Message %d", i),
				Timestamp:  time.Now(),
			}
			if err := system.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue message %d: %v", i, err)
			}
		}

		// Wait for all messages to be processed with timeout
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			messageCount := len(receivedMessages)
			mu.Unlock()
			if messageCount >= 3 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		// Verify messages were processed in order
		mu.Lock()
		defer mu.Unlock()
		if len(receivedMessages) != 3 {
			t.Fatalf("Expected 3 responses, got %d", len(receivedMessages))
		}
		for i := 1; i <= 3; i++ {
			expected := fmt.Sprintf("Response to: Message %d", i)
			if receivedMessages[i-1] != expected {
				t.Errorf("Message %d: expected %q, got %q", i, expected, receivedMessages[i-1])
			}
		}
	})

	// Test 3: Rate limiting
	t.Run("RateLimiting", func(t *testing.T) {
		// Send messages from a new user to avoid rate limit carryover
		user := "rate-limit-test-user"
		
		// Send burst of messages exceeding the burst limit
		burstSize := config.BurstLimit + 2 // Send more than burst allows
		for i := 1; i <= burstSize; i++ {
			msg := signal.IncomingMessage{
				From:       user,
				FromNumber: "+1234567890",
				Text:       fmt.Sprintf("Rate limit test %d", i),
				Timestamp:  time.Now(),
			}
			if err := system.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue message %d: %v", i, err)
			}
		}

		// Wait a bit for rate limiter to kick in
		time.Sleep(100 * time.Millisecond)

		// Check the final state
		stats := system.Stats()
		
		// We're interested in messages that are still queued or being retried
		// With a burst limit of 3, after processing 3 messages quickly, 
		// the remaining 2 should be rate limited
		nonCompletedMessages := stats.TotalQueued + stats.TotalProcessing
		
		// Some messages should still be pending due to rate limiting
		if nonCompletedMessages == 0 {
			t.Errorf("Rate limiting not working: all messages processed immediately, but burst limit is %d", 
				config.BurstLimit)
		}
		
		// Log the actual state for debugging
		t.Logf("Rate limit test results: queued=%d, processing=%d, completed=%d, failed=%d",
			stats.TotalQueued, stats.TotalProcessing, stats.TotalCompleted, stats.TotalFailed)
	})

	// Test 4: Worker scaling
	t.Run("WorkerScaling", func(t *testing.T) {
		initialWorkers := system.WorkerPool.Size()

		// Scale up
		if err := system.ScaleWorkers(2); err != nil {
			t.Fatalf("Failed to scale up workers: %v", err)
		}

		newSize := system.WorkerPool.Size()
		if newSize != initialWorkers+2 {
			t.Errorf("Expected %d workers after scaling up, got %d", initialWorkers+2, newSize)
		}

		// Scale down
		if err := system.ScaleWorkers(-1); err != nil {
			t.Fatalf("Failed to scale down workers: %v", err)
		}

		finalSize := system.WorkerPool.Size()
		if finalSize != newSize-1 {
			t.Errorf("Expected %d workers after scaling down, got %d", newSize-1, finalSize)
		}
	})

	// Test 5: Error handling and retry
	t.Run("ErrorHandlingAndRetry", func(t *testing.T) {
		// Configure LLM to fail first time, succeed second time
		attemptCount := 0
		mockLLM.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			attemptCount++
			if attemptCount == 1 {
				return nil, fmt.Errorf("temporary error")
			}
			return &claude.LLMResponse{
				Message: "Success after retry",
			}, nil
		}

		msg := signal.IncomingMessage{
			From:       "retry-test-user",
			FromNumber: "+1234567890",
			Text:       "Test retry",
			Timestamp:  time.Now(),
		}

		if err := system.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}

		// Wait for initial attempt and retry with timeout
		deadline := time.Now().Add(5 * time.Second) // Longer timeout for retry
		var finalStats Stats
		for time.Now().Before(deadline) {
			finalStats = system.Stats()
			// Check if message completed (either success or failure after max retries)
			if finalStats.TotalCompleted > 0 || finalStats.TotalFailed > 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Verify message was eventually processed successfully
		if finalStats.TotalFailed > 0 {
			t.Errorf("Expected no failed messages after retry, got %d", finalStats.TotalFailed)
		}
		if finalStats.TotalCompleted == 0 {
			t.Errorf("Expected completed message after retry, but got none")
		}
	})
}

// TestQueueSystemShutdown tests graceful shutdown of the queue system.
func TestQueueSystemShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock LLM that simulates long processing
	mockLLM := &mockLLM{
		queryFunc: func(ctx context.Context, _, _ string) (*claude.LLMResponse, error) {
			// Simulate long processing
			select {
			case <-time.After(500 * time.Millisecond):
				return &claude.LLMResponse{Message: "Processed"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	config := SystemConfig{
		WorkerPoolSize:  2,
		ShutdownTimeout: 2 * time.Second,
		LLM:             mockLLM,
		Messenger:       &mockMessenger{},
	}

	system, err := NewSystem(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create queue system: %v", err)
	}

	if err := system.Start(); err != nil {
		t.Fatalf("Failed to start queue system: %v", err)
	}

	// Enqueue some messages
	for i := 0; i < 5; i++ {
		msg := signal.IncomingMessage{
			From:       fmt.Sprintf("user-%d", i),
			FromNumber: fmt.Sprintf("+123456789%d", i),
			Text:       "Test message",
			Timestamp:  time.Now(),
		}
		if err := system.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}
	}

	// Give time for processing to start
	time.Sleep(100 * time.Millisecond)

	// Initiate shutdown
	shutdownStart := time.Now()
	if err := system.Stop(); err != nil {
		t.Fatalf("Failed to stop queue system: %v", err)
	}
	shutdownDuration := time.Since(shutdownStart)

	// Verify shutdown completed within timeout
	if shutdownDuration > 3*time.Second {
		t.Errorf("Shutdown took too long: %v", shutdownDuration)
	}

	// Verify we can get final stats
	stats := system.Stats()
	t.Logf("Final stats: %+v", stats)
}

// TestQueueSystemConfiguration tests configuration validation and defaults.
func TestQueueSystemConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		config    SystemConfig
		wantError bool
	}{
		{
			name: "minimal valid config",
			config: SystemConfig{
				LLM:       &mockLLM{},
				Messenger: &mockMessenger{},
			},
			wantError: false,
		},
		{
			name: "missing LLM",
			config: SystemConfig{
				Messenger: &mockMessenger{},
			},
			wantError: true,
		},
		{
			name: "missing Messenger",
			config: SystemConfig{
				LLM: &mockLLM{},
			},
			wantError: true,
		},
		{
			name: "invalid min/max workers",
			config: SystemConfig{
				LLM:        &mockLLM{},
				Messenger:  &mockMessenger{},
				MinWorkers: 10,
				MaxWorkers: 5,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			system, err := NewSystem(ctx, tt.config)
			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if system != nil {
					if err := system.Stop(); err != nil {
						t.Errorf("Failed to stop system: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkQueueSystemThroughput benchmarks the throughput of the queue system.
func BenchmarkQueueSystemThroughput(b *testing.B) {
	ctx := context.Background()

	// Create a fast mock LLM
	mockLLM := &mockLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			return &claude.LLMResponse{Message: "Response"}, nil
		},
	}

	config := SystemConfig{
		WorkerPoolSize:     5,
		RateLimitPerMinute: 1000, // High limit for benchmark
		LLM:                mockLLM,
		Messenger:          &mockMessenger{},
	}

	system, err := NewSystem(ctx, config)
	if err != nil {
		b.Fatalf("Failed to create queue system: %v", err)
	}
	defer func() {
		if err := system.Stop(); err != nil {
			b.Errorf("Failed to stop system: %v", err)
		}
	}()

	if err := system.Start(); err != nil {
		b.Fatalf("Failed to start queue system: %v", err)
	}

	b.ResetTimer()

	// Enqueue messages
	for i := 0; i < b.N; i++ {
		msg := signal.IncomingMessage{
			From:       fmt.Sprintf("user-%d", i%10), // Distribute across 10 users
			FromNumber: fmt.Sprintf("+123456789%d", i%10),
			Text:       "Benchmark message",
			Timestamp:  time.Now(),
		}
		if err := system.Enqueue(msg); err != nil {
			b.Fatalf("Failed to enqueue message: %v", err)
		}
	}

	// Wait for all messages to be processed
	for {
		stats := system.Stats()
		if stats.TotalCompleted >= b.N {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}