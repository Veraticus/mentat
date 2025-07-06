package signal_test

import (
	"context"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// TestRegistrationStateString verifies the String() method for all states.
func TestRegistrationStateString(t *testing.T) {
	tests := []struct {
		name  string
		state signal.RegistrationState
		want  string
	}{
		{
			name:  "unregistered",
			state: signal.StateUnregistered,
			want:  "Unregistered",
		},
		{
			name:  "registration started",
			state: signal.StateRegistrationStarted,
			want:  "RegistrationStarted",
		},
		{
			name:  "captcha required",
			state: signal.StateCaptchaRequired,
			want:  "CaptchaRequired",
		},
		{
			name:  "awaiting sms code",
			state: signal.StateAwaitingSMSCode,
			want:  "AwaitingSMSCode",
		},
		{
			name:  "registered",
			state: signal.StateRegistered,
			want:  "Registered",
		},
		{
			name:  "unknown state",
			state: signal.RegistrationState(999),
			want:  "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("RegistrationState.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsValidTransition verifies state machine transition rules.
func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		name  string
		from  signal.RegistrationState
		to    signal.RegistrationState
		valid bool
	}{
		// From Unregistered
		{
			name:  "unregistered to registration started",
			from:  signal.StateUnregistered,
			to:    signal.StateRegistrationStarted,
			valid: true,
		},
		{
			name:  "unregistered to unregistered",
			from:  signal.StateUnregistered,
			to:    signal.StateUnregistered,
			valid: true,
		},
		{
			name:  "unregistered to captcha required - invalid",
			from:  signal.StateUnregistered,
			to:    signal.StateCaptchaRequired,
			valid: false,
		},
		// From RegistrationStarted
		{
			name:  "registration started to captcha required",
			from:  signal.StateRegistrationStarted,
			to:    signal.StateCaptchaRequired,
			valid: true,
		},
		{
			name:  "registration started to awaiting sms",
			from:  signal.StateRegistrationStarted,
			to:    signal.StateAwaitingSMSCode,
			valid: true,
		},
		{
			name:  "registration started to unregistered",
			from:  signal.StateRegistrationStarted,
			to:    signal.StateUnregistered,
			valid: true,
		},
		{
			name:  "registration started to registered - invalid",
			from:  signal.StateRegistrationStarted,
			to:    signal.StateRegistered,
			valid: false,
		},
		// From CaptchaRequired
		{
			name:  "captcha required to awaiting sms",
			from:  signal.StateCaptchaRequired,
			to:    signal.StateAwaitingSMSCode,
			valid: true,
		},
		{
			name:  "captcha required to unregistered",
			from:  signal.StateCaptchaRequired,
			to:    signal.StateUnregistered,
			valid: true,
		},
		{
			name:  "captcha required to registered - invalid",
			from:  signal.StateCaptchaRequired,
			to:    signal.StateRegistered,
			valid: false,
		},
		// From AwaitingSMSCode
		{
			name:  "awaiting sms to registered",
			from:  signal.StateAwaitingSMSCode,
			to:    signal.StateRegistered,
			valid: true,
		},
		{
			name:  "awaiting sms to unregistered",
			from:  signal.StateAwaitingSMSCode,
			to:    signal.StateUnregistered,
			valid: true,
		},
		{
			name:  "awaiting sms to captcha - invalid",
			from:  signal.StateAwaitingSMSCode,
			to:    signal.StateCaptchaRequired,
			valid: false,
		},
		// From Registered
		{
			name:  "registered to registered",
			from:  signal.StateRegistered,
			to:    signal.StateRegistered,
			valid: true,
		},
		{
			name:  "registered to unregistered",
			from:  signal.StateRegistered,
			to:    signal.StateUnregistered,
			valid: true,
		},
		{
			name:  "registered to awaiting sms - invalid",
			from:  signal.StateRegistered,
			to:    signal.StateAwaitingSMSCode,
			valid: false,
		},
		// Invalid state
		{
			name:  "invalid state transition",
			from:  signal.RegistrationState(999),
			to:    signal.StateRegistered,
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signal.IsValidTransition(tt.from, tt.to)
			if got != tt.valid {
				t.Errorf("IsValidTransition(%v, %v) = %v, want %v", tt.from, tt.to, got, tt.valid)
			}
		})
	}
}

// TestRegistrationError verifies error formatting.
func TestRegistrationError(t *testing.T) {
	tests := []struct {
		name    string
		err     *signal.RegistrationError
		wantMsg string
	}{
		{
			name: "invalid phone number error",
			err: &signal.RegistrationError{
				Code:    signal.ErrInvalidPhoneNumber,
				Message: "Phone number must be in E.164 format",
			},
			wantMsg: "registration error INVALID_PHONE_NUMBER: Phone number must be in E.164 format",
		},
		{
			name: "rate limited error with retry",
			err: &signal.RegistrationError{
				Code:       signal.ErrRateLimited,
				Message:    "Too many attempts",
				Retryable:  true,
				RetryAfter: 5 * time.Minute,
			},
			wantMsg: "registration error RATE_LIMITED: Too many attempts",
		},
		{
			name: "captcha required error",
			err: &signal.RegistrationError{
				Code:      signal.ErrCaptchaRequired,
				Message:   "Please complete captcha verification",
				Retryable: true,
			},
			wantMsg: "registration error CAPTCHA_REQUIRED: Please complete captcha verification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.wantMsg {
				t.Errorf("RegistrationError.Error() = %v, want %v", got, tt.wantMsg)
			}
		})
	}
}

