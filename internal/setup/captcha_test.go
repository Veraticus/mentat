package setup_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/setup"
	"github.com/Veraticus/mentat/internal/signal"
)

// captchaTestPrompter implements setup.Prompter for testing captcha flows.
// This is a duplicate of testPrompter to avoid conflicts between test files.
type captchaTestPrompter struct {
	responses        map[string]string
	timeoutResponses map[string]string
	timeoutErrors    map[string]error
	messages         []string
	errors           []string
	successes        []string
	capturedInputs   []string
	// For custom behavior in tests
	promptWithTimeoutFunc func(ctx context.Context, message string, timeout time.Duration) (string, error)
}

func newCaptchaTestPrompter() *captchaTestPrompter {
	return &captchaTestPrompter{
		responses:        make(map[string]string),
		timeoutResponses: make(map[string]string),
		timeoutErrors:    make(map[string]error),
	}
}

func (p *captchaTestPrompter) Prompt(_ context.Context, message string) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)
	if resp, ok := p.responses[message]; ok {
		return resp, nil
	}
	return "", nil
}

func (p *captchaTestPrompter) PromptWithDefault(_ context.Context, message, defaultValue string) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)
	if resp, ok := p.responses[message]; ok && resp != "" {
		return resp, nil
	}
	return defaultValue, nil
}

func (p *captchaTestPrompter) PromptWithTimeout(
	ctx context.Context,
	message string,
	timeout time.Duration,
) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)

	// Use custom function if provided
	if p.promptWithTimeoutFunc != nil {
		return p.promptWithTimeoutFunc(ctx, message, timeout)
	}

	// Default behavior
	if err, ok := p.timeoutErrors[message]; ok {
		return "", err
	}
	if resp, ok := p.timeoutResponses[message]; ok {
		return resp, nil
	}
	return "000000", nil
}

func (p *captchaTestPrompter) PromptSecret(_ context.Context, message string) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)
	if resp, ok := p.responses[message]; ok {
		return resp, nil
	}
	return "", nil
}

func (p *captchaTestPrompter) ShowMessage(message string) {
	p.messages = append(p.messages, message)
}

func (p *captchaTestPrompter) ShowError(message string) {
	p.errors = append(p.errors, message)
}

func (p *captchaTestPrompter) ShowSuccess(message string) {
	p.successes = append(p.successes, message)
}

func TestNewCaptchaFlowHandler(t *testing.T) {
	mock := newCaptchaTestPrompter()
	handler := setup.NewCaptchaFlowHandler(mock)

	if handler == nil {
		t.Fatal("NewCaptchaFlowHandler returned nil")
	}

	// Test with custom options
	customURL := "https://custom.captcha.url"
	customTimeout := 10 * time.Minute
	handlerWithOpts := setup.NewCaptchaFlowHandlerWithOptions(mock, customURL, customTimeout)
	if handlerWithOpts == nil {
		t.Fatal("NewCaptchaFlowHandlerWithOptions returned nil")
	}
}

