package queue

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestStateValidator_ValidateTransition(t *testing.T) {
	validator := newStateValidator()
	
	tests := []struct {
		name    string
		setup   func(*Message)
		from    State
		to      State
		wantErr bool
		errMsg  string
	}{
		// Queued -> Processing validations
		{
			name: "queued to processing with valid message",
			setup: func(_ *Message) {
				// Default message has ID and text
			},
			from:    StateQueued,
			to:      StateProcessing,
			wantErr: false,
		},
		{
			name: "queued to processing without ID",
			setup: func(m *Message) {
				m.ID = ""
			},
			from:    StateQueued,
			to:      StateProcessing,
			wantErr: true,
			errMsg:  "cannot process message without ID",
		},
		{
			name: "queued to processing without text",
			setup: func(m *Message) {
				m.Text = ""
			},
			from:    StateQueued,
			to:      StateProcessing,
			wantErr: true,
			errMsg:  "cannot process empty message",
		},
		
		// Processing -> Validating validations
		{
			name: "processing to validating with response",
			setup: func(m *Message) {
				m.SetResponse("test response")
			},
			from:    StateProcessing,
			to:      StateValidating,
			wantErr: false,
		},
		{
			name: "processing to validating without response",
			setup: func(m *Message) {
				m.Response = ""
			},
			from:    StateProcessing,
			to:      StateValidating,
			wantErr: true,
			errMsg:  "cannot validate without response",
		},
		
		// Processing -> Failed validations
		{
			name: "processing to failed with error",
			setup: func(m *Message) {
				m.SetError(fmt.Errorf("processing error"))
			},
			from:    StateProcessing,
			to:      StateFailed,
			wantErr: false,
		},
		{
			name: "processing to failed at max attempts",
			setup: func(m *Message) {
				m.Attempts = 3
				m.MaxAttempts = 3
			},
			from:    StateProcessing,
			to:      StateFailed,
			wantErr: false,
		},
		{
			name: "processing to failed without error when can retry",
			setup: func(m *Message) {
				m.Error = nil
				m.Attempts = 1
				m.MaxAttempts = 3
			},
			from:    StateProcessing,
			to:      StateFailed,
			wantErr: true,
			errMsg:  "cannot fail without error when retries remain",
		},
		
		// Processing -> Retrying validations
		{
			name: "processing to retrying with attempts remaining",
			setup: func(m *Message) {
				m.Attempts = 1
				m.MaxAttempts = 3
			},
			from:    StateProcessing,
			to:      StateRetrying,
			wantErr: false,
		},
		{
			name: "processing to retrying at max attempts",
			setup: func(m *Message) {
				m.Attempts = 3
				m.MaxAttempts = 3
			},
			from:    StateProcessing,
			to:      StateRetrying,
			wantErr: true,
			errMsg:  "cannot retry: maximum attempts (3) exceeded",
		},
		
		// Validating -> Completed validations
		{
			name: "validating to completed with response and timestamp",
			setup: func(m *Message) {
				m.SetResponse("validated response")
			},
			from:    StateValidating,
			to:      StateCompleted,
			wantErr: false,
		},
		{
			name: "validating to completed without response",
			setup: func(m *Message) {
				m.Response = ""
				now := time.Now()
				m.ProcessedAt = &now
			},
			from:    StateValidating,
			to:      StateCompleted,
			wantErr: true,
			errMsg:  "cannot complete without validated response",
		},
		{
			name: "validating to completed without timestamp",
			setup: func(m *Message) {
				m.Response = "test"
				m.ProcessedAt = nil
			},
			from:    StateValidating,
			to:      StateCompleted,
			wantErr: true,
			errMsg:  "cannot complete without processing timestamp",
		},
		
		// Retrying -> Processing validations
		{
			name: "retrying to processing with attempts remaining",
			setup: func(m *Message) {
				m.Attempts = 2
				m.MaxAttempts = 3
			},
			from:    StateRetrying,
			to:      StateProcessing,
			wantErr: false,
		},
		{
			name: "retrying to processing at max attempts",
			setup: func(m *Message) {
				m.Attempts = 3
				m.MaxAttempts = 3
			},
			from:    StateRetrying,
			to:      StateProcessing,
			wantErr: true,
			errMsg:  "cannot retry: already attempted 3 times (max: 3)",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewMessage("test-id", "conv-1", "sender", "test message")
			msg.SetState(tt.from)
			
			if tt.setup != nil {
				tt.setup(msg)
			}
			
			err := validator.validateTransition(msg, tt.from, tt.to)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
			
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("validateTransition() error = %v, want error containing %v", err, tt.errMsg)
			}
		})
	}
}

func TestStateValidator_ExplainInvalidTransition(t *testing.T) {
	validator := newStateValidator()
	
	tests := []struct {
		name         string
		from         State
		to           State
		wantContains string
	}{
		// Terminal state explanations
		{
			name:         "from completed",
			from:         StateCompleted,
			to:           StateProcessing,
			wantContains: "message processing is already complete",
		},
		{
			name:         "from failed",
			from:         StateFailed,
			to:           StateProcessing,
			wantContains: "permanently failed and cannot be reprocessed",
		},
		
		// Invalid target state explanations
		{
			name:         "to queued",
			from:         StateProcessing,
			to:           StateQueued,
			wantContains: "cannot be re-queued once processing has started",
		},
		{
			name:         "validating to processing",
			from:         StateValidating,
			to:           StateProcessing,
			wantContains: "validation must complete before reprocessing",
		},
		{
			name:         "queued to validating",
			from:         StateQueued,
			to:           StateValidating,
			wantContains: "validation can only follow successful processing",
		},
		{
			name:         "queued to completed",
			from:         StateQueued,
			to:           StateCompleted,
			wantContains: "can only be completed after successful validation",
		},
		{
			name:         "queued to failed",
			from:         StateQueued,
			to:           StateFailed,
			wantContains: "must be processed before they can fail",
		},
		{
			name:         "queued to retrying",
			from:         StateQueued,
			to:           StateRetrying,
			wantContains: "only failed processing or validation can be retried",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explanation := validator.explainInvalidTransition(tt.from, tt.to)
			
			if !strings.Contains(explanation, tt.wantContains) {
				t.Errorf("explainInvalidTransition(%s, %s) = %v, want containing %v", 
					tt.from, tt.to, explanation, tt.wantContains)
			}
		})
	}
}

