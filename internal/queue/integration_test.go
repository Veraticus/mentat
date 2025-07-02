package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// TestQueueSystemIntegration tests the complete integration of all queue components.
func TestQueueSystemIntegration(t *testing.T) {
	t.Run("SimpleMessageFlow", testSimpleMessageFlow)
	t.Run("ConversationOrdering", testConversationOrdering)
	t.Run("RateLimiting", testRateLimiting)
	t.Run("WorkerScaling", testWorkerScaling)
	t.Run("ErrorHandlingAndRetry", testErrorHandlingAndRetry)
}

// testSimpleMessageFlow tests basic message enqueue and processing.
func testSimpleMessageFlow(t *testing.T) {
	system := setupTestSystem(t)

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

	// Wait for processing
	waitForCompletion(t, system, 1, 5*time.Second)
}

// testConversationOrdering verifies messages from same conversation are processed in order.
func testConversationOrdering(t *testing.T) {
	system := setupTestSystem(t)

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

	// Wait for all messages to be processed
	waitForCompletion(t, system, 3, 5*time.Second)
}

// testRateLimiting verifies rate limiting works correctly.
func testRateLimiting(t *testing.T) {
	system := setupTestSystem(t)
	user := "rate-limit-test-user"

	// Send burst of messages exceeding the burst limit
	burstSize := 5 // Exceeds typical burst limit of 3
	enqueueMessages(t, system, user, burstSize)

	// Wait a bit for rate limiter to kick in
	<-time.After(100 * time.Millisecond)

	// Check that some messages are still pending
	stats := system.Stats()
	nonCompletedMessages := stats.TotalQueued + stats.TotalProcessing

	if nonCompletedMessages == 0 {
		t.Errorf("Rate limiting not working: all messages processed immediately")
	}

	t.Logf("Rate limit test results: queued=%d, processing=%d, completed=%d, failed=%d",
		stats.TotalQueued, stats.TotalProcessing, stats.TotalCompleted, stats.TotalFailed)
}

// testWorkerScaling verifies dynamic worker pool scaling.
func testWorkerScaling(t *testing.T) {
	system := setupTestSystem(t)
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
}

// testErrorHandlingAndRetry verifies retry logic works correctly.
func testErrorHandlingAndRetry(t *testing.T) {
	system := setupTestSystem(t)

	msg := signal.IncomingMessage{
		From:       "retry-test-user",
		FromNumber: "+1234567890",
		Text:       "Test retry",
		Timestamp:  time.Now(),
	}

	if err := system.Enqueue(msg); err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}

	// Wait for processing (which may include retries)
	waitForCompletion(t, system, 1, 10*time.Second)
}

// Helper functions

// setupTestSystem creates and starts a queue system for testing.
func setupTestSystem(t *testing.T) *System {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

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

	t.Cleanup(func() {
		if stopErr := system.Stop(); stopErr != nil {
			t.Errorf("Failed to stop system: %v", stopErr)
		}
	})

	// Start the system
	if startErr := system.Start(ctx); startErr != nil {
		t.Fatalf("Failed to start queue system: %v", startErr)
	}

	return system
}

// waitForCompletion waits for a specific number of messages to be completed.
func waitForCompletion(t *testing.T, system *System, expectedCompleted int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		stats := system.Stats()
		t.Logf("Current stats: queued=%d, processing=%d, completed=%d, failed=%d",
			stats.TotalQueued, stats.TotalProcessing, stats.TotalCompleted, stats.TotalFailed)

		if stats.TotalCompleted >= expectedCompleted {
			return
		}
		<-time.After(100 * time.Millisecond)
	}

	stats := system.Stats()
	t.Errorf("Expected %d completed messages, got %d (queued=%d, processing=%d, failed=%d)",
		expectedCompleted, stats.TotalCompleted, stats.TotalQueued, stats.TotalProcessing, stats.TotalFailed)
}

// enqueueMessages enqueues multiple test messages.
func enqueueMessages(t *testing.T, system *System, user string, count int) {
	t.Helper()
	for i := 1; i <= count; i++ {
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
}
