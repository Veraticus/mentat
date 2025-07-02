package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

// timingMockLLM implements the LLM interface for timing tests.
type timingMockLLM struct {
	queryFunc func(ctx context.Context, message, sessionID string) (*claude.LLMResponse, error)
	err       error
}

func (m *timingMockLLM) Query(ctx context.Context, message, sessionID string) (*claude.LLMResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, message, sessionID)
	}
	if m.err != nil {
		return nil, m.err
	}
	return &claude.LLMResponse{Message: "Mock response"}, nil
}

// timingMockMessenger implements the Messenger interface for timing tests.
type timingMockMessenger struct {
	sendFunc func(ctx context.Context, to, message string) error
}

func (m *timingMockMessenger) Send(ctx context.Context, to, message string) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, to, message)
	}
	return nil
}

func (m *timingMockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	// Mock implementation - just return nil
	return nil
}

func (m *timingMockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	// Mock implementation - return closed channel
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}

// TestConversationQueueRespectsNextRetryAt verifies that the queue respects NextRetryAt field.
func TestConversationQueueRespectsNextRetryAt(t *testing.T) {
	convQueue := queue.NewConversationQueue("test-conv")

	// Create a message that should be retried in the future
	msg := queue.NewMessage("msg-1", "test-conv", "sender", "+1234567890", "test message")
	msg.SetState(queue.StateRetrying)
	msg.IncrementAttempts()

	// Set retry time to 1 second in the future
	futureTime := time.Now().Add(1 * time.Second)
	msg.SetNextRetryAt(futureTime)

	// Enqueue the message
	if err := convQueue.Enqueue(msg); err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}

	// Try to dequeue immediately - should return nil
	dequeuedMsg := convQueue.Dequeue()
	if dequeuedMsg != nil {
		t.Errorf("Expected nil when message not ready for retry, got message: %s", dequeuedMsg.ID)
	}

	// Check HasReadyMessages
	if convQueue.HasReadyMessages() {
		t.Error("HasReadyMessages returned true when message is not ready")
	}

	// Wait for the retry time to pass
	<-time.After(1100 * time.Millisecond)

	// Now it should be ready
	if !convQueue.HasReadyMessages() {
		t.Error("HasReadyMessages returned false when message should be ready")
	}

	// Try to dequeue again - should succeed
	dequeuedMsg = convQueue.Dequeue()
	if dequeuedMsg == nil {
		t.Error("Expected to dequeue message after retry time passed")
	}
	if dequeuedMsg != nil && dequeuedMsg.ID != msg.ID {
		t.Errorf("Expected message ID %s, got %s", msg.ID, dequeuedMsg.ID)
	}
}

// TestConversationQueueMixedRetryMessages tests queue with mix of ready and not-ready messages.
func TestConversationQueueMixedRetryMessages(t *testing.T) {
	convQueue := queue.NewConversationQueue("test-conv")

	// Create messages with different retry times
	msg1 := queue.NewMessage("msg-1", "test-conv", "sender", "+1234567890", "message 1")
	msg1.SetState(queue.StateRetrying)
	msg1.SetNextRetryAt(time.Now().Add(2 * time.Second)) // Not ready

	msg2 := queue.NewMessage("msg-2", "test-conv", "sender", "+1234567890", "message 2")
	msg2.SetState(queue.StateQueued) // Ready (not in retry state)

	msg3 := queue.NewMessage("msg-3", "test-conv", "sender", "+1234567890", "message 3")
	msg3.SetState(queue.StateRetrying)
	msg3.SetNextRetryAt(time.Now().Add(-1 * time.Second)) // Ready (past time)

	// Enqueue all messages
	for _, msg := range []*queue.Message{msg1, msg2, msg3} {
		if err := convQueue.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message %s: %v", msg.ID, err)
		}
	}

	// Should dequeue msg2 first (not in retry state)
	dequeued := convQueue.Dequeue()
	if dequeued == nil || dequeued.ID != "msg-2" {
		t.Errorf("Expected to dequeue msg-2, got %v", dequeued)
	}

	// Complete processing
	convQueue.Complete()

	// Should dequeue msg3 next (retry time has passed)
	dequeued = convQueue.Dequeue()
	if dequeued == nil || dequeued.ID != "msg-3" {
		t.Errorf("Expected to dequeue msg-3, got %v", dequeued)
	}

	// Complete processing
	convQueue.Complete()

	// msg1 should not be available yet
	dequeued = convQueue.Dequeue()
	if dequeued != nil {
		t.Errorf("Expected nil (msg-1 not ready), got %v", dequeued)
	}
}

// TestWorkerSetsNextRetryAt verifies that the worker sets NextRetryAt on errors.
func TestWorkerSetsNextRetryAt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test infrastructure
	manager := queue.NewManager(ctx)
	mockLLM := &timingMockLLM{err: queue.NewRateLimitError("rate limited", 0, nil)}
	mockMessenger := &timingMockMessenger{}
	rateLimiter := queue.NewRateLimiter(10, 1, time.Minute)

	// Start queue manager
	go manager.Start(ctx)
	defer func() {
		if err := manager.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker config
	config := queue.WorkerConfig{
		ID:           1,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: manager,
		RateLimiter:  rateLimiter,
	}

	worker := queue.NewWorker(config)

	// Create a test message
	msg := queue.NewMessage("test-msg", "test-conv", "sender", "+1234567890", "test message")

	// Process the message (will fail with rate limit error)
	err := queue.ProcessTestMessage(ctx, worker, msg)
	if err == nil {
		t.Error("Expected error from processing")
	}

	// Check that NextRetryAt was set
	nextRetry := msg.GetNextRetryAt()
	if nextRetry == nil {
		t.Fatal("NextRetryAt was not set after rate limit error")
	}

	// Verify it's in the future
	if !nextRetry.After(time.Now()) {
		t.Error("NextRetryAt should be in the future")
	}

	// Verify retry delay is appropriate for rate limit (should be >= 30 seconds)
	delay := time.Until(*nextRetry)
	if delay < 25*time.Second { // Allow some margin for test execution
		t.Errorf("Rate limit retry delay too short: %v", delay)
	}
}
