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