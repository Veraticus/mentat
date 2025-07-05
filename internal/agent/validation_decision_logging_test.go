package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// createTestLogger creates a logger for testing that writes to the provided buffer.
func createTestLogger(buf *bytes.Buffer) *slog.Logger {
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	//nolint:forbidigo // slog.New is acceptable in test utilities
	logger := slog.New(handler)
	return logger
}

// TestValidationDecisionLogging tests that validation decisions are logged correctly.
func TestValidationDecisionLogging(t *testing.T) {
	tests := []struct {
		name                     string
		message                  string
		progressInfo             *claude.ProgressInfo
		expectedValidationReason string
		expectedNeedsValidation  bool
		expectedProgressNil      bool
		expectedComplexity       string
		expectAsyncValidation    bool
	}{
		{
			name:    "simple query skips validation",
			message: "Hi!",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   false,
			},
			expectedValidationReason: "simple_query",
			expectedNeedsValidation:  false,
			expectedProgressNil:      false,
			expectedComplexity:       "simple",
			expectAsyncValidation:    false,
		},
		{
			name:    "complex query with needs_validation true",
			message: "Find all my meetings tomorrow and email the participants",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   true,
			},
			expectedValidationReason: "needs_validation_flag_true",
			expectedNeedsValidation:  true,
			expectedProgressNil:      false,
			expectedComplexity:       "complex",
			expectAsyncValidation:    true,
		},
		{
			name:                     "nil progress info triggers validation",
			message:                  "What's the weather?",
			progressInfo:             nil,
			expectedValidationReason: "progress_nil_defaulting_to_validation",
			expectedNeedsValidation:  true,
			expectedProgressNil:      true,
			expectedComplexity:       "complex",
			expectAsyncValidation:    true,
		},
		{
			name:    "query with continuation and validation",
			message: "Complex multi-step task",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   true,
			},
			expectedValidationReason: "needs_validation_flag_true",
			expectedNeedsValidation:  true,
			expectedProgressNil:      false,
			expectedComplexity:       "complex",
			expectAsyncValidation:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture logs
			var logBuf bytes.Buffer
			logger := createTestLogger(&logBuf)

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
				agent.WithLogger(logger),
			)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Configure mock LLM response
			mockLLM.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				return &claude.LLMResponse{
					Message:  "Test response",
					Progress: tt.progressInfo,
					Metadata: claude.ResponseMetadata{
						ModelVersion: "claude-3",
						Latency:      100 * time.Millisecond,
					},
				}, nil
			}

			// Process the message
			ctx := context.Background()
			msg := signal.IncomingMessage{
				From: "+1234567890",
				Text: tt.message,
			}

			err = handler.Process(ctx, msg)
			if err != nil {
				t.Fatalf("Process failed: %v", err)
			}

			// Give async operations time to complete
			time.Sleep(50 * time.Millisecond)

			// Parse and verify logs
			logs := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
			validationDecisionFound := false
			asyncValidationFound := false

			for _, logLine := range logs {
				var logEntry map[string]any
				if unmarshalErr := json.Unmarshal([]byte(logLine), &logEntry); unmarshalErr != nil {
					continue
				}

				msgValue, msgOk := logEntry["msg"].(string)
				if !msgOk {
					continue
				}

				// Check for validation decision log
				if strings.Contains(msgValue, "validation decision:") {
					validationDecisionFound = true
					verifyValidationDecisionFields(t, logEntry, tt)
				}

				// Check for async validation launch log
				if msgValue == "launching async validation" {
					asyncValidationFound = true
					verifyLaunchReason(t, logEntry, tt.expectedProgressNil)
				}
			}

			if !validationDecisionFound {
				t.Error("No validation decision log found")
			}

			if tt.expectAsyncValidation && !asyncValidationFound {
				t.Error("Expected async validation log but none found")
			} else if !tt.expectAsyncValidation && asyncValidationFound {
				t.Error("Unexpected async validation log found")
			}
		})
	}
}

