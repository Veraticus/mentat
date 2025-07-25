package agent_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/signal"
)

// Test mocks to avoid import cycle with mocks package

type mockLLM struct {
	mu        sync.Mutex
	queryFunc func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error)
}

func (m *mockLLM) Query(
	ctx context.Context,
	prompt, sessionID string,
) (*claude.LLMResponse, error) {
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
	result   agent.ValidationResult
	retry    bool
	recovery string
}

func (m *mockValidationStrategy) Validate(
	_ context.Context,
	_, _, _ string,
	_ claude.LLM,
) agent.ValidationResult {
	return m.result
}

func (m *mockValidationStrategy) ShouldRetry(_ agent.ValidationResult) bool {
	return m.retry
}

func (m *mockValidationStrategy) GenerateRecovery(
	_ context.Context,
	_, _, _ string,
	_ agent.ValidationResult,
	_ claude.LLM,
) string {
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
	t.Run("nil LLM returns error", testNilLLM)
	t.Run("missing dependencies", testMissingDependencies)
	t.Run("valid configurations", testValidConfigurations)
}

func testNilLLM(t *testing.T) {
	handler, err := agent.NewHandler(nil)
	if err == nil {
		t.Errorf("expected error but got none")
		return
	}
	if !contains(err.Error(), "llm is required") {
		t.Errorf("error = %v, want error containing 'llm is required'", err)
	}
	if handler != nil {
		t.Error("expected nil handler but got non-nil")
	}
}

func testMissingDependencies(t *testing.T) {
	tests := []struct {
		name        string
		opts        []agent.HandlerOption
		errContains string
	}{
		{
			name: "missing validation strategy",
			opts: []agent.HandlerOption{
				agent.WithMessenger(&mockMessenger{}),
				agent.WithSessionManager(newMockSessionManager()),
			},
			errContains: "validation strategy is required",
		},
		{
			name: "missing messenger",
			opts: []agent.HandlerOption{
				agent.WithValidationStrategy(&mockValidationStrategy{
					result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
				}),
				agent.WithSessionManager(newMockSessionManager()),
			},
			errContains: "messenger is required",
		},
		{
			name: "missing session manager",
			opts: []agent.HandlerOption{
				agent.WithValidationStrategy(&mockValidationStrategy{
					result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
				}),
				agent.WithMessenger(&mockMessenger{}),
			},
			errContains: "session manager is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := agent.NewHandler(&mockLLM{}, tt.opts...)
			if err == nil {
				t.Errorf("expected error but got none")
				return
			}
			if !contains(err.Error(), tt.errContains) {
				t.Errorf("error = %v, want error containing %v", err, tt.errContains)
			}
			if handler != nil {
				t.Error("expected nil handler but got non-nil")
			}
		})
	}
}

func testValidConfigurations(t *testing.T) {
	tests := []struct {
		name string
		opts []agent.HandlerOption
	}{
		{
			name: "minimum required options",
			opts: []agent.HandlerOption{
				agent.WithValidationStrategy(&mockValidationStrategy{
					result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
				}),
				agent.WithMessenger(&mockMessenger{}),
				agent.WithSessionManager(newMockSessionManager()),
			},
		},
		{
			name: "all options set",
			opts: []agent.HandlerOption{
				agent.WithValidationStrategy(&mockValidationStrategy{
					result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
				}),
				agent.WithMessenger(&mockMessenger{}),
				agent.WithSessionManager(newMockSessionManager()),
				agent.WithIntentEnhancer(&mockIntentEnhancer{}),
				agent.WithConfig(agent.Config{
					MaxRetries:              3,
					EnableIntentEnhancement: false,
					ValidationThreshold:     0.9,
				}),
				agent.WithLogger(slog.Default()),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := agent.NewHandler(&mockLLM{}, tt.opts...)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if handler == nil {
				t.Error("expected handler but got nil")
			}
		})
	}
}

