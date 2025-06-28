package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockLLM implements the LLM interface for testing.
type mockLLM struct {
	mu       sync.Mutex
	queries  []string
	response string
	err      error
	delay    time.Duration
}

func (m *mockLLM) Query(ctx context.Context, prompt string, sessionID string) (string, error) {
	m.mu.Lock()
	m.queries = append(m.queries, prompt)
	m.mu.Unlock()
	
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	
	return m.response, m.err
}

// mockMessenger implements the Messenger interface for testing.
type mockMessenger struct {
	mu            sync.Mutex
	sentMessages  []sentMessage
	typingCount   int32
	sendErr       error
	typingErr     error
	incomingCh    chan IncomingMessage
}

type sentMessage struct {
	recipient string
	message   string
}

func (m *mockMessenger) Send(ctx context.Context, recipient string, message string) error {
	m.mu.Lock()
	m.sentMessages = append(m.sentMessages, sentMessage{recipient, message})
	m.mu.Unlock()
	return m.sendErr
}

func (m *mockMessenger) Subscribe(ctx context.Context) (<-chan IncomingMessage, error) {
	if m.incomingCh == nil {
		m.incomingCh = make(chan IncomingMessage)
	}
	return m.incomingCh, nil
}

func (m *mockMessenger) SendTypingIndicator(ctx context.Context, recipient string) error {
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
	defer queueMgr.Shutdown(time.Second)
	
	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	worker := NewWorker(config)
	
	// Create and process message
	msg := NewMessage("msg-1", "conv-1", "user123", "Hello Claude")
	msg.SetState(StateProcessing)
	
	// Add a small delay to LLM to ensure typing indicator starts
	llm.delay = 20 * time.Millisecond
	
	err := worker.Process(ctx, msg)
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
	if messenger.sentMessages[0].recipient != "user123" {
		t.Errorf("Expected recipient 'user123', got %s", messenger.sentMessages[0].recipient)
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
	defer queueMgr.Shutdown(time.Second)
	
	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	worker := NewWorker(config)
	
	// Create and process message
	msg := NewMessage("msg-1", "conv-1", "user123", "Hello")
	msg.SetState(StateProcessing)
	msg.MaxAttempts = 3
	
	err := worker.Process(ctx, msg)
	if err == nil {
		t.Fatal("Expected error from Process")
	}
	
	// Verify message can retry
	if msg.GetState() != StateRetrying {
		t.Errorf("Expected state %s, got %s", StateRetrying, msg.GetState())
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
	defer queueMgr.Shutdown(time.Second)
	
	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	worker := NewWorker(config)
	
	// Create message at max attempts
	msg := NewMessage("msg-1", "conv-1", "user123", "Hello")
	msg.SetState(StateProcessing)
	msg.MaxAttempts = 3
	msg.Attempts = 3
	
	err := worker.Process(ctx, msg)
	if err == nil {
		t.Fatal("Expected error from Process")
	}
	
	// Should be failed, not retrying
	if msg.GetState() != StateFailed {
		t.Errorf("Expected state %s, got %s", StateFailed, msg.GetState())
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
	defer queueMgr.Shutdown(time.Second)
	
	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	worker := NewWorker(config)
	
	// Use up the token
	rateLimiter.Allow("conv-1")
	
	// Process should wait for rate limit
	msg := NewMessage("msg-1", "conv-1", "user123", "Hello")
	msg.SetState(StateProcessing)
	
	start := time.Now()
	err := worker.Process(ctx, msg)
	duration := time.Since(start)
	
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	
	// Should have waited ~100ms for rate limit
	if duration < 90*time.Millisecond {
		t.Errorf("Expected rate limit delay, only waited %v", duration)
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
			fmt.Sprintf("Message %d", i),
		)
		queueMgr.Submit(msg)
	}
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	// Shutdown
	cancel()
	pool.Wait()
	queueMgr.Shutdown(time.Second)
	
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
	defer queueMgr.Shutdown(time.Second)
	
	// Create worker
	config := WorkerConfig{
		ID:           1,
		LLM:          llm,
		Messenger:    messenger,
		QueueManager: queueMgr,
		RateLimiter:  rateLimiter,
	}
	worker := NewWorker(config)
	
	// Process message
	msg := NewMessage("msg-1", "conv-1", "user123", "Hello")
	msg.SetState(StateProcessing)
	
	err := worker.Process(ctx, msg)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	
	// Should have sent at least 1 typing indicator
	typingCount := atomic.LoadInt32(&messenger.typingCount)
	if typingCount < 1 {
		t.Errorf("Expected at least 1 typing indicator, got %d", typingCount)
	}
}