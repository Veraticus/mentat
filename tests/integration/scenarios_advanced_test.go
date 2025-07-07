//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
)

// TestMessageLifecycleTracking verifies comprehensive message tracking
func TestMessageLifecycleTracking(t *testing.T) {
	RunScenario(t, "lifecycle_tracking", DefaultConfig(), func(t *testing.T, h *TestHarness) {
		h.SetLLMResponse("Tracked response", nil)

		// Send message
		if err := h.SendMessage("+1234567890", "Track this message"); err != nil {
			t.Fatal(err)
		}

		// Wait for message to be sent (completed)
		if _, err := h.WaitForMessage(500 * time.Millisecond); err != nil {
			t.Fatalf("Message not sent: %v", err)
		}

		// Verify completion via queue state
		state := h.VerifyQueueState()
		if state.CompletedMessages != 1 {
			t.Errorf("Expected 1 completed message, got %d", state.CompletedMessages)
		}

		// Verify queue metrics
		metrics := h.GetQueueMetrics()
		t.Logf("Queue metrics: avg depth=%.2f, max depth=%d",
			metrics.AverageDepth, metrics.MaxDepth)

		// Check state changes
		changes := h.GetStateChanges()
		foundMessageSent := false
		foundLLMConfig := false
		for _, change := range changes {
			if change.Type == "message_sent" {
				foundMessageSent = true
			}
			if change.Type == "llm_response_configured" {
				foundLLMConfig = true
			}
		}

		if !foundMessageSent || !foundLLMConfig {
			t.Error("Expected state changes not recorded")
		}
	})
}

// TestHighLoadPerformance tests system under concurrent load
func TestHighLoadPerformance(t *testing.T) {
	config := DefaultConfig()
	config.WorkerCount = 5

	RunScenario(t, "high_load", config, func(t *testing.T, h *TestHarness) {
		h.SetLLMResponse("Concurrent response", nil)

		// Send many messages concurrently
		messageCount := 20
		startTime := time.Now()

		for i := 0; i < messageCount; i++ {
			user := fmt.Sprintf("+123456%04d", i)
			if err := h.SendMessage(user, fmt.Sprintf("Message %d", i)); err != nil {
				t.Error(err)
			}
			time.Sleep(5 * time.Millisecond) // Slight delay to spread load
		}

		// Wait for all to complete
		if err := h.WaitForAllMessagesCompletion(messageCount, 1*time.Second); err != nil {
			t.Logf("Warning: Not all messages completed: %v", err)
		}

		// Analyze performance
		duration := time.Since(startTime)
		state := h.VerifyQueueState()

		t.Logf("Performance Summary:")
		t.Logf("  Total messages: %d", state.TotalMessages)
		t.Logf("  Completed: %d", state.CompletedMessages)
		t.Logf("  Failed: %d", state.FailedMessages)
		t.Logf("  Duration: %v", duration)
		t.Logf("  Throughput: %.2f msg/sec", float64(state.CompletedMessages)/duration.Seconds())

		// Verify completion rate
		completionRate := float64(state.CompletedMessages) / float64(messageCount)
		if completionRate < 0.95 {
			t.Errorf("Low completion rate: %.2f%%", completionRate*100)
		}

		// Check for queue alerts
		alerts := h.queueMonitor.GetAlerts()
		if len(alerts) > 0 {
			t.Logf("Queue alerts detected:")
			for _, alert := range alerts {
				t.Logf("  - %s: %s", alert.Type, alert.Description)
			}
		}
	})
}

// TestResourceLeakDetection verifies no resource leaks
func TestResourceLeakDetection(t *testing.T) {
	RunScenario(t, "resource_leaks", DefaultConfig(), func(t *testing.T, h *TestHarness) {
		// Get baseline
		baseline := h.resourceMonitor.Sample()

		// Process many messages
		for i := 0; i < 10; i++ {
			h.SetLLMResponse(fmt.Sprintf("Response %d", i), nil)
			if err := h.SendMessage(fmt.Sprintf("+12345679%02d", i), "Test"); err != nil {
				t.Error(err)
			}
		}

		// Wait for completion
		time.Sleep(10 * time.Millisecond)

		// Force garbage collection
		runtime.GC()
		time.Sleep(10 * time.Millisecond)

		// Check final state
		final := h.resourceMonitor.Sample()

		goroutineLeak := final.Goroutines - baseline.Goroutines
		if goroutineLeak > 5 { // Allow small variance
			t.Errorf("Potential goroutine leak: started with %d, ended with %d (diff: %d)",
				baseline.Goroutines, final.Goroutines, goroutineLeak)
		}

		// Check memory growth
		memGrowth := int64(final.HeapAlloc) - int64(baseline.HeapAlloc)
		memGrowthMB := float64(memGrowth) / 1024 / 1024
		t.Logf("Memory growth: %.2f MB", memGrowthMB)

		if memGrowthMB > 10 {
			t.Errorf("Excessive memory growth: %.2f MB", memGrowthMB)
		}
	})
}

