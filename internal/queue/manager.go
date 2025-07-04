package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultIncomingChannelSize is the default buffer size for incoming messages.
	DefaultIncomingChannelSize = 100

	// DefaultRequestChannelSize is the default buffer size for worker requests.
	DefaultRequestChannelSize = 10

	// submitTimeout is the timeout for submitting messages to the queue.
	submitTimeout = 5 * time.Second
	// shutdownCheckDelay is the delay to check if Start() was called.
	shutdownCheckDelay = 100 * time.Millisecond
)

// Manager orchestrates multiple conversation queues with fair scheduling.
// It implements a round-robin algorithm with conversation affinity to ensure:
//   - Fair processing across all conversations (no starvation)
//   - Only one message per conversation is processed at a time
//   - Messages within a conversation are processed in order
//   - Conversations with more messages don't starve those with fewer
//
// The scheduling algorithm works as follows:
//  1. Conversations are tracked in a slice maintaining their arrival order
//  2. A currentIndex rotates through conversations in round-robin fashion
//  3. Each conversation can have at most one message in processing state
//  4. Within each conversation, messages are processed in FIFO order
//  5. Empty conversations are automatically cleaned up
type Manager struct {
	stateMachine      StateMachine
	queues            map[string]*ConversationQueue // conversation ID -> queue
	incomingCh        chan *Message                 // channel for new messages
	requestCh         chan chan *Message            // channel for worker requests
	conversationOrder []string                      // ordered list of conversation IDs for fair scheduling
	waitingWorkers    []chan *Message               // workers waiting for messages
	wg                sync.WaitGroup                // tracks active goroutines
	currentIndex      int                           // current position in round-robin
	mu                sync.RWMutex                  // protects shared state
	shutdown          bool                          // indicates shutdown in progress
	started           chan struct{}                 // signals that Start() has begun
	stopCh            chan struct{}                 // channel to signal shutdown
}

// NewManager creates a new queue manager.
func NewManager(_ context.Context) *Manager {
	return &Manager{
		queues:            make(map[string]*ConversationQueue),
		stateMachine:      NewStateMachine(),
		incomingCh:        make(chan *Message, DefaultIncomingChannelSize),
		requestCh:         make(chan chan *Message, DefaultRequestChannelSize),
		waitingWorkers:    make([]chan *Message, 0),
		conversationOrder: make([]string, 0),
		started:           make(chan struct{}),
		stopCh:            make(chan struct{}),
	}
}

// Start begins processing messages. This should be called in a goroutine.
func (m *Manager) Start(ctx context.Context) {
	m.wg.Add(1)
	defer m.wg.Done()
	defer m.cleanup()

	// Signal that we've started
	close(m.started)

	logger := slog.Default()
	logger.InfoContext(ctx, "Queue manager started and processing")

	for {
		select {
		case <-ctx.Done():
			logger.WarnContext(ctx, "Queue manager stopping: context done")
			return
		case <-m.stopCh:
			logger.WarnContext(ctx, "Queue manager stopping: stop signal")
			return

		case msg := <-m.incomingCh:
			logger.InfoContext(ctx, "Queue manager received message",
				slog.String("id", msg.ID),
				slog.String("from", msg.SenderNumber))
			// Add message to appropriate queue
			if err := m.enqueue(msg); err != nil {
				msg.SetError(err)
				msg.SetState(StateFailed)
			} else {
				// Try to dispatch to waiting worker
				m.tryDispatch(ctx)
			}

		case workerCh := <-m.requestCh:
			// Worker is requesting a message
			logger.InfoContext(ctx, "Queue manager received worker request")
			msg := m.getNextMessage()
			logger.DebugContext(ctx, "Queue manager getNextMessage returned",
				slog.Bool("is_nil", msg == nil))
			if msg != nil {
				logger.InfoContext(ctx, "Dispatching message to worker",
					slog.String("id", msg.ID))
				// Transition to processing state
				if err := m.stateMachine.Transition(msg, StateProcessing); err != nil {
					logger.ErrorContext(ctx, "Failed to transition message to processing",
						slog.String("id", msg.ID),
						slog.Any("error", err))
				}
				workerCh <- msg
			} else {
				// No messages available, add to waiting list
				logger.InfoContext(ctx, "No messages available, adding worker to waiting list")
				m.mu.Lock()
				m.waitingWorkers = append(m.waitingWorkers, workerCh)
				logger.InfoContext(ctx, "Current waiting workers count",
					slog.Int("count", len(m.waitingWorkers)))
				m.mu.Unlock()
				logger.DebugContext(ctx, "Worker added to waiting list and will block until message available")
			}
		}
	}
}

