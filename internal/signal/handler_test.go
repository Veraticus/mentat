package signal_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// mockMessenger implements the signal.Messenger interface for testing.
type mockMessenger struct {
	mu             sync.Mutex
	messages       chan signal.IncomingMessage
	subscribeError error
	sendError      error
	sendCalls      []struct {
		recipient string
		message   string
	}
	typingCalls []string
}

func newMockMessenger() *mockMessenger {
	return &mockMessenger{
		messages: make(chan signal.IncomingMessage, 10),
	}
}

func (m *mockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	if m.subscribeError != nil {
		return nil, m.subscribeError
	}
	return m.messages, nil
}

func (m *mockMessenger) Send(_ context.Context, recipient string, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalls = append(m.sendCalls, struct {
		recipient string
		message   string
	}{recipient, message})
	return m.sendError
}

func (m *mockMessenger) SendTypingIndicator(_ context.Context, recipient string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typingCalls = append(m.typingCalls, recipient)
	return nil
}

// mockQueue implements the MessageQueue interface for testing.
type mockQueue struct {
	mu           sync.Mutex
	messages     []signal.IncomingMessage
	enqueueError error
	enqueued     chan struct{} // Signal when message is enqueued
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		enqueued: make(chan struct{}, 100),
	}
}

func (q *mockQueue) Enqueue(msg signal.IncomingMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.enqueueError != nil {
		return q.enqueueError
	}
	q.messages = append(q.messages, msg)
	select {
	case q.enqueued <- struct{}{}:
	default:
	}
	return nil
}

func (q *mockQueue) getMessages() []signal.IncomingMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	msgs := make([]signal.IncomingMessage, len(q.messages))
	copy(msgs, q.messages)
	return msgs
}

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name      string
		messenger signal.Messenger
		queue     signal.MessageEnqueuer
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid dependencies",
			messenger: newMockMessenger(),
			queue:     newMockQueue(),
			wantErr:   false,
		},
		{
			name:      "nil messenger",
			messenger: nil,
			queue:     newMockQueue(),
			wantErr:   true,
			errMsg:    "messenger is required",
		},
		{
			name:      "nil queue",
			messenger: newMockMessenger(),
			queue:     nil,
			wantErr:   true,
			errMsg:    "queue is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := signal.NewHandler(tt.messenger, tt.queue)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if err.Error() != tt.errMsg {
					t.Errorf("error = %v, want %v", err, tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if handler == nil {
				t.Fatal("expected handler but got nil")
			}
		})
	}
}

func TestHandlerWithLogger(t *testing.T) {
	logger := slog.Default()
	_, err := signal.NewHandler(
		newMockMessenger(),
		newMockQueue(),
		signal.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Note: Can't verify logger is set as it's an unexported field
	// The option is tested implicitly through handler behavior
	// Handler is created successfully, confirming the option works
}

// handlerTestSetup creates a handler with test dependencies.
type handlerTestSetup struct {
	messenger *mockMessenger
	queue     *mockQueue
	handler   *signal.Handler
}

func newHandlerTestSetup(t *testing.T) *handlerTestSetup {
	t.Helper()
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := signal.NewHandler(messenger, q)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	return &handlerTestSetup{
		messenger: messenger,
		queue:     q,
		handler:   handler,
	}
}

func startHandlerAsync(ctx context.Context, handler *signal.Handler) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.Start(ctx)
	}()
	return errCh
}

func waitForMessages(t *testing.T, q *mockQueue, count int) {
	t.Helper()
	for range count {
		select {
		case <-q.enqueued:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for message to be enqueued")
		}
	}
}

func waitForHandlerStop(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("handler returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("handler did not stop within timeout")
	}
}

