package signal_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// TestNewMessenger verifies messenger creation.
func TestNewMessenger(t *testing.T) {
	client := &signal.MockClient{}
	selfPhone := "+1234567890"

	m := signal.NewMessenger(client, selfPhone)
	if m == nil {
		t.Fatal("signal.NewMessenger returned nil")
	}

	// Verify it's not nil (interface check)
	_ = m
}

// TestMessengerSend tests the Send method.
func TestMessengerSend(t *testing.T) {
	tests := []struct {
		name        string
		recipient   string
		message     string
		clientErr   error
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful send",
			recipient:   "+0987654321",
			message:     "Hello, World!",
			clientErr:   nil,
			expectError: false,
		},
		{
			name:        "empty recipient",
			recipient:   "",
			message:     "Hello",
			expectError: true,
			errorMsg:    "recipient cannot be empty",
		},
		{
			name:        "empty message",
			recipient:   "+0987654321",
			message:     "",
			expectError: true,
			errorMsg:    "message cannot be empty",
		},
		{
			name:        "client error",
			recipient:   "+0987654321",
			message:     "Hello",
			clientErr:   fmt.Errorf("network error"),
			expectError: true,
			errorMsg:    "failed to send message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &signal.MockClient{
				SendFunc: func(_ context.Context, _ *signal.SendRequest) (*signal.SendResponse, error) {
					return &signal.SendResponse{}, tt.clientErr
				},
			}
			m := signal.NewMessenger(client, "+1234567890")

			err := m.Send(context.Background(), tt.recipient, tt.message)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestMessengerSendTypingIndicator tests the SendTypingIndicator method.
func TestMessengerSendTypingIndicator(t *testing.T) {
	tests := []struct {
		name        string
		recipient   string
		clientErr   error
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful typing indicator",
			recipient:   "+0987654321",
			clientErr:   nil,
			expectError: false,
		},
		{
			name:        "empty recipient",
			recipient:   "",
			expectError: true,
			errorMsg:    "recipient cannot be empty",
		},
		{
			name:        "client error",
			recipient:   "+0987654321",
			clientErr:   fmt.Errorf("network error"),
			expectError: true,
			errorMsg:    "failed to send typing indicator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &signal.MockClient{
				SendTypingIndicatorFunc: func(_ context.Context, recipient string, stop bool) error {
					if recipient != tt.recipient || stop {
						t.Errorf("unexpected typing indicator: recipient=%q, stop=%v", recipient, stop)
					}
					return tt.clientErr
				},
			}
			m := signal.NewMessenger(client, "+1234567890")

			err := m.SendTypingIndicator(context.Background(), tt.recipient)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Helper functions to reduce test complexity.
func createMockSubscribeClient(clientCh chan *signal.Envelope) *signal.MockClient {
	return &signal.MockClient{
		SubscribeFunc: func(_ context.Context) (<-chan *signal.Envelope, error) {
			return clientCh, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}
}

func waitForMessage(
	t *testing.T,
	msgCh <-chan signal.IncomingMessage,
	timeout time.Duration,
) (*signal.IncomingMessage, bool) {
	t.Helper()
	select {
	case msg, ok := <-msgCh:
		return &msg, ok
	case <-time.After(timeout):
		t.Fatal("timeout waiting for message")
		return nil, false
	}
}

func expectChannelClosed(t *testing.T, ch <-chan signal.IncomingMessage, timeout time.Duration) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(timeout):
		t.Fatal("timeout waiting for channel to close")
	}
}

// TestMessengerSubscribe tests the Subscribe method and message flow.
func TestMessengerSubscribe(t *testing.T) {
	t.Run("successful subscription", testSuccessfulSubscription)
	t.Run("subscription error", testSubscriptionError)
	t.Run("filters self messages", testFiltersSelfMessages)
	t.Run("multiple subscriptions", testMultipleSubscriptions)
}

func testSuccessfulSubscription(t *testing.T) {
	// Create a channel for the mock client to send messages
	clientCh := make(chan *signal.Envelope)
	client := createMockSubscribeClient(clientCh)
	m := signal.NewMessenger(client, "+1234567890")

	// Subscribe
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, err := m.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Send a test message
	testEnv := &signal.Envelope{
		Source:       "+0987654321",
		SourceNumber: "+0987654321",
		SourceName:   "Test User",
		Timestamp:    time.Now().UnixMilli(),
		DataMessage: &signal.DataMessage{
			Message: "Hello from test",
		},
	}

	go func() {
		clientCh <- testEnv
		close(clientCh)
	}()

	// Receive and validate the message
	msg, ok := waitForMessage(t, msgCh, time.Second)
	if !ok {
		t.Fatal("message channel closed unexpectedly")
	}
	if msg.From != "Test User" {
		t.Errorf("expected From=%q, got %q", "Test User", msg.From)
	}
	if msg.Text != "Hello from test" {
		t.Errorf("expected Text=%q, got %q", "Hello from test", msg.Text)
	}

	// Cancel and verify channel closes
	cancel()
	expectChannelClosed(t, msgCh, time.Second)
}

func testSubscriptionError(t *testing.T) {
	client := &signal.MockClient{
		SubscribeFunc: func(_ context.Context) (<-chan *signal.Envelope, error) {
			return nil, fmt.Errorf("subscription failed")
		},
	}
	m := signal.NewMessenger(client, "+1234567890")

	msgCh, err := m.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe should not return error: %v", err)
	}

	// Should receive error message
	msg, ok := waitForMessage(t, msgCh, time.Second)
	if !ok {
		t.Fatal("message channel closed unexpectedly")
	}
	if msg.From != "system" {
		t.Errorf("expected system message, got from=%q", msg.From)
	}
	if !contains(msg.Text, "Failed to subscribe") {
		t.Errorf("expected error message, got %q", msg.Text)
	}
}