func TestCaptchaFlowHandler_DetectCaptchaRequirement(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "registration error with captcha code",
			err: &signal.RegistrationError{
				Code:    signal.ErrCaptchaRequired,
				Message: "Captcha required",
			},
			expected: true,
		},
		{
			name: "registration error without captcha code",
			err: &signal.RegistrationError{
				Code:    signal.ErrInvalidPhoneNumber,
				Message: "Invalid phone number",
			},
			expected: false,
		},
		{
			name:     "generic error with captcha in message",
			err:      fmt.Errorf("captcha verification needed"),
			expected: true,
		},
		{
			name:     "generic error with CAPTCHA in uppercase",
			err:      fmt.Errorf("CAPTCHA verification needed"),
			expected: true,
		},
		{
			name:     "generic error without captcha",
			err:      fmt.Errorf("network error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newCaptchaTestPrompter()
			handler := setup.NewCaptchaFlowHandler(mock)

			result := handler.DetectCaptchaRequirement(tt.err)
			if result != tt.expected {
				t.Errorf("DetectCaptchaRequirement(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestCaptchaFlowHandler_GuideUserThroughCaptcha(t *testing.T) {
	t.Run("successful captcha completion", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		mock.timeoutResponses["Paste the captcha token here (or 'retry' for new instructions)"] = "signalcaptcha://valid-token-12345678901234567890"

		handler := setup.NewCaptchaFlowHandler(mock)
		token, err := handler.GuideUserThroughCaptcha(context.Background())

		if err != nil {
			t.Errorf("GuideUserThroughCaptcha() unexpected error: %v", err)
		}
		if token != "valid-token-12345678901234567890" {
			t.Errorf("GuideUserThroughCaptcha() = %v, want %v", token, "valid-token-12345678901234567890")
		}

		// Check that instructions were shown
		if len(mock.messages) < 3 {
			t.Errorf("expected at least 3 messages, got %d", len(mock.messages))
		}
		if len(mock.successes) != 1 {
			t.Errorf("expected 1 success message, got %d", len(mock.successes))
		}
	})

	t.Run("token without prefix", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		mock.timeoutResponses["Paste the captcha token here (or 'retry' for new instructions)"] = "valid-token-12345678901234567890"

		handler := setup.NewCaptchaFlowHandler(mock)
		token, err := handler.GuideUserThroughCaptcha(context.Background())

		if err != nil {
			t.Errorf("GuideUserThroughCaptcha() unexpected error: %v", err)
		}
		if token != "valid-token-12345678901234567890" {
			t.Errorf("GuideUserThroughCaptcha() = %v, want %v", token, "valid-token-12345678901234567890")
		}
	})

	t.Run("retry flow", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		// First attempt: user types retry
		mock.timeoutResponses["Paste the captcha token here (or 'retry' for new instructions)"] = "retry"
		// Set up a counter to handle multiple calls
		callCount := 0
		mock.promptWithTimeoutFunc = func(_ context.Context, _ string, _ time.Duration) (string, error) {
			callCount++
			if callCount == 1 {
				return "retry", nil
			}
			return "valid-token-12345678901234567890", nil
		}

		handler := setup.NewCaptchaFlowHandler(mock)
		token, err := handler.GuideUserThroughCaptcha(context.Background())

		if err != nil {
			t.Errorf("GuideUserThroughCaptcha() unexpected error: %v", err)
		}
		if token != "valid-token-12345678901234567890" {
			t.Errorf("GuideUserThroughCaptcha() = %v, want %v", token, "valid-token-12345678901234567890")
		}
		if callCount != 2 {
			t.Errorf("expected 2 prompt calls for retry, got %d", callCount)
		}
	})

	t.Run("invalid token with retry", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		mock.responses["Would you like to try again?"] = "yes"

		// Track call count for different responses
		callCount := 0
		mock.promptWithTimeoutFunc = func(_ context.Context, _ string, _ time.Duration) (string, error) {
			callCount++
			if callCount == 1 {
				return "short", nil // Too short
			}
			return "valid-token-12345678901234567890", nil
		}

		handler := setup.NewCaptchaFlowHandler(mock)
		token, err := handler.GuideUserThroughCaptcha(context.Background())

		if err != nil {
			t.Errorf("GuideUserThroughCaptcha() unexpected error: %v", err)
		}
		if token != "valid-token-12345678901234567890" {
			t.Errorf("GuideUserThroughCaptcha() = %v, want %v", token, "valid-token-12345678901234567890")
		}
		if len(mock.errors) == 0 {
			t.Error("expected error message for invalid token")
		}
	})

	t.Run("invalid token without retry", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		mock.responses["Would you like to try again?"] = "no"
		mock.timeoutResponses["Paste the captcha token here (or 'retry' for new instructions)"] = "short"

		handler := setup.NewCaptchaFlowHandler(mock)
		_, err := handler.GuideUserThroughCaptcha(context.Background())

		if err == nil {
			t.Error("GuideUserThroughCaptcha() expected error for invalid token")
		}
		if !strings.Contains(err.Error(), "too short") {
			t.Errorf("expected error about token being too short, got: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		mock.timeoutErrors["Paste the captcha token here (or 'retry' for new instructions)"] = context.DeadlineExceeded

		handler := setup.NewCaptchaFlowHandler(mock)
		_, err := handler.GuideUserThroughCaptcha(context.Background())

		if err == nil {
			t.Error("GuideUserThroughCaptcha() expected error for timeout")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("expected timeout error, got: %v", err)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mock.timeoutErrors["Paste the captcha token here (or 'retry' for new instructions)"] = ctx.Err()

		handler := setup.NewCaptchaFlowHandler(mock)
		_, err := handler.GuideUserThroughCaptcha(ctx)

		if err == nil {
			t.Error("GuideUserThroughCaptcha() expected error for canceled context")
		}
	})
}

func TestCaptchaFlowHandler_FormatInstructions(t *testing.T) {
	mock := newCaptchaTestPrompter()
	handler := setup.NewCaptchaFlowHandler(mock)

	instructions := handler.FormatInstructions()
	if len(instructions) != 6 {
		t.Errorf("FormatInstructions() returned %d instructions, want 6", len(instructions))
	}

	// Check that instructions mention key elements
	allInstructions := strings.Join(instructions, " ")
	expectedElements := []string{
		"Open",
		"URL",
		"browser",
		"captcha",
		"redirect",
		"signalcaptcha://",
		"Copy",
		"Paste",
		"extracted",
	}

	for _, element := range expectedElements {
		if !strings.Contains(allInstructions, element) {
			t.Errorf("Instructions missing expected element: %s", element)
		}
	}
}

func TestCaptchaFlowHandler_ValidateCaptchaToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid token",
			token:   "valid-token-12345678901234567890",
			wantErr: false,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
			errMsg:  "empty",
		},
		{
			name:    "token too short",
			token:   "short",
			wantErr: true,
			errMsg:  "too short",
		},
		{
			name:    "token with spaces",
			token:   "token with spaces in it 1234567890",
			wantErr: true,
			errMsg:  "spaces",
		},
		{
			name:    "http URL instead of token",
			token:   "https://example.com/callback",
			wantErr: true,
			errMsg:  "web URL",
		},
		{
			name:    "minimum length token",
			token:   strings.Repeat("a", 20),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newCaptchaTestPrompter()
			handler := setup.NewCaptchaFlowHandler(mock)

			err := handler.ValidateCaptchaToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCaptchaToken() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateCaptchaToken() error = %v, want error containing %v", err, tt.errMsg)
			}
		})
	}
}

