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
	resultHandler      ValidationResultHandler
	logger             *slog.Logger

	// Track active validations for cleanup
	mu         sync.Mutex
	active     map[string]*ValidationTask
	shutdownCh chan struct{}
	wg         sync.WaitGroup

	// Configurable delay before sending corrections
	correctionDelay time.Duration

	// Configurable timeout for validation
	validationTimeout time.Duration
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
	// Create default result handler if not provided
	resultHandler := NewDefaultResultHandler(messenger, strategy, logger)

	return &AsyncValidator{
		messenger:          messenger,
		validationStrategy: strategy,
		resultHandler:      resultHandler,
		logger:             logger,
		active:             make(map[string]*ValidationTask),
		shutdownCh:         make(chan struct{}),
		correctionDelay:    DefaultCorrectionDelay,
		validationTimeout:  DefaultValidationTimeout,
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
	backgroundCtx, cancel := context.WithTimeout(context.Background(), av.validationTimeout)

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

	// Apply delay before validation to give user time to read initial response
	if av.correctionDelay > 0 {
		av.logger.DebugContext(ctx, "waiting before starting validation",
			slog.String("task_id", task.ID),
			slog.Duration("delay", av.correctionDelay))

		timer := time.NewTimer(av.correctionDelay)
		defer timer.Stop()

		select {
		case <-timer.C:
			// Timer expired, proceed with validation
			av.logger.DebugContext(ctx, "delay complete, starting validation",
				slog.String("task_id", task.ID))
		case <-ctx.Done():
			// Context canceled during delay
			av.logger.InfoContext(ctx, "validation canceled during delay",
				slog.String("task_id", task.ID),
				slog.Any("error", ctx.Err()))
			return
		case <-av.shutdownCh:
			// Shutdown requested during delay
			av.logger.InfoContext(ctx, "validation canceled due to shutdown during delay",
				slog.String("task_id", task.ID))
			return
		}
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

	// Handle validation result through the result handler
	if av.resultHandler.ShouldHandle(validationResult) {
		if err := av.resultHandler.HandleResult(ctx, validationResult, task); err != nil {
			av.logger.ErrorContext(ctx, "failed to handle validation result",
				slog.String("task_id", task.ID),
				slog.Any("error", err))
		}
	}

	av.logger.InfoContext(ctx, "async validation task complete",
		slog.String("task_id", task.ID),
		slog.String("session_id", task.SessionID),
		slog.Duration("duration", time.Since(task.StartTime)),
		slog.Bool("correction_sent", av.resultHandler.ShouldHandle(validationResult)))
}

// SetResultHandler allows overriding the default result handler.
// This is useful for custom handling logic or testing.
func (av *AsyncValidator) SetResultHandler(handler ValidationResultHandler) {
	av.mu.Lock()
	defer av.mu.Unlock()
	av.resultHandler = handler
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

	// Also update the result handler if it's a DefaultResultHandler
	if handler, ok := av.resultHandler.(*DefaultResultHandler); ok {
		handler.SetCorrectionDelay(delay)
		// Set clarify delay to match correction delay for consistency in tests
		handler.SetClarifyDelay(delay)
	}
}

// SetValidationTimeout allows overriding the validation timeout (useful for testing).
func (av *AsyncValidator) SetValidationTimeout(timeout time.Duration) {
	av.mu.Lock()
	defer av.mu.Unlock()
	av.validationTimeout = timeout
}
