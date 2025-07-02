package queue_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
)

type stateValidatorTestCase struct {
	name    string
	setup   func(*queue.Message)
	from    queue.State
	to      queue.State
	wantErr bool
	errMsg  string
}

func TestStateValidator_ValidateTransition(t *testing.T) {
	validator := queue.NewStateValidator()
	tests := getStateValidatorTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
			msg.SetState(tt.from)

			if tt.setup != nil {
				tt.setup(msg)
			}

			err := validator.ValidateTransition(msg, tt.from, tt.to)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateTransition() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("validateTransition() error = %v, want error containing %v", err, tt.errMsg)
			}
		})
	}
}

func getStateValidatorTestCases() []stateValidatorTestCase {
	var cases []stateValidatorTestCase
	cases = append(cases, getQueuedToProcessingCases()...)
	cases = append(cases, getProcessingToValidatingCases()...)
	cases = append(cases, getProcessingToFailedCases()...)
	cases = append(cases, getProcessingToRetryingCases()...)
	cases = append(cases, getValidatingToCompletedCases()...)
	cases = append(cases, getRetryingTransitionCases()...)
	return cases
}

func getQueuedToProcessingCases() []stateValidatorTestCase {
	return []stateValidatorTestCase{
		// Queued -> Processing validations
		{
			name: "queued to processing with valid message",
			setup: func(_ *queue.Message) {
				// Default message has ID and text
			},
			from:    queue.StateQueued,
			to:      queue.StateProcessing,
			wantErr: false,
		},
		{
			name: "queued to processing without ID",
			setup: func(m *queue.Message) {
				m.ID = ""
			},
			from:    queue.StateQueued,
			to:      queue.StateProcessing,
			wantErr: true,
			errMsg:  "cannot process message without ID",
		},
		{
			name: "queued to processing without text",
			setup: func(m *queue.Message) {
				m.Text = ""
			},
			from:    queue.StateQueued,
			to:      queue.StateProcessing,
			wantErr: true,
			errMsg:  "cannot process empty message",
		},
	}
}

func getProcessingToValidatingCases() []stateValidatorTestCase {
	return []stateValidatorTestCase{
		// Processing -> Validating validations
		{
			name: "processing to validating with response",
			setup: func(m *queue.Message) {
				m.SetResponse("test response")
			},
			from:    queue.StateProcessing,
			to:      queue.StateValidating,
			wantErr: false,
		},
		{
			name: "processing to validating without response",
			setup: func(m *queue.Message) {
				m.Response = ""
			},
			from:    queue.StateProcessing,
			to:      queue.StateValidating,
			wantErr: true,
			errMsg:  "cannot validate without response",
		},
	}
}

func getProcessingToFailedCases() []stateValidatorTestCase {
	return []stateValidatorTestCase{
		// Processing -> Failed validations
		{
			name: "processing to failed with error",
			setup: func(m *queue.Message) {
				m.SetError(fmt.Errorf("processing error"))
			},
			from:    queue.StateProcessing,
			to:      queue.StateFailed,
			wantErr: false,
		},
		{
			name: "processing to failed at max attempts",
			setup: func(m *queue.Message) {
				m.Attempts = 3
				m.MaxAttempts = 3
			},
			from:    queue.StateProcessing,
			to:      queue.StateFailed,
			wantErr: false,
		},
		{
			name: "processing to failed without error when can retry",
			setup: func(m *queue.Message) {
				m.Error = nil
				m.Attempts = 1
				m.MaxAttempts = 3
			},
			from:    queue.StateProcessing,
			to:      queue.StateFailed,
			wantErr: true,
			errMsg:  "cannot fail without error when retries remain",
		},
	}
}

func getProcessingToRetryingCases() []stateValidatorTestCase {
	return []stateValidatorTestCase{
		// Processing -> Retrying validations
		{
			name: "processing to retrying with attempts remaining",
			setup: func(m *queue.Message) {
				m.Attempts = 1
				m.MaxAttempts = 3
			},
			from:    queue.StateProcessing,
			to:      queue.StateRetrying,
			wantErr: false,
		},
		{
			name: "processing to retrying at max attempts",
			setup: func(m *queue.Message) {
				m.Attempts = 3
				m.MaxAttempts = 3
			},
			from:    queue.StateProcessing,
			to:      queue.StateRetrying,
			wantErr: true,
			errMsg:  "cannot retry: maximum attempts (3) exceeded",
		},
	}
}

