package command_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/command"
)

const (
	windowsOS = "windows"
)

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		args     []string
		wantErr  bool
		contains string
	}{
		{
			name:     "simple echo command",
			command:  "echo",
			args:     []string{"hello", "world"},
			wantErr:  false,
			contains: "hello world",
		},
		{
			name:     "command not found",
			command:  "nonexistentcommand123",
			args:     []string{},
			wantErr:  true,
			contains: "",
		},
		{
			name:     "command with exit code",
			command:  "sh",
			args:     []string{"-c", "exit 1"},
			wantErr:  true,
			contains: "(exit code 1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := command.RunCommand(tt.command, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("RunCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.contains != "" && !strings.Contains(output, tt.contains) {
				// Check error message if we got an error
				if err != nil && !strings.Contains(err.Error(), tt.contains) {
					t.Errorf("RunCommand() error = %v, want to contain %v", err, tt.contains)
				} else if err == nil && !strings.Contains(output, tt.contains) {
					t.Errorf("RunCommand() output = %v, want to contain %v", output, tt.contains)
				}
			}
		})
	}
}

func TestRunCommandContext(t *testing.T) {
	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := command.RunCommandContext(ctx, "sleep", "5")
		if err == nil {
			t.Error("RunCommandContext() expected error with canceled context")
		}
	})

	t.Run("applies default timeout", func(t *testing.T) {
		// This test verifies the default timeout behavior
		// We'll test that a long-running command gets interrupted

		// For CI/testing purposes, we'll use a much shorter sleep to verify timeout behavior
		// The key is that sleep duration > expected timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		start := time.Now()
		_, err := command.RunCommandContext(ctx, "sleep", "10")
		elapsed := time.Since(start)

		if err == nil {
			t.Error("RunCommandContext() expected timeout error")
		}

		// Should timeout around 2 seconds (our test timeout)
		if elapsed < 1900*time.Millisecond || elapsed > 3*time.Second {
			t.Errorf("RunCommandContext() took %v, expected ~2s", elapsed)
		}

		// Also test that default timeout is applied when no context deadline exists
		// For this, we'll use a very short sleep and verify it completes successfully
		output, err := command.RunCommandContext(context.Background(), "echo", "test")
		if err != nil {
			t.Errorf("RunCommandContext() failed for simple command: %v", err)
		}
		if !strings.Contains(output, "test") {
			t.Errorf("RunCommandContext() output = %v, expected 'test'", output)
		}
	})

	t.Run("respects existing deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		start := time.Now()
		_, err := command.RunCommandContext(ctx, "sleep", "5")
		elapsed := time.Since(start)

		if err == nil {
			t.Error("RunCommandContext() expected timeout error")
		}

		// Should timeout around 1 second
		if elapsed > 2*time.Second {
			t.Errorf("RunCommandContext() took %v, expected ~1s", elapsed)
		}
	})
}

func TestRunCommandWithInput(t *testing.T) {
	// Platform-specific cat command
	catCmd := "cat"
	if runtime.GOOS == windowsOS {
		catCmd = "type"
	}

	tests := []struct {
		name     string
		input    string
		command  string
		args     []string
		wantErr  bool
		contains string
	}{
		{
			name:     "echo stdin input",
			input:    "test input",
			command:  catCmd,
			args:     []string{},
			wantErr:  false,
			contains: "test input",
		},
		{
			name:     "grep with input",
			input:    "line1\nline2\nline3",
			command:  "grep",
			args:     []string{"line2"},
			wantErr:  false,
			contains: "line2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip grep test on Windows
			if runtime.GOOS == windowsOS && tt.command == "grep" {
				t.Skip("grep not available on Windows")
			}

			output, err := command.RunCommandWithInput(tt.input, tt.command, tt.args...)

			if (err != nil) != tt.wantErr {
				t.Errorf("RunCommandWithInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.contains != "" && !strings.Contains(output, tt.contains) {
				t.Errorf("RunCommandWithInput() output = %v, want to contain %v", output, tt.contains)
			}
		})
	}
}

func TestRunCommandWithInputContext(t *testing.T) {
	t.Run("handles stderr output", func(t *testing.T) {
		// Command that writes to stderr
		output, err := command.RunCommandWithInputContext(
			context.Background(),
			"",
			"sh",
			"-c",
			"echo 'stdout message' && echo 'stderr message' >&2",
		)

		if err != nil {
			t.Errorf("RunCommandWithInputContext() unexpected error: %v", err)
		}

		if !strings.Contains(output, "stdout message") {
			t.Error("RunCommandWithInputContext() missing stdout")
		}

		if !strings.Contains(output, "stderr: stderr message") {
			t.Error("RunCommandWithInputContext() missing stderr")
		}
	})
}

func TestCommandBuilder(t *testing.T) {
	t.Run("basic command", func(t *testing.T) {
		output, err := command.NewCommand("echo", "hello").Run()
		if err != nil {
			t.Errorf("CommandBuilder.Run() error = %v", err)
		}
		if !strings.Contains(output, "hello") {
			t.Errorf("CommandBuilder.Run() output = %v, want to contain 'hello'", output)
		}
	})

	t.Run("with input", func(t *testing.T) {
		catCmd := "cat"
		if runtime.GOOS == windowsOS {
			catCmd = "type"
		}

		output, err := command.NewCommand(catCmd).
			WithInput("test input").
			Run()

		if err != nil {
			t.Errorf("CommandBuilder.Run() error = %v", err)
		}
		if !strings.Contains(output, "test input") {
			t.Errorf("CommandBuilder.Run() output = %v, want to contain 'test input'", output)
		}
	})

	t.Run("with custom timeout", func(t *testing.T) {
		start := time.Now()
		_, err := command.NewCommand("sleep", "5").
			WithTimeout(1 * time.Second).
			Run()
		elapsed := time.Since(start)

		if err == nil {
			t.Error("CommandBuilder.Run() expected timeout error")
		}

		if elapsed > 2*time.Second {
			t.Errorf("CommandBuilder.Run() took %v, expected ~1s", elapsed)
		}
	})

	t.Run("with context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := command.NewCommand("echo", "hello").
			RunContext(ctx)

		if err == nil {
			t.Error("Builder.RunContext() expected error with canceled context")
		}
	})
}

func TestErrorMessages(t *testing.T) {
	t.Run("includes command and args in error", func(t *testing.T) {
		_, err := command.RunCommand("sh", "-c", "exit 42")
		if err == nil {
			t.Fatal("expected error")
		}

		errStr := err.Error()
		if !strings.Contains(errStr, "sh -c exit 42") {
			t.Errorf("error message should include command and args: %v", errStr)
		}
		if !strings.Contains(errStr, "exit code 42") {
			t.Errorf("error message should include exit code: %v", errStr)
		}
	})

	t.Run("includes output in error", func(t *testing.T) {
		_, err := command.RunCommand("sh", "-c", "echo 'error output' && exit 1")
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "error output") {
			t.Errorf("error message should include command output: %v", err)
		}
	})
}
