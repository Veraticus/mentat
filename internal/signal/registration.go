// Package signal provides interfaces for Signal messenger integration.
package signal

import (
	"context"
	"fmt"
	"time"
)

// RegistrationState represents the current state in the registration flow.
type RegistrationState int

const (
	// StateUnregistered indicates no registration has been attempted.
	StateUnregistered RegistrationState = iota
	// StateRegistrationStarted indicates registration request has been sent.
	StateRegistrationStarted
	// StateCaptchaRequired indicates captcha verification is needed.
	StateCaptchaRequired
	// StateAwaitingSMSCode indicates SMS code has been sent and awaiting verification.
	StateAwaitingSMSCode
	// StateRegistered indicates successful registration completion.
	StateRegistered
)

// String returns the string representation of the registration state.
func (s RegistrationState) String() string {
	switch s {
	case StateUnregistered:
		return "Unregistered"
	case StateRegistrationStarted:
		return "RegistrationStarted"
	case StateCaptchaRequired:
		return "CaptchaRequired"
	case StateAwaitingSMSCode:
		return "AwaitingSMSCode"
	case StateRegistered:
		return "Registered"
	default:
		return "Unknown"
	}
}

// RegistrationManager orchestrates the Signal registration process using a state machine pattern.
// It manages the flow from phone number entry through verification to completion.
type RegistrationManager interface {
	// GetCurrentState returns the current registration state.
	GetCurrentState(ctx context.Context) (RegistrationState, error)

	// StartRegistration initiates the registration process for a phone number.
	// Returns the next state and any additional information needed.
	StartRegistration(ctx context.Context, phoneNumber string) (*RegistrationResponse, error)

	// SubmitCaptcha provides captcha verification when required.
	// This is called when the state is StateCaptchaRequired.
	SubmitCaptcha(ctx context.Context, captchaToken string) (*RegistrationResponse, error)

	// SubmitSMSCode provides the SMS verification code.
	// This is called when the state is StateAwaitingSMSCode.
	SubmitSMSCode(ctx context.Context, code string) (*RegistrationResponse, error)

	// GetRegistrationInfo returns information about the current registration state.
	// Useful for resuming interrupted registrations.
	GetRegistrationInfo(ctx context.Context) (*RegistrationInfo, error)

	// Reset clears any in-progress registration state.
	// Used when starting over with a different phone number.
	Reset(ctx context.Context) error
}

// VerificationHandler abstracts different verification methods used during registration.
// This allows for flexible verification strategies and easy testing.
type VerificationHandler interface {
	// RequestSMSCode triggers sending an SMS verification code.
	RequestSMSCode(ctx context.Context, phoneNumber string) error

	// VerifySMSCode validates the provided SMS code.
	VerifySMSCode(ctx context.Context, phoneNumber string, code string) error

	// GetCaptchaURL returns the URL for captcha verification.
	// Users must visit this URL and complete the captcha.
	GetCaptchaURL(ctx context.Context) (string, error)

	// VerifyCaptchaToken validates the captcha token obtained from the URL.
	VerifyCaptchaToken(ctx context.Context, token string) error

	// IsVerificationRequired checks if any verification is needed.
	IsVerificationRequired(ctx context.Context) (bool, error)

	// GetVerificationTimeout returns how long to wait for verification.
	GetVerificationTimeout() time.Duration
}

// RegistrationResponse contains the result of a registration operation.
type RegistrationResponse struct {
	// Success indicates if the operation succeeded.
	Success bool

	// NextState is the new registration state after this operation.
	NextState RegistrationState

	// Message provides human-readable information about the result.
	Message string

	// CaptchaURL is populated when captcha verification is required.
	CaptchaURL string

	// SMSCodeLength indicates expected length of SMS code (usually 6).
	SMSCodeLength int

	// RetryAfter suggests when to retry if rate limited.
	RetryAfter time.Duration

	// Error contains any error that occurred.
	Error error
}

// RegistrationInfo provides details about the current registration.
type RegistrationInfo struct {
	// State is the current registration state.
	State RegistrationState

	// PhoneNumber being registered (may be empty if not started).
	PhoneNumber string

	// StartedAt is when registration began.
	StartedAt time.Time

	// LastUpdatedAt is when the state last changed.
	LastUpdatedAt time.Time

	// AttemptsRemaining for the current verification step.
	AttemptsRemaining int

	// ExpiresAt is when the current state expires (e.g., SMS code validity).
	ExpiresAt time.Time
}

// RegistrationError represents specific registration failures.
type RegistrationError struct {
	// Code identifies the type of error.
	Code RegistrationErrorCode

	// Message provides human-readable error details.
	Message string

	// Retryable indicates if the operation can be retried.
	Retryable bool

	// RetryAfter suggests when to retry.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *RegistrationError) Error() string {
	return fmt.Sprintf("registration error %s: %s", e.Code, e.Message)
}

// RegistrationErrorCode identifies specific registration error types.
type RegistrationErrorCode string

const (
	// ErrInvalidPhoneNumber indicates the phone number format is invalid.
	ErrInvalidPhoneNumber RegistrationErrorCode = "INVALID_PHONE_NUMBER"

	// ErrPhoneNumberInUse indicates the number is already registered.
	ErrPhoneNumberInUse RegistrationErrorCode = "PHONE_NUMBER_IN_USE"

	// ErrCaptchaRequired indicates captcha verification is needed.
	ErrCaptchaRequired RegistrationErrorCode = "CAPTCHA_REQUIRED"

	// ErrInvalidCaptcha indicates the captcha token was invalid.
	ErrInvalidCaptcha RegistrationErrorCode = "INVALID_CAPTCHA"

	// ErrInvalidSMSCode indicates the SMS code was incorrect.
	ErrInvalidSMSCode RegistrationErrorCode = "INVALID_SMS_CODE"

	// ErrSMSCodeExpired indicates the SMS code has expired.
	ErrSMSCodeExpired RegistrationErrorCode = "SMS_CODE_EXPIRED"

	// ErrRateLimited indicates too many attempts have been made.
	ErrRateLimited RegistrationErrorCode = "RATE_LIMITED"

	// ErrNetworkError indicates a network connectivity issue.
	ErrNetworkError RegistrationErrorCode = "NETWORK_ERROR"

	// ErrInternalError indicates an unexpected error occurred.
	ErrInternalError RegistrationErrorCode = "INTERNAL_ERROR"
)

// IsValidTransition checks if a state transition is allowed.
// This enforces the state machine rules for registration flow.
func IsValidTransition(from, to RegistrationState) bool {
	switch from {
	case StateUnregistered:
		return to == StateRegistrationStarted || to == StateUnregistered

	case StateRegistrationStarted:
		return to == StateCaptchaRequired || to == StateAwaitingSMSCode || to == StateUnregistered

	case StateCaptchaRequired:
		return to == StateAwaitingSMSCode || to == StateUnregistered

	case StateAwaitingSMSCode:
		return to == StateRegistered || to == StateUnregistered

	case StateRegistered:
		return to == StateRegistered || to == StateUnregistered

	default:
		return false
	}
}
