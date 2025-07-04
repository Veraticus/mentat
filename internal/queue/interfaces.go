// Package queue provides message queue management with conversation awareness.
package queue

import (
	"context"
	"time"
)

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

	// Wait blocks until the conversation can process a message
	Wait(ctx context.Context, conversationID string) error

	// Record marks a conversation as active
	Record(conversationID string)

	// CleanupStale removes token buckets that haven't been used recently
	CleanupStale(maxAge time.Duration)

	// Stats returns rate limiter statistics
	Stats() map[string]any
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