func TestHandlerOptions(t *testing.T) {
	t.Run("ValidationStrategy", testValidationStrategyOptions)
	t.Run("IntentEnhancer", testIntentEnhancerOptions)
	t.Run("Messenger", testMessengerOptions)
	t.Run("SessionManager", testSessionManagerOptions)
	t.Run("Logger", testLoggerOptions)
	t.Run("Config", testConfigOptions)
}

func testValidationStrategyOptions(t *testing.T) {
	runOptionTest(t, []optionTest{
		{
			name:        "nil validation strategy returns error",
			option:      agent.WithValidationStrategy(nil),
			wantErr:     true,
			errContains: "validation strategy cannot be nil",
		},
		{
			name: "valid validation strategy succeeds",
			option: agent.WithValidationStrategy(&mockValidationStrategy{
				result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
			}),
			wantErr: false,
		},
	})
}

func testIntentEnhancerOptions(t *testing.T) {
	runOptionTest(t, []optionTest{
		{
			name:        "nil intent enhancer returns error",
			option:      agent.WithIntentEnhancer(nil),
			wantErr:     true,
			errContains: "intent enhancer cannot be nil",
		},
		{
			name:    "valid intent enhancer succeeds",
			option:  agent.WithIntentEnhancer(&mockIntentEnhancer{}),
			wantErr: false,
		},
	})
}

func testMessengerOptions(t *testing.T) {
	runOptionTest(t, []optionTest{
		{
			name:        "nil messenger returns error",
			option:      agent.WithMessenger(nil),
			wantErr:     true,
			errContains: "messenger cannot be nil",
		},
		{
			name:    "valid messenger succeeds",
			option:  agent.WithMessenger(&mockMessenger{}),
			wantErr: false,
		},
	})
}

func testSessionManagerOptions(t *testing.T) {
	runOptionTest(t, []optionTest{
		{
			name:        "nil session manager returns error",
			option:      agent.WithSessionManager(nil),
			wantErr:     true,
			errContains: "session manager cannot be nil",
		},
		{
			name:    "valid session manager succeeds",
			option:  agent.WithSessionManager(newMockSessionManager()),
			wantErr: false,
		},
	})
}

func testLoggerOptions(t *testing.T) {
	runOptionTest(t, []optionTest{
		{
			name:        "nil logger returns error",
			option:      agent.WithLogger(nil),
			wantErr:     true,
			errContains: "logger cannot be nil",
		},
		{
			name:    "valid logger succeeds",
			option:  agent.WithLogger(slog.Default()),
			wantErr: false,
		},
	})
}

func testConfigOptions(t *testing.T) {
	runOptionTest(t, []optionTest{
		{
			name: "negative max retries returns error",
			option: agent.WithConfig(agent.Config{
				MaxRetries:          -1,
				ValidationThreshold: 0.8,
			}),
			wantErr:     true,
			errContains: "max retries cannot be negative",
		},
		{
			name: "validation threshold below 0 returns error",
			option: agent.WithConfig(agent.Config{
				MaxRetries:          2,
				ValidationThreshold: -0.1,
			}),
			wantErr:     true,
			errContains: "validation threshold must be between 0 and 1",
		},
		{
			name: "validation threshold above 1 returns error",
			option: agent.WithConfig(agent.Config{
				MaxRetries:          2,
				ValidationThreshold: 1.1,
			}),
			wantErr:     true,
			errContains: "validation threshold must be between 0 and 1",
		},
		{
			name: "valid config succeeds",
			option: agent.WithConfig(agent.Config{
				MaxRetries:              3,
				EnableIntentEnhancement: false,
				ValidationThreshold:     0.95,
			}),
			wantErr: false,
		},
	})
}

type optionTest struct {
	name        string
	option      agent.HandlerOption
	wantErr     bool
	errContains string
}

