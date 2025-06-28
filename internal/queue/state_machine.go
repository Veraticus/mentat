package queue

import (
	"fmt"
	"sync"
)

// stateMachine implements the StateMachine interface.
type stateMachine struct {
	// transitions defines valid state transitions.
	transitions map[State][]State
	mu          sync.RWMutex
}

// NewStateMachine creates a new state machine with predefined transitions.
func NewStateMachine() StateMachine {
	return &stateMachine{
		transitions: map[State][]State{
			StateQueued:     {StateProcessing},
			StateProcessing: {StateValidating, StateRetrying, StateFailed},
			StateValidating: {StateCompleted, StateRetrying, StateFailed},
			StateRetrying:   {StateProcessing},
			StateCompleted:  {}, // Terminal state
			StateFailed:     {}, // Terminal state
		},
	}
}

// Transition moves a message to a new state if the transition is valid.
func (sm *stateMachine) Transition(msg *Message, to State) error {
	if msg == nil {
		return fmt.Errorf("cannot transition nil message")
	}
	
	currentState := msg.GetState()
	
	if !sm.CanTransition(currentState, to) {
		return fmt.Errorf("invalid transition from %s to %s", currentState, to)
	}
	
	// Log the transition for observability
	oldState := currentState
	msg.SetState(to)
	
	// Handle state-specific logic
	switch to {
	case StateProcessing:
		if oldState == StateRetrying {
			msg.IncrementAttempts()
		}
	case StateFailed:
		if msg.CanRetry() && oldState == StateProcessing {
			// If we can retry, go to retrying instead
			msg.SetState(StateRetrying)
			return nil
		}
	}
	
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