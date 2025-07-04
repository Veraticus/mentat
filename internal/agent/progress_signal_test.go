package agent_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// mockSignalMessenger implements signal.Messenger for testing.
type mockSignalMessenger struct {
	mu        sync.Mutex
	sendCalls []sendCall
	sendFunc  func(ctx context.Context, recipient, message string) error
}

type sendCall struct {
	ctx       context.Context
	recipient string
	message   string
}

func (m *mockSignalMessenger) Send(ctx context.Context, recipient, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sendCalls = append(m.sendCalls, sendCall{
		ctx:       ctx,
		recipient: recipient,
		message:   message,
	})

	if m.sendFunc != nil {
		return m.sendFunc(ctx, recipient, message)
	}
	return nil
}

func (m *mockSignalMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

func (m *mockSignalMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}

func (m *mockSignalMessenger) getSendCalls() []sendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]sendCall, len(m.sendCalls))
	copy(calls, m.sendCalls)
	return calls
}

// mockProgressLLM implements claude.LLM for testing.
type mockProgressLLM struct {
	mu         sync.Mutex
	queryCalls []queryCall
	queryFunc  func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error)
}

type queryCall struct {
	ctx       context.Context
	prompt    string
	sessionID string
}

func (m *mockProgressLLM) Query(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queryCalls = append(m.queryCalls, queryCall{
		ctx:       ctx,
		prompt:    prompt,
		sessionID: sessionID,
	})

	if m.queryFunc != nil {
		return m.queryFunc(ctx, prompt, sessionID)
	}
	return &claude.LLMResponse{Message: "default response"}, nil
}

func (m *mockProgressLLM) getQueryCalls() []queryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]queryCall, len(m.queryCalls))
	copy(calls, m.queryCalls)
	return calls
}

func TestNewSignalProgressReporter(t *testing.T) {
	messenger := &mockSignalMessenger{}
	llm := &mockProgressLLM{}
	senderID := "+1234567890"

	// Test basic creation
	reporter := agent.NewSignalProgressReporter(messenger, llm, senderID)
	if reporter == nil {
		t.Fatal("NewSignalProgressReporter returned nil")
	}

	// Test with custom ErrorRecovery
	customRecovery := agent.NewErrorRecovery()
	reporter2 := agent.NewSignalProgressReporter(
		messenger,
		llm,
		senderID,
		agent.WithErrorRecovery(customRecovery),
	)
	if reporter2 == nil {
		t.Fatal("NewSignalProgressReporter with options returned nil")
	}
}