// WaitForReady blocks until the queue manager has started its event loop.
func (m *Manager) WaitForReady() {
	<-m.started
}

// Submit adds a message to the queue for processing.
func (m *Manager) Submit(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("cannot submit nil message")
	}

	// Check if shutting down
	m.mu.RLock()
	if m.shutdown {
		m.mu.RUnlock()
		return fmt.Errorf("queue manager shutting down")
	}
	m.mu.RUnlock()

	// Handle state based on current state
	currentState := msg.GetState()
	switch currentState {
	case StateRetrying:
		// Only transition to queued if the message is ready for retry
		if msg.IsReadyForRetry() {
			if err := m.stateMachine.Transition(msg, StateQueued); err != nil {
				// If transition fails, force it to queued
				msg.SetState(StateQueued)
			}
		}
		// Otherwise keep it in StateRetrying with its NextRetryAt time
	case "":
		// New message, set initial state
		if err := m.stateMachine.Transition(msg, StateQueued); err != nil {
			msg.SetState(StateQueued)
		}
	case StateQueued, StateProcessing, StateValidating, StateCompleted, StateFailed:
		// For other states, keep the current state
	}

	select {
	case m.incomingCh <- msg:
		return nil
	case <-m.stopCh:
		return fmt.Errorf("queue manager shutting down")
	case <-time.After(submitTimeout):
		return fmt.Errorf("timeout submitting message")
	}
}

// RequestMessage is called by workers to get the next message to process.
// It implements a pull-based model where workers request work when ready.
// If no message is immediately available, the worker's request is queued
// and will be fulfilled when a message becomes available.
// The method respects the context timeout/cancellation.
func (m *Manager) RequestMessage(ctx context.Context) (*Message, error) {
	logger := slog.Default()
	logger.DebugContext(ctx, "Worker requesting message")

	respCh := make(chan *Message, 1)

	select {
	case m.requestCh <- respCh:
		// Request sent
		logger.DebugContext(ctx, "Worker request sent to queue manager")
	case <-ctx.Done():
		return nil, fmt.Errorf("context canceled while sending request: %w", ctx.Err())
	case <-m.stopCh:
		return nil, fmt.Errorf("queue manager shutting down")
	}

	select {
	case msg := <-respCh:
		// Message state transition is handled by Start() method when dispatching
		if msg != nil {
			logger.DebugContext(ctx, "Worker received message",
				slog.String("message_id", msg.ID))
		} else {
			logger.ErrorContext(ctx, "Worker received nil message - this should not happen!")
		}
		return msg, nil
	case <-ctx.Done():
		logger.DebugContext(ctx, "Worker request canceled by context")
		return nil, fmt.Errorf("context canceled while waiting for message: %w", ctx.Err())
	case <-m.stopCh:
		logger.DebugContext(ctx, "Worker request canceled by queue shutdown")
		return nil, fmt.Errorf("queue manager shutting down")
	}
}

// CompleteMessage marks a message as done processing.
func (m *Manager) CompleteMessage(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("cannot complete nil message")
	}

	m.mu.Lock()
	queue, exists := m.queues[msg.ConversationID]
	if !exists {
		m.mu.Unlock()
		// Queue already cleaned up, which is fine
		return nil
	}
	m.mu.Unlock()

	queue.Complete()

	// Don't clean up the queue here - let getNextMessage handle cleanup
	// to avoid race conditions where the queue is deleted while still
	// being referenced elsewhere

	return nil
}

// RetryMessage marks a message for retry and re-enqueues it.
func (m *Manager) RetryMessage(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("cannot retry nil message")
	}

	m.mu.Lock()
	queue, exists := m.queues[msg.ConversationID]
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("no queue found for conversation %s", msg.ConversationID)
	}

	// Mark the current processing as complete
	queue.Complete()

	// Transition to queued state for re-processing
	if err := m.stateMachine.Transition(msg, StateQueued); err != nil {
		// If transition fails, force it to queued
		msg.SetState(StateQueued)
	}

	// Re-submit the message
	return m.Submit(msg)
}

// Shutdown gracefully stops the queue manager.
func (m *Manager) Shutdown(timeout time.Duration) error {
	// Mark as shutting down
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return nil
	}
	m.shutdown = true
	m.mu.Unlock()

	// Signal shutdown
	close(m.stopCh)

	// Wait for Start() to have been called
	select {
	case <-m.started:
		// Start() was called, proceed with shutdown
	case <-time.After(shutdownCheckDelay):
		// Start() was never called, nothing to shut down
		return nil
	}

	// Create done channel before starting goroutine to avoid race
	done := make(chan struct{})

	// Start goroutine to signal completion
	go func() {
		m.wg.Wait()
		close(done)
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("shutdown timeout after %v", timeout)
	}
}

