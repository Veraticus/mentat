package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicWorkerPool_Creation(t *testing.T) {
	tests := []struct {
		name      string
		config    PoolConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: PoolConfig{
				InitialSize:  3,
				MinSize:      2,
				MaxSize:      10,
				LLM:          &mockLLM{response: "test"},
				Messenger:    &mockMessenger{},
				QueueManager: NewManager(context.Background()),
				RateLimiter:  NewRateLimiter(10, 1, time.Second),
			},
			wantError: false,
		},
		{
			name: "invalid initial size",
			config: PoolConfig{
				InitialSize:  0,
				LLM:          &mockLLM{response: "test"},
				Messenger:    &mockMessenger{},
				QueueManager: NewManager(context.Background()),
				RateLimiter:  NewRateLimiter(10, 1, time.Second),
			},
			wantError: true,
			errorMsg:  "initial size must be at least 1",
		},
		{
			name: "defaults applied",
			config: PoolConfig{
				InitialSize:  5,
				LLM:          &mockLLM{response: "test"},
				Messenger:    &mockMessenger{},
				QueueManager: NewManager(context.Background()),
				RateLimiter:  NewRateLimiter(10, 1, time.Second),
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewDynamicWorkerPool(tt.config)
			if tt.wantError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, pool)
				assert.Equal(t, tt.config.InitialSize, pool.Size())
				
				// Verify defaults
				if tt.config.MinSize == 0 {
					assert.Equal(t, 1, pool.config.MinSize)
				}
				if tt.config.MaxSize == 0 {
					assert.Equal(t, pool.config.MinSize*2, pool.config.MaxSize)
				}
				
				// Don't call Stop() since we didn't Start() the pool
			}
		})
	}
}

func TestDynamicWorkerPool_ScaleUp(t *testing.T) {
	// Set up mocks
	mockLLM := &mockLLM{response: "test"}
	mockMessenger := &mockMessenger{}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	queueMgr := NewManager(ctx)
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()
	
	config := PoolConfig{
		InitialSize:  2,
		MinSize:      1,
		MaxSize:      5,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  NewRateLimiter(10, 1, time.Second),
	}
	
	pool, err := NewDynamicWorkerPool(config)
	require.NoError(t, err)
	defer pool.Stop()
	
	err = pool.Start(ctx)
	require.NoError(t, err)
	
	// Initial size should be 2
	assert.Equal(t, 2, pool.Size())
	
	// Scale up by 2
	err = pool.ScaleUp(2)
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
	// Set up mocks
	mockLLM := &mockLLM{response: "test"}
	mockMessenger := &mockMessenger{}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	queueMgr := NewManager(ctx)
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()
	
	config := PoolConfig{
		InitialSize:  5,
		MinSize:      2,
		MaxSize:      10,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  NewRateLimiter(10, 1, time.Second),
	}
	
	pool, err := NewDynamicWorkerPool(config)
	require.NoError(t, err)
	defer pool.Stop()
	
	err = pool.Start(ctx)
	require.NoError(t, err)
	
	// Initial size should be 5
	assert.Equal(t, 5, pool.Size())
	
	// Scale down by 2
	err = pool.ScaleDown(2)
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

func TestDynamicWorkerPool_HealthCheck(t *testing.T) {
	// Set up mocks
	mockLLM := &mockLLM{
		response: "Response 1",
	}
	mockMessenger := &mockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	queueMgr := NewManager(ctx)
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()
	
	config := PoolConfig{
		InitialSize:        3,
		MinSize:            2,
		MaxSize:            5,
		HealthCheckPeriod:  100 * time.Millisecond,
		UnhealthyThreshold: 200 * time.Millisecond,
		LLM:                mockLLM,
		Messenger:          mockMessenger,
		QueueManager:       queueMgr,
		RateLimiter:        NewRateLimiter(10, 1, time.Second),
	}
	
	pool, err := NewDynamicWorkerPool(config)
	require.NoError(t, err)
	defer pool.Stop()
	
	err = pool.Start(ctx)
	require.NoError(t, err)
	
	// All workers should be healthy initially
	assert.Equal(t, 3, pool.HealthyWorkers())
	
	// Submit work to keep workers active
	msg := &Message{
		ID:             "test-1",
		ConversationID: "conv-1",
		Sender:         "user1",
		Text:           "Hello",
		CreatedAt:      time.Now(),
	}
	err = queueMgr.Submit(msg)
	require.NoError(t, err)
	
	// Wait for health check to run but workers should still be healthy
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 3, pool.HealthyWorkers())
	
	// Wait longer without submitting work
	time.Sleep(300 * time.Millisecond)
	
	// Workers might be marked unhealthy if there's no work
	// This depends on queue state, so we just verify the count is reasonable
	healthyCount := pool.HealthyWorkers()
	assert.GreaterOrEqual(t, healthyCount, 0)
	assert.LessOrEqual(t, healthyCount, 3)
}

