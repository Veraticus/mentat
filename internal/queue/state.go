package queue

import (
	"fmt"
	"sync"
)

// stateMachine implements the StateMachine interface.
type stateMachine struct {
	// transitions defines valid state transitions.
	transitions map[State][]State
	// validator provides enhanced transition validation.
	validator   *stateValidator
	mu          sync.RWMutex
}

// NewStateMachine creates a new state machine with predefined transitions.
func NewStateMachine() StateMachine {
	return &stateMachine{
		transitions: map[State][]State{
			StateQueued:     {StateProcessing},
			StateProcessing: {StateValidating, StateRetrying, StateFailed, StateCompleted},
			StateValidating: {StateCompleted, StateRetrying, StateFailed},
			StateRetrying:   {StateQueued, StateProcessing, StateFailed},
			StateCompleted:  {}, // Terminal state
			StateFailed:     {}, // Terminal state
		},
		validator: newStateValidator(),
	}
}

// Transition moves a message to a new state if the transition is valid.
func (sm *stateMachine) Transition(msg *Message, to State) error {
	if msg == nil {
		return fmt.Errorf("cannot transition nil message")
	}
	
	// We need to handle this atomically to prevent race conditions
	currentState := msg.GetState()
	
	// Check structural validity first
	if !sm.CanTransition(currentState, to) {
		// Provide detailed explanation for invalid transitions
		explanation := sm.validator.explainInvalidTransition(currentState, to)
		return fmt.Errorf("invalid transition from %s to %s: %s", currentState, to, explanation)
	}
	
	// Validate business rules
	if err := sm.validator.validateTransition(msg, currentState, to); err != nil {
		return err
	}
	
	// Handle state-specific logic and determine final state
	finalState := to
	reason := "state transition"
	shouldIncrement := false
	
	switch to {
	case StateProcessing:
		switch currentState {
		case StateRetrying:
			shouldIncrement = true
			reason = fmt.Sprintf("retry attempt %d", msg.Attempts+1)
		case StateQueued:
			reason = "starting processing"
		}
	case StateFailed:
		if msg.CanRetry() && currentState == StateProcessing {
			// If we can retry, go to retrying instead
			finalState = StateRetrying
			reason = fmt.Sprintf("failed but retrying (attempt %d/%d)", msg.Attempts, msg.MaxAttempts)
		} else {
			reason = "permanently failed"
			if msg.Error != nil {
				reason = fmt.Sprintf("permanently failed: %v", msg.Error)
			}
		}
	case StateCompleted:
		reason = "successfully completed"
	case StateValidating:
		reason = "starting validation"
	case StateRetrying:
		reason = fmt.Sprintf("scheduling retry (attempt %d/%d)", msg.Attempts, msg.MaxAttempts)
	}
	
	// Attempt atomic transition
	if !msg.AtomicTransition(currentState, finalState) {
		// State changed while we were processing
		return fmt.Errorf("state changed during transition (was %s)", currentState)
	}
	
	// Clear NextRetryAt when transitioning from retrying to queued
	// This ensures the message is immediately available for processing
	if currentState == StateRetrying && finalState == StateQueued {
		msg.ClearNextRetryAt()
	}
	
	// Handle post-transition updates
	if shouldIncrement {
		msg.IncrementAttempts()
	}
	
	// Record the transition in history
	msg.AddStateTransition(currentState, finalState, reason)
	
	return nil
}

// CanTransition checks if a transition from one state to another is valid.
func (sm *stateMachine) CanTransition(from, to State) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	validStates, exists := sm.transitions[from]
	if !exists {
		return false
	}
	
	for _, state := range validStates {
		if state == to {
			return true
		}
	}
	
	return false
}


// IsTerminal checks if a state is terminal (no outgoing transitions).
func (sm *stateMachine) IsTerminal(state State) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	transitions, exists := sm.transitions[state]
	return exists && len(transitions) == 0
}