package queue

import (
	"container/list"
	"fmt"
	"sync"
)

// ConversationQueue manages messages for a single conversation.
// It ensures FIFO ordering and thread-safe operations.
type ConversationQueue struct {
	messages       *list.List
	processing     *Message
	conversationID string
	maxDepth       int
	mu             sync.Mutex
}

// DefaultMaxDepth is the default maximum queue depth per conversation.
const DefaultMaxDepth = 100

// NewConversationQueue creates a new queue for a conversation with default depth limit.
func NewConversationQueue(conversationID string) *ConversationQueue {
	return NewConversationQueueWithDepth(conversationID, DefaultMaxDepth)
}

// NewConversationQueueWithDepth creates a new queue with a specific depth limit.
func NewConversationQueueWithDepth(conversationID string, maxDepth int) *ConversationQueue {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	return &ConversationQueue{
		conversationID: conversationID,
		messages:       &list.List{},
		maxDepth:       maxDepth,
	}
}

// Enqueue adds a message to the queue.
func (cq *ConversationQueue) Enqueue(msg *Message) error {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if msg == nil {
		return fmt.Errorf("cannot enqueue nil message")
	}

	if msg.ConversationID != cq.conversationID {
		return fmt.Errorf("message conversation ID %s does not match queue ID %s",
			msg.ConversationID, cq.conversationID)
	}

	// Check depth limit
	if cq.messages.Len() >= cq.maxDepth {
		return fmt.Errorf("conversation queue full: maximum depth of %d messages reached", cq.maxDepth)
	}

	cq.messages.PushBack(msg)
	return nil
}

// Dequeue removes and returns the next message to process.
// Returns nil if the queue is empty, a message is already being processed,
// or the next message is not ready for retry yet.
func (cq *ConversationQueue) Dequeue() *Message {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	// Can't dequeue if already processing
	if cq.processing != nil {
		return nil
	}

	// Find the first message that's ready to process
	for elem := cq.messages.Front(); elem != nil; elem = elem.Next() {
		msg, ok := elem.Value.(*Message)
		if !ok {
			continue
		}

		// Check if message is ready for retry
		if !msg.IsReadyForRetry() {
			continue
		}

		// Found a message ready to process
		cq.messages.Remove(elem)
		cq.processing = msg
		return msg
	}

	return nil
}

// Complete marks the current processing message as done.
func (cq *ConversationQueue) Complete() {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	cq.processing = nil
}

// Size returns the number of messages waiting in the queue.
func (cq *ConversationQueue) Size() int {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	return cq.messages.Len()
}

// IsProcessing returns true if a message is currently being processed.
func (cq *ConversationQueue) IsProcessing() bool {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	return cq.processing != nil
}

// IsEmpty returns true if the queue has no messages and nothing is processing.
func (cq *ConversationQueue) IsEmpty() bool {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	return cq.messages.Len() == 0 && cq.processing == nil
}

// HasReadyMessages returns true if the queue has messages ready to be processed.
// This considers the NextRetryAt field for messages in retry state.
func (cq *ConversationQueue) HasReadyMessages() bool {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	// Can't process if already processing
	if cq.processing != nil {
		return false
	}

	// Check if any message is ready
	for elem := cq.messages.Front(); elem != nil; elem = elem.Next() {
		msg, ok := elem.Value.(*Message)
		if !ok {
			continue
		}
		if msg.IsReadyForRetry() {
			return true
		}
	}

	return false
}

// MaxDepth returns the maximum depth limit for this queue.
func (cq *ConversationQueue) MaxDepth() int {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	return cq.maxDepth
}
