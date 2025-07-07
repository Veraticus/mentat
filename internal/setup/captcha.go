// Package setup provides the registration setup flow for Mentat.
package setup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

const (
	// DefaultCaptchaURL is the Signal captcha verification URL.
	DefaultCaptchaURL = "https://signalcaptchas.org/registration/generate.html"

	// CaptchaTokenPrefix is the expected prefix for captcha tokens in redirect URLs.
	CaptchaTokenPrefix = "signalcaptcha://" // #nosec G101

	// CaptchaTokenMinLength is the minimum expected length for a valid token.
	CaptchaTokenMinLength = 20

	// DefaultCaptchaTimeout is how long to wait for captcha completion.
	DefaultCaptchaTimeout = 5 * time.Minute

	// MaxCaptchaInstructions is the maximum number of instruction steps to show.
	MaxCaptchaInstructions = 6
)

// CaptchaFlowHandler manages the captcha verification flow with enhanced user experience.
// It separates captcha concerns from other verification methods and provides clear guidance.
type CaptchaFlowHandler struct {
	prompter       Prompter
	captchaURL     string
	captchaTimeout time.Duration
}

// NewCaptchaFlowHandler creates a new captcha flow handler with default settings.
func NewCaptchaFlowHandler(prompter Prompter) *CaptchaFlowHandler {
	return &CaptchaFlowHandler{
		prompter:       prompter,
		captchaURL:     DefaultCaptchaURL,
		captchaTimeout: DefaultCaptchaTimeout,
	}
}

// NewCaptchaFlowHandlerWithOptions creates a new captcha flow handler with custom settings.
func NewCaptchaFlowHandlerWithOptions(prompter Prompter, captchaURL string, timeout time.Duration) *CaptchaFlowHandler {
	if captchaURL == "" {
		captchaURL = DefaultCaptchaURL
	}
	if timeout == 0 {
		timeout = DefaultCaptchaTimeout
	}
	return &CaptchaFlowHandler{
		prompter:       prompter,
		captchaURL:     captchaURL,
		captchaTimeout: timeout,
	}
}

// DetectCaptchaRequirement checks if an error indicates captcha verification is needed.
func (h *CaptchaFlowHandler) DetectCaptchaRequirement(err error) bool {
	if err == nil {
		return false
	}

	// Check for RegistrationError with captcha code
	var regErr *signal.RegistrationError
	if errors.As(err, &regErr) {
		return regErr.Code == signal.ErrCaptchaRequired
	}

	// Check for captcha in error message
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "captcha")
}

// GuideUserThroughCaptcha provides complete captcha verification flow with clear instructions.
func (h *CaptchaFlowHandler) GuideUserThroughCaptcha(ctx context.Context) (string, error) {
	// Show introduction
	h.prompter.ShowMessage("\n🔐 Captcha Verification Required")
	h.prompter.ShowMessage("Signal requires captcha verification to prevent automated registrations.")
	h.prompter.ShowMessage("")

	for {
		// Display step-by-step instructions
		instructions := h.FormatInstructions()
		for i, instruction := range instructions {
			h.prompter.ShowMessage(fmt.Sprintf("%d. %s", i+1, instruction))
		}
		h.prompter.ShowMessage("")

		// Show the URL prominently
		h.prompter.ShowMessage("📎 Captcha URL:")
		h.prompter.ShowMessage(fmt.Sprintf("   %s", h.captchaURL))
		h.prompter.ShowMessage("")

		// Add platform-specific guidance
		h.showPlatformGuidance()

		// Prompt for token with timeout
		token, err := h.prompter.PromptWithTimeout(
			ctx,
			"Paste the captcha token here (or 'retry' for new instructions)",
			h.captchaTimeout,
		)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return "", fmt.Errorf("captcha verification timed out after %v", h.captchaTimeout)
			}
			return "", fmt.Errorf("reading captcha token: %w", err)
		}

		// Handle retry request
		if strings.ToLower(strings.TrimSpace(token)) == "retry" {
			h.prompter.ShowMessage("\n🔄 Showing instructions again...")
			// Continue to next iteration
			continue
		}

		// Clean and validate token
		cleanToken, err := h.extractAndValidateToken(token)
		if err != nil {
			h.prompter.ShowError(fmt.Sprintf("Invalid token: %s", err))

			// Ask if they want to try again
			retry, _ := h.prompter.PromptWithDefault(ctx, "Would you like to try again?", "yes")
			if strings.ToLower(strings.TrimSpace(retry)) == "yes" {
				continue
			}
			return "", err
		}

		h.prompter.ShowSuccess("Token validated successfully!")
		return cleanToken, nil
	}
}

