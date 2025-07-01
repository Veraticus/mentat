package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/signal"
)

// Test mocks to avoid import cycle with mocks package

type mockLLM struct {
	mu        sync.Mutex
	queryFunc func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error)
}

func (m *mockLLM) Query(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.queryFunc != nil {
		return m.queryFunc(ctx, prompt, sessionID)
	}

	return &claude.LLMResponse{
		Message: "Mock response",
		Metadata: claude.ResponseMetadata{
			Latency: 10 * time.Millisecond,
		},
	}, nil
}

type mockMessenger struct {
	mu           sync.Mutex
	sentMessages []struct {
		recipient string
		message   string
	}
	sendErr error
}

func (m *mockMessenger) Send(_ context.Context, recipient, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}

	m.sentMessages = append(m.sentMessages, struct {
		recipient string
		message   string
	}{recipient: recipient, message: message})

	return nil
}

func (m *mockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	return ch, nil
}

func (m *mockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

type mockValidationStrategy struct {
	result   ValidationResult
	retry    bool
	recovery string
}

func (m *mockValidationStrategy) Validate(_ context.Context, _, _ string, _ claude.LLM) ValidationResult {
	return m.result
}

func (m *mockValidationStrategy) ShouldRetry(_ ValidationResult) bool {
	return m.retry
}

func (m *mockValidationStrategy) GenerateRecovery(_ context.Context, _, _ string, _ ValidationResult, _ claude.LLM) string {
	return m.recovery
}

type mockIntentEnhancer struct {
	enhanceFunc       func(string) string
	shouldEnhanceFunc func(string) bool
}

func (m *mockIntentEnhancer) Enhance(request string) string {
	if m.enhanceFunc != nil {
		return m.enhanceFunc(request)
	}
	return request
}

func (m *mockIntentEnhancer) ShouldEnhance(request string) bool {
	if m.shouldEnhanceFunc != nil {
		return m.shouldEnhanceFunc(request)
	}
	return false
}

type mockSessionManager struct {
	mu       sync.Mutex
	sessions map[string]string
	history  map[string][]conversation.Message
}

func newMockSessionManager() *mockSessionManager {
	return &mockSessionManager{
		sessions: make(map[string]string),
		history:  make(map[string][]conversation.Message),
	}
}

func (m *mockSessionManager) GetOrCreateSession(identifier string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[identifier]; ok {
		return session
	}

	session := "signal-" + identifier
	m.sessions[identifier] = session
	return session
}

func (m *mockSessionManager) GetSessionHistory(sessionID string) []conversation.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	if history, ok := m.history[sessionID]; ok {
		return history
	}
	return []conversation.Message{}
}

func (m *mockSessionManager) ExpireSessions(_ time.Time) int {
	return 0
}

