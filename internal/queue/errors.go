package queue

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Common queue errors.
var (
	// ErrRateLimited indicates the request was rate limited by the LLM provider.
	ErrRateLimited = errors.New("rate limited by LLM provider")
	
	// ErrQueueStopped indicates the queue has been stopped.
	ErrQueueStopped = errors.New("queue stopped")
	
	// ErrMessageNotFound indicates the message was not found.
	ErrMessageNotFound = errors.New("message not found")
)

// RateLimitError represents a rate limit error from the LLM provider.
type RateLimitError struct {
	RetryAfter time.Duration // How long to wait before retrying
	Message    string        // Error message from provider
	Err        error         // Underlying error
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited: retry after %v: %s", e.RetryAfter, e.Message)
	}
	return fmt.Sprintf("rate limited: %s", e.Message)
}

// Unwrap returns the underlying error.
func (e *RateLimitError) Unwrap() error {
	return e.Err
}

// IsRateLimitError checks if an error indicates rate limiting.
// It checks for common rate limit indicators in error messages.
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	
	// Check if it's already a RateLimitError
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		return true
	}
	
	// Check if it's our sentinel error
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	
	// Check error message for common rate limit indicators
	errMsg := strings.ToLower(err.Error())
	rateLimitIndicators := []string{
		"rate limit",
		"rate-limit",
		"ratelimit",
		"too many requests",
		"429",
		"quota exceeded",
		"throttled",
		"slow down",
	}
	
	for _, indicator := range rateLimitIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}
	
	return false
}

// NewRateLimitError creates a new RateLimitError.
func NewRateLimitError(message string, retryAfter time.Duration, err error) *RateLimitError {
	return &RateLimitError{
		Message:    message,
		RetryAfter: retryAfter,
		Err:        err,
	}
}

// ExtractRetryAfter attempts to extract retry-after duration from an error.
// Returns 0 if no specific duration can be determined.
func ExtractRetryAfter(err error) time.Duration {
	// Check if it's a RateLimitError with explicit RetryAfter
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) && rateLimitErr.RetryAfter > 0 {
		return rateLimitErr.RetryAfter
	}
	
	// Default to 0, letting the caller decide on backoff strategy
	return 0
}