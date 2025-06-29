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
	mu             sync.Mutex
}

// NewConversationQueue creates a new queue for a conversation.
func NewConversationQueue(conversationID string) *ConversationQueue {
	return &ConversationQueue{
		conversationID: conversationID,
		messages:       list.New(),
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

	cq.messages.PushBack(msg)
	return nil
}

// Dequeue removes and returns the next message to process.
// Returns nil if the queue is empty or a message is already being processed.
func (cq *ConversationQueue) Dequeue() *Message {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	// Can't dequeue if already processing
	if cq.processing != nil {
		return nil
	}

	// Get the front message
	front := cq.messages.Front()
	if front == nil {
		return nil
	}

	msg, ok := front.Value.(*Message)
	if !ok {
		return nil
	}
	cq.messages.Remove(front)
	cq.processing = msg

	return msg
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