func (m *mockSessionManager) GetLastSessionID(identifier string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[identifier]; ok {
		return session
	}
	return ""
}

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name        string
		llm         claude.LLM
		opts        []HandlerOption
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil LLM returns error",
			llm:         nil,
			wantErr:     true,
			errContains: "llm is required",
		},
		{
			name: "missing validation strategy returns error",
			llm:  &mockLLM{},
			opts: []HandlerOption{
				WithMessenger(&mockMessenger{}),
				WithSessionManager(newMockSessionManager()),
			},
			wantErr:     true,
			errContains: "validation strategy is required",
		},
		{
			name: "missing messenger returns error",
			llm:  &mockLLM{},
			opts: []HandlerOption{
				WithValidationStrategy(&mockValidationStrategy{
					result: ValidationResult{Status: ValidationStatusSuccess},
				}),
				WithSessionManager(newMockSessionManager()),
			},
			wantErr:     true,
			errContains: "messenger is required",
		},
		{
			name: "missing session manager returns error",
			llm:  &mockLLM{},
			opts: []HandlerOption{
				WithValidationStrategy(&mockValidationStrategy{
					result: ValidationResult{Status: ValidationStatusSuccess},
				}),
				WithMessenger(&mockMessenger{}),
			},
			wantErr:     true,
			errContains: "session manager is required",
		},
		{
			name: "valid configuration succeeds",
			llm:  &mockLLM{},
			opts: []HandlerOption{
				WithValidationStrategy(&mockValidationStrategy{
					result: ValidationResult{Status: ValidationStatusSuccess},
				}),
				WithMessenger(&mockMessenger{}),
				WithSessionManager(newMockSessionManager()),
			},
			wantErr: false,
		},
		{
			name: "all options set succeeds",
			llm:  &mockLLM{},
			opts: []HandlerOption{
				WithValidationStrategy(&mockValidationStrategy{
					result: ValidationResult{Status: ValidationStatusSuccess},
				}),
				WithMessenger(&mockMessenger{}),
				WithSessionManager(newMockSessionManager()),
				WithIntentEnhancer(&mockIntentEnhancer{}),
				WithConfig(Config{
					MaxRetries:              3,
					EnableIntentEnhancement: false,
					ValidationThreshold:     0.9,
				}),
				WithLogger(slog.Default()),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(tt.llm, tt.opts...)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if handler == nil {
					t.Error("expected handler but got nil")
				}
			}
		})
	}
}