func TestCaptchaFlowHandler_GetCaptchaURL(t *testing.T) {
	t.Run("default URL", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		handler := setup.NewCaptchaFlowHandler(mock)

		url, err := handler.GetCaptchaURL(context.Background())
		if err != nil {
			t.Errorf("GetCaptchaURL() unexpected error: %v", err)
		}
		if url != "https://signalcaptchas.org/registration/generate.html" {
			t.Errorf("GetCaptchaURL() = %v, want default URL", url)
		}
	})

	t.Run("custom URL", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		customURL := "https://custom.captcha.url/verify"
		handler := setup.NewCaptchaFlowHandlerWithOptions(mock, customURL, 0)

		url, err := handler.GetCaptchaURL(context.Background())
		if err != nil {
			t.Errorf("GetCaptchaURL() unexpected error: %v", err)
		}
		if url != customURL {
			t.Errorf("GetCaptchaURL() = %v, want %v", url, customURL)
		}
	})
}

func TestCaptchaFlowHandler_ShowCaptchaError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedError string
		expectedMsg   string
	}{
		{
			name: "nil error",
			err:  nil,
		},
		{
			name: "invalid captcha error",
			err: &signal.RegistrationError{
				Code:    signal.ErrInvalidCaptcha,
				Message: "Token invalid",
			},
			expectedError: "invalid or expired",
			expectedMsg:   "new captcha",
		},
		{
			name: "captcha required error",
			err: &signal.RegistrationError{
				Code:    signal.ErrCaptchaRequired,
				Message: "Need captcha",
			},
			expectedError: "required to continue",
		},
		{
			name: "other registration error",
			err: &signal.RegistrationError{
				Code:    signal.ErrRateLimited,
				Message: "Too many attempts",
			},
			expectedError: "Too many attempts",
		},
		{
			name:          "generic error",
			err:           fmt.Errorf("something went wrong"),
			expectedError: "Captcha error: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newCaptchaTestPrompter()
			handler := setup.NewCaptchaFlowHandler(mock)

			handler.ShowCaptchaError(tt.err)

			if tt.err == nil {
				if len(mock.errors) != 0 {
					t.Error("ShowCaptchaError() showed error for nil input")
				}
				return
			}

			if len(mock.errors) == 0 {
				t.Error("ShowCaptchaError() didn't show any error")
				return
			}

			if tt.expectedError != "" && !strings.Contains(mock.errors[0], tt.expectedError) {
				t.Errorf("ShowCaptchaError() error = %v, want error containing %v", mock.errors[0], tt.expectedError)
			}

			if tt.expectedMsg != "" {
				if len(mock.messages) == 0 {
					t.Error("ShowCaptchaError() didn't show expected message")
				} else if !strings.Contains(mock.messages[0], tt.expectedMsg) {
					t.Errorf("ShowCaptchaError() message = %v, want message containing %v", mock.messages[0], tt.expectedMsg)
				}
			}
		})
	}
}

