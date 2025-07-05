// Package agent provides the multi-agent validation system for processing messages.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// ValidationResultHandler defines the interface for handling validation results.
// Implementations determine what actions to take based on validation outcomes.
type ValidationResultHandler interface {
	// HandleResult processes a validation result and takes appropriate action.
	// This may include sending follow-up messages, logging, or other responses.
	HandleResult(ctx context.Context, result ValidationResult, task *ValidationTask) error

	// ShouldHandle determines if this handler should process the given result.
	// This allows for selective handling based on validation status or other criteria.
	ShouldHandle(result ValidationResult) bool
}

// HandlerFunc is a function type that can handle a specific validation result.
type HandlerFunc func(ctx context.Context, result ValidationResult, task *ValidationTask) error

// DefaultResultHandler provides standard handling for validation results.
// It sends appropriate follow-up messages based on validation status.
type DefaultResultHandler struct {
	messenger          signal.Messenger
	validationStrategy ValidationStrategy
	logger             *slog.Logger

	// Configurable delays for different types of follow-ups
	correctionDelay time.Duration
	clarifyDelay    time.Duration

	// Custom handlers for specific statuses (optional)
	statusHandlers map[ValidationStatus]HandlerFunc
}

// NewDefaultResultHandler creates a new result handler with standard configuration.
func NewDefaultResultHandler(
	messenger signal.Messenger,
	strategy ValidationStrategy,
	logger *slog.Logger,
) *DefaultResultHandler {
	return &DefaultResultHandler{
		messenger:          messenger,
		validationStrategy: strategy,
		logger:             logger,
		correctionDelay:    correctionDelay, // From constants
		clarifyDelay:       time.Second,     // Faster for clarifications
		statusHandlers:     make(map[ValidationStatus]HandlerFunc),
	}
}

// HandleResult processes the validation result with exhaustive status handling.
func (h *DefaultResultHandler) HandleResult(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
) error {
	h.logger.DebugContext(ctx, "handling validation result",
		slog.String("task_id", task.ID),
		slog.String("status", string(result.Status)),
		slog.Float64("confidence", result.Confidence))

	// Check for custom handler first
	if handler, exists := h.statusHandlers[result.Status]; exists {
		return handler(ctx, result, task)
	}

	// Exhaustive switch for all validation statuses
	switch result.Status {
	case ValidationStatusSuccess:
		return h.handleSuccess(ctx, result, task)

	case ValidationStatusPartial:
		return h.handlePartialCompletion(ctx, result, task)

	case ValidationStatusFailed:
		return h.handleFailure(ctx, result, task)

	case ValidationStatusIncompleteSearch:
		return h.handleIncompleteSearch(ctx, result, task)

	case ValidationStatusUnclear:
		return h.handleUnclearRequest(ctx, result, task)

	default:
		// Log unknown status but don't fail
		h.logger.WarnContext(ctx, "unknown validation status",
			slog.String("status", string(result.Status)),
			slog.String("task_id", task.ID))
		return nil
	}
}

// ShouldHandle determines if this handler should process the result.
func (h *DefaultResultHandler) ShouldHandle(result ValidationResult) bool {
	// Handle everything except success by default
	return result.Status != ValidationStatusSuccess
}

// handleSuccess processes successful validations (typically no action needed).
func (h *DefaultResultHandler) handleSuccess(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
) error {
	h.logger.InfoContext(ctx, "validation succeeded, no follow-up needed",
		slog.String("task_id", task.ID),
		slog.Float64("confidence", result.Confidence))
	return nil
}

// handlePartialCompletion sends a follow-up for partial completions.
func (h *DefaultResultHandler) handlePartialCompletion(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
) error {
	return h.sendFollowUp(ctx, result, task, h.correctionDelay, "partial completion")
}

// handleFailure sends a follow-up for failed validations.
func (h *DefaultResultHandler) handleFailure(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
) error {
	return h.sendFollowUp(ctx, result, task, h.correctionDelay, "validation failure")
}