// TestRegistrationManagerInterface verifies the interface is mockable.
func TestRegistrationManagerInterface(_ *testing.T) {
	// This test ensures the interface can be implemented
	var _ signal.RegistrationManager = (*mockRegistrationManager)(nil)
}

// TestVerificationHandlerInterface verifies the interface is mockable.
func TestVerificationHandlerInterface(_ *testing.T) {
	// This test ensures the interface can be implemented
	var _ signal.VerificationHandler = (*mockVerificationHandler)(nil)
}

// mockRegistrationManager is a mock implementation for testing.
type mockRegistrationManager struct {
	currentState signal.RegistrationState
	phoneNumber  string
	captchaURL   string
	smsCode      string
	err          error
}

func (m *mockRegistrationManager) GetCurrentState(_ context.Context) (signal.RegistrationState, error) {
	if m.err != nil {
		return signal.StateUnregistered, m.err
	}
	return m.currentState, nil
}

func (m *mockRegistrationManager) StartRegistration(
	_ context.Context,
	phoneNumber string,
) (*signal.RegistrationResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.phoneNumber = phoneNumber
	m.currentState = signal.StateRegistrationStarted

	return &signal.RegistrationResponse{
		Success:       true,
		NextState:     signal.StateAwaitingSMSCode,
		Message:       "Registration started, SMS code sent",
		SMSCodeLength: 6,
	}, nil
}

func (m *mockRegistrationManager) SubmitCaptcha(_ context.Context, _ string) (*signal.RegistrationResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.currentState != signal.StateCaptchaRequired {
		return nil, &signal.RegistrationError{
			Code:    signal.ErrInternalError,
			Message: "Not in captcha required state",
		}
	}

	m.currentState = signal.StateAwaitingSMSCode
	return &signal.RegistrationResponse{
		Success:       true,
		NextState:     signal.StateAwaitingSMSCode,
		Message:       "Captcha verified, SMS code sent",
		SMSCodeLength: 6,
	}, nil
}

func (m *mockRegistrationManager) SubmitSMSCode(_ context.Context, code string) (*signal.RegistrationResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.currentState != signal.StateAwaitingSMSCode {
		return nil, &signal.RegistrationError{
			Code:    signal.ErrInternalError,
			Message: "Not awaiting SMS code",
		}
	}

	m.smsCode = code
	m.currentState = signal.StateRegistered
	return &signal.RegistrationResponse{
		Success:   true,
		NextState: signal.StateRegistered,
		Message:   "Registration completed successfully",
	}, nil
}

func (m *mockRegistrationManager) GetRegistrationInfo(_ context.Context) (*signal.RegistrationInfo, error) {
	if m.err != nil {
		return nil, m.err
	}

	return &signal.RegistrationInfo{
		State:             m.currentState,
		PhoneNumber:       m.phoneNumber,
		StartedAt:         time.Now().Add(-5 * time.Minute),
		LastUpdatedAt:     time.Now(),
		AttemptsRemaining: 3,
		ExpiresAt:         time.Now().Add(10 * time.Minute),
	}, nil
}