func TestCaptchaFlowHandler_ExtractToken(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "full signalcaptcha URL",
			input:     "signalcaptcha://valid-token-12345678901234567890",
			wantToken: "valid-token-12345678901234567890",
			wantErr:   false,
		},
		{
			name:      "token only",
			input:     "valid-token-12345678901234567890",
			wantToken: "valid-token-12345678901234567890",
			wantErr:   false,
		},
		{
			name:      "token with extra spaces",
			input:     "  valid-token-12345678901234567890  ",
			wantToken: "valid-token-12345678901234567890",
			wantErr:   false,
		},
		{
			name:      "URL with trailing slashes",
			input:     "signalcaptcha://valid-token-12345678901234567890///",
			wantToken: "valid-token-12345678901234567890",
			wantErr:   false,
		},
		{
			name:    "http URL",
			input:   "https://example.com/callback",
			wantErr: true,
		},
		{
			name:    "other protocol URL",
			input:   "ftp://example.com/file",
			wantErr: true,
		},
		{
			name:    "token too short after extraction",
			input:   "signalcaptcha://short",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newCaptchaTestPrompter()
			handler := setup.NewCaptchaFlowHandler(mock)

			// Use the prompt flow to test token extraction
			mock.timeoutResponses["Paste the captcha token here (or 'retry' for new instructions)"] = tt.input

			// For error cases, don't retry - prevent infinite loops
			if tt.wantErr {
				mock.responses["Would you like to try again?"] = "no"
			}

			token, err := handler.GuideUserThroughCaptcha(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("token extraction error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && token != tt.wantToken {
				t.Errorf("extracted token = %v, want %v", token, tt.wantToken)
			}
		})
	}
}

// TestCaptchaFlowHandler_Integration tests the full captcha flow integration.
func TestCaptchaFlowHandler_Integration(t *testing.T) {
	t.Run("complete flow with platform guidance", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		mock.timeoutResponses["Paste the captcha token here (or 'retry' for new instructions)"] = "signalcaptcha://integration-test-token-12345678901234567890"

		handler := setup.NewCaptchaFlowHandler(mock)
		token, err := handler.GuideUserThroughCaptcha(context.Background())

		if err != nil {
			t.Errorf("Complete flow failed: %v", err)
		}
		if token != "integration-test-token-12345678901234567890" {
			t.Errorf("Unexpected token: %v", token)
		}

		// Verify all components were shown
		allMessages := strings.Join(mock.messages, " ")
		expectedComponents := []string{
			"Captcha Verification Required",
			"Platform Tips",
			"Desktop",
			"Mobile",
			"signalcaptchas.org",
		}

		for _, component := range expectedComponents {
			if !strings.Contains(allMessages, component) {
				t.Errorf("Missing expected component in flow: %s", component)
			}
		}
	})
}

