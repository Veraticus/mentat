package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/signal"
)

// TestHandlerWithComplexityGuidance tests that the handler properly integrates
// complexity analysis and adds guidance to complex requests.
func TestHandlerWithComplexityGuidance(t *testing.T) {
	tests := []struct {
		name                 string
		message              string
		complexityScore      float64
		complexitySteps      int
		factors              []agent.ComplexityFactor
		wantGuidanceContains []string
	}{
		{
			name:            "simple request no guidance",
			message:         "What time is it?",
			complexityScore: 0.2,
			complexitySteps: 1,
			factors:         []agent.ComplexityFactor{},
		},
		{
			name:            "medium complexity with guidance",
			message:         "Schedule a meeting with John next week",
			complexityScore: 0.6,
			complexitySteps: 2,
			factors: []agent.ComplexityFactor{
				{Type: agent.ComplexityFactorTemporal, Description: "next week"},
				{Type: agent.ComplexityFactorCoordination, Description: "with John"},
			},
			wantGuidanceContains: []string{
				"Time-related information might be relevant",
				"coordinating information about multiple people",
			},
		},
		{
			name:            "high complexity multi-step",
			message:         "Find all meetings next week, reschedule conflicts, and send updates",
			complexityScore: 0.8,
			complexitySteps: 5,
			factors: []agent.ComplexityFactor{
				{Type: agent.ComplexityFactorMultiStep, Description: "find, reschedule, send"},
				{Type: agent.ComplexityFactorTemporal, Description: "next week"},
				{Type: agent.ComplexityFactorDataIntegration, Description: "calendar and email"},
			},
			wantGuidanceContains: []string{
				"multiple steps",
				"breaking it down into logical parts",
				"Multiple sources of information",
				"Please be thorough",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test doubles
			mockLLM := &guidanceMockLLM{}
			mockLLM.queryFunc = func(_ context.Context, prompt, _ string) (*claude.LLMResponse, error) {
				// Store the prompt for verification
				mockLLM.lastPrompt = prompt
				return &claude.LLMResponse{
					Message: "Test response",
				}, nil
			}

			mockMessenger := &guidanceMockMessenger{
				sendFunc: func(_ context.Context, _, _ string) error {
					return nil
				},
			}

			mockSessionManager := &guidanceMockSessionManager{
				getOrCreateSessionFunc: func(_ string) string {
					return "test-session-123"
				},
				getSessionHistoryFunc: func(_ string) []conversation.Message {
					return []conversation.Message{}
				},
			}

			mockValidator := &guidanceMockValidationStrategy{
				validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
					return agent.ValidationResult{
						Status:     agent.ValidationStatusSuccess,
						Confidence: 1.0,
					}
				},
				shouldRetryFunc: func(_ agent.ValidationResult) bool {
					return false
				},
			}

			mockComplexityAnalyzer := &guidanceMockComplexityAnalyzer{
				analyzeFunc: func(_ string) agent.ComplexityResult {
					return agent.ComplexityResult{
						Score:   tt.complexityScore,
						Steps:   tt.complexitySteps,
						Factors: tt.factors,
					}
				},
			}

			// Create handler with complexity analyzer
			handler, err := agent.NewHandler(
				mockLLM,
				agent.WithValidationStrategy(mockValidator),
				agent.WithMessenger(mockMessenger),
				agent.WithSessionManager(mockSessionManager),
				agent.WithComplexityAnalyzer(mockComplexityAnalyzer),
			)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Process message
			ctx := context.Background()
			msg := signal.IncomingMessage{
				From: "test-user",
				Text: tt.message,
			}

			err = handler.Process(ctx, msg)
			if err != nil {
				t.Fatalf("Process failed: %v", err)
			}

			// Verify guidance was added for complex requests
			if len(tt.wantGuidanceContains) > 0 {
				// The prompt should contain the guidance
				for _, expected := range tt.wantGuidanceContains {
					if !strings.Contains(strings.ToLower(mockLLM.lastPrompt), strings.ToLower(expected)) {
						t.Errorf("Expected prompt to contain %q, got: %q", expected, mockLLM.lastPrompt)
					}
				}
			} else if mockLLM.lastPrompt != tt.message {
				// Simple requests should not have guidance added
				t.Errorf("Simple request should not have guidance. Expected %q, got %q", tt.message, mockLLM.lastPrompt)
			}
		})
	}
}

