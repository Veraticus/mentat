package signal_test

import (
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// TestIncomingMessageZeroValue tests the zero value of the exported IncomingMessage type.
func TestIncomingMessageZeroValue(t *testing.T) {
	var msg signal.IncomingMessage

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

// TestIncomingMessageCreation tests creating an IncomingMessage.
func TestIncomingMessageCreation(t *testing.T) {
	now := time.Now()
	msg := signal.IncomingMessage{
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

// Note: Tests for unexported types (Config, Message, TypingIndicator) have been removed
// as they cannot be tested in black-box tests. These types should be tested in their
// respective implementation files using internal tests if needed.
