package signal

import (
	"testing"
	"time"
)

func TestConfigZeroValue(t *testing.T) {
	var cfg Config

	// Zero value should be identifiable as uninitialized
	if cfg.SocketPath != "" {
		t.Error("Expected empty SocketPath for zero value")
	}
	if cfg.PhoneNumber != "" {
		t.Error("Expected empty PhoneNumber for zero value")
	}
	if cfg.Timeout != 0 {
		t.Error("Expected zero Timeout for zero value")
	}
}

func TestMessageZeroValue(t *testing.T) {
	var msg Message

	// Zero value should be identifiable as uninitialized
	if !msg.Timestamp.IsZero() {
		t.Error("Expected zero Timestamp for zero value")
	}
	if msg.ID != "" {
		t.Error("Expected empty ID for zero value")
	}
	if msg.Sender != "" {
		t.Error("Expected empty Sender for zero value")
	}
	if msg.Recipient != "" {
		t.Error("Expected empty Recipient for zero value")
	}
	if msg.Text != "" {
		t.Error("Expected empty Text for zero value")
	}
	if msg.IsDelivered {
		t.Error("Expected false IsDelivered for zero value")
	}
	if msg.IsRead {
		t.Error("Expected false IsRead for zero value")
	}
}

func TestTypingIndicatorZeroValue(t *testing.T) {
	var ti TypingIndicator

	// Zero value should be identifiable as uninitialized
	if ti.Recipient != "" {
		t.Error("Expected empty Recipient for zero value")
	}
	if ti.IsTyping {
		t.Error("Expected false IsTyping for zero value")
	}
}

func TestIncomingMessageZeroValue(t *testing.T) {
	var msg IncomingMessage

	// Zero value should be identifiable as uninitialized
	if !msg.Timestamp.IsZero() {
		t.Error("Expected zero time for Timestamp")
	}
	if msg.From != "" {
		t.Error("Expected empty From for zero value")
	}
	if msg.Text != "" {
		t.Error("Expected empty Text for zero value")
	}
}

func TestIncomingMessageCreation(t *testing.T) {
	now := time.Now()
	msg := IncomingMessage{
		Timestamp: now,
		From:      "+1234567890",
		Text:      "Hello, world!",
	}

	if msg.Timestamp != now {
		t.Error("Timestamp not set correctly")
	}
	if msg.From != "+1234567890" {
		t.Error("From not set correctly")
	}
	if msg.Text != "Hello, world!" {
		t.Error("Text not set correctly")
	}
}

func TestMessageCreation(t *testing.T) {
	now := time.Now()
	msg := Message{
		Timestamp:   now,
		ID:          "msg-123",
		Sender:      "+1234567890",
		Recipient:   "+0987654321",
		Text:        "Test message",
		IsDelivered: true,
		IsRead:      true,
	}

	if msg.Timestamp != now {
		t.Error("Timestamp not set correctly")
	}
	if msg.ID != "msg-123" {
		t.Error("ID not set correctly")
	}
	if msg.Sender != "+1234567890" {
		t.Error("Sender not set correctly")
	}
	if msg.Recipient != "+0987654321" {
		t.Error("Recipient not set correctly")
	}
	if msg.Text != "Test message" {
		t.Error("Text not set correctly")
	}
	if !msg.IsDelivered {
		t.Error("IsDelivered not set correctly")
	}
	if !msg.IsRead {
		t.Error("IsRead not set correctly")
	}
}
