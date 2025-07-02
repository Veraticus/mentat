package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Veraticus/mentat/internal/config"
)

func TestLoadSystemPrompt(t *testing.T) {
	tests := []struct {
		name string
		test systemPromptTest
	}{
		{
			name: "loads valid prompt file",
			test: systemPromptTest{
				setup:          setupValidPromptFile,
				wantErr:        false,
				validatePrompt: validateSimplePrompt,
			},
		},
		{
			name: "fails on missing file",
			test: systemPromptTest{
				setup:       setupMissingFile,
				wantErr:     true,
				errContains: "system prompt file not found",
			},
		},
		{
			name: "fails on empty file",
			test: systemPromptTest{
				setup:       setupEmptyFile,
				wantErr:     true,
				errContains: "system prompt is empty",
			},
		},
		{
			name: "fails on whitespace-only file",
			test: systemPromptTest{
				setup:       setupWhitespaceFile,
				wantErr:     true,
				errContains: "system prompt is empty",
			},
		},
		{
			name: "handles multiline prompts",
			test: systemPromptTest{
				setup:          setupMultilinePromptFile,
				wantErr:        false,
				validatePrompt: validateMultilinePrompt,
			},
		},
		{
			name: "handles unreadable file",
			test: systemPromptTest{
				setup:       setupUnreadableFile,
				wantErr:     true,
				errContains: "failed to read system prompt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runLoadSystemPromptTest(t, tt.test)
		})
	}
}

// Helper functions for TestLoadSystemPrompt

func setupValidPromptFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system_prompt.txt")
	content := "You are a helpful AI assistant that helps with scheduling and tasks."
	if err := os.WriteFile(promptFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return promptFile
}

func validateSimplePrompt(t *testing.T, prompt string) {
	t.Helper()
	expected := "You are a helpful AI assistant that helps with scheduling and tasks."
	if prompt != expected {
		t.Errorf("got prompt %q, want %q", prompt, expected)
	}
}

func setupMissingFile(_ *testing.T) string {
	return "/nonexistent/system_prompt.txt"
}

func setupEmptyFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system_prompt.txt")
	if err := os.WriteFile(promptFile, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	return promptFile
}

func setupWhitespaceFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system_prompt.txt")
	if err := os.WriteFile(promptFile, []byte("   \n\t\n   "), 0600); err != nil {
		t.Fatal(err)
	}
	return promptFile
}

func setupMultilinePromptFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system_prompt.txt")
	content := `You are Mentat, a personal assistant.

You have access to:
- Calendar
- Email
- Task management

Always be helpful and concise.`
	if err := os.WriteFile(promptFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return promptFile
}

func validateMultilinePrompt(t *testing.T, prompt string) {
	t.Helper()
	if prompt == "" {
		t.Error("expected non-empty multiline prompt")
	}
	expected := `You are Mentat, a personal assistant.

You have access to:
- Calendar
- Email
- Task management

Always be helpful and concise.`
	if prompt != expected {
		t.Error("multiline prompt not preserved correctly")
	}
}

func setupUnreadableFile(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("cannot test permission errors as root")
	}
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "system_prompt.txt")
	if err := os.WriteFile(promptFile, []byte("test"), 0000); err != nil {
		t.Fatal(err)
	}
	return promptFile
}

type systemPromptTest struct {
	setup          func(t *testing.T) string
	wantErr        bool
	errContains    string
	validatePrompt func(t *testing.T, prompt string)
}

func runLoadSystemPromptTest(t *testing.T, tt systemPromptTest) {
	t.Helper()
	promptPath := tt.setup(t)
	prompt, err := config.LoadSystemPrompt(promptPath)

	if tt.wantErr {
		verifyError(t, err, tt.errContains)
		return
	}

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tt.validatePrompt != nil {
		tt.validatePrompt(t, prompt)
	}
}

func verifyError(t *testing.T, err error, errContains string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if errContains != "" && !contains(err.Error(), errContains) {
		t.Errorf("error %q does not contain %q", err.Error(), errContains)
	}
}

func TestValidateSystemPrompt(t *testing.T) {
	tests := []struct {
		name        string
		prompt      string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid prompt",
			prompt:  "You are a helpful assistant.",
			wantErr: false,
		},
		{
			name:        "empty prompt",
			prompt:      "",
			wantErr:     true,
			errContains: "system prompt is empty",
		},
		{
			name:        "whitespace only",
			prompt:      "   \n\t   ",
			wantErr:     true,
			errContains: "system prompt is empty",
		},
		{
			name:    "very long prompt is valid",
			prompt:  string(make([]byte, 10000)), // 10KB prompt
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateSystemPrompt(tt.prompt)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || substr == "" || s[:len(substr)] == substr || contains(s[1:], substr))
}
