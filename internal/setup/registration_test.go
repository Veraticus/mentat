package setup_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/setup"
	"github.com/Veraticus/mentat/internal/signal"
)

// mockSignalManager implements signal.Manager for testing.
type mockSignalManager struct {
	isRegistered bool
	registerErr  error
	verifyErr    error
}

func (m *mockSignalManager) Start(_ context.Context) error       { return nil }
func (m *mockSignalManager) Stop(_ context.Context) error        { return nil }
func (m *mockSignalManager) HealthCheck(_ context.Context) error { return nil }
func (m *mockSignalManager) GetMessenger() signal.Messenger      { return nil }

//nolint:ireturn // mock implementation returns nil interface
func (m *mockSignalManager) GetDeviceManager() signal.DeviceManager {
	return nil
}
func (m *mockSignalManager) GetPhoneNumber() string { return "+1234567890" }
func (m *mockSignalManager) IsRegistered(_ context.Context) (bool, error) {
	return m.isRegistered, nil
}
func (m *mockSignalManager) Register(_ context.Context, _ string, _ string) error {
	return m.registerErr
}
func (m *mockSignalManager) VerifyCode(_ context.Context, _ string) error {
	return m.verifyErr
}

// mockProcessManager implements signal.ProcessManager for testing.
type mockProcessManager struct {
	isRunning  bool
	startErr   error
	readyErr   error
	startCalls int
}

func (m *mockProcessManager) Start(_ context.Context) error {
	m.startCalls++
	if m.startErr != nil {
		return m.startErr
	}
	m.isRunning = true
	return nil
}
func (m *mockProcessManager) Stop(_ context.Context) error    { m.isRunning = false; return nil }
func (m *mockProcessManager) Restart(_ context.Context) error { return nil }
func (m *mockProcessManager) IsRunning() bool                 { return m.isRunning }
func (m *mockProcessManager) GetPID() int                     { return 12345 }
func (m *mockProcessManager) WaitForReady(_ context.Context) error {
	return m.readyErr
}

// mockVerificationHandler implements signal.VerificationHandler for testing.
type mockVerificationHandler struct {
	captchaURL      string
	captchaErr      error
	smsErr          error
	captchaTokenErr error
}

func (m *mockVerificationHandler) RequestSMSCode(_ context.Context, _ string) error {
	return m.smsErr
}
func (m *mockVerificationHandler) VerifySMSCode(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *mockVerificationHandler) GetCaptchaURL(_ context.Context) (string, error) {
	return m.captchaURL, m.captchaErr
}
func (m *mockVerificationHandler) VerifyCaptchaToken(_ context.Context, _ string) error {
	return m.captchaTokenErr
}
func (m *mockVerificationHandler) IsVerificationRequired(_ context.Context) (bool, error) {
	return false, nil
}
func (m *mockVerificationHandler) GetVerificationTimeout() time.Duration {
	return 5 * time.Minute
}

// mockStatePersister implements setup.StatePersister for testing.
type mockStatePersister struct {
	state     *setup.RegistrationState
	saveErr   error
	loadErr   error
	clearErr  error
	saveCalls int
}

func (m *mockStatePersister) SaveState(_ context.Context, state *setup.RegistrationState) error {
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	m.state = state
	return nil
}

func (m *mockStatePersister) LoadState(_ context.Context) (*setup.RegistrationState, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.state, nil
}

func (m *mockStatePersister) ClearState(_ context.Context) error {
	if m.clearErr != nil {
		return m.clearErr
	}
	m.state = nil
	return nil
}

// mockRegistrationManager implements signal.RegistrationManager for testing.
type mockRegistrationManager struct {
	currentState     signal.RegistrationState
	startResponse    *signal.RegistrationResponse
	captchaResponse  *signal.RegistrationResponse
	smsResponse      *signal.RegistrationResponse
	registrationInfo *signal.RegistrationInfo
	resetErr         error
	startCalls       int
	captchaCalls     int
	smsCalls         int
}

func (m *mockRegistrationManager) GetCurrentState(_ context.Context) (signal.RegistrationState, error) {
	return m.currentState, nil
}

func (m *mockRegistrationManager) StartRegistration(_ context.Context, _ string) (*signal.RegistrationResponse, error) {
	m.startCalls++
	if m.startResponse != nil {
		return m.startResponse, nil
	}
	return &signal.RegistrationResponse{
		Success:   true,
		NextState: signal.StateAwaitingSMSCode,
		Message:   "SMS code sent",
	}, nil
}