// TestQueueOverflow tests behavior when queue is full
func TestQueueOverflow(t *testing.T) {
	config := DefaultConfig()
	config.QueueDepth = 2      // Very small queue
	config.WorkerCount = 1     // Minimal workers
	config.RateLimitTokens = 0 // No tokens to prevent processing

	RunScenario(t, "queue_overflow", config, func(t *testing.T, h *TestHarness) {
		userPhone := "+1234567898"
		h.SetLLMResponse("Response", nil)

		// Try to enqueue more than queue depth rapidly
		var sendErrors []error
		for i := 0; i < 5; i++ {
			err := h.SendMessage(userPhone, fmt.Sprintf("Message %d", i+1))
			if err != nil {
				sendErrors = append(sendErrors, err)
			}
		}

		// Give a moment for queue state to stabilize
		time.Sleep(10 * time.Millisecond)

		// Should have gotten overflow errors after filling the queue
		if len(sendErrors) == 0 {
			// Check if queue is actually full
			queueState := h.VerifyQueueState()
			t.Logf("Queue state: pending=%d, processing=%d, completed=%d, total=%d",
				queueState.PendingMessages, queueState.ProcessingMessages,
				queueState.CompletedMessages, queueState.TotalMessages)

			// If we have messages stuck in the queue due to rate limiting, that's acceptable
			if queueState.PendingMessages+queueState.ProcessingMessages+queueState.RetryingMessages < config.QueueDepth {
				t.Error("Expected queue overflow errors or full queue")
			}
		} else {
			t.Logf("Got %d overflow errors as expected", len(sendErrors))
		}
	})
}

// TestMessageOrdering verifies FIFO within conversations
func TestMessageOrdering(t *testing.T) {
	RunScenario(t, "message_ordering", DefaultConfig(), func(t *testing.T, h *TestHarness) {
		userPhone := "+1234567895"
		messageCount := 5

		// Send multiple messages rapidly
		for i := 0; i < messageCount; i++ {
			msg := fmt.Sprintf("Message %d", i)
			if err := h.SendMessage(userPhone, msg); err != nil {
				t.Fatal(err)
			}
		}

		// Collect responses
		responses := make([]string, 0, messageCount)
		for i := 0; i < messageCount; i++ {
			resp, err := h.WaitForMessage(200 * time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			responses = append(responses, resp.Text)
		}

		// Verify order preserved (mock returns "Mock response for: <prompt>")
		for i, resp := range responses {
			expected := fmt.Sprintf("Mock response for: Message %d", i)
			if resp != expected {
				t.Errorf("Message %d out of order: got %q, want %q", i, resp, expected)
			}
		}
	})
}

// TestWorkerFailureRecovery tests worker pool resilience
func TestWorkerFailureRecovery(t *testing.T) {
	config := DefaultConfig()
	config.WorkerCount = 2 // Ensure we have workers

	RunScenario(t, "worker_recovery", config, func(t *testing.T, h *TestHarness) {
		userPhone := "+1234567899"

		// First message succeeds
		h.SetLLMResponse("Success", nil)
		if err := h.SendMessage(userPhone, "Message 1"); err != nil {
			t.Fatal(err)
		}
		response1, err := h.WaitForMessage(200 * time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		if response1.Text != "Success" {
			t.Errorf("First message failed: got %q, want %q", response1.Text, "Success")
		}

		// Configure a transient error scenario
		errorCount := 0
		h.mockLLM.QueryFunc = func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error) {
			// Fail the first call after initial success
			if errorCount == 0 {
				errorCount++
				return nil, fmt.Errorf("transient error")
			}
			// All subsequent calls succeed
			return &claude.LLMResponse{Message: "Recovered"}, nil
		}

		// Send second message that will fail once then retry and succeed
		if err := h.SendMessage(userPhone, "Message 2"); err != nil {
			t.Fatal(err)
		}

		// Wait sufficient time for retry (messages retry after ~2 seconds)
		time.Sleep(200 * time.Millisecond)

		// Send third message to verify pool is still working
		if err := h.SendMessage(userPhone, "Message 3"); err != nil {
			t.Fatal(err)
		}

		// Collect recovered responses
		recoveredCount := 0
		timeoutCount := 0
		maxTimeouts := 3 // Allow some timeouts while waiting for retries

		for recoveredCount < 2 && timeoutCount < maxTimeouts {
			response, err := h.WaitForMessage(200 * time.Millisecond)
			if err != nil {
				timeoutCount++
				continue
			}
			if response.Text == "Recovered" {
				recoveredCount++
				t.Logf("Got recovered response #%d", recoveredCount)
			}
		}

		// Verify we got at least one recovery
		if recoveredCount == 0 {
			t.Fatal("Worker pool did not recover from failure")
		}

		// Verify queue state
		queueState := h.VerifyQueueState()
		t.Logf("Final state: completed=%d, failed=%d, retrying=%d, pending=%d, processing=%d",
			queueState.CompletedMessages, queueState.FailedMessages, queueState.RetryingMessages,
			queueState.PendingMessages, queueState.ProcessingMessages)

		// Test passes if we got any recovered messages and have multiple completions
		if queueState.CompletedMessages >= 2 && recoveredCount > 0 {
			t.Log("Worker pool successfully recovered from transient failure")
		} else {
			t.Errorf("Recovery incomplete: completed=%d, recovered=%d",
				queueState.CompletedMessages, recoveredCount)
		}
	})
}
