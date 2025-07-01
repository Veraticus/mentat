package signal

import "time"

// Config holds configuration for Signal-CLI connection.
type Config struct {
	// SocketPath is the path to the Signal-CLI JSON-RPC socket.
	SocketPath string

	// PhoneNumber is the registered phone number for the Signal account.
	PhoneNumber string

	// Timeout is the maximum time to wait for operations.
	Timeout time.Duration
}

// Message represents a Signal message with all metadata.
type Message struct {
	Timestamp   time.Time `json:"timestamp"`    // 8 bytes
	ID          string    `json:"id"`           // 16 bytes
	Sender      string    `json:"sender"`       // 16 bytes
	Recipient   string    `json:"recipient"`    // 16 bytes
	Text        string    `json:"text"`         // 16 bytes
	IsDelivered bool      `json:"is_delivered"` // 1 byte
	IsRead      bool      `json:"is_read"`      // 1 byte
}

// TypingIndicator represents a typing notification.
type TypingIndicator struct {
	Recipient string `json:"recipient"`
	IsTyping  bool   `json:"is_typing"`
}

// IncomingMessage represents a message received from a user.
type IncomingMessage struct {
	Timestamp  time.Time // 8 bytes
	From       string    // 16 bytes - Display name
	FromNumber string    // 16 bytes - Phone number
	Text       string    // 16 bytes
}
