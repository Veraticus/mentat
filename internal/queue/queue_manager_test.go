package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// Helper function to create test IncomingMessage.
func createIncomingMessage(from, text string) signal.IncomingMessage {
	return signal.IncomingMessage{
		Timestamp: time.Now(),
		From:      from,
		Text:      text,
	}
}

// Helper function to get expected message ID.
func getExpectedMessageID(msg signal.IncomingMessage) string {
	return fmt.Sprintf("%d-%s", msg.Timestamp.UnixNano(), msg.From)
}

func TestQueueManager_ImplementsInterface(_ *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// This test ensures QueueManager implements MessageQueue interface
	var _ MessageQueue = qm
}

func TestQueueManager_EnqueueAndGetNext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Test enqueue
	msg := createIncomingMessage("sender1", "Hello world")
	expectedID := getExpectedMessageID(msg)

	if err := qm.Enqueue(msg); err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}

	// Test get next
	queuedMsg, err := qm.GetNext("worker-1")
	if err != nil {
		t.Fatalf("Failed to get next message: %v", err)
	}

	if queuedMsg == nil {
		t.Fatal("Expected a message but got nil")
	}

	if queuedMsg.ID != expectedID {
		t.Errorf("Expected message ID %s, got %s", expectedID, queuedMsg.ID)
	}

	if queuedMsg.State != MessageStateProcessing {
		t.Errorf("Expected state %d (processing), got %d", MessageStateProcessing, queuedMsg.State)
	}
}

func TestQueueManager_UpdateState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Enqueue a message
	msg := createIncomingMessage("sender1", "Test message")
	expectedID := getExpectedMessageID(msg)

	if err := qm.Enqueue(msg); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Get the message to put it in processing state
	queuedMsg, err := qm.GetNext("worker-1")
	if err != nil || queuedMsg == nil {
		t.Fatalf("Failed to get message: %v", err)
	}

	// Update state to completed
	if err := qm.UpdateState(expectedID, MessageStateCompleted, "Processing completed"); err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}

	// Verify stats reflect the state change
	stats := qm.Stats()
	if stats.TotalCompleted != 1 {
		t.Errorf("Expected 1 completed message, got %d", stats.TotalCompleted)
	}
}

func TestQueueManager_Stats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Initial stats should be zero
	stats := qm.Stats()
	if stats.TotalQueued != 0 || stats.TotalProcessing != 0 || stats.TotalCompleted != 0 {
		t.Error("Expected initial stats to be zero")
	}

	// Add messages from different conversations
	for i := 0; i < 5; i++ {
		msg := createIncomingMessage(fmt.Sprintf("sender-%d", i%3), "Test") // 3 conversations
		if err := qm.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Check stats
	stats = qm.Stats()
	if stats.TotalQueued != 5 {
		t.Errorf("Expected 5 queued messages, got %d", stats.TotalQueued)
	}
	if stats.ConversationCount != 3 {
		t.Errorf("Expected 3 conversations, got %d", stats.ConversationCount)
	}

	// Get one message to process
	if _, err := qm.GetNext("worker-1"); err != nil {
		t.Fatalf("Failed to get next: %v", err)
	}

	stats = qm.Stats()
	if stats.TotalQueued != 4 {
		t.Errorf("Expected 4 queued after processing one, got %d", stats.TotalQueued)
	}
	if stats.TotalProcessing != 1 {
		t.Errorf("Expected 1 processing, got %d", stats.TotalProcessing)
	}
}

func TestQueueManager_FairScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	conversations := []string{"sender-1", "sender-2", "sender-3"}
	messagesPerConv := 3

	// Enqueue messages
	for _, sender := range conversations {
		for i := 0; i < messagesPerConv; i++ {
			msg := createIncomingMessage(sender, fmt.Sprintf("Test message %d", i))
			if err := qm.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue: %v", err)
			}
		}
	}

	// Track which conversations we get messages from
	convCounts := make(map[string]int)
	messages := make([]*QueuedMessage, 0, len(conversations))

	// First round - should get one from each conversation
	// Get all messages first before completing any
	for i := 0; i < len(conversations); i++ {
		msg, err := qm.GetNext(fmt.Sprintf("worker-%d", i))
		if err != nil || msg == nil {
			t.Fatalf("Failed to get message %d: %v", i, err)
		}
		convCounts[msg.ConversationID]++
		messages = append(messages, msg)
	}
	
	// Now complete all messages
	for _, msg := range messages {
		if err := qm.UpdateState(msg.ID, MessageStateCompleted, "done"); err != nil {
			t.Errorf("Failed to complete message: %v", err)
		}
	}

	// Check fairness - each conversation should have been scheduled once
	for _, sender := range conversations {
		// ConversationID is the sender itself in the test helper
		if convCounts[sender] != 1 {
			t.Errorf("Conversation %s scheduled %d times, expected 1 (counts: %v)", sender, convCounts[sender], convCounts)
		}
	}
}

