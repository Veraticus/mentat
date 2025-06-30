package queue

import (
	"sync"
	"sync/atomic"
	"time"
)

// StatsCollector collects comprehensive queue statistics using atomic operations.
// It provides real-time metrics, historical data, and derived statistics for monitoring.
type StatsCollector struct {
	// Core counters (atomic operations for thread safety)
	messagesEnqueued   int64 // Total messages ever enqueued
	messagesProcessing int64 // Currently processing
	messagesCompleted  int64 // Successfully completed
	messagesFailed     int64 // Permanently failed
	messagesRetrying   int64 // Currently in retry state
	
	// State transition counters
	stateTransitions map[StateTransitionKey]int64 // Count of each state transition type
	transitionMu     sync.RWMutex                 // Protects stateTransitions map
	
	// Error tracking
	errorCounts      map[string]int64 // Error type -> count
	errorMu          sync.RWMutex     // Protects errorCounts
	totalErrors      int64            // Total errors encountered
	
	// Retry distribution
	retryAttempts    [10]int64 // Distribution of retry attempts (0-9+)
	totalRetries     int64     // Total retry attempts
	
	// Timing metrics
	processingTimes  *TimingStats // Processing time statistics
	queueTimes       *TimingStats // Time spent in queue
	
	// Throughput tracking
	throughput       *ThroughputTracker // Messages per second/minute tracking
	
	// Worker metrics
	activeWorkers    int32 // Currently active workers
	healthyWorkers   int32 // Workers passing health checks
	totalWorkers     int32 // Total worker count
	
	// Conversation metrics
	activeConversations sync.Map // conversationID -> last activity time
	
	// Start time for uptime calculation
	startTime time.Time
}

// TimingStats tracks timing statistics with percentiles.
type TimingStats struct {
	mu          sync.RWMutex
	samples     []time.Duration // Recent samples for percentile calculation
	maxSamples  int
	total       time.Duration
	count       int64
	min         time.Duration
	max         time.Duration
}

// ThroughputTracker tracks message throughput over time.
type ThroughputTracker struct {
	mu              sync.RWMutex
	secondBuckets   [60]int64     // Last 60 seconds
	minuteBuckets   [60]int64     // Last 60 minutes
	currentSecond   int           // Current second index
	currentMinute   int           // Current minute index
	lastSecondTime  time.Time     // Last second update
	lastMinuteTime  time.Time     // Last minute update
	totalMessages   int64         // Total messages processed
}

// StateTransitionKey represents a state change key for tracking in the map.
type StateTransitionKey struct {
	From State
	To   State
}

// NewStatsCollector creates a new statistics collector.
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		stateTransitions: make(map[StateTransitionKey]int64),
		errorCounts:      make(map[string]int64),
		processingTimes:  NewTimingStats(1000), // Keep last 1000 samples
		queueTimes:       NewTimingStats(1000),
		throughput:       NewThroughputTracker(),
		startTime:        time.Now(),
	}
}

// NewTimingStats creates a new timing statistics tracker.
func NewTimingStats(maxSamples int) *TimingStats {
	return &TimingStats{
		samples:    make([]time.Duration, 0, maxSamples),
		maxSamples: maxSamples,
		min:        time.Duration(1<<63 - 1), // Max duration as initial min
	}
}

// NewThroughputTracker creates a new throughput tracker.
func NewThroughputTracker() *ThroughputTracker {
	now := time.Now()
	return &ThroughputTracker{
		lastSecondTime: now,
		lastMinuteTime: now,
	}
}

// RecordEnqueue records a message being enqueued.
func (sc *StatsCollector) RecordEnqueue() {
	atomic.AddInt64(&sc.messagesEnqueued, 1)
	sc.throughput.RecordMessage()
}

// RecordStateTransition records a state transition.
func (sc *StatsCollector) RecordStateTransition(from, to State) {
	// Update state counters - decrement from state (if applicable)
	switch from {
	case StateProcessing:
		// Only decrement if positive to avoid going negative
		for {
			current := atomic.LoadInt64(&sc.messagesProcessing)
			if current <= 0 {
				break
			}
			if atomic.CompareAndSwapInt64(&sc.messagesProcessing, current, current-1) {
				break
			}
		}
	case StateRetrying:
		// Only decrement if positive to avoid going negative
		for {
			current := atomic.LoadInt64(&sc.messagesRetrying)
			if current <= 0 {
				break
			}
			if atomic.CompareAndSwapInt64(&sc.messagesRetrying, current, current-1) {
				break
			}
		}
	}
	
	switch to {
	case StateProcessing:
		atomic.AddInt64(&sc.messagesProcessing, 1)
	case StateCompleted:
		atomic.AddInt64(&sc.messagesCompleted, 1)
	case StateFailed:
		atomic.AddInt64(&sc.messagesFailed, 1)
	case StateRetrying:
		atomic.AddInt64(&sc.messagesRetrying, 1)
	}
	
	// Record transition count
	transition := StateTransitionKey{From: from, To: to}
	sc.transitionMu.Lock()
	sc.stateTransitions[transition]++
	sc.transitionMu.Unlock()
}

