package queue

import (
	"context"
	"testing"
	"time"
)

// TestMessageStuckInQueue reproduces the issue where messages stay in queued state
func TestMessageStuckInQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManager(ctx)
	go manager.Start()

	// Give manager time to start
	time.Sleep(100 * time.Millisecond)

	// Submit a message
	msg := NewMessage("test-1", "conv-1", "sender", "test message")
	t.Logf("Submitting message with state: %s", msg.GetState())
	
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Give the system time to process
	time.Sleep(200 * time.Millisecond)

	// Check stats to see if message is queued
	stats := manager.Stats()
	t.Logf("Stats after submit: %+v", stats)

	// Try to request the message as a worker would
	reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
	defer reqCancel()

	t.Log("Worker requesting message...")
	received, err := manager.RequestMessage(reqCtx)
	
	if err != nil {
		t.Errorf("Error requesting message: %v", err)
	}
	
	if received == nil {
		t.Error("Expected to receive a message, got nil")
		
		// Check stats again
		stats = manager.Stats()
		t.Logf("Stats after failed request: %+v", stats)
		
		// Let's inspect the internal state
		manager.mu.Lock()
		t.Logf("Number of conversations: %d", len(manager.queues))
		t.Logf("Number of waiting workers: %d", len(manager.waitingWorkers))
		for convID, queue := range manager.queues {
			t.Logf("Conversation %s: size=%d, processing=%v", 
				convID, queue.Size(), queue.IsProcessing())
		}
		manager.mu.Unlock()
	} else {
		t.Logf("Received message: ID=%s, State=%s", received.ID, received.GetState())
	}
}

// TestTryDispatchDeadlock tests if tryDispatch can cause deadlock
func TestTryDispatchDeadlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManager(ctx)
	
	// Start manager in background
	go manager.Start()
	time.Sleep(100 * time.Millisecond)

	// First, add a waiting worker
	workerCh := make(chan *Message, 1)
	
	go func() {
		select {
		case manager.requestCh <- workerCh:
			t.Log("Worker request sent")
		case <-time.After(1 * time.Second):
			t.Error("Timeout sending worker request")
		}
	}()
	
	// Give time for worker to be added to waiting list
	time.Sleep(200 * time.Millisecond)
	
	// Now submit a message which should trigger tryDispatch
	msg := NewMessage("test-2", "conv-2", "sender", "test")
	
	t.Log("Submitting message to trigger tryDispatch...")
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit: %v", err)
	}
	
	// Check if worker receives the message
	select {
	case received := <-workerCh:
		if received != nil {
			t.Logf("Worker received message: %s", received.ID)
		} else {
			t.Error("Worker received nil message")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for message dispatch to worker")
		
		// Debug info
		stats := manager.Stats()
		t.Logf("Final stats: %+v", stats)
	}
}