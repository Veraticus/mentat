// Package agent provides the multi-agent validation system for processing messages.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// AsyncValidator manages background validation tasks with proper lifecycle management.
type AsyncValidator struct {
	messenger          signal.Messenger
	validationStrategy ValidationStrategy
	logger             *slog.Logger

	// Track active validations for cleanup
	mu         sync.Mutex
	active     map[string]*ValidationTask
	shutdownCh chan struct{}
	wg         sync.WaitGroup

	// Configurable delay before sending corrections
	correctionDelay time.Duration
}

// ValidationTask represents a single validation operation running in the background.
type ValidationTask struct {
	ID              string
	To              string
	OriginalRequest string
	Response        *claude.LLMResponse
	SessionID       string
	Cancel          context.CancelFunc
	StartTime       time.Time
}

// NewAsyncValidator creates a new async validator with the given dependencies.
func NewAsyncValidator(
	messenger signal.Messenger,
	strategy ValidationStrategy,
	logger *slog.Logger,
) *AsyncValidator {
	return &AsyncValidator{
		messenger:          messenger,
		validationStrategy: strategy,
		logger:             logger,
		active:             make(map[string]*ValidationTask),
		shutdownCh:         make(chan struct{}),
		correctionDelay:    correctionDelay, // Use default from constants
	}
}

// StartValidation launches a background validation task that continues after the main request.
// It returns immediately, and the validation runs independently.
func (av *AsyncValidator) StartValidation(
	to, originalRequest string,
	response *claude.LLMResponse,
	sessionID string,
) {
	// Create a unique task ID
	taskID := sessionID + "-" + time.Now().Format("20060102-150405")

	// Create independent background context with timeout
	backgroundCtx, cancel := context.WithTimeout(context.Background(), validationTimeout)

	// Create validation task
	task := &ValidationTask{
		ID:              taskID,
		To:              to,
		OriginalRequest: originalRequest,
		Response:        response,
		SessionID:       sessionID,
		Cancel:          cancel,
		StartTime:       time.Now(),
	}

	// Register the task
	av.mu.Lock()
	av.active[taskID] = task
	av.mu.Unlock()

	// Increment wait group for tracking
	av.wg.Add(1)

	// Launch validation in background
	go av.runValidation(backgroundCtx, task)
}

// runValidation performs the actual validation work in the background.
func (av *AsyncValidator) runValidation(ctx context.Context, task *ValidationTask) {
	defer av.wg.Done()
	defer task.Cancel() // Ensure context is canceled when done

	// Remove task from active list when complete
	defer func() {
		av.mu.Lock()
		delete(av.active, task.ID)
		av.mu.Unlock()
	}()

	av.logger.InfoContext(ctx, "starting async validation",
		slog.String("task_id", task.ID),
		slog.String("session_id", task.SessionID),
		slog.String("to", task.To))

	// Check if shutdown was requested
	select {
	case <-av.shutdownCh:
		av.logger.InfoContext(ctx, "validation canceled due to shutdown",
			slog.String("task_id", task.ID))
		return
	default:
		// Continue with validation
	}

	// Perform validation
	validationResult := av.validationStrategy.Validate(
		ctx,
		task.OriginalRequest,
		task.Response.Message,
		task.SessionID,
		nil, // LLM will be injected by the strategy if needed
	)

	av.logger.DebugContext(ctx, "async validation complete",
		slog.String("task_id", task.ID),
		slog.String("session_id", task.SessionID),
		slog.String("status", string(validationResult.Status)),
		slog.Float64("confidence", validationResult.Confidence),
		slog.Any("issues", validationResult.Issues))

	// Check if we need to send a correction
	if av.shouldSendCorrection(validationResult) {
		av.sendCorrection(ctx, task, validationResult)
	}

	av.logger.InfoContext(ctx, "async validation task complete",
		slog.String("task_id", task.ID),
		slog.String("session_id", task.SessionID),
		slog.Duration("duration", time.Since(task.StartTime)),
		slog.Bool("correction_sent", av.shouldSendCorrection(validationResult)))
}

