package queue_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

func TestMessageQueue_EnqueueAndGetNext(t *testing.T) {
	q := queue.NewMessageQueue()

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

	if queuedMsg.Sender != msg.From {
		t.Errorf("Expected sender %s, got %s", msg.From, queuedMsg.Sender)
	}

	if queuedMsg.State != queue.StateProcessing {
		t.Errorf("Expected state %v, got %v", queue.StateProcessing, queuedMsg.State)
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
	q := queue.NewMessageQueue()

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
	err = queue.SetMessageError(q, queuedMsg.ID, fmt.Errorf("test error"))
	if err != nil {
		t.Fatalf("Failed to set error on message: %v", err)
	}

	// Update to retrying (valid transition from processing with error)
	err = q.UpdateState(queuedMsg.ID, queue.StateFailed, "failed")
	if err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}

	// Should be in retrying state since it has attempts left
	msgState, err := queue.GetMessageState(q, queuedMsg.ID)
	if err != nil {
		t.Fatalf("Failed to get message state: %v", err)
	}
	if msgState != queue.StateRetrying {
		t.Errorf("Expected state %v, got %v", queue.StateRetrying, msgState)
	}

	// Try to update non-existent message
	err = q.UpdateState("msg-999", queue.StateFailed, "test")
	if err == nil {
		t.Error("Expected error for non-existent message")
	}
}

func TestMessageQueue_Stats(t *testing.T) {
	q := queue.NewMessageQueue()

	// Enqueue test messages
	enqueueTestMessagesForStats(t, q, 5)

	// Process some messages
	msgIDs := processTestMessages(t, q, 2)

	// Simulate failures
	simulateMessageFailures(t, q, msgIDs)

	// Verify statistics
	verifyQueueStats(t, q)
}

func enqueueTestMessagesForStats(t *testing.T, q queue.MessageQueue, count int) {
	t.Helper()
	for i := range count {
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
}

func processTestMessages(t *testing.T, q queue.MessageQueue, count int) []string {
	t.Helper()
	var msgIDs []string
	for i := range count {
		qMsg, err := q.GetNext(fmt.Sprintf("worker-%d", i))
		if err != nil {
			t.Fatalf("Failed to get message: %v", err)
		}
		if qMsg != nil {
			msgIDs = append(msgIDs, qMsg.ID)
		}
	}
	return msgIDs
}

func simulateMessageFailures(t *testing.T, q queue.MessageQueue, msgIDs []string) {
	t.Helper()
	if len(msgIDs) > 0 {
		simulatePermanentFailure(t, q, msgIDs[0])
	}
	if len(msgIDs) > 1 {
		simulateTemporaryFailure(t, q, msgIDs[1])
	}
}

func simulatePermanentFailure(t *testing.T, q queue.MessageQueue, msgID string) {
	t.Helper()
	err := queue.SetMessageMaxAttempts(q, msgID)
	if err != nil {
		t.Fatalf("Failed to set max attempts: %v", err)
	}
	err = queue.SetMessageError(q, msgID, fmt.Errorf("permanent failure"))
	if err != nil {
		t.Fatalf("Failed to set error: %v", err)
	}
	err = q.UpdateState(msgID, queue.StateFailed, "permanently failed")
	if err != nil {
		t.Fatalf("Failed to update state to failed: %v", err)
	}
}

func simulateTemporaryFailure(t *testing.T, q queue.MessageQueue, msgID string) {
	t.Helper()
	err := queue.SetMessageError(q, msgID, fmt.Errorf("temporary failure"))
	if err != nil {
		t.Fatalf("Failed to set error: %v", err)
	}
	err = q.UpdateState(msgID, queue.StateFailed, "error")
	if err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}
}

func verifyQueueStats(t *testing.T, q queue.MessageQueue) {
	t.Helper()
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
	if stats.TotalFailed != 1 {
		t.Errorf("Expected 1 failed message, got %d", stats.TotalFailed)
	}
	if stats.ConversationCount != 2 {
		t.Errorf("Expected 2 conversations, got %d", stats.ConversationCount)
	}
	if stats.LongestMessageAge < 0 {
		t.Error("LongestMessageAge should be positive")
	}
}

func TestMessageQueue_FIFO(t *testing.T) {
	q := queue.NewMessageQueue()

	// Enqueue messages with slight delays
	enqueuedMsgs := make([]signal.IncomingMessage, 0, 3)
	for i := range 3 {
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
		// Timestamps from loop iteration are sufficient
	}

	// Get messages and verify FIFO order
	for i := range 3 {
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
