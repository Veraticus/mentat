//go:build integration
// +build integration

package main

import (
	"testing"

	"github.com/Veraticus/mentat/internal/config"
)

// TestSystemPromptIntegration verifies the embedded system prompt is valid.
func TestSystemPromptIntegration(t *testing.T) {
	// This test verifies that our embedded system prompt is valid
	prompt := embeddedSystemPrompt
	if err := config.ValidateSystemPrompt(prompt); err != nil {
		t.Fatalf("Invalid embedded system prompt: %v", err)
	}

	// Verify it's not empty
	if prompt == "" {
		t.Error("System prompt is empty")
	}

	// Verify it contains expected content
	expectedPhrases := []string{
		"personal assistant",
		"MCP tools",
		"Tool Usage is Mandatory",
		"Confirmation Specificity",
		"Multi-Source Information Gathering",
		"Progress Tracking",
		"needs_continuation",
		"progress",
	}

	for _, phrase := range expectedPhrases {
		if !contains(prompt, phrase) {
			t.Errorf("System prompt missing expected phrase: %q", phrase)
		}
	}

	t.Logf("Loaded system prompt with %d characters", len(prompt))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
