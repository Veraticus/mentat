package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSystemPrompt(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantErr   bool
		errContains string
		validatePrompt func(t *testing.T, prompt string)
	}{
		{
			name: "loads valid prompt file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				promptFile := filepath.Join(dir, "system_prompt.txt")
				content := "You are a helpful AI assistant that helps with scheduling and tasks."
				if err := os.WriteFile(promptFile, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				return promptFile
			},
			wantErr: false,
			validatePrompt: func(t *testing.T, prompt string) {
				t.Helper()
				expected := "You are a helpful AI assistant that helps with scheduling and tasks."
				if prompt != expected {
					t.Errorf("got prompt %q, want %q", prompt, expected)
				}
			},
		},
		{
			name: "fails on missing file",
			setup: func(_ *testing.T) string {
				return "/nonexistent/system_prompt.txt"
			},
			wantErr:     true,
			errContains: "system prompt file not found",
		},
		{
			name: "fails on empty file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				promptFile := filepath.Join(dir, "system_prompt.txt")
				if err := os.WriteFile(promptFile, []byte(""), 0600); err != nil {
					t.Fatal(err)
				}
				return promptFile
			},
			wantErr:     true,
			errContains: "system prompt is empty",
		},
		{
			name: "fails on whitespace-only file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				promptFile := filepath.Join(dir, "system_prompt.txt")
				if err := os.WriteFile(promptFile, []byte("   \n\t\n   "), 0600); err != nil {
					t.Fatal(err)
				}
				return promptFile
			},
			wantErr:     true,
			errContains: "system prompt is empty",
		},
		{
			name: "handles multiline prompts",
			setup: func(t *testing.T) string {
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
			},
			wantErr: false,
			validatePrompt: func(t *testing.T, prompt string) {
				t.Helper()
				if prompt == "" {
					t.Error("expected non-empty multiline prompt")
				}
				if prompt != `You are Mentat, a personal assistant.

You have access to:
- Calendar
- Email
- Task management

Always be helpful and concise.` {
					t.Error("multiline prompt not preserved correctly")
				}
			},
		},
		{
			name: "handles unreadable file",
			setup: func(t *testing.T) string {
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
			},
			wantErr:     true,
			errContains: "failed to read system prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptPath := tt.setup(t)
			
			prompt, err := LoadSystemPrompt(promptPath)
			
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
			
			if tt.validatePrompt != nil {
				tt.validatePrompt(t, prompt)
			}
		})
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
			err := ValidateSystemPrompt(tt.prompt)
			
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
	return len(s) >= len(substr) && (s == substr || substr == "" || s[:len(substr)] == substr || contains(s[1:], substr))
}