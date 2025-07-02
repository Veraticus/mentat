package queue

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// Message constants.
const (
	// defaultMaxAttempts is the default maximum retry attempts for a message.
	defaultMaxAttempts = 3
	// jitterDivisor is used to calculate jitter (10% jitter).
	jitterDivisor = 10
)

// NewQueuedMessage creates a new queued message from a Signal message.
func NewQueuedMessage(msg signal.IncomingMessage, priority Priority) *QueuedMessage {
	now := time.Now()
	// Generate ID from timestamp and sender
	id := strconv.FormatInt(msg.Timestamp.UnixNano(), 10) + "-" + msg.From
	// ConversationID is based on the sender
	conversationID := "conv-" + msg.From

	return &QueuedMessage{
		ID:             id,
		ConversationID: conversationID,
		From:           msg.From,
		Text:           msg.Text,
		Priority:       priority,
		State:          MessageStateQueued,
		StateHistory: []StateTransition{
			{
				From:      MessageStateQueued,
				To:        MessageStateQueued,
				Timestamp: now,
				Reason:    "message created",
			},
		},
		QueuedAt:     now,
		Attempts:     0,
		MaxAttempts:  defaultMaxAttempts,
		ErrorHistory: []ErrorRecord{},
	}
}

// WithState creates a new QueuedMessage with the given state.
// This maintains immutability by returning a new instance.
func (q *QueuedMessage) WithState(newState MessageState, reason string) (*QueuedMessage, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Validate state transition
	if !isValidTransition(q.State, newState) {
		return nil, fmt.Errorf("invalid transition from %s to %s", q.State, newState)
	}

	// Create a deep copy
	newMsg := q.deepCopy()

	// Update state
	oldState := newMsg.State
	newMsg.State = newState

	// Add transition to history
	transition := StateTransition{
		From:      oldState,
		To:        newState,
		Timestamp: time.Now(),
		Reason:    reason,
	}
	newMsg.StateHistory = append(newMsg.StateHistory, transition)

	// Update timing fields based on state
	now := time.Now()
	switch newState {
	case MessageStateProcessing:
		if newMsg.ProcessedAt == nil {
			newMsg.ProcessedAt = &now
		}
	case MessageStateCompleted:
		newMsg.CompletedAt = &now
	case MessageStateRetrying:
		// Calculate exponential backoff
		retryDelay := CalculateRetryDelay(newMsg.Attempts)
		retryTime := now.Add(retryDelay)
		newMsg.NextRetryAt = &retryTime
	case MessageStateQueued, MessageStateValidating, MessageStateFailed:
		// No timing updates needed for these states
	}

	return newMsg, nil
}

// WithError creates a new QueuedMessage with the given error recorded.
func (q *QueuedMessage) WithError(err error, state MessageState) *QueuedMessage {
	q.mu.RLock()
	defer q.mu.RUnlock()

	newMsg := q.deepCopy()
	newMsg.LastError = err

	// Add to error history
	errorRecord := ErrorRecord{
		Timestamp: time.Now(),
		State:     state,
		Error:     err,
		Attempt:   newMsg.Attempts,
	}
	newMsg.ErrorHistory = append(newMsg.ErrorHistory, errorRecord)

	return newMsg
}

// IncrementAttempts creates a new QueuedMessage with incremented attempt count.
func (q *QueuedMessage) IncrementAttempts() *QueuedMessage {
	q.mu.RLock()
	defer q.mu.RUnlock()

	newMsg := q.deepCopy()
	newMsg.Attempts++
	return newMsg
}

// GetStateHistory returns a copy of the state history.
func (q *QueuedMessage) GetStateHistory() []StateTransition {
	q.mu.RLock()
	defer q.mu.RUnlock()

	history := make([]StateTransition, len(q.StateHistory))
	copy(history, q.StateHistory)
	return history
}

// GetErrorHistory returns a copy of the error history.
func (q *QueuedMessage) GetErrorHistory() []ErrorRecord {
	q.mu.RLock()
	defer q.mu.RUnlock()

	history := make([]ErrorRecord, len(q.ErrorHistory))
	copy(history, q.ErrorHistory)
	return history
}

// GetProcessingDuration returns how long the message has been processing.
// Returns 0 if not yet processed or already completed.
func (q *QueuedMessage) GetProcessingDuration() time.Duration {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.ProcessedAt == nil {
		return 0
	}

	if q.CompletedAt != nil {
		return q.CompletedAt.Sub(*q.ProcessedAt)
	}

	return time.Since(*q.ProcessedAt)
}