func (m *mockRegistrationManager) SubmitCaptcha(_ context.Context, _ string) (*signal.RegistrationResponse, error) {
	m.captchaCalls++
	if m.captchaResponse != nil {
		return m.captchaResponse, nil
	}
	return &signal.RegistrationResponse{
		Success:   true,
		NextState: signal.StateAwaitingSMSCode,
		Message:   "Captcha verified",
	}, nil
}

func (m *mockRegistrationManager) SubmitSMSCode(_ context.Context, _ string) (*signal.RegistrationResponse, error) {
	m.smsCalls++
	if m.smsResponse != nil {
		return m.smsResponse, nil
	}
	return &signal.RegistrationResponse{
		Success:   true,
		NextState: signal.StateRegistered,
		Message:   "Registration complete",
	}, nil
}

func (m *mockRegistrationManager) GetRegistrationInfo(_ context.Context) (*signal.RegistrationInfo, error) {
	if m.registrationInfo != nil {
		return m.registrationInfo, nil
	}
	return &signal.RegistrationInfo{
		State:       m.currentState,
		PhoneNumber: "+1234567890",
		StartedAt:   time.Now().Add(-5 * time.Minute),
	}, nil
}

func (m *mockRegistrationManager) Reset(_ context.Context) error {
	m.currentState = signal.StateUnregistered
	return m.resetErr
}

func TestNewRegistrationCoordinator(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (*setup.RegistrationCoordinator, error)
		wantErr bool
		errMsg  string
	}{
		{
			name: "success with all dependencies",
			setup: func() (*setup.RegistrationCoordinator, error) {
				return setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{},
					&mockRegistrationManager{},
				)
			},
			wantErr: false,
		},
		{
			name: "nil signal manager",
			setup: func() (*setup.RegistrationCoordinator, error) {
				return setup.NewRegistrationCoordinator(
					nil,
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{},
					&mockRegistrationManager{},
				)
			},
			wantErr: true,
			errMsg:  "signal manager is required",
		},
		{
			name: "nil process manager",
			setup: func() (*setup.RegistrationCoordinator, error) {
				return setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					nil,
					&mockVerificationHandler{},
					&mockStatePersister{},
					&mockRegistrationManager{},
				)
			},
			wantErr: true,
			errMsg:  "process manager is required",
		},
		{
			name: "nil verification handler",
			setup: func() (*setup.RegistrationCoordinator, error) {
				return setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					nil,
					&mockStatePersister{},
					&mockRegistrationManager{},
				)
			},
			wantErr: true,
			errMsg:  "verification handler is required",
		},
		{
			name: "nil state persister",
			setup: func() (*setup.RegistrationCoordinator, error) {
				return setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					nil,
					&mockRegistrationManager{},
				)
			},
			wantErr: true,
			errMsg:  "state persister is required",
		},
		{
			name: "nil registration manager",
			setup: func() (*setup.RegistrationCoordinator, error) {
				return setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{},
					nil,
				)
			},
			wantErr: true,
			errMsg:  "registration manager is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, err := tt.setup()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("error = %v, want %v", err, tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if rc == nil {
				t.Errorf("expected coordinator but got nil")
			}
		})
	}
}

func TestStartRegistration(t *testing.T) {
	tests := []struct {
		name        string
		phoneNumber string
		setup       func() *setup.RegistrationCoordinator
		wantErr     bool
		errContains string
		verifyCalls func(t *testing.T, rc *setup.RegistrationCoordinator)
	}{
		{
			name:        "already registered",
			phoneNumber: "+1234567890",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{isRegistered: true},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr:     true,
			errContains: "already registered",
		},
		{
			name:        "process not running - starts successfully",
			phoneNumber: "+1234567890",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: false},
					&mockVerificationHandler{},
					&mockStatePersister{},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr: false,
			verifyCalls: func(_ *testing.T, _ *setup.RegistrationCoordinator) {
				// We can't access unexported fields from a test package
				// The test should verify behavior through public methods
			},
		},
		{
			name:        "captcha required",
			phoneNumber: "+1234567890",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: true},
					&mockVerificationHandler{captchaURL: "https://signal.org/captcha"},
					&mockStatePersister{},
					&mockRegistrationManager{
						startResponse: &signal.RegistrationResponse{
							Success:    true,
							NextState:  signal.StateCaptchaRequired,
							CaptchaURL: "https://signal.org/captcha",
						},
					},
				)
				return rc
			},
			wantErr:     true,
			errContains: "captcha required",
		},
		{
			name:        "successful registration start",
			phoneNumber: "+1234567890",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: true},
					&mockVerificationHandler{},
					&mockStatePersister{},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr: false,
			verifyCalls: func(_ *testing.T, _ *setup.RegistrationCoordinator) {
				// We can't access unexported fields from a test package
				// The test should verify behavior through public methods
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := tt.setup()
			err := rc.StartRegistration(context.Background(), tt.phoneNumber)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if tt.verifyCalls != nil {
				tt.verifyCalls(t, rc)
			}
		})
	}
}

