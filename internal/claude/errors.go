package claude

import "errors"

// AuthenticationError represents an authentication failure with Claude Code.
type AuthenticationError struct {
	Message string
}

// Error implements the error interface.
func (e *AuthenticationError) Error() string {
	return e.Message
}

// IsAuthenticationError checks if an error is an authentication error.
func IsAuthenticationError(err error) bool {
	var authErr *AuthenticationError
	return errors.As(err, &authErr)
}
