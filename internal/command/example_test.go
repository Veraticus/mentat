package command_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Veraticus/mentat/internal/command"
)

func ExampleRunCommand() {
	// Simple command execution
	output, err := command.RunCommand("echo", "Hello, World!")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Output: %s", output)
	// Output:
	// Output: Hello, World!
}

func ExampleRunCommandWithInput() {
	// Execute command with stdin input
	input := "Line 1\nLine 2\nLine 3"
	output, err := command.RunCommandWithInput(input, "grep", "Line 2")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Matched: %s", output)
	// Output:
	// Matched: Line 2
}

func ExampleBuilder() {
	// Using the builder pattern for more control
	output, err := command.NewCommand("echo", "Builder pattern works!").
		WithTimeout(5 * time.Second).
		Run()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Output: %s", output)
	// Output:
	// Output: Builder pattern works!
}

func ExampleRunCommandContext() {
	// Execute with a custom context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	output, err := command.RunCommandContext(ctx, "sleep", "1")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Command completed: %s", output)
	// Output:
	// Command completed:
}
