package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
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
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	msg, err := w.config.QueueManager.RequestMessage(reqCtx)
	if err != nil {
		if err == context.DeadlineExceeded {
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
		log.Printf("Worker %d: Shutting down, not re-submitting message %s",
			w.config.ID, msg.ID)
		return
	default:
	}

	// Complete current processing
	if err := w.config.QueueManager.CompleteMessage(msg); err != nil {
		log.Printf("Worker %d: Failed to complete message %s: %v",
			w.config.ID, msg.ID, err)
	}

	// Re-submit for retry
	if err := w.config.QueueManager.Submit(msg); err != nil {
		log.Printf("Worker %d: Failed to re-submit message %s for retry: %v",
			w.config.ID, msg.ID, err)
	}
}

// processNextMessage handles processing of a single message.
func (w *worker) processNextMessage(ctx context.Context) error {
	msg, err := w.requestMessage(ctx)
	if err != nil {
		if err == context.DeadlineExceeded {
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
	log.Printf("Worker %d: Processing message %s", w.config.ID, msg.ID)
	if err := w.Process(ctx, msg); err != nil {
		log.Printf("Worker %d error processing message %s: %v",
			w.config.ID, msg.ID, err)

		// If message is in retry state, we need to complete current processing and re-submit
		if msg.GetState() == StateRetrying {
			w.handleRetryMessage(ctx, msg)
		}
	}

	return nil
}

// Start begins processing messages. Blocks until context is canceled.
func (w *worker) Start(ctx context.Context) error {
	log.Printf("Worker %d starting", w.config.ID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d shutting down", w.config.ID)
			return ctx.Err()
		default:
			if err := w.processNextMessage(ctx); err != nil {
				return err
			}
		}
	}
}

// handleRateLimitCheck checks rate limiting and handles rate limited messages.
func (w *worker) handleRateLimitCheck(msg *Message) error {
	if w.config.RateLimiter.Allow(msg.ConversationID) {
		w.config.RateLimiter.Record(msg.ConversationID)
		return nil
	}

	// Rate limited - treat this like other rate limit errors
	err := fmt.Errorf("rate limited by local rate limiter")
	msg.SetError(err)
	msg.IncrementAttempts()

	if msg.CanRetry() {
		w.handleRateLimitRetry(msg, "Rate limited by local rate limiter")
	} else {
		log.Printf("Worker %d: Message %s exceeded max retries on local rate limit",
			w.config.ID, msg.ID)
		msg.SetState(StateFailed)
	}

	w.updateMessageState(msg)
	return err
}

// handleRateLimitRetry sets up a rate-limited message for retry.
func (w *worker) handleRateLimitRetry(msg *Message, reason string) {
	retryDelay := calculateRateLimitRetryDelay(msg.Attempts)
	retryTime := time.Now().Add(retryDelay)
	msg.SetNextRetryAt(retryTime)

	log.Printf("Worker %d: Message %s rate limited, will retry at %v (delay: %v)",
		w.config.ID, msg.ID, retryTime.Format(time.RFC3339), retryDelay)

	msg.AddStateTransition(msg.GetState(), StateRetrying, reason)
	msg.SetState(StateRetrying)
}

// updateMessageState updates the message state through the appropriate interface.
func (w *worker) updateMessageState(msg *Message) {
	if w.config.MessageQueue != nil {
		w.updateStateViaQueue(msg)
	} else if w.config.QueueManager != nil {
		w.updateStateViaManager(msg)
	}
}

// updateStateViaQueue updates state through MessageQueue interface.
func (w *worker) updateStateViaQueue(msg *Message) {
	var state MessageState
	var reason string

	switch msg.GetState() {
	case StateRetrying:
		state = MessageStateRetrying
		reason = "Rate limited"
	case StateFailed:
		state = MessageStateFailed
		reason = "Retry limit exceeded"
	case StateCompleted:
		state = MessageStateCompleted
		reason = "Successfully processed"
	default:
		return
	}

	if err := w.config.MessageQueue.UpdateState(msg.ID, state, reason); err != nil {
		log.Printf("Failed to update state for message %s: %v", msg.ID, err)
	}
}

// updateStateViaManager updates state through QueueManager interface.
func (w *worker) updateStateViaManager(msg *Message) {
	if err := w.config.QueueManager.CompleteMessage(msg); err != nil {
		log.Printf("Failed to complete message %s: %v", msg.ID, err)
	}

	if msg.GetState() == StateRetrying {
		if err := w.config.QueueManager.Submit(msg); err != nil {
			log.Printf("Failed to re-submit message %s: %v", msg.ID, err)
		}
	}
}

// handleProcessingError handles errors from message processing.
func (w *worker) handleProcessingError(msg *Message, err error) error {
	msg.SetError(err)
	msg.IncrementAttempts()

	if IsRateLimitError(err) {
		return w.handleRateLimitError(msg, err)
	}

	return w.handleGeneralError(msg, err)
}

// handleRateLimitError handles rate limit errors from LLM provider.
func (w *worker) handleRateLimitError(msg *Message, err error) error {
	log.Printf("Worker %d: Message %s rate limited by LLM provider",
		w.config.ID, msg.ID)

	if msg.CanRetry() {
		w.handleRateLimitRetry(msg, "Rate limited by LLM provider")
		return err
	}

	log.Printf("Worker %d: Message %s exceeded max retries on rate limit",
		w.config.ID, msg.ID)
	msg.SetState(StateFailed)
	return err
}

// handleGeneralError handles non-rate-limit errors.
func (w *worker) handleGeneralError(msg *Message, err error) error {
	if msg.CanRetry() {
		retryDelay := CalculateRetryDelay(msg.Attempts)
		retryTime := time.Now().Add(retryDelay)
		msg.SetNextRetryAt(retryTime)

		log.Printf("Worker %d: Message %s will retry at %v (delay: %v)",
			w.config.ID, msg.ID, retryTime.Format(time.RFC3339), retryDelay)

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
			log.Printf("Failed to start typing indicator: %v", err)
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
	log.Printf("Worker %d: In Process for message %s", w.config.ID, msg.ID)

	// Apply rate limiting
	if err := w.handleRateLimitCheck(msg); err != nil {
		w.updateMessageState(msg)
		return err
	}

	// Start typing indicator
	stopTyping := w.startTypingIndicator(ctx, msg.SenderNumber)
	defer stopTyping()

	// Process the message
	log.Printf("Worker %d: Calling processMessage for %s", w.config.ID, msg.ID)
	response, err := w.processMessage(ctx, msg)
	log.Printf("Worker %d: processMessage returned for %s, err=%v", w.config.ID, msg.ID, err)

	if err != nil {
		return w.handleProcessingError(msg, err)
	}

	// Success - save response and send
	msg.SetResponse(response)
	msg.SetState(StateCompleted)

	// Send response to user
	if err := w.config.Messenger.Send(ctx, msg.SenderNumber, response); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}

	// Update state
	w.updateMessageState(msg)
	return nil
}

// processMessage queries the LLM and returns the response.
func (w *worker) processMessage(ctx context.Context, msg *Message) (string, error) {
	// Create a session ID based on conversation
	sessionID := fmt.Sprintf("signal-%s", msg.ConversationID)

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
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Send initial typing indicator
	if err := w.config.Messenger.SendTypingIndicator(ctx, recipient); err != nil {
		log.Printf("Failed to send initial typing indicator to %s: %v", recipient, err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.config.Messenger.SendTypingIndicator(ctx, recipient); err != nil {
				log.Printf("Failed to send typing indicator to %s: %v", recipient, err)
				return
			}
		}
	}
}

// Stop gracefully stops the worker.
func (w *worker) Stop() error {
	log.Printf("Worker %d stopping", w.config.ID)
	return nil
}

// ID returns the worker's unique identifier.
func (w *worker) ID() string {
	return fmt.Sprintf("worker-%d", w.config.ID)
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
func NewWorkerPool(size int, llm claude.LLM, messenger signal.Messenger, queueMgr *Manager, rateLimiter RateLimiter) *WorkerPool {
	workers := make([]Worker, size)

	// Create a shared typing indicator manager
	typingMgr := signal.NewTypingIndicatorManager(messenger)

	for i := 0; i < size; i++ {
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
			if err := w.Start(ctx); err != nil && err != context.Canceled {
				log.Printf("Worker %d stopped with error: %v", id, err)
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
