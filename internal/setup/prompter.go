// Package setup provides the registration setup flow for Mentat.
package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// TerminalPrompter implements Prompter using stdin/stdout.
type TerminalPrompter struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewTerminalPrompter creates a new terminal prompter.
func NewTerminalPrompter() *TerminalPrompter {
	return &TerminalPrompter{
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}
}

// NewTerminalPrompterWithIO creates a new terminal prompter with custom IO.
// Useful for testing.
func NewTerminalPrompterWithIO(reader io.Reader, writer io.Writer) *TerminalPrompter {
	return &TerminalPrompter{
		reader: bufio.NewReader(reader),
		writer: writer,
	}
}

// Prompt displays a message and waits for user input.
func (tp *TerminalPrompter) Prompt(ctx context.Context, message string) (string, error) {
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("prompt canceled: %w", ctx.Err())
	default:
	}

	_, _ = fmt.Fprintf(tp.writer, "%s: ", message)
	input, err := tp.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	return strings.TrimSpace(input), nil
}

// PromptWithDefault displays a message with a default value.
func (tp *TerminalPrompter) PromptWithDefault(ctx context.Context, message, defaultValue string) (string, error) {
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("prompt canceled: %w", ctx.Err())
	default:
	}

	_, _ = fmt.Fprintf(tp.writer, "%s [%s]: ", message, defaultValue)
	input, err := tp.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

// PromptWithTimeout displays a message and waits for input with a timeout.
// IMPORTANT: This implementation has a limitation - it cannot actually interrupt
// a blocking read operation. If the reader blocks indefinitely, this will not
// timeout properly. This is a known limitation of Go's standard I/O libraries.
// For testing, use a reader that respects cancellation (like io.Pipe with proper closing).
func (tp *TerminalPrompter) PromptWithTimeout(
	ctx context.Context, message string, timeout time.Duration,
) (string, error) {
	// For a true timeout implementation, we would need:
	// 1. Platform-specific non-blocking I/O
	// 2. Or accept that we can't interrupt blocking reads
	//
	// Since we can't properly implement timeout on blocking I/O,
	// we'll just use the regular Prompt and document the limitation

	// Apply timeout to context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check if already canceled
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("prompt canceled: %w", ctx.Err())
	default:
	}

	// Just use regular prompt - timeout won't work on blocking I/O
	// but at least we won't leak goroutines
	return tp.Prompt(ctx, message)
}

// PromptSecret displays a message and reads input without echoing.
func (tp *TerminalPrompter) PromptSecret(ctx context.Context, message string) (string, error) {
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("prompt canceled: %w", ctx.Err())
	default:
	}

	_, _ = fmt.Fprintf(tp.writer, "%s: ", message)

	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Not a terminal, read normally (for testing)
		input, err := tp.reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading input: %w", err)
		}
		return strings.TrimSpace(input), nil
	}

	// Read password without echo
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(tp.writer) // New line after password
	if err != nil {
		return "", fmt.Errorf("reading secret: %w", err)
	}

	return string(password), nil
}

// ShowMessage displays an informational message.
func (tp *TerminalPrompter) ShowMessage(message string) {
	_, _ = fmt.Fprintln(tp.writer, message)
}

// ShowError displays an error message.
func (tp *TerminalPrompter) ShowError(message string) {
	_, _ = fmt.Fprintf(tp.writer, "❌ Error: %s\n", message)
}

// ShowSuccess displays a success message.
func (tp *TerminalPrompter) ShowSuccess(message string) {
	_, _ = fmt.Fprintf(tp.writer, "✅ %s\n", message)
}
