package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/joshsymonds/mentat/internal/claude"
	"github.com/joshsymonds/mentat/internal/signal"
)

// WorkerConfig holds configuration for a worker.
type WorkerConfig struct {
	LLM                   claude.LLM
	Messenger             signal.Messenger
	RateLimiter           RateLimiter
	QueueManager          *Manager
	TypingIndicatorMgr    signal.TypingIndicatorManager
	ID                    int
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

// Start begins processing messages. Blocks until context is canceled.
func (w *worker) Start(ctx context.Context) error {
	log.Printf("Worker %d starting", w.config.ID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d shutting down", w.config.ID)
			return ctx.Err()
		default:
			// Request a message from the queue
			reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			msg, err := w.config.QueueManager.RequestMessage(reqCtx)
			cancel()

			if err != nil {
				if err == context.DeadlineExceeded {
					// No messages available, continue
					continue
				}
				// Context canceled or other error
				return err
			}

			if msg == nil {
				// Queue manager is shutting down
				return nil
			}

			// Process the message
			if err := w.Process(ctx, msg); err != nil {
				log.Printf("Worker %d error processing message %s: %v",
					w.config.ID, msg.ID, err)
			}
		}
	}
}

// Process handles a single message through its lifecycle.
func (w *worker) Process(ctx context.Context, msg *Message) error {
	// Apply rate limiting
	if !w.config.RateLimiter.Allow(msg.ConversationID) {
		return fmt.Errorf("rate limited")
	}
	w.config.RateLimiter.Record(msg.ConversationID)

	// Start typing indicator
	if w.config.TypingIndicatorMgr != nil {
		if err := w.config.TypingIndicatorMgr.Start(ctx, msg.Sender); err != nil {
			log.Printf("Failed to start typing indicator: %v", err)
		}
		defer w.config.TypingIndicatorMgr.Stop(msg.Sender)
	} else {
		// Fallback to inline implementation
		typingCtx, typingCancel := context.WithCancel(ctx)
		defer typingCancel()
		go w.sendTypingIndicator(typingCtx, msg.Sender)
	}

	// Process the message
	response, err := w.processMessage(ctx, msg)

	if err != nil {
		msg.SetError(err)

		// Check if we can retry
		if msg.CanRetry() {
			// Set retrying state first
			if transitionErr := w.config.QueueManager.stateMachine.Transition(msg, StateRetrying); transitionErr != nil {
				// If transition fails, just set the state directly
				msg.SetState(StateRetrying)
			}
			// Don't re-submit, the queue manager should handle retries
		} else {
			if transitionErr := w.config.QueueManager.stateMachine.Transition(msg, StateFailed); transitionErr != nil {
				msg.SetState(StateFailed)
			}
		}

		return err
	}

	// Success - save response and send
	msg.SetResponse(response)
	if err := w.config.QueueManager.stateMachine.Transition(msg, StateCompleted); err != nil {
		msg.SetState(StateCompleted)
	}

	// Send response to user
	if err := w.config.Messenger.Send(ctx, msg.Sender, response); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}

	// Mark message as complete in queue
	if err := w.config.QueueManager.CompleteMessage(msg); err != nil {
		log.Printf("Failed to complete message %s: %v", msg.ID, err)
	}

	return nil
}

// processMessage queries the LLM and returns the response.
func (w *worker) processMessage(ctx context.Context, msg *Message) (string, error) {
	// Create a session ID based on conversation
	sessionID := fmt.Sprintf("signal-%s", msg.ConversationID)

	// Query the LLM
	llmResp, err := w.config.LLM.Query(ctx, msg.Text, sessionID)
	if err != nil {
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
		log.Printf("Failed to send typing indicator: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.config.Messenger.SendTypingIndicator(ctx, recipient); err != nil {
				log.Printf("Failed to send typing indicator: %v", err)
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
