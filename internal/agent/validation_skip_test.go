package agent_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// TestValidationSkipping tests that validation is skipped when NeedsValidation is false.
func TestValidationSkipping(t *testing.T) {
	ctx := context.Background()

	// Track validation calls
	validationCalled := &atomic.Bool{}

	// Create mock components
	mockLLM := &mockLLM{}
	mockMessenger := &mockMessenger{}
	mockSessionMgr := newMockSessionManager()

	// Create a validation strategy that tracks if it was called
	mockStrategy := &mockValidationStrategy{
		result: agent.ValidationResult{
			Status:     agent.ValidationStatusSuccess,
			Confidence: 0.9,
		},
	}

	// Wrap the validation strategy to track calls
	wrappedStrategy := &trackingValidationStrategy{
		base: mockStrategy,
		onCalled: func() {
			validationCalled.Store(true)
		},
	}

	handler, err := agent.NewHandler(mockLLM,
		agent.WithValidationStrategy(wrappedStrategy),
		agent.WithMessenger(mockMessenger),
		agent.WithSessionManager(mockSessionMgr),
	)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	tests := []struct {
		name             string
		message          string
		progressInfo     *claude.ProgressInfo
		expectValidation bool
		expectedResponse string
	}{
		{
			name:    "simple greeting skips validation",
			message: "Hi!",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   false,
			},
			expectValidation: false,
			expectedResponse: "Hello! How can I help you today?",
		},
		{
			name:    "complex query triggers validation",
			message: "Find all my meetings tomorrow and email the participants",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   true,
			},
			expectValidation: true,
			expectedResponse: "I'll help you find your meetings and email participants.",
		},
		{
			name:             "nil progress info triggers validation",
			message:          "What's the weather?",
			progressInfo:     nil,
			expectValidation: true,
			expectedResponse: "Let me check the weather for you.",
		},
		{
			name:    "another simple query skips validation",
			message: "Thanks!",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   false,
			},
			expectValidation: false,
			expectedResponse: "You're welcome!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset validation tracking
			validationCalled.Store(false)

			// Configure mock LLM response
			mockLLM.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				return &claude.LLMResponse{
					Message:  tt.expectedResponse,
					Progress: tt.progressInfo,
					Metadata: claude.ResponseMetadata{
						ModelVersion: "claude-3",
						Latency:      100 * time.Millisecond,
					},
				}, nil
			}

			// Process the message
			msg := signal.IncomingMessage{
				From: "+1234567890",
				Text: tt.message,
			}

			processErr := handler.Process(ctx, msg)
			if processErr != nil {
				t.Fatalf("Process failed: %v", processErr)
			}

			// Give async validation time to start (if it's going to)
			time.Sleep(100 * time.Millisecond)

			// Verify validation was called or not as expected
			if tt.expectValidation != validationCalled.Load() {
				t.Errorf("Expected validation called = %v, but got %v",
					tt.expectValidation, validationCalled.Load())
			}

			// Verify response was sent
			if len(mockMessenger.sentMessages) != 1 {
				t.Fatalf("Expected 1 message sent, got %d", len(mockMessenger.sentMessages))
			}

			if mockMessenger.sentMessages[0].message != tt.expectedResponse {
				t.Errorf("Expected response %q, got %q",
					tt.expectedResponse, mockMessenger.sentMessages[0].message)
			}

			// Clear sent messages for next test
			mockMessenger.sentMessages = nil
		})
	}
}

// TestResponseTimeForSimpleQueries verifies simple queries complete quickly.
func TestResponseTimeForSimpleQueries(t *testing.T) {
	ctx := context.Background()

	// Create mock components
	mockLLM := &mockLLM{}
	mockMessenger := &mockMessenger{}
	mockSessionMgr := newMockSessionManager()
	mockStrategy := &mockValidationStrategy{
		result: agent.ValidationResult{
			Status: agent.ValidationStatusSuccess,
		},
	}

	handler, err := agent.NewHandler(mockLLM,
		agent.WithValidationStrategy(mockStrategy),
		agent.WithMessenger(mockMessenger),
		agent.WithSessionManager(mockSessionMgr),
	)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Configure LLM to return a simple response with no validation needed
	mockLLM.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
		// Simulate a realistic LLM response time
		time.Sleep(500 * time.Millisecond)

		return &claude.LLMResponse{
			Message: "Hello!",
			Progress: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   false,
			},
			Metadata: claude.ResponseMetadata{
				ModelVersion: "claude-3",
				Latency:      500 * time.Millisecond,
			},
		}, nil
	}

	// Test a simple greeting
	msg := signal.IncomingMessage{
		From: "+1234567890",
		Text: "Hi",
	}

	start := time.Now()
	err = handler.Process(ctx, msg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Verify response time is under 3 seconds
	if elapsed > 3*time.Second {
		t.Errorf("Simple query took too long: %v (expected < 3s)", elapsed)
	}

	// Verify response was sent
	if len(mockMessenger.sentMessages) != 1 {
		t.Fatalf("Expected 1 message sent, got %d", len(mockMessenger.sentMessages))
	}

	t.Logf("Simple query completed in %v", elapsed)
}