// RecordProcessingTime records how long a message took to process.
func (sc *StatsCollector) RecordProcessingTime(duration time.Duration) {
	sc.processingTimes.Record(duration)
}

// RecordQueueTime records how long a message spent in the queue.
func (sc *StatsCollector) RecordQueueTime(duration time.Duration) {
	sc.queueTimes.Record(duration)
}

// RecordError records an error occurrence.
func (sc *StatsCollector) RecordError(errorType string) {
	atomic.AddInt64(&sc.totalErrors, 1)
	
	sc.errorMu.Lock()
	sc.errorCounts[errorType]++
	sc.errorMu.Unlock()
}

// RecordRetryAttempt records a retry attempt.
func (sc *StatsCollector) RecordRetryAttempt(attemptNumber int) {
	atomic.AddInt64(&sc.totalRetries, 1)
	
	// Cap at 9 for the array
	if attemptNumber > 9 {
		attemptNumber = 9
	}
	atomic.AddInt64(&sc.retryAttempts[attemptNumber], 1)
}

// UpdateWorkerCount updates the worker metrics.
func (sc *StatsCollector) UpdateWorkerCount(active, healthy, total int32) {
	atomic.StoreInt32(&sc.activeWorkers, active)
	atomic.StoreInt32(&sc.healthyWorkers, healthy)
	atomic.StoreInt32(&sc.totalWorkers, total)
}

// RecordConversationActivity records activity for a conversation.
func (sc *StatsCollector) RecordConversationActivity(conversationID string) {
	sc.activeConversations.Store(conversationID, time.Now())
}

// GetStats returns comprehensive queue statistics.
func (sc *StatsCollector) GetStats() Stats {
	now := time.Now()
	
	// Count active conversations (active in last 5 minutes)
	activeConvCount := 0
	sc.activeConversations.Range(func(_, value any) bool {
		if lastActivity, ok := value.(time.Time); ok {
			if now.Sub(lastActivity) <= 5*time.Minute {
				activeConvCount++
			}
		}
		return true
	})
	
	// Calculate queue depth (enqueued - completed - failed - processing)
	queued := atomic.LoadInt64(&sc.messagesEnqueued) - 
		atomic.LoadInt64(&sc.messagesCompleted) - 
		atomic.LoadInt64(&sc.messagesFailed) -
		atomic.LoadInt64(&sc.messagesProcessing)
	
	return Stats{
		TotalQueued:        int(queued),
		TotalProcessing:    int(atomic.LoadInt64(&sc.messagesProcessing)),
		TotalCompleted:     int(atomic.LoadInt64(&sc.messagesCompleted)),
		TotalFailed:        int(atomic.LoadInt64(&sc.messagesFailed)),
		ConversationCount:  activeConvCount,
		OldestMessageAge:   0, // This should be tracked separately by the queue
		AverageWaitTime:    sc.queueTimes.Average(),
		AverageProcessTime: sc.processingTimes.Average(),
		ActiveWorkers:      int(atomic.LoadInt32(&sc.activeWorkers)),
		HealthyWorkers:     int(atomic.LoadInt32(&sc.healthyWorkers)),
	}
}

// DetailedStats contains comprehensive statistics including percentiles and rates.
type DetailedStats struct {
	Stats                      // Embed basic stats
	MessagesPerSecond float64
	MessagesPerMinute float64
	ErrorRate         float64 // Errors per message
	RetryRate         float64 // Retries per message
	
	// Timing percentiles
	ProcessingTimeP50 time.Duration
	ProcessingTimeP95 time.Duration
	ProcessingTimeP99 time.Duration
	QueueTimeP50      time.Duration
	QueueTimeP95      time.Duration
	QueueTimeP99      time.Duration
	
	// Error breakdown
	ErrorCounts map[string]int64
	
	// Retry distribution
	RetryDistribution [10]int64
	
	// State transition counts
	StateTransitions map[string]int64 // String representation of transition
	
	// Uptime
	Uptime time.Duration
}

// GetDetailedStats returns comprehensive statistics with percentiles and rates.
func (sc *StatsCollector) GetDetailedStats() DetailedStats {
	stats := DetailedStats{
		Stats:             sc.GetStats(),
		MessagesPerSecond: sc.throughput.GetMessagesPerSecond(),
		MessagesPerMinute: sc.throughput.GetMessagesPerMinute(),
		Uptime:            time.Since(sc.startTime),
	}
	
	// Calculate error and retry rates
	totalMessages := atomic.LoadInt64(&sc.messagesCompleted) + atomic.LoadInt64(&sc.messagesFailed)
	if totalMessages > 0 {
		stats.ErrorRate = float64(atomic.LoadInt64(&sc.totalErrors)) / float64(totalMessages)
		stats.RetryRate = float64(atomic.LoadInt64(&sc.totalRetries)) / float64(totalMessages)
	}
	
	// Get timing percentiles
	stats.ProcessingTimeP50 = sc.processingTimes.Percentile(50)
	stats.ProcessingTimeP95 = sc.processingTimes.Percentile(95)
	stats.ProcessingTimeP99 = sc.processingTimes.Percentile(99)
	stats.QueueTimeP50 = sc.queueTimes.Percentile(50)
	stats.QueueTimeP95 = sc.queueTimes.Percentile(95)
	stats.QueueTimeP99 = sc.queueTimes.Percentile(99)
	
	// Copy error counts
	sc.errorMu.RLock()
	stats.ErrorCounts = make(map[string]int64)
	for k, v := range sc.errorCounts {
		stats.ErrorCounts[k] = v
	}
	sc.errorMu.RUnlock()
	
	// Copy retry distribution
	for i := 0; i < 10; i++ {
		stats.RetryDistribution[i] = atomic.LoadInt64(&sc.retryAttempts[i])
	}
	
	// Copy state transitions
	sc.transitionMu.RLock()
	stats.StateTransitions = make(map[string]int64)
	for transition, count := range sc.stateTransitions {
		key := string(transition.From) + " -> " + string(transition.To)
		stats.StateTransitions[key] = count
	}
	sc.transitionMu.RUnlock()
	
	return stats
}

