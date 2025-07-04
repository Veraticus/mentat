package signal

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// messenger implements the Messenger interface using a Signal Client.
type messenger struct {
	client       Client
	selfPhone    string
	mu           sync.Mutex
	subscription *subscription
}

// subscription tracks an active message subscription.
type subscription struct {
	cancel context.CancelFunc
	outCh  chan IncomingMessage
	done   chan struct{}
}

// NewMessenger creates a new Signal messenger.
func NewMessenger(client Client, selfPhone string) Messenger {
	return &messenger{
		client:    client,
		selfPhone: selfPhone,
	}
}

// Send sends a message to the specified recipient.
func (m *messenger) Send(ctx context.Context, recipient string, message string) error {
	if recipient == "" {
		return fmt.Errorf("recipient cannot be empty")
	}
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	req := &SendRequest{
		Message:    message,
		Recipients: []string{recipient},
	}

	_, err := m.client.Send(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// SendTypingIndicator sends a typing indicator to the recipient.
func (m *messenger) SendTypingIndicator(ctx context.Context, recipient string) error {
	if recipient == "" {
		return fmt.Errorf("recipient cannot be empty")
	}

	err := m.client.SendTypingIndicator(ctx, recipient, false)
	if err != nil {
		return fmt.Errorf("failed to send typing indicator: %w", err)
	}

	return nil
}

// Subscribe returns a channel of incoming messages.
func (m *messenger) Subscribe(ctx context.Context) (<-chan IncomingMessage, error) {
	debugLog("Subscribe called")
	m.mu.Lock()
	defer m.mu.Unlock()

	// If there's an existing subscription, close it first
	if m.subscription != nil {
		m.subscription.cancel()
		<-m.subscription.done
		m.subscription = nil
	}

	// Create a new subscription
	subCtx, cancel := context.WithCancel(ctx)
	outCh := make(chan IncomingMessage)
	done := make(chan struct{})

	sub := &subscription{
		cancel: cancel,
		outCh:  outCh,
		done:   done,
	}
	m.subscription = sub

	// Start the subscription goroutine
	go m.runSubscription(subCtx, sub)

	return outCh, nil
}

// runSubscription handles the subscription lifecycle.
func (m *messenger) runSubscription(ctx context.Context, sub *subscription) {
	defer close(sub.done)
	defer close(sub.outCh) // Close channel from sender side only

	// Subscribe to messages from the client
	debugLog("Calling client.Subscribe")
	msgCh, err := m.client.Subscribe(ctx)
	if err != nil {
		m.handleSubscriptionError(ctx, sub, err)
		return
	}

	// Process incoming messages
	m.processMessages(ctx, sub, msgCh)
}

func (m *messenger) handleSubscriptionError(ctx context.Context, sub *subscription, err error) {
	debugLog("client.Subscribe error: %v", err)
	// If we can't subscribe, send an error message on the channel
	select {
	case sub.outCh <- IncomingMessage{
		Timestamp: time.Now(),
		From:      "system",
		Text:      fmt.Sprintf("Failed to subscribe to messages: %v", err),
	}:
	case <-ctx.Done():
	}
}

func (m *messenger) processMessages(ctx context.Context, sub *subscription, msgCh <-chan *Envelope) {
	for {
		select {
		case <-ctx.Done():
			return

		case envelope, ok := <-msgCh:
			if !ok {
				debugLog("Message channel closed")
				return
			}
			m.handleEnvelope(ctx, sub, envelope)
		}
	}
}

func (m *messenger) handleEnvelope(ctx context.Context, sub *subscription, envelope *Envelope) {
	debugLog("Received envelope: %+v", envelope)

	msg := m.convertEnvelope(envelope)
	if msg == nil {
		debugLog("Message conversion returned nil")
		return
	}

	debugLog("Message converted successfully: from=%s, text=%q", msg.From, msg.Text)

	// Send read receipt for data messages
	if envelope.DataMessage != nil {
		go func(ctx context.Context) {
			_ = m.client.SendReceipt(ctx, envelope.Source, envelope.Timestamp, "read")
		}(ctx)
	}

	select {
	case sub.outCh <- *msg:
		debugLog("Message sent to output channel")
	case <-ctx.Done():
		debugLog("Context canceled while sending message")
	}
}

// convertEnvelope converts a Signal envelope to an IncomingMessage.
func (m *messenger) convertEnvelope(env *Envelope) *IncomingMessage {
	// Skip messages from self
	if env.Source == m.selfPhone || env.SourceNumber == m.selfPhone {
		debugLog("Skipping self message: source=%s, sourceNumber=%s, selfPhone=%s",
			env.Source, env.SourceNumber, m.selfPhone)
		return nil
	}

	// Handle data messages
	if env.DataMessage != nil && env.DataMessage.Message != "" {
		debugLog("Converting data message: %q", env.DataMessage.Message)
		return &IncomingMessage{
			Timestamp:  time.Unix(0, env.Timestamp*int64(time.Millisecond)),
			From:       m.getSourceName(env),
			FromNumber: m.getSourceNumber(env),
			Text:       env.DataMessage.Message,
		}
	}

	// Skip sync messages (messages sent from another device)
	// These are messages we sent from another device, so we don't want to process them
	if env.SyncMessage != nil && env.SyncMessage.SentMessage != nil {
		return nil
	}

	// Ignore other message types (typing, receipts, etc.)
	return nil
}

// getSourceName extracts the sender's display name from an envelope.
func (m *messenger) getSourceName(env *Envelope) string {
	if env.SourceName != "" {
		return env.SourceName
	}
	// Fall back to number if no name
	return m.getSourceNumber(env)
}

// getSourceNumber extracts the sender's phone number from an envelope.
func (m *messenger) getSourceNumber(env *Envelope) string {
	if env.SourceNumber != "" {
		return env.SourceNumber
	}
	if env.Source != "" {
		return env.Source
	}
	return ""
}