func TestStateMachine_EnhancedValidation(t *testing.T) {
	sm := NewStateMachine()
	
	t.Run("invalid transition with detailed error", func(t *testing.T) {
		msg := NewMessage("test-id", "conv-1", "sender", "test message")
		msg.SetState(StateCompleted)
		
		err := sm.Transition(msg, StateProcessing)
		if err == nil {
			t.Fatal("Expected error for invalid transition")
		}
		
		// Should contain detailed explanation
		if !strings.Contains(err.Error(), "message processing is already complete") {
			t.Errorf("Expected detailed error explanation, got: %v", err)
		}
	})
	
	t.Run("business rule validation failure", func(t *testing.T) {
		msg := NewMessage("test-id", "conv-1", "sender", "test message")
		msg.SetState(StateProcessing)
		// No response set
		
		err := sm.Transition(msg, StateValidating)
		if err == nil {
			t.Fatal("Expected error for missing response")
		}
		
		if !strings.Contains(err.Error(), "cannot validate without response") {
			t.Errorf("Expected business rule error, got: %v", err)
		}
	})
	
	t.Run("successful transition with business rules", func(t *testing.T) {
		msg := NewMessage("test-id", "conv-1", "sender", "test message")
		msg.SetState(StateProcessing)
		msg.SetResponse("test response")
		
		err := sm.Transition(msg, StateValidating)
		if err != nil {
			t.Errorf("Expected successful transition, got error: %v", err)
		}
		
		if msg.GetState() != StateValidating {
			t.Errorf("Expected state %s, got %s", StateValidating, msg.GetState())
		}
	})
	
	t.Run("retry validation with max attempts", func(t *testing.T) {
		msg := NewMessage("test-id", "conv-1", "sender", "test message")
		msg.SetState(StateProcessing)
		msg.Attempts = 3
		msg.MaxAttempts = 3
		
		// Should not allow retry
		err := sm.Transition(msg, StateRetrying)
		if err == nil {
			t.Fatal("Expected error when max attempts reached")
		}
		
		if !strings.Contains(err.Error(), "maximum attempts (3) exceeded") {
			t.Errorf("Expected max attempts error, got: %v", err)
		}
	})
}

func TestStateMachine_CompleteWorkflow(t *testing.T) {
	sm := NewStateMachine()
	
	t.Run("successful message flow", func(t *testing.T) {
		msg := NewMessage("test-id", "conv-1", "sender", "test message")
		
		// Queued -> Processing
		err := sm.Transition(msg, StateProcessing)
		if err != nil {
			t.Fatalf("Failed to transition to processing: %v", err)
		}
		
		// Set response for validation
		msg.SetResponse("AI response")
		
		// Processing -> Validating
		err = sm.Transition(msg, StateValidating)
		if err != nil {
			t.Fatalf("Failed to transition to validating: %v", err)
		}
		
		// Validating -> Completed
		err = sm.Transition(msg, StateCompleted)
		if err != nil {
			t.Fatalf("Failed to transition to completed: %v", err)
		}
		
		// Verify state history has proper reasons
		history := msg.GetStateHistory()
		if len(history) != 3 {
			t.Fatalf("Expected 3 history entries, got %d", len(history))
		}
		
		// Verify cannot transition from completed
		err = sm.Transition(msg, StateProcessing)
		if err == nil {
			t.Fatal("Expected error transitioning from completed state")
		}
		
		if !strings.Contains(err.Error(), "message processing is already complete") {
			t.Errorf("Expected terminal state error, got: %v", err)
		}
	})
	
	t.Run("retry workflow", func(t *testing.T) {
		msg := NewMessage("test-id", "conv-1", "sender", "test message")
		msg.MaxAttempts = 2
		msg.Attempts = 0 // Start with 0 attempts
		
		// Queued -> Processing
		err := sm.Transition(msg, StateProcessing)
		if err != nil {
			t.Fatalf("Failed to transition to processing: %v", err)
		}
		
		// Simulate failure - this is attempt 1
		msg.SetError(fmt.Errorf("processing failed"))
		msg.IncrementAttempts() // Now attempts = 1
		
		// Processing -> Failed (should go to Retrying since we have attempts left)
		err = sm.Transition(msg, StateFailed)
		if err != nil {
			t.Fatalf("Failed to transition: %v", err)
		}
		
		// Should be in retrying state
		if msg.GetState() != StateRetrying {
			t.Errorf("Expected state %s, got %s", StateRetrying, msg.GetState())
		}
		
		// Retrying -> Processing
		err = sm.Transition(msg, StateProcessing)
		if err != nil {
			t.Fatalf("Failed to retry: %v", err)
		}
		
		// Attempts should have incremented (from transition logic)
		if msg.Attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", msg.Attempts)
		}
		
		// Fail again - now we're at max attempts, should go to failed permanently
		err = sm.Transition(msg, StateFailed)
		if err != nil {
			t.Fatalf("Failed to transition to failed: %v", err)
		}
		
		// Should be permanently failed (no more retries)
		if msg.GetState() != StateFailed {
			t.Errorf("Expected state %s, got %s", StateFailed, msg.GetState())
		}
	})
}