// Test doubles - prefixed with 'guidance' to avoid conflicts

type guidanceMockLLM struct {
	queryFunc  func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error)
	lastPrompt string
}

func (m *guidanceMockLLM) Query(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error) {
	return m.queryFunc(ctx, prompt, sessionID)
}

type guidanceMockMessenger struct {
	sendFunc                func(ctx context.Context, to, message string) error
	sendTypingIndicatorFunc func(ctx context.Context, to string) error
	subscribeFunc           func(ctx context.Context) (<-chan signal.IncomingMessage, error)
}

func (m *guidanceMockMessenger) Send(ctx context.Context, to, message string) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, to, message)
	}
	return nil
}

func (m *guidanceMockMessenger) SendTypingIndicator(ctx context.Context, to string) error {
	if m.sendTypingIndicatorFunc != nil {
		return m.sendTypingIndicatorFunc(ctx, to)
	}
	return nil
}

func (m *guidanceMockMessenger) Subscribe(ctx context.Context) (<-chan signal.IncomingMessage, error) {
	if m.subscribeFunc != nil {
		return m.subscribeFunc(ctx)
	}
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}

type guidanceMockSessionManager struct {
	getOrCreateSessionFunc func(userID string) string
	getSessionHistoryFunc  func(sessionID string) []conversation.Message
	updateSessionFunc      func(sessionID string, message conversation.Message)
	cleanupExpiredFunc     func()
	expireSessionsFunc     func(before time.Time) int
	getLastSessionIDFunc   func(userID string) string
}

func (m *guidanceMockSessionManager) GetOrCreateSession(userID string) string {
	if m.getOrCreateSessionFunc != nil {
		return m.getOrCreateSessionFunc(userID)
	}
	return "default-session"
}

func (m *guidanceMockSessionManager) GetSessionHistory(sessionID string) []conversation.Message {
	if m.getSessionHistoryFunc != nil {
		return m.getSessionHistoryFunc(sessionID)
	}
	return []conversation.Message{}
}

func (m *guidanceMockSessionManager) UpdateSession(sessionID string, message conversation.Message) {
	if m.updateSessionFunc != nil {
		m.updateSessionFunc(sessionID, message)
	}
}

func (m *guidanceMockSessionManager) CleanupExpired() {
	if m.cleanupExpiredFunc != nil {
		m.cleanupExpiredFunc()
	}
}

func (m *guidanceMockSessionManager) ExpireSessions(before time.Time) int {
	if m.expireSessionsFunc != nil {
		return m.expireSessionsFunc(before)
	}
	return 0
}

func (m *guidanceMockSessionManager) GetLastSessionID(userID string) string {
	if m.getLastSessionIDFunc != nil {
		return m.getLastSessionIDFunc(userID)
	}
	return ""
}

type guidanceMockValidationStrategy struct {
	validateFunc         func(ctx context.Context, request, response, sessionID string, llm claude.LLM) agent.ValidationResult
	shouldRetryFunc      func(result agent.ValidationResult) bool
	generateRecoveryFunc func(ctx context.Context, request, response, sessionID string, result agent.ValidationResult, llm claude.LLM) string
}

func (m *guidanceMockValidationStrategy) Validate(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) agent.ValidationResult {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, request, response, sessionID, llm)
	}
	return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
}

func (m *guidanceMockValidationStrategy) ShouldRetry(result agent.ValidationResult) bool {
	if m.shouldRetryFunc != nil {
		return m.shouldRetryFunc(result)
	}
	return false
}

func (m *guidanceMockValidationStrategy) GenerateRecovery(
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

type guidanceMockComplexityAnalyzer struct {
	analyzeFunc   func(request string) agent.ComplexityResult
	isComplexFunc func(request string) bool
}

func (m *guidanceMockComplexityAnalyzer) Analyze(request string) agent.ComplexityResult {
	if m.analyzeFunc != nil {
		return m.analyzeFunc(request)
	}
	return agent.ComplexityResult{}
}

func (m *guidanceMockComplexityAnalyzer) IsComplex(request string) bool {
	if m.isComplexFunc != nil {
		return m.isComplexFunc(request)
	}
	result := m.Analyze(request)
	return result.Score > 0.5 || result.RequiresDecomposition
}
