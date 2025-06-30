package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// testMockLLM is a simple mock implementation for testing.
type testMockLLM struct {
	response *claude.LLMResponse
	err      error
	calls    []struct {
		prompt    string
		sessionID string
	}
}

func (m *testMockLLM) Query(_ context.Context, prompt string, sessionID string) (*claude.LLMResponse, error) {
	m.calls = append(m.calls, struct {
		prompt    string
		sessionID string
	}{prompt: prompt, sessionID: sessionID})
	
	if m.err != nil {
		return nil, m.err
	}
	
	if m.response != nil {
		return m.response, nil
	}
	
	// Default response
	return &claude.LLMResponse{
		Message: "Mock response",
		Metadata: claude.ResponseMetadata{
			ModelVersion: "mock-model",
			Latency:      10 * time.Millisecond,
			TokensUsed:   10,
		},
	}, nil
}

func TestHandler_Process(t *testing.T) {
	tests := []struct {
		name        string
		msg         signal.IncomingMessage
		llmResponse *claude.LLMResponse
		llmError    error
		wantErr     bool
		errContains string
	}{
		{
			name: "successful processing",
			msg: signal.IncomingMessage{
				From:      "+15551234567",
				Text:      "What's the weather today?",
				Timestamp: time.Now(),
			},
			llmResponse: &claude.LLMResponse{
				Message: "I'd be happy to help with weather information. However, I need to know your location to provide accurate weather details.",
				Metadata: claude.ResponseMetadata{
					ModelVersion: "claude-3-opus-20240229",
					Latency:      100 * time.Millisecond,
					TokensUsed:   50,
				},
			},
			wantErr: false,
		},
		{
			name: "empty message text",
			msg: signal.IncomingMessage{
				From:      "+15551234567",
				Text:      "",
				Timestamp: time.Now(),
			},
			llmResponse: &claude.LLMResponse{
				Message: "I didn't receive any message. Could you please send your question again?",
			},
			wantErr: false,
		},
		{
			name: "LLM returns error",
			msg: signal.IncomingMessage{
				From:      "+15551234567",
				Text:      "Hello",
				Timestamp: time.Now(),
			},
			llmError:    fmt.Errorf("LLM service unavailable"),
			wantErr:     true,
			errContains: "LLM query failed",
		},
		{
			name: "context cancellation",
			msg: signal.IncomingMessage{
				From:      "+15551234567",
				Text:      "Long running query",
				Timestamp: time.Now(),
			},
			llmError:    context.Canceled,
			wantErr:     true,
			errContains: "context canceled",
		},
		{
			name: "very long message",
			msg: signal.IncomingMessage{
				From:      "+15551234567",
				Text:      string(make([]byte, 10000)), // 10KB message
				Timestamp: time.Now(),
			},
			llmResponse: &claude.LLMResponse{
				Message: "I've received your message. How can I help you?",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock LLM
			mockLLM := &testMockLLM{
				response: tt.llmResponse,
				err:      tt.llmError,
			}

			// Create handler
			handler := NewHandler(mockLLM)

			// Process message
			ctx := context.Background()
			err := handler.Process(ctx, tt.msg)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errContains != "" {
				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("Process() error = %v, want error containing %q", err, tt.errContains)
				}
			}

			// Verify LLM was called
			if !tt.wantErr && len(mockLLM.calls) != 1 {
				t.Errorf("Expected 1 LLM call, got %d", len(mockLLM.calls))
			}

			if len(mockLLM.calls) > 0 {
				call := mockLLM.calls[0]
				if call.prompt != tt.msg.Text {
					t.Errorf("LLM called with prompt %q, want %q", call.prompt, tt.msg.Text)
				}
				expectedSessionID := fmt.Sprintf("signal-%s", tt.msg.From)
				if call.sessionID != expectedSessionID {
					t.Errorf("LLM called with session ID %q, want %q", call.sessionID, expectedSessionID)
				}
			}
		})
	}
}

func TestHandler_ProcessWithTimeout(t *testing.T) {
	// Create a custom mock that delays
	slowLLM := &slowMockLLM{
		delay: 200 * time.Millisecond,
	}

	handler := NewHandler(slowLLM)

	// Create a context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	msg := signal.IncomingMessage{
		From:      "+15551234567",
		Text:      "This will timeout",
		Timestamp: time.Now(),
	}

	err := handler.Process(ctx, msg)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if !containsString(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected context deadline exceeded error, got: %v", err)
	}
}

func TestHandler_Constructor(t *testing.T) {
	t.Run("nil LLM panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil LLM, but didn't panic")
			}
		}()
		
		_ = NewHandler(nil)
	})

	t.Run("valid constructor", func(t *testing.T) {
		mockLLM := &testMockLLM{}
		handler := NewHandler(mockLLM)
		
		if handler == nil {
			t.Error("Expected non-nil handler")
		}
		
		// Verify it implements the interface
		var _ = handler
	})
}

func TestHandler_SessionIDGeneration(t *testing.T) {
	tests := []struct {
		name              string
		from              string
		expectedSessionID string
	}{
		{
			name:              "standard phone number",
			from:              "+15551234567",
			expectedSessionID: "signal-+15551234567",
		},
		{
			name:              "international number",
			from:              "+447911123456",
			expectedSessionID: "signal-+447911123456",
		},
		{
			name:              "short number",
			from:              "12345",
			expectedSessionID: "signal-12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLLM := &testMockLLM{
				response: &claude.LLMResponse{
					Message: "Response",
				},
			}
			handler := NewHandler(mockLLM)

			msg := signal.IncomingMessage{
				From:      tt.from,
				Text:      "Test message",
				Timestamp: time.Now(),
			}

			_ = handler.Process(context.Background(), msg)

			if len(mockLLM.calls) != 1 {
				t.Fatalf("Expected 1 call, got %d", len(mockLLM.calls))
			}

			if mockLLM.calls[0].sessionID != tt.expectedSessionID {
				t.Errorf("Session ID = %q, want %q", mockLLM.calls[0].sessionID, tt.expectedSessionID)
			}
		})
	}
}

// slowMockLLM adds delay to simulate slow responses.
type slowMockLLM struct {
	delay time.Duration
}

func (s *slowMockLLM) Query(ctx context.Context, _ string, _ string) (*claude.LLMResponse, error) {
	select {
	case <-time.After(s.delay):
		return &claude.LLMResponse{
			Message: "Slow response",
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func containsString(s, substr string) bool {
	return substr != "" && len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}