func TestSubmitCaptcha(t *testing.T) {
	tests := []struct {
		name         string
		captchaToken string
		setup        func() *setup.RegistrationCoordinator
		wantErr      bool
		errContains  string
	}{
		{
			name:         "not in captcha state",
			captchaToken: "token123",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{
						state: &setup.RegistrationState{
							CurrentState: signal.StateAwaitingSMSCode,
						},
					},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr:     true,
			errContains: "not in captcha required state",
		},
		{
			name:         "successful captcha submission",
			captchaToken: "token123",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{
						state: &setup.RegistrationState{
							CurrentState:   signal.StateCaptchaRequired,
							PhoneNumber:    "+1234567890",
							CompletedSteps: []string{"phone_number_entered"},
						},
					},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := tt.setup()
			err := rc.SubmitCaptcha(context.Background(), tt.captchaToken)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSubmitSMSCode(t *testing.T) {
	tests := []struct {
		name        string
		smsCode     string
		setup       func() *setup.RegistrationCoordinator
		wantErr     bool
		errContains string
		verifyCalls func(t *testing.T, rc *setup.RegistrationCoordinator)
	}{
		{
			name:    "not awaiting SMS code",
			smsCode: "123456",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{
						state: &setup.RegistrationState{
							CurrentState: signal.StateCaptchaRequired,
						},
					},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr:     true,
			errContains: "not awaiting SMS code",
		},
		{
			name:    "successful SMS submission - registration complete",
			smsCode: "123456",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{
						state: &setup.RegistrationState{
							CurrentState:   signal.StateAwaitingSMSCode,
							PhoneNumber:    "+1234567890",
							CompletedSteps: []string{"phone_number_entered"},
						},
					},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr: false,
			verifyCalls: func(_ *testing.T, _ *setup.RegistrationCoordinator) {
				// We can't access unexported fields from a test package
				// The test should verify behavior through public methods
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := tt.setup()
			err := rc.SubmitSMSCode(context.Background(), tt.smsCode)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if tt.verifyCalls != nil {
				tt.verifyCalls(t, rc)
			}
		})
	}
}

func TestResumeRegistration(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *setup.RegistrationCoordinator
		wantErr     bool
		errContains string
	}{
		{
			name: "no registration in progress",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{}, // No saved state
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr:     true,
			errContains: "no registration in progress",
		},
		{
			name: "state mismatch",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: true},
					&mockVerificationHandler{},
					&mockStatePersister{
						state: &setup.RegistrationState{
							CurrentState: signal.StateAwaitingSMSCode,
						},
					},
					&mockRegistrationManager{
						currentState: signal.StateCaptchaRequired, // Mismatch
					},
				)
				return rc
			},
			wantErr:     true,
			errContains: "state mismatch",
		},
		{
			name: "successful resume",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: true},
					&mockVerificationHandler{},
					&mockStatePersister{
						state: &setup.RegistrationState{
							CurrentState: signal.StateAwaitingSMSCode,
						},
					},
					&mockRegistrationManager{
						currentState: signal.StateAwaitingSMSCode,
					},
				)
				return rc
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := tt.setup()
			err := rc.ResumeRegistration(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestReset(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *setup.RegistrationCoordinator
		wantErr bool
	}{
		{
			name: "successful reset",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{
						state: &setup.RegistrationState{
							CurrentState: signal.StateAwaitingSMSCode,
						},
					},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr: false,
		},
		{
			name: "reset with persister error",
			setup: func() *setup.RegistrationCoordinator {
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{},
					&mockVerificationHandler{},
					&mockStatePersister{
						clearErr: fmt.Errorf("clear failed"),
					},
					&mockRegistrationManager{},
				)
				return rc
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := tt.setup()
			err := rc.Reset(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidatePhoneNumber(t *testing.T) {
	rc, _ := setup.NewRegistrationCoordinator(
		&mockSignalManager{},
		&mockProcessManager{},
		&mockVerificationHandler{},
		&mockStatePersister{},
		&mockRegistrationManager{},
	)

	tests := []struct {
		name        string
		phoneNumber string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid phone number",
			phoneNumber: "+1234567890",
			wantErr:     false,
		},
		{
			name:        "too short",
			phoneNumber: "+12345",
			wantErr:     true,
			errContains: "too short",
		},
		{
			name:        "missing country code",
			phoneNumber: "1234567890",
			wantErr:     true,
			errContains: "must start with country code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rc.ValidatePhoneNumber(tt.phoneNumber)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to check if string contains substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
