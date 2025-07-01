package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Veraticus/mentat/internal/claude"
)

// testLLM is a simple test implementation of claude.LLM.
type testLLM struct {
	responses map[string]*claude.LLMResponse
	errors    map[string]error
}

func newTestLLM() *testLLM {
	return &testLLM{
		responses: make(map[string]*claude.LLMResponse),
		errors:    make(map[string]error),
	}
}

func (t *testLLM) Query(_ context.Context, _ string, _ string) (*claude.LLMResponse, error) {
	// Check for error first
	if err, ok := t.errors["*"]; ok && err != nil {
		return nil, err
	}

	// Return response if set
	if resp, ok := t.responses["*"]; ok {
		return resp, nil
	}

	// Default response
	return &claude.LLMResponse{
		Message: "",
	}, nil
}

func (t *testLLM) setResponse(key string, message string) {
	t.responses[key] = &claude.LLMResponse{
		Message: message,
	}
}

func (t *testLLM) setError(key string, err error) {
	t.errors[key] = err
}

func TestMultiAgentValidator_Validate(t *testing.T) {
	tests := []struct {
		name               string
		request            string
		response           string
		llmResponse        string
		llmError           error
		expectedStatus     ValidationStatus
		expectedIssues     int
		expectedConfidence float64
		expectError        bool
	}{
		{
			name:     "successful validation",
			request:  "What's the weather today?",
			response: "The weather today is sunny with a high of 72°F.",
			llmResponse: `STATUS: SUCCESS
CONFIDENCE: 0.9
ISSUES: none
SUGGESTIONS: none`,
			expectedStatus:     ValidationStatusSuccess,
			expectedIssues:     0,
			expectedConfidence: 0.9,
		},
		{
			name:     "partial validation",
			request:  "Schedule a meeting and send an email",
			response: "I've scheduled the meeting for tomorrow at 2pm.",
			llmResponse: `STATUS: PARTIAL
CONFIDENCE: 0.6
ISSUES: email not sent, missing email content
SUGGESTIONS: ask for email details, send the email`,
			expectedStatus:     ValidationStatusPartial,
			expectedIssues:     2,
			expectedConfidence: 0.6,
		},
		{
			name:     "failed validation",
			request:  "Book a flight to Paris",
			response: "I cannot book flights.",
			llmResponse: `STATUS: FAILED
CONFIDENCE: 0.95
ISSUES: unable to complete request, missing capability
SUGGESTIONS: suggest alternative booking methods`,
			expectedStatus:     ValidationStatusFailed,
			expectedIssues:     2,
			expectedConfidence: 0.95,
		},
		{
			name:     "incomplete search validation",
			request:  "When is my next meeting with John?",
			response: "I don't see any meetings with John.",
			llmResponse: `STATUS: INCOMPLETE_SEARCH
CONFIDENCE: 0.7
ISSUES: didn't check memory tool, didn't check calendar
SUGGESTIONS: use memory tool, check calendar for John meetings`,
			expectedStatus:     ValidationStatusIncompleteSearch,
			expectedIssues:     2,
			expectedConfidence: 0.7,
		},
		{
			name:               "LLM error handling",
			request:            "Test request",
			response:           "Test response",
			llmError:           errors.New("LLM connection failed"),
			expectedStatus:     ValidationStatusUnclear,
			expectedIssues:     1,
			expectedConfidence: 0.0,
		},
		{
			name:     "malformed response handling",
			request:  "Test request",
			response: "Test response",
			llmResponse: `This is not a properly formatted validation response.
It doesn't follow the expected format.`,
			expectedStatus:     ValidationStatusUnclear,
			expectedIssues:     0,
			expectedConfidence: 0.5,
		},
		{
			name:     "case insensitive status parsing",
			request:  "Test request",
			response: "Test response",
			llmResponse: `status: success
confidence: 0.85
issues: None
suggestions: NONE`,
			expectedStatus:     ValidationStatusSuccess,
			expectedIssues:     0,
			expectedConfidence: 0.85,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewMultiAgentValidator()
			mockLLM := newTestLLM()

			if tt.llmError != nil {
				mockLLM.setError("*", tt.llmError)
			} else {
				mockLLM.setResponse("*", tt.llmResponse)
			}

			result := validator.Validate(context.Background(), tt.request, tt.response, mockLLM)

			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %v, got %v", tt.expectedStatus, result.Status)
			}

			if len(result.Issues) != tt.expectedIssues {
				t.Errorf("expected %d issues, got %d: %v", tt.expectedIssues, len(result.Issues), result.Issues)
			}

			if result.Confidence != tt.expectedConfidence {
				t.Errorf("expected confidence %v, got %v", tt.expectedConfidence, result.Confidence)
			}

			if tt.expectError && result.Metadata["error"] == "" {
				t.Error("expected error in metadata, but none found")
			}
		})
	}
}