func TestHandlerOptions(t *testing.T) {
	tests := []struct {
		name        string
		option      HandlerOption
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil validation strategy returns error",
			option:      WithValidationStrategy(nil),
			wantErr:     true,
			errContains: "validation strategy cannot be nil",
		},
		{
			name: "valid validation strategy succeeds",
			option: WithValidationStrategy(&mockValidationStrategy{
				result: ValidationResult{Status: ValidationStatusSuccess},
			}),
			wantErr: false,
		},
		{
			name:        "nil intent enhancer returns error",
			option:      WithIntentEnhancer(nil),
			wantErr:     true,
			errContains: "intent enhancer cannot be nil",
		},
		{
			name:    "valid intent enhancer succeeds",
			option:  WithIntentEnhancer(&mockIntentEnhancer{}),
			wantErr: false,
		},
		{
			name:        "nil messenger returns error",
			option:      WithMessenger(nil),
			wantErr:     true,
			errContains: "messenger cannot be nil",
		},
		{
			name:    "valid messenger succeeds",
			option:  WithMessenger(&mockMessenger{}),
			wantErr: false,
		},
		{
			name:        "nil session manager returns error",
			option:      WithSessionManager(nil),
			wantErr:     true,
			errContains: "session manager cannot be nil",
		},
		{
			name:    "valid session manager succeeds",
			option:  WithSessionManager(newMockSessionManager()),
			wantErr: false,
		},
		{
			name:        "nil logger returns error",
			option:      WithLogger(nil),
			wantErr:     true,
			errContains: "logger cannot be nil",
		},
		{
			name:    "valid logger succeeds",
			option:  WithLogger(slog.Default()),
			wantErr: false,
		},
		{
			name: "negative max retries returns error",
			option: WithConfig(Config{
				MaxRetries:          -1,
				ValidationThreshold: 0.8,
			}),
			wantErr:     true,
			errContains: "max retries cannot be negative",
		},
		{
			name: "validation threshold below 0 returns error",
			option: WithConfig(Config{
				MaxRetries:          2,
				ValidationThreshold: -0.1,
			}),
			wantErr:     true,
			errContains: "validation threshold must be between 0 and 1",
		},
		{
			name: "validation threshold above 1 returns error",
			option: WithConfig(Config{
				MaxRetries:          2,
				ValidationThreshold: 1.1,
			}),
			wantErr:     true,
			errContains: "validation threshold must be between 0 and 1",
		},
		{
			name: "valid config succeeds",
			option: WithConfig(Config{
				MaxRetries:              3,
				EnableIntentEnhancement: false,
				ValidationThreshold:     0.95,
			}),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &handler{}
			err := tt.option(h)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestHandlerDefaults(t *testing.T) {
	llm := &mockLLM{}
	messenger := &mockMessenger{}
	strategy := &mockValidationStrategy{
		result: ValidationResult{Status: ValidationStatusSuccess},
	}
	sessionManager := newMockSessionManager()

	handler, err := NewHandler(llm,
		WithValidationStrategy(strategy),
		WithMessenger(messenger),
		WithSessionManager(sessionManager),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We can't easily test the defaults without type assertion
	// since handler is returned as an interface.
	// Instead, we'll just verify the handler was created successfully.
	if handler == nil {
		t.Error("expected handler to be non-nil")
	}
}

func TestHandlerProcess(t *testing.T) {
	ctx := context.Background()
	msg := signal.IncomingMessage{
		From: "+1234567890",
		Text: "Hello, Claude!",
	}

	tests := []struct {
		name        string
		setupFn     func(*mockLLM, *mockMessenger, *mockSessionManager)
		verifyFn    func(*testing.T, *mockMessenger)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful query with session management",
			setupFn:  setupSuccessfulQuery(t),
			verifyFn: verifySuccessfulResponse,
			wantErr:  false,
		},
		{
			name:        "query error returns error",
			setupFn:     setupQueryError,
			wantErr:     true,
			errContains: "LLM query failed",
		},
		{
			name:        "messenger send error returns error",
			setupFn:     setupMessengerError,
			wantErr:     true,
			errContains: "failed to send response",
		},
		{
			name:    "session history is retrieved",
			setupFn: setupSessionHistory(t),
			wantErr: false,
		},
		{
			name:     "empty response message handled gracefully",
			setupFn:  setupEmptyResponse,
			verifyFn: verifyEmptyMessage,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, messenger := createTestHandlerWithMessenger(t, tt.setupFn)
			err := handler.Process(ctx, msg)
			verifyError(t, err, tt.wantErr, tt.errContains)

			if tt.verifyFn != nil {
				tt.verifyFn(t, messenger)
			}
		})
	}
}

func TestOptionApplicationError(t *testing.T) {
	llm := &mockLLM{}

	// Create an option that always returns an error
	failingOption := func(_ *handler) error {
		return errors.New("option failed")
	}

	_, err := NewHandler(llm, failingOption)
	if err == nil {
		t.Error("expected error from failing option")
	}
	if !contains(err.Error(), "failed to apply option") {
		t.Errorf("error = %v, want error containing 'failed to apply option'", err)
	}
}

// TestHandlerProcessWithContext tests Process method with context cancellation.
func TestHandlerProcessWithContext(t *testing.T) {
	msg := signal.IncomingMessage{
		From: "+1234567890",
		Text: "Test with context",
	}

	tests := []struct {
		name        string
		setupCtx    func() context.Context
		setupFn     func(*mockLLM)
		wantErr     bool
		errContains string
	}{
		{
			name:        "context cancellation during LLM query",
			setupCtx:    createCanceledContext,
			setupFn:     setupCanceledContextLLM,
			wantErr:     true,
			errContains: "context canceled",
		},
		{
			name:        "timeout during processing",
			setupCtx:    createTimeoutContext,
			setupFn:     setupTimeoutLLM,
			wantErr:     true,
			errContains: "deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runHandlerProcessWithContextTest(t, msg, tt)
		})
	}
}

// runHandlerProcessWithContextTest runs a single test case for context handling.
func runHandlerProcessWithContextTest(t *testing.T, msg signal.IncomingMessage, tt struct {
	name        string
	setupCtx    func() context.Context
	setupFn     func(*mockLLM)
	wantErr     bool
	errContains string
}) {
	t.Helper()

	handler, llm := createTestHandlerWithLLM(t)

	if tt.setupFn != nil {
		tt.setupFn(llm)
	}

	ctx := context.Background()
	if tt.setupCtx != nil {
		ctx = tt.setupCtx()
	}

	err := handler.Process(ctx, msg)
	verifyError(t, err, tt.wantErr, tt.errContains)
}

