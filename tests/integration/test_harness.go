//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/mocks"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

// TestHarness provides a comprehensive integration test environment for Mentat.
// It wires together real components with mocked external dependencies
// to enable deterministic testing of full conversation flows.
type TestHarness struct {
	// Context and synchronization
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	t      *testing.T

	// External dependencies (mocked)
	mockLLM       *mocks.MockLLM
	mockMessenger *MockMessengerWithIncoming

	// Real components
	queueManager  *queue.Manager
	workerPool    *queue.DynamicWorkerPool
	signalHandler *signal.Handler
	queueAdapter  *trackingQueueAdapter

	// Message tracking
	messageTracker *MessageTracker
	messagesSent   chan signal.Message

	// Monitoring
	queueMonitor    *QueueMonitor
	resourceMonitor *ResourceMonitor

	// Test control
	stateChanges chan StateChange
	errors       chan error

	// Configuration
	config HarnessConfig
}

// HarnessConfig controls test harness behavior
type HarnessConfig struct {
	BotPhoneNumber  string
	WorkerCount     int
	QueueDepth      int
	RateLimitTokens int
	RateLimitRefill time.Duration
	DefaultTimeout  time.Duration
	EnableLogging   bool
}

// DefaultConfig returns a standard test configuration
func DefaultConfig() HarnessConfig {
	return HarnessConfig{
		BotPhoneNumber:  "+1234567890",
		WorkerCount:     2,
		QueueDepth:      10,
		RateLimitTokens: 10,
		RateLimitRefill: time.Second,
		DefaultTimeout:  5 * time.Second,
		EnableLogging:   false,
	}
}

// NewTestHarness creates a new integration test harness
func NewTestHarness(t *testing.T, config HarnessConfig) *TestHarness {
	ctx, cancel := context.WithCancel(context.Background())

	return &TestHarness{
		ctx:             ctx,
		cancel:          cancel,
		t:               t,
		config:          config,
		messageTracker:  NewMessageTracker(),
		messagesSent:    make(chan signal.Message, 100),
		queueMonitor:    NewQueueMonitor(),
		resourceMonitor: NewResourceMonitor(),
		stateChanges:    make(chan StateChange, 1000),
		errors:          make(chan error, 100),
	}
}

// Setup initializes all components and starts background goroutines
func (h *TestHarness) Setup() error {
	// Create mocks
	h.mockLLM = mocks.NewMockLLM()
	h.mockMessenger = NewMockMessengerWithIncoming()

	// Hook mock callbacks for test observation
	h.setupMockCallbacks()

	// Create queue components
	limiter := queue.NewRateLimiter(
		h.config.RateLimitTokens,
		1, // refill rate of 1 token
		h.config.RateLimitRefill,
	)
	h.queueManager = queue.NewManager(h.ctx)

	// Create queue adapter with tracking
	adapterCtx, adapterCancel := context.WithCancel(h.ctx)
	h.queueAdapter = &trackingQueueAdapter{
		manager:        h.queueManager,
		tracker:        h.messageTracker,
		activeMessages: make(map[string]*queue.Message),
		ctx:            adapterCtx,
		cancel:         adapterCancel,
		harness:        h,
	}

	// Create worker pool with real workers
	minSize := 1
	if h.config.WorkerCount == 0 {
		// Special case for testing queue overflow - we want no workers
		minSize = 0
	}
	workerConfig := queue.PoolConfig{
		InitialSize:  h.config.WorkerCount,
		MinSize:      minSize,
		MaxSize:      5,
		LLM:          h.mockLLM,
		Messenger:    h.mockMessenger,
		QueueManager: h.queueManager,
		MessageQueue: h.queueAdapter,
		RateLimiter:  limiter,
	}

	var err error
	h.workerPool, err = queue.NewDynamicWorkerPool(h.ctx, workerConfig)
	if err != nil {
		return fmt.Errorf("failed to create worker pool: %w", err)
	}

	// Create message enqueuer adapter with tracking
	enqueuer := &trackingMessageEnqueuer{
		queue:   h.queueManager,
		tracker: h.messageTracker,
		harness: h,
	}

	// Create signal handler
	h.signalHandler, err = signal.NewHandler(h.mockMessenger, enqueuer)
	if err != nil {
		return fmt.Errorf("failed to create signal handler: %w", err)
	}

	// Start queue monitoring
	h.queueMonitor.Start(h.ctx, 100*time.Millisecond, h.queueManager.Stats)

	// Start resource monitoring
	h.wg.Add(1)
	go h.monitorResources()

	// Start all components
	h.startComponents()

	// Record initial resource state
	h.resourceMonitor.Sample()

	// Record setup completion
	h.recordStateChange("harness", "setup_complete", map[string]any{
		"worker_count": h.config.WorkerCount,
		"queue_depth":  h.config.QueueDepth,
	})

	return nil
}

