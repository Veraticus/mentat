package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
)

// TestDynamicWorkerPool_PanicHandling tests various panic handling scenarios.
func TestDynamicWorkerPool_PanicHandling(t *testing.T) {
	t.Run("default panic handler recovers and replaces", func(t *testing.T) {
		var panicCount atomic.Int32
		mockLLM := &mockLLM{
			queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				if panicCount.Add(1) == 1 {
					panic("first worker panic")
				}
				return &claude.LLMResponse{Message: "Response"}, nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		queueMgr := NewManager(ctx)
		go queueMgr.Start()
		defer func() {
			_ = queueMgr.Shutdown(time.Second)
		}()

		config := PoolConfig{
			InitialSize:  2,
			MinSize:      2,
			MaxSize:      4,
			LLM:          mockLLM,
			Messenger:    &mockMessenger{},
			QueueManager: queueMgr,
			RateLimiter:  DefaultRateLimiter(),
			// Uses default panic handler
		}

		pool, err := NewDynamicWorkerPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Stop()

		err = pool.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start pool: %v", err)
		}

		// Submit a message that will cause panic
		msg := &Message{
			ID:             "test-1",
			ConversationID: "conv-1",
			Sender:         "user1",
			Text:           "Hello",
			CreatedAt:      time.Now(),
		}
		err = queueMgr.Submit(msg)
		if err != nil {
			t.Fatalf("Failed to submit message: %v", err)
		}

		// Allow panic handler to process
		<-time.After(200 * time.Millisecond)

		// Pool should maintain minimum size
		if pool.Size() < config.MinSize {
			t.Errorf("Pool size %d is below minimum %d after panic", pool.Size(), config.MinSize)
		}
	})

	t.Run("custom panic handler with metrics", func(t *testing.T) {
		var panicMetrics struct {
			mu     sync.Mutex
			panics []struct {
				workerID   string
				panicValue any
				timestamp  time.Time
			}
		}

		// Create a custom handler that tracks panics
		customHandler := NewMetricsPanicHandler(
			NewDefaultPanicHandler(),
			func(workerID string, panicValue any) {
				panicMetrics.mu.Lock()
				defer panicMetrics.mu.Unlock()
				panicMetrics.panics = append(panicMetrics.panics, struct {
					workerID   string
					panicValue any
					timestamp  time.Time
				}{
					workerID:   workerID,
					panicValue: panicValue,
					timestamp:  time.Now(),
				})
			},
		)

		var callCount atomic.Int32
		mockLLM := &mockLLM{
			queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				count := callCount.Add(1)
				if count <= 2 {
					panic("metrics test panic")
				}
				return &claude.LLMResponse{Message: "Response"}, nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		queueMgr := NewManager(ctx)
		go queueMgr.Start()
		defer func() {
			_ = queueMgr.Shutdown(time.Second)
		}()

		config := PoolConfig{
			InitialSize:  2,
			LLM:          mockLLM,
			Messenger:    &mockMessenger{},
			QueueManager: queueMgr,
			RateLimiter:  DefaultRateLimiter(),
			PanicHandler: customHandler,
		}

		pool, err := NewDynamicWorkerPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Stop()

		err = pool.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start pool: %v", err)
		}

		// Submit messages that will cause panics
		for i := 0; i < 2; i++ {
			msg := &Message{
				ID:             "test-" + string(rune('0'+i)),
				ConversationID: "conv-" + string(rune('0'+i)),
				Sender:         "user1",
				Text:           "Message",
				CreatedAt:      time.Now(),
			}
			_ = queueMgr.Submit(msg)
		}

		// Allow panics to be processed
		<-time.After(300 * time.Millisecond)

		// Check metrics
		panicMetrics.mu.Lock()
		panicCount := len(panicMetrics.panics)
		panicMetrics.mu.Unlock()

		if panicCount < 2 {
			t.Errorf("Expected at least 2 panics to be tracked, got %d", panicCount)
		}

		// Verify all panics were tracked with correct value
		panicMetrics.mu.Lock()
		for i, p := range panicMetrics.panics {
			if p.panicValue != "metrics test panic" {
				t.Errorf("Panic %d has wrong value: %v", i, p.panicValue)
			}
			if p.workerID == "" {
				t.Error("Panic missing worker ID")
			}
		}
		panicMetrics.mu.Unlock()
	})

	t.Run("no panic handler for testing", func(t *testing.T) {
		// Skip this test in normal runs as it intentionally causes a panic
		// to test that NoPanicHandler allows panics to propagate
		t.Skip("Skipping panic propagation test - causes test suite failure by design")

		// Original test code for reference:
		// This test demonstrates using NoPanicHandler in tests
		// to ensure panics are not silently swallowed
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic to propagate with NoPanicHandler")
			}
		}()

		mockLLM := &mockLLM{
			queryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				panic("test panic - should propagate")
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		queueMgr := NewManager(ctx)
		go queueMgr.Start()
		defer func() {
			_ = queueMgr.Shutdown(time.Second)
		}()

		config := PoolConfig{
			InitialSize:  1,
			LLM:          mockLLM,
			Messenger:    &mockMessenger{},
			QueueManager: queueMgr,
			RateLimiter:  DefaultRateLimiter(),
			PanicHandler: NewNoPanicHandler(), // Disables recovery
		}

		pool, err := NewDynamicWorkerPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}

		err = pool.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start pool: %v", err)
		}

		// Submit a message that will cause panic
		msg := &Message{
			ID:             "test-1",
			ConversationID: "conv-1",
			Sender:         "user1",
			Text:           "Hello",
			CreatedAt:      time.Now(),
		}
		_ = queueMgr.Submit(msg)

		// Allow panic to propagate
		<-time.After(100 * time.Millisecond)
		pool.Stop()
		pool.Wait()
	})
}
