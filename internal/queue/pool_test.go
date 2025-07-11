package queue_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// poolMockLLM implements the LLM interface for testing.
type poolMockLLM struct {
	queryFunc func(ctx context.Context, message, sessionID string) (*claude.LLMResponse, error)
}

func (m *poolMockLLM) Query(ctx context.Context, message, sessionID string) (*claude.LLMResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, message, sessionID)
	}
	return &claude.LLMResponse{Message: "Mock response"}, nil
}

// poolMockMessenger implements the Messenger interface for testing.
type poolMockMessenger struct {
	sendFunc func(ctx context.Context, to, message string) error
}

func (m *poolMockMessenger) Send(ctx context.Context, to, message string) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, to, message)
	}
	return nil
}

func (m *poolMockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	// Mock implementation - just return nil
	return nil
}

func (m *poolMockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	// Mock implementation - return closed channel
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}

func TestDynamicWorkerPool_Creation(t *testing.T) {
	tests := []struct {
		name      string
		config    queue.PoolConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: queue.PoolConfig{
				InitialSize: 3,
				MinSize:     2,
				MaxSize:     10,
				LLM: &poolMockLLM{queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{Message: "test"}, nil
				}},
				Messenger:    &poolMockMessenger{},
				QueueManager: queue.NewManager(context.Background()),
				RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
			},
			wantError: false,
		},
		{
			name: "invalid initial size",
			config: queue.PoolConfig{
				InitialSize: 0,
				LLM: &poolMockLLM{queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{Message: "test"}, nil
				}},
				Messenger:    &poolMockMessenger{},
				QueueManager: queue.NewManager(context.Background()),
				RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
			},
			wantError: true,
			errorMsg:  "initial size must be at least 1",
		},
		{
			name: "defaults applied",
			config: queue.PoolConfig{
				InitialSize: 5,
				LLM: &poolMockLLM{queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{Message: "test"}, nil
				}},
				Messenger:    &poolMockMessenger{},
				QueueManager: queue.NewManager(context.Background()),
				RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := queue.NewDynamicWorkerPool(context.Background(), tt.config)
			if tt.wantError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, pool)
				assert.Equal(t, tt.config.InitialSize, pool.Size())

				// Verify defaults are applied correctly by checking the pool size
				// We can't access pool.config directly as it's unexported

				// Don't call Stop() since we didn't Start() the pool
			}
		})
	}
}

// setupPoolTest creates a test pool with the given configuration.
func setupPoolTest(
	t *testing.T,
	initialSize, minSize, maxSize int,
) (*queue.DynamicWorkerPool, context.CancelFunc) {
	t.Helper()
	// Set up mocks
	mockLLM := &poolMockLLM{queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
		return &claude.LLMResponse{Message: "test"}, nil
	}}
	mockMessenger := &poolMockMessenger{}

	ctx, cancel := context.WithCancel(context.Background())

	queueMgr := queue.NewManager(ctx)
	go queueMgr.Start(ctx)
	t.Cleanup(func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	})

	config := queue.PoolConfig{
		InitialSize:  initialSize,
		MinSize:      minSize,
		MaxSize:      maxSize,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
	}

	pool, err := queue.NewDynamicWorkerPool(context.Background(), config)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Stop(context.Background())
	})

	err = pool.Start(ctx)
	require.NoError(t, err)

	return pool, cancel
}

