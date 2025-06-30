package claude

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockCommandRunner implements a mock for command execution.
type mockCommandRunner struct {
	output string
	err    error
	calls  []mockCall
}

type mockCall struct {
	name string
	args []string
}

func (m *mockCommandRunner) RunCommandContext(_ context.Context, name string, args ...string) (string, error) {
	m.calls = append(m.calls, mockCall{name: name, args: args})
	if m.err != nil {
		return m.output, m.err
	}
	return m.output, nil
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Command:       "/usr/bin/claude",
				MCPConfigPath: "/etc/mcp-config.json",
				Timeout:       30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "missing command path",
			config: Config{
				MCPConfigPath: "/etc/mcp-config.json",
				Timeout:       30 * time.Second,
			},
			wantErr: true,
			errMsg:  "command path is required",
		},
		{
			name: "zero timeout uses default",
			config: Config{
				Command:       "/usr/bin/claude",
				MCPConfigPath: "/etc/mcp-config.json",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("expected client but got nil")
			}
		})
	}
}

func TestClient_Query(t *testing.T) {
	tests := []struct {
		name       string
		prompt     string
		sessionID  string
		mockOutput string
		mockErr    error
		want       *LLMResponse
		wantErr    bool
		errMsg     string
	}{
		{
			name:      "successful query with tool calls",
			prompt:    "What's the weather today?",
			sessionID: "test-session-123",
			mockOutput: `{
				"message": "I'll check the weather for you.",
				"tool_calls": [
					{
						"name": "get_weather",
						"parameters": {
							"location": {"type": "string", "value": "San Francisco"}
						}
					}
				],
				"metadata": {
					"model": "claude-3-opus",
					"latency_ms": 1234,
					"tokens_used": 150
				}
			}`,
			want: &LLMResponse{
				Message: "I'll check the weather for you.",
				ToolCalls: []ToolCall{
					{
						Tool: "get_weather",
						Parameters: map[string]ToolParameter{
							"location": NewStringParam("San Francisco"),
						},
					},
				},
				Metadata: ResponseMetadata{
					ModelVersion: "claude-3-opus",
					Latency:      1234 * time.Millisecond,
					TokensUsed:   150,
				},
			},
			wantErr: false,
		},
		{
			name:      "simple response without tool calls",
			prompt:    "Hello",
			sessionID: "test-session-456",
			mockOutput: `{
				"message": "Hello! How can I help you today?",
				"metadata": {
					"model": "claude-3-opus",
					"latency_ms": 500,
					"tokens_used": 20
				}
			}`,
			want: &LLMResponse{
				Message: "Hello! How can I help you today?",
				Metadata: ResponseMetadata{
					ModelVersion: "claude-3-opus",
					Latency:      500 * time.Millisecond,
					TokensUsed:   20,
				},
			},
			wantErr: false,
		},
		{
			name:       "command execution error",
			prompt:     "Test prompt",
			sessionID:  "test-session",
			mockErr:    errors.New("executable not found"),
			wantErr:    true,
			errMsg:     "failed to execute claude CLI",
		},
		{
			name:       "plain text output fallback",
			prompt:     "Test prompt",
			sessionID:  "test-session",
			mockOutput: "This is not valid JSON but still a valid response",
			want: &LLMResponse{
				Message: "This is not valid JSON but still a valid response",
				Metadata: ResponseMetadata{
					ModelVersion: "unknown",
				},
			},
			wantErr:    false,
		},
		{
			name:       "empty response",
			prompt:     "Test prompt",
			sessionID:  "test-session",
			mockOutput: "",
			wantErr:    true,
			errMsg:     "empty or unparseable response",
		},
		{
			name:       "response missing message field",
			prompt:     "Test prompt",
			sessionID:  "test-session",
			mockOutput: `{"metadata": {"model": "claude-3-opus"}}`,
			wantErr:    true,
			errMsg:     "claude response missing message field",
		},
		{
			name:      "empty prompt",
			prompt:    "",
			sessionID: "test-session",
			wantErr:   true,
			errMsg:    "prompt cannot be empty",
		},
		{
			name:      "empty session ID",
			prompt:    "Test prompt",
			sessionID: "",
			wantErr:   true,
			errMsg:    "session ID cannot be empty",
		},
		{
			name:       "multiline plain text response",
			prompt:     "Tell me a story",
			sessionID:  "test-multiline",
			mockOutput: `Once upon a time...
			
			There was a developer
			Who wrote excellent tests`,
			want: &LLMResponse{
				Message: "Once upon a time... There was a developer Who wrote excellent tests",
				Metadata: ResponseMetadata{
					ModelVersion: "unknown",
				},
			},
			wantErr: false,
		},
		{
			name:       "error prefix in output",
			prompt:     "Bad request",
			sessionID:  "test-error",
			mockOutput: `error: Invalid API key
			Please check your configuration`,
			wantErr:    true,
			errMsg:     "Invalid API key Please check your configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &mockCommandRunner{
				output: tt.mockOutput,
				err:    tt.mockErr,
			}

			client, _ := NewClient(Config{
				Command:       "/usr/bin/claude",
				MCPConfigPath: "/etc/mcp-config.json",
				Timeout:       30 * time.Second,
			})
			client.SetCommandRunner(runner)

			ctx := context.Background()
			got, err := client.Query(ctx, tt.prompt, tt.sessionID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the response
			if got.Message != tt.want.Message {
				t.Errorf("Message = %q, want %q", got.Message, tt.want.Message)
			}

			if len(got.ToolCalls) != len(tt.want.ToolCalls) {
				t.Fatalf("ToolCalls length = %d, want %d", len(got.ToolCalls), len(tt.want.ToolCalls))
			}

			// Verify tool calls
			for i, tc := range got.ToolCalls {
				wantTC := tt.want.ToolCalls[i]
				if tc.Tool != wantTC.Tool {
					t.Errorf("ToolCall[%d].Tool = %q, want %q", i, tc.Tool, wantTC.Tool)
				}
				if len(tc.Parameters) != len(wantTC.Parameters) {
					t.Errorf("ToolCall[%d].Parameters length = %d, want %d", i, len(tc.Parameters), len(wantTC.Parameters))
				}
			}

			// Verify metadata
			if got.Metadata.ModelVersion != tt.want.Metadata.ModelVersion {
				t.Errorf("Metadata.ModelVersion = %q, want %q", got.Metadata.ModelVersion, tt.want.Metadata.ModelVersion)
			}
			if got.Metadata.Latency != tt.want.Metadata.Latency {
				t.Errorf("Metadata.Latency = %v, want %v", got.Metadata.Latency, tt.want.Metadata.Latency)
			}
			if got.Metadata.TokensUsed != tt.want.Metadata.TokensUsed {
				t.Errorf("Metadata.TokensUsed = %d, want %d", got.Metadata.TokensUsed, tt.want.Metadata.TokensUsed)
			}

			// Verify command was called correctly
			if len(runner.calls) != 1 {
				t.Fatalf("expected 1 command call, got %d", len(runner.calls))
			}

			call := runner.calls[0]
			if call.name != "/usr/bin/claude" {
				t.Errorf("command = %q, want %q", call.name, "/usr/bin/claude")
			}

			// Verify arguments
			expectedArgs := []string{
				"--print",
				"--output-format", "json",
				"--model", "sonnet",
				"--resume", tt.sessionID,
				"--mcp-config", "/etc/mcp-config.json",
				tt.prompt,
			}

			if !equalStringSlices(call.args, expectedArgs) {
				t.Errorf("args = %v, want %v", call.args, expectedArgs)
			}
		})
	}
}

