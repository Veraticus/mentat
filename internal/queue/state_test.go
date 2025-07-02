package queue_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Veraticus/mentat/internal/queue"
)

func TestStateMachine_Transition(t *testing.T) {
	tests := []struct {
		from       queue.State
		to         queue.State
		finalState queue.State
		name       string
		wantErr    bool
	}{
		// Valid transitions
		{
			name:       "queued to processing",
			from:       queue.StateQueued,
			to:         queue.StateProcessing,
			wantErr:    false,
			finalState: queue.StateProcessing,
		},
		{
			name:       "processing to validating",
			from:       queue.StateProcessing,
			to:         queue.StateValidating,
			wantErr:    false,
			finalState: queue.StateValidating,
		},
		{
			name:       "processing to retrying",
			from:       queue.StateProcessing,
			to:         queue.StateRetrying,
			wantErr:    false,
			finalState: queue.StateRetrying,
		},
		{
			name:       "processing to failed when can retry",
			from:       queue.StateProcessing,
			to:         queue.StateFailed,
			wantErr:    false,
			finalState: queue.StateRetrying, // Should go to retrying instead
		},
		{
			name:       "validating to completed",
			from:       queue.StateValidating,
			to:         queue.StateCompleted,
			wantErr:    false,
			finalState: queue.StateCompleted,
		},
		{
			name:       "retrying to processing",
			from:       queue.StateRetrying,
			to:         queue.StateProcessing,
			wantErr:    false,
			finalState: queue.StateProcessing,
		},

		// Invalid transitions
		{
			name:    "queued to completed",
			from:    queue.StateQueued,
			to:      queue.StateCompleted,
			wantErr: true,
		},
		{
			name:    "completed to processing",
			from:    queue.StateCompleted,
			to:      queue.StateProcessing,
			wantErr: true,
		},
		{
			name:    "failed to processing",
			from:    queue.StateFailed,
			to:      queue.StateProcessing,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := queue.NewStateMachine()
			msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
			msg.SetState(tt.from)

			// Set up message state for validation rules
			switch tt.name {
			case "processing to validating":
				msg.SetResponse("test response")
			case "processing to failed when can retry":
				msg.SetError(fmt.Errorf("processing error"))
			case "validating to completed":
				msg.SetResponse("validated response")
			}

			err := sm.Transition(msg, tt.to)

			if (err != nil) != tt.wantErr {
				t.Errorf("Transition() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && msg.GetState() != tt.finalState {
				t.Errorf("Expected state %s, got %s", tt.finalState, msg.GetState())
			}
		})
	}
}

func TestStateMachine_TransitionWithMaxAttempts(t *testing.T) {
	sm := queue.NewStateMachine()
	msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
	msg.SetState(queue.StateProcessing)
	msg.MaxAttempts = 3
	msg.Attempts = 3 // Already at max attempts

	// Should go to failed, not retrying
	err := sm.Transition(msg, queue.StateFailed)
	if err != nil {
		t.Errorf("Transition() unexpected error: %v", err)
	}

	if msg.GetState() != queue.StateFailed {
		t.Errorf("Expected state %s, got %s", queue.StateFailed, msg.GetState())
	}
}

func TestStateMachine_TransitionNilMessage(t *testing.T) {
	sm := queue.NewStateMachine()

	err := sm.Transition(nil, queue.StateProcessing)
	if err == nil {
		t.Error("Expected error for nil message")
	}
}