// enqueue adds a message to the appropriate conversation queue.
func (m *Manager) enqueue(msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[msg.ConversationID]
	if !exists {
		queue = NewConversationQueue(msg.ConversationID)
		m.queues[msg.ConversationID] = queue
		// Add to conversation order for fair scheduling
		m.conversationOrder = append(m.conversationOrder, msg.ConversationID)
	}

	return queue.Enqueue(msg)
}

// getNextMessage implements fair scheduling across conversations.
// It minimizes lock holding time by taking snapshots and releasing locks during queue operations.
//
// The algorithm ensures fairness by:
//   - Using round-robin to cycle through all conversations
//   - Skipping conversations that already have a message in processing
//   - Processing messages within each conversation in FIFO order
//   - Cleaning up empty conversation queues automatically
//
// Lock optimization strategy:
//   - Takes a snapshot of conversation order to avoid holding lock during iteration
//   - Releases lock while checking individual queue states
//   - Only holds lock when modifying shared state (queues map, conversation order)
//   - Uses double-checking pattern for queue cleanup to prevent race conditions
func (m *Manager) getNextMessage() *Message {
	// Take a snapshot of conversations to check
	snapshot, startIndex := m.getConversationSnapshot()
	if len(snapshot) == 0 {
		return nil
	}

	// Try each conversation in round-robin order
	for i := range snapshot {
		// Calculate index with wraparound
		idx := (startIndex + i) % len(snapshot)
		convID := snapshot[idx]

		// Try to get a message from this conversation
		if msg := m.tryGetMessageFromConversation(convID); msg != nil {
			return msg
		}
	}

	// Update index even if no message found
	m.updateCurrentIndex(startIndex, len(snapshot))
	return nil
}

// getConversationSnapshot creates a snapshot of conversation order.
func (m *Manager) getConversationSnapshot() ([]string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.conversationOrder) == 0 {
		return nil, 0
	}

	// Create a snapshot of conversation order to avoid holding lock during iteration
	snapshot := make([]string, len(m.conversationOrder))
	copy(snapshot, m.conversationOrder)
	return snapshot, m.currentIndex
}

// tryGetMessageFromConversation attempts to get a message from a specific conversation.
func (m *Manager) tryGetMessageFromConversation(convID string) *Message {
	// Get queue reference
	queue := m.getQueueForConversation(convID)
	if queue == nil {
		return nil
	}

	// Check if already processing (without lock)
	if queue.IsProcessing() {
		return nil
	}

	// Try to dequeue (queue has its own locking)
	msg := queue.Dequeue()
	if msg != nil {
		m.updateIndexAfterDequeue(convID)
		return msg
	}

	// Check if queue should be cleaned up
	m.cleanupEmptyQueue(convID, queue)
	return nil
}

// getQueueForConversation gets the queue for a conversation, handling missing queues.
func (m *Manager) getQueueForConversation(convID string) *ConversationQueue {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[convID]
	if !exists {
		// Queue was removed, clean it from order
		m.removeConversationFromOrder(convID)
		return nil
	}
	return queue
}

// updateIndexAfterDequeue updates the current index after successfully dequeuing.
func (m *Manager) updateIndexAfterDequeue(convID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find the actual current position of this conversation
	for i, id := range m.conversationOrder {
		if id == convID {
			// Move to next conversation
			m.currentIndex = (i + 1) % len(m.conversationOrder)
			return
		}
	}
}

// cleanupEmptyQueue removes empty queues that are not processing.
func (m *Manager) cleanupEmptyQueue(convID string, queue *ConversationQueue) {
	if !queue.IsEmpty() || queue.IsProcessing() {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check queue is still empty after acquiring lock
	if queue.IsEmpty() && !queue.IsProcessing() {
		delete(m.queues, convID)
		m.removeConversationFromOrder(convID)
	}
}

// updateCurrentIndex updates the current index after a full round-robin cycle.
func (m *Manager) updateCurrentIndex(startIndex, snapshotLen int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.conversationOrder) > 0 {
		m.currentIndex = (startIndex + snapshotLen) % len(m.conversationOrder)
		if m.currentIndex >= len(m.conversationOrder) {
			m.currentIndex = 0
		}
	} else {
		m.currentIndex = 0
	}
}

