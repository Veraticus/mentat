package agent_test

import (
	"context"
	"fmt"
	"testing"

	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/signal"
)

func TestHandlerRetryLogic(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name                   string
		incomingMessage        signal.IncomingMessage
		initialResponse        claude.LLMResponse
		validationResults      []agent.ValidationResult
		retryResponses         []claude.LLMResponse
		retryErrors            []error
		maxRetries             int
		expectedFinalMessage   string
		expectRetryAttempts    int
		expectRecoveryGenerate bool
	}{
		{
			name: "successful_response_no_retry",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "What's on my calendar?",
			},
			initialResponse: claude.LLMResponse{
				Message: "You have 3 meetings today...",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusSuccess,
					Confidence: 0.9,
				},
			},
			maxRetries:           2,
			expectedFinalMessage: "You have 3 meetings today...",
			expectRetryAttempts:  0,
		},
		{
			name: "incomplete_search_retry_once",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "What did we discuss last time?",
			},
			initialResponse: claude.LLMResponse{
				Message: "I'm not sure what we discussed.",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusIncompleteSearch,
					Confidence: 0.3,
					Metadata:   map[string]string{"expected_tools": "memory"},
				},
				{
					Status:     agent.ValidationStatusSuccess,
					Confidence: 0.9,
				},
			},
			retryResponses: []claude.LLMResponse{
				{
					Message: "Last time we discussed your project timeline.",
				},
			},
			maxRetries:           2,
			expectedFinalMessage: "Last time we discussed your project timeline.",
			expectRetryAttempts:  1,
		},
		{
			name: "incomplete_search_max_retries_exceeded",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "Check my schedule and email",
			},
			initialResponse: claude.LLMResponse{
				Message: "I can help with that.",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusIncompleteSearch,
					Confidence: 0.2,
					Metadata:   map[string]string{"expected_tools": "calendar,email"},
				},
				{
					Status:     agent.ValidationStatusIncompleteSearch,
					Confidence: 0.3,
					Metadata:   map[string]string{"expected_tools": "email"},
				},
				{
					Status:     agent.ValidationStatusIncompleteSearch,
					Confidence: 0.4,
					Metadata:   map[string]string{"expected_tools": "email"},
				},
			},
			retryResponses: []claude.LLMResponse{
				{
					Message: "I checked your calendar...",
				},
				{
					Message: "Still working on it...",
				},
			},
			maxRetries:             2,
			expectedFinalMessage:   "Recovery message",
			expectRetryAttempts:    2,
			expectRecoveryGenerate: true,
		},
		{
			name: "retry_query_fails",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "What tasks do I have?",
			},
			initialResponse: claude.LLMResponse{
				Message: "Let me check.",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusIncompleteSearch,
					Confidence: 0.2,
					Metadata:   map[string]string{"expected_tools": "tasks"},
				},
			},
			retryResponses: []claude.LLMResponse{
				{
					Message: "I found your tasks...", // This would be returned but error prevents it
				},
			},
			retryErrors: []error{
				fmt.Errorf("LLM timeout"), // First retry fails
			},
			maxRetries:           2,
			expectedFinalMessage: "Let me check.", // Falls back to original
			expectRetryAttempts:  1,
		},
		{
			name: "validation_failed_with_recovery",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "Send an important email",
			},
			initialResponse: claude.LLMResponse{
				Message: "Error: unable to send email",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusFailed,
					Confidence: 0.9,
					Issues:     []string{"email service unavailable"},
				},
			},
			maxRetries:             2,
			expectedFinalMessage:   "Recovery message",
			expectRetryAttempts:    0,
			expectRecoveryGenerate: true,
		},
		{
			name: "unclear_with_low_confidence_retry",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "Complex multi-step request",
			},
			initialResponse: claude.LLMResponse{
				Message: "I'll help with that.",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusUnclear,
					Confidence: 0.2, // Below 0.3 threshold
				},
				{
					Status:     agent.ValidationStatusSuccess,
					Confidence: 0.8,
				},
			},
			retryResponses: []claude.LLMResponse{
				{
					Message: "Here's the complete answer...",
				},
			},
			maxRetries:           2,
			expectedFinalMessage: "Here's the complete answer...",
			expectRetryAttempts:  1,
		},
		{
			name: "partial_success_no_retry",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "Two things to do",
			},
			initialResponse: claude.LLMResponse{
				Message: "I completed the first task.",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusPartial,
					Confidence: 0.7,
				},
			},
			maxRetries:           2,
			expectedFinalMessage: "I completed the first task.",
			expectRetryAttempts:  0,
		},
		{
			name: "partial_success_with_recovery",
			incomingMessage: signal.IncomingMessage{
				From: "user123",
				Text: "Check my calendar and send an email to Bob",
			},
			initialResponse: claude.LLMResponse{
				Message: "I found your calendar events.",
			},
			validationResults: []agent.ValidationResult{
				{
					Status:     agent.ValidationStatusPartial,
					Confidence: 0.7,
					Issues:     []string{"email service unavailable", "couldn't find Bob's email address"},
				},
			},
			maxRetries:             2,
			expectedFinalMessage:   "Recovery message",
			expectRetryAttempts:    0,
			expectRecoveryGenerate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockLLM := &testMockLLM{
				responses: []claude.LLMResponse{tt.initialResponse},
				errors:    []error{nil},
			}
			mockMessenger := &testMockMessenger{}
			mockSessionManager := &testMockSessionManager{
				sessionID: "test-session",
			}

			// Add retry responses and errors
			for i, resp := range tt.retryResponses {
				mockLLM.responses = append(mockLLM.responses, resp)
				if i < len(tt.retryErrors) && tt.retryErrors[i] != nil {
					mockLLM.errors = append(mockLLM.errors, tt.retryErrors[i])
				} else {
					mockLLM.errors = append(mockLLM.errors, nil)
				}
			}

			// Create mock validator
			validationIndex := 0
			mockValidator := &testMockValidationStrategy{
				validationResults: tt.validationResults,
				shouldRetryFunc: func(result agent.ValidationResult) bool {
					return result.Status == agent.ValidationStatusIncompleteSearch ||
						(result.Status == agent.ValidationStatusUnclear && result.Confidence < 0.3)
				},
				generateRecoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
					if tt.expectRecoveryGenerate {
						return "Recovery message"
					}
					return ""
				},
				validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
					if validationIndex < len(tt.validationResults) {
						result := tt.validationResults[validationIndex]
						validationIndex++
						return result
					}
					return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
				},
			}

			// Create handler with mocks
			handler, err := agent.NewHandler(
				mockLLM,
				agent.WithValidationStrategy(mockValidator),
				agent.WithMessenger(mockMessenger),
				agent.WithSessionManager(mockSessionManager),
				agent.WithConfig(agent.Config{
					MaxRetries:              tt.maxRetries,
					EnableIntentEnhancement: false,
					ValidationThreshold:     0.8,
				}),
			)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Process message
			err = handler.Process(ctx, tt.incomingMessage)
			if err != nil {
				t.Fatalf("Process failed: %v", err)
			}

			// Verify the final message sent
			if len(mockMessenger.sentMessages) != 1 {
				t.Fatalf("Expected 1 message sent, got %d", len(mockMessenger.sentMessages))
			}

			sentMsg := mockMessenger.sentMessages[0]
			if sentMsg.text != tt.expectedFinalMessage {
				t.Errorf("Expected final message %q, got %q", tt.expectedFinalMessage, sentMsg.text)
			}

			// Verify retry attempts (subtract 1 for initial query)
			actualRetries := mockLLM.callCount - 1
			if actualRetries != tt.expectRetryAttempts {
				t.Errorf("Expected %d retry attempts, got %d", tt.expectRetryAttempts, actualRetries)
			}
		})
	}
}