func TestStateMachine_CanTransition(t *testing.T) {
	tests := []struct {
		name string
		from queue.State
		to   queue.State
		want bool
	}{
		// Valid transitions
		{"queued to processing", queue.StateQueued, queue.StateProcessing, true},
		{"processing to validating", queue.StateProcessing, queue.StateValidating, true},
		{"processing to retrying", queue.StateProcessing, queue.StateRetrying, true},
		{"processing to failed", queue.StateProcessing, queue.StateFailed, true},
		{"validating to completed", queue.StateValidating, queue.StateCompleted, true},
		{"validating to retrying", queue.StateValidating, queue.StateRetrying, true},
		{"validating to failed", queue.StateValidating, queue.StateFailed, true},
		{"retrying to processing", queue.StateRetrying, queue.StateProcessing, true},

		// Invalid transitions
		{"queued to completed", queue.StateQueued, queue.StateCompleted, false},
		{"queued to failed", queue.StateQueued, queue.StateFailed, false},
		{"completed to any", queue.StateCompleted, queue.StateProcessing, false},
		{"failed to any", queue.StateFailed, queue.StateProcessing, false},
		{"processing to queued", queue.StateProcessing, queue.StateQueued, false},

		// Unknown states
		{"unknown from state", queue.State("unknown"), queue.StateProcessing, false},
	}

	sm := queue.NewStateMachine()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sm.CanTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestStateMachine_RetryingIncrementsAttempts(t *testing.T) {
	sm := queue.NewStateMachine()
	msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")

	// Start in retrying state
	msg.SetState(queue.StateRetrying)
	initialAttempts := msg.Attempts

	// Transition to processing
	err := sm.Transition(msg, queue.StateProcessing)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Attempts should have incremented
	if msg.Attempts != initialAttempts+1 {
		t.Errorf("Expected attempts to be %d, got %d", initialAttempts+1, msg.Attempts)
	}
}

func TestStateMachine_ConcurrentTransitions(t *testing.T) {
	sm := queue.NewStateMachine()
	msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")

	// Run multiple goroutines trying to transition
	done := make(chan bool, 10)
	successCount := int32(0)

	for range 10 {
		go func() {
			defer func() { done <- true }()

			// Try valid transition
			err := sm.Transition(msg, queue.StateProcessing)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else if !strings.Contains(err.Error(), "invalid transition") &&
				!strings.Contains(err.Error(), "state changed during transition") {
				// Only log if it's not an expected concurrent modification or invalid transition error
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	// Exactly one goroutine should have succeeded
	if atomic.LoadInt32(&successCount) != 1 {
		t.Errorf("Expected exactly 1 successful transition, got %d", successCount)
	}

	// Message should be in processing state
	if msg.GetState() != queue.StateProcessing {
		t.Errorf("Expected state %s, got %s", queue.StateProcessing, msg.GetState())
	}
}

func TestStateMachine_StateHistory(t *testing.T) {
	sm := queue.NewStateMachine()
	msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")

	// Initial state should have no history
	history := msg.GetStateHistory()
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d entries", len(history))
	}

	// Transition from queued to processing
	err := sm.Transition(msg, queue.StateProcessing)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	history = msg.GetStateHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 history entry, got %d", len(history))
	}

	if history[0].From != queue.MessageStateQueued || history[0].To != queue.MessageStateProcessing {
		t.Errorf("Expected transition from %d to %d, got from %d to %d",
			queue.MessageStateQueued, queue.MessageStateProcessing, history[0].From, history[0].To)
	}

	if history[0].Reason != queue.StartingProcessingReason {
		t.Errorf("Expected reason '%s', got '%s'", queue.StartingProcessingReason, history[0].Reason)
	}

	// Set response before validating
	msg.SetResponse("test response")

	// Transition to validating
	err = sm.Transition(msg, queue.StateValidating)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	history = msg.GetStateHistory()
	if len(history) != 2 {
		t.Fatalf("Expected 2 history entries, got %d", len(history))
	}

	if history[1].Reason != "starting validation" {
		t.Errorf("Expected reason 'starting validation', got '%s'", history[1].Reason)
	}

	// Transition to completed
	err = sm.Transition(msg, queue.StateCompleted)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	history = msg.GetStateHistory()
	if len(history) != 3 {
		t.Fatalf("Expected 3 history entries, got %d", len(history))
	}

	if history[2].Reason != "successfully completed" {
		t.Errorf("Expected reason 'successfully completed', got '%s'", history[2].Reason)
	}
}

func TestStateMachine_RetryHistory(t *testing.T) {
	sm := queue.NewStateMachine()
	msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
	msg.SetState(queue.StateProcessing)
	msg.MaxAttempts = 3
	msg.Attempts = 1

	// Set error before failing
	msg.SetError(fmt.Errorf("processing failed"))

	// Simulate a failure that should trigger retry
	err := sm.Transition(msg, queue.StateFailed)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have transitioned to retrying, not failed
	if msg.GetState() != queue.StateRetrying {
		t.Errorf("Expected state %s, got %s", queue.StateRetrying, msg.GetState())
	}

	history := msg.GetStateHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 history entry, got %d", len(history))
	}

	expectedReason := "failed but retrying (attempt 1/3)"
	if history[0].Reason != expectedReason {
		t.Errorf("Expected reason '%s', got '%s'", expectedReason, history[0].Reason)
	}

	// Transition back to processing for retry
	err = sm.Transition(msg, queue.StateProcessing)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	history = msg.GetStateHistory()
	if len(history) != 2 {
		t.Fatalf("Expected 2 history entries, got %d", len(history))
	}

	// Attempts should have incremented
	if msg.Attempts != 2 {
		t.Errorf("Expected attempts to be 2, got %d", msg.Attempts)
	}

	expectedReason = "retry attempt 2"
	if history[1].Reason != expectedReason {
		t.Errorf("Expected reason '%s', got '%s'", expectedReason, history[1].Reason)
	}
}

func TestStateMachine_IsTerminal(t *testing.T) {
	sm := queue.NewStateMachine()

	tests := []struct {
		name     string
		state    queue.State
		terminal bool
	}{
		{"completed is terminal", queue.StateCompleted, true},
		{"failed is terminal", queue.StateFailed, true},
		{"queued is not terminal", queue.StateQueued, false},
		{"processing is not terminal", queue.StateProcessing, false},
		{"validating is not terminal", queue.StateValidating, false},
		{"retrying is not terminal", queue.StateRetrying, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sm.IsTerminal(tt.state); got != tt.terminal {
				t.Errorf("IsTerminal(%s) = %v, want %v", tt.state, got, tt.terminal)
			}
		})
	}
}
