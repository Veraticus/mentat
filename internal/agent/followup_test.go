package agent_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultResultHandler_HandleResult(t *testing.T) {
	tests := []struct {
		name             string
		result           agent.ValidationResult
		expectMessage    bool
		expectedLogLevel string
		customHandler    bool
	}{
		{
			name: "success status - no follow-up",
			result: agent.ValidationResult{
				Status:     agent.ValidationStatusSuccess,
				Confidence: 0.95,
			},
			expectMessage: false,
		},
		{
			name: "partial completion - sends follow-up",
			result: agent.ValidationResult{
				Status:     agent.ValidationStatusPartial,
				Confidence: 0.7,
				Issues:     []string{"Could not access calendar"},
			},
			expectMessage: true,
		},
		{
			name: "failed validation - sends follow-up",
			result: agent.ValidationResult{
				Status:     agent.ValidationStatusFailed,
				Confidence: 0.3,
				Issues:     []string{"Service unavailable"},
			},
			expectMessage: true,
		},
		{
			name: "incomplete search - sends follow-up",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusIncompleteSearch,
				Metadata: map[string]string{
					"expected_tools": "calendar,memory",
				},
			},
			expectMessage: true,
		},
		{
			name: "unclear request - sends clarification",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusUnclear,
			},
			expectMessage: true,
		},
		{
			name: "unknown status - logs warning",
			result: agent.ValidationResult{
				Status: agent.ValidationStatus("unknown"),
			},
			expectMessage:    false,
			expectedLogLevel: "WARN",
		},
		{
			name: "custom handler override",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusPartial,
			},
			expectMessage: false,
			customHandler: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockMessenger := newMockMessenger()
			mockStrategy := &followupTestValidationStrategy{
				recoveryMessage: "Test correction message",
			}
			logger := newTestLogger()

			handler := agent.NewDefaultResultHandler(mockMessenger, mockStrategy, logger)
			handler.SetCorrectionDelay(0) // No delay for tests
			handler.SetClarifyDelay(0)

			// Add custom handler if needed
			if tt.customHandler {
				handler.SetStatusHandler(
					agent.ValidationStatusPartial,
					func(_ context.Context, _ agent.ValidationResult, _ *agent.ValidationTask) error {
						// Custom handler that does nothing
						return nil
					},
				)
			}

			task := &agent.ValidationTask{
				ID:              "test-task-1",
				To:              "+1234567890",
				OriginalRequest: "test request",
				Response: &claude.LLMResponse{
					Message: "test response",
				},
				SessionID: "test-session",
			}

			// Execute
			ctx := context.Background()
			err := handler.HandleResult(ctx, tt.result, task)

			// Assert
			require.NoError(t, err)

			if tt.expectMessage {
				messages := mockMessenger.GetSentMessages()
				assert.Len(t, messages, 1)
				assert.Equal(t, task.To, messages[0].to)
				assert.Contains(t, messages[0].message, "Test correction message")
			} else {
				assert.Empty(t, mockMessenger.GetSentMessages())
			}
		})
	}
}

func TestDefaultResultHandler_ShouldHandle(t *testing.T) {
	tests := []struct {
		name         string
		status       agent.ValidationStatus
		shouldHandle bool
	}{
		{
			name:         "success - should not handle",
			status:       agent.ValidationStatusSuccess,
			shouldHandle: false,
		},
		{
			name:         "partial - should handle",
			status:       agent.ValidationStatusPartial,
			shouldHandle: true,
		},
		{
			name:         "failed - should handle",
			status:       agent.ValidationStatusFailed,
			shouldHandle: true,
		},
		{
			name:         "unclear - should handle",
			status:       agent.ValidationStatusUnclear,
			shouldHandle: true,
		},
		{
			name:         "incomplete search - should handle",
			status:       agent.ValidationStatusIncompleteSearch,
			shouldHandle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &agent.DefaultResultHandler{}
			result := agent.ValidationResult{Status: tt.status}
			assert.Equal(t, tt.shouldHandle, handler.ShouldHandle(result))
		})
	}
}