// handleIncompleteSearch sends a follow-up for incomplete searches.
func (h *DefaultResultHandler) handleIncompleteSearch(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
) error {
	return h.sendFollowUp(ctx, result, task, h.correctionDelay, "incomplete search")
}

// handleUnclearRequest sends a clarification request.
func (h *DefaultResultHandler) handleUnclearRequest(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
) error {
	// Use shorter delay for clarifications
	return h.sendFollowUp(ctx, result, task, h.clarifyDelay, "unclear request")
}

// sendFollowUp generates and sends a follow-up message after the specified delay.
func (h *DefaultResultHandler) sendFollowUp(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
	delay time.Duration,
	reason string,
) error {
	// Generate the follow-up message
	message := h.validationStrategy.GenerateRecovery(
		ctx,
		task.OriginalRequest,
		task.Response.Message,
		task.SessionID,
		result,
		nil, // LLM will be injected by strategy
	)

	if message == "" {
		h.logger.DebugContext(ctx, "no follow-up message generated",
			slog.String("task_id", task.ID),
			slog.String("reason", reason))
		return nil
	}

	// Apply delay to ensure natural conversation flow
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			// Timer expired, proceed
		case <-ctx.Done():
			// Context canceled
			return fmt.Errorf("follow-up canceled: %w", ctx.Err())
		}
	}

	// Send the follow-up message
	if err := h.messenger.Send(ctx, task.To, message); err != nil {
		h.logger.ErrorContext(ctx, "failed to send follow-up message",
			slog.Any("error", err),
			slog.String("task_id", task.ID),
			slog.String("to", task.To),
			slog.String("reason", reason))
		return fmt.Errorf("send follow-up: %w", err)
	}

	h.logger.InfoContext(ctx, "sent follow-up message",
		slog.String("task_id", task.ID),
		slog.String("to", task.To),
		slog.String("reason", reason),
		slog.String("validation_status", string(result.Status)))

	return nil
}

// SetStatusHandler allows setting a custom handler for a specific validation status.
// This enables extensibility for specialized handling requirements.
func (h *DefaultResultHandler) SetStatusHandler(status ValidationStatus, handler HandlerFunc) {
	h.statusHandlers[status] = handler
}

// SetCorrectionDelay updates the delay before sending correction messages.
func (h *DefaultResultHandler) SetCorrectionDelay(delay time.Duration) {
	h.correctionDelay = delay
}

// SetClarifyDelay updates the delay before sending clarification requests.
func (h *DefaultResultHandler) SetClarifyDelay(delay time.Duration) {
	h.clarifyDelay = delay
}

// ChainedResultHandler allows multiple handlers to process results in sequence.
// This enables composing different handling behaviors.
type ChainedResultHandler struct {
	handlers []ValidationResultHandler
	logger   *slog.Logger
}

// NewChainedResultHandler creates a handler that runs multiple handlers in sequence.
func NewChainedResultHandler(logger *slog.Logger, handlers ...ValidationResultHandler) *ChainedResultHandler {
	return &ChainedResultHandler{
		handlers: handlers,
		logger:   logger,
	}
}

// HandleResult runs all handlers that should handle the result.
func (c *ChainedResultHandler) HandleResult(
	ctx context.Context,
	result ValidationResult,
	task *ValidationTask,
) error {
	for i, handler := range c.handlers {
		if handler.ShouldHandle(result) {
			if err := handler.HandleResult(ctx, result, task); err != nil {
				c.logger.ErrorContext(ctx, "handler failed in chain",
					slog.Int("handler_index", i),
					slog.Any("error", err))
				// Continue with other handlers even if one fails
			}
		}
	}
	return nil
}

// ShouldHandle returns true if any handler should handle the result.
func (c *ChainedResultHandler) ShouldHandle(result ValidationResult) bool {
	for _, handler := range c.handlers {
		if handler.ShouldHandle(result) {
			return true
		}
	}
	return false
}
