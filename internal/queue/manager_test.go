package queue_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
)

func TestManager_SubmitAndRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Wait for manager to be fully started
	manager.WaitForReady()

	// Submit messages
	msg1 := queue.NewMessage("msg-1", "conv-1", "sender1", "+1234567890", "hello")
	msg2 := queue.NewMessage("msg-2", "conv-2", "sender2", "+0987654321", "world")

	if err := manager.Submit(msg1); err != nil {
		t.Fatalf("Failed to submit msg1: %v", err)
	}

	if err := manager.Submit(msg2); err != nil {
		t.Fatalf("Failed to submit msg2: %v", err)
	}

	// Request messages
	reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
	defer reqCancel()

	received1, err := manager.RequestMessage(reqCtx)
	if err != nil {
		t.Fatalf("Failed to request message: %v", err)
	}

	if received1 == nil {
		t.Fatal("Expected to receive a message")
	}

	// Should be in processing state
	if received1.GetState() != queue.StateProcessing {
		t.Errorf("Expected state %s, got %s", queue.StateProcessing, received1.GetState())
	}

	// Complete and request next
	if completeErr := manager.CompleteMessage(received1); completeErr != nil {
		t.Fatalf("Failed to complete message: %v", completeErr)
	}

	received2, err := manager.RequestMessage(reqCtx)
	if err != nil {
		t.Fatalf("Failed to request second message: %v", err)
	}

	if received2 == nil {
		t.Fatal("Expected to receive second message")
	}

	// Should get messages from different conversations (fair scheduling)
	if received1.ConversationID == received2.ConversationID {
		t.Error("Expected messages from different conversations for fairness")
	}
}

func TestManager_FairScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Wait for manager to be fully started
	manager.WaitForReady()

	// Submit multiple messages per conversation
	conversations := []string{"conv-1", "conv-2", "conv-3"}
	messagesPerConv := 3

	// Submit messages one by one with small delays to ensure consistent ordering
	for i := range messagesPerConv {
		for _, convID := range conversations {
			msg := queue.NewMessage(
				fmt.Sprintf("%s-msg-%d", convID, i),
				convID,
				"sender",
				"+1234567890",
				"test",
			)
			if err := manager.Submit(msg); err != nil {
				t.Fatalf("Failed to submit message: %v", err)
			}
		}
	}

	// Give messages time to be queued through channel
	// This is necessary because Submit is async via channels
	<-time.After(20 * time.Millisecond)

	// Request messages and track which conversations we get
	convCounts := make(map[string]int)
	seenConversations := make(map[string]bool)
	reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
	defer reqCancel()

	// First round - should get one from each conversation
	for i := range conversations {
		msg, err := manager.RequestMessage(reqCtx)
		if err != nil || msg == nil {
			t.Fatalf("Failed to get message %d: %v", i, err)
		}

		// Check that we haven't seen this conversation yet in this round
		if seenConversations[msg.ConversationID] {
			t.Errorf("Conversation %s scheduled again before all conversations had a turn", msg.ConversationID)
		}
		seenConversations[msg.ConversationID] = true
		convCounts[msg.ConversationID]++

		if completeErr := manager.CompleteMessage(msg); completeErr != nil {
			t.Errorf("Failed to complete message: %v", completeErr)
		}
	}

	// Check fairness - each conversation should have been scheduled once
	for _, convID := range conversations {
		if convCounts[convID] != 1 {
			t.Errorf("Conversation %s scheduled %d times, expected 1",
				convID, convCounts[convID])
		}
	}
}

func TestManager_Shutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Submit a message
	msg := queue.NewMessage("msg-1", "conv-1", "sender", "+1234567890", "test")
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Shutdown with timeout
	err := manager.Shutdown(2 * time.Second)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Should not be able to submit after shutdown
	err = manager.Submit(queue.NewMessage("msg-2", "conv-1", "sender", "+1234567890", "test"))
	if err == nil {
		t.Error("Expected error submitting after shutdown")
	}
}

// waitForManagerReady polls until the manager is ready or timeout.
func waitForManagerReady(ctx context.Context, manager *queue.Manager, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stats := manager.Stats()
		if stats != nil {
			return nil
		}
		select {
		case <-time.After(time.Millisecond):
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		}
	}
	return fmt.Errorf("timeout waiting for manager ready")
}

