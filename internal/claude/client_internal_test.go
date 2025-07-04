package claude

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockCommandRunner is a test implementation of commandRunner.
type mockCommandRunner struct {
	response string
	err      error
	calls    []mockCall
}

type mockCall struct {
	name string
	args []string
}

func (m *mockCommandRunner) RunCommandContext(_ context.Context, name string, args ...string) (string, error) {
	m.calls = append(m.calls, mockCall{name: name, args: args})
	return m.response, m.err
}

// checkProgressInfo is a helper function to compare ProgressInfo structs.
func checkProgressInfo(t *testing.T, got, want *ProgressInfo) {
	t.Helper()

	if want == nil {
		if got != nil {
			t.Errorf("Progress = %+v, want nil", got)
		}
		return
	}

	if got == nil {
		t.Errorf("Progress = nil, want %+v", want)
		return
	}

	if got.NeedsContinuation != want.NeedsContinuation {
		t.Errorf("Progress.NeedsContinuation = %v, want %v", got.NeedsContinuation, want.NeedsContinuation)
	}
	if got.Status != want.Status {
		t.Errorf("Progress.Status = %q, want %q", got.Status, want.Status)
	}
	if got.Message != want.Message {
		t.Errorf("Progress.Message = %q, want %q", got.Message, want.Message)
	}
	if got.EstimatedRemaining != want.EstimatedRemaining {
		t.Errorf("Progress.EstimatedRemaining = %d, want %d", got.EstimatedRemaining, want.EstimatedRemaining)
	}
}

// TestParseProgressInfo tests parsing of progress information from Claude responses.
func TestParseProgressInfo(t *testing.T) {
	tests := []struct {
		name         string
		jsonResponse string
		wantProgress *ProgressInfo
		wantMessage  string
	}{
		{
			name: "response with progress - needs continuation",
			jsonResponse: `{
				"result": "I'm searching through your emails for that information...",
				"progress": {
					"needs_continuation": true,
					"status": "searching",
					"message": "Searching email archive",
					"estimated_remaining": 2
				},
				"metadata": {
					"model": "claude-3-sonnet",
					"latency_ms": 1500
				}
			}`,
			wantProgress: &ProgressInfo{
				NeedsContinuation:  true,
				Status:             "searching",
				Message:            "Searching email archive",
				EstimatedRemaining: 2,
			},
			wantMessage: "I'm searching through your emails for that information...",
		},
		{
			name: "response with progress - complete",
			jsonResponse: `{
				"result": "I found the information you requested.",
				"progress": {
					"needs_continuation": false,
					"status": "complete",
					"message": "Search completed",
					"estimated_remaining": 0
				},
				"metadata": {
					"model": "claude-3-sonnet",
					"latency_ms": 2000
				}
			}`,
			wantProgress: &ProgressInfo{
				NeedsContinuation:  false,
				Status:             "complete",
				Message:            "Search completed",
				EstimatedRemaining: 0,
			},
			wantMessage: "I found the information you requested.",
		},
		{
			name: "response without progress",
			jsonResponse: `{
				"result": "The weather today is sunny with a high of 75°F.",
				"metadata": {
					"model": "claude-3-sonnet",
					"latency_ms": 800
				}
			}`,
			wantProgress: nil,
			wantMessage:  "The weather today is sunny with a high of 75°F.",
		},
		{
			name: "response with partial progress info",
			jsonResponse: `{
				"result": "Processing your request...",
				"progress": {
					"needs_continuation": true,
					"status": "processing"
				},
				"metadata": {
					"model": "claude-3-sonnet",
					"latency_ms": 1000
				}
			}`,
			wantProgress: &ProgressInfo{
				NeedsContinuation:  true,
				Status:             "processing",
				Message:            "",
				EstimatedRemaining: 0,
			},
			wantMessage: "Processing your request...",
		},
		{
			name: "response with null progress - should handle gracefully",
			jsonResponse: `{
				"result": "Here's the information",
				"progress": null,
				"metadata": {
					"model": "claude-3-sonnet",
					"latency_ms": 1000
				}
			}`,
			wantProgress: nil,
			wantMessage:  "Here's the information",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client with mock runner
			client, err := NewClient(Config{
				Command:       "claude",
				MCPConfigPath: "/test/mcp.json",
				Timeout:       30 * time.Second,
			})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			// Set mock command runner
			mockRunner := &mockCommandRunner{
				response: tt.jsonResponse,
			}
			client.SetCommandRunner(mockRunner)

			// Query the client
			response, err := client.Query(context.Background(), "test prompt", "test-session")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check message
			if response.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", response.Message, tt.wantMessage)
			}

			// Check progress
			checkProgressInfo(t, response.Progress, tt.wantProgress)
		})
	}
}

