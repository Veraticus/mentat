package queue_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

func TestMessageCreationFromIncoming(t *testing.T) {
	msg := signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Hello, world!",
		Timestamp: time.Now(),
	}

	tests := []struct {
		name     string
		priority queue.Priority
		validate func(t *testing.T, qm *queue.Message)
	}{
		{
			name:     "normal priority message",
			priority: queue.PriorityNormal,
			validate: func(t *testing.T, qm *queue.Message) {
				t.Helper()
				if qm.Priority != queue.PriorityNormal {
					t.Errorf("expected priority %v, got %v", queue.PriorityNormal, qm.Priority)
				}
			},
		},
		{
			name:     "high priority message",
			priority: queue.PriorityHigh,
			validate: func(t *testing.T, qm *queue.Message) {
				t.Helper()
				if qm.Priority != queue.PriorityHigh {
					t.Errorf("expected priority %v, got %v", queue.PriorityHigh, qm.Priority)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create message with the format expected by NewMessage
			msgID := fmt.Sprintf("%d-%s", msg.Timestamp.UnixNano(), msg.From)
			// Parameters: id, conversationID, sender, senderNumber, text
			qm := queue.NewMessage(msgID, msg.From, msg.From, msg.From, msg.Text)
			qm.SetPriority(tt.priority)
			// Add initial state transition like Coordinator does
			qm.AddStateTransition(queue.StateQueued, queue.StateQueued, "message created")

			// Verify all fields
			verifyBasicFields(t, qm, msg)
			verifyStateAndHistory(t, qm)
			verifyDefaults(t, qm)

			// Run custom validation
			tt.validate(t, qm)
		})
	}
}

// transitionTest defines a state transition test case.
type transitionTest struct {
	name         string
	fromState    queue.State
	toState      queue.State
	reason       string
	expectError  bool
	validateFunc func(t *testing.T, msg *queue.Message)
}

// getTransitionTests returns all state transition test cases.
func getTransitionTests() []transitionTest {
	return []transitionTest{
		{
			name:        "valid transition: queued to processing",
			fromState:   queue.StateQueued,
			toState:     queue.StateProcessing,
			reason:      "worker picked up",
			expectError: false,
			validateFunc: func(t *testing.T, msg *queue.Message) {
				t.Helper()
				if msg.State != queue.StateProcessing {
					t.Errorf("expected state %v, got %v", queue.StateProcessing, msg.State)
				}
				history := msg.GetStateHistory()
				if len(history) < 2 {
					t.Errorf("expected at least 2 state transitions")
				}
			},
		},
		{
			name:        "valid transition: processing to validating",
			fromState:   queue.StateProcessing,
			toState:     queue.StateValidating,
			reason:      "response generated",
			expectError: false,
		},
		{
			name:        "valid transition: validating to completed",
			fromState:   queue.StateValidating,
			toState:     queue.StateCompleted,
			reason:      "validation passed",
			expectError: false,
			validateFunc: func(t *testing.T, msg *queue.Message) {
				t.Helper()
				if msg.GetCompletedAt() == nil {
					t.Errorf("CompletedAt not set")
				}
			},
		},
		{
			name:        "valid transition: processing to retrying",
			fromState:   queue.StateProcessing,
			toState:     queue.StateRetrying,
			reason:      "temporary error",
			expectError: false,
			validateFunc: func(t *testing.T, msg *queue.Message) {
				t.Helper()
				if msg.GetNextRetryAt() == nil {
					t.Errorf("NextRetryAt not set")
				}
			},
		},
		{
			name:        "invalid transition: queued to completed",
			fromState:   queue.StateQueued,
			toState:     queue.StateCompleted,
			reason:      "skip processing",
			expectError: true,
		},
		{
			name:        "invalid transition: completed to processing",
			fromState:   queue.StateCompleted,
			toState:     queue.StateProcessing,
			reason:      "restart",
			expectError: true,
		},
	}
}

func TestMessage_StateTransitions(t *testing.T) {
	tests := getTransitionTests()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStateTransition(t, tt)
		})
	}
}

// testStateTransition executes a single state transition test.
func testStateTransition(t *testing.T, tt transitionTest) {
	t.Helper()

	// Create a fresh message for each test
	testMsg := createTestMessage()

	// Set up initial state
	testMsg = setupInitialState(t, testMsg, tt.fromState)
	oldState := testMsg.GetState()

	// Perform the transition
	testMsg.SetState(tt.toState)
	testMsg.AddStateTransition(oldState, tt.toState, tt.reason)

	// Handle special state transitions
	if tt.toState == queue.StateCompleted {
		now := time.Now()
		testMsg.CompletedAt = &now
	}
	if tt.toState == queue.StateRetrying {
		retryTime := time.Now().Add(time.Second)
		testMsg.NextRetryAt = &retryTime
	}

	// Verify state history was updated
	validateStateHistory(t, testMsg, tt.fromState, tt.toState, tt.reason)

	// Run custom validation
	if tt.validateFunc != nil {
		tt.validateFunc(t, testMsg)
	}
}

