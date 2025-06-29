package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Manager orchestrates multiple conversation queues with fair scheduling.
type Manager struct {
	ctx               context.Context
	stateMachine      StateMachine
	cancel            context.CancelFunc
	queues            map[string]*ConversationQueue
	incomingCh        chan *Message
	requestCh         chan chan *Message
	conversationOrder []string
	waitingWorkers    []chan *Message
	wg                sync.WaitGroup
	currentIndex      int
	mu                sync.RWMutex
	shutdown          bool
	started           chan struct{} // Signals that Start() has begun
}

// NewManager creates a new queue manager.
func NewManager(ctx context.Context) *Manager {
	ctx, cancel := context.WithCancel(ctx)

	return &Manager{
		queues:            make(map[string]*ConversationQueue),
		stateMachine:      NewStateMachine(),
		incomingCh:        make(chan *Message, 100),
		requestCh:         make(chan chan *Message, 10),
		waitingWorkers:    make([]chan *Message, 0),
		conversationOrder: make([]string, 0),
		ctx:               ctx,
		cancel:            cancel,
		started:           make(chan struct{}),
	}
}

// Start begins processing messages. This should be called in a goroutine.
func (m *Manager) Start() {
	m.wg.Add(1)
	defer m.wg.Done()
	defer m.cleanup()
	
	// Signal that we've started
	close(m.started)

	for {
		select {
		case <-m.ctx.Done():
			return

		case msg := <-m.incomingCh:
			// Add message to appropriate queue
			if err := m.enqueue(msg); err != nil {
				msg.SetError(err)
				msg.SetState(StateFailed)
			} else {
				// Try to dispatch to waiting worker
				m.tryDispatch()
			}

		case workerCh := <-m.requestCh:
			// Worker is requesting a message
			msg := m.getNextMessage()
			if msg != nil {
				workerCh <- msg
			} else {
				// No messages available, add to waiting list
				m.mu.Lock()
				m.waitingWorkers = append(m.waitingWorkers, workerCh)
				m.mu.Unlock()
			}
		}
	}
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

	// Set initial state
	if err := m.stateMachine.Transition(msg, StateQueued); err != nil {
		// Message is probably already in a state, just add it
		msg.SetState(StateQueued)
	}

	select {
	case m.incomingCh <- msg:
		return nil
	case <-m.ctx.Done():
		return fmt.Errorf("queue manager shutting down")
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout submitting message")
	}
}

// RequestMessage is called by workers to get the next message to process.
func (m *Manager) RequestMessage(ctx context.Context) (*Message, error) {
	respCh := make(chan *Message, 1)

	select {
	case m.requestCh <- respCh:
		// Request sent
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.ctx.Done():
		return nil, fmt.Errorf("queue manager shutting down")
	}

	select {
	case msg := <-respCh:
		if msg != nil {
			// Transition to processing state
			if err := m.stateMachine.Transition(msg, StateProcessing); err != nil {
				log.Printf("Failed to transition message %s to processing: %v", msg.ID, err)
			}
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.ctx.Done():
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
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("no queue found for conversation %s", msg.ConversationID)
	}

	queue.Complete()
	
	// Check if we should clean up this queue now
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if queue.IsEmpty() && !queue.IsProcessing() {
		// Remove from queues
		delete(m.queues, msg.ConversationID)
		// Remove from order
		for i, id := range m.conversationOrder {
			if id == msg.ConversationID {
				m.conversationOrder = append(
					m.conversationOrder[:i],
					m.conversationOrder[i+1:]...,
				)
				if m.currentIndex > i {
					m.currentIndex--
				}
				break
			}
		}
	}
	
	return nil
}

// Shutdown gracefully stops the queue manager.
func (m *Manager) Shutdown(timeout time.Duration) error {
	// Mark as shutting down
	m.mu.Lock()
	m.shutdown = true
	m.mu.Unlock()

	// Cancel context to signal shutdown
	m.cancel()

	// Wait for Start() to have been called
	select {
	case <-m.started:
		// Start() was called, proceed with shutdown
	case <-time.After(100 * time.Millisecond):
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
func (m *Manager) getNextMessage() *Message {
	m.mu.Lock()
	defer m.mu.Unlock()

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
			m.conversationOrder = append(
				m.conversationOrder[:m.currentIndex],
				m.conversationOrder[m.currentIndex+1:]...,
			)
			if m.currentIndex >= len(m.conversationOrder) && len(m.conversationOrder) > 0 {
				m.currentIndex = 0
			}
			attemptsLeft--
			continue
		}

		// Move to next conversation for fairness
		m.currentIndex++

		// Skip if already processing
		if queue.IsProcessing() {
			attemptsLeft--
			continue
		}

		msg := queue.Dequeue()
		if msg != nil {
			return msg
		}

		// Don't clean up queues while processing - CompleteMessage needs them
		// Only clean up if empty AND not processing
		if queue.IsEmpty() && !queue.IsProcessing() {
			// Remove from queues
			delete(m.queues, convID)
			// Remove from order
			for i, id := range m.conversationOrder {
				if id == convID {
					m.conversationOrder = append(
						m.conversationOrder[:i],
						m.conversationOrder[i+1:]...,
					)
					if m.currentIndex > i {
						m.currentIndex--
					}
					break
				}
			}
		}

		attemptsLeft--
	}

	return nil
}

// tryDispatch attempts to send a message to a waiting worker.
func (m *Manager) tryDispatch() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.waitingWorkers) == 0 {
		return
	}

	msg := m.getNextMessageLocked()
	if msg == nil {
		return
	}

	// Send to first waiting worker
	workerCh := m.waitingWorkers[0]
	m.waitingWorkers = m.waitingWorkers[1:]

	// Transition to processing state
	if err := m.stateMachine.Transition(msg, StateProcessing); err != nil {
		log.Printf("Failed to transition message %s to processing: %v", msg.ID, err)
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
				log.Printf("Failed to requeue message %s: %v", msg.ID, err)
			}
		}
	}
}

// getNextMessageLocked is like getNextMessage but assumes lock is held.
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
			m.conversationOrder = append(
				m.conversationOrder[:m.currentIndex],
				m.conversationOrder[m.currentIndex+1:]...,
			)
			if m.currentIndex >= len(m.conversationOrder) && len(m.conversationOrder) > 0 {
				m.currentIndex = 0
			}
			attemptsLeft--
			continue
		}

		// Move to next conversation for fairness
		m.currentIndex++

		// Skip if already processing
		if queue.IsProcessing() {
			attemptsLeft--
			continue
		}

		msg := queue.Dequeue()
		if msg != nil {
			return msg
		}

		// Don't clean up queues while processing - CompleteMessage needs them
		// Only clean up if empty AND not processing
		if queue.IsEmpty() && !queue.IsProcessing() {
			// Remove from queues
			delete(m.queues, convID)
			// Remove from order
			for i, id := range m.conversationOrder {
				if id == convID {
					m.conversationOrder = append(
						m.conversationOrder[:i],
						m.conversationOrder[i+1:]...,
					)
					if m.currentIndex > i {
						m.currentIndex--
					}
					break
				}
			}
		}

		attemptsLeft--
	}

	return nil
}

// cleanup releases resources when shutting down.
func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Send nil to all waiting workers to unblock them
	for _, workerCh := range m.waitingWorkers {
		select {
		case workerCh <- nil:
		default:
		}
	}
	m.waitingWorkers = nil

	// Close channels
	close(m.incomingCh)
	close(m.requestCh)
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