func TestDefaultResultHandler_sendFollowUp(t *testing.T) {
	t.Run("successful send", func(t *testing.T) {
		// Setup
		mockMessenger := newMockMessenger()
		mockStrategy := &followupTestValidationStrategy{
			recoveryMessage: "Follow-up message",
		}
		logger := newTestLogger()

		handler := agent.NewDefaultResultHandler(mockMessenger, mockStrategy, logger)

		task := &agent.ValidationTask{
			ID:              "test-task",
			To:              "+1234567890",
			OriginalRequest: "original",
			Response:        &claude.LLMResponse{Message: "response"},
			SessionID:       "session-1",
		}

		result := agent.ValidationResult{
			Status: agent.ValidationStatusPartial,
			Issues: []string{"issue1"},
		}

		// Execute
		ctx := context.Background()
		// We test through the public HandleResult method
		err := handler.HandleResult(ctx, result, task)

		// Assert
		require.NoError(t, err)
		messages := mockMessenger.GetSentMessages()
		assert.Len(t, messages, 1)
		assert.Equal(t, "Follow-up message", messages[0].message)
	})

	t.Run("empty recovery message - no send", func(t *testing.T) {
		// Setup
		mockMessenger := newMockMessenger()
		mockStrategy := &followupTestValidationStrategy{
			recoveryMessage: "", // Empty message
		}
		logger := newTestLogger()

		handler := agent.NewDefaultResultHandler(mockMessenger, mockStrategy, logger)

		task := &agent.ValidationTask{
			ID:       "test-task",
			To:       "+1234567890",
			Response: &claude.LLMResponse{},
		}
		result := agent.ValidationResult{Status: agent.ValidationStatusPartial}

		// Execute
		ctx := context.Background()
		err := handler.HandleResult(ctx, result, task)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, mockMessenger.GetSentMessages())
	})

	t.Run("send error", func(t *testing.T) {
		// Setup
		mockMessenger := newMockMessenger()
		mockMessenger.SetError(fmt.Errorf("send failed"))
		mockStrategy := &followupTestValidationStrategy{
			recoveryMessage: "message",
		}
		logger := newTestLogger()

		handler := agent.NewDefaultResultHandler(mockMessenger, mockStrategy, logger)

		task := &agent.ValidationTask{
			ID:       "test-task",
			To:       "+1234567890",
			Response: &claude.LLMResponse{Message: "response"},
		}
		result := agent.ValidationResult{Status: agent.ValidationStatusFailed}

		// Execute
		ctx := context.Background()
		err := handler.HandleResult(ctx, result, task)

		// Assert - Since HandleResult returns error from sendFollowUp
		require.Error(t, err)
		assert.Contains(t, err.Error(), "send follow-up")
	})

	t.Run("context cancellation during delay", func(t *testing.T) {
		// Setup
		mockMessenger := newMockMessenger()
		mockStrategy := &followupTestValidationStrategy{
			recoveryMessage: "message",
		}
		logger := newTestLogger()

		handler := agent.NewDefaultResultHandler(mockMessenger, mockStrategy, logger)
		handler.SetCorrectionDelay(100 * time.Millisecond)

		task := &agent.ValidationTask{
			ID:       "test-task",
			To:       "+1234567890",
			Response: &claude.LLMResponse{},
		}
		result := agent.ValidationResult{Status: agent.ValidationStatusFailed}

		// Execute with canceling context
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		err := handler.HandleResult(ctx, result, task)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "follow-up canceled")
		assert.Empty(t, mockMessenger.GetSentMessages())
	})
}

