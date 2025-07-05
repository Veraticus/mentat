package agent_test

import (
	"strings"
	"testing"

	"github.com/Veraticus/mentat/internal/agent"
)

func TestGenerateCorrectionMessage(t *testing.T) {
	tests := []struct {
		name     string
		result   agent.ValidationResult
		wantText string
		notWant  string
	}{
		{
			name: "success status returns empty",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusSuccess,
			},
			wantText: "",
		},
		{
			name: "partial completion with no issues",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusPartial,
				Issues: []string{},
			},
			wantText: "Oops! I was only able to partially complete that.",
		},
		{
			name: "partial completion with single issue",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusPartial,
				Issues: []string{"couldn't find the meeting"},
			},
			wantText: "couldn't find the meeting",
		},
		{
			name: "partial completion with metadata",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusPartial,
				Metadata: map[string]string{
					"completed": "check your calendar",
					"failed":    "send the email",
				},
				Suggestions: []string{"Try again later"},
			},
			wantText: "Oops! I managed to check your calendar, but I couldn't send the email. Try again later",
		},
		{
			name: "failed with no issues",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{},
			},
			wantText: "Oops! Something went wrong, but I'm not sure exactly what",
		},
		{
			name: "failed with permission issue",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"permission denied for calendar access"},
			},
			wantText: "Oops! I don't have permission to access",
		},
		{
			name: "failed with specific resource permission",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"access denied"},
				Metadata: map[string]string{
					"resource": "your Google Calendar",
				},
			},
			wantText: "your Google Calendar",
		},
		{
			name: "failed with timeout",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"operation timed out"},
			},
			wantText: "Oops! That took longer than expected and timed out",
		},
		{
			name: "failed with specific service timeout",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"timeout connecting"},
				Metadata: map[string]string{
					"service": "calendar API",
				},
			},
			wantText: "calendar API might be taking extra time",
		},
		{
			name: "failed with technical error",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"connection error: host unreachable"},
			},
			wantText: "technical issue: connection error",
		},
		{
			name: "failed with retry suggestion",
			result: agent.ValidationResult{
				Status:      agent.ValidationStatusFailed,
				Issues:      []string{"temporary network issue"},
				Suggestions: []string{"retry in a few seconds"},
			},
			wantText: "Feel free to ask again",
		},
		{
			name: "incomplete search with single tool",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusIncompleteSearch,
				Metadata: map[string]string{
					"expected_tools": "calendar",
				},
			},
			wantText: "should have checked your calendar",
		},
		{
			name: "incomplete search with multiple tools",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusIncompleteSearch,
				Metadata: map[string]string{
					"expected_tools": "memory,calendar,tasks",
				},
			},
			wantText: "our previous conversations, your calendar, and your task list",
		},
		{
			name: "incomplete search generic",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusIncompleteSearch,
			},
			wantText: "should have checked some additional information",
		},
		{
			name: "unclear request",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusUnclear,
			},
			wantText: "I'm not quite sure I understood what you need",
		},
		{
			name: "unknown status defaults to empty",
			result: agent.ValidationResult{
				Status: agent.ValidationStatus("UNKNOWN"),
			},
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.GenerateCorrectionMessage(tt.result)

			if tt.wantText != "" && !strings.Contains(got, tt.wantText) {
				t.Errorf("GenerateCorrectionMessage() = %q, want to contain %q", got, tt.wantText)
			}

			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("GenerateCorrectionMessage() = %q, should not contain %q", got, tt.notWant)
			}

			// Special case for empty result
			if tt.wantText == "" && got != "" {
				t.Errorf("GenerateCorrectionMessage() = %q, want empty string", got)
			}
		})
	}
}

// TestGenerateCorrectionMessage_Integration tests the exported function
// The internal helper functions are tested indirectly through this

func TestNaturalLanguageInMessages(t *testing.T) {
	// Test that all generated messages feel natural
	testCases := []agent.ValidationResult{
		{
			Status: agent.ValidationStatusPartial,
			Issues: []string{"couldn't access calendar", "email server down"},
		},
		{
			Status: agent.ValidationStatusFailed,
			Issues: []string{"permission denied"},
		},
		{
			Status: agent.ValidationStatusIncompleteSearch,
			Metadata: map[string]string{
				"expected_tools": "memory,calendar",
			},
		},
	}

	for _, result := range testCases {
		msg := agent.GenerateCorrectionMessage(result)

		// Check for natural language patterns
		hasNaturalOpening := strings.Contains(msg, "Oops!") || strings.Contains(msg, "Hmm,") || msg == ""
		if !hasNaturalOpening {
			t.Errorf("Message lacks natural opening: %q", msg)
		}

		// Check that technical jargon is avoided
		lowerMsg := strings.ToLower(msg)
		technicalTerms := []string{"validation", "status", "metadata", "llm", "api"}
		for _, term := range technicalTerms {
			if strings.Contains(lowerMsg, term) {
				t.Errorf("Message contains technical term %q: %q", term, msg)
			}
		}
	}
}

func TestGenerateCorrectionPrompt(t *testing.T) {
	tests := []struct {
		name            string
		result          agent.ValidationResult
		request         string
		response        string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "partial completion prompt",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusPartial,
				Issues: []string{"calendar API timeout"},
			},
			request:  "Schedule a meeting and send invites",
			response: "I scheduled the meeting for tomorrow at 2pm",
			wantContains: []string{
				"partial completion",
				"Schedule a meeting and send invites",
				"calendar API timeout",
				"Oops!",
				"Acknowledges what was successfully completed",
			},
		},
		{
			name: "failed validation prompt",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"permission denied", "invalid credentials"},
			},
			request:  "Check my email",
			response: "",
			wantContains: []string{
				"request failed",
				"Check my email",
				"permission denied, invalid credentials",
				"Oops!",
				"user-friendly terms",
			},
		},
		{
			name: "incomplete search prompt",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusIncompleteSearch,
				Metadata: map[string]string{
					"expected_tools": "calendar,memory",
				},
			},
			request:  "What did we discuss in our last meeting?",
			response: "I don't have that information",
			wantContains: []string{
				"should have used additional tools",
				"What did we discuss",
				"calendar,memory",
				"more thorough search",
			},
		},
		{
			name: "unclear request prompt",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusUnclear,
			},
			request:  "Do the thing",
			response: "",
			wantContains: []string{
				"asking for clarification",
				"Do the thing",
				"Hmm,",
				"rephrase",
			},
		},
		{
			name: "success returns empty",
			result: agent.ValidationResult{
				Status: agent.ValidationStatusSuccess,
			},
			request:      "Test",
			response:     "Done",
			wantContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := agent.GenerateCorrectionPrompt(tt.result, tt.request, tt.response)

			// For success case, we expect empty prompt
			if tt.result.Status == agent.ValidationStatusSuccess {
				if prompt != "" {
					t.Errorf("Expected empty prompt for success status, got: %q", prompt)
				}
				return
			}

			// Check expected content
			for _, expected := range tt.wantContains {
				if !strings.Contains(prompt, expected) {
					t.Errorf("Prompt missing expected content %q\nGot: %s", expected, prompt)
				}
			}

			// Check unwanted content
			for _, unwanted := range tt.wantNotContains {
				if strings.Contains(prompt, unwanted) {
					t.Errorf("Prompt contains unwanted content %q", unwanted)
				}
			}
		})
	}
}
