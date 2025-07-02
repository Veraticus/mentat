package queue

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// Queue manager constants.
const (
	// requestTimeout is the timeout for message requests.
	requestTimeout = 50 * time.Millisecond
	// retryJitterDivisor is used to calculate retry jitter (20% jitter).
	retryJitterDivisor = 5
)

// Coordinator implements the MessageQueue interface with conversation isolation.
// It wraps the Manager to provide the MessageQueue interface while maintaining
// compatibility with the signal.IncomingMessage type.
//
// The Coordinator:
//   - Converts between signal.IncomingMessage and internal Message types
//   - Tracks QueuedMessage metadata for the MessageQueue interface
//   - Maintains statistics about queue operations
//   - Provides the GetNext method required by the MessageQueue interface
//
// Fair scheduling is delegated to the underlying Manager, which implements
// round-robin scheduling with conversation affinity.
type Coordinator struct {
	manager         *Manager
	messages        sync.Map // messageID -> *Message mapping
	stats           *queueStats
	statsCollector  *StatsCollector // New comprehensive stats collector
	stopped         int32
	shutdownTimeout time.Duration
}

// queueStats tracks queue statistics with atomic operations.
type queueStats struct {
	totalQueued     int64
	totalProcessing int64
	totalCompleted  int64
	totalFailed     int64
	totalMessages   int64
	queueStartTime  time.Time
	processingTimes sync.Map // messageID -> start time for processing duration calculation
	completionTimes sync.Map // messageID -> completion time
}

// NewCoordinator creates a new queue coordinator implementing MessageQueue.
func NewCoordinator(ctx context.Context) *Coordinator {
	qm := &Coordinator{
		manager: NewManager(ctx),
		stats: &queueStats{
			queueStartTime: time.Now(),
		},
		statsCollector:  NewStatsCollector(),
		shutdownTimeout: defaultShutdownTimeout,
	}

	// Start the underlying manager
	go qm.manager.Start(ctx)

	return qm
}

// Enqueue adds a message to the queue.
func (qm *Coordinator) Enqueue(msg signal.IncomingMessage) error {
	if atomic.LoadInt32(&qm.stopped) == 1 {
		return fmt.Errorf("queue manager is stopped")
	}

	// Generate ID from timestamp and sender
	msgID := strconv.FormatInt(msg.Timestamp.UnixNano(), 10) + "-" + msg.From
	conversationID := msg.From // Use sender as conversation ID

	// Create internal Message from IncomingMessage
	internalMsg := NewMessage(msgID, conversationID, msg.From, msg.FromNumber, msg.Text)

	// Add initial state transition
	internalMsg.AddStateTransition(StateQueued, StateQueued, "message created")

	// Store the message for later retrieval
	qm.messages.Store(msgID, internalMsg)

	// Update stats
	atomic.AddInt64(&qm.stats.totalQueued, 1)
	atomic.AddInt64(&qm.stats.totalMessages, 1)

	// Update comprehensive stats
	qm.statsCollector.RecordEnqueue()
	qm.statsCollector.RecordConversationActivity(conversationID)

	// Submit to underlying manager
	return qm.manager.Submit(internalMsg)
}

// GetNext returns the next message for a worker to process.
// It uses the underlying Manager's fair scheduling algorithm which ensures:
//   - Round-robin processing across conversations
//   - No conversation starvation
//   - Messages within a conversation are processed in order
//   - Only one message per conversation is active at a time
//
// The method uses a short timeout (50ms) to avoid blocking indefinitely,
// allowing workers to check for shutdown signals periodically.
func (qm *Coordinator) GetNext(workerID string) (*Message, error) {
	if atomic.LoadInt32(&qm.stopped) == 1 {
		return nil, fmt.Errorf("queue manager is stopped")
	}

	// Use a short timeout context to avoid blocking indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	// Request from underlying manager
	msg, err := qm.manager.RequestMessage(ctx)
	if err != nil {
		return nil, err
	}

	if msg == nil {
		return nil, err
	}

	// Update state and stats
	msg.SetState(StateProcessing)
	msg.AddStateTransition(StateQueued, StateProcessing, "Assigned to worker "+workerID)

	now := time.Now()
	atomic.AddInt64(&qm.stats.totalQueued, -1)
	atomic.AddInt64(&qm.stats.totalProcessing, 1)
	qm.stats.processingTimes.Store(msg.ID, now)

	// Update comprehensive stats
	qm.statsCollector.RecordStateTransition(StateQueued, StateProcessing)
	if msg.CreatedAt.Before(now) {
		qm.statsCollector.RecordQueueTime(now.Sub(msg.CreatedAt))
	}

	return msg, nil
}

