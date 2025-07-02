package queue_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

func TestNewQueuedMessage(t *testing.T) {
	msg := signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Hello, world!",
		Timestamp: time.Now(),
	}

	tests := []struct {
		name     string
		priority queue.Priority
		validate func(t *testing.T, qm *queue.QueuedMessage)
	}{
		{
			name:     "normal priority message",
			priority: queue.PriorityNormal,
			validate: func(t *testing.T, qm *queue.QueuedMessage) {
				t.Helper()
				if qm.Priority != queue.PriorityNormal {
					t.Errorf("expected priority %v, got %v", queue.PriorityNormal, qm.Priority)
				}
			},
		},
		{
			name:     "high priority message",
			priority: queue.PriorityHigh,
			validate: func(t *testing.T, qm *queue.QueuedMessage) {
				t.Helper()
				if qm.Priority != queue.PriorityHigh {
					t.Errorf("expected priority %v, got %v", queue.PriorityHigh, qm.Priority)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qm := queue.NewQueuedMessage(msg, tt.priority)

			// Verify all fields
			verifyBasicFields(t, qm, msg)
			verifyStateAndHistory(t, qm)
			verifyDefaults(t, qm)

			// Run custom validation
			tt.validate(t, qm)
		})
	}
}

func TestQueuedMessage_WithState(t *testing.T) {
	msg := createTestMessage()

	tests := []struct {
		name         string
		fromState    queue.MessageState
		toState      queue.MessageState
		reason       string
		expectError  bool
		validateFunc func(t *testing.T, oldMsg, newMsg *queue.QueuedMessage)
	}{
		{
			name:        "valid transition: queued to processing",
			fromState:   queue.MessageStateQueued,
			toState:     queue.MessageStateProcessing,
			reason:      "worker picked up",
			expectError: false,
			validateFunc: func(t *testing.T, oldMsg, newMsg *queue.QueuedMessage) {
				t.Helper()
				// Old message should be unchanged
				if oldMsg.State != queue.MessageStateQueued {
					t.Errorf("old message state changed")
				}

				// New message should have new state
				if newMsg.State != queue.MessageStateProcessing {
					t.Errorf("expected state %v, got %v", queue.MessageStateProcessing, newMsg.State)
				}

				// ProcessedAt should be set
				if newMsg.ProcessedAt == nil {
					t.Errorf("ProcessedAt not set")
				}
			},
		},
		{
			name:        "valid transition: processing to validating",
			fromState:   queue.MessageStateProcessing,
			toState:     queue.MessageStateValidating,
			reason:      "response generated",
			expectError: false,
		},
		{
			name:        "valid transition: validating to completed",
			fromState:   queue.MessageStateValidating,
			toState:     queue.MessageStateCompleted,
			reason:      "validation passed",
			expectError: false,
			validateFunc: func(t *testing.T, _, newMsg *queue.QueuedMessage) {
				t.Helper()
				if newMsg.CompletedAt == nil {
					t.Errorf("CompletedAt not set")
				}
			},
		},
		{
			name:        "valid transition: processing to retrying",
			fromState:   queue.MessageStateProcessing,
			toState:     queue.MessageStateRetrying,
			reason:      "temporary error",
			expectError: false,
			validateFunc: func(t *testing.T, _, newMsg *queue.QueuedMessage) {
				t.Helper()
				if newMsg.NextRetryAt == nil {
					t.Errorf("NextRetryAt not set")
				}
			},
		},
		{
			name:        "invalid transition: queued to completed",
			fromState:   queue.MessageStateQueued,
			toState:     queue.MessageStateCompleted,
			reason:      "skip processing",
			expectError: true,
		},
		{
			name:        "invalid transition: completed to processing",
			fromState:   queue.MessageStateCompleted,
			toState:     queue.MessageStateProcessing,
			reason:      "restart",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			oldMsg := setupInitialState(t, msg, tt.fromState)

			// Perform the transition
			newMsg, err := oldMsg.WithState(tt.toState, tt.reason)

			// Check error expectation
			validateError(t, err, tt.expectError)
			if err != nil {
				return
			}

			// Verify state history was updated
			validateStateHistory(t, newMsg, tt.fromState, tt.toState, tt.reason)

			// Run custom validation
			if tt.validateFunc != nil {
				tt.validateFunc(t, oldMsg, newMsg)
			}
		})
	}
}

