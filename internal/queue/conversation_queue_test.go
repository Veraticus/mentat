package queue

import (
	"fmt"
	"sync"
	"testing"
)

func TestConversationQueue_EnqueueDequeue(t *testing.T) {
	cq := NewConversationQueue("conv-1")
	
	// Test enqueue
	msg1 := NewMessage("msg-1", "conv-1", "sender", "hello")
	msg2 := NewMessage("msg-2", "conv-1", "sender", "world")
	
	if err := cq.Enqueue(msg1); err != nil {
		t.Fatalf("Failed to enqueue msg1: %v", err)
	}
	
	if err := cq.Enqueue(msg2); err != nil {
		t.Fatalf("Failed to enqueue msg2: %v", err)
	}
	
	if cq.Size() != 2 {
		t.Errorf("Expected size 2, got %d", cq.Size())
	}
	
	// Test dequeue - should get messages in FIFO order
	dequeued1 := cq.Dequeue()
	if dequeued1 == nil || dequeued1.ID != "msg-1" {
		t.Errorf("Expected msg-1, got %v", dequeued1)
	}
	
	// Should not dequeue while processing
	dequeued2 := cq.Dequeue()
	if dequeued2 != nil {
		t.Error("Should not dequeue while processing")
	}
	
	// Complete and dequeue next
	cq.Complete()
	dequeued2 = cq.Dequeue()
	if dequeued2 == nil || dequeued2.ID != "msg-2" {
		t.Errorf("Expected msg-2, got %v", dequeued2)
	}
	
	if cq.Size() != 0 {
		t.Errorf("Expected size 0, got %d", cq.Size())
	}
}

func TestConversationQueue_EnqueueErrors(t *testing.T) {
	cq := NewConversationQueue("conv-1")
	
	// Test nil message
	if err := cq.Enqueue(nil); err == nil {
		t.Error("Expected error for nil message")
	}
	
	// Test wrong conversation ID
	wrongMsg := NewMessage("msg-1", "conv-2", "sender", "wrong")
	if err := cq.Enqueue(wrongMsg); err == nil {
		t.Error("Expected error for wrong conversation ID")
	}
}

func TestConversationQueue_EmptyDequeue(t *testing.T) {
	cq := NewConversationQueue("conv-1")
	
	msg := cq.Dequeue()
	if msg != nil {
		t.Error("Expected nil from empty queue")
	}
}

func TestConversationQueue_IsProcessing(t *testing.T) {
	cq := NewConversationQueue("conv-1")
	msg := NewMessage("msg-1", "conv-1", "sender", "test")
	
	if err := cq.Enqueue(msg); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	
	if cq.IsProcessing() {
		t.Error("Should not be processing before dequeue")
	}
	
	cq.Dequeue()
	
	if !cq.IsProcessing() {
		t.Error("Should be processing after dequeue")
	}
	
	cq.Complete()
	
	if cq.IsProcessing() {
		t.Error("Should not be processing after complete")
	}
}

func TestConversationQueue_IsEmpty(t *testing.T) {
	cq := NewConversationQueue("conv-1")
	
	if !cq.IsEmpty() {
		t.Error("New queue should be empty")
	}
	
	msg := NewMessage("msg-1", "conv-1", "sender", "test")
	if err := cq.Enqueue(msg); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	
	if cq.IsEmpty() {
		t.Error("Queue with message should not be empty")
	}
	
	cq.Dequeue()
	
	if cq.IsEmpty() {
		t.Error("Queue with processing message should not be empty")
	}
	
	cq.Complete()
	
	if !cq.IsEmpty() {
		t.Error("Queue should be empty after processing complete")
	}
}

func TestConversationQueue_ConcurrentOperations(t *testing.T) {
	cq := NewConversationQueue("conv-1")
	var wg sync.WaitGroup
	
	// Concurrent enqueuers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := NewMessage(fmt.Sprintf("msg-%d", id), "conv-1", "sender", "test")
			if err := cq.Enqueue(msg); err != nil {
				t.Errorf("Enqueue failed: %v", err)
			}
		}(i)
	}
	
	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cq.Size()
			_ = cq.IsProcessing()
			_ = cq.IsEmpty()
		}()
	}
	
	wg.Wait()
	
	// Verify all messages were enqueued
	if cq.Size() != 10 {
		t.Errorf("Expected 10 messages, got %d", cq.Size())
	}
}

