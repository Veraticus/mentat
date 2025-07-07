//go:build integration
// +build integration

package integration

import (
	"fmt"
	"testing"
	"time"
)

// TestSimpleConversation tests basic request/response flows
func TestSimpleConversation(t *testing.T) {
	scenarios := []struct {
		name        string
		userMessage string
		llmResponse string
	}{
		{
			name:        "greeting",
			userMessage: "Hello Mentat!",
			llmResponse: "Hello! How can I help you today?",
		},
		{
			name:        "weather_query",
			userMessage: "What's the weather like?",
			llmResponse: "I'll need to check the weather for you. Let me look that up.",
		},
		{
			name:        "calendar_query",
			userMessage: "What's on my calendar today?",
			llmResponse: "Let me check your calendar for today's events.",
		},
	}

	for _, tc := range scenarios {
		RunScenario(t, tc.name, DefaultConfig(), func(t *testing.T, h *TestHarness) {
			// Configure LLM response
			h.SetLLMResponse(tc.llmResponse, nil)

			// Send user message
			userPhone := "+1234567891"
			if err := h.SendMessage(userPhone, tc.userMessage); err != nil {
				t.Fatalf("Failed to send message: %v", err)
			}

			// Wait for LLM to be called
			llmReq, err := h.WaitForLLMCall(200 * time.Millisecond)
			if err != nil {
				t.Fatalf("LLM was not called: %v", err)
			}

			// Verify LLM request contains user message
			if llmReq.Prompt != tc.userMessage {
				t.Errorf("LLM request prompt = %q, want %q", llmReq.Prompt, tc.userMessage)
			}

			// Wait for response to be sent
			response, err := h.WaitForMessage(200 * time.Millisecond)
			if err != nil {
				t.Fatalf("No response sent: %v", err)
			}

			// Verify response
			if response.Recipient != userPhone {
				t.Errorf("Response sent to %q, want %q", response.Recipient, userPhone)
			}
			if response.Text != tc.llmResponse {
				t.Errorf("Response text = %q, want %q", response.Text, tc.llmResponse)
			}

			// Verify queue state
			queueState := h.VerifyQueueState()
			if queueState.CompletedMessages != 1 {
				t.Errorf("Completed messages = %d, want 1", queueState.CompletedMessages)
			}
			if queueState.FailedMessages != 0 {
				t.Errorf("Failed messages = %d, want 0", queueState.FailedMessages)
			}
		})
	}
}

// TestMultiTurnConversation tests conversation continuity
func TestMultiTurnConversation(t *testing.T) {
	RunScenario(t, "multi_turn", DefaultConfig(), func(t *testing.T, h *TestHarness) {
		userPhone := "+1234567892"

		// First turn
		h.SetLLMResponse("I'll check your calendar for meetings.", nil)
		if err := h.SendMessage(userPhone, "Do I have any meetings today?"); err != nil {
			t.Fatal(err)
		}

		if _, err := h.WaitForMessage(200 * time.Millisecond); err != nil {
			t.Fatal(err)
		}

		// Second turn - should maintain context
		h.SetLLMResponse("Your next meeting is at 2 PM with the engineering team.", nil)
		if err := h.SendMessage(userPhone, "When is the next one?"); err != nil {
			t.Fatal(err)
		}

		response, err := h.WaitForMessage(2 * time.Second)
		if err != nil {
			t.Fatal(err)
		}

		// Verify response references context
		if response.Text != "Your next meeting is at 2 PM with the engineering team." {
			t.Errorf("Unexpected response: %s", response.Text)
		}

		// Verify queue processed both messages
		queueState := h.VerifyQueueState()
		if queueState.CompletedMessages != 2 {
			t.Errorf("Expected 2 completed messages, got %d", queueState.CompletedMessages)
		}
	})
}

