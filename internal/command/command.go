// Package command provides safe wrappers for executing external commands.
// All command execution in the codebase must go through these functions
// to satisfy linting requirements and ensure consistent error handling.
package command

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout is the default timeout for command execution.
const DefaultTimeout = 30 * time.Second

// RunCommand executes a command and returns its combined output.
// The command will be executed with a default timeout of 30 seconds.
func RunCommand(name string, args ...string) (string, error) {
	return RunCommandContext(context.Background(), name, args...)
}

// RunCommandContext executes a command with the given context and returns its combined output.
// If the context does not have a deadline, a default timeout of 30 seconds will be applied.
func RunCommandContext(ctx context.Context, name string, args ...string) (string, error) {
	// Apply default timeout if context doesn't have a deadline
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	// Set working directory to /tmp to avoid permission issues
	cmd.Dir = "/tmp"
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		// Provide more context in error messages
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("command failed: %s %s (exit code %d): %s",
				name, strings.Join(args, " "), exitErr.ExitCode(), string(output))
		}
		return "", fmt.Errorf("command failed: %s %s: %w (output: %s)",
			name, strings.Join(args, " "), err, string(output))
	}
	
	return string(output), nil
}

// RunCommandWithInput executes a command with the given input and returns its output.
// The command will be executed with a default timeout of 30 seconds.
func RunCommandWithInput(input, name string, args ...string) (string, error) {
	return RunCommandWithInputContext(context.Background(), input, name, args...)
}

// RunCommandWithInputContext executes a command with the given context and input, returning its output.
// If the context does not have a deadline, a default timeout of 30 seconds will be applied.
func RunCommandWithInputContext(ctx context.Context, input, name string, args ...string) (string, error) {
	// Apply default timeout if context doesn't have a deadline
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(input)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	
	// Combine stdout and stderr for output
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += "stderr: " + stderr.String()
	}
	
	if err != nil {
		// Provide more context in error messages
		if exitErr, ok := err.(*exec.ExitError); ok {
			return output, fmt.Errorf("command failed: %s %s (exit code %d): %s",
				name, strings.Join(args, " "), exitErr.ExitCode(), output)
		}
		return output, fmt.Errorf("command failed: %s %s: %w (output: %s)",
			name, strings.Join(args, " "), err, output)
	}
	
	return output, nil
}

// Builder provides a fluent interface for building and executing commands
// with more control over execution parameters.
type Builder struct {
	name    string
	args    []string
	input   string
	timeout time.Duration
	ctx     context.Context
}

// NewCommand creates a new Builder for the given command.
func NewCommand(name string, args ...string) *Builder {
	return &Builder{
		name:    name,
		args:    args,
		timeout: DefaultTimeout,
		ctx:     context.Background(),
	}
}

// WithInput sets the input for the command.
func (cb *Builder) WithInput(input string) *Builder {
	cb.input = input
	return cb
}

// WithTimeout sets the timeout for the command execution.
func (cb *Builder) WithTimeout(timeout time.Duration) *Builder {
	cb.timeout = timeout
	return cb
}

// WithContext sets the context for the command execution.
func (cb *Builder) WithContext(ctx context.Context) *Builder {
	cb.ctx = ctx
	return cb
}

// Run executes the command and returns the output.
func (cb *Builder) Run() (string, error) {
	ctx := cb.ctx
	if cb.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.timeout)
		defer cancel()
	}
	
	if cb.input != "" {
		return RunCommandWithInputContext(ctx, cb.input, cb.name, cb.args...)
	}
	return RunCommandContext(ctx, cb.name, cb.args...)
}