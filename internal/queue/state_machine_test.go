package queue

import (
	"testing"
)

func TestStateMachine_Transition(t *testing.T) {
	tests := []struct {
		name        string
		from        State
		to          State
		wantErr     bool
		finalState  State // Expected final state (may differ from 'to' in some cases)
	}{
		// Valid transitions
		{
			name:       "queued to processing",
			from:       StateQueued,
			to:         StateProcessing,
			wantErr:    false,
			finalState: StateProcessing,
		},
		{
			name:       "processing to validating",
			from:       StateProcessing,
			to:         StateValidating,
			wantErr:    false,
			finalState: StateValidating,
		},
		{
			name:       "processing to retrying",
			from:       StateProcessing,
			to:         StateRetrying,
			wantErr:    false,
			finalState: StateRetrying,
		},
		{
			name:       "processing to failed when can retry",
			from:       StateProcessing,
			to:         StateFailed,
			wantErr:    false,
			finalState: StateRetrying, // Should go to retrying instead
		},
		{
			name:       "validating to completed",
			from:       StateValidating,
			to:         StateCompleted,
			wantErr:    false,
			finalState: StateCompleted,
		},
		{
			name:       "retrying to processing",
			from:       StateRetrying,
			to:         StateProcessing,
			wantErr:    false,
			finalState: StateProcessing,
		},
		
		// Invalid transitions
		{
			name:    "queued to completed",
			from:    StateQueued,
			to:      StateCompleted,
			wantErr: true,
		},
		{
			name:    "completed to processing",
			from:    StateCompleted,
			to:      StateProcessing,
			wantErr: true,
		},
		{
			name:    "failed to processing",
			from:    StateFailed,
			to:      StateProcessing,
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine()
			msg := NewMessage("test-id", "conv-1", "sender", "test message")
			msg.SetState(tt.from)
			
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
	sm := NewStateMachine()
	msg := NewMessage("test-id", "conv-1", "sender", "test message")
	msg.SetState(StateProcessing)
	msg.MaxAttempts = 3
	msg.Attempts = 3 // Already at max attempts
	
	// Should go to failed, not retrying
	err := sm.Transition(msg, StateFailed)
	if err != nil {
		t.Errorf("Transition() unexpected error: %v", err)
	}
	
	if msg.GetState() != StateFailed {
		t.Errorf("Expected state %s, got %s", StateFailed, msg.GetState())
	}
}

func TestStateMachine_TransitionNilMessage(t *testing.T) {
	sm := NewStateMachine()
	
	err := sm.Transition(nil, StateProcessing)
	if err == nil {
		t.Error("Expected error for nil message")
	}
}

func TestStateMachine_CanTransition(t *testing.T) {
	tests := []struct {
		name string
		from State
		to   State
		want bool
	}{
		// Valid transitions
		{"queued to processing", StateQueued, StateProcessing, true},
		{"processing to validating", StateProcessing, StateValidating, true},
		{"processing to retrying", StateProcessing, StateRetrying, true},
		{"processing to failed", StateProcessing, StateFailed, true},
		{"validating to completed", StateValidating, StateCompleted, true},
		{"validating to retrying", StateValidating, StateRetrying, true},
		{"validating to failed", StateValidating, StateFailed, true},
		{"retrying to processing", StateRetrying, StateProcessing, true},
		
		// Invalid transitions
		{"queued to completed", StateQueued, StateCompleted, false},
		{"queued to failed", StateQueued, StateFailed, false},
		{"completed to any", StateCompleted, StateProcessing, false},
		{"failed to any", StateFailed, StateProcessing, false},
		{"processing to queued", StateProcessing, StateQueued, false},
		
		// Unknown states
		{"unknown from state", State("unknown"), StateProcessing, false},
	}
	
	sm := NewStateMachine()
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sm.CanTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestStateMachine_RetryingIncrementsAttempts(t *testing.T) {
	sm := NewStateMachine()
	msg := NewMessage("test-id", "conv-1", "sender", "test message")
	
	// Start in retrying state
	msg.SetState(StateRetrying)
	initialAttempts := msg.Attempts
	
	// Transition to processing
	err := sm.Transition(msg, StateProcessing)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Attempts should have incremented
	if msg.Attempts != initialAttempts+1 {
		t.Errorf("Expected attempts to be %d, got %d", initialAttempts+1, msg.Attempts)
	}
}

func TestStateMachine_ConcurrentTransitions(t *testing.T) {
	sm := NewStateMachine()
	msg := NewMessage("test-id", "conv-1", "sender", "test message")
	
	// Run multiple goroutines trying to transition
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			
			// Try valid transition
			if msg.GetState() == StateQueued {
				sm.Transition(msg, StateProcessing)
			}
		}()
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Message should be in a valid state
	state := msg.GetState()
	if state != StateQueued && state != StateProcessing {
		t.Errorf("Message in unexpected state: %s", state)
	}
}