func TestDefaultResultHandler_Delays(t *testing.T) {
	t.Run("correction delay applied", func(t *testing.T) {
		// Setup
		mockMessenger := newMockMessenger()
		mockStrategy := &followupTestValidationStrategy{
			recoveryMessage: "correction",
		}
		logger := newTestLogger()

		handler := agent.NewDefaultResultHandler(mockMessenger, mockStrategy, logger)
		handler.SetCorrectionDelay(50 * time.Millisecond)

		task := &agent.ValidationTask{
			ID:       "test-task",
			To:       "+1234567890",
			Response: &claude.LLMResponse{Message: "response"},
		}
		result := agent.ValidationResult{Status: agent.ValidationStatusPartial}

		// Execute
		ctx := context.Background()
		start := time.Now()
		err := handler.HandleResult(ctx, result, task)
		duration := time.Since(start)

		// Assert
		require.NoError(t, err)
		assert.GreaterOrEqual(t, duration, 50*time.Millisecond)
		assert.Len(t, mockMessenger.GetSentMessages(), 1)
	})

	t.Run("clarify delay applied", func(t *testing.T) {
		// Setup
		mockMessenger := newMockMessenger()
		mockStrategy := &followupTestValidationStrategy{
			recoveryMessage: "clarification",
		}
		logger := newTestLogger()

		handler := agent.NewDefaultResultHandler(mockMessenger, mockStrategy, logger)
		handler.SetClarifyDelay(30 * time.Millisecond)

		task := &agent.ValidationTask{
			ID:       "test-task",
			To:       "+1234567890",
			Response: &claude.LLMResponse{Message: "response"},
		}
		result := agent.ValidationResult{Status: agent.ValidationStatusUnclear}

		// Execute
		ctx := context.Background()
		start := time.Now()
		err := handler.HandleResult(ctx, result, task)
		duration := time.Since(start)

		// Assert
		require.NoError(t, err)
		assert.GreaterOrEqual(t, duration, 30*time.Millisecond)
		assert.Len(t, mockMessenger.GetSentMessages(), 1)
	})
}