func TestSignalProgressReporterReportError(t *testing.T) {
	tests := []struct {
		name                string
		err                 error
		userFriendlyMessage string
		llmResponse         *claude.LLMResponse
		llmError            error
		messengerError      error
		expectedMessage     string
		expectLLMCall       bool
	}{
		{
			name:                "user friendly message provided",
			err:                 fmt.Errorf("internal error"),
			userFriendlyMessage: "Custom error message",
			expectedMessage:     "Custom error message",
			expectLLMCall:       false,
		},
		{
			name:            "llm generates message successfully",
			err:             fmt.Errorf("network error"),
			llmResponse:     &claude.LLMResponse{Message: "I'm having trouble connecting. Please check your internet."},
			expectedMessage: "I'm having trouble connecting. Please check your internet.",
			expectLLMCall:   true,
		},
		{
			name:            "llm fails, falls back to default",
			err:             &claude.AuthenticationError{Message: "authentication failed"},
			llmError:        fmt.Errorf("llm timeout"),
			expectedMessage: "I need to be authenticated to help you. Please ask your administrator to run the authentication command.",
			expectLLMCall:   true,
		},
		{
			name:            "nil llm, uses default message",
			err:             fmt.Errorf("rate limit exceeded"),
			expectedMessage: "I'm currently experiencing high demand. Please try again in a few moments.",
			expectLLMCall:   false,
		},
		{
			name:            "nil error does nothing",
			err:             nil,
			expectedMessage: "",
			expectLLMCall:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var messenger *mockSignalMessenger
			var llm claude.LLM // Changed to interface type

			if tt.err != nil {
				messenger = &mockSignalMessenger{
					sendFunc: func(_ context.Context, _, message string) error {
						if message != tt.expectedMessage {
							t.Errorf("Expected message %q, got %q", tt.expectedMessage, message)
						}
						return tt.messengerError
					},
				}
			} else {
				messenger = &mockSignalMessenger{}
			}

			if tt.name != "nil llm, uses default message" {
				llm = &mockProgressLLM{
					queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
						return tt.llmResponse, tt.llmError
					},
				}
			} else {
				llm = nil // This is now a nil interface
			}

			senderID := "+1234567890"
			reporter := agent.NewSignalProgressReporter(messenger, llm, senderID)

			ctx := context.Background()
			err := reporter.ReportError(ctx, tt.err, tt.userFriendlyMessage)

			// Check error handling
			if tt.messengerError != nil {
				if err == nil || !errors.Is(err, tt.messengerError) {
					t.Errorf("Expected messenger error, got %v", err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check if message was sent (unless error was nil)
			calls := messenger.getSendCalls()
			if tt.err != nil {
				if len(calls) != 1 {
					t.Errorf("Expected 1 send call, got %d", len(calls))
				} else if calls[0].recipient != senderID {
					t.Errorf("Expected recipient %s, got %s", senderID, calls[0].recipient)
				}
			} else {
				if len(calls) != 0 {
					t.Errorf("Expected no send calls for nil error, got %d", len(calls))
				}
			}

			// Check LLM usage
			if llm != nil {
				// Type assert to access mock methods
				if mockLLM, ok := llm.(*mockProgressLLM); ok {
					llmCalls := mockLLM.getQueryCalls()
					if tt.expectLLMCall && len(llmCalls) != 1 {
						t.Errorf("Expected 1 LLM call, got %d", len(llmCalls))
					} else if !tt.expectLLMCall && len(llmCalls) != 0 {
						t.Errorf("Expected no LLM calls, got %d", len(llmCalls))
					}
				}
			}
		})
	}
}

func TestSignalProgressReporterShouldContinue(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedResult bool
	}{
		{
			name:           "nil error continues",
			err:            nil,
			expectedResult: true,
		},
		{
			name:           "canceled error stops",
			err:            context.Canceled,
			expectedResult: false,
		},
		{
			name:           "authentication error stops",
			err:            fmt.Errorf("authentication failed"),
			expectedResult: false,
		},
		{
			name:           "configuration error stops",
			err:            fmt.Errorf("config not found"),
			expectedResult: false,
		},
		{
			name:           "network error continues",
			err:            fmt.Errorf("network timeout"),
			expectedResult: true,
		},
		{
			name:           "rate limit error continues",
			err:            fmt.Errorf("rate limit exceeded"),
			expectedResult: true,
		},
		{
			name:           "timeout error stops",
			err:            context.DeadlineExceeded,
			expectedResult: false,
		},
		{
			name:           "validation error stops",
			err:            fmt.Errorf("validation failed"),
			expectedResult: false,
		},
		{
			name:           "unknown error stops",
			err:            fmt.Errorf("unknown problem"),
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messenger := &mockSignalMessenger{}
			llm := &mockProgressLLM{}
			reporter := agent.NewSignalProgressReporter(messenger, llm, "+1234567890")

			result := reporter.ShouldContinue(tt.err)
			if result != tt.expectedResult {
				t.Errorf("Expected ShouldContinue=%v for error %v, got %v",
					tt.expectedResult, tt.err, result)
			}
		})
	}
}

func TestSignalProgressReporterConcurrency(t *testing.T) {
	messenger := &mockSignalMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			// Simulate some work
			time.Sleep(time.Millisecond)
			return nil
		},
	}
	llm := &mockProgressLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			// Simulate some work
			time.Sleep(time.Millisecond)
			return &claude.LLMResponse{Message: "concurrent message"}, nil
		},
	}

	reporter := agent.NewSignalProgressReporter(messenger, llm, "+1234567890")

	var wg sync.WaitGroup
	ctx := context.Background()
	iterations := 50

	// Test concurrent ReportError calls
	wg.Add(iterations)
	for i := range iterations {
		go func(i int) {
			defer wg.Done()
			err := reporter.ReportError(ctx, fmt.Errorf("error %d", i), "")
			if err != nil {
				t.Errorf("Unexpected error in iteration %d: %v", i, err)
			}
		}(i)
	}

	// Test concurrent ShouldContinue calls
	wg.Add(iterations)
	for i := range iterations {
		go func(i int) {
			defer wg.Done()
			_ = reporter.ShouldContinue(fmt.Errorf("error %d", i))
		}(i)
	}

	wg.Wait()

	// Verify all messages were sent
	calls := messenger.getSendCalls()
	if len(calls) != iterations {
		t.Errorf("Expected %d send calls, got %d", iterations, len(calls))
	}
}