// removeConversationFromOrder removes a conversation from the order slice.
// Must be called with m.mu held.
func (m *Manager) removeConversationFromOrder(convID string) {
	for i, id := range m.conversationOrder {
		if id == convID {
			m.conversationOrder = append(
				m.conversationOrder[:i],
				m.conversationOrder[i+1:]...,
			)
			if m.currentIndex > i {
				m.currentIndex--
			} else if m.currentIndex >= len(m.conversationOrder) && len(m.conversationOrder) > 0 {
				m.currentIndex = 0
			}
			break
		}
	}
}

// tryDispatch attempts to send a message to a waiting worker.
func (m *Manager) tryDispatch(ctx context.Context) {
	logger := slog.Default()
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.waitingWorkers) == 0 {
		return
	}

	logger.InfoContext(ctx, "Trying to dispatch to waiting workers",
		slog.Int("waiting_count", len(m.waitingWorkers)))

	msg := m.getNextMessageLocked()
	if msg == nil {
		logger.InfoContext(ctx, "No messages available to dispatch")
		return
	}

	// Send to first waiting worker
	workerCh := m.waitingWorkers[0]
	m.waitingWorkers = m.waitingWorkers[1:]

	logger.InfoContext(ctx, "Dispatching message to waiting worker",
		slog.String("message_id", msg.ID),
		slog.Int("remaining_waiting", len(m.waitingWorkers)))

	// Transition to processing state
	if err := m.stateMachine.Transition(msg, StateProcessing); err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to transition message to processing",
			slog.String("id", msg.ID),
			slog.Any("error", err),
		)
	}

	// Send message (non-blocking in case worker gave up)
	select {
	case workerCh <- msg:
	default:
		// Worker gave up, requeue the message
		msg.SetState(StateQueued)
		if queue, exists := m.queues[msg.ConversationID]; exists {
			queue.Complete() // Reset processing state
			if err := queue.Enqueue(msg); err != nil {
				logger.ErrorContext(ctx, "Failed to requeue message", slog.String("id", msg.ID), slog.Any("error", err))
			}
		}
	}
}

// getNextMessageLocked is like getNextMessage but assumes lock is held.
// It's used internally by tryDispatch and other methods that already hold the lock.
func (m *Manager) getNextMessageLocked() *Message {
	if len(m.conversationOrder) == 0 {
		return nil
	}

	// Try each conversation in round-robin order
	attemptsLeft := len(m.conversationOrder)
	for attemptsLeft > 0 {
		// Wrap around if needed
		if m.currentIndex >= len(m.conversationOrder) {
			m.currentIndex = 0
		}

		convID := m.conversationOrder[m.currentIndex]
		queue, exists := m.queues[convID]

		if !exists {
			// Remove from order and adjust index
			m.removeConversationFromOrder(convID)
			attemptsLeft--
			continue
		}

		// Skip if already processing
		if queue.IsProcessing() {
			m.currentIndex++ // Move to next conversation
			attemptsLeft--
			continue
		}

		msg := queue.Dequeue()
		if msg != nil {
			// Move to next conversation for fairness only after successful dequeue
			m.currentIndex++
			return msg
		}

		// No message available, move to next conversation
		m.currentIndex++

		// Don't clean up queues while processing - CompleteMessage needs them
		// Only clean up if empty AND not processing
		if queue.IsEmpty() && !queue.IsProcessing() {
			// Remove from queues
			delete(m.queues, convID)
			m.removeConversationFromOrder(convID)
		}

		attemptsLeft--
	}

	return nil
}

// cleanup releases resources when shutting down.
func (m *Manager) cleanup() {
	logger := slog.Default()
	logger.Warn("Queue manager cleanup called - sending nil to waiting workers")

	m.mu.Lock()
	defer m.mu.Unlock()

	// Send nil to all waiting workers to unblock them
	logger.Warn("Sending nil to waiting workers", slog.Int("count", len(m.waitingWorkers)))
	for _, workerCh := range m.waitingWorkers {
		select {
		case workerCh <- nil:
		default:
		}
	}
	m.waitingWorkers = nil

	// Don't close channels here - let them be garbage collected
	// Closing them can cause "send on closed channel" panics
	// The context cancellation is sufficient to stop all operations
}

// Stats returns current queue statistics.
func (m *Manager) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)
	totalQueued := 0
	totalProcessing := 0

	for _, queue := range m.queues {
		totalQueued += queue.Size()
		if queue.IsProcessing() {
			totalProcessing++
		}
	}

	stats["conversations"] = len(m.queues)
	stats["queued"] = totalQueued
	stats["processing"] = totalProcessing
	stats["waiting_workers"] = len(m.waitingWorkers)

	return stats
}