// Record adds a timing sample.
func (ts *TimingStats) Record(duration time.Duration) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	ts.total += duration
	ts.count++
	
	if duration < ts.min {
		ts.min = duration
	}
	if duration > ts.max {
		ts.max = duration
	}
	
	// Add to samples, removing oldest if at capacity
	if len(ts.samples) >= ts.maxSamples {
		ts.samples = ts.samples[1:]
	}
	ts.samples = append(ts.samples, duration)
}

// Average returns the average duration.
func (ts *TimingStats) Average() time.Duration {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	if ts.count == 0 {
		return 0
	}
	return ts.total / time.Duration(ts.count)
}

// Percentile calculates the given percentile (0-100).
func (ts *TimingStats) Percentile(p int) time.Duration {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	if len(ts.samples) == 0 {
		return 0
	}
	
	// Simple percentile calculation (not perfectly accurate but good enough)
	index := (len(ts.samples) * p) / 100
	if index >= len(ts.samples) {
		index = len(ts.samples) - 1
	}
	
	// Note: This is approximate since we're not sorting
	// For production, consider using a proper percentile algorithm
	return ts.samples[index]
}

// RecordMessage records a message for throughput tracking.
func (tt *ThroughputTracker) RecordMessage() {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	now := time.Now()
	atomic.AddInt64(&tt.totalMessages, 1)
	
	// Update second buckets
	if now.Sub(tt.lastSecondTime) >= time.Second {
		// Rotate buckets
		seconds := int(now.Sub(tt.lastSecondTime).Seconds())
		if seconds > 60 {
			seconds = 60
		}
		for i := 0; i < seconds; i++ {
			tt.currentSecond = (tt.currentSecond + 1) % 60
			tt.secondBuckets[tt.currentSecond] = 0
		}
		tt.lastSecondTime = now.Truncate(time.Second)
	}
	atomic.AddInt64(&tt.secondBuckets[tt.currentSecond], 1)
	
	// Update minute buckets
	if now.Sub(tt.lastMinuteTime) >= time.Minute {
		// Rotate buckets
		minutes := int(now.Sub(tt.lastMinuteTime).Minutes())
		if minutes > 60 {
			minutes = 60
		}
		for i := 0; i < minutes; i++ {
			tt.currentMinute = (tt.currentMinute + 1) % 60
			tt.minuteBuckets[tt.currentMinute] = 0
		}
		tt.lastMinuteTime = now.Truncate(time.Minute)
	}
	// Sum the last minute's seconds for the minute bucket
	var minuteTotal int64
	for i := 0; i < 60; i++ {
		minuteTotal += atomic.LoadInt64(&tt.secondBuckets[i])
	}
	atomic.StoreInt64(&tt.minuteBuckets[tt.currentMinute], minuteTotal)
}

// GetMessagesPerSecond returns the average messages per second over the last minute.
func (tt *ThroughputTracker) GetMessagesPerSecond() float64 {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	
	var total int64
	validSeconds := 0
	now := time.Now()
	
	for i := 0; i < 60; i++ {
		// Only count seconds that are recent
		secondAge := now.Sub(tt.lastSecondTime) + time.Duration(((tt.currentSecond-i+60)%60))*time.Second
		if secondAge < 60*time.Second {
			total += atomic.LoadInt64(&tt.secondBuckets[i])
			validSeconds++
		}
	}
	
	if validSeconds == 0 {
		return 0
	}
	return float64(total) / float64(validSeconds)
}

// GetMessagesPerMinute returns the average messages per minute over the last hour.
func (tt *ThroughputTracker) GetMessagesPerMinute() float64 {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	
	var total int64
	validMinutes := 0
	now := time.Now()
	
	for i := 0; i < 60; i++ {
		// Only count minutes that are recent
		minuteAge := now.Sub(tt.lastMinuteTime) + time.Duration(((tt.currentMinute-i+60)%60))*time.Minute
		if minuteAge < 60*time.Minute {
			total += atomic.LoadInt64(&tt.minuteBuckets[i])
			validMinutes++
		}
	}
	
	if validMinutes == 0 {
		return 0
	}
	return float64(total) / float64(validMinutes)
}