func TestQueuedMessage_WithError(t *testing.T) {
	msg := createTestMessage()
	testError := fmt.Errorf("test error")

	// Add error
	newMsg := msg.WithError(testError, queue.MessageStateProcessing)

	// Original should be unchanged
	if msg.LastError != nil {
		t.Errorf("original message error was modified")
	}
	if len(msg.ErrorHistory) != 0 {
		t.Errorf("original message error history was modified")
	}

	// New message should have error
	if !errors.Is(newMsg.LastError, testError) {
		t.Errorf("expected LastError to be %v, got %v", testError, newMsg.LastError)
	}

	// Check error history
	errorHistory := newMsg.GetErrorHistory()
	if len(errorHistory) != 1 {
		t.Fatalf("expected 1 error history entry, got %d", len(errorHistory))
	}
	if !errors.Is(errorHistory[0].Error, testError) {
		t.Errorf("expected error %v in history, got %v", testError, errorHistory[0].Error)
	}
	if errorHistory[0].State != queue.MessageStateProcessing {
		t.Errorf("expected state %v in error history, got %v", queue.MessageStateProcessing, errorHistory[0].State)
	}
	if errorHistory[0].Attempt != 0 {
		t.Errorf("expected attempt 0 in error history, got %d", errorHistory[0].Attempt)
	}
}

func TestQueuedMessage_IncrementAttempts(t *testing.T) {
	msg := createTestMessage()

	// Increment attempts
	msg1 := msg.IncrementAttempts()
	msg2 := msg1.IncrementAttempts()
	msg3 := msg2.IncrementAttempts()

	// Verify immutability
	if msg.Attempts != 0 {
		t.Errorf("original message attempts was modified")
	}

	// Verify increments
	if msg1.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", msg1.Attempts)
	}
	if msg2.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", msg2.Attempts)
	}
	if msg3.Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", msg3.Attempts)
	}
}

