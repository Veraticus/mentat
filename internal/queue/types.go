package queue

import (
	"sync"
	"time"
)

// Import StateTransition from message.go to avoid duplication
// NOTE: This is a temporary measure until the full refactor is complete

// State represents the current state of a message in the queue.
type State string

const (
	// defaultMessageMaxAttempts is the default maximum retry attempts for a message.
	defaultMessageMaxAttempts = 3

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
	From      State
	To        State
	Timestamp time.Time
	Reason    string
	Error     error
}

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
	CompletedAt    *time.Time // When the message completed processing
	Sender         string     // Display name of sender
	SenderNumber   string     // Phone number of sender
	ID             string
	ConversationID string
	Text           string
	Response       string
	State          State
	Priority       Priority // Message priority for scheduling
	Attempts       int
	MaxAttempts    int
	StateHistory   []StateTransition
	ErrorHistory   []ErrorRecord // History of all errors encountered
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
		Priority:       PriorityNormal,
		Attempts:       0,
		MaxAttempts:    defaultMessageMaxAttempts,
		CreatedAt:      now,
		UpdatedAt:      now,
		StateHistory:   []StateTransition{},
		ErrorHistory:   []ErrorRecord{},
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

// SetError safely sets the error field and adds to error history.
func (m *Message) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Error = err
	m.UpdatedAt = time.Now()

	// Add to error history if error is not nil
	if err != nil {
		errorRecord := ErrorRecord{
			Timestamp: time.Now(),
			State:     m.State,
			Error:     err,
			Attempt:   m.Attempts,
		}
		m.ErrorHistory = append(m.ErrorHistory, errorRecord)
	}
}

// SetResponse safely sets the response and marks completion time.
func (m *Message) SetResponse(response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Response = response
	now := time.Now()
	m.ProcessedAt = &now
	m.UpdatedAt = now
	// Set CompletedAt when response is recorded
	if m.State == StateCompleted {
		m.CompletedAt = &now
	}
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
		From:      from,
		To:        to,
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

// SetPriority safely sets the message priority.
func (m *Message) SetPriority(priority Priority) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Priority = priority
	m.UpdatedAt = time.Now()
}

// GetPriority safely gets the message priority.
func (m *Message) GetPriority() Priority {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Priority
}

// SetCompletedAt safely sets the completion time.
func (m *Message) SetCompletedAt(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CompletedAt = &t
	m.UpdatedAt = time.Now()
}

// GetCompletedAt safely gets the completion time.
func (m *Message) GetCompletedAt() *time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.CompletedAt == nil {
		return nil
	}
	t := *m.CompletedAt
	return &t
}

// GetErrorHistory returns a copy of the error history.
func (m *Message) GetErrorHistory() []ErrorRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modifications
	history := make([]ErrorRecord, len(m.ErrorHistory))
	copy(history, m.ErrorHistory)
	return history
}

// ErrorRecord tracks an error that occurred during processing.
type ErrorRecord struct {
	Timestamp time.Time
	State     State
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
	LongestMessageAge  time.Duration
	AverageWaitTime    time.Duration
	AverageProcessTime time.Duration
	ActiveWorkers      int
	HealthyWorkers     int
}
