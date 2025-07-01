package signal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// MessageEnqueuer defines the interface for enqueuing messages.
type MessageEnqueuer interface {
	Enqueue(msg IncomingMessage) error
}

// Handler manages Signal message reception and queue integration.
type Handler struct {
	messenger Messenger
	queue     MessageEnqueuer
	logger    *slog.Logger
	wg        sync.WaitGroup
	mu        sync.RWMutex
	running   bool
}

// HandlerOption configures the handler.
type HandlerOption func(*Handler)

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.logger = logger
	}
}

// NewHandler creates a new Signal handler.
func NewHandler(messenger Messenger, queue MessageEnqueuer, opts ...HandlerOption) (*Handler, error) {
	if messenger == nil {
		return nil, fmt.Errorf("messenger is required")
	}
	if queue == nil {
		return nil, fmt.Errorf("queue is required")
	}

	h := &Handler{
		messenger: messenger,
		queue:     queue,
		logger:    slog.Default(),
	}

	for _, opt := range opts {
		opt(h)
	}

	return h, nil
}

// Start begins processing Signal messages.
func (h *Handler) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return fmt.Errorf("handler already running")
	}
	h.running = true
	h.mu.Unlock()

	// Get the message channel from Signal
	messages, err := h.messenger.Subscribe(ctx)
	if err != nil {
		h.mu.Lock()
		h.running = false
		h.mu.Unlock()
		return fmt.Errorf("failed to subscribe to messages: %w", err)
	}

	h.logger.Info("signal handler started")

	// Start the message processing goroutine
	h.wg.Add(1)
	go h.processMessages(ctx, messages)

	// Wait for context cancellation
	<-ctx.Done()

	h.logger.Info("signal handler stopping")

	// Mark as not running
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()

	// Wait for message processor to finish
	h.wg.Wait()

	h.logger.Info("signal handler stopped")
	return nil
}

// processMessages handles incoming messages from Signal.
func (h *Handler) processMessages(ctx context.Context, messages <-chan IncomingMessage) {
	defer h.wg.Done()

	for {
		select {
		case <-ctx.Done():
			h.logger.Debug("message processor stopping due to context cancellation")
			return

		case msg, ok := <-messages:
			if !ok {
				h.logger.Debug("message channel closed")
				return
			}

			// Process message without blocking
			h.handleMessage(msg)
		}
	}
}

// handleMessage enqueues a single message without blocking.
func (h *Handler) handleMessage(msg IncomingMessage) {
	h.logger.Debug("received message",
		"from", msg.From,
		"timestamp", msg.Timestamp,
		"text_length", len(msg.Text))

	// Enqueue the message
	if err := h.queue.Enqueue(msg); err != nil {
		h.logger.Error("failed to enqueue message",
			"error", err,
			"from", msg.From,
			"timestamp", msg.Timestamp)
		// Continue processing other messages
		return
	}

	h.logger.Debug("message enqueued successfully",
		"from", msg.From,
		"timestamp", msg.Timestamp)
}

// IsRunning returns whether the handler is currently running.
func (h *Handler) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}