func TestQueuedMessage_HasRetriesRemaining(t *testing.T) {
	tests := []struct {
		name        string
		attempts    int
		maxAttempts int
		expected    bool
	}{
		{"no attempts", 0, 3, true},
		{"one attempt", 1, 3, true},
		{"two attempts", 2, 3, true},
		{"at max attempts", 3, 3, false},
		{"over max attempts", 4, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := createTestMessage()
			msg.Attempts = tt.attempts
			msg.MaxAttempts = tt.maxAttempts

			if got := msg.HasRetriesRemaining(); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestQueuedMessage_IsReadyForRetry(t *testing.T) {
	msg := createTestMessage()

	// No retry time set - should be ready
	if !msg.IsReadyForRetry() {
		t.Errorf("expected message to be ready for retry when NextRetryAt is nil")
	}

	// Future retry time - not ready
	future := time.Now().Add(time.Hour)
	msg.NextRetryAt = &future
	if msg.IsReadyForRetry() {
		t.Errorf("expected message not to be ready for retry when NextRetryAt is in future")
	}

	// Past retry time - ready
	past := time.Now().Add(-time.Hour)
	msg.NextRetryAt = &past
	if !msg.IsReadyForRetry() {
		t.Errorf("expected message to be ready for retry when NextRetryAt is in past")
	}
}

func TestQueuedMessage_GetProcessingDuration(t *testing.T) {
	msg := createTestMessage()

	// Not processed yet
	if duration := msg.GetProcessingDuration(); duration != 0 {
		t.Errorf("expected 0 duration for unprocessed message, got %v", duration)
	}

	// Processing started
	processedAt := time.Now().Add(-5 * time.Second)
	msg.ProcessedAt = &processedAt
	duration := msg.GetProcessingDuration()
	if duration < 5*time.Second || duration > 6*time.Second {
		t.Errorf("expected duration around 5s, got %v", duration)
	}

	// Processing completed
	completedAt := processedAt.Add(3 * time.Second)
	msg.CompletedAt = &completedAt
	duration = msg.GetProcessingDuration()
	if duration != 3*time.Second {
		t.Errorf("expected duration 3s, got %v", duration)
	}
}

func TestQueuedMessage_GetQueueDuration(t *testing.T) {
	msg := createTestMessage()
	msg.QueuedAt = time.Now().Add(-10 * time.Second)

	// Still in queue
	duration := msg.GetQueueDuration()
	if duration < 10*time.Second || duration > 11*time.Second {
		t.Errorf("expected duration around 10s, got %v", duration)
	}

	// Processed after 5 seconds
	processedAt := msg.QueuedAt.Add(5 * time.Second)
	msg.ProcessedAt = &processedAt
	duration = msg.GetQueueDuration()
	if duration != 5*time.Second {
		t.Errorf("expected duration 5s, got %v", duration)
	}
}

func TestQueuedMessage_GetLastTransition(t *testing.T) {
	msg := createTestMessage()

	// Initial state
	last := msg.GetLastTransition()
	if last == nil {
		t.Fatal("expected last transition, got nil")
	}
	if last.To != queue.MessageStateQueued {
		t.Errorf("expected last transition to %v, got %v", queue.MessageStateQueued, last.To)
	}

	// After state change
	newMsg, _ := msg.WithState(queue.MessageStateProcessing, "test")
	last = newMsg.GetLastTransition()
	if last.From != queue.MessageStateQueued || last.To != queue.MessageStateProcessing {
		t.Errorf("expected transition from %v to %v, got from %v to %v",
			queue.MessageStateQueued, queue.MessageStateProcessing, last.From, last.To)
	}
}

func TestCalculateRetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{"first retry", 0, 900 * time.Millisecond, 1100 * time.Millisecond},
		{"second retry", 1, 1800 * time.Millisecond, 2200 * time.Millisecond},
		{"third retry", 2, 3600 * time.Millisecond, 4400 * time.Millisecond},
		{"max delay", 10, 4*time.Minute + 30*time.Second, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := queue.CalculateRetryDelay(tt.attempts)
			if delay < tt.minDelay || delay > tt.maxDelay {
				t.Errorf("expected delay between %v and %v, got %v", tt.minDelay, tt.maxDelay, delay)
			}
		})
	}
}

