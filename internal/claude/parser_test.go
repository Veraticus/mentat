package claude //nolint:testpackage // Need access to unexported functions

import (
	"strings"
	"testing"
)

func TestStructuredJSONResponse(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedMessage  string
		expectedProgress *jsonProgressInfo
		expectedError    bool
	}{
		{
			name: "valid JSON response with message and progress",
			input: `{
				"message": "I found 3 meetings on your calendar today.",
				"progress": {
					"needs_continuation": false,
					"status": "complete",
					"message": "Calendar check completed",
					"estimated_remaining": 0
				}
			}`,
			expectedMessage: "I found 3 meetings on your calendar today.",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  false,
				Status:             "complete",
				Message:            "Calendar check completed",
				EstimatedRemaining: 0,
			},
			expectedError: false,
		},
		{
			name:            "JSON wrapped in markdown code blocks",
			input:           "```json\n{\n  \"message\": \"Hello Josh! I'm Claude, your personal assistant.\",\n  \"progress\": {\n    \"needs_continuation\": false,\n    \"status\": \"complete\",\n    \"message\": \"Introduction completed\",\n    \"estimated_remaining\": 0,\n    \"needs_validation\": false\n  }\n}\n```",
			expectedMessage: "Hello Josh! I'm Claude, your personal assistant.",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  false,
				Status:             "complete",
				Message:            "Introduction completed",
				EstimatedRemaining: 0,
				NeedsValidation:    false,
			},
			expectedError: false,
		},
		{
			name:            "JSON wrapped in code blocks without language tag",
			input:           "```\n{\n  \"message\": \"Let me check that for you.\",\n  \"progress\": {\n    \"needs_continuation\": true,\n    \"status\": \"searching\",\n    \"estimated_remaining\": 1\n  }\n}\n```",
			expectedMessage: "Let me check that for you.",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  true,
				Status:             "searching",
				EstimatedRemaining: 1,
			},
			expectedError: false,
		},
		{
			name: "JSON response needing continuation",
			input: `{
				"message": "I'm searching through your emails for updates. Found 15 so far...",
				"progress": {
					"needs_continuation": true,
					"status": "searching",
					"message": "Scanning remaining folders",
					"estimated_remaining": 2
				}
			}`,
			expectedMessage: "I'm searching through your emails for updates. Found 15 so far...",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  true,
				Status:             "searching",
				Message:            "Scanning remaining folders",
				EstimatedRemaining: 2,
			},
			expectedError: false,
		},
		{
			name: "JSON response with missing progress field",
			input: `{
				"message": "Hello! How can I help you today?"
			}`,
			expectedMessage:  "Hello! How can I help you today?",
			expectedProgress: nil,
			expectedError:    false,
		},
		{
			name: "malformed JSON response",
			input: `{
				"message": "This is invalid JSON
				"progress": {
					"status": "error"
				}
			}`,
			expectedMessage:  "",
			expectedProgress: nil,
			expectedError:    true,
		},
		{
			name: "JSON response with extra fields",
			input: `{
				"message": "Task completed successfully.",
				"progress": {
					"needs_continuation": false,
					"status": "complete",
					"message": "All done",
					"estimated_remaining": 0,
					"extra_field": "ignored"
				},
				"tool_calls": [],
				"metadata": {"latency_ms": 123}
			}`,
			expectedMessage: "Task completed successfully.",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  false,
				Status:             "complete",
				Message:            "All done",
				EstimatedRemaining: 0,
			},
			expectedError: false,
		},
		{
			name: "JSON response with empty message",
			input: `{
				"message": "",
				"progress": {
					"needs_continuation": false,
					"status": "complete"
				}
			}`,
			expectedMessage: "",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  false,
				Status:             "complete",
				Message:            "",
				EstimatedRemaining: 0,
			},
			expectedError: false,
		},
		{
			name: "JSON response with progress fields in different order",
			input: `{
				"progress": {
					"estimated_remaining": 1,
					"message": "Processing request",
					"status": "working",
					"needs_continuation": true
				},
				"message": "I'm working on your request."
			}`,
			expectedMessage: "I'm working on your request.",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  true,
				Status:             "working",
				Message:            "Processing request",
				EstimatedRemaining: 1,
			},
			expectedError: false,
		},
		{
			name:             "plain text response fallback",
			input:            "This is a plain text response without JSON structure.",
			expectedMessage:  "This is a plain text response without JSON structure.",
			expectedProgress: nil,
			expectedError:    false,
		},
		{
			name: "JSON response with null progress",
			input: `{
				"message": "Simple response",
				"progress": null
			}`,
			expectedMessage:  "Simple response",
			expectedProgress: nil,
			expectedError:    false,
		},
		{
			name: "JSON response with partial progress fields",
			input: `{
				"message": "Partial progress info",
				"progress": {
					"needs_continuation": true,
					"status": "processing"
				}
			}`,
			expectedMessage: "Partial progress info",
			expectedProgress: &jsonProgressInfo{
				NeedsContinuation:  true,
				Status:             "processing",
				Message:            "",
				EstimatedRemaining: 0,
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseResponse(tt.input)

			// Check error
			if tt.expectedError {
				if err == nil {
					t.Errorf("parseResponse() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Errorf("parseResponse() unexpected error = %v", err)
				return
			}

			// Check message
			actualMessage := resp.Message
			if actualMessage == "" {
				actualMessage = resp.Result
			}
			if actualMessage != tt.expectedMessage {
				t.Errorf("parseResponse() message = %q, want %q", actualMessage, tt.expectedMessage)
			}

			// Check progress
			compareProgress(t, resp.Progress, tt.expectedProgress)
		})
	}
}