// Teardown stops all components and waits for graceful shutdown
func (h *TestHarness) Teardown() {
	// Stop monitoring first
	h.queueMonitor.Stop()

	// Stop worker pool first (before cancelling context)
	if h.workerPool != nil {
		h.workerPool.Stop(context.Background())
	}

	// Shutdown queue manager
	if h.queueManager != nil {
		h.queueManager.Shutdown(2 * time.Second)
	}

	// Cancel queue adapter context to signal monitoring goroutines to stop
	if h.queueAdapter != nil && h.queueAdapter.cancel != nil {
		h.queueAdapter.cancel()

		// Wait for all monitoring goroutines to complete
		// This prevents the race condition where goroutines are still
		// reading from the context's done channel when it's closed
		h.queueAdapter.Wait()
	}

	// Cancel main context to signal shutdown
	h.cancel()

	// Wait for all goroutines with timeout
	done := make(chan bool)
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean shutdown
	case <-time.After(h.config.DefaultTimeout):
		h.t.Error("Teardown timeout - some goroutines did not exit")
	}

	// Close test channels only after all goroutines have stopped
	// This prevents races where goroutines try to send to closed channels
	close(h.messagesSent)
	close(h.stateChanges)
	close(h.errors)

	// Check for resource leaks
	if err := h.resourceMonitor.CheckLeaks(); err != nil {
		h.t.Errorf("Resource leak detected: %v", err)
	}

	// Report any queue alerts
	alerts := h.queueMonitor.GetAlerts()
	if len(alerts) > 0 {
		h.t.Logf("Queue alerts during test:")
		for _, alert := range alerts {
			h.t.Logf("  - %s: %s", alert.Type, alert.Description)
		}
	}
}

// SendMessage simulates receiving a Signal message
func (h *TestHarness) SendMessage(phoneNumber, text string) error {
	timestamp := time.Now()
	msgID := fmt.Sprintf("msg-%d", timestamp.UnixNano())

	msg := signal.IncomingMessage{
		Timestamp:  timestamp,
		From:       "Test User",
		FromNumber: phoneNumber,
		Text:       text,
	}

	h.recordStateChange("test", "message_sent", map[string]any{
		"from": phoneNumber,
		"text": text,
		"id":   msgID,
	})

	return h.mockMessenger.SimulateIncomingMessage(msg)
}

// SetLLMResponse configures the next LLM response for any session
func (h *TestHarness) SetLLMResponse(response string, err error) {
	if err != nil {
		h.mockLLM.SetError(err)
	} else {
		// Set response for common test session patterns
		resp := &claude.LLMResponse{
			Message: response,
		}
		// Set default response for any session (empty string matches all)
		h.mockLLM.SetSessionResponse("", resp)
	}

	h.recordStateChange("test", "llm_response_configured", map[string]any{
		"response": response,
		"error":    err != nil,
	})
}

// WaitForMessage waits for a message to be sent or times out
func (h *TestHarness) WaitForMessage(timeout time.Duration) (signal.Message, error) {
	select {
	case msg := <-h.messagesSent:
		return msg, nil
	case <-time.After(timeout):
		return signal.Message{}, fmt.Errorf("timeout waiting for message")
	}
}