func (m *mockRegistrationManager) Reset(_ context.Context) error {
	m.currentState = signal.StateUnregistered
	m.phoneNumber = ""
	m.captchaURL = ""
	m.smsCode = ""
	return m.err
}

// mockVerificationHandler is a mock implementation for testing.
type mockVerificationHandler struct {
	phoneNumber     string
	smsCode         string
	captchaToken    string
	captchaURL      string
	verificationReq bool
	err             error
}

func (m *mockVerificationHandler) RequestSMSCode(_ context.Context, phoneNumber string) error {
	if m.err != nil {
		return m.err
	}
	m.phoneNumber = phoneNumber
	return nil
}

func (m *mockVerificationHandler) VerifySMSCode(_ context.Context, phoneNumber string, code string) error {
	if m.err != nil {
		return m.err
	}
	if m.phoneNumber != phoneNumber {
		return &signal.RegistrationError{
			Code:    signal.ErrInvalidPhoneNumber,
			Message: "Phone number mismatch",
		}
	}
	m.smsCode = code
	return nil
}

func (m *mockVerificationHandler) GetCaptchaURL(_ context.Context) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.captchaURL, nil
}

func (m *mockVerificationHandler) VerifyCaptchaToken(_ context.Context, token string) error {
	if m.err != nil {
		return m.err
	}
	m.captchaToken = token
	return nil
}

func (m *mockVerificationHandler) IsVerificationRequired(_ context.Context) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.verificationReq, nil
}

func (m *mockVerificationHandler) GetVerificationTimeout() time.Duration {
	return 5 * time.Minute
}

// TestMockImplementations verifies mock behaviors.
func TestMockImplementations(t *testing.T) {
	ctx := context.Background()

	t.Run("registration manager mock", func(t *testing.T) {
		mock := &mockRegistrationManager{
			currentState: signal.StateUnregistered,
		}

		// Test start registration
		resp, err := mock.StartRegistration(ctx, "+1234567890")
		if err != nil {
			t.Fatalf("StartRegistration failed: %v", err)
		}
		if resp.NextState != signal.StateAwaitingSMSCode {
			t.Errorf("Expected next state %v, got %v", signal.StateAwaitingSMSCode, resp.NextState)
		}
		if mock.phoneNumber != "+1234567890" {
			t.Errorf("Expected phone number %v, got %v", "+1234567890", mock.phoneNumber)
		}

		// Test submit SMS code
		mock.currentState = signal.StateAwaitingSMSCode
		resp, err = mock.SubmitSMSCode(ctx, "123456")
		if err != nil {
			t.Fatalf("SubmitSMSCode failed: %v", err)
		}
		if resp.NextState != signal.StateRegistered {
			t.Errorf("Expected next state %v, got %v", signal.StateRegistered, resp.NextState)
		}
		if mock.smsCode != "123456" {
			t.Errorf("Expected SMS code %v, got %v", "123456", mock.smsCode)
		}
	})

	t.Run("verification handler mock", func(t *testing.T) {
		mock := &mockVerificationHandler{
			captchaURL: "https://signalcaptchas.org/challenge/test",
		}

		// Test request SMS code
		err := mock.RequestSMSCode(ctx, "+1234567890")
		if err != nil {
			t.Fatalf("RequestSMSCode failed: %v", err)
		}
		if mock.phoneNumber != "+1234567890" {
			t.Errorf("Expected phone number %v, got %v", "+1234567890", mock.phoneNumber)
		}

		// Test verify SMS code
		err = mock.VerifySMSCode(ctx, "+1234567890", "123456")
		if err != nil {
			t.Fatalf("VerifySMSCode failed: %v", err)
		}
		if mock.smsCode != "123456" {
			t.Errorf("Expected SMS code %v, got %v", "123456", mock.smsCode)
		}

		// Test get captcha URL
		url, err := mock.GetCaptchaURL(ctx)
		if err != nil {
			t.Fatalf("GetCaptchaURL failed: %v", err)
		}
		if url != mock.captchaURL {
			t.Errorf("Expected captcha URL %v, got %v", mock.captchaURL, url)
		}

		// Test timeout
		timeout := mock.GetVerificationTimeout()
		if timeout != 5*time.Minute {
			t.Errorf("Expected timeout %v, got %v", 5*time.Minute, timeout)
		}
	})
}
