// +build integration

package integration

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// TestMessageLifecycleTracking verifies comprehensive message tracking
func TestMessageLifecycleTracking(t *testing.T) {
	RunScenario(t, "lifecycle_tracking", DefaultConfig(), func(t *testing.T, h *TestHarness) {
		h.SetLLMResponse("Tracked response", nil)
		
		// Send message
		if err := h.SendMessage("+1234567890", "Track this message"); err != nil {
			t.Fatal(err)
		}
		
		// Wait for processing
		time.Sleep(2 * time.Second)
		
		// Get message statistics
		stats := h.GetMessageStats()
		if stats.Completed != 1 {
			t.Errorf("Expected 1 completed message, got %d", stats.Completed)
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
			time.Sleep(10 * time.Millisecond) // Slight delay to spread load
		}
		
		// Wait for all to complete
		time.Sleep(5 * time.Second)
		
		// Analyze performance
		duration := time.Since(startTime)
		stats := h.GetMessageStats()
		
		t.Logf("Performance Summary:")
		t.Logf("  Total messages: %d", stats.Total)
		t.Logf("  Completed: %d", stats.Completed)
		t.Logf("  Failed: %d", stats.Failed)
		t.Logf("  Duration: %v", duration)
		t.Logf("  Throughput: %.2f msg/sec", float64(stats.Completed)/duration.Seconds())
		
		// Verify completion rate
		completionRate := float64(stats.Completed) / float64(messageCount)
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
		time.Sleep(2 * time.Second)
		
		// Force garbage collection
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
		
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
	config.QueueDepth = 2  // Very small queue
	
	RunScenario(t, "queue_overflow", config, func(t *testing.T, h *TestHarness) {
		userPhone := "+1234567898"
		h.SetLLMResponse("Response", nil)
		
		// Try to enqueue more than queue depth
		var sendErrors []error
		for i := 0; i < 5; i++ {
			err := h.SendMessage(userPhone, fmt.Sprintf("Message %d", i+1))
			if err != nil {
				sendErrors = append(sendErrors, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
		
		// Should have gotten overflow errors
		if len(sendErrors) == 0 {
			t.Error("Expected queue overflow errors")
		}
		
		// Verify queue state
		queueState := h.VerifyQueueState()
		t.Logf("Queue state: pending=%d, processing=%d, completed=%d",
			queueState.PendingMessages, queueState.ProcessingMessages, queueState.CompletedMessages)
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
			resp, err := h.WaitForMessage(2 * time.Second)
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
	RunScenario(t, "worker_recovery", DefaultConfig(), func(t *testing.T, h *TestHarness) {
		userPhone := "+1234567899"
		
		// First message succeeds
		h.SetLLMResponse("Success", nil)
		if err := h.SendMessage(userPhone, "Message 1"); err != nil {
			t.Fatal(err)
		}
		if _, err := h.WaitForMessage(2 * time.Second); err != nil {
			t.Fatal(err)
		}
		
		// Simulate worker failure by causing LLM error
		h.SetLLMResponse("", fmt.Errorf("worker crash"))
		if err := h.SendMessage(userPhone, "Message 2"); err != nil {
			t.Fatal(err)
		}
		
		// Wait a bit for failure handling
		time.Sleep(500 * time.Millisecond)
		
		// Recovery - next message should work
		h.SetLLMResponse("Recovered", nil)
		if err := h.SendMessage(userPhone, "Message 3"); err != nil {
			t.Fatal(err)
		}
		
		response, err := h.WaitForMessage(3 * time.Second)
		if err != nil {
			t.Fatal("Worker pool did not recover from failure")
		}
		
		if response.Text != "Recovered" {
			t.Errorf("Got response %q, want %q", response.Text, "Recovered")
		}
		
		// Verify queue state shows failure and recovery
		queueState := h.VerifyQueueState()
		t.Logf("Final state: completed=%d, failed=%d", 
			queueState.CompletedMessages, queueState.FailedMessages)
	})
}