package claude_test

import (
	"context"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
)

// TestNeedsValidationFieldPropagation verifies that needs_validation field is properly propagated.
func TestNeedsValidationFieldPropagation(t *testing.T) {
	tests := []struct {
		name                    string
		jsonResponse            string
		expectedNeedsValidation bool
		expectedHasProgress     bool
	}{
		{
			name: "simple query with needs_validation false",
			jsonResponse: `{
				"message": "Hello! How can I help you today?",
				"progress": {
					"needs_continuation": false,
					"status": "complete",
					"needs_validation": false
				},
				"metadata": {
					"model": "claude-3",
					"latency_ms": 100
				}
			}`,
			expectedNeedsValidation: false,
			expectedHasProgress:     true,
		},
		{
			name: "complex query with needs_validation true",
			jsonResponse: `{
				"message": "I'll help you find all meetings and send emails.",
				"progress": {
					"needs_continuation": false,
					"status": "complete",
					"needs_validation": true
				},
				"metadata": {
					"model": "claude-3",
					"latency_ms": 200
				}
			}`,
			expectedNeedsValidation: true,
			expectedHasProgress:     true,
		},
		{
			name: "progress without needs_validation defaults to false",
			jsonResponse: `{
				"message": "Processing your request.",
				"progress": {
					"needs_continuation": false,
					"status": "complete"
				},
				"metadata": {
					"model": "claude-3",
					"latency_ms": 150
				}
			}`,
			expectedNeedsValidation: false,
			expectedHasProgress:     true,
		},
		{
			name: "no progress info",
			jsonResponse: `{
				"message": "Simple response without progress.",
				"metadata": {
					"model": "claude-3",
					"latency_ms": 50
				}
			}`,
			expectedNeedsValidation: false,
			expectedHasProgress:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock command runner
			mockRunner := &mockCommandRunner{
				output: tt.jsonResponse,
			}

			// Create client with mock runner
			client, err := claude.NewClient(claude.Config{
				Command: "/usr/bin/claude",
				Timeout: 30 * time.Second,
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Set the mock runner
			client.SetCommandRunner(mockRunner)

			// Query the client
			response, err := client.Query(context.Background(), "test prompt", "test-session")
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}

			// Verify progress exists or not
			if tt.expectedHasProgress && response.Progress == nil {
				t.Error("Expected progress info but got nil")
			}
			if !tt.expectedHasProgress && response.Progress != nil {
				t.Error("Expected no progress info but got one")
			}

			// Verify needs_validation field
			if response.Progress != nil {
				if response.Progress.NeedsValidation != tt.expectedNeedsValidation {
					t.Errorf("NeedsValidation = %v, want %v",
						response.Progress.NeedsValidation, tt.expectedNeedsValidation)
				}
			}
		})
	}
}

// mockCommandRunner is a mock implementation for testing.
type mockCommandRunner struct {
	output string
	err    error
}

func (m *mockCommandRunner) RunCommandContext(_ context.Context, _ string, _ ...string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.output, nil
}
