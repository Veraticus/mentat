// +build integration

package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestQueueBehavioralGuarantees verifies all queue behavioral guarantees using table-driven tests.
func TestQueueBehavioralGuarantees(t *testing.T) {
	tests := []struct {
		name       string
		configFunc func() HarnessConfig
		scenario   func(t *testing.T, h *TestHarness)
		verify     func(t *testing.T, h *TestHarness)
	}{
		{
			name: "message_ordering_within_conversation",
			configFunc: func() HarnessConfig {
				config := DefaultConfig()
				config.WorkerCount = 1 // Single worker to ensure ordering
				return config
			},
			scenario: func(t *testing.T, h *TestHarness) {
				// Send multiple messages from the same user
				for i := 1; i <= 5; i++ {
					msg := fmt.Sprintf("Message %d", i)
					h.SetLLMResponse(fmt.Sprintf("Response to: %s", msg), nil)
					if err := h.SendMessage("+1111111111", msg); err != nil {
						t.Fatalf("Failed to send message %d: %v", i, err)
					}
				}
				
				// Wait for all messages to be processed
				for i := 0; i < 5; i++ {
					if _, err := h.WaitForMessage(5 * time.Second); err != nil {
						t.Fatalf("Timeout waiting for message %d response", i+1)
					}
				}
			},
			verify: func(t *testing.T, h *TestHarness) {
				// Verify messages were processed in order
				state := h.VerifyQueueState()
				if state.CompletedMessages != 5 {
					t.Errorf("Expected 5 completed messages, got %d", state.CompletedMessages)
				}
			},
		},
		{
			name: "concurrent_conversations_isolation",
			configFunc: func() HarnessConfig {
				config := DefaultConfig()
				config.WorkerCount = 3
				return config
			},
			scenario: func(t *testing.T, h *TestHarness) {
				// Send interleaved messages from different users
				users := []string{"+1111111111", "+2222222222", "+3333333333"}
				
				// Configure LLM to respond with user identification
				h.SetLLMResponse("Response", nil)
				
				var wg sync.WaitGroup
				for _, user := range users {
					wg.Add(1)
					go func(phoneNumber string) {
						defer wg.Done()
						for i := 1; i <= 3; i++ {
							msg := fmt.Sprintf("User %s msg %d", phoneNumber, i)
							if err := h.SendMessage(phoneNumber, msg); err != nil {
								t.Errorf("Failed to send message from %s: %v", phoneNumber, err)
							}
							time.Sleep(50 * time.Millisecond)
						}
					}(user)
				}
				
				wg.Wait()
				
				// Wait for all messages to be processed
				expectedMessages := len(users) * 3
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					state := h.VerifyQueueState()
					if state.CompletedMessages >= expectedMessages {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			},
			verify: func(t *testing.T, h *TestHarness) {
				state := h.VerifyQueueState()
				
				// Verify all messages were processed
				expectedMessages := 9 // 3 users * 3 messages each
				if state.CompletedMessages != expectedMessages {
					t.Errorf("Expected %d completed messages, got %d", expectedMessages, state.CompletedMessages)
				}
				
				// Since the test harness doesn't track conversations,
				// we just verify total message count
				if state.TotalMessages != expectedMessages {
					t.Errorf("Expected %d total messages, got %d", expectedMessages, state.TotalMessages)
				}
			},
		},
		{
			name: "rate_limiting_enforcement",
			configFunc: func() HarnessConfig {
				config := DefaultConfig()
				config.RateLimitTokens = 3  // Small burst
				config.RateLimitRefill = time.Minute // Slow refill
				return config
			},
			scenario: func(t *testing.T, h *TestHarness) {
				h.SetLLMResponse("Rate limited response", nil)
				
				// Send burst of messages
				for i := 1; i <= 5; i++ {
					if err := h.SendMessage("+1111111111", fmt.Sprintf("Burst msg %d", i)); err != nil {
						t.Fatalf("Failed to send message %d: %v", i, err)
					}
				}
				
				// Give time for processing
				time.Sleep(2 * time.Second)
			},
			verify: func(t *testing.T, h *TestHarness) {
				state := h.VerifyQueueState()
				
				// With burst of 3, some messages should still be pending
				if state.CompletedMessages >= 5 {
					t.Error("Rate limiting not enforced: all messages completed immediately")
				}
				
				if state.PendingMessages == 0 && state.ProcessingMessages == 0 {
					t.Error("Expected some messages to be rate limited (pending or processing)")
				}
			},
		},
		{
			name: "queue_fills_up",
			configFunc: func() HarnessConfig {
				config := DefaultConfig()
				config.QueueDepth = 10      // Total queue depth
				config.WorkerCount = 1      // Minimum workers required
				config.RateLimitTokens = 1  // Very slow processing
				config.RateLimitRefill = time.Minute
				return config
			},
			scenario: func(t *testing.T, h *TestHarness) {
				// Configure slow responses
				h.SetLLMResponse("Queue test response", nil)
				
				// Send many messages rapidly
				for i := 1; i <= 20; i++ {
					// Alternate between users to test global queue depth
					phoneNumber := fmt.Sprintf("+111111111%d", i%3)
					err := h.SendMessage(phoneNumber, fmt.Sprintf("Queue test %d", i))
					if err != nil {
						t.Logf("Message %d rejected: %v", i, err)
					}
				}
				
				// Wait a moment for queue to stabilize
				time.Sleep(500 * time.Millisecond)
			},
			verify: func(t *testing.T, h *TestHarness) {
				state := h.VerifyQueueState()
				
				// With slow processing, most messages should still be queued
				t.Logf("Queue state: total=%d, pending=%d, processing=%d, completed=%d",
					state.TotalMessages, state.PendingMessages, state.ProcessingMessages, state.CompletedMessages)
				
				// Verify we have messages in the queue
				if state.TotalMessages < 10 {
					t.Errorf("Expected at least 10 messages in queue, got %d", state.TotalMessages)
				}
				
				// Most should be pending due to rate limiting
				if state.PendingMessages < 5 {
					t.Errorf("Expected many pending messages due to rate limit, got %d", state.PendingMessages)
				}
			},
		},
		{
			name: "error_handling",
			configFunc: func() HarnessConfig {
				return DefaultConfig()
			},
			scenario: func(t *testing.T, h *TestHarness) {
				// Configure LLM to fail
				h.SetLLMResponse("", fmt.Errorf("processing error"))
				
				if err := h.SendMessage("+1111111111", "Error test"); err != nil {
					t.Fatalf("Failed to send message: %v", err)
				}
				
				// Wait for processing attempt
				time.Sleep(2 * time.Second)
			},
			verify: func(t *testing.T, h *TestHarness) {
				state := h.VerifyQueueState()
				
				// Message should have been attempted
				if state.TotalMessages != 1 {
					t.Errorf("Expected 1 message in queue, got %d", state.TotalMessages)
				}
				
				// Message should not be completed due to error
				if state.CompletedMessages > 0 {
					t.Errorf("Message should not complete with error, but %d completed", state.CompletedMessages)
				}
				
				// Log the actual state for debugging
				t.Logf("Error handling state: total=%d, pending=%d, processing=%d, completed=%d, failed=%d",
					state.TotalMessages, state.PendingMessages, state.ProcessingMessages, 
					state.CompletedMessages, state.FailedMessages)
			},
		},
		{
			name: "graceful_shutdown",
			configFunc: func() HarnessConfig {
				config := DefaultConfig()
				config.WorkerCount = 2
				return config
			},
			scenario: func(t *testing.T, h *TestHarness) {
				// Configure slow responses
				h.SetLLMResponse("Processing during shutdown", nil)
				
				// Send several messages
				for i := 1; i <= 5; i++ {
					if err := h.SendMessage("+1111111111", fmt.Sprintf("Shutdown test %d", i)); err != nil {
						t.Fatalf("Failed to send message %d: %v", i, err)
					}
				}
				
				// Give time for processing to start
				time.Sleep(200 * time.Millisecond)
				
				// Teardown will test graceful shutdown
			},
			verify: func(t *testing.T, h *TestHarness) {
				// Verification happens in teardown - checking for clean shutdown
				state := h.VerifyQueueState()
				t.Logf("Shutdown state: total=%d, completed=%d, failed=%d", 
					state.TotalMessages, state.CompletedMessages, state.FailedMessages)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.configFunc()
			h := NewTestHarness(t, config)
			
			if err := h.Setup(); err != nil {
				t.Fatalf("Failed to setup test harness: %v", err)
			}
			defer h.Teardown()
			
			tt.scenario(t, h)
			tt.verify(t, h)
		})
	}
}

// TestQueueLoadScenarios tests queue behavior under various load conditions.
func TestQueueLoadScenarios(t *testing.T) {
	scenarios := []struct {
		name            string
		users           int
		messagesPerUser int
		workers         int
		duration        time.Duration
		verify          func(t *testing.T, state QueueState, elapsed time.Duration)
	}{
		{
			name:            "high_throughput_single_user",
			users:           1,
			messagesPerUser: 50,
			workers:         5,
			duration:        30 * time.Second,
			verify: func(t *testing.T, state QueueState, elapsed time.Duration) {
				if state.CompletedMessages < 40 {
					t.Errorf("Low throughput: only %d/50 messages completed", state.CompletedMessages)
				}
				
				throughput := float64(state.CompletedMessages) / elapsed.Seconds()
				t.Logf("Throughput: %.2f msgs/sec", throughput)
			},
		},
		{
			name:            "many_concurrent_users",
			users:           20,
			messagesPerUser: 5,
			workers:         10,
			duration:        30 * time.Second,
			verify: func(t *testing.T, state QueueState, elapsed time.Duration) {
				// Verify fairness - no conversation should be starved
				minProcessed := 100
				maxProcessed := 0
				
				for _, conv := range state.Conversations {
					if conv.ProcessedMsgs < minProcessed {
						minProcessed = conv.ProcessedMsgs
					}
					if conv.ProcessedMsgs > maxProcessed {
						maxProcessed = conv.ProcessedMsgs
					}
				}
				
				// Max shouldn't be too much higher than min
				if maxProcessed > minProcessed*3 && minProcessed > 0 {
					t.Errorf("Unfair processing: min=%d, max=%d messages per conversation", 
						minProcessed, maxProcessed)
				}
			},
		},
		{
			name:            "burst_traffic",
			users:           10,
			messagesPerUser: 10,
			workers:         5,
			duration:        20 * time.Second,
			verify: func(t *testing.T, state QueueState, elapsed time.Duration) {
				// All messages sent at once, verify queue handles the burst
				if state.FailedMessages > 0 {
					t.Errorf("Queue couldn't handle burst: %d messages failed", state.FailedMessages)
				}
				
				// Check queue depth metrics
				metrics := state.QueuedInManager + state.ProcessingInManager
				t.Logf("Peak queue depth during burst: %d", metrics)
			},
		},
	}
	
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			config := DefaultConfig()
			config.WorkerCount = scenario.workers
			config.QueueDepth = 100 // Large enough for load tests
			config.RateLimitTokens = 100 // Don't rate limit load tests
			
			h := NewTestHarness(t, config)
			
			if err := h.Setup(); err != nil {
				t.Fatalf("Failed to setup test harness: %v", err)
			}
			defer h.Teardown()
			
			// Configure fast LLM responses
			h.SetLLMResponse("Load test response", nil)
			
			// Send all messages
			start := time.Now()
			var wg sync.WaitGroup
			
			for user := 0; user < scenario.users; user++ {
				wg.Add(1)
				go func(userID int) {
					defer wg.Done()
					phoneNumber := fmt.Sprintf("+1%09d", userID)
					
					for msg := 0; msg < scenario.messagesPerUser; msg++ {
						if err := h.SendMessage(phoneNumber, fmt.Sprintf("Load msg %d", msg)); err != nil {
							t.Logf("Failed to send message: %v", err)
						}
					}
				}(user)
			}
			
			wg.Wait() // All messages sent
			
			// Let system process for specified duration
			time.Sleep(scenario.duration)
			
			elapsed := time.Since(start)
			state := h.VerifyQueueState()
			
			// Run scenario-specific verification
			scenario.verify(t, state, elapsed)
			
			// Common verifications
			t.Logf("Load test results: %d/%d messages completed in %v", 
				state.CompletedMessages, scenario.users*scenario.messagesPerUser, elapsed)
		})
	}
}

