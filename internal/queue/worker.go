package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// Worker constants.
const (
	// messageRequestTimeout is the timeout for requesting messages from queue.
	messageRequestTimeout = 30 * time.Second
	// typingIndicatorInterval is how often to send typing indicators.
	typingIndicatorInterval = 10 * time.Second
)

// WorkerConfig holds configuration for a worker.
type WorkerConfig struct {
	LLM                claude.LLM
	Messenger          signal.Messenger
	RateLimiter        RateLimiter
	QueueManager       *Manager
	MessageQueue       MessageQueue // For updating state and getting messages
	TypingIndicatorMgr signal.TypingIndicatorManager
	ID                 int
}

// worker processes messages from the queue.
type worker struct {
	stateMachine StateMachine // 16 bytes (interface)
	config       WorkerConfig // 64 bytes (embedded struct)
}

// NewWorker creates a new worker instance.
func NewWorker(config WorkerConfig) Worker {
	return &worker{
		config:       config,
		stateMachine: NewStateMachine(),
	}
}

// requestMessage requests a message from the queue manager.
func (w *worker) requestMessage(ctx context.Context) (*Message, error) {
	reqCtx, cancel := context.WithTimeout(ctx, messageRequestTimeout)
	defer cancel()

	msg, err := w.config.QueueManager.RequestMessage(reqCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// No messages available, return nil message with no error
			// The caller will check for nil message
			return nil, err
		}
		return nil, err
	}

	return msg, nil
}

// handleRetryMessage handles re-submission of messages that need to be retried.
func (w *worker) handleRetryMessage(ctx context.Context, msg *Message) {
	// Check if we're shutting down before trying to re-submit
	select {
	case <-ctx.Done():
		logger := slog.Default()
		logger.InfoContext(ctx,
			"Worker shutting down, not re-submitting message",
			slog.Int("worker_id", w.config.ID),
			slog.String("message_id", msg.ID),
		)
		return
	default:
	}

	// Complete current processing
	if err := w.config.QueueManager.CompleteMessage(msg); err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx,
			"Worker failed to complete message",
			slog.Int("worker_id", w.config.ID),
			slog.String("message_id", msg.ID),
			slog.Any("error", err),
		)
	}

	// Re-submit for retry
	if err := w.config.QueueManager.Submit(msg); err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx,
			"Worker failed to re-submit message for retry",
			slog.Int("worker_id", w.config.ID),
			slog.String("message_id", msg.ID),
			slog.Any("error", err),
		)
	}
}

// processNextMessage handles processing of a single message.
func (w *worker) processNextMessage(ctx context.Context) error {
	msg, err := w.requestMessage(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// No messages available, continue
			return nil
		}
		return err
	}

	if msg == nil {
		// Queue manager shutting down
		return nil
	}

	// Process the message (message is already in processing state from Manager)
	logger := slog.Default()
	logger.InfoContext(ctx, "Worker processing message",
		slog.Int("worker_id", w.config.ID),
		slog.String("message_id", msg.ID))
	if processErr := w.Process(ctx, msg); processErr != nil {
		logger.ErrorContext(ctx,
			"Worker error processing message",
			slog.Int("worker_id", w.config.ID),
			slog.String("message_id", msg.ID),
			slog.Any("error", processErr),
		)

		// If message is in retry state, we need to complete current processing and re-submit
		if msg.GetState() == StateRetrying {
			w.handleRetryMessage(ctx, msg)
		}
	}

	return nil
}

// Start begins processing messages. Blocks until context is canceled.
func (w *worker) Start(ctx context.Context) error {
	logger := slog.Default()
	logger.InfoContext(ctx, "Worker starting", slog.Int("worker_id", w.config.ID))

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "Worker shutting down", slog.Int("worker_id", w.config.ID))
			return fmt.Errorf("worker context canceled: %w", ctx.Err())
		default:
			if err := w.processNextMessage(ctx); err != nil {
				return err
			}
		}
	}
}

