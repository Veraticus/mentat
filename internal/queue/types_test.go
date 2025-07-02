package queue_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
)

func TestMessageStateConstants(t *testing.T) {
	// Verify that MessageState constants have expected values
	if queue.MessageStateQueued != 0 {
		t.Error("Expected MessageStateQueued to be 0")
	}
	if queue.MessageStateProcessing != 1 {
		t.Error("Expected MessageStateProcessing to be 1")
	}
	if queue.MessageStateValidating != 2 {
		t.Error("Expected MessageStateValidating to be 2")
	}
	if queue.MessageStateCompleted != 3 {
		t.Error("Expected MessageStateCompleted to be 3")
	}
	if queue.MessageStateFailed != 4 {
		t.Error("Expected MessageStateFailed to be 4")
	}
	if queue.MessageStateRetrying != 5 {
		t.Error("Expected MessageStateRetrying to be 5")
	}
}

func TestStateConstants(t *testing.T) {
	// Verify that State constants have expected string values
	tests := []struct {
		state    queue.State
		expected string
	}{
		{queue.StateQueued, "queued"},
		{queue.StateProcessing, "processing"},
		{queue.StateValidating, "validating"},
		{queue.StateCompleted, "completed"},
		{queue.StateFailed, "failed"},
		{queue.StateRetrying, "retrying"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("State value = %v, want %v", string(tt.state), tt.expected)
			}
		})
	}
}

func TestPriorityConstants(t *testing.T) {
	// Verify that priority constants have expected values
	if queue.PriorityNormal != 0 {
		t.Error("Expected PriorityNormal to be 0")
	}
	if queue.PriorityHigh != 1 {
		t.Error("Expected PriorityHigh to be 1")
	}
}

func TestMessageZeroValue(t *testing.T) {
	var msg queue.Message

	// Zero value should be identifiable as uninitialized
	if msg.ID != "" {
		t.Error("Expected empty ID for zero value")
	}
	if msg.ConversationID != "" {
		t.Error("Expected empty ConversationID for zero value")
	}
	if msg.Sender != "" {
		t.Error("Expected empty Sender for zero value")
	}
	if msg.Text != "" {
		t.Error("Expected empty Text for zero value")
	}
	if msg.Response != "" {
		t.Error("Expected empty Response for zero value")
	}
	if msg.State != "" {
		t.Error("Expected empty string as default State")
	}
	if msg.Attempts != 0 {
		t.Error("Expected zero Attempts for zero value")
	}
	if msg.Error != nil {
		t.Error("Expected nil Error for zero value")
	}
	if !msg.CreatedAt.IsZero() {
		t.Error("Expected zero time for CreatedAt")
	}
	if !msg.UpdatedAt.IsZero() {
		t.Error("Expected zero time for UpdatedAt")
	}
	if msg.ProcessedAt != nil {
		t.Error("Expected nil ProcessedAt for zero value")
	}
}

func TestNewMessage(t *testing.T) {
	beforeCreate := time.Now()
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test message")
	afterCreate := time.Now()

	// Verify ID is generated
	if msg.ID == "" {
		t.Error("Expected ID to be generated")
	}

	// Verify fields are set correctly
	if msg.ConversationID != "conv-123" {
		t.Error("ConversationID not set correctly")
	}
	if msg.Sender != "sender" {
		t.Error("Sender not set correctly")
	}
	if msg.SenderNumber != "+1234567890" {
		t.Error("SenderNumber not set correctly")
	}
	if msg.Text != "Test message" {
		t.Error("Text not set correctly")
	}

	// Verify default state
	if msg.State != queue.StateQueued {
		t.Error("Expected initial state to be StateQueued")
	}
	if msg.Attempts != 0 {
		t.Error("Expected initial attempts to be 0")
	}

	// Verify timestamps
	if msg.CreatedAt.Before(beforeCreate) || msg.CreatedAt.After(afterCreate) {
		t.Error("CreatedAt not set correctly")
	}
	if msg.UpdatedAt != msg.CreatedAt {
		t.Error("UpdatedAt should equal CreatedAt on creation")
	}
	if msg.ProcessedAt != nil {
		t.Error("ProcessedAt should be nil on creation")
	}
}

func TestMessageSetState(t *testing.T) {
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	beforeUpdate := msg.UpdatedAt
	// Use a channel with timeout to ensure time difference
	done := make(chan struct{})
	go func() {
		time.After(time.Millisecond)
		close(done)
	}()
	<-done

	msg.SetState(queue.StateProcessing)

	if msg.State != queue.StateProcessing {
		t.Error("State not updated correctly")
	}
	if !msg.UpdatedAt.After(beforeUpdate) {
		t.Error("UpdatedAt should be updated when state changes")
	}
}