func runOptionTest(t *testing.T, tests []optionTest) {
	t.Helper()

	// Create default valid dependencies
	defaultValidationStrategy := &mockValidationStrategy{
		result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
	}
	defaultMessenger := &mockMessenger{}
	defaultSessionManager := newMockSessionManager()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a handler with all required dependencies plus the option being tested
			llm := &mockLLM{}
			opts := []agent.HandlerOption{
				agent.WithValidationStrategy(defaultValidationStrategy),
				agent.WithMessenger(defaultMessenger),
				agent.WithSessionManager(defaultSessionManager),
				tt.option, // Add the option being tested
			}

			_, err := agent.NewHandler(llm, opts...)
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
		result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
	}
	sessionManager := newMockSessionManager()

	handler, err := agent.NewHandler(llm,
		agent.WithValidationStrategy(strategy),
		agent.WithMessenger(messenger),
		agent.WithSessionManager(sessionManager),
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
	// Since HandlerOption expects *handler (unexported), we can't create a custom failing option
	// from the test package. This test is no longer possible with separate test packages.
	// Instead, test error handling with nil LLM
	_, err := agent.NewHandler(nil)
	if err == nil {
		t.Error("expected error from failing option")
	}
	if !contains(err.Error(), "llm is required") {
		t.Errorf("error = %v, want error containing 'llm is required'", err)
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
func createTestHandlerWithLLM(t *testing.T) (agent.Handler, *mockLLM) {
	t.Helper()

	llm := &mockLLM{}
	messenger := &mockMessenger{}
	sessionMgr := newMockSessionManager()
	strategy := &mockValidationStrategy{
		result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
	}

	handler, err := agent.NewHandler(llm,
		agent.WithValidationStrategy(strategy),
		agent.WithMessenger(messenger),
		agent.WithSessionManager(sessionMgr),
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
		result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
	}

	handler, err := agent.NewHandler(llm,
		agent.WithValidationStrategy(strategy),
		agent.WithMessenger(messenger),
		agent.WithSessionManager(sessionMgr),
	)
	if err != nil {
		t.Fatalf("unexpected error creating handler: %v", err)
	}

	// Process multiple messages concurrently
	const numMessages = 10
	var wg sync.WaitGroup
	errors := make(chan error, numMessages)

	for i := range numMessages {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			msg := signal.IncomingMessage{
				From: fmt.Sprintf("+123456789%d", index),
				Text: fmt.Sprintf("Message %d", index),
			}
			if processErr := handler.Process(context.Background(), msg); processErr != nil {
				errors <- processErr
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
		t.Errorf(
			"expected message 'Hello! How can I help you today?', got %s",
			messenger.sentMessages[0].message,
		)
	}
}

func setupQueryError(llm *mockLLM, _ *mockMessenger, _ *mockSessionManager) {
	llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
		return nil, fmt.Errorf("API error: rate limit exceeded")
	}
}

func setupMessengerError(llm *mockLLM, messenger *mockMessenger, _ *mockSessionManager) {
	llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
		return &claude.LLMResponse{
			Message: "Test response",
		}, nil
	}
	messenger.sendErr = fmt.Errorf("network error")
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

func createTestHandlerWithMessenger(
	t *testing.T,
	setupFn func(*mockLLM, *mockMessenger, *mockSessionManager),
) (agent.Handler, *mockMessenger) {
	t.Helper()
	llm := &mockLLM{}
	messenger := &mockMessenger{}
	sessionMgr := newMockSessionManager()
	strategy := &mockValidationStrategy{
		result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
	}

	if setupFn != nil {
		setupFn(llm, messenger, sessionMgr)
	}

	handler, err := agent.NewHandler(llm,
		agent.WithValidationStrategy(strategy),
		agent.WithMessenger(messenger),
		agent.WithSessionManager(sessionMgr),
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
	return substr != "" && len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

// TestSmartInitialResponse tests the smart initial response system for Phase 56.
func TestSmartInitialResponse(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		request          string
		setupLLM         func(*mockLLM)
		expectValidation bool
		verifyFn         func(*testing.T, *claude.LLMResponse, error)
	}{
		{
			name:    "simple query completes without validation",
			request: "What time is it?",
			setupLLM: func(llm *mockLLM) {
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{
						Message: "It's 3:00 PM.",
						Progress: &claude.ProgressInfo{
							NeedsContinuation:  false,
							Status:             "complete",
							Message:            "Simple time query answered",
							EstimatedRemaining: 0,
							NeedsValidation:    false,
						},
					}, nil
				}
			},
			expectValidation: false,
			verifyFn: func(t *testing.T, resp *claude.LLMResponse, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp.Message != "It's 3:00 PM." {
					t.Errorf("expected message 'It's 3:00 PM.', got %s", resp.Message)
				}
			},
		},
		{
			name:    "complex query with needs_continuation skips validation",
			request: "Schedule a meeting with John next week",
			setupLLM: func(llm *mockLLM) {
				callCount := 0
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					callCount++
					if callCount == 1 {
						return &claude.LLMResponse{
							Message: "I'll help you schedule that meeting...",
							Progress: &claude.ProgressInfo{
								NeedsContinuation:  true,
								Status:             "searching",
								Message:            "Looking up John's availability",
								EstimatedRemaining: 1,
								NeedsValidation:    false,
							},
						}, nil
					}
					// Continuation response
					return &claude.LLMResponse{
						Message: "I've scheduled the meeting with John for next Tuesday at 2 PM.",
						Progress: &claude.ProgressInfo{
							NeedsContinuation: false,
							Status:            "complete",
							NeedsValidation:   false,
						},
					}, nil
				}
			},
			expectValidation: false, // Continuation flow skips validation
			verifyFn: func(t *testing.T, resp *claude.LLMResponse, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil || resp.Message != "I've scheduled the meeting with John for next Tuesday at 2 PM." {
					t.Errorf("unexpected response: %v", resp)
				}
			},
		},
		{
			name:    "complex query without progress info triggers validation",
			request: "Schedule a meeting with John next week",
			setupLLM: func(llm *mockLLM) {
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{
						Message: "I'll help you schedule that meeting...",
						// No Progress info, so validation is triggered
					}, nil
				}
			},
			expectValidation: true,
			verifyFn: func(t *testing.T, _ *claude.LLMResponse, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// Response should come from validation/retry
			},
		},
		{
			name:    "missing progress info triggers validation",
			request: "Find emails from yesterday",
			setupLLM: func(llm *mockLLM) {
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{
						Message:  "Let me search for those emails...",
						Progress: nil, // No progress info
					}, nil
				}
			},
			expectValidation: true,
			verifyFn: func(t *testing.T, _ *claude.LLMResponse, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			},
		},
		{
			name:    "chat query completes quickly",
			request: "How are you today?",
			setupLLM: func(llm *mockLLM) {
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{
						Message: "I'm doing well, thank you for asking!",
						Progress: &claude.ProgressInfo{
							NeedsContinuation:  false,
							Status:             "complete",
							Message:            "Chat response",
							EstimatedRemaining: 0,
							NeedsValidation:    false,
						},
					}, nil
				}
			},
			expectValidation: false,
			verifyFn: func(t *testing.T, resp *claude.LLMResponse, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp.Message != "I'm doing well, thank you for asking!" {
					t.Errorf("unexpected response: %s", resp.Message)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track if validation was called
			validationCalled := false

			llm := &mockLLM{}
			if tt.setupLLM != nil {
				tt.setupLLM(llm)
			}

			messenger := &mockMessenger{}
			sessionMgr := newMockSessionManager()
			strategy := &mockValidationStrategy{
				result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
			}

			// Wrap validation strategy to track calls
			wrappedStrategy := &trackingValidationStrategy{
				base:     strategy,
				onCalled: func() { validationCalled = true },
			}

			handler, err := agent.NewHandler(llm,
				agent.WithValidationStrategy(wrappedStrategy),
				agent.WithMessenger(messenger),
				agent.WithSessionManager(sessionMgr),
				agent.WithAsyncValidatorDelay(0), // Set to 0 for immediate validation in tests
			)
			if err != nil {
				t.Fatalf("unexpected error creating handler: %v", err)
			}

			// Test Query method
			resp, err := handler.Query(ctx, tt.request, "test-session")

			// Verify validation was called as expected
			if tt.expectValidation && !validationCalled {
				t.Error("expected validation to be called but it wasn't")
			}
			if !tt.expectValidation && validationCalled {
				t.Error("expected validation to be skipped but it was called")
			}

			// Run custom verification
			if tt.verifyFn != nil {
				tt.verifyFn(t, &resp, err)
			}
		})
	}
}

