package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// ErrorType represents the type of error for user-friendly message generation.
type ErrorType int

const (
	// ErrorTypeUnknown represents an unknown error type.
	ErrorTypeUnknown ErrorType = iota
	// ErrorTypeCancelled represents a canceled request.
	ErrorTypeCancelled
	// ErrorTypeTimeout represents a timeout error.
	ErrorTypeTimeout
	// ErrorTypeAuthentication represents an authentication error.
	ErrorTypeAuthentication
	// ErrorTypeRateLimit represents a rate limit error.
	ErrorTypeRateLimit
	// ErrorTypeNetwork represents a network error.
	ErrorTypeNetwork
	// ErrorTypeValidation represents a validation error.
	ErrorTypeValidation
	// ErrorTypeConfiguration represents a configuration error.
	ErrorTypeConfiguration
	// ErrorTypeMessenger represents a messenger/Signal error.
	ErrorTypeMessenger
	// ErrorTypeLLM represents an LLM/Claude error.
	ErrorTypeLLM
)

// ErrorRecovery provides user-friendly error messages and recovery strategies.
type ErrorRecovery struct {
	defaultMessages map[ErrorType]string
}

// NewErrorRecovery creates a new error recovery handler.
func NewErrorRecovery() *ErrorRecovery {
	return &ErrorRecovery{
		defaultMessages: map[ErrorType]string{
			ErrorTypeCancelled:      "The request was canceled. Please try again if you still need assistance.",
			ErrorTypeTimeout:        "The request took too long to process. Please try again with a simpler request.",
			ErrorTypeAuthentication: "I need to be authenticated to help you. Please ask your administrator to run the authentication command.",
			ErrorTypeRateLimit:      "I'm currently experiencing high demand. Please try again in a few moments.",
			ErrorTypeNetwork:        "I'm having trouble connecting to my services. Please try again in a moment.",
			ErrorTypeValidation:     "I couldn't validate my response properly. Let me try a different approach.",
			ErrorTypeConfiguration:  "There's a configuration issue preventing me from helping. Please contact your administrator.",
			ErrorTypeMessenger:      "I'm having trouble with the messaging service. Please try again later.",
			ErrorTypeLLM:            "I encountered an issue processing your request. Please try rephrasing your question.",
			ErrorTypeUnknown:        "I encountered an unexpected issue processing your request. Please try again or rephrase your question.",
		},
	}
}

// ClassifyError determines the type of error for appropriate user messaging.
func (r *ErrorRecovery) ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	// Check for context errors
	if errors.Is(err, context.Canceled) {
		return ErrorTypeCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorTypeTimeout
	}

	// Check for authentication errors
	if claude.IsAuthenticationError(err) {
		return ErrorTypeAuthentication
	}

	// Check for rate limit errors
	if isRateLimitError(err) {
		return ErrorTypeRateLimit
	}

	// Check error message for patterns
	errMsg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errMsg, "network"):
		return ErrorTypeNetwork
	case strings.Contains(errMsg, "validation"):
		return ErrorTypeValidation
	case strings.Contains(errMsg, "config"):
		return ErrorTypeConfiguration
	case strings.Contains(errMsg, "authentication") || strings.Contains(errMsg, "auth"):
		return ErrorTypeAuthentication
	case strings.Contains(errMsg, "messenger") || strings.Contains(errMsg, "signal"):
		return ErrorTypeMessenger
	case strings.Contains(errMsg, "llm") || strings.Contains(errMsg, "claude"):
		return ErrorTypeLLM
	default:
		return ErrorTypeUnknown
	}
}

// GenerateUserMessage creates a user-friendly error message based on the error type.
func (r *ErrorRecovery) GenerateUserMessage(err error) string {
	errorType := r.ClassifyError(err)
	return r.defaultMessages[errorType]
}

// GenerateContextualMessage creates a context-aware user-friendly error message.
func (r *ErrorRecovery) GenerateContextualMessage(err error, originalRequest string) string {
	errorType := r.ClassifyError(err)
	baseMessage := r.defaultMessages[errorType]

	// Add contextual information for certain error types
	const longRequestThreshold = 100
	switch errorType {
	case ErrorTypeTimeout:
		if len(originalRequest) > longRequestThreshold {
			return baseMessage + " Your request was quite detailed - consider breaking it into smaller parts."
		}
	case ErrorTypeValidation:
		return "I had trouble with your request about: \"" + truncateRequest(originalRequest) + "\". " +
			"Let me know if you'd like me to try a different approach."
	case ErrorTypeRateLimit:
		return baseMessage + " Your request has been queued and will be processed soon."
	case ErrorTypeUnknown, ErrorTypeCancelled, ErrorTypeAuthentication,
		ErrorTypeNetwork, ErrorTypeConfiguration, ErrorTypeMessenger, ErrorTypeLLM:
		// Return the base message for these error types
	}

	return baseMessage
}

// isRateLimitError checks if an error is a rate limit error.
func isRateLimitError(err error) bool {
	// Check for error message patterns
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "too many requests")
}

// truncateRequest truncates a request to a reasonable length for error messages.
func truncateRequest(request string) string {
	const maxLength = 50
	if len(request) <= maxLength {
		return request
	}
	return request[:maxLength-3] + "..."
}

// ProcessingErrorHandler wraps error handling for the Process method.
type ProcessingErrorHandler struct {
	recovery  *ErrorRecovery
	messenger signal.Messenger
}

// NewProcessingErrorHandler creates a new processing error handler.
func NewProcessingErrorHandler(messenger signal.Messenger) *ProcessingErrorHandler {
	return &ProcessingErrorHandler{
		recovery:  NewErrorRecovery(),
		messenger: messenger,
	}
}

// HandleError processes an error and sends a user-friendly message.
func (h *ProcessingErrorHandler) HandleError(ctx context.Context, err error, originalRequest, recipient string) error {
	if err == nil {
		return nil
	}

	// Generate user-friendly error message
	userMessage := h.recovery.GenerateContextualMessage(err, originalRequest)

	// Attempt to send the error message to the user
	if sendErr := h.messenger.Send(ctx, recipient, userMessage); sendErr != nil {
		// Log both errors but return the original error
		return fmt.Errorf("processing failed (%w) and couldn't send error message (%w)", err, sendErr)
	}

	// Return the original error for logging/monitoring
	return err
}

// WrapWithRecovery wraps a function to ensure error recovery messages are sent.
func (h *ProcessingErrorHandler) WrapWithRecovery(
	ctx context.Context,
	recipient string,
	originalRequest string,
	fn func() error,
) error {
	err := fn()
	if err != nil {
		return h.HandleError(ctx, err, originalRequest, recipient)
	}
	return nil
}
