// Package setup provides the registration setup flow for Mentat.
package setup

import (
	"context"
	"time"
)

// Prompter abstracts user interaction for the setup flow.
// This allows for terminal-based implementation and easy testing with mocks.
type Prompter interface {
	// Prompt displays a message and waits for user input.
	// Returns the user's input as a string.
	Prompt(ctx context.Context, message string) (string, error)

	// PromptWithDefault displays a message with a default value.
	// If the user provides no input, the default is returned.
	PromptWithDefault(ctx context.Context, message, defaultValue string) (string, error)

	// PromptWithTimeout displays a message and waits for input with a timeout.
	// Returns an error if the timeout is exceeded.
	PromptWithTimeout(ctx context.Context, message string, timeout time.Duration) (string, error)

	// PromptSecret displays a message and reads input without echoing (for passwords).
	PromptSecret(ctx context.Context, message string) (string, error)

	// ShowMessage displays an informational message without waiting for input.
	ShowMessage(message string)

	// ShowError displays an error message.
	ShowError(message string)

	// ShowSuccess displays a success message.
	ShowSuccess(message string)
}
