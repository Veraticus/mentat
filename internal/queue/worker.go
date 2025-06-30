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
	LLM                   claude.LLM
	Messenger             signal.Messenger
	RateLimiter           RateLimiter
	QueueManager          *Manager
	MessageQueue          MessageQueue  // For updating state and getting messages
	TypingIndicatorMgr    signal.TypingIndicatorManager
	ActivityRecorder      ActivityRecorder
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

			// Process the message (message is already in processing state from Manager)
			log.Printf("Worker %d: Processing message %s", w.config.ID, msg.ID)
			if err := w.Process(ctx, msg); err != nil {
				log.Printf("Worker %d error processing message %s: %v",
					w.config.ID, msg.ID, err)
				
				// If message is in retry state, we need to complete current processing and re-submit
				if msg.GetState() == StateRetrying {
					// Check if we're shutting down before trying to re-submit
					select {
					case <-ctx.Done():
						log.Printf("Worker %d: Shutting down, not re-submitting message %s",
							w.config.ID, msg.ID)
						return ctx.Err()
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
			}
		}
	}
}

// Process handles a single message through its lifecycle.
func (w *worker) Process(ctx context.Context, msg *Message) error {
	// Record activity for health monitoring
	if w.config.ActivityRecorder != nil {
		w.config.ActivityRecorder.RecordActivity(w.ID())
	}
	
	log.Printf("Worker %d: In Process for message %s", w.config.ID, msg.ID)
	
	// Apply rate limiting
	if !w.config.RateLimiter.Allow(msg.ConversationID) {
		// Rate limited - treat this like other rate limit errors
		err := fmt.Errorf("rate limited by local rate limiter")
		msg.SetError(err)
		msg.IncrementAttempts()
		
		if msg.CanRetry() {
			// Calculate retry delay
			retryDelay := calculateRateLimitRetryDelay(msg.Attempts)
			retryTime := time.Now().Add(retryDelay)
			msg.SetNextRetryAt(retryTime)
			
			log.Printf("Worker %d: Message %s rate limited locally, will retry at %v (delay: %v)",
				w.config.ID, msg.ID, retryTime.Format(time.RFC3339), retryDelay)
			
			msg.AddStateTransition(msg.GetState(), StateRetrying, "Rate limited by local rate limiter")
			msg.SetState(StateRetrying)
		} else {
			log.Printf("Worker %d: Message %s exceeded max retries on local rate limit",
				w.config.ID, msg.ID)
			msg.SetState(StateFailed)
		}
		
		// Update state through MessageQueue interface
		if w.config.MessageQueue != nil {
			if msg.GetState() == StateRetrying {
				if updateErr := w.config.MessageQueue.UpdateState(msg.ID, MessageStateRetrying, "Rate limited by local rate limiter"); updateErr != nil {
					log.Printf("Failed to update state for rate-limited message %s: %v", msg.ID, updateErr)
				}
			} else {
				if updateErr := w.config.MessageQueue.UpdateState(msg.ID, MessageStateFailed, "Rate limit retry limit exceeded"); updateErr != nil {
					log.Printf("Failed to update state for failed message %s: %v", msg.ID, updateErr)
				}
			}
		} else {
			// Fallback to direct manager calls
			if completeErr := w.config.QueueManager.CompleteMessage(msg); completeErr != nil {
				log.Printf("Failed to complete rate-limited message %s: %v", msg.ID, completeErr)
			}
			
			if msg.GetState() == StateRetrying {
				if submitErr := w.config.QueueManager.Submit(msg); submitErr != nil {
					log.Printf("Failed to re-submit rate-limited message %s: %v", msg.ID, submitErr)
				}
			}
		}
		
		return err
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
	log.Printf("Worker %d: Calling processMessage for %s", w.config.ID, msg.ID)
	response, err := w.processMessage(ctx, msg)
	log.Printf("Worker %d: processMessage returned for %s, err=%v", w.config.ID, msg.ID, err)

	if err != nil {
		msg.SetError(err)
		msg.IncrementAttempts()

		// Check if this is a rate limit error
		if IsRateLimitError(err) {
			log.Printf("Worker %d: Message %s rate limited by LLM provider",
				w.config.ID, msg.ID)
			
			// Always retry rate-limited requests if we have attempts left
			if msg.CanRetry() {
				// Calculate retry delay for rate limit
				retryDelay := calculateRateLimitRetryDelay(msg.Attempts)
				retryTime := time.Now().Add(retryDelay)
				msg.SetNextRetryAt(retryTime)
				
				log.Printf("Worker %d: Message %s will retry at %v (delay: %v)",
					w.config.ID, msg.ID, retryTime.Format(time.RFC3339), retryDelay)
				
				// Mark as retrying with rate limit reason
				msg.AddStateTransition(msg.GetState(), StateRetrying, "Rate limited by LLM provider")
				msg.SetState(StateRetrying)
				
				// Update state through MessageQueue interface
				if w.config.MessageQueue != nil {
					if updateErr := w.config.MessageQueue.UpdateState(msg.ID, MessageStateRetrying, "Rate limited by LLM provider"); updateErr != nil {
						log.Printf("Failed to update state for rate-limited message %s: %v", msg.ID, updateErr)
					}
					return err
				}
			} else {
				// Max retries exceeded even for rate limit
				log.Printf("Worker %d: Message %s exceeded max retries on rate limit",
					w.config.ID, msg.ID)
				msg.SetState(StateFailed)
				
				// Update state through MessageQueue interface
				if w.config.MessageQueue != nil {
					if updateErr := w.config.MessageQueue.UpdateState(msg.ID, MessageStateFailed, "Rate limit retry limit exceeded"); updateErr != nil {
						log.Printf("Failed to update state for failed message %s: %v", msg.ID, updateErr)
					}
					return err
				}
			}
		} else {
			// Non-rate-limit error - check if we can retry
			if msg.CanRetry() {
				// Calculate standard retry delay
				retryDelay := CalculateRetryDelay(msg.Attempts)
				retryTime := time.Now().Add(retryDelay)
				msg.SetNextRetryAt(retryTime)
				
				log.Printf("Worker %d: Message %s will retry at %v (delay: %v)",
					w.config.ID, msg.ID, retryTime.Format(time.RFC3339), retryDelay)
				
				msg.AddStateTransition(msg.GetState(), StateRetrying, fmt.Sprintf("Error: %v", err))
				msg.SetState(StateRetrying)
				
				// Update state through MessageQueue interface
				if w.config.MessageQueue != nil {
					if updateErr := w.config.MessageQueue.UpdateState(msg.ID, MessageStateRetrying, fmt.Sprintf("Error: %v", err)); updateErr != nil {
						log.Printf("Failed to update state for retrying message %s: %v", msg.ID, updateErr)
					}
				}
			} else {
				msg.SetState(StateFailed)
				
				// Update state through MessageQueue interface
				if w.config.MessageQueue != nil {
					if updateErr := w.config.MessageQueue.UpdateState(msg.ID, MessageStateFailed, fmt.Sprintf("Max retries exceeded: %v", err)); updateErr != nil {
						log.Printf("Failed to update state for failed message %s: %v", msg.ID, updateErr)
					}
				}
			}
		}

		return err
	}

	// Success - save response and send
	msg.SetResponse(response)
	msg.SetState(StateCompleted)

	// Send response to user
	if err := w.config.Messenger.Send(ctx, msg.Sender, response); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}

	// Mark message as complete in queue
	if w.config.MessageQueue != nil {
		// Use MessageQueue interface to update state (which updates stats)
		if err := w.config.MessageQueue.UpdateState(msg.ID, MessageStateCompleted, "Successfully processed"); err != nil {
			log.Printf("Failed to update message state to completed %s: %v", msg.ID, err)
		}
	} else if w.config.QueueManager != nil {
		// Fallback to direct manager call
		if err := w.config.QueueManager.CompleteMessage(msg); err != nil {
			log.Printf("Failed to complete message %s: %v", msg.ID, err)
		}
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
