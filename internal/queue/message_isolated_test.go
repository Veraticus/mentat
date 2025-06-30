// +build isolated

package queue_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

func TestNewQueuedMessage_Isolated(t *testing.T) {
	msg := signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Hello, world!",
		Timestamp: time.Now(),
	}
	
	qm := queue.NewQueuedMessage(msg, queue.PriorityNormal)
	
	// Verify basic fields
	expectedIDPrefix := fmt.Sprintf("%d-%s", msg.Timestamp.UnixNano(), msg.From)
	if qm.ID != expectedIDPrefix {
		t.Errorf("expected ID %s, got %s", expectedIDPrefix, qm.ID)
	}
	
	expectedConvID := fmt.Sprintf("conv-%s", msg.From)
	if qm.ConversationID != expectedConvID {
		t.Errorf("expected ConversationID %s, got %s", expectedConvID, qm.ConversationID)
	}
	
	if qm.From != msg.From {
		t.Errorf("expected From %s, got %s", msg.From, qm.From)
	}
	
	if qm.Text != msg.Text {
		t.Errorf("expected Text %s, got %s", msg.Text, qm.Text)
	}
	
	// Verify initial state
	if qm.State != queue.MessageStateQueued {
		t.Errorf("expected initial state %v, got %v", queue.MessageStateQueued, qm.State)
	}
	
	// Verify state history
	history := qm.GetStateHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 state history entry, got %d", len(history))
	}
	
	// Verify defaults
	if qm.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", qm.Attempts)
	}
	if qm.MaxAttempts != 3 {
		t.Errorf("expected 3 max attempts, got %d", qm.MaxAttempts)
	}
}

func TestQueuedMessage_StateTransitions_Isolated(t *testing.T) {
	msg := signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Test message",
		Timestamp: time.Now(),
	}
	
	qm := queue.NewQueuedMessage(msg, queue.PriorityNormal)
	
	// Test valid transition
	newQm, err := qm.WithState(queue.MessageStateProcessing, "worker picked up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Old message should be unchanged
	if qm.State != queue.MessageStateQueued {
		t.Errorf("original message state changed")
	}
	
	// New message should have new state
	if newQm.State != queue.MessageStateProcessing {
		t.Errorf("expected state %v, got %v", queue.MessageStateProcessing, newQm.State)
	}
	
	// ProcessedAt should be set
	if newQm.ProcessedAt == nil {
		t.Errorf("ProcessedAt not set")
	}
	
	// Test invalid transition
	_, err = qm.WithState(queue.MessageStateCompleted, "skip processing")
	if err == nil {
		t.Errorf("expected error for invalid transition")
	}
}

func TestQueuedMessage_ErrorTracking_Isolated(t *testing.T) {
	msg := signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Test message",
		Timestamp: time.Now(),
	}
	
	qm := queue.NewQueuedMessage(msg, queue.PriorityNormal)
	testError := errors.New("test error")
	
	// Add error
	newQm := qm.WithError(testError, queue.MessageStateProcessing)
	
	// Original should be unchanged
	if qm.LastError != nil {
		t.Errorf("original message error was modified")
	}
	
	// New message should have error
	if newQm.LastError != testError {
		t.Errorf("expected LastError to be %v, got %v", testError, newQm.LastError)
	}
	
	// Check error history
	errorHistory := newQm.GetErrorHistory()
	if len(errorHistory) != 1 {
		t.Fatalf("expected 1 error history entry, got %d", len(errorHistory))
	}
}