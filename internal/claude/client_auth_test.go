package claude

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockCommandRunnerAuth simulates authentication failures from Claude Code.
type mockCommandRunnerAuth struct{}

func (m *mockCommandRunnerAuth) RunCommandContext(_ context.Context, _ string, _ ...string) (string, error) {
	// Simulate the exact error output from Claude Code when authentication fails
	// The command runner embeds the JSON output in the error message
	authErrorJSON := `{"type":"result","subtype":"success","is_error":true,"duration_ms":133,"duration_api_ms":0,"num_turns":1,"result":"Invalid API key · Please run /login","session_id":"b49b0c80-09cb-4ade-b65b-daec50393d06","total_cost_usd":0,"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0,"server_tool_use":{"web_search_requests":0},"service_tier":"standard"}}`
	
	// Match the actual error format from command.RunCommandContext
	return "", fmt.Errorf("command failed: /usr/local/bin/claude-mentat --print --output-format json --model sonnet (exit code 1): %s", authErrorJSON)
}

func TestClientAuthenticationError(t *testing.T) {
	config := Config{
		Command:       "/usr/local/bin/claude-mentat",
		MCPConfigPath: "",
		Timeout:       30 * time.Second,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Inject mock command runner
	client.SetCommandRunner(&mockCommandRunnerAuth{})

	// Test authentication error handling
	ctx := context.Background()
	_, err = client.Query(ctx, "Hello Claude", "test-session")

	// Verify we get an AuthenticationError
	if !IsAuthenticationError(err) {
		t.Errorf("Expected AuthenticationError, got: %v", err)
	}

	// Verify the error message
	if err == nil || err.Error() != "Claude Code authentication required" {
		t.Errorf("Expected 'Claude Code authentication required', got: %v", err)
	}
}

func TestWorkerAuthenticationErrorResponse(_ *testing.T) {
	// This test would be in the worker package, but we'll document the expected behavior here
	// When the worker receives an AuthenticationError from Claude, it should:
	// 1. Not treat it as a failure that needs retry
	// 2. Send a user-friendly message to the user explaining how to authenticate
	// 3. The message should include the exact command to run
	
	expectedUserMessage := `Claude Code authentication required. Please run the following command on the server to log in:

sudo -u signal-cli /usr/local/bin/claude-mentat /login

Once authenticated, I'll be able to respond to your messages.`
	
	// This documents the expected user-facing message
	_ = expectedUserMessage
}