// TestCaptchaFlowHandler_Concurrency tests thread safety.
func TestCaptchaFlowHandler_Concurrency(t *testing.T) {
	mock := newCaptchaTestPrompter()
	handler := setup.NewCaptchaFlowHandler(mock)

	// Run multiple goroutines accessing the handler
	done := make(chan bool, 3)

	go func() {
		_ = handler.DetectCaptchaRequirement(fmt.Errorf("captcha"))
		done <- true
	}()

	go func() {
		_ = handler.ValidateCaptchaToken("test-token-12345678901234567890")
		done <- true
	}()

	go func() {
		_, _ = handler.GetCaptchaURL(context.Background())
		done <- true
	}()

	// Wait for all goroutines
	for range 3 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Goroutine timeout - possible race condition")
		}
	}
}

// TestCaptchaFlowHandler_EdgeCases tests edge cases.
func TestCaptchaFlowHandler_EdgeCases(t *testing.T) {
	t.Run("empty captcha URL", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		handler := setup.NewCaptchaFlowHandlerWithOptions(mock, "", 0)

		url, err := handler.GetCaptchaURL(context.Background())
		if err != nil {
			t.Errorf("GetCaptchaURL() error for empty URL: %v", err)
		}
		// Should use default
		if url != "https://signalcaptchas.org/registration/generate.html" {
			t.Errorf("Expected default URL, got: %v", url)
		}
	})

	t.Run("zero timeout uses default", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		handler := setup.NewCaptchaFlowHandlerWithOptions(mock, "", 0)

		// Can't directly test timeout value, but ensure handler is created
		if handler == nil {
			t.Error("Handler creation failed with zero timeout")
		}
	})

	t.Run("very long token", func(t *testing.T) {
		mock := newCaptchaTestPrompter()
		handler := setup.NewCaptchaFlowHandler(mock)

		longToken := strings.Repeat("a", 1000)
		err := handler.ValidateCaptchaToken(longToken)
		if err != nil {
			t.Errorf("ValidateCaptchaToken() failed for long token: %v", err)
		}
	})
}

// Benchmark tests.
func BenchmarkCaptchaFlowHandler_ValidateCaptchaToken(b *testing.B) {
	mock := newCaptchaTestPrompter()
	handler := setup.NewCaptchaFlowHandler(mock)
	token := "valid-token-12345678901234567890"

	b.ResetTimer()
	for range b.N {
		_ = handler.ValidateCaptchaToken(token)
	}
}

func BenchmarkCaptchaFlowHandler_DetectCaptchaRequirement(b *testing.B) {
	mock := newCaptchaTestPrompter()
	handler := setup.NewCaptchaFlowHandler(mock)
	err := &signal.RegistrationError{
		Code:    signal.ErrCaptchaRequired,
		Message: "Captcha required",
	}

	b.ResetTimer()
	for range b.N {
		_ = handler.DetectCaptchaRequirement(err)
	}
}

// Example test to demonstrate usage.
func ExampleCaptchaFlowHandler_FormatInstructions() {
	mock := newCaptchaTestPrompter()
	handler := setup.NewCaptchaFlowHandler(mock)

	instructions := handler.FormatInstructions()
	for i, instruction := range instructions {
		fmt.Printf("%d. %s\n", i+1, instruction)
	}
	// Output:
	// 1. Open the URL below in your web browser
	// 2. Complete the captcha challenge (select images, etc.)
	// 3. After success, you'll be redirected to a 'signalcaptcha://' URL
	// 4. Copy the ENTIRE redirect URL (it starts with 'signalcaptcha://')
	// 5. Paste the URL here when prompted
	// 6. The token will be automatically extracted
}
