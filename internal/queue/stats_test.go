package queue

import (
	"sync"
	"testing"
	"time"
)

func TestStatsCollector_RecordEnqueue(t *testing.T) {
	sc := NewStatsCollector()
	
	// Record multiple enqueues
	for i := 0; i < 10; i++ {
		sc.RecordEnqueue()
	}
	
	stats := sc.GetStats()
	if stats.TotalQueued != 10 {
		t.Errorf("expected 10 queued messages, got %d", stats.TotalQueued)
	}
}

func TestStatsCollector_StateTransitions(t *testing.T) {
	// Test various state transitions
	tests := []struct {
		name     string
		from     State
		to       State
		validate func(t *testing.T, stats Stats)
	}{
		{
			name: "queued to processing",
			from: StateQueued,
			to:   StateProcessing,
			validate: func(t *testing.T, stats Stats) {
				t.Helper()
				if stats.TotalProcessing != 1 {
					t.Errorf("expected 1 processing, got %d", stats.TotalProcessing)
				}
			},
		},
		{
			name: "processing to completed",
			from: StateProcessing,
			to:   StateCompleted,
			validate: func(t *testing.T, stats Stats) {
				t.Helper()
				if stats.TotalProcessing != 0 {
					t.Errorf("expected 0 processing, got %d", stats.TotalProcessing)
				}
				if stats.TotalCompleted != 1 {
					t.Errorf("expected 1 completed, got %d", stats.TotalCompleted)
				}
			},
		},
		{
			name: "processing to failed",
			from: StateProcessing,
			to:   StateFailed,
			validate: func(t *testing.T, stats Stats) {
				t.Helper()
				if stats.TotalFailed != 1 {
					t.Errorf("expected 1 failed, got %d", stats.TotalFailed)
				}
			},
		},
	}
	
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc := NewStatsCollector()
			sc.RecordStateTransition(tc.from, tc.to)
			stats := sc.GetStats()
			tc.validate(t, stats)
		})
	}
}

func TestStatsCollector_TimingStats(t *testing.T) {
	sc := NewStatsCollector()
	
	// Record various processing times
	times := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}
	
	for _, d := range times {
		sc.RecordProcessingTime(d)
		sc.RecordQueueTime(d)
	}
	
	stats := sc.GetStats()
	
	// Average should be 300ms
	expectedAvg := 300 * time.Millisecond
	if stats.AverageProcessTime != expectedAvg {
		t.Errorf("expected average process time %v, got %v", expectedAvg, stats.AverageProcessTime)
	}
	if stats.AverageWaitTime != expectedAvg {
		t.Errorf("expected average wait time %v, got %v", expectedAvg, stats.AverageWaitTime)
	}
}

func TestStatsCollector_ErrorTracking(t *testing.T) {
	sc := NewStatsCollector()
	
	// Record different error types
	errorTypes := []string{
		"timeout",
		"rate_limit",
		"validation_failed",
		"timeout", // Duplicate to test counting
	}
	
	for _, errType := range errorTypes {
		sc.RecordError(errType)
	}
	
	detailed := sc.GetDetailedStats()
	
	// Check error counts
	if detailed.ErrorCounts["timeout"] != 2 {
		t.Errorf("expected 2 timeout errors, got %d", detailed.ErrorCounts["timeout"])
	}
	if detailed.ErrorCounts["rate_limit"] != 1 {
		t.Errorf("expected 1 rate_limit error, got %d", detailed.ErrorCounts["rate_limit"])
	}
}

func TestStatsCollector_RetryDistribution(t *testing.T) {
	sc := NewStatsCollector()
	
	// Record retry attempts
	for i := 0; i < 5; i++ {
		for j := 0; j <= i; j++ {
			sc.RecordRetryAttempt(i)
		}
	}
	
	// Also test capping at 9
	sc.RecordRetryAttempt(15)
	
	detailed := sc.GetDetailedStats()
	
	// Check distribution
	for i := 0; i < 5; i++ {
		expected := int64(i + 1)
		if detailed.RetryDistribution[i] != expected {
			t.Errorf("expected %d retries at attempt %d, got %d", expected, i, detailed.RetryDistribution[i])
		}
	}
	
	// Check that attempt 15 was capped at 9
	if detailed.RetryDistribution[9] != 1 {
		t.Errorf("expected 1 retry at attempt 9 (capped), got %d", detailed.RetryDistribution[9])
	}
}

func TestStatsCollector_WorkerMetrics(t *testing.T) {
	sc := NewStatsCollector()
	
	sc.UpdateWorkerCount(5, 4, 5)
	
	stats := sc.GetStats()
	if stats.ActiveWorkers != 5 {
		t.Errorf("expected 5 active workers, got %d", stats.ActiveWorkers)
	}
	if stats.HealthyWorkers != 4 {
		t.Errorf("expected 4 healthy workers, got %d", stats.HealthyWorkers)
	}
}