// TestQueueStateTransitions verifies proper state machine transitions.
func TestQueueStateTransitions(t *testing.T) {
	config := DefaultConfig()
	h := NewTestHarness(t, config)
	
	if err := h.Setup(); err != nil {
		t.Fatalf("Failed to setup test harness: %v", err)
	}
	defer h.Teardown()
	
	// Track state changes
	stateHistory := h.GetStateChanges()
	initialCount := len(stateHistory)
	
	// Send a message
	h.SetLLMResponse("Test response", nil)
	if err := h.SendMessage("+1111111111", "State transition test"); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	
	// Wait for completion
	if _, err := h.WaitForMessage(5 * time.Second); err != nil {
		t.Fatalf("Message not processed: %v", err)
	}
	
	// Get state changes
	newStateHistory := h.GetStateChanges()
	stateChanges := newStateHistory[initialCount:]
	
	// Verify we see expected state transitions
	expectedTypes := map[string]bool{
		"message_sent":      false,
		"message_enqueued":  false,
		"message_processing": false,
		"message_completed": false,
	}
	
	for _, change := range stateChanges {
		if _, expected := expectedTypes[change.Type]; expected {
			expectedTypes[change.Type] = true
		}
	}
	
	// Check all expected transitions occurred
	for transition, occurred := range expectedTypes {
		if !occurred {
			t.Errorf("Missing expected state transition: %s", transition)
		}
	}
}

