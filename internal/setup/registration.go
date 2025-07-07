// Package setup provides the registration setup flow for Mentat.
package setup

import (
	"context"
	"fmt"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// StatePersister handles persistence of registration state for resumability.
type StatePersister interface {
	// SaveState persists the current registration state
	SaveState(ctx context.Context, state *RegistrationState) error

	// LoadState retrieves the saved registration state
	LoadState(ctx context.Context) (*RegistrationState, error)

	// ClearState removes any saved registration state
	ClearState(ctx context.Context) error
}

// RegistrationState represents the persistent state of registration.
type RegistrationState struct {
	// CurrentState is the current registration state
	CurrentState signal.RegistrationState

	// PhoneNumber being registered
	PhoneNumber string

	// StartedAt is when registration began
	StartedAt time.Time

	// LastUpdatedAt is when the state last changed
	LastUpdatedAt time.Time

	// CompletedSteps tracks which steps have been completed
	CompletedSteps []string
}

// CaptchaRequiredError indicates that captcha verification is needed.
type CaptchaRequiredError struct {
	URL string
}

// Error implements the error interface.
func (e *CaptchaRequiredError) Error() string {
	return fmt.Sprintf("captcha required: %s", e.URL)
}

// RegistrationCoordinator orchestrates the Signal registration flow with dependency injection.
type RegistrationCoordinator struct {
	signalManager       signal.Manager
	processManager      signal.ProcessManager
	verificationHandler signal.VerificationHandler
	statePersister      StatePersister
	registrationManager signal.RegistrationManager
}

// NewRegistrationCoordinator creates a new registration coordinator with injected dependencies.
func NewRegistrationCoordinator(
	signalManager signal.Manager,
	processManager signal.ProcessManager,
	verificationHandler signal.VerificationHandler,
	statePersister StatePersister,
	registrationManager signal.RegistrationManager,
) (*RegistrationCoordinator, error) {
	if signalManager == nil {
		return nil, fmt.Errorf("signal manager is required")
	}
	if processManager == nil {
		return nil, fmt.Errorf("process manager is required")
	}
	if verificationHandler == nil {
		return nil, fmt.Errorf("verification handler is required")
	}
	if statePersister == nil {
		return nil, fmt.Errorf("state persister is required")
	}
	if registrationManager == nil {
		return nil, fmt.Errorf("registration manager is required")
	}

	return &RegistrationCoordinator{
		signalManager:       signalManager,
		processManager:      processManager,
		verificationHandler: verificationHandler,
		statePersister:      statePersister,
		registrationManager: registrationManager,
	}, nil
}

// StartRegistration begins the registration process for a phone number.
func (rc *RegistrationCoordinator) StartRegistration(ctx context.Context, phoneNumber string) error {
	// Check if already registered
	registered, err := rc.signalManager.IsRegistered(ctx)
	if err != nil {
		return fmt.Errorf("checking registration status: %w", err)
	}
	if registered {
		return fmt.Errorf("already registered with Signal")
	}

	// Ensure process is running
	if !rc.processManager.IsRunning() {
		err = rc.processManager.Start(ctx)
		if err != nil {
			return fmt.Errorf("starting signal process: %w", err)
		}

		// Wait for process to be ready
		err = rc.processManager.WaitForReady(ctx)
		if err != nil {
			return fmt.Errorf("waiting for signal process: %w", err)
		}
	}

	// Start registration via registration manager
	resp, err := rc.registrationManager.StartRegistration(ctx, phoneNumber)
	if err != nil {
		return fmt.Errorf("starting registration: %w", err)
	}

	// Save initial state
	state := &RegistrationState{
		CurrentState:   resp.NextState,
		PhoneNumber:    phoneNumber,
		StartedAt:      time.Now(),
		LastUpdatedAt:  time.Now(),
		CompletedSteps: []string{"phone_number_entered"},
	}

	err = rc.statePersister.SaveState(ctx, state)
	if err != nil {
		return fmt.Errorf("saving registration state: %w", err)
	}

	// Handle captcha if required
	if resp.NextState == signal.StateCaptchaRequired {
		return rc.handleCaptchaRequired(ctx, resp.CaptchaURL)
	}

	return nil
}

// SubmitCaptcha provides the captcha token after user completes verification.
func (rc *RegistrationCoordinator) SubmitCaptcha(ctx context.Context, captchaToken string) error {
	// Load current state
	state, err := rc.statePersister.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("loading registration state: %w", err)
	}

	if state.CurrentState != signal.StateCaptchaRequired {
		return fmt.Errorf("not in captcha required state")
	}

	// Submit captcha via registration manager
	resp, err := rc.registrationManager.SubmitCaptcha(ctx, captchaToken)
	if err != nil {
		return fmt.Errorf("submitting captcha: %w", err)
	}

	// Update state
	state.CurrentState = resp.NextState
	state.LastUpdatedAt = time.Now()
	state.CompletedSteps = append(state.CompletedSteps, "captcha_completed")

	err = rc.statePersister.SaveState(ctx, state)
	if err != nil {
		return fmt.Errorf("saving registration state: %w", err)
	}

	return nil
}