// GetQueueDuration returns how long the message waited in queue.
func (q *QueuedMessage) GetQueueDuration() time.Duration {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.ProcessedAt != nil {
		return q.ProcessedAt.Sub(q.QueuedAt)
	}

	return time.Since(q.QueuedAt)
}

// HasRetriesRemaining checks if the message can be retried.
func (q *QueuedMessage) HasRetriesRemaining() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.Attempts < q.MaxAttempts
}

// IsReadyForRetry checks if enough time has passed for retry.
func (q *QueuedMessage) IsReadyForRetry() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.NextRetryAt == nil {
		return true
	}

	return time.Now().After(*q.NextRetryAt)
}

// GetLastTransition returns the most recent state transition.
func (q *QueuedMessage) GetLastTransition() *StateTransition {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.StateHistory) == 0 {
		return nil
	}

	last := q.StateHistory[len(q.StateHistory)-1]
	return &last
}

// deepCopy creates a deep copy of the QueuedMessage.
func (q *QueuedMessage) deepCopy() *QueuedMessage {
	newMsg := &QueuedMessage{
		ID:             q.ID,
		ConversationID: q.ConversationID,
		From:           q.From,
		Text:           q.Text,
		Priority:       q.Priority,
		State:          q.State,
		QueuedAt:       q.QueuedAt,
		LastError:      q.LastError,
		Attempts:       q.Attempts,
		MaxAttempts:    q.MaxAttempts,
	}

	// Deep copy pointers
	if q.ProcessedAt != nil {
		t := *q.ProcessedAt
		newMsg.ProcessedAt = &t
	}
	if q.CompletedAt != nil {
		t := *q.CompletedAt
		newMsg.CompletedAt = &t
	}
	if q.NextRetryAt != nil {
		t := *q.NextRetryAt
		newMsg.NextRetryAt = &t
	}

	// Deep copy slices
	newMsg.StateHistory = make([]StateTransition, len(q.StateHistory))
	copy(newMsg.StateHistory, q.StateHistory)

	newMsg.ErrorHistory = make([]ErrorRecord, len(q.ErrorHistory))
	copy(newMsg.ErrorHistory, q.ErrorHistory)

	return newMsg
}

// CalculateRetryDelay calculates exponential backoff for retries.
func CalculateRetryDelay(attempts int) time.Duration {
	const (
		baseDelay = time.Second
		maxDelay  = 5 * time.Minute
		maxShift  = 30 // Prevent overflow in bit shifting
	)

	// Handle edge cases
	if attempts <= 0 {
		return baseDelay
	}

	// Use multiplication instead of bit shifting to avoid gosec warnings
	delay := baseDelay
	for i := 0; i < attempts && i < maxShift; i++ {
		delay *= 2
		if delay > maxDelay {
			return maxDelay
		}
	}

	// Add 10% jitter
	jitterRange := delay / jitterDivisor
	if jitterRange > 0 {
		// Use modulo to get a value within jitter range
		jitter := time.Duration(time.Now().UnixNano() % int64(jitterRange))
		delay = delay - jitterRange/2 + jitter
	}

	return delay
}

// MessageStateString returns the string representation of a MessageState.
func MessageStateString(state MessageState) string {
	switch state {
	case MessageStateQueued:
		return "queued"
	case MessageStateProcessing:
		return "processing"
	case MessageStateValidating:
		return "validating"
	case MessageStateCompleted:
		return "completed"
	case MessageStateFailed:
		return "failed"
	case MessageStateRetrying:
		return "retrying"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

// String returns the string representation of a MessageState.
func (s MessageState) String() string {
	return MessageStateString(s)
}

// isValidTransition checks if a state transition is allowed.
// This will be replaced by the full state machine in Phase 11/12.
func isValidTransition(from, to MessageState) bool {
	validTransitions := map[MessageState][]MessageState{
		MessageStateQueued: {
			MessageStateProcessing,
			MessageStateFailed,
		},
		MessageStateProcessing: {
			MessageStateValidating,
			MessageStateFailed,
			MessageStateRetrying,
			MessageStateCompleted,
		},
		MessageStateValidating: {
			MessageStateCompleted,
			MessageStateFailed,
			MessageStateRetrying,
		},
		MessageStateRetrying: {
			MessageStateProcessing,
			MessageStateFailed,
		},
		MessageStateFailed: {
			MessageStateRetrying,
		},
		MessageStateCompleted: {}, // Terminal state
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, state := range allowed {
		if state == to {
			return true
		}
	}

	return false
}
