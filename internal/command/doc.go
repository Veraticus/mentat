// Package command provides safe, controlled execution of external commands.
//
// This package is the required interface for all command execution in the mentat
// codebase. Direct use of os/exec.Command is forbidden by linting rules - all
// command execution must go through the functions provided here.
//
// Features:
//   - Automatic timeout handling (default 30 seconds)
//   - Context support for cancellation
//   - Detailed error messages with exit codes and output
//   - Support for stdin input
//   - Combined stdout/stderr output
//   - Fluent builder interface for complex scenarios
//
// Basic usage:
//
//	// Simple command
//	output, err := command.RunCommand("echo", "hello")
//
//	// Command with input
//	output, err := command.RunCommandWithInput("input data", "grep", "pattern")
//
//	// Command with custom timeout
//	output, err := command.NewCommand("long-running-tool").
//	    WithTimeout(5 * time.Minute).
//	    Run()
//
// All functions in this package ensure that commands are executed safely with
// proper cleanup and resource management. Commands that exceed the timeout are
// automatically terminated.
package command
