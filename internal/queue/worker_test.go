package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// mockLLM implements the LLM interface for testing.
type mockLLM struct {
	err       error
	response  string
	queries   []string
	delay     time.Duration
	mu        sync.Mutex
	queryFunc func(ctx context.Context, prompt string, sessionID string) (*claude.LLMResponse, error)
}

func (m *mockLLM) Query(ctx context.Context, prompt string, sessionID string) (*claude.LLMResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, prompt, sessionID)
	}

	m.mu.Lock()
	m.queries = append(m.queries, prompt)
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.err != nil {
		return nil, m.err
	}

	return &claude.LLMResponse{
		Message:  m.response,
		Metadata: claude.ResponseMetadata{},
	}, nil
}

// mockMessenger implements the Messenger interface for testing.
type mockMessenger struct {
	sendErr      error
	typingErr    error
	incomingCh   chan signal.IncomingMessage
	sentMessages []sentMessage
	mu           sync.Mutex
	typingCount  int32
	sendFunc     func(ctx context.Context, recipient string, message string) error
}

type sentMessage struct {
	recipient string
	message   string
}

func (m *mockMessenger) Send(ctx context.Context, recipient string, message string) error {
	m.mu.Lock()
	sendFunc := m.sendFunc
	m.mu.Unlock()

	if sendFunc != nil {
		return sendFunc(ctx, recipient, message)
	}

	m.mu.Lock()
	m.sentMessages = append(m.sentMessages, sentMessage{recipient, message})
	m.mu.Unlock()
	return m.sendErr
}

func (m *mockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	if m.incomingCh == nil {
		m.incomingCh = make(chan signal.IncomingMessage)
	}
	return m.incomingCh, nil
}

func (m *mockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	atomic.AddInt32(&m.typingCount, 1)
	return m.typingErr
}

func TestWorker_ProcessSuccess(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	llm := &mockLLM{response: "Hello from Claude!"}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(10, 1, time.Second)

	// Start queue manager
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	w := NewWorker(config)
	worker, ok := w.(*worker)
	if !ok {
		t.Fatal("NewWorker did not return *worker")
	}

	// Create and submit message to queue first
	msg := NewMessage("msg-1", "conv-1", "user123", "+1234567890", "Hello Claude")
	if err := queueMgr.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Get the message from queue (simulating normal flow)
	reqMsg, err := queueMgr.RequestMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to get message from queue: %v", err)
	}

	// Add a small delay to LLM to ensure typing indicator starts
	llm.delay = 20 * time.Millisecond

	err = worker.Process(ctx, reqMsg)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Verify LLM was called
	if len(llm.queries) != 1 || llm.queries[0] != "Hello Claude" {
		t.Errorf("Expected LLM query 'Hello Claude', got %v", llm.queries)
	}

	// Verify response was sent
	if len(messenger.sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(messenger.sentMessages))
	}
	if messenger.sentMessages[0].recipient != "+1234567890" {
		t.Errorf("Expected recipient '+1234567890', got %s", messenger.sentMessages[0].recipient)
	}
	if messenger.sentMessages[0].message != "Hello from Claude!" {
		t.Errorf("Expected message 'Hello from Claude!', got %s", messenger.sentMessages[0].message)
	}

	// Verify message state
	if msg.GetState() != StateCompleted {
		t.Errorf("Expected state %s, got %s", StateCompleted, msg.GetState())
	}
	if msg.Response != "Hello from Claude!" {
		t.Errorf("Expected response saved in message")
	}

	// Verify typing indicator was sent
	typingCount := atomic.LoadInt32(&messenger.typingCount)
	if typingCount < 1 {
		t.Error("Expected at least one typing indicator")
	}
}

func TestWorker_ProcessLLMError(t *testing.T) {
	ctx := context.Background()

	// Setup mocks with LLM error
	llm := &mockLLM{err: errors.New("LLM unavailable")}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(10, 1, time.Second)

	// Start queue manager
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	w := NewWorker(config)
	worker, ok := w.(*worker)
	if !ok {
		t.Fatal("NewWorker did not return *worker")
	}

	// Create and submit message to queue first
	msg := NewMessage("msg-1", "conv-1", "user123", "+1234567890", "Hello")
	msg.MaxAttempts = 3
	if err := queueMgr.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Get the message from queue
	reqMsg, err := queueMgr.RequestMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to get message from queue: %v", err)
	}

	err = worker.Process(ctx, reqMsg)
	if err == nil {
		t.Fatal("Expected error from Process")
	}

	// Verify message can retry
	if reqMsg.GetState() != StateRetrying {
		t.Errorf("Expected state %s, got %s", StateRetrying, reqMsg.GetState())
	}

	// Verify no response was sent
	if len(messenger.sentMessages) != 0 {
		t.Errorf("Expected no sent messages, got %d", len(messenger.sentMessages))
	}
}