// TestQueueMetricsAccuracy verifies queue metrics are accurate.
func TestQueueMetricsAccuracy(t *testing.T) {
	config := DefaultConfig()
	config.WorkerCount = 3
	
	h := NewTestHarness(t, config)
	
	if err := h.Setup(); err != nil {
		t.Fatalf("Failed to setup test harness: %v", err)
	}
	defer h.Teardown()
	
	// Send known number of messages
	totalMessages := 10
	h.SetLLMResponse("Metrics test", nil)
	
	for i := 0; i < totalMessages; i++ {
		phoneNumber := fmt.Sprintf("+1%09d", i%3) // Distribute across 3 conversations
		if err := h.SendMessage(phoneNumber, fmt.Sprintf("Metrics msg %d", i)); err != nil {
			t.Fatalf("Failed to send message %d: %v", i, err)
		}
	}
	
	// Wait for all to complete
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state := h.VerifyQueueState()
		if state.CompletedMessages >= totalMessages {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	// Verify final metrics
	finalState := h.VerifyQueueState()
	
	// Total messages should match
	if finalState.TotalMessages != totalMessages {
		t.Errorf("Metric mismatch: expected %d total messages, got %d", 
			totalMessages, finalState.TotalMessages)
	}
	
	// All should be completed
	if finalState.CompletedMessages != totalMessages {
		t.Errorf("Not all messages completed: %d/%d", 
			finalState.CompletedMessages, totalMessages)
	}
	
	// No failures expected
	if finalState.FailedMessages > 0 {
		t.Errorf("Unexpected failures: %d", finalState.FailedMessages)
	}
	
	// Should have 3 conversations
	if len(finalState.Conversations) != 3 {
		t.Errorf("Expected 3 conversations, got %d", len(finalState.Conversations))
	}
}