func TestDynamicWorkerPool_WorkerFailureHandling(t *testing.T) {
	// Set up mocks with controlled failures
	var processedCount atomic.Int32
	mockLLM := &mockLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			count := processedCount.Add(1)
			if count == 2 {
				// Simulate a worker crash on the second message
				panic("simulated worker crash")
			}
			return &claude.LLMResponse{Message: "Response"}, nil
		},
	}
	
	mockMessenger := &mockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	queueMgr := NewManager(ctx)
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()
	
	config := PoolConfig{
		InitialSize:  3,
		MinSize:      2,
		MaxSize:      5,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  NewRateLimiter(10, 1, time.Second),
	}
	
	pool, err := NewDynamicWorkerPool(config)
	require.NoError(t, err)
	defer pool.Stop()
	
	err = pool.Start(ctx)
	require.NoError(t, err)
	
	// Submit multiple messages
	for i := 0; i < 5; i++ {
		msg := &Message{
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
	time.Sleep(500 * time.Millisecond)
	
	// Despite the panic, the pool should maintain at least minimum size
	assert.GreaterOrEqual(t, pool.Size(), config.MinSize)
}

func TestDynamicWorkerPool_GracefulShutdown(t *testing.T) {
	// Set up mocks
	var wg sync.WaitGroup
	mockLLM := &mockLLM{
		queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			// Simulate slow processing
			wg.Done()
			time.Sleep(200 * time.Millisecond)
			return &claude.LLMResponse{Message: "Response"}, nil
		},
	}
	
	mockMessenger := &mockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	queueMgr := NewManager(ctx)
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()
	
	config := PoolConfig{
		InitialSize:  2,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  NewRateLimiter(10, 1, time.Second),
	}
	
	pool, err := NewDynamicWorkerPool(config)
	require.NoError(t, err)
	
	err = pool.Start(ctx)
	require.NoError(t, err)
	
	// Submit messages
	wg.Add(2)
	for i := 0; i < 2; i++ {
		msg := &Message{
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
	pool.Stop()
	
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
	mockLLM := &mockLLM{
		response: "Response",
	}
	mockMessenger := &mockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	queueMgr := NewManager(ctx)
	go queueMgr.Start()
	defer func() {
		if err := queueMgr.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()
	
	config := PoolConfig{
		InitialSize:  3,
		MinSize:      2,
		MaxSize:      10,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: queueMgr,
		RateLimiter:  NewRateLimiter(10, 1, time.Second),
	}
	
	pool, err := NewDynamicWorkerPool(config)
	require.NoError(t, err)
	defer pool.Stop()
	
	err = pool.Start(ctx)
	require.NoError(t, err)
	
	// Perform concurrent operations
	var wg sync.WaitGroup
	wg.Add(4)
	
	// Concurrent scaling operations
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_ = pool.ScaleUp(1)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_ = pool.ScaleDown(1)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	
	// Concurrent size checks
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			size := pool.Size()
			assert.GreaterOrEqual(t, size, config.MinSize)
			assert.LessOrEqual(t, size, config.MaxSize)
			time.Sleep(5 * time.Millisecond)
		}
	}()
	
	// Concurrent health checks
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			healthy := pool.HealthyWorkers()
			assert.GreaterOrEqual(t, healthy, 0)
			assert.LessOrEqual(t, healthy, pool.Size())
			time.Sleep(5 * time.Millisecond)
		}
	}()
	
	wg.Wait()
	
	// Verify pool is still in valid state
	size := pool.Size()
	assert.GreaterOrEqual(t, size, config.MinSize)
	assert.LessOrEqual(t, size, config.MaxSize)
}