func TestWorker_ProcessMaxRetries(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	llm := &mockLLM{err: errors.New("LLM error")}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(10, 1, time.Second)

	// Start queue manager
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	w := NewWorker(config)
	worker, ok := w.(*worker)
	if !ok {
		t.Fatal("NewWorker did not return *worker")
	}

	// Create message at max attempts
	msg := NewMessage("msg-1", "conv-1", "user123", "+1234567890", "Hello")
	msg.MaxAttempts = 3
	msg.Attempts = 3
	if err := queueMgr.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Get the message from queue
	reqMsg, err := queueMgr.RequestMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to get message from queue: %v", err)
	}

	err = worker.Process(ctx, reqMsg)
	if err == nil {
		t.Fatal("Expected error from Process")
	}

	// Should be failed, not retrying
	if reqMsg.GetState() != StateFailed {
		t.Errorf("Expected state %s, got %s", StateFailed, reqMsg.GetState())
	}
}

func TestWorker_RateLimiting(t *testing.T) {
	ctx := context.Background()

	// Setup with restrictive rate limiter
	llm := &mockLLM{response: "Response"}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(1, 1, 100*time.Millisecond)

	// Start queue manager
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	w := NewWorker(config)
	worker, ok := w.(*worker)
	if !ok {
		t.Fatal("NewWorker did not return *worker")
	}

	// Use up the token
	rateLimiter.Allow("conv-1")

	// Submit message to queue first
	msg := NewMessage("msg-1", "conv-1", "user123", "+1234567890", "Hello")
	if err := queueMgr.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Get message from queue
	reqMsg, err := queueMgr.RequestMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to get message from queue: %v", err)
	}

	// Process should be rate limited
	err = worker.Process(ctx, reqMsg)

	// Should fail with rate limit error
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("Expected rate limit error, got: %v", err)
	}

	// Wait for token refill and try again
	<-time.After(100 * time.Millisecond)

	// Should succeed now
	err = worker.Process(ctx, reqMsg)
	if err != nil {
		t.Errorf("Process failed after rate limit refill: %v", err)
	}
}

func TestWorkerPool_Start(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup
	llm := &mockLLM{response: "Response", delay: 50 * time.Millisecond}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(10, 1, time.Second)

	// Start queue manager
	go queueMgr.Start()

	// Create worker pool
	pool := NewWorkerPool(3, llm, messenger, queueMgr, rateLimiter)

	// Start pool
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}

	// Submit some messages
	for i := 0; i < 5; i++ {
		msg := NewMessage(
			fmt.Sprintf("msg-%d", i),
			fmt.Sprintf("conv-%d", i%2),
			"user",
			"+1234567890",
			fmt.Sprintf("Message %d", i),
		)
		if err := queueMgr.Submit(msg); err != nil {
			t.Errorf("Failed to submit message: %v", err)
		}
	}

	// Wait for processing
	<-time.After(200 * time.Millisecond)

	// Shutdown
	cancel()
	pool.Wait()
	if err := queueMgr.Shutdown(time.Second); err != nil {
		t.Errorf("Failed to shutdown queue manager: %v", err)
	}

	// Verify messages were processed
	if len(messenger.sentMessages) != 5 {
		t.Errorf("Expected 5 messages sent, got %d", len(messenger.sentMessages))
	}
}

func TestWorkerPool_Size(t *testing.T) {
	ctx := context.Background()

	llm := &mockLLM{}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(10, 1, time.Second)

	pool := NewWorkerPool(5, llm, messenger, queueMgr, rateLimiter)

	if pool.Size() != 5 {
		t.Errorf("Expected pool size 5, got %d", pool.Size())
	}
}

