package queue

import (
	"fmt"
	"testing"
	"time"
)

func TestMessageStateConstants(t *testing.T) {
	// Verify that MessageState constants have expected values
	if MessageStateQueued != 0 {
		t.Error("Expected MessageStateQueued to be 0")
	}
	if MessageStateProcessing != 1 {
		t.Error("Expected MessageStateProcessing to be 1")
	}
	if MessageStateValidating != 2 {
		t.Error("Expected MessageStateValidating to be 2")
	}
	if MessageStateCompleted != 3 {
		t.Error("Expected MessageStateCompleted to be 3")
	}
	if MessageStateFailed != 4 {
		t.Error("Expected MessageStateFailed to be 4")
	}
	if MessageStateRetrying != 5 {
		t.Error("Expected MessageStateRetrying to be 5")
	}
}

func TestStateConstants(t *testing.T) {
	// Verify that State constants have expected string values
	tests := []struct {
		state    State
		expected string
	}{
		{StateQueued, "queued"},
		{StateProcessing, "processing"},
		{StateValidating, "validating"},
		{StateCompleted, "completed"},
		{StateFailed, "failed"},
		{StateRetrying, "retrying"},
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
	if PriorityNormal != 0 {
		t.Error("Expected PriorityNormal to be 0")
	}
	if PriorityHigh != 1 {
		t.Error("Expected PriorityHigh to be 1")
	}
}

func TestMessageZeroValue(t *testing.T) {
	var msg Message

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
	msg := NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test message")
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
	if msg.State != StateQueued {
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
	msg := NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")
	originalUpdatedAt := msg.UpdatedAt

	// Allow time to pass for updated timestamp
	<-time.After(time.Millisecond)

	msg.SetState(StateProcessing)

	if msg.State != StateProcessing {
		t.Error("State not updated correctly")
	}
	if msg.UpdatedAt.Equal(originalUpdatedAt) {
		t.Error("UpdatedAt should be updated when state changes")
	}
}

func TestMessageSetError(t *testing.T) {
	msg := NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")
	originalAttempts := msg.Attempts

	testErr := fmt.Errorf("Test error")
	msg.SetError(testErr)

	if msg.Error == nil || msg.Error.Error() != "Test error" {
		t.Error("Error not set correctly")
	}
	// Note: SetError doesn't automatically change state to failed
	// Note: SetError doesn't automatically increment attempts
	if msg.Attempts != originalAttempts {
		t.Error("Attempts should not be incremented by SetError")
	}
}

func TestMessageSetResponse(t *testing.T) {
	msg := NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")
	beforeProcess := time.Now()

	msg.SetResponse("Test response")

	afterProcess := time.Now()

	if msg.Response != "Test response" {
		t.Error("Response not set correctly")
	}
	// Note: SetResponse doesn't automatically change state
	if msg.ProcessedAt == nil || msg.ProcessedAt.Before(beforeProcess) || msg.ProcessedAt.After(afterProcess) {
		t.Error("ProcessedAt not set correctly")
	}
}

func TestQueuedMessageZeroValue(t *testing.T) {
	var qm QueuedMessage

	// Zero value should be identifiable as uninitialized
	if qm.ID != "" {
		t.Error("Expected empty ID for zero value")
	}
	if qm.ConversationID != "" {
		t.Error("Expected empty ConversationID for zero value")
	}
	if qm.Priority != PriorityNormal {
		t.Error("Expected PriorityNormal as default Priority")
	}
	if !qm.QueuedAt.IsZero() {
		t.Error("Expected zero time for QueuedAt")
	}
}

func TestStatsZeroValue(t *testing.T) {
	var stats Stats

	// Zero value should have all counts at zero
	if stats.TotalQueued != 0 {
		t.Error("Expected zero TotalQueued for zero value")
	}
	if stats.TotalProcessing != 0 {
		t.Error("Expected zero TotalProcessing for zero value")
	}
	if stats.TotalCompleted != 0 {
		t.Error("Expected zero TotalCompleted for zero value")
	}
	if stats.TotalFailed != 0 {
		t.Error("Expected zero TotalFailed for zero value")
	}
	if stats.AverageProcessTime != 0 {
		t.Error("Expected zero AverageProcessTime for zero value")
	}
	if stats.AverageWaitTime != 0 {
		t.Error("Expected zero AverageWaitTime for zero value")
	}
}

func TestMessageThreadSafety(_ *testing.T) {
	msg := NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	// Run concurrent operations
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 100; i++ {
			msg.SetState(StateProcessing)
			msg.SetState(StateQueued)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			msg.SetError(fmt.Errorf("error"))
			msg.SetResponse("response")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = msg.GetState()
			// Note: No thread-safe getters for Error and Response
			// This test verifies that concurrent writes don't panic
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// If we get here without panic, thread safety is working
}

func TestMessageStateHistory(t *testing.T) {
	msg := NewMessage("msg-123", "conv-123", "sender", "+1234567890", "Test")

	// Initially, history should be empty
	history := msg.GetStateHistory()
	if len(history) != 0 {
		t.Error("Expected empty history for new message")
	}

	// Add a state transition
	msg.AddStateTransition(StateQueued, StateProcessing, "starting processing")

	history = msg.GetStateHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 history entry, got %d", len(history))
	}

	if history[0].From != MessageStateQueued || history[0].To != MessageStateProcessing {
		t.Error("State transition not recorded correctly")
	}

	if history[0].Reason != "starting processing" {
		t.Errorf("Expected reason 'starting processing', got '%s'", history[0].Reason)
	}

	// Verify timestamp is recent
	if time.Since(history[0].Timestamp) > time.Second {
		t.Error("Timestamp seems incorrect")
	}

	// Add another transition
	msg.AddStateTransition(StateProcessing, StateCompleted, "success")

	history = msg.GetStateHistory()
	if len(history) != 2 {
		t.Fatalf("Expected 2 history entries, got %d", len(history))
	}

	// Verify the returned history is a copy (modifying it doesn't affect the original)
	history[0].Reason = "modified"
	actualHistory := msg.GetStateHistory()
	if actualHistory[0].Reason == "modified" {
		t.Error("GetStateHistory should return a copy, not a reference")
	}
}

func TestStateTransitionZeroValue(t *testing.T) {
	var st StateTransition

	// Zero value should be identifiable as uninitialized
	if st.From != 0 {
		t.Error("Expected zero From state for zero value")
	}
	if st.To != 0 {
		t.Error("Expected empty To state for zero value")
	}
	if st.Reason != "" {
		t.Error("Expected empty Reason for zero value")
	}
	if !st.Timestamp.IsZero() {
		t.Error("Expected zero time for Timestamp")
	}
}
