package setup_test

import (
	"context"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/setup"
)

// Compile-time check that mockPrompter implements Prompter.
var _ setup.Prompter = (*mockPrompter)(nil)

// mockPrompter is a test implementation of Prompter.
type mockPrompter struct {
	responses      map[string]string
	messages       []string
	errors         []string
	successes      []string
	shouldError    bool
	capturedInputs []string
}

func (m *mockPrompter) Prompt(_ context.Context, message string) (string, error) {
	m.capturedInputs = append(m.capturedInputs, message)
	if m.shouldError {
		return "", context.Canceled
	}
	if resp, ok := m.responses[message]; ok {
		return resp, nil
	}
	return "default-response", nil
}

func (m *mockPrompter) PromptWithDefault(_ context.Context, message, defaultValue string) (string, error) {
	m.capturedInputs = append(m.capturedInputs, message)
	if m.shouldError {
		return "", context.Canceled
	}
	if resp, ok := m.responses[message]; ok && resp != "" {
		return resp, nil
	}
	return defaultValue, nil
}

func (m *mockPrompter) PromptWithTimeout(_ context.Context, message string, _ time.Duration) (string, error) {
	m.capturedInputs = append(m.capturedInputs, message)
	if m.shouldError {
		return "", context.DeadlineExceeded
	}
	if resp, ok := m.responses[message]; ok {
		return resp, nil
	}
	return "timeout-response", nil
}

func (m *mockPrompter) PromptSecret(_ context.Context, message string) (string, error) {
	m.capturedInputs = append(m.capturedInputs, message)
	if m.shouldError {
		return "", context.Canceled
	}
	if resp, ok := m.responses[message]; ok {
		return resp, nil
	}
	return "secret-response", nil
}

func (m *mockPrompter) ShowMessage(message string) {
	m.messages = append(m.messages, message)
}

func (m *mockPrompter) ShowError(message string) {
	m.errors = append(m.errors, message)
}

func (m *mockPrompter) ShowSuccess(message string) {
	m.successes = append(m.successes, message)
}

func TestPrompterInterface(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, p setup.Prompter)
	}{
		{
			name: "Prompt",
			fn: func(t *testing.T, p setup.Prompter) {
				t.Helper()
				resp, err := p.Prompt(context.Background(), "Enter value")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == "" {
					t.Error("expected non-empty response")
				}
			},
		},
		{
			name: "PromptWithDefault",
			fn: func(t *testing.T, p setup.Prompter) {
				t.Helper()
				resp, err := p.PromptWithDefault(context.Background(), "Enter value", "default")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == "" {
					t.Error("expected non-empty response")
				}
			},
		},
		{
			name: "PromptWithTimeout",
			fn: func(t *testing.T, p setup.Prompter) {
				t.Helper()
				resp, err := p.PromptWithTimeout(context.Background(), "Enter value", 5*time.Second)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == "" {
					t.Error("expected non-empty response")
				}
			},
		},
		{
			name: "PromptSecret",
			fn: func(t *testing.T, p setup.Prompter) {
				t.Helper()
				resp, err := p.PromptSecret(context.Background(), "Enter secret")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == "" {
					t.Error("expected non-empty response")
				}
			},
		},
		{
			name: "ShowMessage",
			fn: func(_ *testing.T, p setup.Prompter) {
				p.ShowMessage("Info message")
			},
		},
		{
			name: "ShowError",
			fn: func(_ *testing.T, p setup.Prompter) {
				p.ShowError("Error message")
			},
		},
		{
			name: "ShowSuccess",
			fn: func(_ *testing.T, p setup.Prompter) {
				p.ShowSuccess("Success message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPrompter{
				responses: make(map[string]string),
			}
			tt.fn(t, mock)
		})
	}
}

func TestMockPrompterBehavior(t *testing.T) {
	t.Run("captures inputs", func(t *testing.T) {
		mock := &mockPrompter{
			responses: make(map[string]string),
		}

		ctx := context.Background()
		_, _ = mock.Prompt(ctx, "test1")
		_, _ = mock.PromptWithDefault(ctx, "test2", "default")

		if len(mock.capturedInputs) != 2 {
			t.Errorf("expected 2 captured inputs, got %d", len(mock.capturedInputs))
		}
	})

	t.Run("returns configured responses", func(t *testing.T) {
		mock := &mockPrompter{
			responses: map[string]string{
				"Enter code": "123456",
			},
		}

		resp, err := mock.Prompt(context.Background(), "Enter code")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if resp != "123456" {
			t.Errorf("expected '123456', got '%s'", resp)
		}
	})

	t.Run("handles errors", func(t *testing.T) {
		mock := &mockPrompter{
			responses:   make(map[string]string),
			shouldError: true,
		}

		_, err := mock.Prompt(context.Background(), "test")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("collects display messages", func(t *testing.T) {
		mock := &mockPrompter{
			responses: make(map[string]string),
		}

		mock.ShowMessage("info")
		mock.ShowError("error")
		mock.ShowSuccess("success")

		if len(mock.messages) != 1 || mock.messages[0] != "info" {
			t.Error("message not captured correctly")
		}
		if len(mock.errors) != 1 || mock.errors[0] != "error" {
			t.Error("error not captured correctly")
		}
		if len(mock.successes) != 1 || mock.successes[0] != "success" {
			t.Error("success not captured correctly")
		}
	})
}