func TestQueueManager_100ConcurrentConversations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	numConversations := 100
	messagesPerConv := 10
	var enqueuedCount int32
	var processedCount int32
	var wg sync.WaitGroup

	// Start workers first
	numWorkers := 20
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					msg, err := qm.GetNext(workerID)
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						// Timeout is expected when queue is empty
						time.Sleep(10 * time.Millisecond)
						continue
					}
					if msg == nil {
						// No messages available
						time.Sleep(10 * time.Millisecond)
						continue
					}

					// Simulate processing
					time.Sleep(time.Millisecond)

					// Mark as completed
					if err := qm.UpdateState(msg.ID, MessageStateCompleted, "processed"); err != nil {
						t.Errorf("Failed to update state: %v", err)
					}
					atomic.AddInt32(&processedCount, 1)
				}
			}
		}(fmt.Sprintf("worker-%d", w))
	}

	// Concurrently enqueue messages from different conversations
	for c := 0; c < numConversations; c++ {
		wg.Add(1)
		go func(convNum int) {
			defer wg.Done()
			for m := 0; m < messagesPerConv; m++ {
				msg := createIncomingMessage(
					fmt.Sprintf("sender-%d", convNum),
					fmt.Sprintf("Message %d from conversation %d", m, convNum),
				)
				if err := qm.Enqueue(msg); err != nil {
					t.Errorf("Failed to enqueue message: %v", err)
					return
				}
				atomic.AddInt32(&enqueuedCount, 1)
			}
		}(c)
	}

	// Wait for all messages to be enqueued
	time.Sleep(500 * time.Millisecond)

	// Check enqueue count
	expectedTotal := numConversations * messagesPerConv
	if int(atomic.LoadInt32(&enqueuedCount)) != expectedTotal {
		t.Errorf("Expected %d messages enqueued, got %d", expectedTotal, enqueuedCount)
	}

	// Wait for processing to complete (with timeout)
	done := make(chan bool)
	go func() {
		for int(atomic.LoadInt32(&processedCount)) < expectedTotal {
			time.Sleep(100 * time.Millisecond)
		}
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatalf("Timeout: only processed %d/%d messages", atomic.LoadInt32(&processedCount), expectedTotal)
	}

	// Cancel context to stop workers
	cancel()
	wg.Wait()

	// Verify final stats
	finalStats := qm.Stats()
	if finalStats.TotalCompleted != expectedTotal {
		t.Errorf("Expected %d completed messages, got %d", expectedTotal, finalStats.TotalCompleted)
	}

	t.Logf("Successfully processed %d messages from %d concurrent conversations", expectedTotal, numConversations)
}

func TestQueueManager_UpdateStateInvalidMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Try to update state of non-existent message
	err := qm.UpdateState("non-existent-id", MessageStateCompleted, "test")
	if err == nil {
		t.Error("Expected error when updating non-existent message")
	}
}

func TestQueueManager_GetNextTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Try to get message when queue is empty
	start := time.Now()
	msg, err := qm.GetNext("worker-1")
	duration := time.Since(start)

	if msg != nil {
		t.Error("Expected nil message when queue is empty")
	}

	if err == nil {
		t.Error("Expected error for empty queue, got nil")
	}

	// Should return quickly (not block indefinitely)
	if duration > 100*time.Millisecond {
		t.Errorf("GetNext took too long on empty queue: %v", duration)
	}
}

func TestQueueManager_ConcurrentEnqueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	numGoroutines := 50
	messagesPerGoroutine := 20
	var wg sync.WaitGroup
	var successCount int32

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			for m := 0; m < messagesPerGoroutine; m++ {
				msg := createIncomingMessage(
					fmt.Sprintf("sender-%d", gID%10), // 10 conversations
					"Test",
				)
				if err := qm.Enqueue(msg); err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}
		}(g)
	}

	wg.Wait()

	expectedTotal := numGoroutines * messagesPerGoroutine
	if int(successCount) != expectedTotal {
		t.Errorf("Expected %d successful enqueues, got %d", expectedTotal, successCount)
	}

	stats := qm.Stats()
	if stats.TotalQueued != expectedTotal {
		t.Errorf("Expected %d queued messages, got %d", expectedTotal, stats.TotalQueued)
	}
}

func TestQueueManager_StopGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewCoordinator(ctx)

	// Enqueue some messages
	for i := 0; i < 5; i++ {
		msg := createIncomingMessage("sender", fmt.Sprintf("Test %d", i))
		if err := qm.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Stop should complete quickly
	done := make(chan bool)
	go func() {
		_ = qm.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop took too long")
	}

	// Should not be able to enqueue after stop
	err := qm.Enqueue(createIncomingMessage("sender", "Test after stop"))
	if err == nil {
		t.Error("Expected error when enqueuing after stop")
	}
}