// TestValidationDecisionQueryMethod tests validation decision logging in Query method.
func TestValidationDecisionQueryMethod(t *testing.T) {
	tests := []struct {
		name                     string
		query                    string
		progressInfo             *claude.ProgressInfo
		expectedValidationReason string
		expectedLogged           bool
	}{
		{
			name:  "simple query in Query method",
			query: "Thanks!",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   false,
			},
			expectedValidationReason: "simple_query",
			expectedLogged:           true,
		},
		{
			name:  "complex query in Query method",
			query: "Analyze my calendar and suggest optimizations",
			progressInfo: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
				NeedsValidation:   true,
			},
			expectedValidationReason: "needs_validation_flag_true",
			expectedLogged:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture logs
			var logBuf bytes.Buffer
			logger := createTestLogger(&logBuf)

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
				agent.WithLogger(logger),
			)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Configure mock LLM response
			mockLLM.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				return &claude.LLMResponse{
					Message:  "Test response",
					Progress: tt.progressInfo,
					Metadata: claude.ResponseMetadata{
						ModelVersion: "claude-3",
						Latency:      100 * time.Millisecond,
					},
				}, nil
			}

			// Call Query method
			ctx := context.Background()
			_, err = handler.Query(ctx, tt.query, "test-session-123")
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}

			// Parse and verify logs
			logs := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
			validationDecisionFound := false

			for _, logLine := range logs {
				var logEntry map[string]any
				if unmarshalErr := json.Unmarshal([]byte(logLine), &logEntry); unmarshalErr != nil {
					continue
				}

				msgValue, msgOk := logEntry["msg"].(string)
				if !msgOk {
					continue
				}

				// Check for validation decision log
				if strings.Contains(msgValue, "validation decision:") {
					validationDecisionFound = true

					// Verify validation reason
					if reason, reasonOk := logEntry["validation_reason"].(string); reasonOk {
						if reason != tt.expectedValidationReason {
							t.Errorf("Expected validation_reason=%q, got %q", tt.expectedValidationReason, reason)
						}
					}
				}
			}

			if tt.expectedLogged && !validationDecisionFound {
				t.Error("Expected validation decision log but none found")
			}
		})
	}
}

// verifyValidationDecisionFields verifies the validation decision log fields.
func verifyValidationDecisionFields(t *testing.T, logEntry map[string]any, tt struct {
	name                     string
	message                  string
	progressInfo             *claude.ProgressInfo
	expectedValidationReason string
	expectedNeedsValidation  bool
	expectedProgressNil      bool
	expectedComplexity       string
	expectAsyncValidation    bool
}) {
	t.Helper()

	// Verify expected fields
	if reason, reasonOk := logEntry["validation_reason"].(string); reasonOk {
		if reason != tt.expectedValidationReason {
			t.Errorf("Expected validation_reason=%q, got %q", tt.expectedValidationReason, reason)
		}
	} else {
		t.Error("validation_reason field not found in log")
	}

	if progressNil, nilOk := logEntry["progress_nil"].(bool); nilOk {
		if progressNil != tt.expectedProgressNil {
			t.Errorf("Expected progress_nil=%v, got %v", tt.expectedProgressNil, progressNil)
		}
	} else {
		t.Error("progress_nil field not found in log")
	}

	if needsValidation, validationOk := logEntry["needs_validation_flag"].(bool); validationOk {
		if needsValidation != tt.expectedNeedsValidation {
			t.Errorf("Expected needs_validation_flag=%v, got %v",
				tt.expectedNeedsValidation, needsValidation)
		}
	} else {
		t.Error("needs_validation_flag field not found in log")
	}

	if complexity, complexityOk := logEntry["query_complexity"].(string); complexityOk {
		if complexity != tt.expectedComplexity {
			t.Errorf("Expected query_complexity=%q, got %q", tt.expectedComplexity, complexity)
		}
	} else {
		t.Error("query_complexity field not found in log")
	}
}

// verifyLaunchReason verifies the launch reason in async validation logs.
func verifyLaunchReason(t *testing.T, logEntry map[string]any, expectedProgressNil bool) {
	t.Helper()

	if launchReason, launchOk := logEntry["launch_reason"].(string); launchOk {
		expectedLaunchReason := "needs_validation_flag_true"
		if expectedProgressNil {
			expectedLaunchReason = "progress_nil_requires_validation"
		}
		if launchReason != expectedLaunchReason {
			t.Errorf("Expected launch_reason=%q, got %q", expectedLaunchReason, launchReason)
		}
	}
}