func TestWorker_TypingIndicator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup with slow LLM
	llm := &mockLLM{response: "Response", delay: 150 * time.Millisecond}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(10, 1, time.Second)

	// Start queue manager
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	w := NewWorker(config)
	worker, ok := w.(*worker)
	if !ok {
		t.Fatal("NewWorker did not return *worker")
	}

	// Submit and get message from queue
	msg := NewMessage("msg-1", "conv-1", "user123", "+1234567890", "Hello")
	if err := queueMgr.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	reqMsg, err := queueMgr.RequestMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to get message from queue: %v", err)
	}

	err = worker.Process(ctx, reqMsg)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Should have sent at least 1 typing indicator
	typingCount := atomic.LoadInt32(&messenger.typingCount)
	if typingCount < 1 {
		t.Errorf("Expected at least 1 typing indicator, got %d", typingCount)
	}
}

func TestWorker_ProcessesMessagesInOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track message processing order
	var processedOrder []string
	var orderMu sync.Mutex

	// Setup LLM that records processing order
	llm := &mockLLM{
		response: "Response",
		delay:    10 * time.Millisecond,
	}
	llm.queryFunc = func(ctx context.Context, prompt string, _ string) (*claude.LLMResponse, error) {
		orderMu.Lock()
		processedOrder = append(processedOrder, prompt)
		orderMu.Unlock()

		// Simulate delay
		if llm.delay > 0 {
			select {
			case <-time.After(llm.delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return &claude.LLMResponse{
			Message:  llm.response,
			Metadata: claude.ResponseMetadata{},
		}, nil
	}

	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(100, 10, time.Second) // High rate limit

	// Start queue manager
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create single worker to ensure sequential processing
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	worker := NewWorker(config)

	// Start worker
	workerCtx, workerCancel := context.WithCancel(ctx)
	go func() {
		if err := worker.Start(workerCtx); err != nil && err != context.Canceled {
			t.Logf("Worker error: %v", err)
		}
	}()

	// Submit messages from same conversation
	messages := []string{"First", "Second", "Third", "Fourth", "Fifth"}
	for i, text := range messages {
		msg := NewMessage(
			fmt.Sprintf("msg-%d", i),
			"conv-1", // Same conversation
			"user123",
			"+1234567890",
			text,
		)
		if err := queueMgr.Submit(msg); err != nil {
			t.Fatalf("Failed to submit message %d: %v", i, err)
		}
		// Small delay to ensure order is preserved
		<-time.After(5 * time.Millisecond)
	}

	// Wait for processing
	<-time.After(200 * time.Millisecond)

	// Stop worker
	workerCancel()

	// Verify messages were processed in order
	orderMu.Lock()
	defer orderMu.Unlock()

	if len(processedOrder) != len(messages) {
		t.Fatalf("Expected %d messages processed, got %d", len(messages), len(processedOrder))
	}

	for i, expected := range messages {
		if processedOrder[i] != expected {
			t.Errorf("Message %d: expected '%s', got '%s'", i, expected, processedOrder[i])
		}
	}
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track message states
	var processedCount int32

	// Setup LLM with quick response
	llm := &mockLLM{
		response: "Response",
		delay:    10 * time.Millisecond,
	}

	// Track sent messages
	messenger := &mockMessenger{}
	messenger.sendFunc = func(_ context.Context, recipient string, message string) error {
		atomic.AddInt32(&processedCount, 1)
		// Also track in sentMessages
		messenger.mu.Lock()
		messenger.sentMessages = append(messenger.sentMessages, sentMessage{recipient, message})
		messenger.mu.Unlock()
		return nil
	}

	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(100, 10, time.Second)

	// Start queue manager
	go queueMgr.Start()

	// Create worker pool
	pool := NewWorkerPool(2, llm, messenger, queueMgr, rateLimiter)

	// Start pool
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}

	// Submit messages
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		msg := NewMessage(
			fmt.Sprintf("msg-%d", i),
			fmt.Sprintf("conv-%d", i%2), // Two conversations
			fmt.Sprintf("user-%d", i),
			fmt.Sprintf("+123456789%d", i),
			fmt.Sprintf("Message %d", i),
		)
		if err := queueMgr.Submit(msg); err != nil {
			t.Errorf("Failed to submit message: %v", err)
		}
	}

	// Let some messages process
	<-time.After(50 * time.Millisecond)

	// Initiate graceful shutdown
	cancel()

	// Wait for workers to finish
	pool.Wait()

	// Get final stats before shutdown
	stats := queueMgr.Stats()
	t.Logf("Queue stats at shutdown: %+v", stats)

	// Shutdown queue manager
	if err := queueMgr.Shutdown(time.Second); err != nil {
		t.Errorf("Failed to shutdown queue manager: %v", err)
	}

	// Stop typing indicators
	pool.Stop()

	processed := atomic.LoadInt32(&processedCount)

	t.Logf("Processed %d messages successfully", processed)

	// We should have processed at least some messages
	if processed == 0 {
		t.Error("No messages were processed before shutdown")
	}

	// In a graceful shutdown, workers stop accepting new work but finish current work
	// So we expect some messages to be processed, and the rest to remain in queue
	if processed > int32(messageCount) {
		t.Errorf("Processed more messages than submitted: %d > %d", processed, messageCount)
	}
}

