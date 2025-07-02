package queue

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// MessageQueue constants.
const (
	// messageIDByteSize is the number of random bytes for message ID generation.
	messageIDByteSize = 8
)

// SimpleMessageQueue implements the MessageQueue interface.
type SimpleMessageQueue struct {
	messages     map[string]*Message
	stateMachine StateMachine
	mu           sync.RWMutex
}

// NewMessageQueue creates a new message queue implementation.
func NewMessageQueue() *SimpleMessageQueue {
	return &SimpleMessageQueue{
		messages:     make(map[string]*Message),
		stateMachine: NewStateMachine(),
	}
}

// Enqueue adds a message to the queue.
func (q *SimpleMessageQueue) Enqueue(msg signal.IncomingMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Generate a unique message ID
	msgID, err := generateMessageID()
	if err != nil {
		return fmt.Errorf("failed to generate message ID: %w", err)
	}

	// Use the sender's phone number as the conversation ID
	// In a real system, this might be more sophisticated
	conversationID := msg.FromNumber
	if conversationID == "" {
		// Fallback to From if FromNumber is not provided
		conversationID = msg.From
	}

	// Create a new Message from the IncomingMessage
	queuedMsg := NewMessage(msgID, conversationID, msg.From, msg.FromNumber, msg.Text)

	// Store the message
	q.messages[msgID] = queuedMsg

	return nil
}

// GetNext returns the next message for a worker to process.
func (q *SimpleMessageQueue) GetNext(_ string) (*Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Find the oldest queued message
	var oldestMsg *Message
	for _, msg := range q.messages {
		if msg.GetState() == StateQueued {
			if oldestMsg == nil || msg.CreatedAt.Before(oldestMsg.CreatedAt) {
				oldestMsg = msg
			}
		}
	}

	if oldestMsg == nil {
		return nil, nil //nolint:nilnil // nil is valid when no messages available
	}

	// Transition to processing
	if err := q.stateMachine.Transition(oldestMsg, StateProcessing); err != nil {
		return nil, fmt.Errorf("failed to transition message to processing: %w", err)
	}

	return oldestMsg, nil
}

// UpdateState marks a message state transition.
func (q *SimpleMessageQueue) UpdateState(msgID string, state State, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg, exists := q.messages[msgID]
	if !exists {
		return fmt.Errorf("message %s not found", msgID)
	}

	// Use the state machine to transition
	if err := q.stateMachine.Transition(msg, state); err != nil {
		return fmt.Errorf("failed to transition message: %w", err)
	}

	// Clean up completed/failed messages after some time
	// In a real implementation, we might schedule cleanup
	// For now, we'll keep them for stats

	return nil
}

// generateMessageID creates a unique message ID.
func generateMessageID() (string, error) {
	bytes := make([]byte, messageIDByteSize)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// Stats returns queue statistics.
func (q *SimpleMessageQueue) Stats() Stats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := Stats{}

	var oldestQueued *time.Time
	totalWaitTime := time.Duration(0)
	totalProcessTime := time.Duration(0)
	processedCount := 0

	for _, msg := range q.messages {
		switch msg.GetState() {
		case StateQueued:
			stats.TotalQueued++
			if oldestQueued == nil || msg.CreatedAt.Before(*oldestQueued) {
				oldestQueued = &msg.CreatedAt
			}
		case StateProcessing:
			stats.TotalProcessing++
		case StateCompleted:
			stats.TotalCompleted++
			if msg.ProcessedAt != nil {
				processTime := msg.ProcessedAt.Sub(msg.CreatedAt)
				totalProcessTime += processTime
				processedCount++
			}
		case StateFailed:
			stats.TotalFailed++
		case StateRetrying:
			// Retrying messages aren't counted in any of the current stats fields
			// They're technically still "in progress" but waiting for retry
		case StateValidating:
			// Validating messages are in progress but not separately counted
		}
	}

	// Calculate conversation count
	conversations := make(map[string]bool)
	for _, msg := range q.messages {
		conversations[msg.ConversationID] = true
	}
	stats.ConversationCount = len(conversations)

	// Calculate oldest message age
	if oldestQueued != nil {
		stats.LongestMessageAge = time.Since(*oldestQueued)
	}

	// Calculate averages
	if stats.TotalQueued > 0 {
		stats.AverageWaitTime = totalWaitTime / time.Duration(stats.TotalQueued)
	}
	if processedCount > 0 {
		stats.AverageProcessTime = totalProcessTime / time.Duration(processedCount)
	}

	return stats
}
