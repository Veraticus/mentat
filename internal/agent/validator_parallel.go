// Package agent provides the multi-agent validation system for processing messages.
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
)

// AsyncValidationStrategy extends ValidationStrategy with async-aware capabilities.
// This interface is for internal use and allows strategies to optimize for async execution.
type AsyncValidationStrategy interface {
	ValidationStrategy

	// ValidateAsync performs validation with full async awareness.
	// This method can leverage the async context for optimizations.
	ValidateAsync(ctx context.Context, request, response, sessionID string, llm claude.LLM) ValidationResult
}

// ParallelValidationWrapper provides a base for wrapping synchronous validators for async execution.
// It ensures proper context handling and timeout management.
type ParallelValidationWrapper struct {
	baseValidator ValidationStrategy
	timeout       time.Duration
	mu            sync.RWMutex
}

// NewParallelValidationWrapper creates a new wrapper with the specified timeout.
func NewParallelValidationWrapper(validator ValidationStrategy, timeout time.Duration) *ParallelValidationWrapper {
	if timeout <= 0 {
		timeout = validationTimeout // Use default from constants
	}
	return &ParallelValidationWrapper{
		baseValidator: validator,
		timeout:       timeout,
	}
}

// Validate implements ValidationStrategy with context-aware execution.
func (w *ParallelValidationWrapper) Validate(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) ValidationResult {
	// Create a timeout context for the validation
	validationCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	// Channel to receive the result
	resultCh := make(chan ValidationResult, 1)
	errCh := make(chan error, 1)

	// Run validation in a goroutine to respect context cancellation
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("validation panic: %v", r)
			}
		}()

		result := w.baseValidator.Validate(validationCtx, request, response, sessionID, llm)
		select {
		case resultCh <- result:
		case <-validationCtx.Done():
			// Context canceled, don't send result
		}
	}()

	// Wait for result or timeout/cancellation
	select {
	case result := <-resultCh:
		return result
	case err := <-errCh:
		return ValidationResult{
			Status:     ValidationStatusFailed,
			Issues:     []string{fmt.Sprintf("validation error: %v", err)},
			Confidence: 0.0,
			Metadata:   map[string]string{"error": err.Error(), "async": "true"},
		}
	case <-validationCtx.Done():
		// Determine the reason for context cancellation
		var issue string
		if validationCtx.Err() == context.DeadlineExceeded {
			issue = fmt.Sprintf("validation timeout after %v", w.timeout)
		} else {
			issue = "validation canceled"
		}
		return ValidationResult{
			Status:     ValidationStatusUnclear,
			Issues:     []string{issue},
			Confidence: 0.0,
			Metadata:   map[string]string{"timeout": "true", "async": "true"},
		}
	}
}

// ShouldRetry delegates to the base validator.
func (w *ParallelValidationWrapper) ShouldRetry(result ValidationResult) bool {
	return w.baseValidator.ShouldRetry(result)
}

// GenerateRecovery delegates to the base validator with context awareness.
func (w *ParallelValidationWrapper) GenerateRecovery(
	ctx context.Context,
	request, response, sessionID string,
	result ValidationResult,
	llm claude.LLM,
) string {
	// Use a shorter timeout for recovery generation
	recoveryCtx, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()

	// Channel for recovery message
	recoveryCh := make(chan string, 1)

	go func() {
		recovery := w.baseValidator.GenerateRecovery(
			recoveryCtx, request, response, sessionID, result, llm,
		)
		select {
		case recoveryCh <- recovery:
		case <-recoveryCtx.Done():
			// Context canceled
		}
	}()

	select {
	case recovery := <-recoveryCh:
		return recovery
	case <-recoveryCtx.Done():
		// Return a default message on timeout
		return GenerateCorrectionMessage(result)
	}
}

// ParallelMultiAgentValidator wraps MultiAgentValidator for async execution.
type ParallelMultiAgentValidator struct {
	*ParallelValidationWrapper

	multiAgent *MultiAgentValidator
}

// NewParallelMultiAgentValidator creates a new async multi-agent validator.
func NewParallelMultiAgentValidator() *ParallelMultiAgentValidator {
	multiAgent := NewMultiAgentValidator()
	return &ParallelMultiAgentValidator{
		ParallelValidationWrapper: NewParallelValidationWrapper(multiAgent, validationTimeout),
		multiAgent:                multiAgent,
	}
}

