package queue_test

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

// ExampleQueueSystem demonstrates how to use the integrated queue system.
func ExampleSystem() {
	// Create a context for the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create your LLM and Messenger implementations
	llm := &mockLLM{
		queryFunc: func(_ context.Context, message, _ string) (*claude.LLMResponse, error) {
			// Simulate LLM processing without any delay
			return &claude.LLMResponse{
				Message: fmt.Sprintf("I received your message: %s", message),
			}, nil
		},
	}

	messenger := &mockMessenger{
		sendFunc: func(_ context.Context, to, message string) error {
			fmt.Printf("Sending to %s: %s\n", to, message)
			return nil
		},
	}

	// Configure the queue system
	config := queue.SystemConfig{
		WorkerPoolSize:     3,                // Start with 3 workers
		MinWorkers:         1,                // Minimum 1 worker
		MaxWorkers:         10,               // Maximum 10 workers
		RateLimitPerMinute: 10,               // 10 messages per minute per conversation
		BurstLimit:         3,                // Allow burst of 3 messages
		HealthCheckPeriod:  30 * time.Second, // Check worker health every 30 seconds
		ShutdownTimeout:    30 * time.Second, // 30 second graceful shutdown
		LLM:                llm,
		Messenger:          messenger,
	}

	// Create the queue system
	system, err := queue.NewSystem(ctx, config)
	if err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx, "Failed to create queue system", slog.Any("error", err))
		return
	}

	// Start the system
	err = system.Start(ctx)
	if err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx, "Failed to start queue system", slog.Any("error", err))
		return
	}

	// Enqueue a message
	msg := signal.IncomingMessage{
		From:       "John Doe",
		FromNumber: "+1234567890",
		Text:       "Hello, world!",
		Timestamp:  time.Now(),
	}

	err = system.Enqueue(msg)
	if err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx, "Failed to enqueue message", slog.Any("error", err))
	}

	// Wait for message to be processed
	timeout := time.NewTimer(2 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer timeout.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			// Timeout reached
			fmt.Println("Timeout waiting for message processing")
			return
		case <-ticker.C:
			stats := system.Stats()
			// Message is processed when queue is empty
			if stats.TotalQueued == 0 && stats.TotalProcessing == 0 {
				// Message processed, wait a bit more for output
				<-time.After(100 * time.Millisecond)
				goto done
			}
		}
	}
done:

	// Check system statistics
	stats := system.Stats()
	// Verify the message was processed (queue should be empty)
	if stats.TotalQueued == 0 && stats.TotalProcessing == 0 {
		fmt.Println("Message successfully processed!")
		fmt.Printf("Messages in queue: %d\n", stats.TotalQueued)
		fmt.Printf("Active conversations: %d\n", stats.ConversationCount)
		fmt.Printf("Active workers: %d\n", stats.ActiveWorkers)
	}

	// Scale workers if needed
	if stats.TotalQueued > 10 {
		err = system.ScaleWorkers(2)
		if err != nil {
			logger := slog.Default()
			logger.ErrorContext(ctx, "Failed to scale workers", slog.Any("error", err))
		}
	}

	// Graceful shutdown
	err = system.Stop()
	if err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx, "Failed to stop system", slog.Any("error", err))
	}

	// Output:
	// Sending to +1234567890: I received your message: Hello, world!
	// Message successfully processed!
	// Messages in queue: 0
	// Active conversations: 0
	// Active workers: 3
}

// Mock implementations for the example.
type mockLLM struct {
	queryFunc func(ctx context.Context, message, sessionID string) (*claude.LLMResponse, error)
}

func (m *mockLLM) Query(ctx context.Context, message, sessionID string) (*claude.LLMResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, message, sessionID)
	}
	return &claude.LLMResponse{Message: "Mock response"}, nil
}

type mockMessenger struct {
	sendFunc func(ctx context.Context, to, message string) error
}

func (m *mockMessenger) Send(ctx context.Context, to, message string) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, to, message)
	}
	return nil
}

func (m *mockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

func (m *mockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}