func TestStatsCollector_ConversationTracking(t *testing.T) {
	sc := NewStatsCollector()
	
	// Record activity for different conversations
	conversations := []string{"conv1", "conv2", "conv3"}
	for _, convID := range conversations {
		sc.RecordConversationActivity(convID)
	}
	
	stats := sc.GetStats()
	if stats.ConversationCount != 3 {
		t.Errorf("expected 3 active conversations, got %d", stats.ConversationCount)
	}
}

func TestStatsCollector_ConcurrentAccess(t *testing.T) {
	sc := NewStatsCollector()
	
	// Test concurrent access to ensure thread safety
	var wg sync.WaitGroup
	iterations := 1000
	goroutines := 10
	
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				sc.RecordEnqueue()
				sc.RecordStateTransition(StateQueued, StateProcessing)
				sc.RecordProcessingTime(time.Duration(j) * time.Millisecond)
				sc.RecordError("test_error")
				sc.RecordRetryAttempt(j % 5)
				sc.RecordConversationActivity(string(rune('a' + id)))
			}
		}(i)
	}
	
	wg.Wait()
	
	// Verify counts
	stats := sc.GetStats()
	expectedProcessing := goroutines * iterations
	if stats.TotalProcessing != expectedProcessing {
		t.Errorf("expected %d processing messages, got %d", expectedProcessing, stats.TotalProcessing)
	}
	// TotalQueued should be 0 since all messages were transitioned to processing
	if stats.TotalQueued != 0 {
		t.Errorf("expected 0 queued messages (all processing), got %d", stats.TotalQueued)
	}
	
	detailed := sc.GetDetailedStats()
	expectedErrors := int64(goroutines * iterations)
	if detailed.ErrorCounts["test_error"] != expectedErrors {
		t.Errorf("expected %d test errors, got %d", expectedErrors, detailed.ErrorCounts["test_error"])
	}
}

func TestTimingStats_Percentiles(t *testing.T) {
	ts := NewTimingStats(100)
	
	// Add samples in order for predictable percentiles
	for i := 1; i <= 100; i++ {
		ts.Record(time.Duration(i) * time.Millisecond)
	}
	
	// Note: Our simple percentile implementation is approximate
	// Just verify it returns reasonable values
	p50 := ts.Percentile(50)
	p95 := ts.Percentile(95)
	p99 := ts.Percentile(99)
	
	if p50 == 0 || p95 == 0 || p99 == 0 {
		t.Error("percentiles should not be zero")
	}
	
	if p50 >= p95 || p95 >= p99 {
		t.Errorf("percentiles should increase: p50=%v, p95=%v, p99=%v", p50, p95, p99)
	}
}

func TestThroughputTracker_MessagesPerSecond(t *testing.T) {
	tt := NewThroughputTracker()
	
	// Record messages
	for i := 0; i < 10; i++ {
		tt.RecordMessage()
	}
	
	// Give it a moment to process
	time.Sleep(10 * time.Millisecond)
	
	mps := tt.GetMessagesPerSecond()
	// Should have recorded messages
	if mps == 0 {
		t.Errorf("expected non-zero messages per second, got %f", mps)
	}
}

func TestStatsCollector_DetailedStats(t *testing.T) {
	sc := NewStatsCollector()
	
	// Set up a scenario
	for i := 0; i < 100; i++ {
		sc.RecordEnqueue()
		sc.RecordStateTransition(StateQueued, StateProcessing)
		sc.RecordProcessingTime(time.Duration(i) * time.Millisecond)
		
		if i%10 == 0 {
			sc.RecordError("test_error")
			sc.RecordRetryAttempt(i / 10)
		}
		
		if i%2 == 0 {
			sc.RecordStateTransition(StateProcessing, StateCompleted)
		} else {
			sc.RecordStateTransition(StateProcessing, StateRetrying)
		}
	}
	
	detailed := sc.GetDetailedStats()
	
	// Verify detailed stats
	if detailed.Uptime <= 0 {
		t.Error("uptime should be positive")
	}
	
	if detailed.ErrorRate == 0 {
		t.Error("error rate should be non-zero")
	}
	
	if detailed.RetryRate == 0 {
		t.Error("retry rate should be non-zero")
	}
	
	// Check state transitions
	if len(detailed.StateTransitions) == 0 {
		t.Error("should have recorded state transitions")
	}
	
	// Verify specific transition
	queuedToProcessing := detailed.StateTransitions["queued -> processing"]
	if queuedToProcessing != 100 {
		t.Errorf("expected 100 queued->processing transitions, got %d", queuedToProcessing)
	}
}

func TestStatsCollector_QueueDepthCalculation(t *testing.T) {
	sc := NewStatsCollector()
	
	// Enqueue 10 messages
	for i := 0; i < 10; i++ {
		sc.RecordEnqueue()
	}
	
	// Complete 3
	for i := 0; i < 3; i++ {
		sc.RecordStateTransition(StateProcessing, StateCompleted)
	}
	
	// Fail 2
	for i := 0; i < 2; i++ {
		sc.RecordStateTransition(StateProcessing, StateFailed)
	}
	
	stats := sc.GetStats()
	// Queue depth should be 10 - 3 - 2 = 5
	if stats.TotalQueued != 5 {
		t.Errorf("expected queue depth of 5, got %d", stats.TotalQueued)
	}
}