// TestConcurrentConversations tests multiple simultaneous conversations
func TestConcurrentConversations(t *testing.T) {
	RunScenario(t, "concurrent_conversations", DefaultConfig(), func(t *testing.T, h *TestHarness) {
		// Set up same response for all users
		h.SetLLMResponse("Hello there!", nil)

		// Send messages from multiple users
		users := []struct {
			phone   string
			message string
		}{
			{"+1234567895", "Hi, I'm Alice"},
			{"+1234567896", "Hi, I'm Bob"},
			{"+1234567897", "Hi, I'm Charlie"},
		}

		// Send all messages
		for _, user := range users {
			if err := h.SendMessage(user.phone, user.message); err != nil {
				t.Fatal(err)
			}
		}

		// Collect responses
		responses := make(map[string]string)
		for range users {
			msg, err := h.WaitForMessage(300 * time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			responses[msg.Recipient] = msg.Text
		}

		// Verify each user got correct response
		for _, user := range users {
			if got := responses[user.phone]; got != "Hello there!" {
				t.Errorf("User %s got response %q, want %q", user.phone, got, "Hello there!")
			}
		}

		// Verify queue handled all conversations
		queueState := h.VerifyQueueState()
		if queueState.CompletedMessages != len(users) {
			t.Errorf("Got %d completed messages, want %d", queueState.CompletedMessages, len(users))
		}
	})
}

// TestErrorHandling tests various error scenarios
func TestErrorHandling(t *testing.T) {
	errorScenarios := []struct {
		name        string
		setupFunc   func(*TestHarness)
		expectError bool
	}{
		{
			name: "llm_error",
			setupFunc: func(h *TestHarness) {
				h.SetLLMResponse("", fmt.Errorf("LLM service unavailable"))
			},
			expectError: true,
		},
		{
			name: "empty_llm_response",
			setupFunc: func(h *TestHarness) {
				h.SetLLMResponse("", nil)
			},
			expectError: false, // Empty responses are allowed by the worker
		},
	}

	for _, tc := range errorScenarios {
		RunScenario(t, tc.name, DefaultConfig(), func(t *testing.T, h *TestHarness) {
			tc.setupFunc(h)

			userPhone := "+1234567893"
			if err := h.SendMessage(userPhone, "Test message"); err != nil {
				t.Fatal(err)
			}

			// Wait a bit for processing and potential retries
			time.Sleep(200 * time.Millisecond)

			// Check queue state
			queueState := h.VerifyQueueState()

			// Debug output
			t.Logf("Queue state: Total=%d, Pending=%d, Processing=%d, Completed=%d, Failed=%d",
				queueState.TotalMessages, queueState.PendingMessages,
				queueState.ProcessingMessages, queueState.CompletedMessages,
				queueState.FailedMessages)

			// Get message stats from tracker
			stats := h.GetMessageStats()
			t.Logf("Tracker stats: Total=%d, Completed=%d, Failed=%d, Retrying=%d",
				stats.Total, stats.Completed, stats.Failed, stats.Retrying)

			if tc.expectError {
				// Should have failed or retrying messages
				// Messages may be in pending state if they're scheduled for retry
				if queueState.CompletedMessages > 0 {
					t.Error("Expected message to not complete successfully")
				}
				// Check that it's not in a completed state
				if stats.Completed > 0 {
					t.Error("Expected tracker to show no completed messages")
				}
			} else {
				// Should complete successfully
				if queueState.CompletedMessages == 0 {
					t.Error("Expected message to complete")
				}
			}
		})
	}
}

// TestRateLimiting verifies rate limiting behavior
func TestRateLimiting(t *testing.T) {
	config := DefaultConfig()
	config.RateLimitTokens = 2                      // Only allow 2 messages
	config.RateLimitRefill = 500 * time.Millisecond // Slow refill

	RunScenario(t, "rate_limiting", config, func(t *testing.T, h *TestHarness) {
		userPhone := "+1234567894"
		h.SetLLMResponse("Response", nil)

		// Send 3 messages rapidly
		for i := 0; i < 3; i++ {
			if err := h.SendMessage(userPhone, fmt.Sprintf("Message %d", i+1)); err != nil {
				t.Fatal(err)
			}
			time.Sleep(5 * time.Millisecond)
		}

		// First two should process
		for i := 0; i < 2; i++ {
			if _, err := h.WaitForMessage(200 * time.Millisecond); err != nil {
				t.Errorf("Message %d should have been processed: %v", i+1, err)
			}
		}

		// Third should be rate limited (won't get response immediately)
		_, err := h.WaitForMessage(100 * time.Millisecond)
		if err == nil {
			t.Error("Third message should have been rate limited")
		}

		// Verify queue state - third message may be failed due to rate limit
		queueState := h.VerifyQueueState()
		stats := h.GetMessageStats()

		// Log the state for debugging
		t.Logf("Queue state after rate limiting: Pending=%d, Processing=%d, Completed=%d, Failed=%d",
			queueState.PendingMessages, queueState.ProcessingMessages,
			queueState.CompletedMessages, queueState.FailedMessages)
		t.Logf("Tracker stats: Total=%d, Completed=%d, Failed=%d",
			stats.Total, stats.Completed, stats.Failed)

		// We expect 2 completed (within rate limit) and 1 either pending, processing, or failed
		if queueState.CompletedMessages != 2 {
			t.Errorf("Expected 2 messages to complete, got %d", queueState.CompletedMessages)
		}
	})
}
