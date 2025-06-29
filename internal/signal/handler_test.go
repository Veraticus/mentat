package signal

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// mockMessenger implements the Messenger interface for testing.
type mockMessenger struct {
	mu             sync.Mutex
	messages       chan IncomingMessage
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
		messages: make(chan IncomingMessage, 10),
	}
}

func (m *mockMessenger) Subscribe(_ context.Context) (<-chan IncomingMessage, error) {
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
	messages     []IncomingMessage
	enqueueError error
}

func newMockQueue() *mockQueue {
	return &mockQueue{}
}

func (q *mockQueue) Enqueue(msg IncomingMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.enqueueError != nil {
		return q.enqueueError
	}
	q.messages = append(q.messages, msg)
	return nil
}

func (q *mockQueue) getMessages() []IncomingMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	msgs := make([]IncomingMessage, len(q.messages))
	copy(msgs, q.messages)
	return msgs
}

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name      string
		messenger Messenger
		queue     MessageEnqueuer
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
			handler, err := NewHandler(tt.messenger, tt.queue)
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
	logger := slog.New(slog.NewTextHandler(nil, nil))
	handler, err := NewHandler(
		newMockMessenger(),
		newMockQueue(),
		WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.logger != logger {
		t.Error("logger option not applied")
	}
}

func TestHandlerStart(t *testing.T) {
	t.Run("successful message processing", func(t *testing.T) {
		messenger := newMockMessenger()
		q := newMockQueue()
		handler, err := NewHandler(messenger, q)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start handler in background
		errCh := make(chan error, 1)
		go func() {
			errCh <- handler.Start(ctx)
		}()

		// Give handler time to start
		time.Sleep(50 * time.Millisecond)

		// Send test messages
		messages := []IncomingMessage{
			{From: "+1234567890", Text: "Hello", Timestamp: time.Now()},
			{From: "+0987654321", Text: "World", Timestamp: time.Now()},
		}

		for _, msg := range messages {
			messenger.messages <- msg
		}

		// Give handler time to process
		time.Sleep(50 * time.Millisecond)

		// Verify messages were enqueued
		enqueuedMsgs := q.getMessages()
		if len(enqueuedMsgs) != len(messages) {
			t.Errorf("expected %d messages, got %d", len(messages), len(enqueuedMsgs))
		}

		// Stop handler
		cancel()

		// Wait for handler to stop
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("handler returned error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("handler did not stop within timeout")
		}
	})

	t.Run("subscribe error", func(t *testing.T) {
		messenger := newMockMessenger()
		messenger.subscribeError = errors.New("subscribe failed")
		q := newMockQueue()

		handler, err := NewHandler(messenger, q)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		ctx := context.Background()
		err = handler.Start(ctx)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if err.Error() != "failed to subscribe to messages: subscribe failed" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("already running", func(t *testing.T) {
		messenger := newMockMessenger()
		q := newMockQueue()
		handler, err := NewHandler(messenger, q)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start handler
		go func() {
			_ = handler.Start(ctx)
		}()
		time.Sleep(50 * time.Millisecond)

		// Try to start again
		err = handler.Start(ctx)
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
	handler, err := NewHandler(messenger, q)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start handler
	go func() {
		_ = handler.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Send message that will succeed
	messenger.messages <- IncomingMessage{From: "+1111111111", Text: "Success"}
	time.Sleep(50 * time.Millisecond)

	// Set queue to fail
	q.mu.Lock()
	q.enqueueError = errors.New("queue full")
	q.mu.Unlock()

	// Send message that will fail to enqueue
	messenger.messages <- IncomingMessage{From: "+2222222222", Text: "Fail"}
	time.Sleep(50 * time.Millisecond)

	// Clear error
	q.mu.Lock()
	q.enqueueError = nil
	q.mu.Unlock()

	// Send another message that should succeed
	messenger.messages <- IncomingMessage{From: "+3333333333", Text: "Success again"}
	time.Sleep(50 * time.Millisecond)

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
	handler, err := NewHandler(messenger, q)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start handler
	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Close the channel
	close(messenger.messages)
	
	// Give it a moment to detect channel closure
	time.Sleep(50 * time.Millisecond)
	
	// Cancel context to trigger shutdown
	cancel()

	// Handler should exit gracefully
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("handler returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("handler did not stop after context canceled")
	}
}

func TestHandlerIsRunning(t *testing.T) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := NewHandler(messenger, q)
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
	go func() {
		_ = handler.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Should be running
	if !handler.IsRunning() {
		t.Error("handler should be running")
	}

	// Stop handler
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Should not be running
	if handler.IsRunning() {
		t.Error("handler should not be running after stop")
	}
}

func TestHandlerConcurrentAccess(t *testing.T) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := NewHandler(messenger, q)
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
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			// Check running status
			_ = handler.IsRunning()
			// Send message
			messenger.messages <- IncomingMessage{
				From:      "+1234567890",
				Text:      "Concurrent message",
				Timestamp: time.Now(),
			}
		}(i)
	}

	wg.Wait()
	cancel()
}

// Benchmark message processing.
func BenchmarkHandlerMessageProcessing(b *testing.B) {
	messenger := newMockMessenger()
	q := newMockQueue()
	handler, err := NewHandler(messenger, q)
	if err != nil {
		b.Fatalf("failed to create handler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = handler.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messenger.messages <- IncomingMessage{
			From:      "+1234567890",
			Text:      "Benchmark message",
			Timestamp: time.Now(),
		}
	}

	// Give time to process all messages
	time.Sleep(50 * time.Millisecond)
	b.StopTimer()

	cancel()
}