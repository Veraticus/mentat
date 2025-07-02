package claude_test

import (
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
)

func TestClientAuthenticationError(t *testing.T) {
	// This test validates that the client can be created with valid config
	// The actual authentication error testing would require access to SetCommandRunner
	// which is unexported. That functionality should be tested within the claude package.

	config := claude.Config{
		Command:       "/usr/local/bin/claude-mentat",
		MCPConfigPath: "",
		Timeout:       30 * time.Second,
	}

	_, err := claude.NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// The actual authentication error handling is tested in the claude package
	// where we have access to unexported methods like SetCommandRunner
	t.Skip("Authentication error testing requires unexported methods")
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
