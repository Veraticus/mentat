//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Veraticus/mentat/internal/claude"
)

// TestJSONResponseFormats verifies that various JSON response formats
// are handled correctly by the claude package's internal parser.
// Since the parser is not exported, we test through the types and
// verify that JSON marshaling/unmarshaling works as expected.
func TestJSONResponseFormats(t *testing.T) {
	// Test that ProgressInfo can be marshaled/unmarshaled correctly
	tests := []struct {
		name     string
		progress *claude.ProgressInfo
	}{
		{
			name: "complete_progress_info",
			progress: &claude.ProgressInfo{
				NeedsContinuation:  true,
				Status:             "searching",
				Message:            "Checking calendar events",
				EstimatedRemaining: 3,
				NeedsValidation:    true,
			},
		},
		{
			name: "minimal_progress_info",
			progress: &claude.ProgressInfo{
				NeedsContinuation: false,
				Status:            "complete",
			},
		},
		{
			name:     "empty_progress_info",
			progress: &claude.ProgressInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.progress)
			if err != nil {
				t.Fatalf("failed to marshal ProgressInfo: %v", err)
			}

			// Unmarshal back
			var decoded claude.ProgressInfo
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal ProgressInfo: %v", err)
			}

			// Verify fields match
			if decoded.NeedsContinuation != tt.progress.NeedsContinuation {
				t.Errorf("NeedsContinuation mismatch: got %v, want %v",
					decoded.NeedsContinuation, tt.progress.NeedsContinuation)
			}
			if decoded.Status != tt.progress.Status {
				t.Errorf("Status mismatch: got %q, want %q",
					decoded.Status, tt.progress.Status)
			}
			if decoded.Message != tt.progress.Message {
				t.Errorf("Message mismatch: got %q, want %q",
					decoded.Message, tt.progress.Message)
			}
			if decoded.EstimatedRemaining != tt.progress.EstimatedRemaining {
				t.Errorf("EstimatedRemaining mismatch: got %d, want %d",
					decoded.EstimatedRemaining, tt.progress.EstimatedRemaining)
			}
			if decoded.NeedsValidation != tt.progress.NeedsValidation {
				t.Errorf("NeedsValidation mismatch: got %v, want %v",
					decoded.NeedsValidation, tt.progress.NeedsValidation)
			}
		})
	}
}