// WaitForLLMCall waits for an LLM call or times out
func (h *TestHarness) WaitForLLMCall(timeout time.Duration) (LLMCall, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		calls := h.mockLLM.GetCalls()
		if len(calls) > 0 {
			lastCall := calls[len(calls)-1]
			return LLMCall{
				Prompt:    lastCall.Prompt,
				SessionID: lastCall.SessionID,
				Timestamp: lastCall.Timestamp,
			}, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return LLMCall{}, fmt.Errorf("timeout waiting for LLM call")
}

// WaitForMessageCompletion waits for a specific message to complete
func (h *TestHarness) WaitForMessageCompletion(msgID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		msg, ok := h.messageTracker.GetMessage(msgID)
		if ok && msg.CurrentState == queue.StateCompleted {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Return detailed error with state history
	if msg, ok := h.messageTracker.GetMessage(msgID); ok {
		return fmt.Errorf("message %s did not complete in time, current state: %v, history: %+v",
			msgID, msg.CurrentState, msg.StateHistory)
	}

	return fmt.Errorf("message %s not found", msgID)
}

// WaitForAllMessagesCompletion waits for all tracked messages to complete processing
func (h *TestHarness) WaitForAllMessagesCompletion(expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		stats := h.messageTracker.GetStats()
		completed := stats.Completed
		failed := stats.Failed
		total := stats.Total

		// Check if all messages have been processed (either completed or failed)
		processedCount := completed + failed
		if processedCount >= expectedCount {
			// If we have the expected number, we're done
			return nil
		}

		// Also check if all tracked messages are in a terminal state
		if total > 0 && processedCount >= total {
			// All messages that were submitted have been processed
			if total < expectedCount {
				// We didn't get all the messages we expected
				return fmt.Errorf("only %d messages were tracked, expected %d", total, expectedCount)
			}
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Timeout - provide detailed state information
	state := h.VerifyQueueState()
	return fmt.Errorf("timeout waiting for %d messages to complete: completed=%d, failed=%d, pending=%d, processing=%d, retrying=%d",
		expectedCount, state.CompletedMessages, state.FailedMessages,
		state.PendingMessages, state.ProcessingMessages, state.RetryingMessages)
}

// VerifyQueueState checks the current queue state with full tracking
func (h *TestHarness) VerifyQueueState() QueueState {
	stats := h.messageTracker.GetStats()
	managerStats := h.queueManager.Stats()

	// Build conversation state from tracked messages
	conversations := make(map[string]ConversationState)
	h.messageTracker.mu.RLock()
	for _, msg := range h.messageTracker.messages {
		conv, exists := conversations[msg.ConversationID]
		if !exists {
			conv = ConversationState{
				LastActivity: msg.EnqueuedAt,
			}
		}
		if msg.CurrentState == queue.StateCompleted {
			conv.ProcessedMsgs++
		}
		if msg.CurrentState == queue.StateQueued || msg.CurrentState == queue.StateProcessing {
			conv.QueueDepth++
		}
		if msg.CompletedAt != nil && msg.CompletedAt.After(conv.LastActivity) {
			conv.LastActivity = *msg.CompletedAt
		}
		conversations[msg.ConversationID] = conv
	}
	h.messageTracker.mu.RUnlock()

	return QueueState{
		TotalMessages:       stats.Total,
		PendingMessages:     stats.ByState[queue.StateQueued],
		ProcessingMessages:  stats.ByState[queue.StateProcessing],
		CompletedMessages:   stats.Completed,
		FailedMessages:      stats.Failed,
		RetryingMessages:    stats.ByState[queue.StateRetrying],
		Conversations:       conversations,
		QueuedInManager:     managerStats["queued_messages"],
		ProcessingInManager: managerStats["processing_messages"],
	}
}

// GetMessageStats returns detailed message statistics
func (h *TestHarness) GetMessageStats() MessageStats {
	return h.messageTracker.GetStats()
}

// GetQueueMetrics returns queue performance metrics
func (h *TestHarness) GetQueueMetrics() QueueMetrics {
	samples := h.queueMonitor.GetSamples()
	if len(samples) == 0 {
		return QueueMetrics{}
	}

	var totalDepth, maxDepth int
	var totalProcessing int

	for _, s := range samples {
		totalDepth += s.QueueDepth
		if s.QueueDepth > maxDepth {
			maxDepth = s.QueueDepth
		}
		totalProcessing += s.Processing
	}

	return QueueMetrics{
		AverageDepth:      float64(totalDepth) / float64(len(samples)),
		MaxDepth:          maxDepth,
		AverageProcessing: float64(totalProcessing) / float64(len(samples)),
		TotalSamples:      len(samples),
	}
}

// GetStateChanges returns all recorded state changes
func (h *TestHarness) GetStateChanges() []StateChange {
	var changes []StateChange

	// Drain channel
	for {
		select {
		case change := <-h.stateChanges:
			changes = append(changes, change)
		default:
			return changes
		}
	}
}

// GetErrors returns all recorded errors
func (h *TestHarness) GetErrors() []error {
	var errors []error

	// Drain channel
	for {
		select {
		case err := <-h.errors:
			errors = append(errors, err)
		default:
			return errors
		}
	}
}

// setupMockCallbacks configures mocks to record calls for verification
func (h *TestHarness) setupMockCallbacks() {
	// Set up a goroutine to monitor MockMessenger's sent messages
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		lastCount := 0

		for {
			select {
			case <-h.ctx.Done():
				return
			case <-ticker.C:
				sent := h.mockMessenger.GetSentMessages()
				for i := lastCount; i < len(sent); i++ {
					msg := signal.Message{
						Timestamp: sent[i].Timestamp,
						Recipient: sent[i].Recipient,
						Text:      sent[i].Message,
						Sender:    h.config.BotPhoneNumber,
					}
					select {
					case h.messagesSent <- msg:
					case <-h.ctx.Done():
						return
					}
				}
				lastCount = len(sent)
			}
		}
	}()
}

// startComponents launches all background goroutines
func (h *TestHarness) startComponents() {
	// Start queue manager
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.queueManager.Start(h.ctx)
	}()

	// Start worker pool
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		if err := h.workerPool.Start(h.ctx); err != nil {
			select {
			case h.errors <- fmt.Errorf("worker pool error: %w", err):
			case <-h.ctx.Done():
			}
		}
	}()

	// Start signal handler
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		if err := h.signalHandler.Start(h.ctx); err != nil {
			select {
			case h.errors <- fmt.Errorf("signal handler error: %w", err):
			case <-h.ctx.Done():
			}
		}
	}()

	// Start message completion tracking
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.trackMessageCompletions()
	}()

	// Give components time to initialize
	time.Sleep(100 * time.Millisecond)
}