func TestMultiAgentValidator_ShouldRetry(t *testing.T) {
	validator := NewMultiAgentValidator()

	tests := []struct {
		name          string
		result        ValidationResult
		expectedRetry bool
	}{
		{
			name: "incomplete search should retry",
			result: ValidationResult{
				Status: ValidationStatusIncompleteSearch,
			},
			expectedRetry: true,
		},
		{
			name: "unclear with low confidence should retry",
			result: ValidationResult{
				Status:     ValidationStatusUnclear,
				Confidence: 0.2,
			},
			expectedRetry: true,
		},
		{
			name: "unclear with high confidence should not retry",
			result: ValidationResult{
				Status:     ValidationStatusUnclear,
				Confidence: 0.7,
			},
			expectedRetry: false,
		},
		{
			name: "success should not retry",
			result: ValidationResult{
				Status: ValidationStatusSuccess,
			},
			expectedRetry: false,
		},
		{
			name: "failed should not retry",
			result: ValidationResult{
				Status: ValidationStatusFailed,
			},
			expectedRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRetry := validator.ShouldRetry(tt.result)
			if shouldRetry != tt.expectedRetry {
				t.Errorf("expected retry=%v, got %v", tt.expectedRetry, shouldRetry)
			}
		})
	}
}

func TestMultiAgentValidator_GenerateRecovery(t *testing.T) {
	validator := NewMultiAgentValidator()
	ctx := context.Background()

	tests := []struct {
		name             string
		request          string
		response         string
		result           ValidationResult
		llmResponse      string
		llmError         error
		expectedContains string
	}{
		{
			name:     "partial success recovery",
			request:  "Schedule meeting and send email",
			response: "Meeting scheduled",
			result: ValidationResult{
				Status: ValidationStatusPartial,
				Issues: []string{"email not sent"},
			},
			llmResponse:      "I scheduled the meeting but wasn't able to send the email.",
			expectedContains: "scheduled the meeting",
		},
		{
			name:     "partial success with LLM error",
			request:  "Test request",
			response: "Test response",
			result: ValidationResult{
				Status: ValidationStatusPartial,
				Issues: []string{"some issue"},
			},
			llmError:         errors.New("LLM failed"),
			expectedContains: "part of your request",
		},
		{
			name:     "failed validation recovery",
			request:  "Book a flight",
			response: "Cannot book flights",
			result: ValidationResult{
				Status: ValidationStatusFailed,
				Issues: []string{"missing capability", "service unavailable"},
			},
			expectedContains: "missing capability and service unavailable",
		},
		{
			name:     "failed validation no issues",
			request:  "Test request",
			response: "Test response",
			result: ValidationResult{
				Status: ValidationStatusFailed,
				Issues: []string{},
			},
			expectedContains: "technical difficulties",
		},
		{
			name:     "unclear validation recovery",
			request:  "Complex request",
			response: "Unclear response",
			result: ValidationResult{
				Status: ValidationStatusUnclear,
			},
			expectedContains: "having trouble",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLLM := newTestLLM()

			if tt.llmError != nil {
				mockLLM.setError("*", tt.llmError)
			} else if tt.llmResponse != "" {
				mockLLM.setResponse("*", tt.llmResponse)
			}

			recovery := validator.GenerateRecovery(ctx, tt.request, tt.response, tt.result, mockLLM)

			if !strings.Contains(recovery, tt.expectedContains) {
				t.Errorf("expected recovery to contain '%s', got: %s", tt.expectedContains, recovery)
			}
		})
	}
}

