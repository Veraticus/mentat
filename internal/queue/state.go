package queue

import (
	"fmt"
	"sync"
)

const (
	// StartingProcessingReason is the reason used when transitioning from queued to processing.
	StartingProcessingReason = "starting processing"
	// DefaultTransitionReason is the default reason for state transitions.
	DefaultTransitionReason = "state transition"
)

// StateMachineImpl implements the StateMachine interface.
type StateMachineImpl struct {
	// transitions defines valid state transitions.
	transitions map[State][]State
	// validator provides enhanced transition validation.
	validator StateValidator
	mu        sync.RWMutex
}

// NewStateMachine creates a new state machine with predefined transitions.
func NewStateMachine() *StateMachineImpl {
	return &StateMachineImpl{
		transitions: map[State][]State{
			StateQueued:     {StateProcessing},
			StateProcessing: {StateValidating, StateRetrying, StateFailed, StateCompleted},
			StateValidating: {StateCompleted, StateRetrying, StateFailed},
			StateRetrying:   {StateQueued, StateProcessing, StateFailed},
			StateCompleted:  {}, // Terminal state
			StateFailed:     {}, // Terminal state
		},
		validator: NewStateValidator(),
	}
}

// determineTransitionDetails determines the final state and reason for a transition.
func (sm *StateMachineImpl) determineTransitionDetails(msg *Message, currentState, to State) (State, string, bool) {
	finalState := to
	reason := DefaultTransitionReason
	shouldIncrement := false

	switch to {
	case StateProcessing:
		reason, shouldIncrement = sm.handleProcessingTransition(msg, currentState)
	case StateFailed:
		finalState, reason = sm.handleFailedTransition(msg, currentState)
	case StateCompleted:
		reason = "successfully completed"
	case StateValidating:
		reason = "starting validation"
	case StateRetrying:
		reason = fmt.Sprintf("scheduling retry (attempt %d/%d)", msg.Attempts, msg.MaxAttempts)
	case StateQueued:
		reason = "queuing message"
	}

	return finalState, reason, shouldIncrement
}

// handleProcessingTransition handles transitions to the processing state.
func (sm *StateMachineImpl) handleProcessingTransition(msg *Message, currentState State) (string, bool) {
	switch currentState {
	case StateRetrying:
		return fmt.Sprintf("retry attempt %d", msg.Attempts+1), true
	case StateQueued:
		return StartingProcessingReason, false
	case StateProcessing, StateValidating, StateCompleted, StateFailed:
		return DefaultTransitionReason, false
	}
	// This should never be reached as all cases are handled
	return "state transition", false
}

// handleFailedTransition handles transitions to the failed state.
func (sm *StateMachineImpl) handleFailedTransition(msg *Message, currentState State) (State, string) {
	if msg.CanRetry() && currentState == StateProcessing {
		// If we can retry, go to retrying instead
		return StateRetrying, fmt.Sprintf("failed but retrying (attempt %d/%d)", msg.Attempts, msg.MaxAttempts)
	}

	reason := "permanently failed"
	if msg.Error != nil {
		reason = fmt.Sprintf("permanently failed: %v", msg.Error)
	}
	return StateFailed, reason
}

// Transition moves a message to a new state if the transition is valid.
func (sm *StateMachineImpl) Transition(msg *Message, to State) error {
	if msg == nil {
		return fmt.Errorf("cannot transition nil message")
	}

	// We need to handle this atomically to prevent race conditions
	currentState := msg.GetState()

	// Check structural validity first
	if !sm.CanTransition(currentState, to) {
		// Provide detailed explanation for invalid transitions
		explanation := sm.validator.ExplainInvalidTransition(currentState, to)
		return fmt.Errorf("invalid transition from %s to %s: %s", currentState, to, explanation)
	}

	// Validate business rules
	if err := sm.validator.ValidateTransition(msg, currentState, to); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Handle state-specific logic and determine final state
	finalState, reason, shouldIncrement := sm.determineTransitionDetails(msg, currentState, to)

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
func (sm *StateMachineImpl) CanTransition(from, to State) bool {
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
func (sm *StateMachineImpl) IsTerminal(state State) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	transitions, exists := sm.transitions[state]
	return exists && len(transitions) == 0
}
