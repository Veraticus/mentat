package queue_test

import (
	"sync/atomic"
	"testing"

	"github.com/Veraticus/mentat/internal/queue"
)

func TestDefaultPanicHandler(t *testing.T) {
	handler := queue.NewDefaultPanicHandler()

	// Test panic handling
	shouldReplace := handler.HandlePanic("test-worker", "test panic", []byte("stack trace here"))

	// Should always return true (replace worker)
	if !shouldReplace {
		t.Error("Default handler should always return true")
	}
}

func TestNoPanicHandler(t *testing.T) {
	handler := queue.NewNoPanicHandler()

	// Should return false (no replacement)
	shouldReplace := handler.HandlePanic("test-worker", "test panic", []byte("stack"))
	if shouldReplace {
		t.Error("NoPanicHandler should return false to not replace worker")
	}
}

func TestMetricsPanicHandler(t *testing.T) {
	var panicCount atomic.Int32
	var lastWorkerID string
	var lastPanicValue any

	onPanic := func(workerID string, panicValue any) {
		panicCount.Add(1)
		lastWorkerID = workerID
		lastPanicValue = panicValue
	}

	// Test with wrapped default handler
	handler := queue.NewMetricsPanicHandler(queue.NewDefaultPanicHandler(), onPanic)

	shouldReplace := handler.HandlePanic("worker-1", "panic 1", []byte("stack"))
	if !shouldReplace {
		t.Error("Should return true when wrapped handler returns true")
	}

	if panicCount.Load() != 1 {
		t.Errorf("Expected panic count 1, got %d", panicCount.Load())
	}
	if lastWorkerID != "worker-1" {
		t.Errorf("Expected worker ID worker-1, got %s", lastWorkerID)
	}
	if lastPanicValue != "panic 1" {
		t.Errorf("Expected panic value 'panic 1', got %v", lastPanicValue)
	}

	// Test multiple panics
	handler.HandlePanic("worker-2", "panic 2", []byte("stack"))
	if panicCount.Load() != 2 {
		t.Errorf("Expected panic count 2, got %d", panicCount.Load())
	}
}

func TestHandleRecoveredPanic(t *testing.T) {
	tests := []struct {
		name          string
		panicValue    any
		handler       queue.PanicHandler
		expectReplace bool
	}{
		{
			name:          "default handler always replaces",
			panicValue:    "test panic",
			handler:       queue.NewDefaultPanicHandler(),
			expectReplace: true,
		},
		{
			name:          "nil handler uses default",
			panicValue:    "test panic",
			handler:       nil,
			expectReplace: true,
		},
		{
			name:          "custom handler can choose not to replace",
			panicValue:    "test panic",
			handler:       &customPanicHandler{shouldReplace: false},
			expectReplace: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: With slog, we don't need to capture output as it's structured logging
			// The default handler will log via slog which can be configured separately

			shouldReplace := queue.HandleRecoveredPanic("test-worker", tt.panicValue, tt.handler)

			if shouldReplace != tt.expectReplace {
				t.Errorf("Expected replace=%v, got %v", tt.expectReplace, shouldReplace)
			}
		})
	}
}

// TestPanicHandlerIntegration tests panic handling in the context of a worker pool.
func TestPanicHandlerIntegration(t *testing.T) {
	// Test that NoPanicHandler returns false
	t.Run("no panic handler returns false", func(t *testing.T) {
		handler := queue.NewNoPanicHandler()
		shouldReplace := handler.HandlePanic("test-worker", "test panic", []byte("stack"))
		if shouldReplace {
			t.Error("NoPanicHandler should return false")
		}
	})

	// Test custom handler behavior
	t.Run("custom handler", func(t *testing.T) {
		customHandler := &customPanicHandler{shouldReplace: false}

		// Handler should track panics
		shouldReplace := queue.HandleRecoveredPanic("worker-1", "panic 1", customHandler)
		if shouldReplace {
			t.Error("Custom handler should return false")
		}

		if len(customHandler.panics) != 1 {
			t.Errorf("Expected 1 panic tracked, got %d", len(customHandler.panics))
		}

		// Multiple panics should be tracked
		queue.HandleRecoveredPanic("worker-2", "panic 2", customHandler)
		if len(customHandler.panics) != 2 {
			t.Errorf("Expected 2 panics tracked, got %d", len(customHandler.panics))
		}
	})
}

// customPanicHandler for testing custom behavior.
type customPanicHandler struct {
	shouldReplace bool
	panics        []any
}

func (h *customPanicHandler) HandlePanic(_ string, panicValue any, _ []byte) bool {
	h.panics = append(h.panics, panicValue)
	return h.shouldReplace
}
