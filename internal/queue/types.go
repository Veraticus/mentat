package queue

import (
	"sync"
	"time"
)

// State represents the current state of a message in the queue.
type State string

const (
	// StateQueued indicates the message is waiting to be processed.
	StateQueued State = "queued"
	
	// StateProcessing indicates the message is being handled by a worker.
	StateProcessing State = "processing"
	
	// StateValidating indicates the message is being validated.
	StateValidating State = "validating"
	
	// StateCompleted indicates the message was processed successfully.
	StateCompleted State = "completed"
	
	// StateFailed indicates the message processing failed permanently.
	StateFailed State = "failed"
	
	// StateRetrying indicates the message will be retried.
	StateRetrying State = "retrying"
)

// Message represents a message in the queue system.
type Message struct {
	// ID is the unique identifier for this message.
	ID string
	
	// ConversationID groups related messages together.
	ConversationID string
	
	// Sender is the user who sent the message.
	Sender string
	
	// Text is the message content.
	Text string
	
	// State is the current processing state.
	State State
	
	// Attempts tracks how many times we've tried to process this.
	Attempts int
	
	// MaxAttempts is the maximum number of processing attempts.
	MaxAttempts int
	
	// CreatedAt is when the message entered the queue.
	CreatedAt time.Time
	
	// UpdatedAt is when the message was last modified.
	UpdatedAt time.Time
	
	// ProcessedAt is when processing completed (if applicable).
	ProcessedAt *time.Time
	
	// Error contains the last error if processing failed.
	Error error
	
	// Response contains the LLM response if processing succeeded.
	Response string
	
	// mu protects concurrent access to the message.
	mu sync.RWMutex
}

// NewMessage creates a new message with default values.
func NewMessage(id, conversationID, sender, text string) *Message {
	now := time.Now()
	return &Message{
		ID:             id,
		ConversationID: conversationID,
		Sender:         sender,
		Text:           text,
		State:          StateQueued,
		Attempts:       0,
		MaxAttempts:    3,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// SetState safely updates the message state.
func (m *Message) SetState(state State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.State = state
	m.UpdatedAt = time.Now()
}

// GetState safely reads the message state.
func (m *Message) GetState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.State
}

// IncrementAttempts safely increments the attempt counter.
func (m *Message) IncrementAttempts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Attempts++
	m.UpdatedAt = time.Now()
	return m.Attempts
}

// SetError safely sets the error field.
func (m *Message) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Error = err
	m.UpdatedAt = time.Now()
}

// SetResponse safely sets the response and marks completion time.
func (m *Message) SetResponse(response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Response = response
	now := time.Now()
	m.ProcessedAt = &now
	m.UpdatedAt = now
}

// CanRetry checks if the message can be retried.
func (m *Message) CanRetry() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Attempts < m.MaxAttempts
}