// testMockValidationStrategy for testing.
type testMockValidationStrategy struct {
	validationResults    []agent.ValidationResult
	validateFunc         func(context.Context, string, string, string, claude.LLM) agent.ValidationResult
	shouldRetryFunc      func(agent.ValidationResult) bool
	generateRecoveryFunc func(context.Context, string, string, string, agent.ValidationResult, claude.LLM) string
}

func (m *testMockValidationStrategy) Validate(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) agent.ValidationResult {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, request, response, sessionID, llm)
	}
	if len(m.validationResults) > 0 {
		return m.validationResults[0]
	}
	return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
}

func (m *testMockValidationStrategy) ShouldRetry(result agent.ValidationResult) bool {
	if m.shouldRetryFunc != nil {
		return m.shouldRetryFunc(result)
	}
	return false
}

func (m *testMockValidationStrategy) GenerateRecovery(
	ctx context.Context,
	request, response, sessionID string,
	result agent.ValidationResult,
	llm claude.LLM,
) string {
	if m.generateRecoveryFunc != nil {
		return m.generateRecoveryFunc(ctx, request, response, sessionID, result, llm)
	}
	return ""
}

// Test mocks to avoid import cycle

type testMockLLM struct {
	mu        sync.Mutex
	responses []claude.LLMResponse
	errors    []error
	callCount int
}

func (m *testMockLLM) Query(_ context.Context, _ string, _ string) (*claude.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("no more responses")
	}

	resp := m.responses[m.callCount]
	var err error
	if m.callCount < len(m.errors) {
		err = m.errors[m.callCount]
	}
	m.callCount++
	return &resp, err
}

type testMockMessenger struct {
	mu           sync.Mutex
	sentMessages []struct {
		to   string
		text string
	}
}

func (m *testMockMessenger) Send(_ context.Context, to, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = append(m.sentMessages, struct {
		to   string
		text string
	}{to: to, text: text})
	return nil
}

func (m *testMockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

func (m *testMockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

type testMockSessionManager struct {
	sessionID string
}

func (m *testMockSessionManager) GetOrCreateSession(_ string) string {
	return m.sessionID
}

func (m *testMockSessionManager) GetSessionHistory(_ string) []conversation.Message {
	return []conversation.Message{}
}

func (m *testMockSessionManager) ExpireSessions(_ time.Time) int {
	return 0
}

func (m *testMockSessionManager) GetLastSessionID(_ string) string {
	return m.sessionID
}