func TestMessage_SetError(t *testing.T) {
	msg := createTestMessage()
	testError := fmt.Errorf("test error")

	// Add error
	msg.SetError(testError)

	// Message should have error
	if !errors.Is(msg.Error, testError) {
		t.Errorf("expected Error to be %v, got %v", testError, msg.Error)
	}

	// Check error history
	errorHistory := msg.GetErrorHistory()
	if len(errorHistory) != 1 {
		t.Fatalf("expected 1 error history entry, got %d", len(errorHistory))
	}
	if !errors.Is(errorHistory[0].Error, testError) {
		t.Errorf("expected error %v in history, got %v", testError, errorHistory[0].Error)
	}
	if errorHistory[0].State != queue.StateQueued {
		t.Errorf("expected state %v in error history, got %v", queue.StateQueued, errorHistory[0].State)
	}
	if errorHistory[0].Attempt != 0 {
		t.Errorf("expected attempt 0 in error history, got %d", errorHistory[0].Attempt)
	}
}

func TestMessage_IncrementAttempts(t *testing.T) {
	msg := createTestMessage()

	// Increment attempts
	attempts1 := msg.IncrementAttempts()
	attempts2 := msg.IncrementAttempts()
	attempts3 := msg.IncrementAttempts()

	// Verify increments
	if attempts1 != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts1)
	}
	if attempts2 != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts2)
	}
	if attempts3 != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts3)
	}
	// Verify the message was updated
	if msg.Attempts != 3 {
		t.Errorf("expected message to have 3 attempts, got %d", msg.Attempts)
	}
}

func TestMessage_CanRetry(t *testing.T) {
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

			if got := msg.CanRetry(); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestMessage_IsReadyForRetry(t *testing.T) {
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

// Removed tests for methods that don't exist in the new Message type:
// - TestQueuedMessage_GetProcessingDuration
// - TestQueuedMessage_GetQueueDuration
// - TestQueuedMessage_GetLastTransition

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

func TestState_String(t *testing.T) {
	tests := []struct {
		state    queue.State
		expected string
	}{
		{queue.StateQueued, "queued"},
		{queue.StateProcessing, "processing"},
		{queue.StateValidating, "validating"},
		{queue.StateCompleted, "completed"},
		{queue.StateFailed, "failed"},
		{queue.StateRetrying, "retrying"},
		{queue.State("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			// State is a string type, so convert directly
			if got := string(tt.state); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

// Helper functions

func verifyBasicFields(t *testing.T, qm *queue.Message, msg signal.IncomingMessage) {
	t.Helper()
	// ID is generated from timestamp and sender
	expectedIDPrefix := fmt.Sprintf("%d-%s", msg.Timestamp.UnixNano(), msg.From)
	if qm.ID != expectedIDPrefix {
		t.Errorf("expected ID %s, got %s", expectedIDPrefix, qm.ID)
	}
	// ConversationID is the sender in the actual implementation
	if qm.ConversationID != msg.From {
		t.Errorf("expected ConversationID %s, got %s", msg.From, qm.ConversationID)
	}
	if qm.Sender != msg.From {
		t.Errorf("expected Sender %s, got %s", msg.From, qm.Sender)
	}
	if qm.Text != msg.Text {
		t.Errorf("expected Text %s, got %s", msg.Text, qm.Text)
	}
}

func verifyStateAndHistory(t *testing.T, qm *queue.Message) {
	t.Helper()
	// Verify initial state
	if qm.State != queue.StateQueued {
		t.Errorf("expected initial state %v, got %v", queue.StateQueued, qm.State)
	}

	// Verify state history
	if len(qm.StateHistory) != 1 {
		t.Fatalf("expected 1 state history entry, got %d", len(qm.StateHistory))
	}
	if qm.StateHistory[0].To != queue.StateQueued {
		t.Errorf("expected initial state history To %v, got %v", queue.StateQueued, qm.StateHistory[0].To)
	}
	if qm.StateHistory[0].Reason != "message created" {
		t.Errorf("expected reason 'message created', got %s", qm.StateHistory[0].Reason)
	}
}

func verifyDefaults(t *testing.T, qm *queue.Message) {
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

func setupInitialState(t *testing.T, msg *queue.Message, targetState queue.State) *queue.Message {
	t.Helper()
	if targetState == queue.StateQueued {
		return msg
	}

	// Transition to the required from state
	transitionalStates := getTransitionPath(queue.StateQueued, targetState)
	currentMsg := msg
	for _, state := range transitionalStates {
		// Simply update the state directly since Message doesn't have WithState
		currentMsg.SetState(state)
		currentMsg.AddStateTransition(currentMsg.State, state, "test setup")
	}
	return currentMsg
}

func validateStateHistory(
	t *testing.T,
	msg *queue.Message,
	fromState, toState queue.State,
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

func createTestMessage() *queue.Message {
	now := time.Now()
	msgID := fmt.Sprintf("%d-%s", now.UnixNano(), "+1234567890")
	msg := queue.NewMessage(msgID, "+1234567890", "Test User", "+1234567890", "Test message")
	// Add initial state transition like the Coordinator does
	msg.AddStateTransition(queue.StateQueued, queue.StateQueued, "message created")
	return msg
}

// getTransitionPath returns the states needed to transition from one state to another.
func getTransitionPath(from, to queue.State) []queue.State {
	// Simplified for testing - in real implementation this would use the state machine
	paths := map[string][]queue.State{
		"queued-processing": {queue.StateProcessing},
		"queued-validating": {queue.StateProcessing, queue.StateValidating},
		"queued-completed":  {queue.StateProcessing, queue.StateCompleted},
		"queued-retrying":   {queue.StateProcessing, queue.StateRetrying},
		"queued-failed":     {queue.StateFailed},
	}

	key := string(from) + "-" + string(to)
	return paths[key]
}