func TestSimpleValidator_Validate(t *testing.T) {
	tests := []struct {
		name               string
		request            string
		response           string
		expectedStatus     ValidationStatus
		expectedIssues     []string
		expectedConfidence float64
	}{
		{
			name:               "successful response",
			request:            "Schedule a meeting",
			response:           "I've scheduled the meeting for tomorrow at 2pm.",
			expectedStatus:     ValidationStatusSuccess,
			expectedIssues:     []string{},
			expectedConfidence: 0.8,
		},
		{
			name:               "response too short",
			request:            "What's the weather?",
			response:           "Error",
			expectedStatus:     ValidationStatusFailed,
			expectedIssues:     []string{"response too short"},
			expectedConfidence: 0.9,
		},
		{
			name:               "error indicators",
			request:            "Book a flight",
			response:           "I'm sorry, I cannot book flights. I'm unable to access that service.",
			expectedStatus:     ValidationStatusFailed,
			expectedIssues:     []string{"response contains error indicators"},
			expectedConfidence: 0.7,
		},
		{
			name:               "partial success",
			request:            "Do something",
			response:           "I completed part of the task but encountered an error with the rest.",
			expectedStatus:     ValidationStatusPartial,
			expectedIssues:     []string{},
			expectedConfidence: 0.6,
		},
		{
			name:               "unclear response",
			request:            "Complex request",
			response:           "This is a response without clear indicators.",
			expectedStatus:     ValidationStatusUnclear,
			expectedIssues:     []string{},
			expectedConfidence: 0.4,
		},
		{
			name:               "response with questions",
			request:            "Send an email",
			response:           "I've sent the email. Did you want me to do anything else?",
			expectedStatus:     ValidationStatusPartial,
			expectedIssues:     []string{"response contains questions"},
			expectedConfidence: 0.64, // 0.8 * 0.8
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewSimpleValidator()
			result := validator.Validate(context.Background(), tt.request, tt.response, nil)

			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %v, got %v", tt.expectedStatus, result.Status)
			}

			if len(result.Issues) != len(tt.expectedIssues) {
				t.Errorf("expected %d issues, got %d", len(tt.expectedIssues), len(result.Issues))
			}

			for i, issue := range tt.expectedIssues {
				if i < len(result.Issues) && result.Issues[i] != issue {
					t.Errorf("expected issue '%s', got '%s'", issue, result.Issues[i])
				}
			}

			// Allow small floating point differences
			if diff := result.Confidence - tt.expectedConfidence; diff > 0.01 || diff < -0.01 {
				t.Errorf("expected confidence %v, got %v", tt.expectedConfidence, result.Confidence)
			}
		})
	}
}

func TestSimpleValidator_ShouldRetry(t *testing.T) {
	validator := NewSimpleValidator()

	// SimpleValidator should never suggest retries
	results := []ValidationResult{
		{Status: ValidationStatusSuccess},
		{Status: ValidationStatusFailed},
		{Status: ValidationStatusPartial},
		{Status: ValidationStatusUnclear},
		{Status: ValidationStatusIncompleteSearch},
	}

	for _, result := range results {
		if validator.ShouldRetry(result) {
			t.Errorf("SimpleValidator should never suggest retry, but did for status %v", result.Status)
		}
	}
}

