package signal_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

type messageFlowTest struct {
	envelope        *signal.Envelope
	expectedMessage *signal.IncomingMessage
	shouldReceive   bool
}

func createTestMessages(selfPhone string) []messageFlowTest {
	return []messageFlowTest{
		{
			// Regular message from another user
			envelope: &signal.Envelope{
				Source:      "+0987654321",
				SourceName:  "Alice",
				Timestamp:   time.Now().UnixMilli(),
				DataMessage: &signal.DataMessage{Message: "Hello, Bob!"},
			},
			expectedMessage: &signal.IncomingMessage{
				From: "Alice",
				Text: "Hello, Bob!",
			},
			shouldReceive: true,
		},
		{
			// Message from user without name
			envelope: &signal.Envelope{
				SourceNumber: "+1122334455",
				Timestamp:    time.Now().UnixMilli(),
				DataMessage:  &signal.DataMessage{Message: "Anonymous message"},
			},
			expectedMessage: &signal.IncomingMessage{
				From: "+1122334455",
				Text: "Anonymous message",
			},
			shouldReceive: true,
		},
		{
			// Message from self (should be filtered)
			envelope: &signal.Envelope{
				Source:      selfPhone,
				Timestamp:   time.Now().UnixMilli(),
				DataMessage: &signal.DataMessage{Message: "Self message"},
			},
			shouldReceive: false,
		},
		{
			// Typing indicator (should be filtered)
			envelope: &signal.Envelope{
				Source:        "+0987654321",
				TypingMessage: &signal.TypingMessage{Action: "STARTED"},
			},
			shouldReceive: false,
		},
		{
			// Read receipt (should be filtered)
			envelope: &signal.Envelope{
				Source:         "+0987654321",
				ReceiptMessage: &signal.ReceiptMessage{IsRead: true},
			},
			shouldReceive: false,
		},
		{
			// Empty message (should be filtered)
			envelope: &signal.Envelope{
				Source:      "+0987654321",
				DataMessage: &signal.DataMessage{Message: ""},
			},
			shouldReceive: false,
		},
		{
			// Group message
			envelope: &signal.Envelope{
				Source:     "+5556667777",
				SourceName: "Charlie",
				Timestamp:  time.Now().UnixMilli(),
				DataMessage: &signal.DataMessage{
					Message: "Group chat message",
					GroupInfo: &signal.GroupInfo{
						GroupID: "group123",
						Type:    "UPDATE",
					},
				},
			},
			expectedMessage: &signal.IncomingMessage{
				From: "Charlie",
				Text: "Group chat message",
			},
			shouldReceive: true,
		},
	}
}

func verifyMessage(t *testing.T, msgCh <-chan signal.IncomingMessage, test messageFlowTest) {
	t.Helper()

	if test.shouldReceive {
		verifyExpectedMessage(t, msgCh, test.expectedMessage)
	} else {
		verifyNoMessage(t, msgCh)
	}
}

