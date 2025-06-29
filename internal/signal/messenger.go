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
	ctx    context.Context
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
		ctx:    subCtx,
		cancel: cancel,
		outCh:  outCh,
		done:   done,
	}
	m.subscription = sub

	// Start the subscription goroutine
	go m.runSubscription(sub)

	return outCh, nil
}

// runSubscription handles the subscription lifecycle.
func (m *messenger) runSubscription(sub *subscription) {
	defer close(sub.done)
	defer close(sub.outCh) // Close channel from sender side only

	// Subscribe to messages from the client
	msgCh, err := m.client.Subscribe(sub.ctx)
	if err != nil {
		// If we can't subscribe, send an error message on the channel
		select {
		case sub.outCh <- IncomingMessage{
			Timestamp: time.Now(),
			From:      "system",
			Text:      fmt.Sprintf("Failed to subscribe to messages: %v", err),
		}:
		case <-sub.ctx.Done():
		}
		return
	}

	// Process incoming messages
	for {
		select {
		case <-sub.ctx.Done():
			return

		case envelope, ok := <-msgCh:
			if !ok {
				// Channel closed, subscription ended
				return
			}

			// Convert envelope to IncomingMessage
			msg := m.convertEnvelope(envelope)
			if msg != nil {
				select {
				case sub.outCh <- *msg:
				case <-sub.ctx.Done():
					return
				}
			}
		}
	}
}

// convertEnvelope converts a Signal envelope to an IncomingMessage.
func (m *messenger) convertEnvelope(env *Envelope) *IncomingMessage {
	// Skip messages from self
	if env.Source == m.selfPhone || env.SourceNumber == m.selfPhone {
		return nil
	}

	// Handle data messages
	if env.DataMessage != nil && env.DataMessage.Message != "" {
		return &IncomingMessage{
			Timestamp: time.Unix(0, env.Timestamp*int64(time.Millisecond)),
			From:      m.getSource(env),
			Text:      env.DataMessage.Message,
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

// getSource extracts the sender's identifier from an envelope.
func (m *messenger) getSource(env *Envelope) string {
	if env.SourceName != "" {
		return env.SourceName
	}
	if env.SourceNumber != "" {
		return env.SourceNumber
	}
	if env.Source != "" {
		return env.Source
	}
	return "unknown"
}