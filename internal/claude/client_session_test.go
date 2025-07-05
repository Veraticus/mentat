package claude_test

import (
	"context"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
)

// mockCommandRunnerWithSession simulates Claude CLI responses with session IDs.
type mockCommandRunnerWithSession struct {
	responses []string
	calls     []string
}

func (m *mockCommandRunnerWithSession) RunCommandContext(_ context.Context, _ string, args ...string) (string, error) {
	// Record the call
	m.calls = append(m.calls, args...)

	// Return the next response
	if len(m.responses) > 0 {
		resp := m.responses[0]
		m.responses = m.responses[1:]
		return resp, nil
	}

	// Default response
	return `{
		"type": "result",
		"is_error": false,
		"result": "Default response",
		"session_id": "default-session-id"
	}`, nil
}

func TestSessionIDPersistence(t *testing.T) {
	tests := []struct {
		name              string
		firstResponse     string
		secondResponse    string
		mentatSessionID   string
		expectResumeFlag  bool
		expectedSessionID string
	}{
		{
			name: "first call creates new session",
			firstResponse: `{
				"type": "result",
				"is_error": false,
				"result": "Hi! I'm Claude.",
				"session_id": "claude-session-123"
			}`,
			mentatSessionID:  "signal-+17734804810",
			expectResumeFlag: false, // First call should not have --resume
		},
		{
			name: "second call uses stored session",
			firstResponse: `{
				"type": "result",
				"is_error": false,
				"result": "Hi! I'm Claude.",
				"session_id": "claude-session-456"
			}`,
			secondResponse: `{
				"type": "result",
				"is_error": false,
				"result": "Yes, I remember you!",
				"session_id": "claude-session-456"
			}`,
			mentatSessionID:   "signal-+17734804810",
			expectResumeFlag:  true,
			expectedSessionID: "claude-session-456",
		},
		{
			name: "different mentat session gets different claude session",
			firstResponse: `{
				"type": "result",
				"is_error": false,
				"result": "Hi user 1!",
				"session_id": "claude-session-user1"
			}`,
			mentatSessionID:  "signal-user1",
			expectResumeFlag: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock command runner
			mockRunner := &mockCommandRunnerWithSession{
				responses: []string{tc.firstResponse},
			}
			if tc.secondResponse != "" {
				mockRunner.responses = append(mockRunner.responses, tc.secondResponse)
			}

			// Create client
			config := claude.Config{
				Command:       "claude",
				MCPConfigPath: "/path/to/mcp.json",
				SystemPrompt:  "test prompt",
				Timeout:       30 * time.Second,
			}
			client, err := claude.NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			client.SetCommandRunner(mockRunner)

			// First call
			ctx := context.Background()
			resp1, err := client.Query(ctx, "Hello", tc.mentatSessionID)
			if err != nil {
				t.Fatalf("First query failed: %v", err)
			}

			// Verify first response
			if resp1.SessionID == "" {
				t.Error("Expected session ID in first response")
			}

			// Check if --resume was used in first call
			hasResumeInFirst := false
			for i, arg := range mockRunner.calls {
				if arg == "--resume" && i+1 < len(mockRunner.calls) {
					hasResumeInFirst = true
					break
				}
			}
			if hasResumeInFirst {
				t.Error("First call should not have --resume flag")
			}

			// If we have a second response, make another call
			//nolint:nestif // Test code with reasonable nesting
			if tc.secondResponse != "" {
				// Clear previous calls
				mockRunner.calls = []string{}

				// Second call with same mentat session
				resp2, err2 := client.Query(ctx, "Do you remember me?", tc.mentatSessionID)
				if err2 != nil {
					t.Fatalf("Second query failed: %v", err2)
				}

				// Verify session ID is maintained
				if resp2.SessionID != tc.expectedSessionID {
					t.Errorf("Expected session ID %s, got %s", tc.expectedSessionID, resp2.SessionID)
				}

				// Check if --resume was used with correct session ID
				foundResume := false
				for i, arg := range mockRunner.calls {
					if arg == "--resume" && i+1 < len(mockRunner.calls) {
						foundResume = true
						if mockRunner.calls[i+1] != tc.expectedSessionID {
							t.Errorf("Expected --resume %s, got --resume %s",
								tc.expectedSessionID, mockRunner.calls[i+1])
						}
						break
					}
				}

				if tc.expectResumeFlag && !foundResume {
					t.Error("Expected --resume flag in second call")
				}
			}
		})
	}
}

