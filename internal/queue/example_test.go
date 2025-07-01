package queue_test

import (
	"context"
	"fmt"
	"log"
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
			// Simulate LLM processing
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
		log.Printf("Failed to create queue system: %v", err)
		return
	}

	// Start the system
	if err := system.Start(); err != nil {
		log.Printf("Failed to start queue system: %v", err)
		return
	}

	// Enqueue a message
	msg := signal.IncomingMessage{
		From:       "John Doe",
		FromNumber: "+1234567890",
		Text:       "Hello, world!",
		Timestamp:  time.Now(),
	}

	if err := system.Enqueue(msg); err != nil {
		log.Printf("Failed to enqueue message: %v", err)
	}

	// Wait for message to be processed
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stats := system.Stats()
		if stats.TotalCompleted >= 1 {
			// Message processed!
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Check system statistics
	stats := system.Stats()
	fmt.Printf("Queue stats: %+v\n", stats)

	// Scale workers if needed
	if stats.TotalQueued > 10 {
		if err := system.ScaleWorkers(2); err != nil {
			log.Printf("Failed to scale workers: %v", err)
		}
	}

	// Graceful shutdown
	if err := system.Stop(); err != nil {
		log.Printf("Failed to stop system: %v", err)
	}

	// Output:
	// Sending to +1234567890: I received your message: Hello, world!
	// Queue stats: {TotalQueued:0 TotalProcessing:0 TotalCompleted:1 TotalFailed:0 ConversationCount:1 OldestMessageAge:0s AverageWaitTime:0s AverageProcessTime:0s ActiveWorkers:3 HealthyWorkers:3}
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
