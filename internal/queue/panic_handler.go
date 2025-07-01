package queue

import (
	"fmt"
	"log"
	"runtime/debug"
)

// PanicHandler defines how to handle worker panics.
type PanicHandler interface {
	// HandlePanic is called when a worker panics.
	// It should return true to replace the worker, false to not replace.
	HandlePanic(workerID string, panicValue any, stackTrace []byte) bool
}

// defaultPanicHandler logs panics with stack traces and always replaces workers.
type defaultPanicHandler struct{}

func (h *defaultPanicHandler) HandlePanic(workerID string, panicValue any, stackTrace []byte) bool {
	log.Printf("PANIC in worker %s: %v\n%s", workerID, panicValue, stackTrace)
	return true // Always replace panicked workers
}

// noPanicHandler disables panic recovery (useful for tests/debugging).
type noPanicHandler struct{}

func (h *noPanicHandler) HandlePanic(workerID string, panicValue any, _ []byte) bool {
	// Re-panic to maintain original behavior
	panic(fmt.Sprintf("Worker %s panicked: %v", workerID, panicValue))
}

// metricsPanicHandler can be used to track panic metrics.
type metricsPanicHandler struct {
	wrapped PanicHandler
	onPanic func(workerID string, panicValue any)
}

func (h *metricsPanicHandler) HandlePanic(workerID string, panicValue any, stackTrace []byte) bool {
	if h.onPanic != nil {
		h.onPanic(workerID, panicValue)
	}
	if h.wrapped != nil {
		return h.wrapped.HandlePanic(workerID, panicValue, stackTrace)
	}
	return true
}

// NewDefaultPanicHandler returns the default panic handler.
func NewDefaultPanicHandler() PanicHandler {
	return &defaultPanicHandler{}
}

// NewNoPanicHandler returns a handler that disables panic recovery.
func NewNoPanicHandler() PanicHandler {
	return &noPanicHandler{}
}

// NewMetricsPanicHandler wraps another handler to add metrics tracking.
func NewMetricsPanicHandler(wrapped PanicHandler, onPanic func(string, any)) PanicHandler {
	return &metricsPanicHandler{
		wrapped: wrapped,
		onPanic: onPanic,
	}
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
