package queue

import (
	"context"
	"fmt"
)

// Export internal types for testing

// ProcessTestMessage allows tests to call the Process method on a worker.
func ProcessTestMessage(ctx context.Context, w Worker, msg *Message) error {
	// Type assert to access the internal method
	if worker, ok := w.(*worker); ok {
		return worker.Process(ctx, msg)
	}
	return fmt.Errorf("ProcessTestMessage: worker is not a *worker type")
}