// handleCompletedState handles the transition to completed state.
func (qm *Coordinator) handleCompletedState(msgID string, msg *Message, now time.Time) error {
	msg.SetCompletedAt(now)
	qm.stats.completionTimes.Store(msgID, now)
	atomic.AddInt64(&qm.stats.totalProcessing, -1)
	atomic.AddInt64(&qm.stats.totalCompleted, 1)

	// Record processing time if we have start time
	if startTime, ok := qm.stats.processingTimes.Load(msgID); ok {
		if start, isTime := startTime.(time.Time); isTime {
			qm.statsCollector.RecordProcessingTime(now.Sub(start))
		}
	}

	// Mark as completed in underlying manager
	if err := qm.manager.CompleteMessage(msg); err != nil {
		return fmt.Errorf("failed to complete message: %w", err)
	}
	return nil
}

// handleFailedState handles the transition to failed state.
func (qm *Coordinator) handleFailedState(msg *Message, _ time.Time) error {
	atomic.AddInt64(&qm.stats.totalProcessing, -1)
	atomic.AddInt64(&qm.stats.totalFailed, 1)

	// Record error if present
	if msg.Error != nil {
		qm.recordError(msg.Error)
	}

	// Mark as completed in underlying manager (failed is terminal)
	if err := qm.manager.CompleteMessage(msg); err != nil {
		return fmt.Errorf("failed to mark message as failed: %w", err)
	}
	return nil
}

// recordError records an error in the stats collector.
func (qm *Coordinator) recordError(err error) {
	var errType string
	switch {
	case IsRateLimitError(err):
		errType = "rate_limit"
	case strings.Contains(strings.ToLower(err.Error()), "validation"):
		errType = "validation"
	default:
		errType = "processing"
	}
	qm.statsCollector.RecordError(errType)
}

// handleRetryingState handles the transition to retrying state.
func (qm *Coordinator) handleRetryingState(msgID string, msg *Message, now time.Time) error {
	attempts := msg.IncrementAttempts()
	qm.statsCollector.RecordRetryAttempt(attempts)

	// Calculate retry delay
	retryDelay := qm.calculateRetryDelay(msgID, msg)

	nextRetryTime := now.Add(retryDelay)
	msg.SetNextRetryAt(nextRetryTime)

	// Requeue the message
	msg.SetState(StateQueued)
	if err := qm.manager.Submit(msg); err != nil {
		return fmt.Errorf("failed to requeue message for retry: %w", err)
	}
	atomic.AddInt64(&qm.stats.totalProcessing, -1)
	atomic.AddInt64(&qm.stats.totalQueued, 1)

	return nil
}

// calculateRetryDelay calculates the retry delay based on error type and attempt count.
func (qm *Coordinator) calculateRetryDelay(msgID string, msg *Message) time.Duration {
	var retryDelay time.Duration

	// Check if the last error was a rate limit error
	if msg.Error != nil {
		if IsRateLimitError(msg.Error) {
			// For rate limit errors, use longer backoff
			retryDelay = calculateRateLimitRetryDelay(msg.Attempts)
			logger := slog.Default()
			logger.InfoContext(context.TODO(), "Message rate limited",
				slog.String("id", msgID),
				slog.Duration("retry_delay", retryDelay),
				slog.Int("attempt", msg.Attempts),
				slog.Int("max_attempts", msg.MaxAttempts))
		} else {
			// For other errors, use standard exponential backoff
			retryDelay = CalculateRetryDelay(msg.Attempts)
		}
	}

	// If no delay was calculated, use default
	if retryDelay == 0 {
		retryDelay = CalculateRetryDelay(msg.Attempts)
	}

	return retryDelay
}

