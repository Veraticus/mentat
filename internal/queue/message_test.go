package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"

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
		priority Priority
		validate func(t *testing.T, qm *QueuedMessage)
	}{
		{
			name:     "normal priority message",
			priority: PriorityNormal,
			validate: func(t *testing.T, qm *QueuedMessage) {
				t.Helper()
				if qm.Priority != PriorityNormal {
					t.Errorf("expected priority %v, got %v", PriorityNormal, qm.Priority)
				}
			},
		},
		{
			name:     "high priority message",
			priority: PriorityHigh,
			validate: func(t *testing.T, qm *QueuedMessage) {
				t.Helper()
				if qm.Priority != PriorityHigh {
					t.Errorf("expected priority %v, got %v", PriorityHigh, qm.Priority)
				}
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qm := NewQueuedMessage(msg, tt.priority)
			
			// Verify basic fields
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
			
			// Verify initial state
			if qm.State != MessageStateQueued {
				t.Errorf("expected initial state %v, got %v", MessageStateQueued, qm.State)
			}
			
			// Verify state history
			if len(qm.StateHistory) != 1 {
				t.Fatalf("expected 1 state history entry, got %d", len(qm.StateHistory))
			}
			if qm.StateHistory[0].To != MessageStateQueued {
				t.Errorf("expected initial state history To %v, got %v", MessageStateQueued, qm.StateHistory[0].To)
			}
			if qm.StateHistory[0].Reason != "message created" {
				t.Errorf("expected reason 'message created', got %s", qm.StateHistory[0].Reason)
			}
			
			// Verify defaults
			if qm.Attempts != 0 {
				t.Errorf("expected 0 attempts, got %d", qm.Attempts)
			}
			if qm.MaxAttempts != 3 {
				t.Errorf("expected 3 max attempts, got %d", qm.MaxAttempts)
			}
			if len(qm.ErrorHistory) != 0 {
				t.Errorf("expected empty error history, got %d entries", len(qm.ErrorHistory))
			}
			
			// Run custom validation
			tt.validate(t, qm)
		})
	}
}

func TestQueuedMessage_WithState(t *testing.T) {
	msg := createTestMessage()
	
	tests := []struct {
		name          string
		fromState     MessageState
		toState       MessageState
		reason        string
		expectError   bool
		validateFunc  func(t *testing.T, oldMsg, newMsg *QueuedMessage)
	}{
		{
			name:        "valid transition: queued to processing",
			fromState:   MessageStateQueued,
			toState:     MessageStateProcessing,
			reason:      "worker picked up",
			expectError: false,
			validateFunc: func(t *testing.T, oldMsg, newMsg *QueuedMessage) {
				t.Helper()
				// Old message should be unchanged
				if oldMsg.State != MessageStateQueued {
					t.Errorf("old message state changed")
				}
				
				// New message should have new state
				if newMsg.State != MessageStateProcessing {
					t.Errorf("expected state %v, got %v", MessageStateProcessing, newMsg.State)
				}
				
				// ProcessedAt should be set
				if newMsg.ProcessedAt == nil {
					t.Errorf("ProcessedAt not set")
				}
			},
		},
		{
			name:        "valid transition: processing to validating",
			fromState:   MessageStateProcessing,
			toState:     MessageStateValidating,
			reason:      "response generated",
			expectError: false,
		},
		{
			name:        "valid transition: validating to completed",
			fromState:   MessageStateValidating,
			toState:     MessageStateCompleted,
			reason:      "validation passed",
			expectError: false,
			validateFunc: func(t *testing.T, _, newMsg *QueuedMessage) {
				t.Helper()
				if newMsg.CompletedAt == nil {
					t.Errorf("CompletedAt not set")
				}
			},
		},
		{
			name:        "valid transition: processing to retrying",
			fromState:   MessageStateProcessing,
			toState:     MessageStateRetrying,
			reason:      "temporary error",
			expectError: false,
			validateFunc: func(t *testing.T, _, newMsg *QueuedMessage) {
				t.Helper()
				if newMsg.NextRetryAt == nil {
					t.Errorf("NextRetryAt not set")
				}
			},
		},
		{
			name:        "invalid transition: queued to completed",
			fromState:   MessageStateQueued,
			toState:     MessageStateCompleted,
			reason:      "skip processing",
			expectError: true,
		},
		{
			name:        "invalid transition: completed to processing",
			fromState:   MessageStateCompleted,
			toState:     MessageStateProcessing,
			reason:      "restart",
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			oldMsg := msg
			if tt.fromState != MessageStateQueued {
				// Transition to the required from state
				transitionalStates := getTransitionPath(MessageStateQueued, tt.fromState)
				for _, state := range transitionalStates {
					var err error
					oldMsg, err = oldMsg.WithState(state, "test setup")
					if err != nil {
						t.Fatalf("failed to set up initial state: %v", err)
					}
				}
			}
			
			// Perform the transition
			newMsg, err := oldMsg.WithState(tt.toState, tt.reason)
			
			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			
			if err != nil {
				return
			}
			
			// Verify state history was updated
			history := newMsg.GetStateHistory()
			lastTransition := history[len(history)-1]
			if lastTransition.From != tt.fromState {
				t.Errorf("expected transition from %v, got %v", tt.fromState, lastTransition.From)
			}
			if lastTransition.To != tt.toState {
				t.Errorf("expected transition to %v, got %v", tt.toState, lastTransition.To)
			}
			if lastTransition.Reason != tt.reason {
				t.Errorf("expected reason %s, got %s", tt.reason, lastTransition.Reason)
			}
			
			// Run custom validation
			if tt.validateFunc != nil {
				tt.validateFunc(t, oldMsg, newMsg)
			}
		})
	}
}