func TestSignalProgressReporterWithContext(t *testing.T) {
	t.Run("context timeout during LLM call", func(t *testing.T) {
		messenger := &mockSignalMessenger{}
		llm := &mockLLM{
			queryFunc: func(ctx context.Context, _, _ string) (*claude.LLMResponse, error) {
				// Simulate slow LLM
				select {
				case <-time.After(10 * time.Second):
					return &claude.LLMResponse{Message: "too slow"}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}

		reporter := agent.NewSignalProgressReporter(messenger, llm, "+1234567890")

		// Create a context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := reporter.ReportError(ctx, fmt.Errorf("test error"), "")
		if err != nil {
			t.Errorf("Expected no error (should fall back to default), got %v", err)
		}

		// Should have sent a message with default text
		calls := messenger.getSendCalls()
		if len(calls) != 1 {
			t.Errorf("Expected 1 send call, got %d", len(calls))
		}
	})

	t.Run("context canceled during send", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		messenger := &mockSignalMessenger{
			sendFunc: func(msgCtx context.Context, _, _ string) error {
				cancel() // Cancel the context
				return msgCtx.Err()
			},
		}
		llm := &mockProgressLLM{}

		reporter := agent.NewSignalProgressReporter(messenger, llm, "+1234567890")

		err := reporter.ReportError(ctx, fmt.Errorf("test error"), "Test message")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})
}

func TestSignalProgressReporterErrorTypeNames(t *testing.T) {
	// This test verifies that getErrorTypeName handles all error types correctly
	errorTypes := []struct {
		err              error
		expectedContains string
	}{
		{
			err:              context.Canceled,
			expectedContains: "Canceled",
		},
		{
			err:              context.DeadlineExceeded,
			expectedContains: "Timeout",
		},
		{
			err:              fmt.Errorf("network connection failed"),
			expectedContains: "Network",
		},
		{
			err:              fmt.Errorf("rate limit exceeded"),
			expectedContains: "Rate Limit",
		},
		{
			err:              fmt.Errorf("authentication required"),
			expectedContains: "Authentication",
		},
		{
			err:              fmt.Errorf("validation error"),
			expectedContains: "Validation",
		},
		{
			err:              fmt.Errorf("config missing"),
			expectedContains: "Configuration",
		},
		{
			err:              fmt.Errorf("signal messenger error"),
			expectedContains: "Messenger",
		},
		{
			err:              fmt.Errorf("llm processing failed"),
			expectedContains: "LLM",
		},
		{
			err:              fmt.Errorf("something else"),
			expectedContains: "Unknown",
		},
	}

	for _, tc := range errorTypes {
		t.Run(tc.expectedContains, func(t *testing.T) {
			capturedPrompt := ""
			messenger := &mockSignalMessenger{}
			llm := &mockLLM{
				queryFunc: func(_ context.Context, prompt, _ string) (*claude.LLMResponse, error) {
					capturedPrompt = prompt
					return &claude.LLMResponse{Message: "test"}, nil
				},
			}

			reporter := agent.NewSignalProgressReporter(messenger, llm, "+1234567890")
			_ = reporter.ReportError(context.Background(), tc.err, "")

			// Check that the error type name appears in the LLM prompt
			if capturedPrompt == "" {
				t.Fatal("Expected LLM to be called")
			}
			if !progressContains(capturedPrompt, tc.expectedContains) {
				t.Errorf("Expected prompt to contain %q, got: %s", tc.expectedContains, capturedPrompt)
			}
		})
	}
}

// progressContains is a helper function to check if a string contains a substring.
func progressContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && (progressStringContains(s, substr)))
}

func progressStringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