// TestProgressInfoWithToolCalls tests progress info alongside tool calls.
func TestProgressInfoWithToolCalls(t *testing.T) {
	jsonResponse := `{
		"result": "Let me check your calendar for tomorrow's events...",
		"tool_calls": [
			{
				"name": "mcp_google_calendar_list_events",
				"parameters": {
					"date": {
						"type": "string",
						"value": "2024-12-20"
					}
				}
			}
		],
		"progress": {
			"needs_continuation": true,
			"status": "fetching_calendar",
			"message": "Retrieving calendar events",
			"estimated_remaining": 1
		},
		"metadata": {
			"model": "claude-3-sonnet",
			"latency_ms": 1200
		}
	}`

	client, err := NewClient(Config{
		Command:       "claude",
		MCPConfigPath: "/test/mcp.json",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	mockRunner := &mockCommandRunner{
		response: jsonResponse,
	}
	client.SetCommandRunner(mockRunner)

	response, err := client.Query(context.Background(), "What's on my calendar tomorrow?", "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that we have both tool calls and progress
	if len(response.ToolCalls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(response.ToolCalls))
	}

	if response.Progress == nil {
		t.Error("Expected progress info, got nil")
	} else {
		if !response.Progress.NeedsContinuation {
			t.Error("Expected needs_continuation = true")
		}
		if response.Progress.Status != "fetching_calendar" {
			t.Errorf("Expected status = 'fetching_calendar', got %q", response.Progress.Status)
		}
	}
}

// TestProgressInfoErrorCases tests error handling with progress info.
func TestProgressInfoErrorCases(t *testing.T) {
	tests := []struct {
		name         string
		jsonResponse string
		wantErr      bool
	}{
		{
			name:         "completely invalid JSON",
			jsonResponse: `{invalid json`,
			wantErr:      false, // Falls back to plain text parsing, so no error
		},
		{
			name: "empty response with progress",
			jsonResponse: `{
				"result": "",
				"message": "",
				"progress": {
					"needs_continuation": true,
					"status": "processing"
				}
			}`,
			wantErr: false, // Should handle empty message gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(Config{
				Command: "claude",
				Timeout: 30 * time.Second,
			})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			mockRunner := &mockCommandRunner{
				response: tt.jsonResponse,
			}
			client.SetCommandRunner(mockRunner)

			_, err = client.Query(context.Background(), "test", "test-session")
			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			} else if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestProgressInfoIntegration tests a realistic multi-step scenario.
func TestProgressInfoIntegration(t *testing.T) {
	// Simulate a multi-step operation
	responses := []string{
		// Step 1: Initial response with progress
		`{
			"result": "I'll help you find that information. Let me search through your emails first...",
			"progress": {
				"needs_continuation": true,
				"status": "searching_emails",
				"message": "Searching email archive",
				"estimated_remaining": 2
			},
			"metadata": {"model": "claude-3-sonnet", "latency_ms": 1000}
		}`,
		// Step 2: Intermediate progress
		`{
			"result": "Found some relevant emails. Now checking your calendar for related events...",
			"progress": {
				"needs_continuation": true,
				"status": "checking_calendar",
				"message": "Cross-referencing with calendar",
				"estimated_remaining": 1
			},
			"metadata": {"model": "claude-3-sonnet", "latency_ms": 1500}
		}`,
		// Step 3: Final response
		`{
			"result": "Based on my search, here's what I found: The meeting is scheduled for 2 PM tomorrow.",
			"progress": {
				"needs_continuation": false,
				"status": "complete",
				"message": "Search completed successfully",
				"estimated_remaining": 0
			},
			"metadata": {"model": "claude-3-sonnet", "latency_ms": 800}
		}`,
	}

	client, err := NewClient(Config{
		Command: "claude",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	for i, responseJSON := range responses {
		mockRunner := &mockCommandRunner{
			response: responseJSON,
		}
		client.SetCommandRunner(mockRunner)

		response, queryErr := client.Query(context.Background(), fmt.Sprintf("step %d", i+1), "test-session")
		if queryErr != nil {
			t.Fatalf("Step %d: unexpected error: %v", i+1, queryErr)
		}

		if response.Progress == nil {
			t.Fatalf("Step %d: expected progress info", i+1)
		}

		// Check continuation status
		expectedContinuation := i < len(responses)-1
		if response.Progress.NeedsContinuation != expectedContinuation {
			t.Errorf("Step %d: NeedsContinuation = %v, want %v",
				i+1, response.Progress.NeedsContinuation, expectedContinuation)
		}

		// Check estimated remaining
		expectedRemaining := len(responses) - i - 1
		if response.Progress.EstimatedRemaining != expectedRemaining {
			t.Errorf("Step %d: EstimatedRemaining = %d, want %d",
				i+1, response.Progress.EstimatedRemaining, expectedRemaining)
		}
	}
}
