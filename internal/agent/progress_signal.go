package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

const (
	// shortTimeout is used for LLM error message generation.
	shortTimeout = 5 * time.Second
)

// getErrorTypeName returns a human-readable name for an ErrorType.
func getErrorTypeName(errorType ErrorType) string {
	switch errorType {
	case ErrorTypeCancelled:
		return "Canceled"
	case ErrorTypeTimeout:
		return "Timeout"
	case ErrorTypeAuthentication:
		return "Authentication"
	case ErrorTypeRateLimit:
		return "Rate Limit"
	case ErrorTypeNetwork:
		return "Network"
	case ErrorTypeValidation:
		return "Validation"
	case ErrorTypeConfiguration:
		return "Configuration"
	case ErrorTypeMessenger:
		return "Messenger"
	case ErrorTypeLLM:
		return "LLM"
	case ErrorTypeUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// SignalProgressReporter implements ProgressReporter for Signal messenger
// It uses the LLM to generate natural language error messages and falls back
// to default messages when the LLM is unavailable.
type SignalProgressReporter struct {
	messenger signal.Messenger
	llm       claude.LLM
	senderID  string
	recovery  *ErrorRecovery
	mu        sync.RWMutex
}

// ProgressOption allows customization of SignalProgressReporter.
type ProgressOption func(*SignalProgressReporter)

// WithErrorRecovery sets a custom ErrorRecovery instance.
func WithErrorRecovery(recovery *ErrorRecovery) ProgressOption {
	return func(spr *SignalProgressReporter) {
		spr.recovery = recovery
	}
}

// NewSignalProgressReporter creates a new SignalProgressReporter.
func NewSignalProgressReporter(
	messenger signal.Messenger,
	llm claude.LLM,
	senderID string,
	opts ...ProgressOption,
) *SignalProgressReporter {
	spr := &SignalProgressReporter{
		messenger: messenger,
		llm:       llm,
		senderID:  senderID,
		recovery:  NewErrorRecovery(),
	}

	for _, opt := range opts {
		opt(spr)
	}

	return spr
}

// ReportError sends an error message to the user via Signal
// It attempts to use the LLM to generate a contextual error message,
// falling back to default messages if the LLM is unavailable.
func (spr *SignalProgressReporter) ReportError(ctx context.Context, err error, userFriendlyMessage string) error {
	spr.mu.RLock()
	defer spr.mu.RUnlock()

	if err == nil {
		return nil
	}

	// If we already have a user-friendly message, use it
	var message string
	if userFriendlyMessage != "" {
		message = userFriendlyMessage
	} else {
		// Try to generate a better message using LLM
		message = spr.generateErrorMessage(ctx, err)
	}

	// Send the message via Signal
	if sendErr := spr.messenger.Send(ctx, spr.senderID, message); sendErr != nil {
		return fmt.Errorf("failed to send error message: %w", sendErr)
	}
	return nil
}

// generateErrorMessage attempts to use the LLM to create a natural language error message.
func (spr *SignalProgressReporter) generateErrorMessage(ctx context.Context, err error) string {
	// First, get the default message from ErrorRecovery
	defaultMessage := spr.recovery.GenerateUserMessage(err)

	// If LLM is not available, return the default
	if spr.llm == nil {
		return defaultMessage
	}

	// Create a prompt for the LLM to generate a natural error message
	errorType := spr.recovery.ClassifyError(err)
	prompt := fmt.Sprintf(`You are helping a user understand an error that occurred. 
Generate a brief, friendly explanation for this error:

Error Type: %s
Technical Error: %s

Provide a single paragraph explanation that:
- Explains what went wrong in simple terms
- Suggests what the user might do (if applicable)
- Maintains a helpful, conversational tone

Do not include any technical jargon or error codes.`, getErrorTypeName(errorType), err.Error())

	// Try to get an LLM response with a short timeout
	llmCtx, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()

	response, llmErr := spr.llm.Query(llmCtx, prompt, "error-reporter")
	if llmErr != nil || response == nil || response.Message == "" {
		// Fall back to default message if LLM fails
		return defaultMessage
	}

	return response.Message
}

// ShouldContinue determines if processing should continue after an error.
func (spr *SignalProgressReporter) ShouldContinue(err error) bool {
	if err == nil {
		return true
	}

	spr.mu.RLock()
	defer spr.mu.RUnlock()

	errorType := spr.recovery.ClassifyError(err)

	// Don't continue for certain error types
	switch errorType {
	case ErrorTypeCancelled, ErrorTypeAuthentication, ErrorTypeConfiguration:
		return false
	case ErrorTypeNetwork, ErrorTypeRateLimit:
		// These might be temporary, allow continuation
		return true
	case ErrorTypeUnknown, ErrorTypeTimeout, ErrorTypeValidation, ErrorTypeMessenger, ErrorTypeLLM:
		// For other errors, default to not continuing
		return false
	}
	// This should be unreachable as all error types are handled
	return false
}
