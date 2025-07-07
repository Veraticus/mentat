package setup

import (
	"context"
	"errors"
	"fmt"

	"github.com/Veraticus/mentat/internal/signal"
)

// InteractiveRegistrationFlow orchestrates the complete registration flow with user interaction.
type InteractiveRegistrationFlow struct {
	coordinator *RegistrationCoordinator
	prompter    Prompter
}

// NewInteractiveRegistrationFlow creates a new interactive registration flow.
func NewInteractiveRegistrationFlow(
	coordinator *RegistrationCoordinator,
	prompter Prompter,
) (*InteractiveRegistrationFlow, error) {
	if coordinator == nil {
		return nil, fmt.Errorf("registration coordinator is required")
	}
	if prompter == nil {
		return nil, fmt.Errorf("prompter is required")
	}

	return &InteractiveRegistrationFlow{
		coordinator: coordinator,
		prompter:    prompter,
	}, nil
}

// RunRegistration executes the complete interactive registration flow.
func (f *InteractiveRegistrationFlow) RunRegistration(ctx context.Context, phoneNumber string) error {
	// Validate phone number first
	if err := f.coordinator.ValidatePhoneNumber(phoneNumber); err != nil {
		return fmt.Errorf("invalid phone number: %w", err)
	}

	// Start registration
	err := f.coordinator.StartRegistration(ctx, phoneNumber)
	if err != nil {
		var captchaErr *CaptchaRequiredError
		if errors.As(err, &captchaErr) {
			// Handle captcha interactively
			return f.handleCaptchaFlow(ctx, captchaErr.URL)
		}
		return fmt.Errorf("starting registration: %w", err)
	}

	// Check if SMS verification is needed
	state, err := f.coordinator.statePersister.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if state.CurrentState == signal.StateAwaitingSMSCode {
		return f.handleSMSVerification(ctx)
	}

	return nil
}

// handleCaptchaFlow manages the interactive captcha verification process.
func (f *InteractiveRegistrationFlow) handleCaptchaFlow(ctx context.Context, captchaURL string) error {
	// Create captcha flow handler
	captchaHandler := NewCaptchaFlowHandlerWithOptions(f.prompter, captchaURL, DefaultCaptchaTimeout)

	// Guide user through captcha
	token, err := captchaHandler.GuideUserThroughCaptcha(ctx)
	if err != nil {
		return fmt.Errorf("captcha verification: %w", err)
	}

	// Submit captcha token
	err = f.coordinator.SubmitCaptcha(ctx, token)
	if err != nil {
		captchaHandler.ShowCaptchaError(err)
		return fmt.Errorf("submitting captcha: %w", err)
	}

	// Check if SMS verification is needed after captcha
	state, err := f.coordinator.statePersister.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("loading state after captcha: %w", err)
	}

	if state.CurrentState == signal.StateAwaitingSMSCode {
		return f.handleSMSVerification(ctx)
	}

	return nil
}

// handleSMSVerification manages the SMS code verification process.
func (f *InteractiveRegistrationFlow) handleSMSVerification(ctx context.Context) error {
	// Get phone number from state
	state, err := f.coordinator.statePersister.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("loading state for SMS verification: %w", err)
	}

	// Create SMS verification handler
	smsHandler := NewSMSVerificationHandler(f.prompter, state.PhoneNumber)

	// Prompt for SMS code
	code, err := smsHandler.PromptForSMSCode(ctx)
	if err != nil {
		return fmt.Errorf("getting SMS code: %w", err)
	}

	// Submit SMS code
	err = f.coordinator.SubmitSMSCode(ctx, code)
	if err != nil {
		return fmt.Errorf("submitting SMS code: %w", err)
	}

	f.prompter.ShowSuccess("Registration completed successfully!")
	return nil
}

// GetRegistrationState returns the current registration state.
func (f *InteractiveRegistrationFlow) GetRegistrationState(ctx context.Context) (*RegistrationState, error) {
	state, err := f.coordinator.statePersister.LoadState(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading registration state: %w", err)
	}
	return state, nil
}

// Reset clears any in-progress registration.
func (f *InteractiveRegistrationFlow) Reset(ctx context.Context) error {
	return f.coordinator.Reset(ctx)
}
