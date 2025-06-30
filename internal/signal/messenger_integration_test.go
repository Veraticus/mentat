package signal

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestMessengerIntegrationMessageFlow verifies messages flow through subscription channel.
func TestMessengerIntegrationMessageFlow(t *testing.T) {
	// Create a mock client with a channel we control
	clientCh := make(chan *Envelope, 10)
	client := &MockClient{
		SubscribeFunc: func(_ context.Context) (<-chan *Envelope, error) {
			return clientCh, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}

	selfPhone := "+1234567890"
	m := NewMessenger(client, selfPhone)

	// Set up subscription
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, err := m.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Test data
	testMessages := []struct {
		envelope        *Envelope
		expectedMessage *IncomingMessage
		shouldReceive   bool
	}{
		{
			// Regular message from another user
			envelope: &Envelope{
				Source:       "+0987654321",
				SourceName:   "Alice",
				Timestamp:    time.Now().UnixMilli(),
				DataMessage:  &DataMessage{Message: "Hello, Bob!"},
			},
			expectedMessage: &IncomingMessage{
				From: "Alice",
				Text: "Hello, Bob!",
			},
			shouldReceive: true,
		},
		{
			// Message from user without name
			envelope: &Envelope{
				SourceNumber: "+1122334455",
				Timestamp:    time.Now().UnixMilli(),
				DataMessage:  &DataMessage{Message: "Anonymous message"},
			},
			expectedMessage: &IncomingMessage{
				From: "+1122334455",
				Text: "Anonymous message",
			},
			shouldReceive: true,
		},
		{
			// Message from self (should be filtered)
			envelope: &Envelope{
				Source:      selfPhone,
				Timestamp:   time.Now().UnixMilli(),
				DataMessage: &DataMessage{Message: "Self message"},
			},
			shouldReceive: false,
		},
		{
			// Typing indicator (should be filtered)
			envelope: &Envelope{
				Source:        "+0987654321",
				TypingMessage: &TypingMessage{Action: "STARTED"},
			},
			shouldReceive: false,
		},
		{
			// Read receipt (should be filtered)
			envelope: &Envelope{
				Source:         "+0987654321",
				ReceiptMessage: &ReceiptMessage{IsRead: true},
			},
			shouldReceive: false,
		},
		{
			// Empty message (should be filtered)
			envelope: &Envelope{
				Source:      "+0987654321",
				DataMessage: &DataMessage{Message: ""},
			},
			shouldReceive: false,
		},
		{
			// Group message
			envelope: &Envelope{
				Source:     "+5556667777",
				SourceName: "Charlie",
				Timestamp:  time.Now().UnixMilli(),
				DataMessage: &DataMessage{
					Message: "Group chat message",
					GroupInfo: &GroupInfo{
						GroupID: "group123",
						Type:    "UPDATE",
					},
				},
			},
			expectedMessage: &IncomingMessage{
				From: "Charlie",
				Text: "Group chat message",
			},
			shouldReceive: true,
		},
	}

	// Send messages and verify reception
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		for _, test := range testMessages {
			// Send the envelope
			clientCh <- test.envelope

			if test.shouldReceive {
				// Should receive message
				select {
				case msg := <-msgCh:
					if msg.From != test.expectedMessage.From {
						t.Errorf("Expected From=%q, got %q", test.expectedMessage.From, msg.From)
					}
					if msg.Text != test.expectedMessage.Text {
						t.Errorf("Expected Text=%q, got %q", test.expectedMessage.Text, msg.Text)
					}
				case <-time.After(100 * time.Millisecond):
					t.Errorf("Timeout waiting for message: %+v", test.expectedMessage)
				}
			} else {
				// Should NOT receive message
				select {
				case msg := <-msgCh:
					t.Errorf("Unexpected message received: %+v", msg)
				case <-time.After(50 * time.Millisecond):
					// Expected - no message
				}
			}
		}
		
		// Close the client channel
		close(clientCh)
	}()

	// Wait for test completion
	wg.Wait()

	// Verify subscription channel closes when client channel closes
	select {
	case _, ok := <-msgCh:
		if ok {
			t.Error("Expected message channel to be closed")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for channel to close")
	}
}

// TestMessengerIntegrationConcurrentMessages tests handling of concurrent messages.
func TestMessengerIntegrationConcurrentMessages(t *testing.T) {
	clientCh := make(chan *Envelope, 100)
	client := &MockClient{
		SubscribeFunc: func(_ context.Context) (<-chan *Envelope, error) {
			return clientCh, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}

	m := NewMessenger(client, "+1234567890")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, err := m.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Send many messages concurrently
	messageCount := 100
	var sendWg sync.WaitGroup
	sendWg.Add(messageCount)

	for i := 0; i < messageCount; i++ {
		go func(_ int) {
			defer sendWg.Done()
			clientCh <- &Envelope{
				Source:      "+0987654321",
				SourceName:  "Concurrent User",
				Timestamp:   time.Now().UnixMilli(),
				DataMessage: &DataMessage{Message: "Concurrent message"},
			}
		}(i)
	}

	// Receive all messages
	received := 0
	receiveTimeout := time.After(5 * time.Second)
	
	for received < messageCount {
		select {
		case msg := <-msgCh:
			if msg.From != "Concurrent User" || msg.Text != "Concurrent message" {
				t.Errorf("Unexpected message: %+v", msg)
			}
			received++
		case <-receiveTimeout:
			t.Fatalf("Timeout: only received %d of %d messages", received, messageCount)
		}
	}

	sendWg.Wait()
	close(clientCh)
}

// TestMessengerIntegrationContextCancellation tests proper cleanup on context cancellation.
func TestMessengerIntegrationContextCancellation(t *testing.T) {
	clientCh := make(chan *Envelope)
	var clientCtx context.Context
	ctxCaptured := make(chan struct{})
	
	client := &MockClient{
		SubscribeFunc: func(ctx context.Context) (<-chan *Envelope, error) {
			clientCtx = ctx
			close(ctxCaptured)
			return clientCh, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}

	m := NewMessenger(client, "+1234567890")

	// Subscribe with cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	msgCh, err := m.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Wait for client context to be captured
	<-ctxCaptured

	// Send a message to verify subscription is working
	go func() {
		clientCh <- &Envelope{
			Source:      "+0987654321",
			DataMessage: &DataMessage{Message: "Test message"},
		}
	}()

	select {
	case msg := <-msgCh:
		if msg.Text != "Test message" {
			t.Errorf("Unexpected message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for test message")
	}

	// Cancel context
	cancel()

	// Verify client context is canceled
	select {
	case <-clientCtx.Done():
		// Expected
	case <-time.After(time.Second):
		t.Error("Client context was not canceled")
	}

	// Verify message channel is closed
	select {
	case _, ok := <-msgCh:
		if ok {
			t.Error("Expected message channel to be closed")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for message channel to close")
	}
}

// TestMessengerIntegrationSubscriptionReplacement tests that new subscriptions cancel old ones.
func TestMessengerIntegrationSubscriptionReplacement(t *testing.T) {
	var mu sync.Mutex
	var contexts []context.Context
	
	client := &MockClient{
		SubscribeFunc: func(ctx context.Context) (<-chan *Envelope, error) {
			mu.Lock()
			contexts = append(contexts, ctx)
			mu.Unlock()
			
			ch := make(chan *Envelope)
			go func() {
				<-ctx.Done()
				close(ch)
			}()
			return ch, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}

	m := NewMessenger(client, "+1234567890")

	// Create multiple subscriptions
	subscriptionCount := 5
	var channels []<-chan IncomingMessage

	for i := 0; i < subscriptionCount; i++ {
		ctx := context.Background()
		ch, err := m.Subscribe(ctx)
		if err != nil {
			t.Fatalf("Subscribe %d failed: %v", i, err)
		}
		channels = append(channels, ch)
	}

	// All previous channels should be closed except the last one
	for i, ch := range channels {
		if i < len(channels)-1 {
			// Should be closed
			select {
			case _, ok := <-ch:
				if ok {
					t.Errorf("Channel %d should be closed", i)
				}
			case <-time.After(time.Second):
				t.Errorf("Timeout waiting for channel %d to close", i)
			}
		} else {
			// Last channel should be open
			select {
			case <-ch:
				t.Error("Last channel should not be closed")
			default:
				// Expected
			}
		}
	}

	// Give a small delay to ensure all contexts are processed
	time.Sleep(100 * time.Millisecond)
	
	// Verify all contexts except the last are canceled
	mu.Lock()
	ctxs := contexts
	mu.Unlock()

	// The messenger creates its own derived contexts, so we just verify
	// that previous subscriptions' contexts are canceled
	for i, ctx := range ctxs {
		if i < len(ctxs)-1 {
			select {
			case <-ctx.Done():
				// Expected - previous subscriptions should be canceled
			default:
				t.Errorf("Context %d should be canceled", i)
			}
		}
		// Note: We don't check the last context because it's managed by
		// the messenger and may or may not be canceled depending on timing
	}
}