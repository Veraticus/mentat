package agent_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

func TestErrorRecovery(t *testing.T) {
	recovery := agent.NewErrorRecovery()

	tests := []struct {
		name             string
		err              error
		expectedContains string
	}{
		{
			name:             "canceled error",
			err:              context.Canceled,
			expectedContains: "canceled",
		},
		{
			name:             "timeout error",
			err:              context.DeadlineExceeded,
			expectedContains: "too long",
		},
		{
			name:             "authentication error",
			err:              &claude.AuthenticationError{Message: "not authenticated"},
			expectedContains: "authenticated",
		},
		{
			name:             "rate limit error",
			err:              fmt.Errorf("rate limit exceeded"),
			expectedContains: "high demand",
		},
		{
			name:             "network error",
			err:              fmt.Errorf("network failure"),
			expectedContains: "connecting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := recovery.GenerateUserMessage(tt.err)
			if msg == "" {
				t.Error("expected non-empty message")
			}
			if !strings.Contains(msg, tt.expectedContains) {
				t.Errorf("expected message to contain %q, got %q", tt.expectedContains, msg)
			}
		})
	}
}

func TestContextualErrorMessages(t *testing.T) {
	recovery := agent.NewErrorRecovery()

	tests := []struct {
		name             string
		err              error
		request          string
		expectedContains string
	}{
		{
			name:             "timeout with long request",
			err:              context.DeadlineExceeded,
			request:          "This is a very long request that exceeds the threshold and should trigger a special message about breaking it into smaller parts",
			expectedContains: "breaking it into smaller parts",
		},
		{
			name:             "timeout with short request",
			err:              context.DeadlineExceeded,
			request:          "Short request",
			expectedContains: "too long to process",
		},
		{
			name:             "validation error",
			err:              fmt.Errorf("validation failed"),
			request:          "Find my calendar events",
			expectedContains: "Find my calendar events",
		},
		{
			name:             "rate limit error",
			err:              fmt.Errorf("rate limit exceeded"),
			request:          "Any request",
			expectedContains: "queued",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := recovery.GenerateContextualMessage(tt.err, tt.request)
			if !strings.Contains(msg, tt.expectedContains) {
				t.Errorf("expected message to contain %q, got %q", tt.expectedContains, msg)
			}
		})
	}
}

func TestProcessingErrorHandler(t *testing.T) {
	// Create a test messenger
	type testMessenger struct {
		sendCalled    bool
		sendError     error
		sentMessage   string
		sentRecipient string
	}

	tests := []struct {
		name              string
		err               error
		request           string
		sendError         error
		expectSendMessage bool
		expectedError     bool
	}{
		{
			name:              "successful error handling",
			err:               fmt.Errorf("test error"),
			request:           "test request",
			sendError:         nil,
			expectSendMessage: true,
			expectedError:     true,
		},
		{
			name:              "failed to send error message",
			err:               fmt.Errorf("test error"),
			request:           "test request",
			sendError:         fmt.Errorf("send failed"),
			expectSendMessage: true,
			expectedError:     true,
		},
		{
			name:              "no error to handle",
			err:               nil,
			request:           "test request",
			sendError:         nil,
			expectSendMessage: false,
			expectedError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messenger := &testMessenger{
				sendError: tt.sendError,
			}

			// Create a signal.Messenger implementation
			messengerAdapter := &messengerAdapter{
				send: func(_ context.Context, recipient, message string) error {
					messenger.sendCalled = true
					messenger.sentMessage = message
					messenger.sentRecipient = recipient
					return messenger.sendError
				},
			}

			handler := agent.NewProcessingErrorHandler(messengerAdapter)
			err := handler.HandleError(context.Background(), tt.err, tt.request, "test-recipient")

			if tt.expectSendMessage != messenger.sendCalled {
				t.Errorf("expected send called %v, got %v", tt.expectSendMessage, messenger.sendCalled)
			}

			if tt.expectSendMessage && messenger.sentMessage == "" {
				t.Error("expected non-empty error message")
			}

			if tt.expectedError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestWrapWithRecovery(t *testing.T) {
	tests := []struct {
		name              string
		fnError           error
		sendError         error
		expectSendMessage bool
		expectedError     bool
	}{
		{
			name:              "function succeeds",
			fnError:           nil,
			sendError:         nil,
			expectSendMessage: false,
			expectedError:     false,
		},
		{
			name:              "function fails, recovery message sent",
			fnError:           fmt.Errorf("operation failed"),
			sendError:         nil,
			expectSendMessage: true,
			expectedError:     true,
		},
		{
			name:              "function fails, recovery message fails",
			fnError:           fmt.Errorf("operation failed"),
			sendError:         fmt.Errorf("send failed"),
			expectSendMessage: true,
			expectedError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sendCalled := false
			messengerAdapter := &messengerAdapter{
				send: func(_ context.Context, _, _ string) error {
					sendCalled = true
					return tt.sendError
				},
			}

			handler := agent.NewProcessingErrorHandler(messengerAdapter)
			err := handler.WrapWithRecovery(
				context.Background(),
				"test-recipient",
				"test request",
				func() error {
					return tt.fnError
				},
			)

			if tt.expectSendMessage != sendCalled {
				t.Errorf("expected send called %v, got %v", tt.expectSendMessage, sendCalled)
			}

			if tt.expectedError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

// messengerAdapter implements signal.Messenger for testing.
type messengerAdapter struct {
	send func(context.Context, string, string) error
}

func (m *messengerAdapter) Send(ctx context.Context, recipient, message string) error {
	if m.send != nil {
		return m.send(ctx, recipient, message)
	}
	return nil
}

func (m *messengerAdapter) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

func (m *messengerAdapter) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}