// FormatInstructions returns clear, step-by-step instructions for captcha completion.
func (h *CaptchaFlowHandler) FormatInstructions() []string {
	return []string{
		"Open the URL below in your web browser",
		"Complete the captcha challenge (select images, etc.)",
		"After success, you'll be redirected to a 'signalcaptcha://' URL",
		"Copy the ENTIRE redirect URL (it starts with 'signalcaptcha://')",
		"Paste the URL here when prompted",
		"The token will be automatically extracted",
	}
}

// ValidateCaptchaToken performs validation on a captcha token.
func (h *CaptchaFlowHandler) ValidateCaptchaToken(token string) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	if len(token) < CaptchaTokenMinLength {
		return fmt.Errorf("token too short (minimum %d characters)", CaptchaTokenMinLength)
	}

	// Check for common mistakes
	if strings.Contains(token, " ") {
		return fmt.Errorf("token contains spaces - please check you copied it correctly")
	}

	if strings.HasPrefix(token, "http") {
		return fmt.Errorf("this appears to be a web URL - please copy the 'signalcaptcha://' redirect URL instead")
	}

	return nil
}

// GetCaptchaURL returns the URL for captcha verification.
func (h *CaptchaFlowHandler) GetCaptchaURL(_ context.Context) (string, error) {
	// Validate URL format
	if _, err := url.Parse(h.captchaURL); err != nil {
		return "", fmt.Errorf("invalid captcha URL: %w", err)
	}
	return h.captchaURL, nil
}

// extractAndValidateToken cleans and validates a token from user input.
func (h *CaptchaFlowHandler) extractAndValidateToken(input string) (string, error) {
	input = strings.TrimSpace(input)

	// Extract token from full URL if provided
	var token string
	switch {
	case strings.HasPrefix(input, CaptchaTokenPrefix):
		// Full signalcaptcha:// URL provided
		token = strings.TrimPrefix(input, CaptchaTokenPrefix)
	case !strings.Contains(input, "://"):
		// Assume it's just the token
		token = input
	default:
		return "", fmt.Errorf("unexpected URL format - expected signalcaptcha:// URL or token only")
	}

	// Clean up common issues
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "/") // Remove trailing slashes

	// Validate the extracted token
	if err := h.ValidateCaptchaToken(token); err != nil {
		return "", err
	}

	return token, nil
}

// showPlatformGuidance provides platform-specific tips for copying URLs.
func (h *CaptchaFlowHandler) showPlatformGuidance() {
	h.prompter.ShowMessage("💡 Platform Tips:")
	h.prompter.ShowMessage("   • Desktop: Right-click the address bar → Copy")
	h.prompter.ShowMessage("   • Mobile: Long-press the URL → Select All → Copy")
	h.prompter.ShowMessage("   • The redirect happens quickly - be ready to copy!")
	h.prompter.ShowMessage("")
}

// ShowCaptchaError provides user-friendly error messages for common captcha issues.
func (h *CaptchaFlowHandler) ShowCaptchaError(err error) {
	if err == nil {
		return
	}

	var regErr *signal.RegistrationError
	if errors.As(err, &regErr) {
		switch regErr.Code {
		case signal.ErrInvalidCaptcha:
			h.prompter.ShowError("The captcha token was invalid or expired")
			h.prompter.ShowMessage("Please complete a new captcha challenge")
		case signal.ErrCaptchaRequired:
			h.prompter.ShowError("Captcha verification is required to continue")
		case signal.ErrInvalidPhoneNumber,
			signal.ErrPhoneNumberInUse,
			signal.ErrInvalidSMSCode,
			signal.ErrSMSCodeExpired,
			signal.ErrRateLimited,
			signal.ErrNetworkError,
			signal.ErrInternalError:
			// Non-captcha errors - show generic message
			h.prompter.ShowError(regErr.Message)
		}
		return
	}

	// Generic error
	h.prompter.ShowError(fmt.Sprintf("Captcha error: %s", err))
}