func getValidatingToCompletedCases() []stateValidatorTestCase {
	return []stateValidatorTestCase{
		// Validating -> Completed validations
		{
			name: "validating to completed with response and timestamp",
			setup: func(m *queue.Message) {
				m.SetResponse("validated response")
			},
			from:    queue.StateValidating,
			to:      queue.StateCompleted,
			wantErr: false,
		},
		{
			name: "validating to completed without response",
			setup: func(m *queue.Message) {
				m.Response = ""
				now := time.Now()
				m.ProcessedAt = &now
			},
			from:    queue.StateValidating,
			to:      queue.StateCompleted,
			wantErr: true,
			errMsg:  "cannot complete without validated response",
		},
		{
			name: "validating to completed without timestamp",
			setup: func(m *queue.Message) {
				m.Response = "test"
				m.ProcessedAt = nil
			},
			from:    queue.StateValidating,
			to:      queue.StateCompleted,
			wantErr: true,
			errMsg:  "cannot complete without processing timestamp",
		},
	}
}

func getRetryingTransitionCases() []stateValidatorTestCase {
	return []stateValidatorTestCase{
		// Retrying -> Processing validations
		{
			name: "retrying to processing with attempts remaining",
			setup: func(m *queue.Message) {
				m.Attempts = 2
				m.MaxAttempts = 3
			},
			from:    queue.StateRetrying,
			to:      queue.StateProcessing,
			wantErr: false,
		},
		{
			name: "retrying to processing at max attempts",
			setup: func(m *queue.Message) {
				m.Attempts = 3
				m.MaxAttempts = 3
			},
			from:    queue.StateRetrying,
			to:      queue.StateProcessing,
			wantErr: true,
			errMsg:  "cannot retry: already attempted 3 times (max: 3)",
		},
	}
}

func TestStateValidator_ExplainInvalidTransition(t *testing.T) {
	validator := queue.NewStateValidator()

	tests := []struct {
		name         string
		from         queue.State
		to           queue.State
		wantContains string
	}{
		// Terminal state explanations
		{
			name:         "from completed",
			from:         queue.StateCompleted,
			to:           queue.StateProcessing,
			wantContains: "message processing is already complete",
		},
		{
			name:         "from failed",
			from:         queue.StateFailed,
			to:           queue.StateProcessing,
			wantContains: "permanently failed and cannot be reprocessed",
		},

		// Invalid target state explanations
		{
			name:         "to queued",
			from:         queue.StateProcessing,
			to:           queue.StateQueued,
			wantContains: "cannot be re-queued once processing has started",
		},
		{
			name:         "validating to processing",
			from:         queue.StateValidating,
			to:           queue.StateProcessing,
			wantContains: "validation must complete before reprocessing",
		},
		{
			name:         "queued to validating",
			from:         queue.StateQueued,
			to:           queue.StateValidating,
			wantContains: "validation can only follow successful processing",
		},
		{
			name:         "queued to completed",
			from:         queue.StateQueued,
			to:           queue.StateCompleted,
			wantContains: "can only be completed after successful validation",
		},
		{
			name:         "queued to failed",
			from:         queue.StateQueued,
			to:           queue.StateFailed,
			wantContains: "must be processed before they can fail",
		},
		{
			name:         "queued to retrying",
			from:         queue.StateQueued,
			to:           queue.StateRetrying,
			wantContains: "only failed processing or validation can be retried",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explanation := validator.ExplainInvalidTransition(tt.from, tt.to)

			if !strings.Contains(explanation, tt.wantContains) {
				t.Errorf("explainInvalidTransition(%s, %s) = %v, want containing %v",
					tt.from, tt.to, explanation, tt.wantContains)
			}
		})
	}
}