// TestProcessWithSmartInitialResponse tests Process method with smart initial response.
func TestProcessWithSmartInitialResponse(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		msg              signal.IncomingMessage
		setupLLM         func(*mockLLM)
		expectValidation bool
		verifySent       func(*testing.T, *mockMessenger)
	}{
		{
			name: "simple message skips validation",
			msg: signal.IncomingMessage{
				From: "+1234567890",
				Text: "What's 2+2?",
			},
			setupLLM: func(llm *mockLLM) {
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{
						Message: "2+2 equals 4.",
						Progress: &claude.ProgressInfo{
							NeedsContinuation:  false,
							Status:             "complete",
							Message:            "Math calculation complete",
							EstimatedRemaining: 0,
							NeedsValidation:    false,
						},
					}, nil
				}
			},
			expectValidation: false,
			verifySent: func(t *testing.T, messenger *mockMessenger) {
				t.Helper()
				if len(messenger.sentMessages) != 1 {
					t.Errorf("expected 1 sent message, got %d", len(messenger.sentMessages))
					return
				}
				if messenger.sentMessages[0].message != "2+2 equals 4." {
					t.Errorf("unexpected message: %s", messenger.sentMessages[0].message)
				}
			},
		},
		{
			name: "complex message with needs_continuation skips validation",
			msg: signal.IncomingMessage{
				From: "+1234567890",
				Text: "Book a flight to New York next Friday",
			},
			setupLLM: func(llm *mockLLM) {
				callCount := 0
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					callCount++
					if callCount == 1 {
						// Initial response needs continuation
						return &claude.LLMResponse{
							Message: "I'll help you book that flight...",
							Progress: &claude.ProgressInfo{
								NeedsContinuation:  true,
								Status:             "searching",
								Message:            "Checking flight availability",
								EstimatedRemaining: 1,
								NeedsValidation:    false,
							},
						}, nil
					}
					// Continuation response
					return &claude.LLMResponse{
						Message: "I found several flights to New York next Friday. Here are the options...",
						Progress: &claude.ProgressInfo{
							NeedsContinuation: false,
							Status:            "complete",
							NeedsValidation:   false,
						},
					}, nil
				}
			},
			expectValidation: false, // Continuation flow doesn't trigger validation
			verifySent: func(t *testing.T, messenger *mockMessenger) {
				t.Helper()
				if len(messenger.sentMessages) != 1 {
					t.Errorf("expected 1 sent message, got %d", len(messenger.sentMessages))
				}
			},
		},
		{
			name: "complex message without progress info triggers validation",
			msg: signal.IncomingMessage{
				From: "+1234567890",
				Text: "Book a flight to New York next Friday",
			},
			setupLLM: func(llm *mockLLM) {
				llm.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
					return &claude.LLMResponse{
						Message: "I'll help you book that flight...",
						// No Progress info, so validation is triggered
					}, nil
				}
			},
			expectValidation: true,
			verifySent: func(t *testing.T, messenger *mockMessenger) {
				t.Helper()
				if len(messenger.sentMessages) != 1 {
					t.Errorf("expected 1 sent message, got %d", len(messenger.sentMessages))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track if validation was called using atomic operations
			var validationCalled atomic.Bool

			llm := &mockLLM{}
			if tt.setupLLM != nil {
				tt.setupLLM(llm)
			}

			messenger := &mockMessenger{}
			sessionMgr := newMockSessionManager()
			strategy := &mockValidationStrategy{
				result: agent.ValidationResult{Status: agent.ValidationStatusSuccess},
			}

			// Wrap validation strategy to track calls
			wrappedStrategy := &trackingValidationStrategy{
				base:     strategy,
				onCalled: func() { validationCalled.Store(true) },
			}

			handler, err := agent.NewHandler(llm,
				agent.WithValidationStrategy(wrappedStrategy),
				agent.WithMessenger(messenger),
				agent.WithSessionManager(sessionMgr),
				agent.WithAsyncValidatorDelay(0), // Set to 0 for immediate validation in tests
			)
			if err != nil {
				t.Fatalf("unexpected error creating handler: %v", err)
			}

			// Test Process method
			err = handler.Process(ctx, tt.msg)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// For async validation, give it time to run
			if tt.expectValidation {
				time.Sleep(100 * time.Millisecond)
			}

			// Verify validation was called as expected
			if tt.expectValidation && !validationCalled.Load() {
				t.Error("expected validation to be called but it wasn't")
			}
			if !tt.expectValidation && validationCalled.Load() {
				t.Error("expected validation to be skipped but it was called")
			}

			// Verify sent messages
			if tt.verifySent != nil {
				tt.verifySent(t, messenger)
			}
		})
	}
}