func verifyExpectedMessage(t *testing.T, msgCh <-chan signal.IncomingMessage, expected *signal.IncomingMessage) {
	t.Helper()

	select {
	case msg := <-msgCh:
		if msg.From != expected.From {
			t.Errorf("Expected From=%q, got %q", expected.From, msg.From)
		}
		if msg.Text != expected.Text {
			t.Errorf("Expected Text=%q, got %q", expected.Text, msg.Text)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Timeout waiting for message: %+v", expected)
	}
}

func verifyNoMessage(t *testing.T, msgCh <-chan signal.IncomingMessage) {
	t.Helper()

	select {
	case msg := <-msgCh:
		t.Errorf("Unexpected message received: %+v", msg)
	case <-time.After(50 * time.Millisecond):
		// Expected - no message
	}
}

func verifyChannelClosed(t *testing.T, msgCh <-chan signal.IncomingMessage) {
	t.Helper()

	select {
	case _, ok := <-msgCh:
		if ok {
			t.Error("Expected message channel to be closed")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for channel to close")
	}
}

// TestMessengerIntegrationMessageFlow verifies messages flow through subscription channel.
func TestMessengerIntegrationMessageFlow(t *testing.T) {
	// Create a mock client with a channel we control
	clientCh := make(chan *signal.Envelope, 10)
	client := &signal.MockClient{
		SubscribeFunc: func(_ context.Context) (<-chan *signal.Envelope, error) {
			return clientCh, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}

	selfPhone := "+1234567890"
	m := signal.NewMessenger(client, selfPhone)

	// Set up subscription
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, err := m.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Test data
	testMessages := createTestMessages(selfPhone)

	// Send messages and verify reception
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		for _, test := range testMessages {
			clientCh <- test.envelope
			verifyMessage(t, msgCh, test)
		}

		close(clientCh)
	}()

	// Wait for test completion
	wg.Wait()

	// Verify subscription channel closes when client channel closes
	verifyChannelClosed(t, msgCh)
}

// TestMessengerIntegrationConcurrentMessages tests handling of concurrent messages.
func TestMessengerIntegrationConcurrentMessages(t *testing.T) {
	clientCh := make(chan *signal.Envelope, 100)
	client := &signal.MockClient{
		SubscribeFunc: func(_ context.Context) (<-chan *signal.Envelope, error) {
			return clientCh, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}

	m := signal.NewMessenger(client, "+1234567890")

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

	for i := range messageCount {
		go func(_ int) {
			defer sendWg.Done()
			clientCh <- &signal.Envelope{
				Source:      "+0987654321",
				SourceName:  "Concurrent User",
				Timestamp:   time.Now().UnixMilli(),
				DataMessage: &signal.DataMessage{Message: "Concurrent message"},
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
	clientCh := make(chan *signal.Envelope)
	ctxChan := make(chan context.Context, 1)
	ctxCaptured := make(chan struct{})

	client := &signal.MockClient{
		SubscribeFunc: func(ctx context.Context) (<-chan *signal.Envelope, error) {
			select {
			case ctxChan <- ctx:
				// Store the context
			default:
				// Channel already has a context, replace it
				<-ctxChan
				ctxChan <- ctx
			}
			close(ctxCaptured)
			return clientCh, nil
		},
		SendReceiptFunc: func(_ context.Context, _ string, _ int64, _ string) error {
			return nil
		},
	}

	m := signal.NewMessenger(client, "+1234567890")

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
		clientCh <- &signal.Envelope{
			Source:      "+0987654321",
			DataMessage: &signal.DataMessage{Message: "Test message"},
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

	// Get the captured context from the channel
	var capturedCtx context.Context
	select {
	case capturedCtx = <-ctxChan:
		// Got the context
	default:
		t.Fatal("No context was captured")
	}

	// Verify client context is canceled
	select {
	case <-capturedCtx.Done():
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

	client := &signal.MockClient{
		SubscribeFunc: func(ctx context.Context) (<-chan *signal.Envelope, error) {
			mu.Lock()
			contexts = append(contexts, ctx)
			mu.Unlock()

			ch := make(chan *signal.Envelope)
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

	m := signal.NewMessenger(client, "+1234567890")

	// Create multiple subscriptions
	subscriptionCount := 5
	channels := make([]<-chan signal.IncomingMessage, 0, subscriptionCount)

	for i := range subscriptionCount {
		ctx := context.Background()
		ch, err := m.Subscribe(ctx)
		if err != nil {
			t.Fatalf("Subscribe %d failed: %v", i, err)
		}
		channels = append(channels, ch)
	}

	// Verify channel states
	verifyChannelStates(t, channels)

	// Allow contexts to be processed
	<-time.After(100 * time.Millisecond)

	// Verify all contexts except the last are canceled
	mu.Lock()
	ctxs := contexts
	mu.Unlock()

	verifyContextStates(t, ctxs)
}

func verifyChannelStates(t *testing.T, channels []<-chan signal.IncomingMessage) {
	t.Helper()
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
}

func verifyContextStates(t *testing.T, ctxs []context.Context) {
	t.Helper()
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