func TestQueuedMessage_WithError(t *testing.T) {
	msg := createTestMessage()
	testError := errors.New("test error")
	
	// Add error
	newMsg := msg.WithError(testError, MessageStateProcessing)
	
	// Original should be unchanged
	if msg.LastError != nil {
		t.Errorf("original message error was modified")
	}
	if len(msg.ErrorHistory) != 0 {
		t.Errorf("original message error history was modified")
	}
	
	// New message should have error
	if newMsg.LastError != testError {
		t.Errorf("expected LastError to be %v, got %v", testError, newMsg.LastError)
	}
	
	// Check error history
	errorHistory := newMsg.GetErrorHistory()
	if len(errorHistory) != 1 {
		t.Fatalf("expected 1 error history entry, got %d", len(errorHistory))
	}
	if errorHistory[0].Error != testError {
		t.Errorf("expected error %v in history, got %v", testError, errorHistory[0].Error)
	}
	if errorHistory[0].State != MessageStateProcessing {
		t.Errorf("expected state %v in error history, got %v", MessageStateProcessing, errorHistory[0].State)
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
	if last.To != MessageStateQueued {
		t.Errorf("expected last transition to %v, got %v", MessageStateQueued, last.To)
	}
	
	// After state change
	newMsg, _ := msg.WithState(MessageStateProcessing, "test")
	last = newMsg.GetLastTransition()
	if last.From != MessageStateQueued || last.To != MessageStateProcessing {
		t.Errorf("expected transition from %v to %v, got from %v to %v",
			MessageStateQueued, MessageStateProcessing, last.From, last.To)
	}
}

func TestCalculateRetryDelay(t *testing.T) {
	tests := []struct {
		name        string
		attempts    int
		minDelay    time.Duration
		maxDelay    time.Duration
	}{
		{"first retry", 0, 900 * time.Millisecond, 1100 * time.Millisecond},
		{"second retry", 1, 1800 * time.Millisecond, 2200 * time.Millisecond},
		{"third retry", 2, 3600 * time.Millisecond, 4400 * time.Millisecond},
		{"max delay", 10, 4*time.Minute + 30*time.Second, 5*time.Minute},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := CalculateRetryDelay(tt.attempts)
			if delay < tt.minDelay || delay > tt.maxDelay {
				t.Errorf("expected delay between %v and %v, got %v", tt.minDelay, tt.maxDelay, delay)
			}
		})
	}
}

func TestMessageStateString(t *testing.T) {
	tests := []struct {
		state    MessageState
		expected string
	}{
		{MessageStateQueued, "queued"},
		{MessageStateProcessing, "processing"},
		{MessageStateValidating, "validating"},
		{MessageStateCompleted, "completed"},
		{MessageStateFailed, "failed"},
		{MessageStateRetrying, "retrying"},
		{MessageState(99), "unknown(99)"},
	}
	
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := MessageStateString(tt.state); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

// Helper functions

func createTestMessage() *QueuedMessage {
	return NewQueuedMessage(signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Test message",
		Timestamp: time.Now(),
	}, PriorityNormal)
}

// getTransitionPath returns the states needed to transition from one state to another.
func getTransitionPath(from, to MessageState) []MessageState {
	// Simplified for testing - in real implementation this would use the state machine
	paths := map[string][]MessageState{
		"queued-processing":  {MessageStateProcessing},
		"queued-validating":  {MessageStateProcessing, MessageStateValidating},
		"queued-completed":   {MessageStateProcessing, MessageStateCompleted},
		"queued-retrying":    {MessageStateProcessing, MessageStateRetrying},
		"queued-failed":      {MessageStateFailed},
	}
	
	key := MessageStateString(from) + "-" + MessageStateString(to)
	return paths[key]
}