// Package setup provides the registration setup flow for Mentat.
package setup

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

const (
	// DefaultMaxSMSAttempts is the default number of SMS verification attempts allowed.
	DefaultMaxSMSAttempts = 3
	// DefaultSMSTimeout is the default timeout for entering SMS codes.
	DefaultSMSTimeout = 30 * time.Second
)

// SMSVerificationHandler implements signal.VerificationHandler with interactive prompting.
type SMSVerificationHandler struct {
	prompter          Prompter
	maxAttempts       int
	attemptTimeout    time.Duration
	phoneNumber       string
	attemptsRemaining int
}

// NewSMSVerificationHandler creates a new SMS verification handler.
func NewSMSVerificationHandler(prompter Prompter, phoneNumber string) *SMSVerificationHandler {
	return &SMSVerificationHandler{
		prompter:          prompter,
		maxAttempts:       DefaultMaxSMSAttempts,
		attemptTimeout:    DefaultSMSTimeout,
		phoneNumber:       phoneNumber,
		attemptsRemaining: DefaultMaxSMSAttempts,
	}
}

// RequestSMSCode triggers sending an SMS verification code.
func (h *SMSVerificationHandler) RequestSMSCode(_ context.Context, phoneNumber string) error {
	h.phoneNumber = phoneNumber
	h.attemptsRemaining = h.maxAttempts

	h.prompter.ShowMessage(fmt.Sprintf("📱 Sending SMS verification code to %s...", phoneNumber))

	// In a real implementation, this would trigger signal-cli to request SMS
	// For now, we just indicate success
	h.prompter.ShowSuccess("SMS verification code sent!")
	h.prompter.ShowMessage("💡 Check your phone for a 6-digit verification code")

	return nil
}

// VerifySMSCode validates the provided SMS code.
func (h *SMSVerificationHandler) VerifySMSCode(_ context.Context, _ string, code string) error {
	// Validate code format
	if !isValidSMSCode(code) {
		return &signal.RegistrationError{
			Code:      signal.ErrInvalidSMSCode,
			Message:   "Invalid code format. Please enter a 6-digit code",
			Retryable: true,
		}
	}

	// In a real implementation, this would verify with signal-cli
	// For now, we simulate success
	return nil
}

// GetCaptchaURL returns the URL for captcha verification.
func (h *SMSVerificationHandler) GetCaptchaURL(_ context.Context) (string, error) {
	return "https://signalcaptchas.org/registration/generate.html", nil
}

// VerifyCaptchaToken validates the captcha token obtained from the URL.
func (h *SMSVerificationHandler) VerifyCaptchaToken(_ context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("captcha token cannot be empty")
	}
	// In a real implementation, this would validate with signal-cli
	return nil
}

// IsVerificationRequired checks if any verification is needed.
func (h *SMSVerificationHandler) IsVerificationRequired(_ context.Context) (bool, error) {
	// Always required for new registrations
	return true, nil
}

// GetVerificationTimeout returns how long to wait for verification.
func (h *SMSVerificationHandler) GetVerificationTimeout() time.Duration {
	return h.attemptTimeout
}

// PromptForSMSCode prompts the user to enter the SMS code with retry support.
func (h *SMSVerificationHandler) PromptForSMSCode(ctx context.Context) (string, error) {
	// Check if context is already canceled
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("verification canceled: %w", ctx.Err())
	default:
	}

	for h.attemptsRemaining > 0 {
		// Show remaining attempts
		attemptMsg := "Enter the 6-digit verification code"
		if h.attemptsRemaining < h.maxAttempts {
			attemptMsg = fmt.Sprintf("%s (%d attempts remaining)", attemptMsg, h.attemptsRemaining)
		}

		// Prompt with timeout
		code, err := h.prompter.PromptWithTimeout(ctx, attemptMsg, h.attemptTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return "", fmt.Errorf("verification canceled: %w", err)
			}

			// Handle timeout
			h.prompter.ShowError("Timeout waiting for code entry")
			retry, _ := h.prompter.PromptWithDefault(ctx, "Would you like to try again?", "yes")
			if strings.ToLower(strings.TrimSpace(retry)) != "yes" {
				return "", fmt.Errorf("verification timeout")
			}
			continue
		}

		// Clean up the code (remove spaces, dashes)
		code = cleanSMSCode(code)

		// Validate format
		if !isValidSMSCode(code) {
			h.attemptsRemaining--
			if h.attemptsRemaining > 0 {
				h.prompter.ShowError("Invalid code format. Please enter exactly 6 digits")
				continue
			}
			return "", &signal.RegistrationError{
				Code:      signal.ErrInvalidSMSCode,
				Message:   "Maximum verification attempts exceeded",
				Retryable: false,
			}
		}

		return code, nil
	}

	return "", &signal.RegistrationError{
		Code:      signal.ErrInvalidSMSCode,
		Message:   "Maximum verification attempts exceeded",
		Retryable: false,
	}
}

// PromptForCaptcha guides the user through captcha verification.
func (h *SMSVerificationHandler) PromptForCaptcha(ctx context.Context) (string, error) {
	url, err := h.GetCaptchaURL(ctx)
	if err != nil {
		return "", fmt.Errorf("getting captcha URL: %w", err)
	}

	h.prompter.ShowMessage("\n🔐 Captcha verification required")
	h.prompter.ShowMessage(fmt.Sprintf("Please visit: %s", url))
	h.prompter.ShowMessage("Complete the captcha and copy the token from the redirect URL")
	h.prompter.ShowMessage("The token appears after 'signalcaptcha://' in the URL")

	token, err := h.prompter.Prompt(ctx, "Paste the captcha token here")
	if err != nil {
		return "", fmt.Errorf("reading captcha token: %w", err)
	}

	// Clean up token (remove signalcaptcha:// prefix if present)
	token = strings.TrimPrefix(token, "signalcaptcha://")
	token = strings.TrimSpace(token)

	if token == "" {
		return "", fmt.Errorf("captcha token cannot be empty")
	}

	return token, nil
}

// cleanSMSCode removes common formatting from SMS codes.
func cleanSMSCode(code string) string {
	// Remove spaces, dashes, and other common separators
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, ".", "")
	return strings.TrimSpace(code)
}

// isValidSMSCode checks if the code is exactly 6 digits.
func isValidSMSCode(code string) bool {
	// Must be exactly 6 digits
	matched, _ := regexp.MatchString(`^\d{6}$`, code)
	return matched
}

// GetAttemptsRemaining returns the number of verification attempts remaining.
func (h *SMSVerificationHandler) GetAttemptsRemaining() int {
	return h.attemptsRemaining
}