// waitForStats polls until the stats match expected values or timeout.
func waitForStats(
	ctx context.Context,
	manager *queue.Manager,
	expectedConversations, expectedQueued int,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stats := manager.Stats()
		if stats["conversations"] == expectedConversations && stats["queued"] == expectedQueued {
			return nil
		}
		select {
		case <-time.After(time.Millisecond):
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		}
	}
	return fmt.Errorf("timeout waiting for stats")
}

func TestManager_Stats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Wait for manager to be ready
	if err := waitForManagerReady(ctx, manager, 100*time.Millisecond); err != nil {
		t.Fatalf("Manager not ready: %v", err)
	}

	// Initial stats
	stats := manager.Stats()
	if stats["conversations"] != 0 || stats["queued"] != 0 || stats["processing"] != 0 {
		t.Error("Expected empty initial stats")
	}

	// Submit messages
	if err := manager.Submit(queue.NewMessage("msg-1", "conv-1", "sender", "+1234567890", "test")); err != nil {
		t.Fatalf("Failed to submit msg-1: %v", err)
	}
	if err := manager.Submit(queue.NewMessage("msg-2", "conv-1", "sender", "+1234567890", "test")); err != nil {
		t.Fatalf("Failed to submit msg-2: %v", err)
	}
	if err := manager.Submit(queue.NewMessage("msg-3", "conv-2", "sender", "+0987654321", "test")); err != nil {
		t.Errorf("Failed to submit message: %v", err)
	}

	// Wait for expected stats
	if err := waitForStats(ctx, manager, 2, 3, 100*time.Millisecond); err != nil {
		t.Errorf("Stats not as expected: %v", err)
	}

	stats = manager.Stats()
	if stats["conversations"] != 2 {
		t.Errorf("Expected 2 conversations, got %d", stats["conversations"])
	}
	if stats["queued"] != 3 {
		t.Errorf("Expected 3 queued messages, got %d", stats["queued"])
	}

	// Request a message
	reqCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	msg, _ := manager.RequestMessage(reqCtx)

	if msg != nil {
		stats = manager.Stats()
		if stats["processing"] != 1 {
			t.Errorf("Expected 1 processing, got %d", stats["processing"])
		}
		if stats["queued"] != 2 {
			t.Errorf("Expected 2 queued after processing one, got %d", stats["queued"])
		}
	}
}

func TestManager_ConcurrentSubmit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Wait for manager to be ready
	if err := waitForManagerReady(ctx, manager, 100*time.Millisecond); err != nil {
		t.Fatalf("Manager not ready: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	messagesPerGoroutine := 5

	// Concurrent submits
	for i := range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := range messagesPerGoroutine {
				msg := queue.NewMessage(
					fmt.Sprintf("g%d-m%d", goroutineID, j),
					fmt.Sprintf("conv-%d", goroutineID%3), // 3 conversations
					"sender",
					"+1234567890",
					"test",
				)
				if err := manager.Submit(msg); err != nil {
					t.Errorf("Submit failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Wait for all messages to be enqueued
	expectedMessages := numGoroutines * messagesPerGoroutine
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		stats := manager.Stats()
		totalMessages := stats["queued"] + stats["processing"]
		if totalMessages == expectedMessages {
			break
		}
		select {
		case <-time.After(time.Millisecond):
		case <-ctx.Done():
			t.Fatal("Context canceled while waiting for messages")
		}
	}

	// Check total messages (some may be processing)
	stats := manager.Stats()
	totalMessages := stats["queued"] + stats["processing"]
	if totalMessages != expectedMessages {
		t.Errorf("Expected %d total messages (queued+processing), got %d (queued=%d, processing=%d)",
			expectedMessages, totalMessages, stats["queued"], stats["processing"])
	}
}

func TestManager_CompleteNonExistentMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Try to complete a message that was never submitted
	msg := queue.NewMessage("fake", "fake-conv", "sender", "+1234567890", "test")
	err := manager.CompleteMessage(msg)
	// CompleteMessage returns nil for non-existent queues (treats it as already cleaned up)
	if err != nil {
		t.Errorf("Expected nil when completing non-existent message, got: %v", err)
	}
}

func TestManager_RequestTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Manager starts immediately

	// Request with short timeout when no messages available
	reqCtx, reqCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer reqCancel()

	start := time.Now()
	msg, err := manager.RequestMessage(reqCtx)
	duration := time.Since(start)

	if msg != nil {
		t.Error("Expected nil message on timeout")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected DeadlineExceeded error, got %v", err)
	}

	// Should timeout after ~100ms
	if duration < 90*time.Millisecond || duration > 200*time.Millisecond {
		t.Errorf("Unexpected timeout duration: %v", duration)
	}
}
