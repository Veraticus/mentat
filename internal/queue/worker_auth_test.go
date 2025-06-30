package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// mockLLMAuth simulates Claude authentication errors.
type mockLLMAuth struct{}

func (m *mockLLMAuth) Query(ctx context.Context, prompt string, sessionID string) (*claude.LLMResponse, error) {
	return nil, &claude.AuthenticationError{
		Message: "Claude Code authentication required",
	}
}

// mockMessengerCapture captures sent messages for verification.
type mockMessengerCapture struct {
	sentMessages []authTestSentMessage
}

type authTestSentMessage struct {
	recipient string
	message   string
}

func (m *mockMessengerCapture) Send(ctx context.Context, recipient string, message string) error {
	m.sentMessages = append(m.sentMessages, authTestSentMessage{
		recipient: recipient,
		message:   message,
	})
	return nil
}

func (m *mockMessengerCapture) SendTypingIndicator(ctx context.Context, recipient string) error {
	return nil
}

func (m *mockMessengerCapture) Start(ctx context.Context) error {
	return nil
}

func (m *mockMessengerCapture) Stop() error {
	return nil
}

func TestWorkerHandlesAuthenticationError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock components
	mockLLM := &mockLLMAuth{}
	mockMessenger := &mockMessengerCapture{}
	mockQueueMgr := NewManager(ctx)
	
	// Start queue manager
	go mockQueueMgr.Start()
	time.Sleep(100 * time.Millisecond)

	// Create worker config
	config := WorkerConfig{
		ID:           1,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: mockQueueMgr,
		RateLimiter:  NewRateLimiter(10, 1, time.Minute),
		TypingIndicatorMgr: signal.NewTypingIndicatorManager(mockMessenger),
	}

	// Create worker
	worker := NewWorker(config)

	// Create test message
	msg := NewMessage("test-123", "+1234567890", "John Doe", "+1234567890", "Hello Claude")

	// Submit message to queue
	if err := mockQueueMgr.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Start worker
	workerDone := make(chan error)
	go func() {
		workerDone <- worker.Start(ctx)
	}()

	// Give worker time to process
	time.Sleep(500 * time.Millisecond)

	// Cancel context to stop worker
	cancel()

	// Wait for worker to finish
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker did not stop in time")
	}

	// Shutdown queue manager
	if err := mockQueueMgr.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("Failed to shutdown queue manager: %v", err)
	}

	// Verify the response was sent
	if len(mockMessenger.sentMessages) != 1 {
		t.Fatalf("Expected 1 message sent, got %d", len(mockMessenger.sentMessages))
	}

	sentMsg := mockMessenger.sentMessages[0]
	
	// Verify recipient
	if sentMsg.recipient != "+1234567890" {
		t.Errorf("Expected recipient +1234567890, got %s", sentMsg.recipient)
	}

	// Verify message content
	expectedMessage := "Claude Code authentication required. Please run the following command on the server to log in:\n\n" +
		"sudo -u signal-cli /usr/local/bin/claude-mentat /login\n\n" +
		"Once authenticated, I'll be able to respond to your messages."
	
	if sentMsg.message != expectedMessage {
		t.Errorf("Message mismatch.\nExpected:\n%s\n\nGot:\n%s", expectedMessage, sentMsg.message)
	}

	// Verify message contains key elements
	if !strings.Contains(sentMsg.message, "Claude Code authentication required") {
		t.Error("Message should mention Claude Code authentication")
	}
	if !strings.Contains(sentMsg.message, "/usr/local/bin/claude-mentat /login") {
		t.Error("Message should include the login command")
	}
	if !strings.Contains(sentMsg.message, "sudo -u signal-cli") {
		t.Error("Message should include sudo command")
	}
}

func TestWorkerAuthErrorNotRetried(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock components
	mockLLM := &mockLLMAuth{}
	mockMessenger := &mockMessengerCapture{}
	mockQueueMgr := NewManager(ctx)
	
	// Start queue manager
	go mockQueueMgr.Start()
	time.Sleep(100 * time.Millisecond)

	// Create worker config
	config := WorkerConfig{
		ID:           1,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: mockQueueMgr,
		RateLimiter:  NewRateLimiter(10, 1, time.Minute),
	}

	// Create worker
	worker := NewWorker(config)

	// Create test message
	msg := NewMessage("test-456", "+1234567890", "John Doe", "+1234567890", "Test message")

	// Submit message to queue
	if err := mockQueueMgr.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Start worker
	workerDone := make(chan error)
	go func() {
		workerDone <- worker.Start(ctx)
	}()

	// Give worker time to process
	time.Sleep(500 * time.Millisecond)

	// Check queue stats - message should be completed, not retrying
	stats := mockQueueMgr.Stats()
	if stats["processing"] != 0 {
		t.Errorf("Expected 0 messages processing, got %d", stats["processing"])
	}
	if stats["queued"] != 0 {
		t.Errorf("Expected 0 messages queued (for retry), got %d", stats["queued"])
	}

	// Cancel context to stop worker
	cancel()

	// Wait for worker to finish
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker did not stop in time")
	}

	// Verify exactly one message was sent (no retries)
	if len(mockMessenger.sentMessages) != 1 {
		t.Errorf("Expected exactly 1 message sent (no retries), got %d", len(mockMessenger.sentMessages))
	}
}