// trackMessageCompletions monitors for message state changes
func (h *TestHarness) trackMessageCompletions() {
	// This is now handled by the trackingQueueAdapter
	// which intercepts all UpdateState calls from workers
	<-h.ctx.Done()
}

// monitorResources continuously tracks resource usage
func (h *TestHarness) monitorResources() {
	defer h.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.resourceMonitor.Sample()
		}
	}
}

// recordStateChange records a state change for later verification
func (h *TestHarness) recordStateChange(component, changeType string, details map[string]any) {
	change := StateChange{
		Component: component,
		Type:      changeType,
		Details:   details,
		Timestamp: time.Now(),
	}

	select {
	case h.stateChanges <- change:
	default:
		// Channel full, drop oldest
		<-h.stateChanges
		h.stateChanges <- change
	}
}

// StateChange represents a change in system state for verification
type StateChange struct {
	Component string
	Type      string
	Details   map[string]any
	Timestamp time.Time
}

// QueueState represents the current state of the message queue
type QueueState struct {
	TotalMessages       int
	PendingMessages     int
	ProcessingMessages  int
	CompletedMessages   int
	FailedMessages      int
	RetryingMessages    int
	Conversations       map[string]ConversationState
	QueuedInManager     int // From the actual queue manager
	ProcessingInManager int // From the actual queue manager
}

// ConversationState represents the state of a single conversation
type ConversationState struct {
	QueueDepth    int
	ProcessedMsgs int
	LastActivity  time.Time
}

// QueueMetrics provides queue performance metrics
type QueueMetrics struct {
	AverageDepth      float64
	MaxDepth          int
	AverageProcessing float64
	TotalSamples      int
}

// LLMCall represents a call to the LLM for test verification
type LLMCall struct {
	Prompt    string
	SessionID string
	Timestamp time.Time
}

// MockMessengerWithIncoming extends MockMessenger to support simulating incoming messages
type MockMessengerWithIncoming struct {
	*mocks.MockMessenger
}