// trackingValidationStrategy wraps a ValidationStrategy to track when it's called.
type trackingValidationStrategy struct {
	base     agent.ValidationStrategy
	onCalled func()
}

func (t *trackingValidationStrategy) Validate(
	ctx context.Context,
	originalRequest, response, sessionID string,
	llm claude.LLM,
) agent.ValidationResult {
	if t.onCalled != nil {
		t.onCalled()
	}
	return t.base.Validate(ctx, originalRequest, response, sessionID, llm)
}

func (t *trackingValidationStrategy) ShouldRetry(result agent.ValidationResult) bool {
	return t.base.ShouldRetry(result)
}

func (t *trackingValidationStrategy) GenerateRecovery(
	ctx context.Context,
	originalRequest, response, sessionID string,
	result agent.ValidationResult,
	llm claude.LLM,
) string {
	return t.base.GenerateRecovery(ctx, originalRequest, response, sessionID, result, llm)
}

// TestContinueWithProgress tests the continueWithProgress method.
func TestContinueWithProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		initialResponse       *claude.LLMResponse
		continuationResponses []*claude.LLMResponse
		expectedMessage       string
		expectedContinuations int
		expectError           bool
		contextTimeout        bool
	}{
		{
			name: "single continuation needed",
			initialResponse: &claude.LLMResponse{
				Message: "Starting processing...",
				Progress: &claude.ProgressInfo{
					NeedsContinuation: true,
					Status:            "searching",
					Message:           "Searching for information",
					NeedsValidation:   false,
				},
			},
			continuationResponses: []*claude.LLMResponse{
				{
					Message: "Found the information",
					Progress: &claude.ProgressInfo{
						NeedsContinuation: false,
						Status:            "complete",
						NeedsValidation:   false,
					},
				},
			},
			expectedMessage:       "Found the information",
			expectedContinuations: 1,
		},
		{
			name: "multiple continuations needed",
			initialResponse: &claude.LLMResponse{
				Message: "Starting complex task...",
				Progress: &claude.ProgressInfo{
					NeedsContinuation:  true,
					Status:             "step1",
					EstimatedRemaining: 3,
					NeedsValidation:    false,
				},
			},
			continuationResponses: []*claude.LLMResponse{
				{
					Message: "Step 1 complete",
					Progress: &claude.ProgressInfo{
						NeedsContinuation:  true,
						Status:             "step2",
						EstimatedRemaining: 2,
						NeedsValidation:    false,
					},
				},
				{
					Message: "Step 2 complete",
					Progress: &claude.ProgressInfo{
						NeedsContinuation:  true,
						Status:             "step3",
						EstimatedRemaining: 1,
						NeedsValidation:    false,
					},
				},
				{
					Message: "All steps complete",
					Progress: &claude.ProgressInfo{
						NeedsContinuation: false,
						Status:            "complete",
						NeedsValidation:   false,
					},
				},
			},
			expectedMessage:       "All steps complete",
			expectedContinuations: 3,
		},
		{
			name: "max continuations reached",
			initialResponse: &claude.LLMResponse{
				Message: "Starting endless task...",
				Progress: &claude.ProgressInfo{
					NeedsContinuation: true,
					Status:            "processing",
					NeedsValidation:   false,
				},
			},
			continuationResponses: func() []*claude.LLMResponse {
				var responses []*claude.LLMResponse
				for i := 1; i <= 5; i++ {
					responses = append(responses, &claude.LLMResponse{
						Message: fmt.Sprintf("Still processing %d", i),
						Progress: &claude.ProgressInfo{
							NeedsContinuation: true,
							Status:            "processing",
						},
					})
				}
				return responses
			}(),
			expectedMessage:       "Still processing 5",
			expectedContinuations: 5, // Max is 5
		},
		{
			name: "no continuation needed",
			initialResponse: &claude.LLMResponse{
				Message: "Already complete",
				Progress: &claude.ProgressInfo{
					NeedsContinuation: false,
					Status:            "complete",
					NeedsValidation:   false,
				},
			},
			continuationResponses: []*claude.LLMResponse{},
			expectedMessage:       "Already complete",
			expectedContinuations: 0,
		},
		{
			name: "continuation error recovery",
			initialResponse: &claude.LLMResponse{
				Message: "Starting with error...",
				Progress: &claude.ProgressInfo{
					NeedsContinuation: true,
					Status:            "processing",
					NeedsValidation:   false,
				},
			},
			continuationResponses: []*claude.LLMResponse{
				nil, // This will cause an error
			},
			expectedMessage:       "Starting with error...", // Should return initial response on error
			expectedContinuations: 1,
		},
		{
			name: "context cancellation",
			initialResponse: &claude.LLMResponse{
				Message: "Starting context test...",
				Progress: &claude.ProgressInfo{
					NeedsContinuation: true,
					Status:            "processing",
					NeedsValidation:   false,
				},
			},
			continuationResponses: []*claude.LLMResponse{},
			expectedMessage:       "The request was canceled", // Error recovery will generate a cancellation message
			contextTimeout:        true,
			expectError:           true,
			expectedContinuations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set up mock LLM
			callCount := 0
			mockLLM := &mockLLM{
				queryFunc: func(_ context.Context, prompt, _ string) (*claude.LLMResponse, error) {
					// Check if this is a continuation prompt
					if contains(prompt, "continue") {
						if callCount < len(tt.continuationResponses) {
							resp := tt.continuationResponses[callCount]
							callCount++
							if resp == nil {
								return nil, fmt.Errorf("mock error")
							}
							return resp, nil
						}
					}
					return &claude.LLMResponse{
						Message: "Unexpected call",
					}, nil
				},
			}

			// Create handler with the private continueWithProgress method
			// We'll need to test through the public Process or Query methods
			handler, err := agent.NewHandler(
				mockLLM,
				agent.WithMessenger(&mockMessenger{}),
				agent.WithSessionManager(&mockSessionManager{
					sessions: make(map[string]string),
					history:  make(map[string][]conversation.Message),
				}),
				agent.WithValidationStrategy(&mockValidationStrategy{}),
				agent.WithLogger(slog.Default()),
			)
			if err != nil {
				t.Fatalf("Failed to create handler: %v", err)
			}

			// Create context
			ctx := context.Background()
			if tt.contextTimeout {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // Cancel immediately
			}

			// Test through Query method which uses continueWithProgress
			// Set up the mock to return the initial response first
			queryCount := 0
			mockLLM.queryFunc = func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				if queryCount == 0 {
					queryCount++
					return tt.initialResponse, nil
				}
				// Subsequent calls are continuations
				if callCount < len(tt.continuationResponses) {
					resp := tt.continuationResponses[callCount]
					callCount++
					if resp == nil {
						return nil, fmt.Errorf("mock error")
					}
					return resp, nil
				}
				return &claude.LLMResponse{
					Message: "Unexpected call",
				}, nil
			}

			// Call Query which will trigger continueWithProgress if needed
			response, err := handler.Query(ctx, "Test message", "test-session")

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil && !tt.expectError {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			if !contains(response.Message, tt.expectedMessage) {
				t.Errorf("Expected message to contain %q, got %q", tt.expectedMessage, response.Message)
			}

			if callCount != tt.expectedContinuations {
				t.Errorf("Expected %d continuations, got %d", tt.expectedContinuations, callCount)
			}
		})
	}
}