func TestSimpleValidator_GenerateRecovery(t *testing.T) {
	validator := NewSimpleValidator()
	ctx := context.Background()

	tests := []struct {
		name            string
		result          ValidationResult
		expectedMessage string
	}{
		{
			name:            "failed recovery",
			result:          ValidationResult{Status: ValidationStatusFailed},
			expectedMessage: "I encountered an issue with that request. Please try again or rephrase your question.",
		},
		{
			name:            "partial recovery",
			result:          ValidationResult{Status: ValidationStatusPartial},
			expectedMessage: "I was able to partially complete your request. Let me know if you need anything else.",
		},
		{
			name:            "unclear recovery",
			result:          ValidationResult{Status: ValidationStatusUnclear},
			expectedMessage: "I'm not certain I fully addressed your request. Could you clarify what you need?",
		},
		{
			name:            "success no recovery",
			result:          ValidationResult{Status: ValidationStatusSuccess},
			expectedMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recovery := validator.GenerateRecovery(ctx, "", "", tt.result, nil)
			if recovery != tt.expectedMessage {
				t.Errorf("expected '%s', got '%s'", tt.expectedMessage, recovery)
			}
		})
	}
}

func TestNoopValidator(t *testing.T) {
	validator := NewNoopValidator()
	ctx := context.Background()

	t.Run("always returns success", func(t *testing.T) {
		result := validator.Validate(ctx, "any request", "any response", nil)

		if result.Status != ValidationStatusSuccess {
			t.Errorf("expected SUCCESS, got %v", result.Status)
		}

		if result.Confidence != 1.0 {
			t.Errorf("expected confidence 1.0, got %v", result.Confidence)
		}

		if len(result.Issues) != 0 {
			t.Errorf("expected no issues, got %v", result.Issues)
		}

		if len(result.Suggestions) != 0 {
			t.Errorf("expected no suggestions, got %v", result.Suggestions)
		}

		if result.Metadata["validator"] != "noop" {
			t.Errorf("expected validator metadata 'noop', got %v", result.Metadata["validator"])
		}
	})

	t.Run("never suggests retry", func(t *testing.T) {
		// Even with a "failed" result passed in, should return false
		result := ValidationResult{Status: ValidationStatusFailed}
		if validator.ShouldRetry(result) {
			t.Error("NoopValidator should never suggest retry")
		}
	})

	t.Run("always returns empty recovery", func(t *testing.T) {
		result := ValidationResult{Status: ValidationStatusFailed}
		recovery := validator.GenerateRecovery(ctx, "request", "response", result, nil)
		if recovery != "" {
			t.Errorf("expected empty recovery, got '%s'", recovery)
		}
	})
}

func TestParseList(t *testing.T) {
	validator := NewMultiAgentValidator()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "comma separated list",
			input:    "issue1, issue2, issue3",
			expected: []string{"issue1", "issue2", "issue3"},
		},
		{
			name:     "list with extra spaces",
			input:    "  issue1  ,   issue2  ,  issue3  ",
			expected: []string{"issue1", "issue2", "issue3"},
		},
		{
			name:     "single item",
			input:    "single issue",
			expected: []string{"single issue"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "list with empty items",
			input:    "issue1,,issue2,  ,issue3",
			expected: []string{"issue1", "issue2", "issue3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.parseList(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d items, got %d", len(tt.expected), len(result))
				return
			}

			for i, item := range tt.expected {
				if result[i] != item {
					t.Errorf("expected item[%d]='%s', got '%s'", i, item, result[i])
				}
			}
		})
	}
}