// ValidateAsync implements AsyncValidationStrategy for optimized async validation.
func (v *ParallelMultiAgentValidator) ValidateAsync(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) ValidationResult {
	// For multi-agent validation, we can potentially parallelize tool detection
	// and the actual validation query in the future
	return v.Validate(ctx, request, response, sessionID, llm)
}

// ParallelSimpleValidator wraps SimpleValidator for async execution.
type ParallelSimpleValidator struct {
	*ParallelValidationWrapper

	simple *SimpleValidator
}

// NewParallelSimpleValidator creates a new async simple validator.
func NewParallelSimpleValidator() *ParallelSimpleValidator {
	simple := NewSimpleValidator()
	return &ParallelSimpleValidator{
		ParallelValidationWrapper: NewParallelValidationWrapper(simple, validationTimeout),
		simple:                    simple,
	}
}

// ValidateAsync implements AsyncValidationStrategy.
// Simple validation is fast, so no special async optimizations needed.
func (v *ParallelSimpleValidator) ValidateAsync(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) ValidationResult {
	// Simple validation is synchronous and fast
	return v.simple.Validate(ctx, request, response, sessionID, llm)
}

// ParallelNoopValidator wraps NoopValidator for async execution.
// Since NoopValidator always returns success immediately, no async wrapper needed.
type ParallelNoopValidator struct {
	*NoopValidator
}

// NewParallelNoopValidator creates a new async noop validator.
func NewParallelNoopValidator() *ParallelNoopValidator {
	return &ParallelNoopValidator{
		NoopValidator: NewNoopValidator(),
	}
}

// ValidateAsync implements AsyncValidationStrategy.
func (v *ParallelNoopValidator) ValidateAsync(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) ValidationResult {
	// Noop validation is instant
	return v.Validate(ctx, request, response, sessionID, llm)
}

// Validate implements ValidationStrategy (inherited from NoopValidator).
func (v *ParallelNoopValidator) Validate(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) ValidationResult {
	// Check context before returning success
	select {
	case <-ctx.Done():
		return ValidationResult{
			Status:     ValidationStatusUnclear,
			Issues:     []string{"validation canceled"},
			Confidence: 0.0,
			Metadata:   map[string]string{"canceled": "true", "async": "true"},
		}
	default:
		return v.NoopValidator.Validate(ctx, request, response, sessionID, llm)
	}
}

// CreateParallelValidator creates an async-aware validator from any ValidationStrategy.
// This is a factory function that returns the appropriate parallel wrapper.
func CreateParallelValidator(strategy ValidationStrategy) ValidationStrategy {
	// Check if already async-aware
	if async, ok := strategy.(AsyncValidationStrategy); ok {
		return async
	}

	// Wrap based on concrete type for optimizations
	switch v := strategy.(type) {
	case *MultiAgentValidator:
		return &ParallelMultiAgentValidator{
			ParallelValidationWrapper: NewParallelValidationWrapper(v, validationTimeout),
			multiAgent:                v,
		}
	case *SimpleValidator:
		return &ParallelSimpleValidator{
			ParallelValidationWrapper: NewParallelValidationWrapper(v, validationTimeout),
			simple:                    v,
		}
	case *NoopValidator:
		return NewParallelNoopValidator()
	default:
		// Generic wrapper for unknown strategies
		return NewParallelValidationWrapper(strategy, validationTimeout)
	}
}

// SetTimeout updates the timeout for a parallel validator if supported.
func SetTimeout(validator ValidationStrategy, timeout time.Duration) error {
	switch v := validator.(type) {
	case *ParallelValidationWrapper:
		v.mu.Lock()
		v.timeout = timeout
		v.mu.Unlock()
		return nil
	case *ParallelMultiAgentValidator:
		v.mu.Lock()
		v.timeout = timeout
		v.mu.Unlock()
		return nil
	case *ParallelSimpleValidator:
		v.mu.Lock()
		v.timeout = timeout
		v.mu.Unlock()
		return nil
	default:
		return fmt.Errorf("validator does not support timeout configuration")
	}
}
