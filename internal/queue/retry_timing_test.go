package queue

import (
	"context"
	"testing"
	"time"
)

// TestConversationQueueRespectsNextRetryAt verifies that the queue respects NextRetryAt field.
func TestConversationQueueRespectsNextRetryAt(t *testing.T) {
	queue := NewConversationQueue("test-conv")

	// Create a message that should be retried in the future
	msg := NewMessage("msg-1", "test-conv", "sender", "+1234567890", "test message")
	msg.SetState(StateRetrying)
	msg.IncrementAttempts()
	
	// Set retry time to 1 second in the future
	futureTime := time.Now().Add(1 * time.Second)
	msg.SetNextRetryAt(futureTime)

	// Enqueue the message
	if err := queue.Enqueue(msg); err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}

	// Try to dequeue immediately - should return nil
	dequeuedMsg := queue.Dequeue()
	if dequeuedMsg != nil {
		t.Errorf("Expected nil when message not ready for retry, got message: %s", dequeuedMsg.ID)
	}

	// Check HasReadyMessages
	if queue.HasReadyMessages() {
		t.Error("HasReadyMessages returned true when message is not ready")
	}

	// Wait for the retry time to pass
	time.Sleep(1100 * time.Millisecond)

	// Now it should be ready
	if !queue.HasReadyMessages() {
		t.Error("HasReadyMessages returned false when message should be ready")
	}

	// Try to dequeue again - should succeed
	dequeuedMsg = queue.Dequeue()
	if dequeuedMsg == nil {
		t.Error("Expected to dequeue message after retry time passed")
	}
	if dequeuedMsg != nil && dequeuedMsg.ID != msg.ID {
		t.Errorf("Expected message ID %s, got %s", msg.ID, dequeuedMsg.ID)
	}
}

// TestConversationQueueMixedRetryMessages tests queue with mix of ready and not-ready messages.
func TestConversationQueueMixedRetryMessages(t *testing.T) {
	queue := NewConversationQueue("test-conv")

	// Create messages with different retry times
	msg1 := NewMessage("msg-1", "test-conv", "sender", "+1234567890", "message 1")
	msg1.SetState(StateRetrying)
	msg1.SetNextRetryAt(time.Now().Add(2 * time.Second)) // Not ready

	msg2 := NewMessage("msg-2", "test-conv", "sender", "+1234567890", "message 2")
	msg2.SetState(StateQueued) // Ready (not in retry state)

	msg3 := NewMessage("msg-3", "test-conv", "sender", "+1234567890", "message 3")
	msg3.SetState(StateRetrying)
	msg3.SetNextRetryAt(time.Now().Add(-1 * time.Second)) // Ready (past time)

	// Enqueue all messages
	for _, msg := range []*Message{msg1, msg2, msg3} {
		if err := queue.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message %s: %v", msg.ID, err)
		}
	}

	// Should dequeue msg2 first (not in retry state)
	dequeued := queue.Dequeue()
	if dequeued == nil || dequeued.ID != "msg-2" {
		t.Errorf("Expected to dequeue msg-2, got %v", dequeued)
	}

	// Complete processing
	queue.Complete()

	// Should dequeue msg3 next (retry time has passed)
	dequeued = queue.Dequeue()
	if dequeued == nil || dequeued.ID != "msg-3" {
		t.Errorf("Expected to dequeue msg-3, got %v", dequeued)
	}

	// Complete processing
	queue.Complete()

	// msg1 should not be available yet
	dequeued = queue.Dequeue()
	if dequeued != nil {
		t.Errorf("Expected nil (msg-1 not ready), got %v", dequeued)
	}
}

// TestWorkerSetsNextRetryAt verifies that the worker sets NextRetryAt on errors.
func TestWorkerSetsNextRetryAt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test infrastructure
	manager := NewManager(ctx)
	mockLLM := &mockLLM{err: NewRateLimitError("rate limited", 0, nil)}
	mockMessenger := &mockMessenger{}
	rateLimiter := NewRateLimiter(10, 1, time.Minute)

	// Start queue manager
	go manager.Start()
	defer func() {
		if err := manager.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker config
	config := WorkerConfig{
		ID:           1,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: manager,
		RateLimiter:  rateLimiter,
	}

	w, ok := NewWorker(config).(*worker) // Type assert to access Process method
	if !ok {
		t.Fatal("Failed to type assert worker")
	}
	worker := w

	// Create a test message
	msg := NewMessage("test-msg", "test-conv", "sender", "+1234567890", "test message")

	// Process the message (will fail with rate limit error)
	err := worker.Process(ctx, msg)
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