func TestChainedResultHandler(t *testing.T) {
	t.Run("multiple handlers called", func(t *testing.T) {
		// Setup
		var called1, called2 bool
		handler1 := &testHandler{
			shouldHandle: true,
			handleFunc: func(_ context.Context, _ agent.ValidationResult, _ *agent.ValidationTask) error {
				called1 = true
				return nil
			},
		}
		handler2 := &testHandler{
			shouldHandle: true,
			handleFunc: func(_ context.Context, _ agent.ValidationResult, _ *agent.ValidationTask) error {
				called2 = true
				return nil
			},
		}

		logger := newTestLogger()
		chained := agent.NewChainedResultHandler(logger, handler1, handler2)

		// Execute
		ctx := context.Background()
		result := agent.ValidationResult{Status: agent.ValidationStatusPartial}
		task := &agent.ValidationTask{ID: "test"}
		err := chained.HandleResult(ctx, result, task)

		// Assert
		require.NoError(t, err)
		assert.True(t, called1)
		assert.True(t, called2)
	})

	t.Run("only matching handlers called", func(t *testing.T) {
		// Setup
		var called1, called2 bool
		handler1 := &testHandler{
			shouldHandle: false,
			handleFunc: func(_ context.Context, _ agent.ValidationResult, _ *agent.ValidationTask) error {
				called1 = true
				return nil
			},
		}
		handler2 := &testHandler{
			shouldHandle: true,
			handleFunc: func(_ context.Context, _ agent.ValidationResult, _ *agent.ValidationTask) error {
				called2 = true
				return nil
			},
		}

		logger := newTestLogger()
		chained := agent.NewChainedResultHandler(logger, handler1, handler2)

		// Execute
		ctx := context.Background()
		result := agent.ValidationResult{Status: agent.ValidationStatusPartial}
		task := &agent.ValidationTask{ID: "test"}
		err := chained.HandleResult(ctx, result, task)

		// Assert
		require.NoError(t, err)
		assert.False(t, called1)
		assert.True(t, called2)
	})

	t.Run("continues on error", func(t *testing.T) {
		// Setup
		var called2 bool
		handler1 := &testHandler{
			shouldHandle: true,
			handleFunc: func(_ context.Context, _ agent.ValidationResult, _ *agent.ValidationTask) error {
				return fmt.Errorf("handler1 error")
			},
		}
		handler2 := &testHandler{
			shouldHandle: true,
			handleFunc: func(_ context.Context, _ agent.ValidationResult, _ *agent.ValidationTask) error {
				called2 = true
				return nil
			},
		}

		logger := newTestLogger()
		chained := agent.NewChainedResultHandler(logger, handler1, handler2)

		// Execute
		ctx := context.Background()
		result := agent.ValidationResult{Status: agent.ValidationStatusPartial}
		task := &agent.ValidationTask{ID: "test"}
		err := chained.HandleResult(ctx, result, task)

		// Assert
		require.NoError(t, err) // Chain doesn't return errors
		assert.True(t, called2)
	})

	t.Run("should handle if any handler matches", func(t *testing.T) {
		// Setup
		handler1 := &testHandler{shouldHandle: false}
		handler2 := &testHandler{shouldHandle: true}
		handler3 := &testHandler{shouldHandle: false}

		logger := newTestLogger()
		chained := agent.NewChainedResultHandler(logger, handler1, handler2, handler3)

		// Execute
		result := agent.ValidationResult{Status: agent.ValidationStatusPartial}
		shouldHandle := chained.ShouldHandle(result)

		// Assert
		assert.True(t, shouldHandle)
	})

	t.Run("should not handle if no handler matches", func(t *testing.T) {
		// Setup
		handler1 := &testHandler{shouldHandle: false}
		handler2 := &testHandler{shouldHandle: false}

		logger := newTestLogger()
		chained := agent.NewChainedResultHandler(logger, handler1, handler2)

		// Execute
		result := agent.ValidationResult{Status: agent.ValidationStatusSuccess}
		shouldHandle := chained.ShouldHandle(result)

		// Assert
		assert.False(t, shouldHandle)
	})
}

// Test helpers

type followupTestValidationStrategy struct {
	recoveryMessage string
}

func (m *followupTestValidationStrategy) Validate(
	_ context.Context,
	_, _, _ string,
	_ claude.LLM,
) agent.ValidationResult {
	return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
}

func (m *followupTestValidationStrategy) ShouldRetry(_ agent.ValidationResult) bool {
	return false
}

func (m *followupTestValidationStrategy) GenerateRecovery(
	_ context.Context,
	_, _, _ string,
	_ agent.ValidationResult,
	_ claude.LLM,
) string {
	return m.recoveryMessage
}

type testHandler struct {
	shouldHandle bool
	handleFunc   func(ctx context.Context, result agent.ValidationResult, task *agent.ValidationTask) error
}

func (h *testHandler) HandleResult(
	ctx context.Context,
	result agent.ValidationResult,
	task *agent.ValidationTask,
) error {
	if h.handleFunc != nil {
		return h.handleFunc(ctx, result, task)
	}
	return nil
}

func (h *testHandler) ShouldHandle(_ agent.ValidationResult) bool {
	return h.shouldHandle
}

// Mock messenger for testing.
type followupTestMessenger struct {
	mu       sync.Mutex
	messages []sentMessage
	err      error
}

type sentMessage struct {
	to      string
	message string
}

func newMockMessenger() *followupTestMessenger {
	return &followupTestMessenger{
		messages: make([]sentMessage, 0),
	}
}

func (m *followupTestMessenger) Send(_ context.Context, to, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	m.messages = append(m.messages, sentMessage{to: to, message: message})
	return nil
}

func (m *followupTestMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

func (m *followupTestMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}

func (m *followupTestMessenger) GetSentMessages() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]sentMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *followupTestMessenger) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// Test logger.
func newTestLogger() *slog.Logger {
	return slog.Default()
}