// sendCorrection generates and sends a correction message if needed.
func (av *AsyncValidator) sendCorrection(
	ctx context.Context,
	task *ValidationTask,
	validationResult ValidationResult,
) {
	// Generate correction message
	correctionMsg := av.validationStrategy.GenerateRecovery(
		ctx,
		task.OriginalRequest,
		task.Response.Message,
		task.SessionID,
		validationResult,
		nil, // LLM will be injected by the strategy if needed
	)

	if correctionMsg == "" {
		av.logger.DebugContext(ctx, "no correction message generated",
			slog.String("task_id", task.ID))
		return
	}

	// Small delay before sending correction to ensure user has read initial response
	timer := time.NewTimer(av.correctionDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Timer expired, proceed with sending
	case <-ctx.Done():
		// Context canceled, exit early
		av.logger.WarnContext(ctx, "validation canceled before sending correction",
			slog.String("task_id", task.ID))
		return
	case <-av.shutdownCh:
		// Shutdown requested, exit early
		av.logger.WarnContext(ctx, "shutdown requested before sending correction",
			slog.String("task_id", task.ID))
		return
	}

	// Send the correction
	if err := av.messenger.Send(ctx, task.To, correctionMsg); err != nil {
		av.logger.ErrorContext(ctx, "failed to send correction message",
			slog.Any("error", err),
			slog.String("task_id", task.ID),
			slog.String("to", task.To),
			slog.String("session_id", task.SessionID))
	} else {
		av.logger.InfoContext(ctx, "sent correction message",
			slog.String("task_id", task.ID),
			slog.String("to", task.To),
			slog.String("session_id", task.SessionID),
			slog.String("validation_status", string(validationResult.Status)))
	}
}

// shouldSendCorrection determines if a correction message should be sent.
func (av *AsyncValidator) shouldSendCorrection(result ValidationResult) bool {
	// Send corrections for failures, partial successes, or incomplete searches
	return result.Status == ValidationStatusFailed ||
		result.Status == ValidationStatusPartial ||
		result.Status == ValidationStatusIncompleteSearch
}

// Shutdown gracefully stops all active validations and waits for them to complete.
func (av *AsyncValidator) Shutdown(ctx context.Context) error {
	av.logger.InfoContext(ctx, "shutting down async validator")

	// Signal shutdown to all active validations
	close(av.shutdownCh)

	// Cancel all active validation contexts
	av.mu.Lock()
	for _, task := range av.active {
		task.Cancel()
	}
	activeCount := len(av.active)
	av.mu.Unlock()

	av.logger.InfoContext(ctx, "canceling active validations",
		slog.Int("active_count", activeCount))

	// Wait for all validations to complete or context to expire
	done := make(chan struct{})
	go func() {
		av.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		av.logger.InfoContext(ctx, "all validations completed")
		return nil
	case <-ctx.Done():
		av.logger.WarnContext(ctx, "shutdown timeout, some validations may be incomplete",
			slog.Int("remaining", av.ActiveCount()))
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}
}

// ActiveCount returns the number of currently running validations.
func (av *AsyncValidator) ActiveCount() int {
	av.mu.Lock()
	defer av.mu.Unlock()
	return len(av.active)
}

// GetActiveTasks returns a snapshot of currently active validation tasks.
// This is useful for monitoring and debugging.
func (av *AsyncValidator) GetActiveTasks() []ValidationTask {
	av.mu.Lock()
	defer av.mu.Unlock()

	tasks := make([]ValidationTask, 0, len(av.active))
	for _, task := range av.active {
		// Create a copy to avoid data races
		tasks = append(tasks, ValidationTask{
			ID:              task.ID,
			To:              task.To,
			OriginalRequest: task.OriginalRequest,
			SessionID:       task.SessionID,
			StartTime:       task.StartTime,
			// Don't include Response or Cancel to avoid potential issues
		})
	}
	return tasks
}

// SetCorrectionDelay allows overriding the correction delay (useful for testing).
func (av *AsyncValidator) SetCorrectionDelay(delay time.Duration) {
	av.mu.Lock()
	defer av.mu.Unlock()
	av.correctionDelay = delay
}