func compareProgress(t *testing.T, got, want *jsonProgressInfo) {
	t.Helper()

	if want == nil {
		if got != nil {
			t.Errorf("extractProgressJSON() progress = %+v, want nil", got)
		}
		return
	}

	if got == nil {
		t.Errorf("extractProgressJSON() progress = nil, want %+v", want)
		return
	}

	if got.NeedsContinuation != want.NeedsContinuation {
		t.Errorf("NeedsContinuation = %v, want %v", got.NeedsContinuation, want.NeedsContinuation)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
	if got.Message != want.Message {
		t.Errorf("Message = %q, want %q", got.Message, want.Message)
	}
	if got.EstimatedRemaining != want.EstimatedRemaining {
		t.Errorf("EstimatedRemaining = %d, want %d", got.EstimatedRemaining, want.EstimatedRemaining)
	}
}

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedMsg    string
		expectedErr    bool
		expectedErrMsg string
		hasProgress    bool
	}{
		{
			name: "valid JSON with message and progress",
			input: `{
  "message": "I found your meeting details.",
  "progress": {
    "needs_continuation": false,
    "status": "complete",
    "message": "Search completed",
    "estimated_remaining": 0
  },
  "metadata": {
    "model": "claude-3.5-sonnet",
    "latency_ms": 1234
  }
}`,
			expectedMsg: "I found your meeting details.",
			expectedErr: false,
			hasProgress: true,
		},
		{
			name: "valid JSON with result field (Claude Code style)",
			input: `{
  "result": "Task completed successfully.",
  "type": "text",
  "is_error": false
}`,
			expectedMsg: "Task completed successfully.",
			expectedErr: false,
			hasProgress: false,
		},
		{
			name:           "invalid JSON",
			input:          `{invalid json`,
			expectedErr:    true,
			expectedErrMsg: "failed to parse JSON response",
		},
		{
			name:        "JSON missing message and result",
			input:       `{"metadata": {"model": "test"}}`,
			expectedErr: false,
			expectedMsg: "",
			hasProgress: false,
		},
		{
			name: "plain text response",
			input: `This is plain text response
without JSON structure`,
			expectedMsg: "This is plain text response without JSON structure",
			expectedErr: false,
			hasProgress: false,
		},
		{
			name:           "error in output",
			input:          "Error: Authentication failed",
			expectedErr:    true,
			expectedErrMsg: "claude CLI error: Error: Authentication failed",
		},
		{
			name:           "empty output",
			input:          "",
			expectedErr:    true,
			expectedErrMsg: "claude returned empty or unparseable response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseResponse(tt.input)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("parseResponse() error = nil, want error containing %q", tt.expectedErrMsg)
				} else if !strings.Contains(err.Error(), tt.expectedErrMsg) {
					t.Errorf("parseResponse() error = %v, want error containing %q", err, tt.expectedErrMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("parseResponse() unexpected error = %v", err)
				return
			}

			// Check message content
			actualMsg := resp.Message
			if actualMsg == "" {
				actualMsg = resp.Result
			}

			if actualMsg != tt.expectedMsg {
				t.Errorf("parseResponse() message = %q, want %q", actualMsg, tt.expectedMsg)
			}

			// Check progress presence
			if tt.hasProgress && resp.Progress == nil {
				t.Errorf("parseResponse() expected progress but got nil")
			} else if !tt.hasProgress && resp.Progress != nil {
				t.Errorf("parseResponse() unexpected progress = %+v", resp.Progress)
			}
		})
	}
}

func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty output",
			input:    "",
			expected: "command produced no output",
		},
		{
			name:     "simple error message",
			input:    "Error: File not found",
			expected: "Error: File not found",
		},
		{
			name: "multi-line error with details",
			input: `Warning: something minor
Error: Connection timeout
Failed to connect to server
Additional context here`,
			expected: "Error: Connection timeout; Failed to connect to server",
		},
		{
			name:     "permission denied",
			input:    "bash: /usr/bin/claude: Permission denied",
			expected: "bash: /usr/bin/claude: Permission denied",
		},
		{
			name:     "timeout error",
			input:    "Operation timed out after 30 seconds",
			expected: "Operation timed out after 30 seconds",
		},
		{
			name: "no explicit error keywords",
			input: `Just some output
Without error keywords`,
			expected: "Without error keywords",
		},
		{
			name:     "whitespace only",
			input:    "   \n\t  \n   ",
			expected: "command produced no output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractErrorMessage(tt.input)
			if result != tt.expected {
				t.Errorf("extractErrorMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}
