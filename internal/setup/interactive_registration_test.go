package setup_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/setup"
	"github.com/Veraticus/mentat/internal/signal"
)

// interactiveTestPrompter implements setup.Prompter for testing interactive flows.
type interactiveTestPrompter struct {
	responses        map[string]string
	timeoutResponses map[string]string
	messages         []string
	errors           []string
	successes        []string
	capturedInputs   []string
}

func newInteractiveTestPrompter() *interactiveTestPrompter {
	return &interactiveTestPrompter{
		responses:        make(map[string]string),
		timeoutResponses: make(map[string]string),
		messages:         []string{},
		errors:           []string{},
		successes:        []string{},
		capturedInputs:   []string{},
	}
}

func (p *interactiveTestPrompter) Prompt(_ context.Context, message string) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)
	if resp, ok := p.responses[message]; ok {
		return resp, nil
	}
	return "", nil
}

func (p *interactiveTestPrompter) PromptWithDefault(_ context.Context, message, defaultValue string) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)
	if resp, ok := p.responses[message]; ok && resp != "" {
		return resp, nil
	}
	return defaultValue, nil
}

func (p *interactiveTestPrompter) PromptWithTimeout(
	_ context.Context,
	message string,
	_ time.Duration,
) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)
	if resp, ok := p.timeoutResponses[message]; ok {
		return resp, nil
	}
	return "", nil
}

func (p *interactiveTestPrompter) PromptSecret(_ context.Context, message string) (string, error) {
	p.capturedInputs = append(p.capturedInputs, message)
	if resp, ok := p.responses[message]; ok {
		return resp, nil
	}
	return "", nil
}

func (p *interactiveTestPrompter) ShowMessage(message string) {
	p.messages = append(p.messages, message)
}

func (p *interactiveTestPrompter) ShowError(message string) {
	p.errors = append(p.errors, message)
}

func (p *interactiveTestPrompter) ShowSuccess(message string) {
	p.successes = append(p.successes, message)
}

// mockInteractiveStatePersister for testing to avoid name collision.
type mockInteractiveStatePersister struct {
	state    *setup.RegistrationState
	loadErr  error
	saveErr  error
	clearErr error
}

func (m *mockInteractiveStatePersister) LoadState(_ context.Context) (*setup.RegistrationState, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.state, nil
}

func (m *mockInteractiveStatePersister) SaveState(_ context.Context, state *setup.RegistrationState) error {
	m.state = state
	return m.saveErr
}

func (m *mockInteractiveStatePersister) ClearState(_ context.Context) error {
	m.state = nil
	return m.clearErr
}

func TestNewInteractiveRegistrationFlow(t *testing.T) {
	tests := []struct {
		name        string
		coordinator *setup.RegistrationCoordinator
		prompter    setup.Prompter
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil coordinator",
			coordinator: nil,
			prompter:    newInteractiveTestPrompter(),
			wantErr:     true,
			errContains: "registration coordinator is required",
		},
		{
			name:        "nil prompter",
			coordinator: &setup.RegistrationCoordinator{},
			prompter:    nil,
			wantErr:     true,
			errContains: "prompter is required",
		},
		{
			name:        "valid flow",
			coordinator: &setup.RegistrationCoordinator{},
			prompter:    newInteractiveTestPrompter(),
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow, err := setup.NewInteractiveRegistrationFlow(tt.coordinator, tt.prompter)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want to contain %v", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if flow == nil {
				t.Errorf("expected non-nil flow")
			}
		})
	}
}