func TestMessageStateString(t *testing.T) {
	tests := []struct {
		state    queue.MessageState
		expected string
	}{
		{queue.MessageStateQueued, "queued"},
		{queue.MessageStateProcessing, "processing"},
		{queue.MessageStateValidating, "validating"},
		{queue.MessageStateCompleted, "completed"},
		{queue.MessageStateFailed, "failed"},
		{queue.MessageStateRetrying, "retrying"},
		{queue.MessageState(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := queue.MessageStateString(tt.state); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

// Helper functions

func verifyBasicFields(t *testing.T, qm *queue.QueuedMessage, msg signal.IncomingMessage) {
	t.Helper()
	// ID is generated from timestamp and sender
	expectedIDPrefix := fmt.Sprintf("%d-%s", msg.Timestamp.UnixNano(), msg.From)
	if qm.ID != expectedIDPrefix {
		t.Errorf("expected ID %s, got %s", expectedIDPrefix, qm.ID)
	}
	// ConversationID is generated from sender
	expectedConvID := fmt.Sprintf("conv-%s", msg.From)
	if qm.ConversationID != expectedConvID {
		t.Errorf("expected ConversationID %s, got %s", expectedConvID, qm.ConversationID)
	}
	if qm.From != msg.From {
		t.Errorf("expected From %s, got %s", msg.From, qm.From)
	}
	if qm.Text != msg.Text {
		t.Errorf("expected Text %s, got %s", msg.Text, qm.Text)
	}
}

func verifyStateAndHistory(t *testing.T, qm *queue.QueuedMessage) {
	t.Helper()
	// Verify initial state
	if qm.State != queue.MessageStateQueued {
		t.Errorf("expected initial state %v, got %v", queue.MessageStateQueued, qm.State)
	}

	// Verify state history
	if len(qm.StateHistory) != 1 {
		t.Fatalf("expected 1 state history entry, got %d", len(qm.StateHistory))
	}
	if qm.StateHistory[0].To != queue.MessageStateQueued {
		t.Errorf("expected initial state history To %v, got %v", queue.MessageStateQueued, qm.StateHistory[0].To)
	}
	if qm.StateHistory[0].Reason != "message created" {
		t.Errorf("expected reason 'message created', got %s", qm.StateHistory[0].Reason)
	}
}

func verifyDefaults(t *testing.T, qm *queue.QueuedMessage) {
	t.Helper()
	if qm.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", qm.Attempts)
	}
	if qm.MaxAttempts != 3 {
		t.Errorf("expected 3 max attempts, got %d", qm.MaxAttempts)
	}
	if len(qm.ErrorHistory) != 0 {
		t.Errorf("expected empty error history, got %d entries", len(qm.ErrorHistory))
	}
}

func setupInitialState(t *testing.T, msg *queue.QueuedMessage, targetState queue.MessageState) *queue.QueuedMessage {
	t.Helper()
	if targetState == queue.MessageStateQueued {
		return msg
	}

	// Transition to the required from state
	transitionalStates := getTransitionPath(queue.MessageStateQueued, targetState)
	currentMsg := msg
	for _, state := range transitionalStates {
		var err error
		currentMsg, err = currentMsg.WithState(state, "test setup")
		if err != nil {
			t.Fatalf("failed to set up initial state: %v", err)
		}
	}
	return currentMsg
}

func validateError(t *testing.T, err error, expectError bool) {
	t.Helper()
	if expectError && err == nil {
		t.Errorf("expected error but got none")
	}
	if !expectError && err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func validateStateHistory(
	t *testing.T,
	msg *queue.QueuedMessage,
	fromState, toState queue.MessageState,
	reason string,
) {
	t.Helper()
	history := msg.GetStateHistory()
	lastTransition := history[len(history)-1]
	if lastTransition.From != fromState {
		t.Errorf("expected transition from %v, got %v", fromState, lastTransition.From)
	}
	if lastTransition.To != toState {
		t.Errorf("expected transition to %v, got %v", toState, lastTransition.To)
	}
	if lastTransition.Reason != reason {
		t.Errorf("expected reason %s, got %s", reason, lastTransition.Reason)
	}
}

func createTestMessage() *queue.QueuedMessage {
	return queue.NewQueuedMessage(signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Test message",
		Timestamp: time.Now(),
	}, queue.PriorityNormal)
}

// getTransitionPath returns the states needed to transition from one state to another.
func getTransitionPath(from, to queue.MessageState) []queue.MessageState {
	// Simplified for testing - in real implementation this would use the state machine
	paths := map[string][]queue.MessageState{
		"queued-processing": {queue.MessageStateProcessing},
		"queued-validating": {queue.MessageStateProcessing, queue.MessageStateValidating},
		"queued-completed":  {queue.MessageStateProcessing, queue.MessageStateCompleted},
		"queued-retrying":   {queue.MessageStateProcessing, queue.MessageStateRetrying},
		"queued-failed":     {queue.MessageStateFailed},
	}

	key := queue.MessageStateString(from) + "-" + queue.MessageStateString(to)
	return paths[key]
}