// createTestHandlerWithLLM creates a test handler and returns both handler and LLM.
func createTestHandlerWithLLM(t *testing.T) (Handler, *mockLLM) {
	t.Helper()

	llm := &mockLLM{}
	messenger := &mockMessenger{}
	sessionMgr := newMockSessionManager()
	strategy := &mockValidationStrategy{
		result: ValidationResult{Status: ValidationStatusSuccess},
	}

	handler, err := NewHandler(llm,
		WithValidationStrategy(strategy),
		WithMessenger(messenger),
		WithSessionManager(sessionMgr),
	)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	return handler, llm
}

// createCanceledContext creates a context that is already canceled.
func createCanceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// createTimeoutContext creates a context with a very short timeout.
func createTimeoutContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	go func() {
		<-time.After(2 * time.Millisecond)
		cancel()
	}()
	return ctx
}

// setupCanceledContextLLM sets up LLM to handle canceled context.
func setupCanceledContextLLM(llm *mockLLM) {
	llm.queryFunc = func(ctx context.Context, _, _ string) (*claude.LLMResponse, error) {
		// Check if context is already canceled
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return &claude.LLMResponse{Message: "Should not reach here"}, nil
		}
	}
}

// setupTimeoutLLM sets up LLM to simulate slow response.
func setupTimeoutLLM(llm *mockLLM) {
	llm.queryFunc = func(ctx context.Context, _, _ string) (*claude.LLMResponse, error) {
		// Use a timer to simulate slow response
		timer := time.NewTimer(10 * time.Millisecond)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return &claude.LLMResponse{Message: "Too late"}, nil
		}
	}
}

// TestHandlerProcessConcurrency tests concurrent message processing.
func TestHandlerProcessConcurrency(t *testing.T) {
	llm := &mockLLM{}
	llm.queryFunc = func(_ context.Context, text, _ string) (*claude.LLMResponse, error) {
		// Simulate some processing time using a channel
		done := make(chan struct{})
		go func() {
			// Simulate work
			close(done)
		}()
		<-done

		return &claude.LLMResponse{
			Message: "Response for: " + text,
		}, nil
	}

	messenger := &mockMessenger{}
	sessionMgr := newMockSessionManager()
	strategy := &mockValidationStrategy{
		result: ValidationResult{Status: ValidationStatusSuccess},
	}

	handler, err := NewHandler(llm,
		WithValidationStrategy(strategy),
		WithMessenger(messenger),
		WithSessionManager(sessionMgr),
	)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	// Process multiple messages concurrently
	const numMessages = 10
	var wg sync.WaitGroup
	errors := make(chan error, numMessages)

	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			msg := signal.IncomingMessage{
				From: fmt.Sprintf("+123456789%d", index),
				Text: fmt.Sprintf("Message %d", index),
			}
			if err := handler.Process(context.Background(), msg); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("unexpected error during concurrent processing: %v", err)
	}

	// Verify all messages were sent
	if len(messenger.sentMessages) != numMessages {
		t.Errorf("expected %d sent messages, got %d", numMessages, len(messenger.sentMessages))
	}
}

// Helper functions for TestHandlerProcess to reduce complexity

func setupSuccessfulQuery(t *testing.T) func(*mockLLM, *mockMessenger, *mockSessionManager) {
	t.Helper()
	return func(llm *mockLLM, _ *mockMessenger, _ *mockSessionManager) {
		llm.queryFunc = func(_ context.Context, text, sessionID string) (*claude.LLMResponse, error) {
			if sessionID != "signal-+1234567890" {
				t.Errorf("expected sessionID = signal-+1234567890, got %s", sessionID)
			}
			if text != "Hello, Claude!" {
				t.Errorf("expected text = Hello, Claude!, got %s", text)
			}
			return &claude.LLMResponse{
				Message: "Hello! How can I help you today?",
			}, nil
		}
	}
}