// NewMockMessengerWithIncoming creates a new mock messenger with incoming message support
func NewMockMessengerWithIncoming() *MockMessengerWithIncoming {
	base := mocks.NewMockMessenger()
	return &MockMessengerWithIncoming{
		MockMessenger: base,
	}
}

// SimulateIncomingMessage sends a message to all subscribers
func (m *MockMessengerWithIncoming) SimulateIncomingMessage(msg signal.IncomingMessage) error {
	m.InjectMessage(msg)
	return nil
}

// trackingQueueAdapter provides comprehensive queue tracking
type trackingQueueAdapter struct {
	manager        *queue.Manager
	tracker        *MessageTracker
	mu             sync.Mutex
	activeMessages map[string]*queue.Message // Track messages being processed
	ctx            context.Context
	cancel         context.CancelFunc
	harness        *TestHarness   // Back reference for state changes
	wg             sync.WaitGroup // Track monitoring goroutines
}

// Enqueue implements queue.MessageQueue with tracking
func (a *trackingQueueAdapter) Enqueue(msg signal.IncomingMessage) error {
	msgID := fmt.Sprintf("msg-%d", msg.Timestamp.UnixNano())

	// Track the message
	a.tracker.TrackMessage(msgID, msg.FromNumber, msg.Text)

	// Create queue message
	queueMsg := &queue.Message{
		ID:             msgID,
		ConversationID: msg.FromNumber,
		Text:           msg.Text,
		SenderNumber:   msg.FromNumber,
		Sender:         msg.From,
		CreatedAt:      msg.Timestamp,
		State:          queue.StateQueued,
		MaxAttempts:    3, // Allow retries for rate limiting
	}

	// Submit to queue
	err := a.manager.Submit(queueMsg)

	// Track state change and record harness state change
	if err != nil {
		a.tracker.RecordStateChange(msgID, queue.StateQueued, queue.StateFailed,
			"enqueue failed", err)
	} else {
		// Record successful enqueue for test verification
		if a.harness != nil {
			a.harness.recordStateChange("queue", "message_enqueued", map[string]any{
				"message_id":      msgID,
				"conversation_id": msg.FromNumber,
			})
		}
	}

	return err
}

// GetNext implements queue.MessageQueue with state tracking
func (a *trackingQueueAdapter) GetNext(workerID string) (*queue.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg, err := a.manager.RequestMessage(ctx)
	if err != nil || msg == nil {
		return nil, err
	}

	// Track state change
	a.tracker.RecordStateChange(msg.ID, queue.StateQueued, queue.StateProcessing,
		fmt.Sprintf("assigned to worker %s", workerID), nil)

	// Record harness state change for test verification
	if a.harness != nil {
		a.harness.recordStateChange("queue", "message_processing", map[string]any{
			"message_id": msg.ID,
			"worker_id":  workerID,
		})
	}

	// Keep reference to track completion
	a.mu.Lock()
	a.activeMessages[msg.ID] = msg
	a.mu.Unlock()

	// Start monitoring this message for completion
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.monitorMessageCompletion(msg)
	}()

	// Return the message directly
	return msg, nil
}

// UpdateState implements queue.MessageQueue with tracking
func (a *trackingQueueAdapter) UpdateState(msgID string, state queue.State, reason string) error {
	// Track the state change
	if msg, ok := a.tracker.GetMessage(msgID); ok {
		a.tracker.RecordStateChange(msgID, msg.CurrentState, state, reason, nil)
	}

	// Remove from active if completed/failed
	if state == queue.StateCompleted || state == queue.StateFailed {
		a.mu.Lock()
		activeMsg, exists := a.activeMessages[msgID]
		delete(a.activeMessages, msgID)
		a.mu.Unlock()

		// Record harness state change for test verification
		if a.harness != nil {
			if state == queue.StateCompleted {
				a.harness.recordStateChange("queue", "message_completed", map[string]any{
					"message_id": msgID,
				})
			} else if state == queue.StateFailed {
				a.harness.recordStateChange("queue", "message_failed", map[string]any{
					"message_id": msgID,
					"reason":     reason,
				})
			}
		}

		// If the message was completed, we need to tell the queue manager
		// so it can process the next message in the conversation
		if exists && (state == queue.StateCompleted || state == queue.StateFailed) {
			if err := a.manager.CompleteMessage(activeMsg); err != nil {
				return fmt.Errorf("failed to complete message in manager: %w", err)
			}
		}
	}

	return nil
}