func TestMessageSetError(t *testing.T) {
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	testErr := fmt.Errorf("test error")
	msg.SetError(testErr)

	if !errors.Is(msg.Error, testErr) {
		t.Error("Error not set correctly")
	}
	// Note: SetError doesn't automatically change state to failed
	// Note: SetError doesn't automatically increment attempts
	if msg.Attempts != 0 {
		t.Error("Attempts should not be incremented by SetError")
	}
}

func TestMessageSetResponse(t *testing.T) {
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	msg.SetResponse("Test response")

	if msg.Response != "Test response" {
		t.Error("Response not set correctly")
	}
	if msg.ProcessedAt == nil {
		t.Error("ProcessedAt should be set when response is set")
	}
	// Note: SetResponse doesn't automatically change state
}

func TestMessageIncrementAttempts(t *testing.T) {
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	// Test initial increment
	count := msg.IncrementAttempts()
	if count != 1 {
		t.Errorf("Expected 1 attempt, got %d", count)
	}
	if msg.Attempts != 1 {
		t.Error("Attempts not incremented correctly")
	}

	// Test multiple increments
	for i := 2; i <= 5; i++ {
		count = msg.IncrementAttempts()
		if count != i {
			t.Errorf("Expected %d attempts, got %d", i, count)
		}
	}
}

func TestMessageCanRetry(t *testing.T) {
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	// Test with default max attempts (3)
	msg.MaxAttempts = 3

	tests := []struct {
		attempts int
		expected bool
	}{
		{0, true},
		{1, true},
		{2, true},
		{3, false},
		{4, false},
	}

	for _, tt := range tests {
		msg.Attempts = tt.attempts
		if msg.CanRetry() != tt.expected {
			t.Errorf("Attempts=%d: expected CanRetry=%v", tt.attempts, tt.expected)
		}
	}
}

func TestMessageConcurrency(t *testing.T) {
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	// Test concurrent operations
	done := make(chan bool)
	operations := 100

	go func() {
		for range operations {
			msg.SetState(queue.StateProcessing)
			msg.SetState(queue.StateQueued)
		}
		done <- true
	}()

	go func() {
		for range operations {
			msg.SetError(fmt.Errorf("error"))
			msg.SetResponse("response")
		}
		done <- true
	}()

	go func() {
		for range operations {
			_ = msg.GetState()
			_ = msg.IncrementAttempts()
		}
		done <- true
	}()

	// Wait for all goroutines
	for range 3 {
		<-done
	}

	// Verify data integrity (basic check)
	if msg.Attempts < operations {
		t.Error("Concurrent increments may have been lost")
	}
}

func TestMessageStateTransitions(t *testing.T) {
	msg := queue.NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	// Test initial state history
	history := msg.GetStateHistory()
	if len(history) != 0 {
		t.Error("Initial state history should be empty")
	}

	// Add first transition
	msg.AddStateTransition(queue.StateQueued, queue.StateProcessing, queue.StartingProcessingReason)

	history = msg.GetStateHistory()
	if len(history) != 1 {
		t.Fatal("Expected 1 state transition")
	}

	first := history[0]
	if first.From != queue.MessageStateQueued {
		t.Error("Incorrect From state")
	}
	if first.To != queue.MessageStateProcessing {
		t.Error("Incorrect To state")
	}
	if first.Reason != queue.StartingProcessingReason {
		t.Error("Incorrect transition reason")
	}
	if first.Error != nil {
		t.Error("Expected no error in transition")
	}

	// Add another transition
	msg.AddStateTransition(queue.StateProcessing, queue.StateCompleted, "success")

	history = msg.GetStateHistory()
	if len(history) != 2 {
		t.Error("Expected 2 state transitions")
	}

	// Verify history is a copy
	history[0].Reason = "modified"
	actualHistory := msg.GetStateHistory()
	if actualHistory[0].Reason == "modified" {
		t.Error("GetStateHistory should return a copy, not a reference")
	}
}

func TestStatsZeroValue(t *testing.T) {
	var stats queue.Stats

	// All counters should be zero
	if stats.TotalQueued != 0 {
		t.Error("TotalQueued should be 0")
	}
	if stats.TotalProcessing != 0 {
		t.Error("TotalProcessing should be 0")
	}
	if stats.TotalCompleted != 0 {
		t.Error("TotalCompleted should be 0")
	}
	if stats.TotalFailed != 0 {
		t.Error("TotalFailed should be 0")
	}
	if stats.ConversationCount != 0 {
		t.Error("ConversationCount should be 0")
	}
	if stats.ActiveWorkers != 0 {
		t.Error("ActiveWorkers should be 0")
	}
	if stats.HealthyWorkers != 0 {
		t.Error("HealthyWorkers should be 0")
	}

	// Durations should be zero
	if stats.LongestMessageAge != 0 {
		t.Error("LongestMessageAge should be 0")
	}
	if stats.AverageWaitTime != 0 {
		t.Error("AverageWaitTime should be 0")
	}
	if stats.AverageProcessTime != 0 {
		t.Error("AverageProcessTime should be 0")
	}
}