func TestParseConfidence(t *testing.T) {
	validator := NewMultiAgentValidator()

	tests := []struct {
		name     string
		input    string
		expected float64
		hasError bool
	}{
		{
			name:     "valid confidence",
			input:    "0.85",
			expected: 0.85,
			hasError: false,
		},
		{
			name:     "confidence at lower bound",
			input:    "0.0",
			expected: 0.0,
			hasError: false,
		},
		{
			name:     "confidence at upper bound",
			input:    "1.0",
			expected: 1.0,
			hasError: false,
		},
		{
			name:     "confidence below range",
			input:    "-0.5",
			expected: 0.0,
			hasError: false,
		},
		{
			name:     "confidence above range",
			input:    "1.5",
			expected: 1.0,
			hasError: false,
		},
		{
			name:     "invalid format",
			input:    "not a number",
			expected: 0,
			hasError: true,
		},
		{
			name:     "percentage format",
			input:    "85%",
			expected: 0,
			hasError: true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.parseConfidence(tt.input)

			if tt.hasError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.hasError && result != tt.expected {
				t.Errorf("expected confidence %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestParseIssuesAndSuggestions(t *testing.T) {
	validator := NewMultiAgentValidator()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "multiple items",
			input:    "issue1, issue2, issue3",
			expected: []string{"issue1", "issue2", "issue3"},
		},
		{
			name:     "none value",
			input:    "none",
			expected: []string{},
		},
		{
			name:     "NONE uppercase",
			input:    "NONE",
			expected: []string{},
		},
		{
			name:     "None mixed case",
			input:    "None",
			expected: []string{},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: []string{},
		},
		{
			name:     "single item",
			input:    "single issue",
			expected: []string{"single issue"},
		},
	}

	for _, tt := range tests {
		t.Run("parseIssues_"+tt.name, func(t *testing.T) {
			result := validator.parseIssues(tt.input)
			assertStringSlicesEqual(t, tt.expected, result, "issues")
		})

		t.Run("parseSuggestions_"+tt.name, func(t *testing.T) {
			result := validator.parseSuggestions(tt.input)
			assertStringSlicesEqual(t, tt.expected, result, "suggestions")
		})
	}
}

// assertStringSlicesEqual compares two string slices for equality
func assertStringSlicesEqual(t *testing.T, expected, actual []string, label string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("expected %d %s, got %d", len(expected), label, len(actual))
		return
	}

	for i, item := range expected {
		if actual[i] != item {
			t.Errorf("expected %s[%d]='%s', got '%s'", label, i, item, actual[i])
		}
	}
}

func TestParseValidationResponseEdgeCases(t *testing.T) {
	validator := NewMultiAgentValidator()

	tests := []struct {
		name               string
		response           string
		expectedStatus     ValidationStatus
		expectedConfidence float64
		expectedIssues     int
	}{
		{
			name:               "empty response",
			response:           "",
			expectedStatus:     ValidationStatusUnclear,
			expectedConfidence: 0.5,
			expectedIssues:     0,
		},
		{
			name: "response with extra colons",
			response: `STATUS: SUCCESS: with extra info
CONFIDENCE: 0.9: high
ISSUES: issue1: with colon, issue2
SUGGESTIONS: none`,
			expectedStatus:     ValidationStatusSuccess,
			expectedConfidence: 0.9,
			expectedIssues:     2,
		},
		{
			name: "response with no colons",
			response: `STATUS SUCCESS
CONFIDENCE 0.9
ISSUES none
SUGGESTIONS none`,
			expectedStatus:     ValidationStatusUnclear,
			expectedConfidence: 0.5,
			expectedIssues:     0,
		},
		{
			name: "response with mixed formatting",
			response: `Some preamble text
status: partial
More text here
CONFIDENCE: 0.65
Random line
issues: missing data, incomplete request
suggestions: retry with more context`,
			expectedStatus:     ValidationStatusPartial,
			expectedConfidence: 0.65,
			expectedIssues:     2,
		},
		{
			name: "duplicate keys (last wins)",
			response: `STATUS: SUCCESS
CONFIDENCE: 0.8
STATUS: FAILED
CONFIDENCE: 0.2`,
			expectedStatus:     ValidationStatusFailed,
			expectedConfidence: 0.2,
			expectedIssues:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.parseValidationResponse(tt.response)

			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %v, got %v", tt.expectedStatus, result.Status)
			}

			if result.Confidence != tt.expectedConfidence {
				t.Errorf("expected confidence %v, got %v", tt.expectedConfidence, result.Confidence)
			}

			if len(result.Issues) != tt.expectedIssues {
				t.Errorf("expected %d issues, got %d: %v", tt.expectedIssues, len(result.Issues), result.Issues)
			}

			if result.Metadata["raw_response"] != tt.response {
				t.Error("raw response not stored in metadata")
			}
		})
	}
}
