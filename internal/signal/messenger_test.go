package signal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNewMessenger verifies messenger creation.
func TestNewMessenger(t *testing.T) {
	client := &MockClient{}
	selfPhone := "+1234567890"

	m := NewMessenger(client, selfPhone)
	if m == nil {
		t.Fatal("NewMessenger returned nil")
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
			clientErr:   errors.New("network error"),
			expectError: true,
			errorMsg:    "failed to send message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockClient{
				SendFunc: func(_ context.Context, _ *SendRequest) (*SendResponse, error) {
					return &SendResponse{}, tt.clientErr
				},
			}
			m := NewMessenger(client, "+1234567890")

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
			clientErr:   errors.New("network error"),
			expectError: true,
			errorMsg:    "failed to send typing indicator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockClient{
				SendTypingIndicatorFunc: func(_ context.Context, recipient string, stop bool) error {
					if recipient != tt.recipient || stop {
						t.Errorf("unexpected typing indicator: recipient=%q, stop=%v", recipient, stop)
					}
					return tt.clientErr
				},
			}
			m := NewMessenger(client, "+1234567890")

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

// TestMessengerSubscribe tests the Subscribe method and message flow.
func TestMessengerSubscribe(t *testing.T) {
	t.Run("successful subscription", func(t *testing.T) {
		// Create a channel for the mock client to send messages
		clientCh := make(chan *Envelope)
		client := &MockClient{
			SubscribeFunc: func(_ context.Context) (<-chan *Envelope, error) {
				return clientCh, nil
			},
			SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
				return nil
			},
		}
		m := NewMessenger(client, "+1234567890")

		// Subscribe
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		msgCh, err := m.Subscribe(ctx)
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}

		// Send a test message
		testEnv := &Envelope{
			Source:       "+0987654321",
			SourceNumber: "+0987654321",
			SourceName:   "Test User",
			Timestamp:    time.Now().UnixMilli(),
			DataMessage: &DataMessage{
				Message: "Hello from test",
			},
		}

		go func() {
			clientCh <- testEnv
			close(clientCh)
		}()

		// Receive the message
		select {
		case msg, ok := <-msgCh:
			if !ok {
				t.Fatal("message channel closed unexpectedly")
			}
			if msg.From != "Test User" {
				t.Errorf("expected From=%q, got %q", "Test User", msg.From)
			}
			if msg.Text != "Hello from test" {
				t.Errorf("expected Text=%q, got %q", "Hello from test", msg.Text)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for message")
		}

		// Cancel and verify channel closes
		cancel()
		select {
		case _, ok := <-msgCh:
			if ok {
				t.Fatal("expected channel to be closed")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for channel to close")
		}
	})

	t.Run("subscription error", func(t *testing.T) {
		client := &MockClient{
			SubscribeFunc: func(_ context.Context) (<-chan *Envelope, error) {
				return nil, errors.New("subscription failed")
			},
		}
		m := NewMessenger(client, "+1234567890")

		msgCh, err := m.Subscribe(context.Background())
		if err != nil {
			t.Fatalf("Subscribe should not return error: %v", err)
		}

		// Should receive error message
		select {
		case msg, ok := <-msgCh:
			if !ok {
				t.Fatal("message channel closed unexpectedly")
			}
			if msg.From != "system" {
				t.Errorf("expected system message, got from=%q", msg.From)
			}
			if !contains(msg.Text, "Failed to subscribe") {
				t.Errorf("expected error message, got %q", msg.Text)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for error message")
		}
	})

	t.Run("filters self messages", func(t *testing.T) {
		selfPhone := "+1234567890"
		clientCh := make(chan *Envelope)
		client := &MockClient{
			SubscribeFunc: func(_ context.Context) (<-chan *Envelope, error) {
				return clientCh, nil
			},
			SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
				return nil
			},
		}
		m := NewMessenger(client, selfPhone)

		msgCh, err := m.Subscribe(context.Background())
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}

		// Send messages from self (should be filtered)
		go func() {
			clientCh <- &Envelope{
				Source:      selfPhone,
				DataMessage: &DataMessage{Message: "From self"},
			}
			clientCh <- &Envelope{
				SourceNumber: selfPhone,
				DataMessage:  &DataMessage{Message: "Also from self"},
			}
			// Send message from other (should pass through)
			clientCh <- &Envelope{
				Source:      "+0987654321",
				DataMessage: &DataMessage{Message: "From other"},
			}
			close(clientCh)
		}()

		// Should only receive the message from other
		select {
		case msg, ok := <-msgCh:
			if !ok {
				t.Fatal("message channel closed unexpectedly")
			}
			if msg.Text != "From other" {
				t.Errorf("expected message from other, got %q", msg.Text)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for message")
		}

		// Channel should close after client channel closes
		select {
		case _, ok := <-msgCh:
			if ok {
				t.Fatal("expected channel to be closed")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for channel to close")
		}
	})

	t.Run("multiple subscriptions", func(t *testing.T) {
		client := &MockClient{
			SubscribeFunc: func(ctx context.Context) (<-chan *Envelope, error) {
				ch := make(chan *Envelope)
				go func() {
					// Keep channel open until context is done
					<-ctx.Done()
					close(ch)
				}()
				return ch, nil
			},
		}
		m := NewMessenger(client, "+1234567890")

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
		select {
		case _, ok := <-ch1:
			if ok {
				t.Fatal("expected first channel to be closed")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for first channel to close")
		}

		// Second channel should still be open
		select {
		case <-ch2:
			t.Fatal("second channel should not be closed yet")
		default:
			// Expected - channel is still open
		}

		// Cleanup
		cancel1()
	})
}

// TestMessengerConvertEnvelope tests envelope conversion logic.
func TestMessengerConvertEnvelope(t *testing.T) {
	selfPhone := "+1234567890"
	m := &messenger{selfPhone: selfPhone}

	tests := []struct {
		name     string
		envelope *Envelope
		want     *IncomingMessage
	}{
		{
			name: "data message with all fields",
			envelope: &Envelope{
				Source:       "+0987654321",
				SourceNumber: "+0987654321",
				SourceName:   "Test User",
				Timestamp:    1000,
				DataMessage: &DataMessage{
					Message: "Hello",
				},
			},
			want: &IncomingMessage{
				Timestamp: time.Unix(0, 1000*int64(time.Millisecond)),
				From:      "Test User",
				Text:      "Hello",
			},
		},
		{
			name: "data message without name",
			envelope: &Envelope{
				SourceNumber: "+0987654321",
				Timestamp:    1000,
				DataMessage: &DataMessage{
					Message: "Hello",
				},
			},
			want: &IncomingMessage{
				Timestamp: time.Unix(0, 1000*int64(time.Millisecond)),
				From:      "+0987654321",
				Text:      "Hello",
			},
		},
		{
			name: "message from self",
			envelope: &Envelope{
				Source: selfPhone,
				DataMessage: &DataMessage{
					Message: "Self message",
				},
			},
			want: nil,
		},
		{
			name: "empty data message",
			envelope: &Envelope{
				Source: "+0987654321",
				DataMessage: &DataMessage{
					Message: "",
				},
			},
			want: nil,
		},
		{
			name: "typing message",
			envelope: &Envelope{
				Source: "+0987654321",
				TypingMessage: &TypingMessage{
					Action: "STARTED",
				},
			},
			want: nil,
		},
		{
			name: "receipt message",
			envelope: &Envelope{
				Source: "+0987654321",
				ReceiptMessage: &ReceiptMessage{
					IsRead: true,
					When:   time.Now().UnixMilli(),
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.convertEnvelope(tt.envelope)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
			} else {
				if got == nil {
					t.Errorf("expected %+v, got nil", tt.want)
				} else {
					if got.From != tt.want.From {
						t.Errorf("From: expected %q, got %q", tt.want.From, got.From)
					}
					if got.Text != tt.want.Text {
						t.Errorf("Text: expected %q, got %q", tt.want.Text, got.Text)
					}
					if !got.Timestamp.Equal(tt.want.Timestamp) {
						t.Errorf("Timestamp: expected %v, got %v", tt.want.Timestamp, got.Timestamp)
					}
				}
			}
		})
	}
}

// TestMessengerConcurrency tests concurrent access.
func TestMessengerConcurrency(t *testing.T) {
	client := &MockClient{
		SendFunc: func(_ context.Context, _ *SendRequest) (*SendResponse, error) {
			return &SendResponse{}, nil
		},
		SendTypingIndicatorFunc: func(_ context.Context, _ string, _ bool) error {
			return nil
		},
		SubscribeFunc: func(ctx context.Context) (<-chan *Envelope, error) {
			ch := make(chan *Envelope)
			go func() {
				<-ctx.Done()
				close(ch)
			}()
			return ch, nil
		},
	}
	m := NewMessenger(client, "+1234567890")

	// Run concurrent operations
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Multiple sends
	for i := 0; i < 10; i++ {
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
	for i := 0; i < 10; i++ {
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
	for i := 0; i < 5; i++ {
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