package queue

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestComprehensiveMessageStuckScenarios tests all scenarios where messages can get stuck
func TestComprehensiveMessageStuckScenarios(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Message)
		expected string
	}{
		{
			name: "message_with_future_retry",
			setup: func(msg *Message) {
				msg.SetState(StateRetrying)
				msg.SetNextRetryAt(time.Now().Add(10 * time.Second))
			},
			expected: "stuck_not_ready",
		},
		{
			name: "message_transitioning_from_retry_to_queued",
			setup: func(msg *Message) {
				msg.SetState(StateRetrying)
				msg.SetNextRetryAt(time.Now().Add(10 * time.Second))
				// This simulates what happens in Submit when StateRetrying is detected
			},
			expected: "stuck_after_requeue",
		},
		{
			name: "normal_message",
			setup: func(msg *Message) {
				// Normal message, should work fine
			},
			expected: "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			manager := NewManager(ctx)
			go manager.Start()
			time.Sleep(100 * time.Millisecond)

			// Create and setup message
			msg := NewMessage(fmt.Sprintf("msg-%s", tt.name), "conv-1", "sender", "test")
			tt.setup(msg)

			t.Logf("Submitting message with state: %s, NextRetryAt: %v", 
				msg.GetState(), msg.GetNextRetryAt())

			if err := manager.Submit(msg); err != nil {
				t.Fatalf("Failed to submit: %v", err)
			}

			// Try to request the message
			reqCtx, reqCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer reqCancel()

			received, err := manager.RequestMessage(reqCtx)

			// Analyze result
			if err == context.DeadlineExceeded {
				// Message is stuck
				stats := manager.Stats()
				t.Logf("Message stuck - Stats: %+v", stats)
				
				// Debug queue state
				manager.mu.Lock()
				for convID, queue := range manager.queues {
					t.Logf("Queue %s: size=%d, processing=%v, hasReady=%v", 
						convID, queue.Size(), queue.IsProcessing(), queue.HasReadyMessages())
					
					// Check the actual message in the queue
					if queue.Size() > 0 {
						// We need to peek at the message to see its state
						queue.mu.Lock()
						if queue.messages.Front() != nil {
							if m, ok := queue.messages.Front().Value.(*Message); ok {
								t.Logf("  First message: ID=%s, State=%s, NextRetryAt=%v, IsReady=%v",
									m.ID, m.GetState(), m.GetNextRetryAt(), m.IsReadyForRetry())
							}
						}
						queue.mu.Unlock()
					}
				}
				manager.mu.Unlock()

				if tt.expected != "stuck_not_ready" && tt.expected != "stuck_after_requeue" {
					t.Errorf("Expected success but message got stuck")
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			} else if received != nil {
				t.Logf("Successfully received message: %s", received.ID)
				if tt.expected != "success" {
					t.Errorf("Expected message to be stuck but it was delivered")
				}
			}
		})
	}
}

// TestRetryTimeClearance tests if NextRetryAt is cleared when transitioning to queued
func TestRetryTimeClearance(t *testing.T) {
	// Create a message in retry state with future retry time
	msg := NewMessage("test-1", "conv-1", "sender", "test")
	msg.SetState(StateRetrying)
	futureTime := time.Now().Add(10 * time.Second)
	msg.SetNextRetryAt(futureTime)

	t.Logf("Initial state: %s, NextRetryAt: %v", msg.GetState(), msg.GetNextRetryAt())

	// Create state machine and try to transition to queued
	sm := NewStateMachine()
	err := sm.Transition(msg, StateQueued)
	
	if err != nil {
		t.Logf("Transition error: %v", err)
	}

	t.Logf("After transition: State=%s, NextRetryAt=%v", msg.GetState(), msg.GetNextRetryAt())

	// Check if the message is ready for retry now
	if !msg.IsReadyForRetry() {
		t.Error("Message should be ready for retry after transitioning to queued state")
	}
}