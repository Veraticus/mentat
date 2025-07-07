package agent_test

import (
	"fmt"
	"time"
)

// Example demonstrates how to configure validation delays using handler options.
func Example_configurationOptions() {
	// Configure validation delays when creating a handler:
	//
	// handler, err := agent.NewHandler(llm,
	//     agent.WithCorrectionDelay(100 * time.Millisecond),   // Fast for testing
	//     agent.WithValidationTimeout(5 * time.Second),        // Custom timeout
	//     // ... other options
	// )

	// These configuration options allow you to customize the delays
	// without relying on environment variables.

	fmt.Println("Configuration options available:")
	fmt.Printf("WithCorrectionDelay(%v)\n", 100*time.Millisecond)
	fmt.Printf("WithValidationTimeout(%v)\n", 5*time.Second)

	// Output:
	// Configuration options available:
	// WithCorrectionDelay(100ms)
	// WithValidationTimeout(5s)
}