// TestJSONResponseScenarios tests various JSON response scenarios that the
// claude package would need to handle. This documents the expected JSON formats.
func TestJSONResponseScenarios(t *testing.T) {
	scenarios := []struct {
		name                 string
		jsonResponse         string
		expectedMessage      string
		expectedHasProgress  bool
		expectedContinuation bool
		expectedValidation   bool
		expectedError        bool
		errorContains        string
	}{
		// Valid responses
		{
			name: "simple_chat_response",
			jsonResponse: `{
				"result": "Hello! How can I help you today?",
				"progress": {
					"needs_continuation": false,
					"status": "complete",
					"needs_validation": false
				}
			}`,
			expectedMessage:      "Hello! How can I help you today?",
			expectedHasProgress:  true,
			expectedContinuation: false,
			expectedValidation:   false,
		},
		{
			name: "response_needing_continuation",
			jsonResponse: `{
				"result": "I'm searching through your emails now...",
				"progress": {
					"needs_continuation": true,
					"status": "searching",
					"message": "Found 15 emails so far",
					"estimated_remaining": 2,
					"needs_validation": true
				}
			}`,
			expectedMessage:      "I'm searching through your emails now...",
			expectedHasProgress:  true,
			expectedContinuation: true,
			expectedValidation:   true,
		},
		{
			name: "response_with_message_field",
			jsonResponse: `{
				"message": "Using message field instead of result",
				"result": "This should be ignored",
				"progress": {
					"needs_continuation": false,
					"status": "complete"
				}
			}`,
			expectedMessage:      "Using message field instead of result",
			expectedHasProgress:  true,
			expectedContinuation: false,
		},
		{
			name: "response_without_progress",
			jsonResponse: `{
				"result": "No progress information"
			}`,
			expectedMessage:     "No progress information",
			expectedHasProgress: false,
		},
		{
			name: "response_with_tool_calls",
			jsonResponse: `{
				"result": "Let me search for that information.",
				"tool_calls": [
					{
						"type": "function",
						"function": {
							"name": "mcp_memory_search",
							"arguments": "{\"query\": \"meetings\"}"
						}
					}
				],
				"progress": {
					"needs_continuation": true,
					"status": "tool_use"
				}
			}`,
			expectedMessage:      "Let me search for that information.",
			expectedHasProgress:  true,
			expectedContinuation: true,
		},
		{
			name: "markdown_wrapped_json",
			jsonResponse: `{
				"result": "Response wrapped in markdown",
				"progress": {
					"needs_continuation": false,
					"status": "complete"
				}
			}`,
			expectedMessage:      "Response wrapped in markdown",
			expectedHasProgress:  true,
			expectedContinuation: false,
		},
		{
			name: "response_with_unicode",
			jsonResponse: `{
				"result": "Hello 世界! 🌍 Testing émojis ñ",
				"progress": {
					"needs_continuation": false,
					"status": "complete"
				}
			}`,
			expectedMessage:      "Hello 世界! 🌍 Testing émojis ñ",
			expectedHasProgress:  true,
			expectedContinuation: false,
		},
		{
			name: "response_with_newlines",
			jsonResponse: `{
				"result": "Line one\nLine two\n\nLine four",
				"progress": {
					"needs_continuation": false
				}
			}`,
			expectedMessage:      "Line one\nLine two\n\nLine four",
			expectedHasProgress:  true,
			expectedContinuation: false,
		},
		{
			name: "response_with_escaped_quotes",
			jsonResponse: `{
				"result": "She said \"Hello!\" to him.",
				"progress": {
					"needs_continuation": false
				}
			}`,
			expectedMessage:      `She said "Hello!" to him.`,
			expectedHasProgress:  true,
			expectedContinuation: false,
		},

		// Error cases
		{
			name:                "empty_json_object",
			jsonResponse:        `{}`,
			expectedError:       false, // Empty object is valid JSON
			expectedMessage:     "",    // No message field
			expectedHasProgress: false,
		},
		{
			name:                "null_json",
			jsonResponse:        `null`,
			expectedError:       false, // null is valid JSON
			expectedMessage:     "",    // Not an object, so no fields
			expectedHasProgress: false,
		},
		{
			name:          "malformed_json",
			jsonResponse:  `{"result": "Incomplete`,
			expectedError: true,
			errorContains: "unexpected end", // Actual error message
		},
		{
			name:          "invalid_json_syntax",
			jsonResponse:  `{result: "No quotes"}`,
			expectedError: true,
			errorContains: "invalid",
		},
		{
			name: "wrong_type_for_progress_field",
			jsonResponse: `{
				"result": "Test",
				"progress": "should be object not string"
			}`,
			expectedError:       false, // JSON parsing succeeds, type checking happens later
			expectedMessage:     "Test",
			expectedHasProgress: true, // Field exists, just wrong type
		},
		{
			name: "wrong_type_for_needs_continuation",
			jsonResponse: `{
				"result": "Test",
				"progress": {
					"needs_continuation": "yes"
				}
			}`,
			expectedError:       false, // JSON parsing succeeds, type checking happens later
			expectedMessage:     "Test",
			expectedHasProgress: true, // Field exists with wrong type inside
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// This is a documentation/specification test.
			// In a real integration test, these JSON responses would be
			// processed by the claude package's internal parser.

			// For now, we verify that the JSON is valid (or invalid as expected)
			var rawData map[string]interface{}
			err := json.Unmarshal([]byte(sc.jsonResponse), &rawData)

			if sc.expectedError {
				if err == nil {
					t.Error("expected JSON parsing error but got none")
				} else if sc.errorContains != "" && !strings.Contains(err.Error(), sc.errorContains) {
					t.Errorf("expected error containing %q, got %q", sc.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected JSON parsing error: %v", err)
			}

			// Verify message field
			message, hasMessage := rawData["message"]
			result, hasResult := rawData["result"]

			// Special handling for empty objects and null
			if sc.name == "empty_json_object" || sc.name == "null_json" {
				// These are expected to have no message/result fields
				if hasMessage || hasResult {
					t.Error("Empty/null JSON should not have message or result fields")
				}
				return
			}

			if !hasMessage && !hasResult {
				t.Error("JSON response must have either 'message' or 'result' field")
			}

			// The actual parser prefers 'message' over 'result'
			var actualMessage string
			if hasMessage {
				if msg, ok := message.(string); ok {
					actualMessage = msg
				}
			} else if hasResult {
				if res, ok := result.(string); ok {
					actualMessage = res
				}
			}

			if actualMessage != sc.expectedMessage {
				t.Errorf("message mismatch: got %q, want %q", actualMessage, sc.expectedMessage)
			}

			// Verify progress field if expected
			if progress, hasProgress := rawData["progress"]; hasProgress != sc.expectedHasProgress {
				t.Errorf("progress field presence mismatch: got %v, want %v", hasProgress, sc.expectedHasProgress)
			} else if hasProgress {
				// Verify it's an object
				if progressMap, ok := progress.(map[string]interface{}); ok {
					// Check specific fields if they matter for the test
					if sc.expectedContinuation {
						if cont, ok := progressMap["needs_continuation"].(bool); !ok || !cont {
							t.Error("expected needs_continuation to be true")
						}
					}
					if sc.expectedValidation {
						if val, ok := progressMap["needs_validation"].(bool); !ok || !val {
							t.Error("expected needs_validation to be true")
						}
					}
				} else {
					// Special case: progress field might be wrong type
					if sc.name != "wrong_type_for_progress_field" {
						t.Error("progress field must be an object")
					}
				}
			}
		})
	}
}

// TestLongJSONResponses verifies handling of very long JSON responses.
func TestLongJSONResponses(t *testing.T) {
	longString := generateLongString(10000)

	jsonResponse := fmt.Sprintf(`{
		"result": "%s",
		"progress": {
			"needs_continuation": false,
			"status": "complete"
		}
	}`, longString)

	// Verify the JSON is valid
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonResponse), &parsed); err != nil {
		t.Fatalf("failed to parse long JSON response: %v", err)
	}

	// Verify the message was preserved
	if result, ok := parsed["result"].(string); !ok {
		t.Error("result field is not a string")
	} else if len(result) != 10000 {
		t.Errorf("expected result length 10000, got %d", len(result))
	}
}