func testFiltersSelfMessages(t *testing.T) {
	selfPhone := "+1234567890"
	clientCh := make(chan *signal.Envelope)
	client := createMockSubscribeClient(clientCh)
	m := signal.NewMessenger(client, selfPhone)

	msgCh, err := m.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Send messages from self and other
	go func() {
		clientCh <- &signal.Envelope{
			Source:      selfPhone,
			DataMessage: &signal.DataMessage{Message: "From self"},
		}
		clientCh <- &signal.Envelope{
			SourceNumber: selfPhone,
			DataMessage:  &signal.DataMessage{Message: "Also from self"},
		}
		clientCh <- &signal.Envelope{
			Source:      "+0987654321",
			DataMessage: &signal.DataMessage{Message: "From other"},
		}
		close(clientCh)
	}()

	// Should only receive the message from other
	msg, ok := waitForMessage(t, msgCh, time.Second)
	if !ok {
		t.Fatal("message channel closed unexpectedly")
	}
	if msg.Text != "From other" {
		t.Errorf("expected message from other, got %q", msg.Text)
	}

	// Channel should close after client channel closes
	expectChannelClosed(t, msgCh, time.Second)
}

func testMultipleSubscriptions(t *testing.T) {
	client := &signal.MockClient{
		SubscribeFunc: func(ctx context.Context) (<-chan *signal.Envelope, error) {
			ch := make(chan *signal.Envelope)
			go func() {
				// Keep channel open until context is done
				<-ctx.Done()
				close(ch)
			}()
			return ch, nil
		},
	}
	m := signal.NewMessenger(client, "+1234567890")

	// First subscription
	ctx1, cancel1 := context.WithCancel(context.Background())
	ch1, err := m.Subscribe(ctx1)
	if err != nil {
		t.Fatalf("First subscribe failed: %v", err)
	}

	// Second subscription should cancel the first
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	ch2, err := m.Subscribe(ctx2)
	if err != nil {
		t.Fatalf("Second subscribe failed: %v", err)
	}

	// First channel should be closed
	expectChannelClosed(t, ch1, time.Second)

	// Second channel should still be open
	select {
	case <-ch2:
		t.Fatal("second channel should not be closed yet")
	default:
		// Expected - channel is still open
	}

	// Cleanup
	cancel1()
}

// TestMessengerConvertEnvelope tests envelope conversion logic.
// Since we can't access the unexported convertEnvelope method in black-box tests,
// we test the conversion logic indirectly through the Subscribe method.
func TestMessengerConvertEnvelope(t *testing.T) {
	t.Skip("Cannot test unexported convertEnvelope method in black-box test - " +
		"conversion logic is tested through Subscribe tests")
}

// TestMessengerConcurrency tests concurrent access.
func TestMessengerConcurrency(t *testing.T) {
	client := &signal.MockClient{
		SendFunc: func(_ context.Context, _ *signal.SendRequest) (*signal.SendResponse, error) {
			return &signal.SendResponse{}, nil
		},
		SendTypingIndicatorFunc: func(_ context.Context, _ string, _ bool) error {
			return nil
		},
		SubscribeFunc: func(ctx context.Context) (<-chan *signal.Envelope, error) {
			ch := make(chan *signal.Envelope)
			go func() {
				<-ctx.Done()
				close(ch)
			}()
			return ch, nil
		},
	}
	m := signal.NewMessenger(client, "+1234567890")

	// Run concurrent operations
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Multiple sends
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := m.Send(ctx, "+0987654321", fmt.Sprintf("Message %d", i))
			if err != nil {
				t.Errorf("Send %d failed: %v", i, err)
			}
		}(i)
	}

	// Multiple typing indicators
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := m.SendTypingIndicator(ctx, "+0987654321")
			if err != nil {
				t.Errorf("SendTypingIndicator %d failed: %v", i, err)
			}
		}(i)
	}

	// Multiple subscriptions
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := m.Subscribe(ctx)
			if err != nil {
				t.Errorf("Subscribe %d failed: %v", i, err)
				return
			}
			// Don't wait for messages since previous subscriptions get canceled
			// Just verify we can subscribe successfully
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Cancel context to clean up the last subscription
	cancel()
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