// handleRateLimitCheck checks rate limiting and handles rate limited messages.
func (w *worker) handleRateLimitCheck(ctx context.Context, msg *Message) error {
	if w.config.RateLimiter.Allow(msg.ConversationID) {
		w.config.RateLimiter.Record(msg.ConversationID)
		return nil
	}

	// Rate limited - treat this like other rate limit errors
	err := fmt.Errorf("rate limited by local rate limiter")
	msg.SetError(err)
	msg.IncrementAttempts()

	if msg.CanRetry() {
		w.handleRateLimitRetry(ctx, msg, "Rate limited by local rate limiter")
	} else {
		logger := slog.Default()
		logger.WarnContext(ctx, "Worker: Message exceeded max retries on local rate limit", slog.Int("worker_id", w.config.ID), slog.String("message_id", msg.ID))
		msg.SetState(StateFailed)
	}

	w.updateMessageState(ctx, msg)
	return err
}

// handleRateLimitRetry sets up a rate-limited message for retry.
func (w *worker) handleRateLimitRetry(ctx context.Context, msg *Message, reason string) {
	retryDelay := calculateRateLimitRetryDelay(msg.Attempts)
	retryTime := time.Now().Add(retryDelay)
	msg.SetNextRetryAt(retryTime)

	logger := slog.Default()
	logger.InfoContext(ctx,
		"Worker: Message rate limited, will retry",
		slog.Int("worker_id", w.config.ID),
		slog.String("message_id", msg.ID),
		slog.String("retryAt", retryTime.Format(time.RFC3339)),
		slog.Duration("delay", retryDelay),
	)

	msg.AddStateTransition(msg.GetState(), StateRetrying, reason)
	msg.SetState(StateRetrying)
}

// updateMessageState updates the message state through the appropriate interface.
func (w *worker) updateMessageState(ctx context.Context, msg *Message) {
	if w.config.MessageQueue != nil {
		w.updateStateViaQueue(ctx, msg)
	} else if w.config.QueueManager != nil {
		w.updateStateViaManager(ctx, msg)
	}
}

// updateStateViaQueue updates state through MessageQueue interface.
func (w *worker) updateStateViaQueue(ctx context.Context, msg *Message) {
	var reason string

	state := msg.GetState()
	switch state {
	case StateRetrying:
		reason = "Rate limited"
	case StateFailed:
		reason = "Retry limit exceeded"
	case StateCompleted:
		reason = "Successfully processed"
	case StateQueued, StateProcessing, StateValidating:
		// These states are not updated via this method
		return
	}

	if err := w.config.MessageQueue.UpdateState(msg.ID, state, reason); err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx, "Failed to update state for message",
			slog.String("message_id", msg.ID),
			slog.Any("error", err))
	}
}

// updateStateViaManager updates state through QueueManager interface.
func (w *worker) updateStateViaManager(ctx context.Context, msg *Message) {
	if err := w.config.QueueManager.CompleteMessage(msg); err != nil {
		logger := slog.Default()
		logger.ErrorContext(ctx, "Failed to complete message",
			slog.String("message_id", msg.ID),
			slog.Any("error", err))
	}

	if msg.GetState() == StateRetrying {
		if err := w.config.QueueManager.Submit(msg); err != nil {
			logger := slog.Default()
			logger.ErrorContext(
				ctx,
				"Failed to re-submit message",
				slog.String("message_id", msg.ID),
				slog.Any("error", err),
			)
		}
	}
}

// handleProcessingError handles errors from message processing.
func (w *worker) handleProcessingError(ctx context.Context, msg *Message, err error) error {
	msg.SetError(err)
	msg.IncrementAttempts()

	if IsRateLimitError(err) {
		return w.handleRateLimitError(ctx, msg, err)
	}

	return w.handleGeneralError(ctx, msg, err)
}