func TestStateMachine_EnhancedValidation(t *testing.T) {
	sm := queue.NewStateMachine()

	t.Run("invalid transition with detailed error", func(t *testing.T) {
		msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
		msg.SetState(queue.StateCompleted)

		err := sm.Transition(msg, queue.StateProcessing)
		if err == nil {
			t.Fatal("Expected error for invalid transition")
		}

		// Should contain detailed explanation
		if !strings.Contains(err.Error(), "message processing is already complete") {
			t.Errorf("Expected detailed error explanation, got: %v", err)
		}
	})

	t.Run("business rule validation failure", func(t *testing.T) {
		msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
		msg.SetState(queue.StateProcessing)
		// No response set

		err := sm.Transition(msg, queue.StateValidating)
		if err == nil {
			t.Fatal("Expected error for missing response")
		}

		if !strings.Contains(err.Error(), "cannot validate without response") {
			t.Errorf("Expected business rule error, got: %v", err)
		}
	})

	t.Run("successful transition with business rules", func(t *testing.T) {
		msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
		msg.SetState(queue.StateProcessing)
		msg.SetResponse("test response")

		err := sm.Transition(msg, queue.StateValidating)
		if err != nil {
			t.Errorf("Expected successful transition, got error: %v", err)
		}

		if msg.GetState() != queue.StateValidating {
			t.Errorf("Expected state %s, got %s", queue.StateValidating, msg.GetState())
		}
	})

	t.Run("retry validation with max attempts", func(t *testing.T) {
		msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
		msg.SetState(queue.StateProcessing)
		msg.Attempts = 3
		msg.MaxAttempts = 3

		// Should not allow retry
		err := sm.Transition(msg, queue.StateRetrying)
		if err == nil {
			t.Fatal("Expected error when max attempts reached")
		}

		if !strings.Contains(err.Error(), "maximum attempts (3) exceeded") {
			t.Errorf("Expected max attempts error, got: %v", err)
		}
	})
}

func executeTransition(t *testing.T, sm queue.StateMachine, msg *queue.Message, targetState queue.State) {
	t.Helper()
	err := sm.Transition(msg, targetState)
	if err != nil {
		t.Fatalf("Failed to transition to %s: %v", targetState, err)
	}
}

func verifyState(t *testing.T, msg *queue.Message, expectedState queue.State) {
	t.Helper()
	if msg.GetState() != expectedState {
		t.Errorf("Expected state %s, got %s", expectedState, msg.GetState())
	}
}

func verifyTransitionError(
	t *testing.T,
	sm queue.StateMachine,
	msg *queue.Message,
	targetState queue.State,
	expectedError string,
) {
	t.Helper()
	err := sm.Transition(msg, targetState)
	if err == nil {
		t.Fatal("Expected error but got nil")
	}
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing %q, got: %v", expectedError, err)
	}
}

func TestStateMachine_CompleteWorkflow(t *testing.T) {
	sm := queue.NewStateMachine()

	t.Run("successful message flow", func(t *testing.T) {
		msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")

		// Execute successful flow
		executeTransition(t, sm, msg, queue.StateProcessing)
		msg.SetResponse("AI response")
		executeTransition(t, sm, msg, queue.StateValidating)
		executeTransition(t, sm, msg, queue.StateCompleted)

		// Verify history
		history := msg.GetStateHistory()
		if len(history) != 3 {
			t.Fatalf("Expected 3 history entries, got %d", len(history))
		}

		// Verify terminal state
		verifyTransitionError(t, sm, msg, queue.StateProcessing, "message processing is already complete")
	})

	t.Run("retry workflow", func(t *testing.T) {
		msg := queue.NewMessage("test-id", "conv-1", "sender", "+1234567890", "test message")
		msg.MaxAttempts = 2
		msg.Attempts = 0

		// First attempt
		executeTransition(t, sm, msg, queue.StateProcessing)
		msg.SetError(fmt.Errorf("processing failed"))
		msg.IncrementAttempts()

		// Should go to retrying
		executeTransition(t, sm, msg, queue.StateFailed)
		verifyState(t, msg, queue.StateRetrying)

		// Retry
		executeTransition(t, sm, msg, queue.StateProcessing)
		if msg.Attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", msg.Attempts)
		}

		// Final failure
		executeTransition(t, sm, msg, queue.StateFailed)
		verifyState(t, msg, queue.StateFailed)
	})
}
