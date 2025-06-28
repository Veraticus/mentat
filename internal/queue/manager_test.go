package queue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestManager_SubmitAndRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	manager := NewManager(ctx)
	go manager.Start()
	
	// Submit messages
	msg1 := NewMessage("msg-1", "conv-1", "sender1", "hello")
	msg2 := NewMessage("msg-2", "conv-2", "sender2", "world")
	
	if err := manager.Submit(msg1); err != nil {
		t.Fatalf("Failed to submit msg1: %v", err)
	}
	
	if err := manager.Submit(msg2); err != nil {
		t.Fatalf("Failed to submit msg2: %v", err)
	}
	
	// Request messages
	reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
	defer reqCancel()
	
	received1, err := manager.RequestMessage(reqCtx)
	if err != nil {
		t.Fatalf("Failed to request message: %v", err)
	}
	
	if received1 == nil {
		t.Fatal("Expected to receive a message")
	}
	
	// Should be in processing state
	if received1.GetState() != StateProcessing {
		t.Errorf("Expected state %s, got %s", StateProcessing, received1.GetState())
	}
	
	// Complete and request next
	manager.CompleteMessage(received1)
	
	received2, err := manager.RequestMessage(reqCtx)
	if err != nil {
		t.Fatalf("Failed to request second message: %v", err)
	}
	
	if received2 == nil {
		t.Fatal("Expected to receive second message")
	}
	
	// Should get messages from different conversations (fair scheduling)
	if received1.ConversationID == received2.ConversationID {
		t.Error("Expected messages from different conversations for fairness")
	}
}

func TestManager_FairScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	manager := NewManager(ctx)
	go manager.Start()
	
	// Give manager time to start
	time.Sleep(10 * time.Millisecond)
	
	// Submit multiple messages per conversation
	conversations := []string{"conv-1", "conv-2", "conv-3"}
	messagesPerConv := 3
	
	for _, convID := range conversations {
		for i := 0; i < messagesPerConv; i++ {
			msg := NewMessage(
				fmt.Sprintf("%s-msg-%d", convID, i),
				convID,
				"sender",
				"test",
			)
			if err := manager.Submit(msg); err != nil {
				t.Fatalf("Failed to submit message: %v", err)
			}
		}
	}
	
	// Give time for messages to be queued
	time.Sleep(20 * time.Millisecond)
	
	// Request messages and track which conversations we get
	convCounts := make(map[string]int)
	reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
	defer reqCancel()
	
	// First round - should get one from each conversation
	for i := 0; i < len(conversations); i++ {
		msg, err := manager.RequestMessage(reqCtx)
		if err != nil || msg == nil {
			t.Fatalf("Failed to get message %d: %v", i, err)
		}
		convCounts[msg.ConversationID]++
		manager.CompleteMessage(msg)
		
		// Small delay to ensure proper ordering
		time.Sleep(5 * time.Millisecond)
	}
	
	// Check fairness - each conversation should have been scheduled once
	for _, convID := range conversations {
		if convCounts[convID] != 1 {
			t.Errorf("Conversation %s scheduled %d times, expected 1", 
				convID, convCounts[convID])
		}
	}
}

func TestManager_Shutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	manager := NewManager(ctx)
	go manager.Start()
	
	// Submit a message
	msg := NewMessage("msg-1", "conv-1", "sender", "test")
	manager.Submit(msg)
	
	// Shutdown with timeout
	err := manager.Shutdown(2 * time.Second)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	
	// Should not be able to submit after shutdown
	err = manager.Submit(NewMessage("msg-2", "conv-1", "sender", "test"))
	if err == nil {
		t.Error("Expected error submitting after shutdown")
	}
}

func TestManager_Stats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	manager := NewManager(ctx)
	go manager.Start()
	
	// Initial stats
	stats := manager.Stats()
	if stats["conversations"] != 0 || stats["queued"] != 0 || stats["processing"] != 0 {
		t.Error("Expected empty initial stats")
	}
	
	// Submit messages
	manager.Submit(NewMessage("msg-1", "conv-1", "sender", "test"))
	manager.Submit(NewMessage("msg-2", "conv-1", "sender", "test"))
	manager.Submit(NewMessage("msg-3", "conv-2", "sender", "test"))
	
	// Give time for messages to be queued
	time.Sleep(10 * time.Millisecond)
	
	stats = manager.Stats()
	if stats["conversations"] != 2 {
		t.Errorf("Expected 2 conversations, got %d", stats["conversations"])
	}
	if stats["queued"] != 3 {
		t.Errorf("Expected 3 queued messages, got %d", stats["queued"])
	}
	
	// Request a message
	reqCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	msg, _ := manager.RequestMessage(reqCtx)
	
	if msg != nil {
		stats = manager.Stats()
		if stats["processing"] != 1 {
			t.Errorf("Expected 1 processing, got %d", stats["processing"])
		}
		if stats["queued"] != 2 {
			t.Errorf("Expected 2 queued after processing one, got %d", stats["queued"])
		}
	}
}

func TestManager_ConcurrentSubmit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	manager := NewManager(ctx)
	go manager.Start()
	
	var wg sync.WaitGroup
	numGoroutines := 10
	messagesPerGoroutine := 5
	
	// Concurrent submits
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := NewMessage(
					fmt.Sprintf("g%d-m%d", goroutineID, j),
					fmt.Sprintf("conv-%d", goroutineID%3), // 3 conversations
					"sender",
					"test",
				)
				if err := manager.Submit(msg); err != nil {
					t.Errorf("Submit failed: %v", err)
				}
			}
		}(i)
	}
	
	wg.Wait()
	
	// Give time for all messages to be queued
	time.Sleep(50 * time.Millisecond)
	
	stats := manager.Stats()
	expectedMessages := numGoroutines * messagesPerGoroutine
	if stats["queued"] != expectedMessages {
		t.Errorf("Expected %d queued messages, got %d", expectedMessages, stats["queued"])
	}
}

func TestManager_CompleteNonExistentMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	manager := NewManager(ctx)
	go manager.Start()
	
	// Try to complete a message that was never submitted
	msg := NewMessage("fake", "fake-conv", "sender", "test")
	err := manager.CompleteMessage(msg)
	if err == nil {
		t.Error("Expected error completing non-existent message")
	}
}

func TestManager_RequestTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	manager := NewManager(ctx)
	go manager.Start()
	
	// Give manager time to start
	time.Sleep(10 * time.Millisecond)
	
	// Request with short timeout when no messages available
	reqCtx, reqCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer reqCancel()
	
	start := time.Now()
	msg, err := manager.RequestMessage(reqCtx)
	duration := time.Since(start)
	
	if msg != nil {
		t.Error("Expected nil message on timeout")
	}
	
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded error, got %v", err)
	}
	
	// Should timeout after ~100ms
	if duration < 90*time.Millisecond || duration > 200*time.Millisecond {
		t.Errorf("Unexpected timeout duration: %v", duration)
	}
}