func TestClient_Query_Timeout(t *testing.T) {
	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, _ := NewClient(Config{
		Command:       "/usr/bin/claude",
		MCPConfigPath: "/etc/mcp-config.json",
		Timeout:       30 * time.Second,
	})

	client.SetCommandRunner(&mockCommandRunner{
		err: context.DeadlineExceeded,
	})

	_, err := client.Query(ctx, "test prompt", "session-123")
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !containsString(err.Error(), "context deadline exceeded") && !containsString(err.Error(), "context canceled") {
		t.Errorf("error = %q, want to contain 'context deadline exceeded' or 'context canceled'", err.Error())
	}
}

func TestClient_Query_CommandErrorWithOutput(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput string
		mockErr    error
		wantErrMsg string
	}{
		{
			name:       "command error with helpful output",
			mockOutput: "Error: Authentication failed\nInvalid API credentials",
			mockErr:    errors.New("exit status 1"),
			wantErrMsg: "Error: Authentication failed; Invalid API credentials",
		},
		{
			name:       "command error with no output",
			mockOutput: "",
			mockErr:    errors.New("command not found"),
			wantErrMsg: "command produced no output",
		},
		{
			name:       "command error with unhelpful output",
			mockOutput: "Starting claude...\nInitializing...",
			mockErr:    errors.New("exit status 255"),
			wantErrMsg: "Starting claude...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := NewClient(Config{
				Command:       "/usr/bin/claude",
				MCPConfigPath: "/etc/mcp-config.json",
				Timeout:       30 * time.Second,
			})

			client.SetCommandRunner(&mockCommandRunner{
				output: tt.mockOutput,
				err:    tt.mockErr,
			})

			_, err := client.Query(context.Background(), "test", "session")
			if err == nil {
				t.Fatal("expected error")
			}

			if !containsString(err.Error(), tt.wantErrMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestClient_Query_ComplexToolCalls(t *testing.T) {
	mockOutput := `{
		"message": "I'll help you with multiple tasks.",
		"tool_calls": [
			{
				"name": "search_memory",
				"parameters": {
					"query": {"type": "string", "value": "meeting notes"},
					"limit": {"type": "int", "value": 10}
				}
			},
			{
				"name": "send_email",
				"parameters": {
					"to": {"type": "string", "value": "user@example.com"},
					"subject": {"type": "string", "value": "Meeting Summary"},
					"body": {"type": "string", "value": "Here are the notes..."}
				}
			}
		],
		"metadata": {
			"model": "claude-3-opus",
			"latency_ms": 2500,
			"tokens_used": 350
		}
	}`

	runner := &mockCommandRunner{
		output: mockOutput,
	}

	client, _ := NewClient(Config{
		Command:       "/usr/bin/claude",
		MCPConfigPath: "/etc/mcp-config.json",
		Timeout:       30 * time.Second,
	})
	client.SetCommandRunner(runner)

	resp, err := client.Query(context.Background(), "Find my meeting notes and email them", "session-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}

	// Verify first tool call
	tc1 := resp.ToolCalls[0]
	if tc1.Tool != "search_memory" {
		t.Errorf("first tool call name = %q, want %q", tc1.Tool, "search_memory")
	}
	if len(tc1.Parameters) != 2 {
		t.Errorf("first tool call parameters = %d, want 2", len(tc1.Parameters))
	}

	// Verify second tool call
	tc2 := resp.ToolCalls[1]
	if tc2.Tool != "send_email" {
		t.Errorf("second tool call name = %q, want %q", tc2.Tool, "send_email")
	}
	if len(tc2.Parameters) != 3 {
		t.Errorf("second tool call parameters = %d, want 3", len(tc2.Parameters))
	}
}

// Helper functions.
func containsString(s, substr string) bool {
	return substr != "" && len(s) >= len(substr) && 
		(s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestClient_Query_JSONStructures verifies we can parse various JSON structures.
func TestClient_Query_JSONStructures(t *testing.T) {
	tests := []struct {
		name   string
		output string
		verify func(t *testing.T, resp *LLMResponse)
	}{
		{
			name: "nested object parameters",
			output: `{
				"message": "Processing complex data",
				"tool_calls": [{
					"name": "process_data",
					"parameters": {
						"config": {
							"type": "object",
							"value": {
								"timeout": {"type": "int", "value": 30},
								"retries": {"type": "int", "value": 3}
							}
						}
					}
				}]
			}`,
			verify: func(t *testing.T, resp *LLMResponse) {
				t.Helper()
				if len(resp.ToolCalls) != 1 {
					t.Fatal("expected 1 tool call")
				}
				param := resp.ToolCalls[0].Parameters["config"]
				if param.Type != ToolParamObject {
					t.Errorf("param type = %v, want %v", param.Type, ToolParamObject)
				}
			},
		},
		{
			name: "array parameters",
			output: `{
				"message": "Processing list",
				"tool_calls": [{
					"name": "process_items",
					"parameters": {
						"items": {
							"type": "array",
							"value": [
								{"type": "string", "value": "item1"},
								{"type": "string", "value": "item2"}
							]
						}
					}
				}]
			}`,
			verify: func(t *testing.T, resp *LLMResponse) {
				t.Helper()
				if len(resp.ToolCalls) != 1 {
					t.Fatal("expected 1 tool call")
				}
				param := resp.ToolCalls[0].Parameters["items"]
				if param.Type != ToolParamArray {
					t.Errorf("param type = %v, want %v", param.Type, ToolParamArray)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &mockCommandRunner{output: tt.output}
			client, _ := NewClient(Config{
				Command:       "/usr/bin/claude",
				MCPConfigPath: "/etc/mcp-config.json",
				Timeout:       30 * time.Second,
			})
			client.SetCommandRunner(runner)

			resp, err := client.Query(context.Background(), "test", "session")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tt.verify(t, resp)
		})
	}
}