func TestDynamicWorkerPool_ScaleUp(t *testing.T) {
	pool, cancel := setupPoolTest(t, 2, 1, 5)
	defer cancel()

	// Initial size should be 2
	assert.Equal(t, 2, pool.Size())

	// Scale up by 2
	err := pool.ScaleUp(2)
	require.NoError(t, err)
	assert.Equal(t, 4, pool.Size())

	// Try to scale beyond max
	err = pool.ScaleUp(3)
	require.NoError(t, err) // Should succeed but only add 1
	assert.Equal(t, 5, pool.Size())

	// Try to scale when at max
	err = pool.ScaleUp(1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool at maximum size")
}

func TestDynamicWorkerPool_ScaleDown(t *testing.T) {
	pool, cancel := setupPoolTest(t, 5, 2, 10)
	defer cancel()

	// Initial size should be 5
	assert.Equal(t, 5, pool.Size())

	// Scale down by 2
	err := pool.ScaleDown(2)
	require.NoError(t, err)
	assert.Equal(t, 3, pool.Size())

	// Try to scale below minimum
	err = pool.ScaleDown(2)
	require.NoError(t, err) // Should succeed but only remove 1
	assert.Equal(t, 2, pool.Size())

	// Try to scale when at minimum
	err = pool.ScaleDown(1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool at minimum size")
}

func TestDynamicWorkerPool_WorkerFailureHandling(t *testing.T) {
	// Set up mocks with controlled failures
	var processedCount atomic.Int32
	mockLLM := &poolMockLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			count := processedCount.Add(1)
			if count == 2 {
				// Simulate a worker crash on the second message
				// Intentionally panic to test worker recovery mechanism
				panic("simulated worker crash") //nolint:forbidigo // Testing worker recovery
			}
			return &claude.LLMResponse{Message: "Response"}, nil
		},
	}

	mockMessenger := &poolMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queueMgr := queue.NewManager(ctx)
	go queueMgr.Start(ctx)
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	config := queue.PoolConfig{
		InitialSize:  3,
		MinSize:      2,
		MaxSize:      5,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
	}

	pool, err := queue.NewDynamicWorkerPool(context.Background(), config)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	err = pool.Start(ctx)
	require.NoError(t, err)

	// Submit multiple messages
	for i := range 5 {
		msg := &queue.Message{
			ID:             fmt.Sprintf("test-%d", i),
			ConversationID: fmt.Sprintf("conv-%d", i%2),
			Sender:         "user1",
			Text:           fmt.Sprintf("Message %d", i),
			CreatedAt:      time.Now(),
		}
		err = queueMgr.Submit(msg)
		require.NoError(t, err)
	}

	// Wait for processing
	<-time.After(500 * time.Millisecond)

	// Despite the panic, the pool should maintain at least minimum size
	assert.GreaterOrEqual(t, pool.Size(), config.MinSize)
}

func TestDynamicWorkerPool_GracefulShutdown(t *testing.T) {
	// Set up mocks
	var wg sync.WaitGroup
	mockLLM := &poolMockLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			// Simulate slow processing
			wg.Done()
			<-time.After(200 * time.Millisecond)
			return &claude.LLMResponse{Message: "Response"}, nil
		},
	}

	mockMessenger := &poolMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queueMgr := queue.NewManager(ctx)
	go queueMgr.Start(ctx)
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	config := queue.PoolConfig{
		InitialSize:  2,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
	}

	pool, err := queue.NewDynamicWorkerPool(context.Background(), config)
	require.NoError(t, err)

	err = pool.Start(ctx)
	require.NoError(t, err)

	// Submit messages
	wg.Add(2)
	for i := range 2 {
		msg := &queue.Message{
			ID:             fmt.Sprintf("test-%d", i),
			ConversationID: fmt.Sprintf("conv-%d", i),
			Sender:         "user1",
			Text:           fmt.Sprintf("Message %d", i),
			CreatedAt:      time.Now(),
		}
		err = queueMgr.Submit(msg)
		require.NoError(t, err)
	}

	// Wait for workers to start processing
	wg.Wait()

	// Cancel context and stop pool
	cancel()
	pool.Stop(context.Background())

	// Wait should complete without hanging
	done := make(chan bool)
	go func() {
		pool.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Pool failed to shut down gracefully")
	}
}

func TestDynamicWorkerPool_ConcurrentOperations(t *testing.T) {
	// Set up mocks
	mockLLM := &poolMockLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			return &claude.LLMResponse{Message: "Response"}, nil
		},
	}
	mockMessenger := &poolMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queueMgr := queue.NewManager(ctx)
	go queueMgr.Start(ctx)
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	config := queue.PoolConfig{
		InitialSize:  3,
		MinSize:      2,
		MaxSize:      10,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
	}

	pool, err := queue.NewDynamicWorkerPool(context.Background(), config)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	err = pool.Start(ctx)
	require.NoError(t, err)

	// Perform concurrent operations
	var wg sync.WaitGroup
	wg.Add(4)

	// Concurrent scaling operations
	go func() {
		defer wg.Done()
		for range 5 {
			_ = pool.ScaleUp(1)
			<-time.After(10 * time.Millisecond)
		}
	}()

	go func() {
		defer wg.Done()
		for range 5 {
			_ = pool.ScaleDown(1)
			<-time.After(10 * time.Millisecond)
		}
	}()

	// Concurrent size checks
	go func() {
		defer wg.Done()
		for range 10 {
			size := pool.Size()
			assert.GreaterOrEqual(t, size, config.MinSize)
			assert.LessOrEqual(t, size, config.MaxSize)
			<-time.After(5 * time.Millisecond)
		}
	}()

	// Concurrent size checks
	go func() {
		defer wg.Done()
		for range 10 {
			size := pool.Size()
			assert.GreaterOrEqual(t, size, 0)
			assert.LessOrEqual(t, size, config.MaxSize)
			<-time.After(5 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Verify pool is still in valid state
	size := pool.Size()
	assert.GreaterOrEqual(t, size, config.MinSize)
	assert.LessOrEqual(t, size, config.MaxSize)
}