// handleRateLimitError handles rate limit errors from LLM provider.
func (w *worker) handleRateLimitError(ctx context.Context, msg *Message, err error) error {
	logger := slog.Default()
	logger.InfoContext(ctx,
		"Worker: Message rate limited by LLM provider",
		slog.Int("worker_id", w.config.ID),
		slog.String("message_id", msg.ID),
	)

	if msg.CanRetry() {
		w.handleRateLimitRetry(ctx, msg, "Rate limited by LLM provider")
		return err
	}

	logger.WarnContext(ctx,
		"Worker: Message exceeded max retries on rate limit",
		slog.Int("worker_id", w.config.ID),
		slog.String("message_id", msg.ID),
	)
	msg.SetState(StateFailed)
	return err
}

// handleGeneralError handles non-rate-limit errors.
func (w *worker) handleGeneralError(ctx context.Context, msg *Message, err error) error {
	if msg.CanRetry() {
		retryDelay := CalculateRetryDelay(msg.Attempts)
		retryTime := time.Now().Add(retryDelay)
		msg.SetNextRetryAt(retryTime)

		logger := slog.Default()
		logger.InfoContext(ctx,
			"Worker: Message will retry",
			slog.Int("worker_id", w.config.ID),
			slog.String("message_id", msg.ID),
			slog.String("retryAt", retryTime.Format(time.RFC3339)),
			slog.Duration("delay", retryDelay),
		)

		msg.AddStateTransition(msg.GetState(), StateRetrying, fmt.Sprintf("Error: %v", err))
		msg.SetState(StateRetrying)
	} else {
		msg.SetState(StateFailed)
	}

	return err
}

// startTypingIndicator starts the typing indicator for the message.
func (w *worker) startTypingIndicator(ctx context.Context, recipient string) func() {
	if w.config.TypingIndicatorMgr != nil {
		if err := w.config.TypingIndicatorMgr.Start(ctx, recipient); err != nil {
			logger := slog.Default()
			logger.ErrorContext(ctx, "Failed to start typing indicator", slog.Any("error", err))
		}
		return func() { w.config.TypingIndicatorMgr.Stop(recipient) }
	}

	// Fallback to inline implementation
	typingCtx, typingCancel := context.WithCancel(ctx)
	go w.sendTypingIndicator(typingCtx, recipient)
	return typingCancel
}

// Process handles a single message through its lifecycle.
func (w *worker) Process(ctx context.Context, msg *Message) error {
	logger := slog.Default()
	logger.DebugContext(
		ctx,
		"Worker: In Process for message",
		slog.Int("worker_id", w.config.ID),
		slog.String("message_id", msg.ID),
	)

	// Apply rate limiting
	if err := w.handleRateLimitCheck(ctx, msg); err != nil {
		w.updateMessageState(ctx, msg)
		return err
	}

	// Start typing indicator
	stopTyping := w.startTypingIndicator(ctx, msg.SenderNumber)
	defer stopTyping()

	// Process the message
	logger.DebugContext(
		ctx,
		"Worker: Calling processMessage",
		slog.Int("worker_id", w.config.ID),
		slog.String("message_id", msg.ID),
	)
	response, err := w.processMessage(ctx, msg)
	logger.DebugContext(ctx,
		"Worker: processMessage returned",
		slog.Int("worker_id", w.config.ID),
		slog.String("message_id", msg.ID),
		slog.Any("error", err),
	)

	if err != nil {
		return w.handleProcessingError(ctx, msg, err)
	}

	// Success - save response and send
	msg.SetResponse(response)
	msg.SetState(StateCompleted)

	// Send response to user
	if sendErr := w.config.Messenger.Send(ctx, msg.SenderNumber, response); sendErr != nil {
		return fmt.Errorf("failed to send response: %w", sendErr)
	}

	// Update state
	w.updateMessageState(ctx, msg)
	return nil
}