func TestInteractiveRegistrationFlow_RunRegistration(t *testing.T) {
	tests := []struct {
		name        string
		phoneNumber string
		setupMocks  func() (*setup.RegistrationCoordinator, *interactiveTestPrompter, *mockInteractiveStatePersister)
		wantErr     bool
		errContains string
		verifyMocks func(t *testing.T, prompter *interactiveTestPrompter)
	}{
		{
			name:        "invalid phone number",
			phoneNumber: "",
			setupMocks: func() (*setup.RegistrationCoordinator, *interactiveTestPrompter, *mockInteractiveStatePersister) {
				persister := &mockInteractiveStatePersister{}
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: true},
					&mockVerificationHandler{},
					persister,
					&mockRegistrationManager{},
				)
				prompter := newInteractiveTestPrompter()
				return rc, prompter, persister
			},
			wantErr:     true,
			errContains: "invalid phone number",
		},
		{
			name:        "registration with captcha",
			phoneNumber: "+1234567890",
			setupMocks: func() (*setup.RegistrationCoordinator, *interactiveTestPrompter, *mockInteractiveStatePersister) {
				persister := &mockInteractiveStatePersister{
					state: &setup.RegistrationState{
						CurrentState: signal.StateAwaitingSMSCode,
						PhoneNumber:  "+1234567890",
					},
				}
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: true},
					&mockVerificationHandler{captchaURL: "https://signal.org/captcha"},
					persister,
					&mockRegistrationManager{
						startResponse: &signal.RegistrationResponse{
							Success:    true,
							NextState:  signal.StateCaptchaRequired,
							CaptchaURL: "https://signal.org/captcha",
						},
					},
				)
				prompter := newInteractiveTestPrompter()
				prompter.timeoutResponses["Paste the captcha token here (or 'retry' for new instructions)"] = "signalcaptcha://valid-token-12345678901234567890"
				prompter.timeoutResponses["Enter the 6-digit verification code"] = "123456"
				return rc, prompter, persister
			},
			wantErr: false,
			verifyMocks: func(t *testing.T, prompter *interactiveTestPrompter) {
				t.Helper()
				// Verify success message was shown
				found := false
				for _, msg := range prompter.successes {
					if strings.Contains(msg, "Registration completed successfully") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected success message not found")
				}
			},
		},
		{
			name:        "direct to SMS verification",
			phoneNumber: "+1234567890",
			setupMocks: func() (*setup.RegistrationCoordinator, *interactiveTestPrompter, *mockInteractiveStatePersister) {
				persister := &mockInteractiveStatePersister{
					state: &setup.RegistrationState{
						CurrentState: signal.StateAwaitingSMSCode,
						PhoneNumber:  "+1234567890",
					},
				}
				rc, _ := setup.NewRegistrationCoordinator(
					&mockSignalManager{},
					&mockProcessManager{isRunning: true},
					&mockVerificationHandler{},
					persister,
					&mockRegistrationManager{
						startResponse: &signal.RegistrationResponse{
							Success:   true,
							NextState: signal.StateAwaitingSMSCode,
						},
					},
				)
				prompter := newInteractiveTestPrompter()
				prompter.timeoutResponses["Enter the 6-digit verification code"] = "123456"
				return rc, prompter, persister
			},
			wantErr: false,
			verifyMocks: func(t *testing.T, prompter *interactiveTestPrompter) {
				t.Helper()
				// Verify success message was shown
				found := false
				for _, msg := range prompter.successes {
					if strings.Contains(msg, "Registration completed successfully") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected success message not found")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, prompter, _ := tt.setupMocks()

			flow, _ := setup.NewInteractiveRegistrationFlow(rc, prompter)

			err := flow.RunRegistration(context.Background(), tt.phoneNumber)

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

			if tt.verifyMocks != nil {
				tt.verifyMocks(t, prompter)
			}
		})
	}
}

func TestInteractiveRegistrationFlow_Reset(t *testing.T) {
	persister := &mockInteractiveStatePersister{}
	rc, _ := setup.NewRegistrationCoordinator(
		&mockSignalManager{},
		&mockProcessManager{isRunning: true},
		&mockVerificationHandler{},
		persister,
		&mockRegistrationManager{},
	)

	flow, _ := setup.NewInteractiveRegistrationFlow(rc, newInteractiveTestPrompter())

	err := flow.Reset(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInteractiveRegistrationFlow_GetRegistrationState(t *testing.T) {
	expectedState := &setup.RegistrationState{
		CurrentState: signal.StateAwaitingSMSCode,
		PhoneNumber:  "+1234567890",
	}

	persister := &mockInteractiveStatePersister{
		state: expectedState,
	}

	rc, _ := setup.NewRegistrationCoordinator(
		&mockSignalManager{},
		&mockProcessManager{isRunning: true},
		&mockVerificationHandler{},
		persister,
		&mockRegistrationManager{},
	)

	flow, _ := setup.NewInteractiveRegistrationFlow(rc, newInteractiveTestPrompter())

	state, err := flow.GetRegistrationState(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if state == nil {
		t.Fatal("expected non-nil state")
	}

	if state.PhoneNumber != expectedState.PhoneNumber {
		t.Errorf("phone number = %v, want %v", state.PhoneNumber, expectedState.PhoneNumber)
	}
}
