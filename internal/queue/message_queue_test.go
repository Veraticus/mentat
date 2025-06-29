package queue

import (
	"fmt"
	"testing"
	"time"

	"github.com/joshsymonds/mentat/internal/signal"
)

func TestMessageQueue_EnqueueAndGetNext(t *testing.T) {
	q := NewMessageQueue()

	// Enqueue a message
	msg := signal.IncomingMessage{
		From:      "+1234567890",
		Text:      "Test message",
		Timestamp: time.Now(),
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
		From:      "+1234567890",
		Text:      "Test message",
		Timestamp: time.Now(),
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

	// Update to validating
	err = q.UpdateState(queuedMsg.ID, MessageStateValidating, "starting validation")
	if err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}

	// Update to completed
	err = q.UpdateState(queuedMsg.ID, MessageStateCompleted, "success")
	if err != nil {
		t.Fatalf("Failed to update state: %v", err)
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
			From:      fmt.Sprintf("+123456789%d", i%2), // 2 conversations
			Text:      "Test message",
			Timestamp: time.Now(),
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

	// Complete one (first transition to validating, then to completed)
	if len(msgIDs) > 0 {
		err := q.UpdateState(msgIDs[0], MessageStateValidating, "validating")
		if err != nil {
			t.Fatalf("Failed to update state to validating: %v", err)
		}
		err = q.UpdateState(msgIDs[0], MessageStateCompleted, "done")
		if err != nil {
			t.Fatalf("Failed to update state to completed: %v", err)
		}
	}

	// Fail one (it will go to retrying state since it has attempts left)
	if len(msgIDs) > 1 {
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

	if stats.TotalCompleted != 1 {
		t.Errorf("Expected 1 completed message, got %d", stats.TotalCompleted)
	}

	// The failed message will actually be in retrying state because it has attempts left
	// So we shouldn't expect any failed messages
	if stats.TotalFailed != 0 {
		t.Errorf("Expected 0 failed messages, got %d", stats.TotalFailed)
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
			From:      "+1234567890",
			Text:      fmt.Sprintf("Message %d", i+1),
			Timestamp: time.Now(),
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