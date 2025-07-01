package queue

import (
	"sync"
	"time"
)

// Import StateTransition from message.go to avoid duplication
// NOTE: This is a temporary measure until the full refactor is complete

// State represents the current state of a message in the queue.
type State string

// MessageState represents the state machine states.
type MessageState int

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

// StateTransition represents a single state change in the message lifecycle.
type StateTransition struct {
	From      MessageState
	To        MessageState
	Timestamp time.Time
	Reason    string
	Error     error
}

const (
	// MessageStateQueued indicates the message is waiting to be processed.
	MessageStateQueued MessageState = iota
	// MessageStateProcessing indicates the message is being processed.
	MessageStateProcessing
	// MessageStateValidating indicates the message is being validated.
	MessageStateValidating
	// MessageStateCompleted indicates processing completed successfully.
	MessageStateCompleted
	// MessageStateFailed indicates processing failed permanently.
	MessageStateFailed
	// MessageStateRetrying indicates the message will be retried.
	MessageStateRetrying
)

// Priority represents message priority levels.
type Priority int

const (
	// PriorityNormal is the default priority.
	PriorityNormal Priority = iota
	// PriorityHigh is for scheduled tasks and retries.
	PriorityHigh
)

// Message represents a message in the queue system.
type Message struct {
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Error          error
	ProcessedAt    *time.Time
	NextRetryAt    *time.Time // When the message should be retried (if in retry state)
	Sender         string     // Display name of sender
	SenderNumber   string     // Phone number of sender
	ID             string
	ConversationID string
	Text           string
	Response       string
	State          State
	Attempts       int
	MaxAttempts    int
	StateHistory   []StateTransition
	mu             sync.RWMutex
}

// NewMessage creates a new message with default values.
func NewMessage(id, conversationID, sender, senderNumber, text string) *Message {
	now := time.Now()
	return &Message{
		ID:             id,
		ConversationID: conversationID,
		Sender:         sender,
		SenderNumber:   senderNumber,
		Text:           text,
		State:          StateQueued,
		Attempts:       0,
		MaxAttempts:    3,
		CreatedAt:      now,
		UpdatedAt:      now,
		StateHistory:   []StateTransition{},
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

// AddStateTransition records a state transition in the message history.
func (m *Message) AddStateTransition(from, to State, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	transition := StateTransition{
		From:      stateToMessageState(from),
		To:        stateToMessageState(to),
		Timestamp: time.Now(),
		Reason:    reason,
	}

	m.StateHistory = append(m.StateHistory, transition)
}

// GetStateHistory returns a copy of the state history.
func (m *Message) GetStateHistory() []StateTransition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modifications
	history := make([]StateTransition, len(m.StateHistory))
	copy(history, m.StateHistory)
	return history
}

// AtomicTransition performs an atomic state transition.
// It returns true if the transition was successful, false otherwise.
func (m *Message) AtomicTransition(expectedState, newState State) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.State != expectedState {
		return false
	}

	m.State = newState
	m.UpdatedAt = time.Now()
	return true
}

// stateToMessageState converts a State to MessageState.
func stateToMessageState(s State) MessageState {
	switch s {
	case StateQueued:
		return MessageStateQueued
	case StateProcessing:
		return MessageStateProcessing
	case StateValidating:
		return MessageStateValidating
	case StateCompleted:
		return MessageStateCompleted
	case StateFailed:
		return MessageStateFailed
	case StateRetrying:
		return MessageStateRetrying
	default:
		return MessageStateQueued
	}
}

// SetNextRetryAt safely sets the next retry time.
func (m *Message) SetNextRetryAt(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NextRetryAt = &t
	m.UpdatedAt = time.Now()
}

// ClearNextRetryAt clears the next retry time.
func (m *Message) ClearNextRetryAt() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NextRetryAt = nil
	m.UpdatedAt = time.Now()
}

// GetNextRetryAt safely gets the next retry time.
func (m *Message) GetNextRetryAt() *time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.NextRetryAt == nil {
		return nil
	}
	t := *m.NextRetryAt
	return &t
}

// IsReadyForRetry checks if the message is ready to be retried.
func (m *Message) IsReadyForRetry() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// If NextRetryAt is set and in the future, the message is not ready
	if m.NextRetryAt != nil && time.Now().Before(*m.NextRetryAt) {
		return false
	}
	// Otherwise, the message is ready to process
	return true
}

// QueuedMessage represents a message in the queue with comprehensive state tracking.
// This type is designed to be immutable - state changes return new instances.
type QueuedMessage struct {
	// Core message fields
	ID             string
	ConversationID string
	From           string
	Text           string
	Priority       Priority

	// State tracking
	State        MessageState
	StateHistory []StateTransition

	// Timing information
	QueuedAt    time.Time
	ProcessedAt *time.Time
	CompletedAt *time.Time

	// Error tracking
	LastError    error
	ErrorHistory []ErrorRecord

	// Attempt tracking
	Attempts    int
	MaxAttempts int
	NextRetryAt *time.Time

	// Internal fields (not exposed)
	mu sync.RWMutex
}

// ErrorRecord tracks an error that occurred during processing.
type ErrorRecord struct {
	Timestamp time.Time
	State     MessageState
	Error     error
	Attempt   int
}

// Stats provides queue statistics.
type Stats struct {
	TotalQueued        int
	TotalProcessing    int
	TotalCompleted     int
	TotalFailed        int
	ConversationCount  int
	OldestMessageAge   time.Duration
	AverageWaitTime    time.Duration
	AverageProcessTime time.Duration
	ActiveWorkers      int
	HealthyWorkers     int
}
