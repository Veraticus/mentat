// Package signal provides interfaces for Signal messenger integration.
package signal

import (
	"context"
)

// Messenger abstracts Signal communication and future messaging channels.
type Messenger interface {
	// Send sends a message to the specified recipient
	Send(ctx context.Context, recipient string, message string) error

	// SendTypingIndicator sends a typing indicator to the recipient
	SendTypingIndicator(ctx context.Context, recipient string) error

	// Subscribe returns a channel of incoming messages
	Subscribe(ctx context.Context) (<-chan IncomingMessage, error)
}