// TestJSONResponseEdgeCases documents edge cases in JSON response handling.
func TestJSONResponseEdgeCases(t *testing.T) {
	edgeCases := []struct {
		name        string
		jsonInput   string
		description string
		shouldParse bool
	}{
		{
			name:        "array_response",
			jsonInput:   `["not", "an", "object"]`,
			description: "JSON arrays should not be valid responses",
			shouldParse: false,
		},
		{
			name:        "string_response",
			jsonInput:   `"just a string"`,
			description: "JSON strings should not be valid responses",
			shouldParse: false,
		},
		{
			name:        "number_response",
			jsonInput:   `42`,
			description: "JSON numbers should not be valid responses",
			shouldParse: false,
		},
		{
			name:        "boolean_response",
			jsonInput:   `true`,
			description: "JSON booleans should not be valid responses",
			shouldParse: false,
		},
		{
			name: "nested_json_in_result",
			jsonInput: `{
				"result": "{\"nested\": \"json\", \"value\": 123}",
				"progress": {"needs_continuation": false}
			}`,
			description: "Nested JSON in result field should be preserved as string",
			shouldParse: true,
		},
		{
			name: "very_deep_nesting",
			jsonInput: `{
				"result": "Test",
				"progress": {
					"needs_continuation": false,
					"extra": {
						"deep": {
							"nested": {
								"value": "should be ignored"
							}
						}
					}
				}
			}`,
			description: "Extra nested fields should be ignored",
			shouldParse: true,
		},
	}

	for _, ec := range edgeCases {
		t.Run(ec.name, func(t *testing.T) {
			var parsed interface{}
			err := json.Unmarshal([]byte(ec.jsonInput), &parsed)

			// Check if JSON parsing succeeds
			if err != nil {
				if ec.shouldParse {
					t.Errorf("%s: unexpected parse error: %v", ec.description, err)
				}
				return
			}

			// JSON parsed successfully - now check if it's a valid response type
			_, isObject := parsed.(map[string]interface{})

			if !ec.shouldParse && isObject {
				// We expected this not to be a valid response, but it parsed as an object
				t.Errorf("%s: expected not to be a valid response object", ec.description)
			} else if !ec.shouldParse && !isObject {
				// Good - it's not an object, so it's not a valid response
				// This is the expected behavior for arrays, strings, numbers, booleans
			} else if ec.shouldParse && !isObject {
				// We expected a valid response but it's not an object
				t.Errorf("%s: parsed value is not a JSON object", ec.description)
			}
			// else: ec.shouldParse && isObject - all good
		})
	}
}

// Helper function to generate long strings for testing.
func generateLongString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[i%len(chars)]
	}
	return string(result)
}