func TestMultipleSessionsIsolation(t *testing.T) {
	// Create mock command runner
	mockRunner := &mockCommandRunnerWithSession{
		responses: []string{
			`{"type": "result", "is_error": false, "result": "Hi user 1!", "session_id": "session-1"}`,
			`{"type": "result", "is_error": false, "result": "Hi user 2!", "session_id": "session-2"}`,
			`{"type": "result", "is_error": false, "result": "Welcome back user 1!", "session_id": "session-1"}`,
			`{"type": "result", "is_error": false, "result": "Welcome back user 2!", "session_id": "session-2"}`,
		},
	}

	// Create client
	config := claude.Config{
		Command:       "claude",
		MCPConfigPath: "/path/to/mcp.json",
		SystemPrompt:  "test prompt",
		Timeout:       30 * time.Second,
	}
	client, err := claude.NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.SetCommandRunner(mockRunner)

	ctx := context.Background()

	// User 1 - first message
	resp1_1, err := client.Query(ctx, "Hello from user 1", "signal-user1")
	if err != nil {
		t.Fatalf("User 1 first query failed: %v", err)
	}
	if resp1_1.SessionID != "session-1" {
		t.Errorf("Expected session-1 for user 1, got %s", resp1_1.SessionID)
	}

	// User 2 - first message
	resp2_1, err := client.Query(ctx, "Hello from user 2", "signal-user2")
	if err != nil {
		t.Fatalf("User 2 first query failed: %v", err)
	}
	if resp2_1.SessionID != "session-2" {
		t.Errorf("Expected session-2 for user 2, got %s", resp2_1.SessionID)
	}

	// Clear call history
	mockRunner.calls = []string{}

	// User 1 - second message (should resume session-1)
	resp1_2, err := client.Query(ctx, "Another message from user 1", "signal-user1")
	if err != nil {
		t.Fatalf("User 1 second query failed: %v", err)
	}

	// Check that user 1 resumed with session-1
	foundUser1Resume := false
	for i, arg := range mockRunner.calls {
		if arg == "--resume" && i+1 < len(mockRunner.calls) && mockRunner.calls[i+1] == "session-1" {
			foundUser1Resume = true
			break
		}
	}
	if !foundUser1Resume {
		t.Error("Expected user 1 to resume with session-1")
	}

	// Clear call history
	mockRunner.calls = []string{}

	// User 2 - second message (should resume session-2)
	resp2_2, err := client.Query(ctx, "Another message from user 2", "signal-user2")
	if err != nil {
		t.Fatalf("User 2 second query failed: %v", err)
	}

	// Check that user 2 resumed with session-2
	foundUser2Resume := false
	for i, arg := range mockRunner.calls {
		if arg == "--resume" && i+1 < len(mockRunner.calls) && mockRunner.calls[i+1] == "session-2" {
			foundUser2Resume = true
			break
		}
	}
	if !foundUser2Resume {
		t.Error("Expected user 2 to resume with session-2")
	}

	// Verify responses maintain correct session IDs
	if resp1_2.SessionID != "session-1" {
		t.Errorf("User 1 second response should have session-1, got %s", resp1_2.SessionID)
	}
	if resp2_2.SessionID != "session-2" {
		t.Errorf("User 2 second response should have session-2, got %s", resp2_2.SessionID)
	}
}
