package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// WorkerConfig holds configuration for a worker.
type WorkerConfig struct {
	ID           int
	LLM          LLM
	Messenger    Messenger
	QueueManager *Manager
	RateLimiter  RateLimiter
}

// worker processes messages from the queue.
type worker struct {
	config      WorkerConfig
	stateMachine StateMachine
}

// NewWorker creates a new worker instance.
func NewWorker(config WorkerConfig) Worker {
	return &worker{
		config:       config,
		stateMachine: NewStateMachine(),
	}
}

// Start begins processing messages. Blocks until context is cancelled.
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
				// Context cancelled or other error
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
	if err := w.config.RateLimiter.Wait(ctx, msg.ConversationID); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}
	
	// Start typing indicator
	typingCtx, typingCancel := context.WithCancel(ctx)
	defer typingCancel()
	
	go w.sendTypingIndicator(typingCtx, msg.Sender)
	
	// Process the message
	response, err := w.processMessage(ctx, msg)
	
	if err != nil {
		msg.SetError(err)
		
		// Check if we can retry
		if msg.CanRetry() {
			// Set retrying state first
			if err := w.config.QueueManager.stateMachine.Transition(msg, StateRetrying); err != nil {
				// If transition fails, just set the state directly
				msg.SetState(StateRetrying)
			}
			// Don't re-submit, the queue manager should handle retries
		} else {
			if err := w.config.QueueManager.stateMachine.Transition(msg, StateFailed); err != nil {
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
	w.config.QueueManager.CompleteMessage(msg)
	
	return nil
}

// processMessage queries the LLM and returns the response.
func (w *worker) processMessage(ctx context.Context, msg *Message) (string, error) {
	// Create a session ID based on conversation
	sessionID := fmt.Sprintf("signal-%s", msg.ConversationID)
	
	// Query the LLM
	response, err := w.config.LLM.Query(ctx, msg.Text, sessionID)
	if err != nil {
		return "", fmt.Errorf("LLM query failed: %w", err)
	}
	
	return response, nil
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

// WorkerPool manages multiple workers.
type WorkerPool struct {
	workers     []Worker
	queueMgr    *Manager
	rateLimiter RateLimiter
	wg          sync.WaitGroup
	mu          sync.RWMutex
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(size int, llm LLM, messenger Messenger, queueMgr *Manager, rateLimiter RateLimiter) *WorkerPool {
	workers := make([]Worker, size)
	
	for i := 0; i < size; i++ {
		config := WorkerConfig{
			ID:           i + 1,
			LLM:          llm,
			Messenger:    messenger,
			QueueManager: queueMgr,
			RateLimiter:  rateLimiter,
		}
		workers[i] = NewWorker(config)
	}
	
	return &WorkerPool{
		workers:     workers,
		queueMgr:    queueMgr,
		rateLimiter: rateLimiter,
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

// Size returns the number of workers in the pool.
func (wp *WorkerPool) Size() int {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	return len(wp.workers)
}