// UpdateState marks a message state transition.
func (qm *Coordinator) UpdateState(msgID string, state State, reason string) error {
	if atomic.LoadInt32(&qm.stopped) == 1 {
		return fmt.Errorf("queue manager is stopped")
	}

	// Get the internal message
	msgValue, ok := qm.messages.Load(msgID)
	if !ok {
		return fmt.Errorf("message not found: %s", msgID)
	}
	msg, ok := msgValue.(*Message)
	if !ok {
		return fmt.Errorf("invalid message type for ID: %s", msgID)
	}

	// Record state transition
	oldState := msg.GetState()
	msg.SetState(state)
	msg.AddStateTransition(oldState, state, reason)

	now := time.Now()

	// Record in comprehensive stats
	qm.statsCollector.RecordStateTransition(oldState, state)

	// Update timing information
	switch state {
	case StateCompleted:
		return qm.handleCompletedState(msgID, msg, now)
	case StateFailed:
		return qm.handleFailedState(msg, now)
	case StateRetrying:
		return qm.handleRetryingState(msgID, msg, now)
	case StateQueued, StateProcessing, StateValidating:
		// These states don't require special handling
	}

	return nil
}

// Stats returns queue statistics.
func (qm *Coordinator) Stats() Stats {
	// Get stats from the comprehensive collector
	stats := qm.statsCollector.GetStats()

	// Calculate oldest message age from actual messages
	var oldestMessageAge time.Duration
	now := time.Now()
	qm.messages.Range(func(_, value any) bool {
		msg, ok := value.(*Message)
		if !ok {
			return true
		}
		state := msg.GetState()
		if state == StateQueued || state == StateProcessing || state == StateRetrying {
			age := now.Sub(msg.CreatedAt)
			if age > oldestMessageAge {
				oldestMessageAge = age
			}
		}
		return true
	})

	stats.LongestMessageAge = oldestMessageAge
	return stats
}

// GetDetailedStats returns comprehensive queue statistics including rates and percentiles.
func (qm *Coordinator) GetDetailedStats() DetailedStats {
	return qm.statsCollector.GetDetailedStats()
}

// Stop gracefully shuts down the queue manager.
func (qm *Coordinator) Stop() error {
	if !atomic.CompareAndSwapInt32(&qm.stopped, 0, 1) {
		return fmt.Errorf("already stopped")
	}

	return qm.manager.Shutdown(qm.shutdownTimeout)
}

// calculateRateLimitRetryDelay calculates the retry delay for rate-limited messages.
// Uses longer delays than regular retries to respect provider limits.
func calculateRateLimitRetryDelay(attempts int) time.Duration {
	const (
		baseDelay = 30 * time.Second // Start with 30 seconds for rate limits
		maxDelay  = 10 * time.Minute // Cap at 10 minutes
		maxShift  = 10               // Prevent overflow
	)

	// Handle edge cases
	if attempts <= 0 {
		return baseDelay
	}

	// Use multiplication for exponential backoff
	delay := baseDelay
	for i := 0; i < attempts && i < maxShift; i++ {
		delay *= 2
		if delay > maxDelay {
			return maxDelay
		}
	}

	// Add 10-20% jitter to prevent thundering herd
	jitterRange := delay / retryJitterDivisor // 20% total range
	if jitterRange > 0 {
		// Use modulo to get a value within jitter range
		jitter := time.Duration(time.Now().UnixNano() % int64(jitterRange))
		delay = delay - jitterRange/2 + jitter
	}

	return delay
}
