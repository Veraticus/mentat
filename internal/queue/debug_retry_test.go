package queue

import (
	"context"
	"testing"
	"time"
)

// TestMessageWithRetryDelay tests if messages with NextRetryAt set are properly handled
func TestMessageWithRetryDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManager(ctx)
	go manager.Start()
	time.Sleep(100 * time.Millisecond)

	// Create a message and set it to retry state with future retry time
	msg := NewMessage("retry-1", "conv-1", "sender", "test")
	
	// Manually set retry state with a future retry time
	msg.SetState(StateRetrying)
	futureTime := time.Now().Add(5 * time.Second)
	msg.SetNextRetryAt(futureTime)
	
	t.Logf("Submitting message in retry state with NextRetryAt: %v", futureTime)
	
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit: %v", err)
	}

	// Check stats
	stats := manager.Stats()
	t.Logf("Stats after submit: %+v", stats)

	// Try to request immediately (should not get the message)
	reqCtx, reqCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer reqCancel()

	received, err := manager.RequestMessage(reqCtx)
	
	if err == context.DeadlineExceeded {
		t.Log("Request timed out as expected (message not ready for retry)")
		
		// Check if message is still in queue
		stats = manager.Stats()
		t.Logf("Stats after timeout: %+v", stats)
		
		// Inspect queue state
		manager.mu.Lock()
		for convID, queue := range manager.queues {
			t.Logf("Queue %s: size=%d, processing=%v, hasReady=%v", 
				convID, queue.Size(), queue.IsProcessing(), queue.HasReadyMessages())
		}
		manager.mu.Unlock()
	} else if err != nil {
		t.Errorf("Unexpected error: %v", err)
	} else if received != nil {
		t.Errorf("Should not have received message yet, but got: %s", received.ID)
	}
}

// TestMessageStuckAfterProcessing simulates a message getting stuck after initial processing
func TestMessageStuckAfterProcessing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManager(ctx)
	go manager.Start()
	time.Sleep(100 * time.Millisecond)

	// Submit a message
	msg := NewMessage("stuck-1", "conv-1", "sender", "test")
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit: %v", err)
	}

	// Request it as a worker
	reqCtx1, cancel1 := context.WithTimeout(ctx, 1*time.Second)
	defer cancel1()
	
	received, err := manager.RequestMessage(reqCtx1)
	if err != nil || received == nil {
		t.Fatalf("Failed to get message: %v", err)
	}

	t.Logf("Got message, state: %s", received.GetState())

	// Simulate processing failure - transition to retry state
	if err := manager.stateMachine.Transition(received, StateRetrying); err != nil {
		t.Logf("Failed to transition to retry: %v", err)
	}

	// Try to retry the message
	if err := manager.RetryMessage(received); err != nil {
		t.Fatalf("Failed to retry message: %v", err)
	}

	// Check if the message is available again
	reqCtx2, cancel2 := context.WithTimeout(ctx, 1*time.Second)
	defer cancel2()
	
	received2, err2 := manager.RequestMessage(reqCtx2)
	
	if err2 == context.DeadlineExceeded {
		t.Error("Message stuck after retry - not available for processing")
		
		stats := manager.Stats()
		t.Logf("Final stats: %+v", stats)
		
		// Debug queue state
		manager.mu.Lock()
		for convID, queue := range manager.queues {
			t.Logf("Queue %s details:", convID)
			t.Logf("  Size: %d", queue.Size())
			t.Logf("  IsProcessing: %v", queue.IsProcessing())
			t.Logf("  HasReadyMessages: %v", queue.HasReadyMessages())
		}
		manager.mu.Unlock()
	} else if err2 != nil {
		t.Errorf("Unexpected error: %v", err2)
	} else if received2 != nil {
		t.Logf("Successfully got retried message: %s", received2.ID)
	}
}