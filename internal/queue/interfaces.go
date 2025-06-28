package queue

import (
	"context"
	"time"
)

// LLM represents the language model interface for processing messages.
// Implementations should handle timeouts and context cancellation.
type LLM interface {
	// Query sends a prompt to the LLM and returns the response.
	// The sessionID maintains conversation context across queries.
	Query(ctx context.Context, prompt string, sessionID string) (string, error)
}

// Messenger represents the interface for sending and receiving messages.
// Implementations must be safe for concurrent use.
type Messenger interface {
	// Send delivers a message to the specified recipient.
	Send(ctx context.Context, recipient string, message string) error
	
	// Subscribe returns a channel of incoming messages.
	// The channel is closed when the context is cancelled.
	Subscribe(ctx context.Context) (<-chan IncomingMessage, error)
	
	// SendTypingIndicator notifies the recipient that we're typing.
	SendTypingIndicator(ctx context.Context, recipient string) error
}

// IncomingMessage represents a message received from a user.
type IncomingMessage struct {
	ID           string    // Unique message identifier
	Sender       string    // Sender identifier (e.g., phone number)
	Text         string    // Message content
	Timestamp    time.Time // When the message was received
	Conversation string    // Conversation identifier
}

// Worker processes messages from the queue.
type Worker interface {
	// Start begins processing messages. Blocks until context is cancelled.
	Start(ctx context.Context) error
	
	// Process handles a single message through its lifecycle.
	Process(ctx context.Context, msg *Message) error
}

// RateLimiter controls the rate of message processing per conversation.
type RateLimiter interface {
	// Allow returns true if the conversation can process a message now.
	Allow(conversationID string) bool
	
	// Wait blocks until the conversation can process a message.
	// Returns error if context is cancelled.
	Wait(ctx context.Context, conversationID string) error
}

// StateMachine manages message state transitions.
type StateMachine interface {
	// Transition moves a message to a new state if allowed.
	Transition(msg *Message, to State) error
	
	// CanTransition checks if a transition is valid.
	CanTransition(from, to State) bool
}