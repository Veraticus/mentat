package claude

import (
	"strings"
	"testing"
)

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantMsg    string
		wantErr    bool
		errContains string
	}{
		{
			name: "valid JSON response",
			input: `{
				"message": "Hello, I can help you with that.",
				"metadata": {"model": "claude-3-opus", "latency_ms": 500}
			}`,
			wantMsg: "Hello, I can help you with that.",
			wantErr: false,
		},
		{
			name: "valid JSON with multiline message",
			input: `{
				"message": "Line 1\nLine 2\nLine 3",
				"metadata": {"model": "claude-3-opus"}
			}`,
			wantMsg: "Line 1\nLine 2\nLine 3",
			wantErr: false,
		},
		{
			name:        "empty JSON message",
			input:       `{"message": "", "metadata": {}}`,
			wantErr:     true,
			errContains: "missing message/result field",
		},
		{
			name:        "JSON missing message field",
			input:       `{"metadata": {"model": "claude-3-opus"}}`,
			wantErr:     true,
			errContains: "missing message/result field",
		},
		{
			name: "plain text single line",
			input: "This is a plain text response from Claude.",
			wantMsg: "This is a plain text response from Claude.",
			wantErr: false,
		},
		{
			name: "plain text multiline",
			input: `First line of response
				Second line with leading spaces
				
				Third line after empty line`,
			wantMsg: "First line of response Second line with leading spaces Third line after empty line",
			wantErr: false,
		},
		{
			name: "error output with error prefix",
			input: `error: Authentication failed
				Please check your API key`,
			wantErr:     true,
			errContains: "Authentication failed Please check your API key",
		},
		{
			name: "error output with Error prefix",
			input: `Error: Rate limit exceeded
				Try again in 60 seconds`,
			wantErr:     true,
			errContains: "Rate limit exceeded Try again in 60 seconds",
		},
		{
			name:        "empty input",
			input:       "",
			wantErr:     true,
			errContains: "empty or unparseable response",
		},
		{
			name:        "whitespace only",
			input:       "   \n\t\n   ",
			wantErr:     true,
			errContains: "empty or unparseable response",
		},
		{
			name: "malformed JSON falls back to plain text",
			input: `{"message": "Incomplete JSON`,
			wantMsg: `{"message": "Incomplete JSON`,
			wantErr: false,
		},
		{
			name: "JSON with tool calls",
			input: `{
				"message": "I'll search for that information.",
				"tool_calls": [{"name": "search", "parameters": {"query": {"type": "string", "value": "test"}}}],
				"metadata": {"model": "claude-3-opus"}
			}`,
			wantMsg: "I'll search for that information.",
			wantErr: false,
		},
		{
			name: "stderr mixed with output",
			input: `stderr: Warning: deprecated flag used
				{"message": "Response despite warning", "metadata": {}}`,
			wantMsg: "stderr: Warning: deprecated flag used {\"message\": \"Response despite warning\", \"metadata\": {}}",
			wantErr: false,
		},
		{
			name: "HTML-like content in plain text",
			input: `<html>
				<body>Error: Service unavailable</body>
			</html>`,
			wantMsg: "<html> <body>Error: Service unavailable</body> </html>",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseResponse(tt.input)
			
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			if resp.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", resp.Message, tt.wantMsg)
			}
		})
	}
}

func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "command produced no output",
		},
		{
			name:  "simple error message",
			input: "Error: file not found",
			want:  "Error: file not found",
		},
		{
			name:  "multiple error lines",
			input: "Error: authentication failed\nInvalid credentials provided",
			want:  "Error: authentication failed; Invalid credentials provided",
		},
		{
			name:  "mixed output with errors",
			input: "Starting process...\nError: connection timeout\nFailed to connect to server",
			want:  "Error: connection timeout; Failed to connect to server",
		},
		{
			name:  "no error keywords",
			input: "Process completed successfully\nAll tests passed",
			want:  "Process completed successfully",
		},
		{
			name:  "permission denied error",
			input: "mkdir: cannot create directory '/root/test': Permission denied",
			want:  "mkdir: cannot create directory '/root/test': Permission denied",
		},
		{
			name:  "timeout error",
			input: "Operation timed out after 30 seconds\nTimeout exceeded",
			want:  "Operation timed out after 30 seconds; Timeout exceeded",
		},
		{
			name:  "whitespace and empty lines",
			input: "\n\n  Error: invalid input  \n\n  Please try again  \n\n",
			want:  "Error: invalid input",
		},
		{
			name:  "case insensitive error detection",
			input: "ERROR: SYSTEM FAILURE\nerror: disk full\nError: out of memory",
			want:  "ERROR: SYSTEM FAILURE; error: disk full; Error: out of memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrorMessage(tt.input)
			if got != tt.want {
				t.Errorf("extractErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseResponse_ComplexScenarios(t *testing.T) {
	tests := []struct {
		name string
		input string
		check func(t *testing.T, resp *jsonResponse, err error)
	}{
		{
			name: "very long multiline plain text",
			input: strings.Repeat("This is a long line. ", 100) + "\n" +
				strings.Repeat("Another long line. ", 100),
			check: func(t *testing.T, resp *jsonResponse, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(resp.Message, "This is a long line.") {
					t.Error("message should contain repeated text")
				}
				if !strings.Contains(resp.Message, "Another long line.") {
					t.Error("message should contain both lines")
				}
			},
		},
		{
			name: "JSON with Unicode characters",
			input: `{"message": "Hello 👋 こんにちは 🌍", "metadata": {"model": "claude-3"}}`,
			check: func(t *testing.T, resp *jsonResponse, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if resp.Message != "Hello 👋 こんにちは 🌍" {
					t.Errorf("Unicode not preserved: %q", resp.Message)
				}
			},
		},
		{
			name: "partial JSON error at end",
			input: `Some initial output
				{"message": "This JSON is incomplete...`,
			check: func(t *testing.T, resp *jsonResponse, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// Should fall back to plain text parsing
				if !strings.Contains(resp.Message, "Some initial output") {
					t.Error("should include all output as plain text")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseResponse(tt.input)
			tt.check(t, resp, err)
		})
	}
}