// processMessage queries the LLM and returns the response.
func (w *worker) processMessage(ctx context.Context, msg *Message) (string, error) {
	// Create a session ID based on conversation
	sessionID := "signal-" + msg.ConversationID

	// Query the LLM
	llmResp, err := w.config.LLM.Query(ctx, msg.Text, sessionID)
	if err != nil {
		// Check if it's an authentication error
		if claude.IsAuthenticationError(err) {
			// Return a user-friendly message about authentication
			return "Claude Code authentication required. Please run the following command on the server to log in:\n\n" +
				"sudo -u signal-cli /usr/local/bin/claude-mentat /login\n\n" +
				"Once authenticated, I'll be able to respond to your messages.", nil
		}
		return "", fmt.Errorf("LLM query failed: %w", err)
	}

	return llmResp.Message, nil
}

// sendTypingIndicator sends typing indicators periodically.
func (w *worker) sendTypingIndicator(ctx context.Context, recipient string) {
	ticker := time.NewTicker(typingIndicatorInterval)
	defer ticker.Stop()

	// Send initial typing indicator
	if err := w.config.Messenger.SendTypingIndicator(ctx, recipient); err != nil {
		logger := slog.Default()
		logger.ErrorContext(
			ctx,
			"Failed to send initial typing indicator",
			slog.String("recipient", recipient),
			slog.Any("error", err),
		)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.config.Messenger.SendTypingIndicator(ctx, recipient); err != nil {
				logger := slog.Default()
				logger.ErrorContext(
					ctx,
					"Failed to send typing indicator",
					slog.String("recipient", recipient),
					slog.Any("error", err),
				)
				return
			}
		}
	}
}

// Stop gracefully stops the worker.
func (w *worker) Stop() error {
	logger := slog.Default()
	logger.InfoContext(context.Background(), "Worker stopping", slog.Int("worker_id", w.config.ID))
	return nil
}

// ID returns the worker's unique identifier.
func (w *worker) ID() string {
	return "worker-" + strconv.Itoa(w.config.ID)
}

// WorkerPool manages multiple workers.
type WorkerPool struct {
	rateLimiter RateLimiter
	queueMgr    *Manager
	workers     []Worker
	typingMgr   signal.TypingIndicatorManager
	wg          sync.WaitGroup
	mu          sync.RWMutex
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(
	size int,
	llm claude.LLM,
	messenger signal.Messenger,
	queueMgr *Manager,
	rateLimiter RateLimiter,
) *WorkerPool {
	workers := make([]Worker, size)

	// Create a shared typing indicator manager
	typingMgr := signal.NewTypingIndicatorManager(messenger)

	for i := range size {
		config := WorkerConfig{
			ID:                 i + 1,
			LLM:                llm,
			Messenger:          messenger,
			QueueManager:       queueMgr,
			RateLimiter:        rateLimiter,
			TypingIndicatorMgr: typingMgr,
		}
		workers[i] = NewWorker(config)
	}

	return &WorkerPool{
		workers:     workers,
		queueMgr:    queueMgr,
		rateLimiter: rateLimiter,
		typingMgr:   typingMgr,
	}
}

// Start begins all workers.
func (wp *WorkerPool) Start(ctx context.Context) error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	for i, worker := range wp.workers {
		wp.wg.Add(1)
		go func(w Worker, id int) {
			defer wp.wg.Done()
			if err := w.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger := slog.Default()
				logger.ErrorContext(ctx, "Worker stopped with error", slog.Int("workerID", id), slog.Any("error", err))
			}
		}(worker, i+1)
	}

	return nil
}

// Wait blocks until all workers have stopped.
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Stop gracefully stops the worker pool.
func (wp *WorkerPool) Stop() {
	// Stop all typing indicators
	if wp.typingMgr != nil {
		wp.typingMgr.StopAll()
	}
}

// Size returns the number of workers in the pool.
func (wp *WorkerPool) Size() int {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	return len(wp.workers)
}
