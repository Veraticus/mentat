package queue

import (
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

func TestMessageQueue_EnqueueAndGetNext(t *testing.T) {
	q := NewMessageQueue()

	// Enqueue a message
	msg := signal.IncomingMessage{
		From:       "John Doe",
		FromNumber: "+1234567890",
		Text:       "Test message",
		Timestamp:  time.Now(),
	}

	err := q.Enqueue(msg)
	if err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}

	// Get the next message
	queuedMsg, err := q.GetNext("worker-1")
	if err != nil {
		t.Fatalf("Failed to get next message: %v", err)
	}

	if queuedMsg == nil {
		t.Fatal("Expected to get a message, got nil")
	}

	if queuedMsg.From != msg.From {
		t.Errorf("Expected from %s, got %s", msg.From, queuedMsg.From)
	}

	if queuedMsg.State != MessageStateProcessing {
		t.Errorf("Expected state %v, got %v", MessageStateProcessing, queuedMsg.State)
	}

	// Getting next should return nil (no more queued messages)
	queuedMsg2, err := q.GetNext("worker-2")
	if err != nil {
		t.Fatalf("Failed to get next message: %v", err)
	}
	if queuedMsg2 != nil {
		t.Error("Expected nil when no queued messages available")
	}
}

func TestMessageQueue_UpdateState(t *testing.T) {
	q := NewMessageQueue()

	// Enqueue a message
	msg := signal.IncomingMessage{
		From:       "John Doe",
		FromNumber: "+1234567890",
		Text:       "Test message",
		Timestamp:  time.Now(),
	}

	err := q.Enqueue(msg)
	if err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}

	// Get the message to transition it to processing
	queuedMsg, err := q.GetNext("worker-1")
	if err != nil {
		t.Fatalf("Failed to get next message: %v", err)
	}
	if queuedMsg == nil {
		t.Fatal("Expected to get a message")
	}

	// Set an error on the message before failing
	sq, ok := q.(*simpleMessageQueue)
	if !ok {
		t.Fatal("Failed to cast to simpleMessageQueue")
	}
	sq.mu.Lock()
	if msg, exists := sq.messages[queuedMsg.ID]; exists {
		msg.SetError(fmt.Errorf("test error"))
	}
	sq.mu.Unlock()
	
	// Update to retrying (valid transition from processing with error)
	err = q.UpdateState(queuedMsg.ID, MessageStateFailed, "failed")
	if err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}
	
	// Should be in retrying state since it has attempts left
	sq.mu.RLock()
	if msg, exists := sq.messages[queuedMsg.ID]; exists {
		msgState := msg.GetState()
		sq.mu.RUnlock()
		if msgState != StateRetrying {
			t.Errorf("Expected state %v, got %v", StateRetrying, msgState)
		}
	} else {
		sq.mu.RUnlock()
		t.Error("Message not found")
	}

	// Try to update non-existent message
	err = q.UpdateState("msg-999", MessageStateCompleted, "test")
	if err == nil {
		t.Error("Expected error for non-existent message")
	}
}

func TestMessageQueue_Stats(t *testing.T) {
	q := NewMessageQueue()

	// Enqueue multiple messages
	var msgIDs []string
	for i := 0; i < 5; i++ {
		msg := signal.IncomingMessage{
			From:       fmt.Sprintf("User %d", i%2),
			FromNumber: fmt.Sprintf("+123456789%d", i%2), // 2 conversations
			Text:       "Test message",
			Timestamp:  time.Now(),
		}
		err := q.Enqueue(msg)
		if err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}
	}

	// Process some messages and store their IDs
	for i := 0; i < 2; i++ {
		qMsg, err := q.GetNext(fmt.Sprintf("worker-%d", i))
		if err != nil {
			t.Fatalf("Failed to get message: %v", err)
		}
		if qMsg != nil {
			msgIDs = append(msgIDs, qMsg.ID)
		}
	}

	// Fail one permanently (set max attempts and error to prevent retry)
	if len(msgIDs) > 0 {
		// Access the internal message queue to set max attempts and error
		// In a real scenario, this would be handled by the worker
		sq, ok := q.(*simpleMessageQueue)
		if !ok {
			t.Fatal("Failed to cast to simpleMessageQueue")
		}
		sq.mu.Lock()
		if msg, exists := sq.messages[msgIDs[0]]; exists {
			msg.Attempts = msg.MaxAttempts
			msg.SetError(fmt.Errorf("permanent failure"))
		}
		sq.mu.Unlock()
		
		err := q.UpdateState(msgIDs[0], MessageStateFailed, "permanently failed")
		if err != nil {
			t.Fatalf("Failed to update state to failed: %v", err)
		}
	}

	// Fail one (it will go to retrying state since it has attempts left)
	if len(msgIDs) > 1 {
		// Set error before failing
		sq, ok := q.(*simpleMessageQueue)
		if !ok {
			t.Fatal("Failed to cast to simpleMessageQueue")
		}
		sq.mu.Lock()
		if msg, exists := sq.messages[msgIDs[1]]; exists {
			msg.SetError(fmt.Errorf("temporary failure"))
		}
		sq.mu.Unlock()
		
		err := q.UpdateState(msgIDs[1], MessageStateFailed, "error")
		if err != nil {
			t.Fatalf("Failed to update state: %v", err)
		}
	}

	stats := q.Stats()

	if stats.TotalQueued != 3 {
		t.Errorf("Expected 3 queued messages, got %d", stats.TotalQueued)
	}

	if stats.TotalProcessing != 0 {
		t.Errorf("Expected 0 processing messages, got %d", stats.TotalProcessing)
	}

	if stats.TotalCompleted != 0 {
		t.Errorf("Expected 0 completed messages, got %d", stats.TotalCompleted)
	}

	// First message is permanently failed, second is in retrying state
	if stats.TotalFailed != 1 {
		t.Errorf("Expected 1 failed message, got %d", stats.TotalFailed)
	}

	if stats.ConversationCount != 2 {
		t.Errorf("Expected 2 conversations, got %d", stats.ConversationCount)
	}

	if stats.OldestMessageAge < 0 {
		t.Error("OldestMessageAge should be positive")
	}
}

func TestMessageQueue_FIFO(t *testing.T) {
	q := NewMessageQueue()

	// Enqueue messages with slight delays
	var enqueuedMsgs []signal.IncomingMessage
	for i := 0; i < 3; i++ {
		msg := signal.IncomingMessage{
			From:       "John Doe",
			FromNumber: "+1234567890",
			Text:       fmt.Sprintf("Message %d", i+1),
			Timestamp:  time.Now(),
		}
		enqueuedMsgs = append(enqueuedMsgs, msg)
		err := q.Enqueue(msg)
		if err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Get messages and verify FIFO order
	for i := 0; i < 3; i++ {
		queuedMsg, err := q.GetNext(fmt.Sprintf("worker-%d", i))
		if err != nil {
			t.Fatalf("Failed to get message: %v", err)
		}
		if queuedMsg == nil {
			t.Fatal("Expected to get a message, got nil")
		}
		if queuedMsg.Text != enqueuedMsgs[i].Text {
			t.Errorf("Expected message text '%s' at position %d, got '%s'", enqueuedMsgs[i].Text, i, queuedMsg.Text)
		}
	}
}