// SubmitSMSCode provides the SMS verification code.
func (rc *RegistrationCoordinator) SubmitSMSCode(ctx context.Context, code string) error {
	// Load current state
	state, err := rc.statePersister.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("loading registration state: %w", err)
	}

	if state.CurrentState != signal.StateAwaitingSMSCode {
		return fmt.Errorf("not awaiting SMS code")
	}

	// Submit SMS code via registration manager
	resp, err := rc.registrationManager.SubmitSMSCode(ctx, code)
	if err != nil {
		return fmt.Errorf("submitting SMS code: %w", err)
	}

	// Update state
	state.CurrentState = resp.NextState
	state.LastUpdatedAt = time.Now()
	state.CompletedSteps = append(state.CompletedSteps, "sms_verified")

	err = rc.statePersister.SaveState(ctx, state)
	if err != nil {
		return fmt.Errorf("saving registration state: %w", err)
	}

	// Clear state if registration completed
	if resp.NextState == signal.StateRegistered {
		state.CompletedSteps = append(state.CompletedSteps, "registration_completed")
		err = rc.statePersister.SaveState(ctx, state)
		if err != nil {
			return fmt.Errorf("saving final state: %w", err)
		}
	}

	return nil
}

// GetCurrentState returns the current registration state.
func (rc *RegistrationCoordinator) GetCurrentState(ctx context.Context) (*RegistrationState, error) {
	state, err := rc.statePersister.LoadState(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading registration state: %w", err)
	}
	return state, nil
}

// ResumeRegistration continues an interrupted registration flow.
func (rc *RegistrationCoordinator) ResumeRegistration(ctx context.Context) error {
	state, err := rc.statePersister.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("loading registration state: %w", err)
	}

	if state == nil {
		return fmt.Errorf("no registration in progress")
	}

	// Ensure process is running
	if !rc.processManager.IsRunning() {
		err = rc.processManager.Start(ctx)
		if err != nil {
			return fmt.Errorf("starting signal process: %w", err)
		}

		err = rc.processManager.WaitForReady(ctx)
		if err != nil {
			return fmt.Errorf("waiting for signal process: %w", err)
		}
	}

	// Get current registration info
	info, err := rc.registrationManager.GetRegistrationInfo(ctx)
	if err != nil {
		return fmt.Errorf("getting registration info: %w", err)
	}

	// Validate state consistency
	if info.State != state.CurrentState {
		return fmt.Errorf("state mismatch: persisted=%s, actual=%s", state.CurrentState, info.State)
	}

	return nil
}

// Reset clears any in-progress registration.
func (rc *RegistrationCoordinator) Reset(ctx context.Context) error {
	// Clear persisted state
	if err := rc.statePersister.ClearState(ctx); err != nil {
		return fmt.Errorf("clearing persisted state: %w", err)
	}

	// Reset registration manager
	if err := rc.registrationManager.Reset(ctx); err != nil {
		return fmt.Errorf("resetting registration manager: %w", err)
	}

	return nil
}

// handleCaptchaRequired processes the captcha requirement.
func (rc *RegistrationCoordinator) handleCaptchaRequired(ctx context.Context, captchaURL string) error {
	if captchaURL == "" {
		// Get captcha URL from verification handler
		url, err := rc.verificationHandler.GetCaptchaURL(ctx)
		if err != nil {
			return fmt.Errorf("getting captcha URL: %w", err)
		}
		captchaURL = url
	}

	// Return with captcha URL for user to complete
	return &CaptchaRequiredError{URL: captchaURL}
}

// ValidatePhoneNumber checks if a phone number is valid for registration.
func (rc *RegistrationCoordinator) ValidatePhoneNumber(phoneNumber string) error {
	// This should use a proper phone validation from the signal package
	// For now, basic validation
	const minPhoneNumberLength = 10
	if len(phoneNumber) < minPhoneNumberLength {
		return fmt.Errorf("phone number too short")
	}
	if phoneNumber[0] != '+' {
		return fmt.Errorf("phone number must start with country code (e.g., +1)")
	}
	return nil
}
