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
	h.logger.InfoContext(ctx, "subscribing to Signal messages...")
	messages, err := h.messenger.Subscribe(ctx)
	if err != nil {
		h.mu.Lock()
		h.running = false
		h.mu.Unlock()
		return fmt.Errorf("failed to subscribe to messages: %w", err)
	}
	h.logger.InfoContext(ctx, "successfully subscribed to Signal messages")

	h.logger.InfoContext(ctx, "signal handler started")

	// Start the message processing goroutine
	h.wg.Add(1)
	go h.processMessages(ctx, messages)

	// Wait for context cancellation
	<-ctx.Done()

	h.logger.InfoContext(ctx, "signal handler stopping")

	// Mark as not running
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()

	// Wait for message processor to finish
	h.wg.Wait()

	h.logger.InfoContext(ctx, "signal handler stopped")
	return nil
}

// processMessages handles incoming messages from Signal.
func (h *Handler) processMessages(ctx context.Context, messages <-chan IncomingMessage) {
	defer h.wg.Done()

	for {
		select {
		case <-ctx.Done():
			h.logger.DebugContext(ctx, "message processor stopping due to context cancellation")
			return

		case msg, ok := <-messages:
			if !ok {
				h.logger.DebugContext(ctx, "message channel closed")
				return
			}

			// Process message without blocking
			h.handleMessage(ctx, msg)
		}
	}
}

// handleMessage enqueues a single message without blocking.
func (h *Handler) handleMessage(ctx context.Context, msg IncomingMessage) {
	// Log at INFO level to make sure we see it
	h.logger.InfoContext(ctx, "received message",
		slog.String("from", msg.From),
		slog.Time("timestamp", msg.Timestamp),
		slog.Int("text_length", len(msg.Text)),
		slog.String("text", msg.Text))

	// Enqueue the message
	if err := h.queue.Enqueue(msg); err != nil {
		h.logger.ErrorContext(ctx, "failed to enqueue message",
			slog.Any("error", err),
			slog.String("from", msg.From),
			slog.Time("timestamp", msg.Timestamp))
		// Continue processing other messages
		return
	}

	h.logger.InfoContext(ctx, "message enqueued successfully",
		slog.String("from", msg.From),
		slog.Time("timestamp", msg.Timestamp))
}

// IsRunning returns whether the handler is currently running.
func (h *Handler) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}
