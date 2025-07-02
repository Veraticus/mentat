package queue

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// PanicHandler defines how to handle worker panics.
type PanicHandler interface {
	// HandlePanic is called when a worker panics.
	// It should return true to replace the worker, false to not replace.
	HandlePanic(workerID string, panicValue any, stackTrace []byte) bool
}

// DefaultPanicHandler logs panics with stack traces and always replaces workers.
type DefaultPanicHandler struct{}

// NewDefaultPanicHandler returns the default panic handler.
func NewDefaultPanicHandler() *DefaultPanicHandler {
	return &DefaultPanicHandler{}
}

// HandlePanic logs the panic with stack trace and returns true to replace the worker.
func (h *DefaultPanicHandler) HandlePanic(workerID string, panicValue any, stackTrace []byte) bool {
	logger := slog.Default()
	logger.ErrorContext(context.Background(), "PANIC in worker",
		slog.String("worker_id", workerID),
		slog.Any("panic", panicValue),
		slog.String("stack_trace", string(stackTrace)))
	return true // Always replace panicked workers
}

// NoPanicHandler disables panic recovery (useful for tests/debugging).
type NoPanicHandler struct{}

// NewNoPanicHandler returns a handler that disables panic recovery.
func NewNoPanicHandler() *NoPanicHandler {
	return &NoPanicHandler{}
}

// HandlePanic logs the panic and returns false to not replace the worker.
func (h *NoPanicHandler) HandlePanic(workerID string, panicValue any, _ []byte) bool {
	// For debugging, log and return false to not replace the worker
	logger := slog.Default()
	logger.ErrorContext(
		context.Background(),
		"Worker panic (no replacement)",
		slog.String("worker_id", workerID),
		slog.Any("panic", panicValue),
	)
	return false
}

// MetricsPanicHandler can be used to track panic metrics.
type MetricsPanicHandler struct {
	wrapped PanicHandler
	onPanic func(workerID string, panicValue any)
}

// NewMetricsPanicHandler wraps another handler to add metrics tracking.
func NewMetricsPanicHandler(wrapped PanicHandler, onPanic func(string, any)) *MetricsPanicHandler {
	return &MetricsPanicHandler{
		wrapped: wrapped,
		onPanic: onPanic,
	}
}

// HandlePanic calls the metrics callback and delegates to the wrapped handler.
func (h *MetricsPanicHandler) HandlePanic(workerID string, panicValue any, stackTrace []byte) bool {
	if h.onPanic != nil {
		h.onPanic(workerID, panicValue)
	}
	if h.wrapped != nil {
		return h.wrapped.HandlePanic(workerID, panicValue, stackTrace)
	}
	return true
}

// HandleRecoveredPanic processes a panic that was recovered.
// Returns true if the worker should be replaced.
func HandleRecoveredPanic(workerID string, panicValue any, handler PanicHandler) bool {
	if handler == nil {
		handler = NewDefaultPanicHandler()
	}
	stackTrace := debug.Stack()
	return handler.HandlePanic(workerID, panicValue, stackTrace)
}