// monitorMessageCompletion watches for message state changes
func (a *trackingQueueAdapter) monitorMessageCompletion(msg *queue.Message) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-a.ctx.Done():
			// Context cancelled, clean up
			a.mu.Lock()
			delete(a.activeMessages, msg.ID)
			a.mu.Unlock()
			return
		case <-timeout:
			// Timeout - mark as failed
			a.tracker.RecordStateChange(msg.ID, queue.StateProcessing,
				queue.StateFailed, "processing timeout", nil)
			a.mu.Lock()
			delete(a.activeMessages, msg.ID)
			a.mu.Unlock()
			return
		case <-ticker.C:
			// Check message state
			currentState := msg.GetState()
			if currentState != queue.StateProcessing {
				// State changed - update tracker
				a.tracker.RecordStateChange(msg.ID, queue.StateProcessing,
					currentState, "state change detected", nil)

				a.mu.Lock()
				delete(a.activeMessages, msg.ID)
				a.mu.Unlock()
				return
			}

			// Also check if message was completed/failed through queue manager
			if a.manager != nil {
				// Check if message is still in processing
				stats := a.manager.Stats()
				if stats["processing_messages"] == 0 {
					// No messages processing, this one must have completed or failed
					// Default to failed if we got here
					a.tracker.RecordStateChange(msg.ID, queue.StateProcessing,
						queue.StateFailed, "processing ended without state update", nil)

					a.mu.Lock()
					delete(a.activeMessages, msg.ID)
					a.mu.Unlock()
					return
				}
			}
		}
	}
}

// Stats implements queue.MessageQueue
func (a *trackingQueueAdapter) Stats() queue.Stats {
	managerStats := a.manager.Stats()
	trackerStats := a.tracker.GetStats()

	return queue.Stats{
		TotalQueued:       managerStats["queued_messages"],
		TotalProcessing:   managerStats["processing_messages"],
		TotalCompleted:    trackerStats.Completed,
		TotalFailed:       trackerStats.Failed,
		ConversationCount: managerStats["conversations"],
	}
}

// Wait waits for all monitoring goroutines to complete
func (a *trackingQueueAdapter) Wait() {
	a.wg.Wait()
}

// trackingMessageEnqueuer tracks all enqueued messages
type trackingMessageEnqueuer struct {
	queue   *queue.Manager
	tracker *MessageTracker
	harness *TestHarness // Back reference for state changes
}

// Enqueue implements signal.MessageEnqueuer with tracking
func (e *trackingMessageEnqueuer) Enqueue(msg signal.IncomingMessage) error {
	msgID := fmt.Sprintf("msg-%d", msg.Timestamp.UnixNano())

	// Track the message
	e.tracker.TrackMessage(msgID, msg.FromNumber, msg.Text)

	// Create and submit queue message
	queueMsg := &queue.Message{
		ID:             msgID,
		ConversationID: msg.FromNumber,
		Text:           msg.Text,
		SenderNumber:   msg.FromNumber,
		Sender:         msg.From,
		CreatedAt:      msg.Timestamp,
		State:          queue.StateQueued,
		MaxAttempts:    3, // Allow retries for rate limiting
	}

	err := e.queue.Submit(queueMsg)

	// Record state change for test verification
	if err == nil && e.harness != nil {
		e.harness.recordStateChange("queue", "message_enqueued", map[string]any{
			"message_id":      msgID,
			"conversation_id": msg.FromNumber,
		})
	}

	return err
}

// RunScenario executes a test scenario with proper setup/teardown
func RunScenario(t *testing.T, name string, config HarnessConfig, scenario func(*testing.T, *TestHarness)) {
	t.Run(name, func(t *testing.T) {
		harness := NewTestHarness(t, config)

		if err := harness.Setup(); err != nil {
			t.Fatalf("Failed to setup test harness: %v", err)
		}

		defer harness.Teardown()

		scenario(t, harness)

		// Check for unexpected errors
		if errors := harness.GetErrors(); len(errors) > 0 {
			for _, err := range errors {
				t.Errorf("Unexpected error: %v", err)
			}
		}
	})
}
