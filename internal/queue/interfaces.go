// Package queue provides message queue management with conversation awareness.
package queue

import (
	"context"
	
	"github.com/Veraticus/mentat/internal/signal"
)

// MessageQueue provides conversation-aware queuing.
type MessageQueue interface {
	// Enqueue adds a message to the queue
	Enqueue(msg signal.IncomingMessage) error
	
	// GetNext returns the next message for a worker to process
	GetNext(workerID string) (*QueuedMessage, error)
	
	// UpdateState marks a message state transition
	UpdateState(msgID string, state MessageState, reason string) error
	
	// Stats returns queue statistics
	Stats() Stats
}

// Worker processes messages from the queue.
type Worker interface {
	// Start begins processing messages. Blocks until context is canceled
	Start(ctx context.Context) error
	
	// Stop gracefully stops the worker
	Stop() error
	
	// ID returns the worker's unique identifier
	ID() string
}

// RateLimiter provides per-conversation rate limiting.
type RateLimiter interface {
	// Allow returns true if the conversation can proceed
	Allow(conversationID string) bool
	
	// Record marks a conversation as active
	Record(conversationID string)
}

// StateMachine manages message state transitions.
type StateMachine interface {
	// CanTransition validates if a state transition is allowed
	CanTransition(from, to State) bool
	
	// Transition updates the message state
	Transition(msg *Message, to State) error
	
	// IsTerminal checks if a state is terminal (no outgoing transitions)
	IsTerminal(state State) bool
}