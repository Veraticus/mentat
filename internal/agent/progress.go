// Package agent provides the multi-agent validation system for processing messages.
package agent

import "context"

// ProgressReporter provides a simplified interface for reporting errors and continuation status
// during agent processing. It focuses on natural language error messages without staged progress tracking.
type ProgressReporter interface {
	// ReportError sends a natural language error message to the user.
	// The error message should be user-friendly and explain what went wrong
	// in terms the user can understand, without exposing internal implementation details.
	// Returns an error if the report itself fails.
	ReportError(ctx context.Context, err error, userFriendlyMessage string) error

	// ShouldContinue determines whether processing should continue after an error.
	// This allows for graceful handling of non-critical errors where partial results
	// may still be valuable to the user.
	ShouldContinue(err error) bool
}