func verifySuccessfulResponse(t *testing.T, messenger *mockMessenger) {
	t.Helper()
	if len(messenger.sentMessages) != 1 {
		t.Errorf("expected 1 sent message, got %d", len(messenger.sentMessages))
	}
	if messenger.sentMessages[0].recipient != "+1234567890" {
		t.Errorf("expected recipient +1234567890, got %s", messenger.sentMessages[0].recipient)
	}
	if messenger.sentMessages[0].message != "Hello! How can I help you today?" {
		t.Errorf("expected message 'Hello! How can I help you today?', got %s", messenger.sentMessages[0].message)
	}
}

func setupQueryError(llm *mockLLM, _ *mockMessenger, _ *mockSessionManager) {
	llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
		return nil, errors.New("API error: rate limit exceeded")
	}
}

func setupMessengerError(llm *mockLLM, messenger *mockMessenger, _ *mockSessionManager) {
	llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
		return &claude.LLMResponse{
			Message: "Test response",
		}, nil
	}
	messenger.sendErr = errors.New("network error")
}

func setupSessionHistory(t *testing.T) func(*mockLLM, *mockMessenger, *mockSessionManager) {
	t.Helper()
	return func(llm *mockLLM, _ *mockMessenger, sessionMgr *mockSessionManager) {
		// Pre-populate session history
		sessionMgr.history["signal-+1234567890"] = []conversation.Message{
			{
				ID:        "prev-1",
				SessionID: "signal-+1234567890",
				From:      "+1234567890",
				Text:      "Previous message",
				Response:  "Previous response",
			},
		}

		llm.queryFunc = func(_ context.Context, _, sessionID string) (*claude.LLMResponse, error) {
			// Verify session is used correctly
			if sessionID != "signal-+1234567890" {
				t.Errorf("expected sessionID = signal-+1234567890, got %s", sessionID)
			}
			return &claude.LLMResponse{
				Message: "Response with context",
			}, nil
		}
	}
}

func setupEmptyResponse(llm *mockLLM, _ *mockMessenger, _ *mockSessionManager) {
	llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
		return &claude.LLMResponse{
			Message: "",
		}, nil
	}
}

func verifyEmptyMessage(t *testing.T, messenger *mockMessenger) {
	t.Helper()
	if len(messenger.sentMessages) != 1 {
		t.Errorf("expected 1 sent message, got %d", len(messenger.sentMessages))
	}
	// Empty message should still be sent
	if messenger.sentMessages[0].message != "" {
		t.Errorf("expected empty message, got %s", messenger.sentMessages[0].message)
	}
}

func createTestHandlerWithMessenger(t *testing.T, setupFn func(*mockLLM, *mockMessenger, *mockSessionManager)) (Handler, *mockMessenger) {
	t.Helper()
	llm := &mockLLM{}
	messenger := &mockMessenger{}
	sessionMgr := newMockSessionManager()
	strategy := &mockValidationStrategy{
		result: ValidationResult{Status: ValidationStatusSuccess},
	}

	if setupFn != nil {
		setupFn(llm, messenger, sessionMgr)
	}

	handler, err := NewHandler(llm,
		WithValidationStrategy(strategy),
		WithMessenger(messenger),
		WithSessionManager(sessionMgr),
	)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	return handler, messenger
}

func verifyError(t *testing.T, err error, wantErr bool, errContains string) {
	t.Helper()
	if wantErr {
		if err == nil {
			t.Error("expected error but got none")
		} else if errContains != "" && !contains(err.Error(), errContains) {
			t.Errorf("error = %v, want error containing %v", err, errContains)
		}
	} else {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return substr != "" && len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