func TestConversationQueue_DepthLimit(t *testing.T) {
	// Test with custom depth limit
	maxDepth := 3
	cq := NewConversationQueueWithDepth("conv-1", maxDepth)
	
	if cq.MaxDepth() != maxDepth {
		t.Errorf("Expected max depth %d, got %d", maxDepth, cq.MaxDepth())
	}
	
	// Fill queue to limit
	for i := 0; i < maxDepth; i++ {
		msg := NewMessage(fmt.Sprintf("msg-%d", i), "conv-1", "sender", "test")
		if err := cq.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message %d: %v", i, err)
		}
	}
	
	// Verify queue is at capacity
	if cq.Size() != maxDepth {
		t.Errorf("Expected size %d, got %d", maxDepth, cq.Size())
	}
	
	// Try to exceed limit
	overflowMsg := NewMessage("overflow", "conv-1", "sender", "test")
	err := cq.Enqueue(overflowMsg)
	if err == nil {
		t.Error("Expected error when exceeding depth limit")
	}
	
	// Verify error message is clear
	expectedErr := fmt.Sprintf("conversation queue full: maximum depth of %d messages reached", maxDepth)
	if err.Error() != expectedErr {
		t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
	}
	
	// Verify size didn't change
	if cq.Size() != maxDepth {
		t.Errorf("Queue size changed after overflow: expected %d, got %d", maxDepth, cq.Size())
	}
}

func TestConversationQueue_DepthLimitAfterDequeue(t *testing.T) {
	maxDepth := 2
	cq := NewConversationQueueWithDepth("conv-1", maxDepth)
	
	// Fill queue
	msg1 := NewMessage("msg-1", "conv-1", "sender", "test1")
	msg2 := NewMessage("msg-2", "conv-1", "sender", "test2")
	
	if err := cq.Enqueue(msg1); err != nil {
		t.Fatalf("Failed to enqueue msg1: %v", err)
	}
	if err := cq.Enqueue(msg2); err != nil {
		t.Fatalf("Failed to enqueue msg2: %v", err)
	}
	
	// Queue should be full
	msg3 := NewMessage("msg-3", "conv-1", "sender", "test3")
	if err := cq.Enqueue(msg3); err == nil {
		t.Error("Expected error when queue is full")
	}
	
	// Dequeue one message
	dequeued := cq.Dequeue()
	if dequeued == nil || dequeued.ID != "msg-1" {
		t.Errorf("Expected to dequeue msg-1, got %v", dequeued)
	}
	
	// Complete processing
	cq.Complete()
	
	// Now we should be able to enqueue again
	if err := cq.Enqueue(msg3); err != nil {
		t.Errorf("Failed to enqueue after dequeue: %v", err)
	}
	
	// Verify queue size
	if cq.Size() != maxDepth {
		t.Errorf("Expected size %d, got %d", maxDepth, cq.Size())
	}
}

func TestConversationQueue_DefaultDepthLimit(t *testing.T) {
	cq := NewConversationQueue("conv-1")
	
	if cq.MaxDepth() != DefaultMaxDepth {
		t.Errorf("Expected default max depth %d, got %d", DefaultMaxDepth, cq.MaxDepth())
	}
}

func TestConversationQueue_InvalidDepthLimit(t *testing.T) {
	// Test with zero depth
	cq := NewConversationQueueWithDepth("conv-1", 0)
	if cq.MaxDepth() != DefaultMaxDepth {
		t.Errorf("Expected default max depth for zero input, got %d", cq.MaxDepth())
	}
	
	// Test with negative depth
	cq = NewConversationQueueWithDepth("conv-1", -10)
	if cq.MaxDepth() != DefaultMaxDepth {
		t.Errorf("Expected default max depth for negative input, got %d", cq.MaxDepth())
	}
}