func TestWorker_MessageOrderWithinConversation(t *testing.T) {
	ctx := context.Background()

	// Track processing order per conversation
	convOrder := make(map[string][]string)
	var orderMu sync.Mutex

	// Setup LLM that tracks order
	llm := &mockLLM{
		response: "Response",
		delay:    5 * time.Millisecond,
	}
	llm.queryFunc = func(_ context.Context, prompt string, sessionID string) (*claude.LLMResponse, error) {
		orderMu.Lock()
		// Extract conversation ID from sessionID (format: "signal-conv-X")
		var convID string
		if _, err := fmt.Sscanf(sessionID, "signal-%s", &convID); err == nil {
			convOrder[convID] = append(convOrder[convID], prompt)
		}
		orderMu.Unlock()

		// Simulate processing
		<-time.After(llm.delay)

		return &claude.LLMResponse{
			Message:  llm.response,
			Metadata: claude.ResponseMetadata{},
		}, nil
	}

	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(100, 10, time.Second)

	// Start queue manager
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker pool with multiple workers
	pool := NewWorkerPool(3, llm, messenger, queueMgr, rateLimiter)

	// Start pool
	poolCtx, poolCancel := context.WithCancel(ctx)
	if err := pool.Start(poolCtx); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}

	// Submit messages to multiple conversations
	conversations := []string{"conv-1", "conv-2", "conv-3"}
	messagesPerConv := 5

	for _, convID := range conversations {
		for i := 0; i < messagesPerConv; i++ {
			msg := NewMessage(
				fmt.Sprintf("msg-%s-%d", convID, i),
				convID,
				"user123",
				"+1234567890",
				fmt.Sprintf("%s-Message-%d", convID, i),
			)
			if err := queueMgr.Submit(msg); err != nil {
				t.Errorf("Failed to submit message: %v", err)
			}
		}
	}

	// Wait for processing
	<-time.After(200 * time.Millisecond)

	// Stop pool
	poolCancel()
	pool.Wait()

	// Verify order within each conversation
	orderMu.Lock()
	defer orderMu.Unlock()

	for _, convID := range conversations {
		messages := convOrder[convID]
		if len(messages) != messagesPerConv {
			t.Errorf("Conversation %s: expected %d messages, got %d",
				convID, messagesPerConv, len(messages))
		}

		// Check order
		for i := 0; i < len(messages); i++ {
			expected := fmt.Sprintf("%s-Message-%d", convID, i)
			if messages[i] != expected {
				t.Errorf("Conversation %s: message %d out of order. Expected '%s', got '%s'",
					convID, i, expected, messages[i])
			}
		}
	}
}

func TestWorker_StartStopIdempotent(t *testing.T) {
	ctx := context.Background()

	// Setup
	llm := &mockLLM{response: "Response"}
	messenger := &mockMessenger{}
	queueMgr := NewManager(ctx)
	rateLimiter := NewRateLimiter(10, 1, time.Second)

	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	worker := NewWorker(config)

	// Multiple stops should not panic
	if err := worker.Stop(); err != nil {
		t.Errorf("First stop failed: %v", err)
	}
	if err := worker.Stop(); err != nil {
		t.Errorf("Second stop failed: %v", err)
	}

	// Verify ID is consistent
	expectedID := "worker-1"
	if worker.ID() != expectedID {
		t.Errorf("Expected worker ID %s, got %s", expectedID, worker.ID())
	}
}