func TestHandlerStart(t *testing.T) {
	t.Run("successful message processing", func(t *testing.T) {
		setup := newHandlerTestSetup(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := startHandlerAsync(ctx, setup.handler)

		// Send test messages
		messages := []signal.IncomingMessage{
			{From: "+1234567890", Text: "Hello", Timestamp: time.Now()},
			{From: "+0987654321", Text: "World", Timestamp: time.Now()},
		}

		for _, msg := range messages {
			setup.messenger.messages <- msg
		}

		// Wait and verify
		waitForMessages(t, setup.queue, len(messages))

		enqueuedMsgs := setup.queue.getMessages()
		if len(enqueuedMsgs) != len(messages) {
			t.Errorf("expected %d messages, got %d", len(messages), len(enqueuedMsgs))
		}

		cancel()
		waitForHandlerStop(t, errCh)
	})

	t.Run("subscribe error", func(t *testing.T) {
		setup := newHandlerTestSetup(t)
		setup.messenger.subscribeError = fmt.Errorf("subscribe failed")

		err := setup.handler.Start(context.Background())
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "failed to subscribe to messages: subscribe failed" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("already running", func(t *testing.T) {
		setup := newHandlerTestSetup(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start handler
		started := make(chan struct{})
		go func() {
			close(started)
			_ = setup.handler.Start(ctx)
		}()
		<-started

		// Try to start again
		err := setup.handler.Start(ctx)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "handler already running" {
			t.Errorf("unexpected error: %v", err)
		}

		cancel()
	})
}

func TestHandlerEnqueueError(t *testing.T) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := signal.NewHandler(messenger, q)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start handler
	started := make(chan struct{})
	go func() {
		close(started)
		_ = handler.Start(ctx)
	}()
	<-started

	// Send message that will succeed
	messenger.messages <- signal.IncomingMessage{From: "+1111111111", Text: "Success"}
	// Wait for enqueue
	select {
	case <-q.enqueued:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first message")
	}

	// Set queue to fail
	q.mu.Lock()
	q.enqueueError = fmt.Errorf("queue full")
	q.mu.Unlock()

	// Send message that will fail to enqueue
	messenger.messages <- signal.IncomingMessage{From: "+2222222222", Text: "Fail"}
	// Give time for failed enqueue attempt
	<-time.After(10 * time.Millisecond)

	// Clear error
	q.mu.Lock()
	q.enqueueError = nil
	q.mu.Unlock()

	// Send another message that should succeed
	messenger.messages <- signal.IncomingMessage{From: "+3333333333", Text: "Success again"}
	// Wait for enqueue
	select {
	case <-q.enqueued:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for third message")
	}

	// Verify only successful messages were enqueued
	enqueuedMsgs := q.getMessages()
	if len(enqueuedMsgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(enqueuedMsgs))
	}

	cancel()
}

func TestHandlerChannelClosed(t *testing.T) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := signal.NewHandler(messenger, q)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start handler
	errCh := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		errCh <- handler.Start(ctx)
	}()
	<-started

	// Close the channel
	close(messenger.messages)

	// Let the handler process the closed channel
	<-time.After(10 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// signal.Handler should exit gracefully
	select {
	case handlerErr := <-errCh:
		if handlerErr != nil {
			t.Errorf("handler returned error: %v", handlerErr)
		}
	case <-time.After(time.Second):
		t.Error("handler did not stop after context canceled")
	}
}

func TestHandlerIsRunning(t *testing.T) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := signal.NewHandler(messenger, q)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Initially not running
	if handler.IsRunning() {
		t.Error("handler should not be running initially")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start handler
	started := make(chan struct{})
	go func() {
		close(started)
		_ = handler.Start(ctx)
	}()
	<-started

	// Should be running
	if !handler.IsRunning() {
		t.Error("handler should be running")
	}

	// Stop handler
	cancel()
	// Wait for handler to actually stop
	for range 10 {
		if !handler.IsRunning() {
			break
		}
		<-time.After(10 * time.Millisecond)
	}

	// Should not be running
	if handler.IsRunning() {
		t.Error("handler should not be running after stop")
	}
}

func TestHandlerConcurrentAccess(t *testing.T) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := signal.NewHandler(messenger, q)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start handler
	go func() {
		_ = handler.Start(ctx)
	}()

	// Concurrent operations
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Check running status
			_ = handler.IsRunning()
			// Send message
			messenger.messages <- signal.IncomingMessage{
				From:      "+1234567890",
				Text:      "Concurrent message",
				Timestamp: time.Now(),
			}
		}()
	}

	wg.Wait()
	cancel()
}

// Benchmark message processing.
func BenchmarkHandlerMessageProcessing(b *testing.B) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := signal.NewHandler(messenger, q)
	if err != nil {
		b.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	go func() {
		close(started)
		_ = handler.Start(ctx)
	}()
	<-started

	b.ResetTimer()
	for range b.N {
		messenger.messages <- signal.IncomingMessage{
			From:      "+1234567890",
			Text:      "Benchmark message",
			Timestamp: time.Now(),
		}
	}

	// Wait for all messages to be processed
	for range b.N {
		select {
		case <-q.enqueued:
		case <-time.After(time.Second):
			b.Fatal("timeout waiting for message")
		}
	}
	b.StopTimer()

	cancel()
}
