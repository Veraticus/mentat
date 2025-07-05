package agent_test

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCorrectionDelay is a short delay for testing to make tests run faster.
const testCorrectionDelay = 100 * time.Millisecond

// asyncValidationStrategy extends mockValidationStrategy with tracking for async validation tests.
type asyncValidationStrategy struct {
	mu              sync.Mutex
	validateCalls   []validateCall
	recoveryCalls   []recoveryCall
	validateFunc    func(ctx context.Context, request, response, sessionID string, llm claude.LLM) agent.ValidationResult
	recoveryFunc    func(ctx context.Context, request, response, sessionID string, result agent.ValidationResult, llm claude.LLM) string
	shouldRetryFunc func(result agent.ValidationResult) bool
}

type validateCall struct {
	request   string
	response  string
	sessionID string
}

type recoveryCall struct {
	request   string
	response  string
	sessionID string
	result    agent.ValidationResult
}

func (m *asyncValidationStrategy) Validate(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) agent.ValidationResult {
	m.mu.Lock()
	m.validateCalls = append(m.validateCalls, validateCall{request: request, response: response, sessionID: sessionID})
	m.mu.Unlock()

	if m.validateFunc != nil {
		return m.validateFunc(ctx, request, response, sessionID, llm)
	}
	return agent.ValidationResult{Status: agent.ValidationStatusSuccess, Confidence: 1.0}
}

func (m *asyncValidationStrategy) ShouldRetry(result agent.ValidationResult) bool {
	if m.shouldRetryFunc != nil {
		return m.shouldRetryFunc(result)
	}
	return false
}

func (m *asyncValidationStrategy) GenerateRecovery(
	ctx context.Context,
	request, response, sessionID string,
	result agent.ValidationResult,
	llm claude.LLM,
) string {
	m.mu.Lock()
	m.recoveryCalls = append(m.recoveryCalls, recoveryCall{
		request:   request,
		response:  response,
		sessionID: sessionID,
		result:    result,
	})
	m.mu.Unlock()

	if m.recoveryFunc != nil {
		return m.recoveryFunc(ctx, request, response, sessionID, result, llm)
	}
	return ""
}

func (m *asyncValidationStrategy) getValidateCalls() []validateCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]validateCall, len(m.validateCalls))
	copy(calls, m.validateCalls)
	return calls
}

func (m *asyncValidationStrategy) getRecoveryCalls() []recoveryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]recoveryCall, len(m.recoveryCalls))
	copy(calls, m.recoveryCalls)
	return calls
}

// asyncSendCall represents a single send operation for async tests.
type asyncSendCall struct {
	ctx       context.Context
	recipient string
	message   string
}

// asyncMockMessenger implements the Messenger interface for async validation tests.
type asyncMockMessenger struct {
	mu        sync.Mutex
	sendCalls []asyncSendCall
	sendFunc  func(ctx context.Context, to, message string) error
}

func (m *asyncMockMessenger) Send(ctx context.Context, to, message string) error {
	m.mu.Lock()
	m.sendCalls = append(m.sendCalls, asyncSendCall{ctx: ctx, recipient: to, message: message})
	m.mu.Unlock()

	if m.sendFunc != nil {
		return m.sendFunc(ctx, to, message)
	}
	return nil
}

func (m *asyncMockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	ch := make(chan signal.IncomingMessage)
	return ch, nil
}

func (m *asyncMockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	return nil
}

func (m *asyncMockMessenger) GetSendCalls() []asyncSendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]asyncSendCall, len(m.sendCalls))
	copy(calls, m.sendCalls)
	return calls
}

func TestAsyncValidator_StartValidation(t *testing.T) {
	tests := []struct {
		name             string
		validationResult agent.ValidationResult
		recoveryMessage  string
		expectCorrection bool
		expectValidation bool
	}{
		{
			name: "successful validation with no correction needed",
			validationResult: agent.ValidationResult{
				Status:     agent.ValidationStatusSuccess,
				Confidence: 1.0,
			},
			expectValidation: true,
			expectCorrection: false,
		},
		{
			name: "failed validation triggers correction",
			validationResult: agent.ValidationResult{
				Status:     agent.ValidationStatusFailed,
				Confidence: 0.2,
				Issues:     []string{"Missing required information"},
			},
			recoveryMessage:  "Let me help you with that...",
			expectValidation: true,
			expectCorrection: true,
		},
		{
			name: "partial validation triggers correction",
			validationResult: agent.ValidationResult{
				Status:     agent.ValidationStatusPartial,
				Confidence: 0.6,
				Issues:     []string{"Some information missing"},
			},
			recoveryMessage:  "I found some additional info...",
			expectValidation: true,
			expectCorrection: true,
		},
		{
			name: "incomplete search triggers correction",
			validationResult: agent.ValidationResult{
				Status: agent.ValidationStatusIncompleteSearch,
				Metadata: map[string]string{
					"expectedTools": "memory,calendar",
				},
			},
			recoveryMessage:  "Let me check those resources...",
			expectValidation: true,
			expectCorrection: true,
		},
		{
			name: "unclear validation does not trigger correction",
			validationResult: agent.ValidationResult{
				Status:     agent.ValidationStatusUnclear,
				Confidence: 0.5,
			},
			expectValidation: true,
			expectCorrection: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			messenger := &asyncMockMessenger{
				sendFunc: func(_ context.Context, _, _ string) error {
					return nil
				},
			}

			strategy := &asyncValidationStrategy{
				validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
					return tt.validationResult
				},
				recoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
					return tt.recoveryMessage
				},
			}

			logger := slog.Default()
			av := agent.NewAsyncValidator(messenger, strategy, logger)
			av.SetCorrectionDelay(testCorrectionDelay)

			// Execute
			response := &claude.LLMResponse{
				Message: "Here's your answer",
			}

			av.StartValidation("+1234567890", "What's the weather?", response, "session-123")

			// Wait for validation delay + async operation to complete
			time.Sleep(testCorrectionDelay + 100*time.Millisecond)

			// Verify validation was called
			if tt.expectValidation {
				validateCalls := strategy.getValidateCalls()
				require.Len(t, validateCalls, 1)
				assert.Equal(t, "What's the weather?", validateCalls[0].request)
				assert.Equal(t, "Here's your answer", validateCalls[0].response)
				assert.Equal(t, "session-123", validateCalls[0].sessionID)
			}

			// Verify correction was sent if expected
			if tt.expectCorrection {
				// Wait for correction delay
				time.Sleep(testCorrectionDelay + 100*time.Millisecond)

				recoveryCalls := strategy.getRecoveryCalls()
				require.Len(t, recoveryCalls, 1)
				assert.Equal(t, tt.validationResult.Status, recoveryCalls[0].result.Status)

				sendCalls := messenger.GetSendCalls()
				require.Len(t, sendCalls, 1)
				assert.Equal(t, "+1234567890", sendCalls[0].recipient)
				assert.Equal(t, tt.recoveryMessage, sendCalls[0].message)
			} else {
				// Ensure no correction was sent
				time.Sleep(testCorrectionDelay + 100*time.Millisecond)
				sendCalls := messenger.GetSendCalls()
				assert.Empty(t, sendCalls)
			}

			// Cleanup
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			require.NoError(t, av.Shutdown(ctx))
		})
	}
}

func TestAsyncValidator_MultipleValidations(t *testing.T) {
	// Setup
	messenger := &asyncMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	validationCount := 0
	var mu sync.Mutex

	strategy := &asyncValidationStrategy{
		validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			mu.Lock()
			validationCount++
			mu.Unlock()

			// Simulate some work
			time.Sleep(50 * time.Millisecond)

			return agent.ValidationResult{
				Status:     agent.ValidationStatusSuccess,
				Confidence: 1.0,
			}
		},
	}

	logger := slog.Default()
	av := agent.NewAsyncValidator(messenger, strategy, logger)
	av.SetCorrectionDelay(testCorrectionDelay)

	// Execute multiple validations concurrently
	numValidations := 5
	for i := range numValidations {
		response := &claude.LLMResponse{
			Message: fmt.Sprintf("Response %d", i),
		}
		sessionID := fmt.Sprintf("session-%d", i)
		av.StartValidation("+1234567890", "Test request", response, sessionID)
	}

	// Check active count
	time.Sleep(10 * time.Millisecond) // Let goroutines start
	activeCount := av.ActiveCount()
	assert.GreaterOrEqual(t, activeCount, 1)
	assert.LessOrEqual(t, activeCount, numValidations)

	// Wait for all to complete
	time.Sleep(200 * time.Millisecond)

	// Verify all validations completed
	mu.Lock()
	finalCount := validationCount
	mu.Unlock()
	assert.Equal(t, numValidations, finalCount)

	// Verify no active tasks remain
	assert.Equal(t, 0, av.ActiveCount())

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	require.NoError(t, av.Shutdown(ctx))
}

func TestAsyncValidator_Shutdown(t *testing.T) {
	tests := []struct {
		name            string
		shutdownTimeout time.Duration
		validationDelay time.Duration
		expectError     bool
	}{
		{
			name:            "graceful shutdown completes quickly",
			shutdownTimeout: 1 * time.Second,
			validationDelay: 10 * time.Millisecond,
			expectError:     false,
		},
		{
			name:            "shutdown times out with slow validations",
			shutdownTimeout: 50 * time.Millisecond,
			validationDelay: 200 * time.Millisecond,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			messenger := &asyncMockMessenger{
				sendFunc: func(_ context.Context, _, _ string) error {
					return nil
				},
			}

			validationStarted := make(chan struct{})
			validationBlocked := make(chan struct{})

			strategy := &asyncValidationStrategy{
				validateFunc: func(ctx context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
					close(validationStarted)

					// Block for the specified delay
					// For the timeout test, we need to ensure we don't return early
					timer := time.NewTimer(tt.validationDelay)
					defer timer.Stop()

					select {
					case <-timer.C:
						// Normal completion after delay
					case <-ctx.Done():
						// Context canceled, but we still need to wait a bit for the timeout test
						if tt.expectError {
							// For timeout test, ensure we block long enough
							time.Sleep(tt.shutdownTimeout + 10*time.Millisecond)
						}
					}

					close(validationBlocked)
					return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
				},
			}

			logger := slog.Default()
			av := agent.NewAsyncValidator(messenger, strategy, logger)
			av.SetCorrectionDelay(testCorrectionDelay)

			// Start a validation
			response := &claude.LLMResponse{Message: "Test"}
			av.StartValidation("+1234567890", "Test", response, "session-1")

			// Wait for validation to start
			<-validationStarted

			// Attempt shutdown
			ctx, cancel := context.WithTimeout(context.Background(), tt.shutdownTimeout)
			defer cancel()

			err := av.Shutdown(ctx)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "shutdown timeout")
			} else {
				require.NoError(t, err)
				// Verify validation completed
				<-validationBlocked
			}
		})
	}
}

func TestAsyncValidator_NoGoroutineLeaks(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	initialGoroutines := runtime.NumGoroutine()

	// Setup
	messenger := &asyncMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	strategy := &asyncValidationStrategy{
		validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			return agent.ValidationResult{
				Status:     agent.ValidationStatusFailed,
				Confidence: 0.2,
			}
		},
		recoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
			return "Correction message"
		},
	}

	logger := slog.Default()
	av := agent.NewAsyncValidator(messenger, strategy, logger)
	av.SetCorrectionDelay(testCorrectionDelay)

	// Run multiple validations
	for i := range 10 {
		response := &claude.LLMResponse{
			Message: fmt.Sprintf("Response %d", i),
		}
		av.StartValidation("+1234567890", "Test", response, fmt.Sprintf("session-%d", i))
	}

	// Wait for validations to complete
	time.Sleep(testCorrectionDelay + 500*time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, av.Shutdown(ctx))

	// Give time for goroutines to clean up
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+1,
		"Goroutine leak detected: initial=%d, final=%d", initialGoroutines, finalGoroutines)
}

func TestAsyncValidator_GetActiveTasks(t *testing.T) {
	// Setup
	messenger := &asyncMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	blockValidation := make(chan struct{})
	strategy := &asyncValidationStrategy{
		validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			<-blockValidation // Block until signaled
			return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
		},
	}

	logger := slog.Default()
	av := agent.NewAsyncValidator(messenger, strategy, logger)
	av.SetCorrectionDelay(testCorrectionDelay)

	// Start multiple validations
	for i := range 3 {
		response := &claude.LLMResponse{
			Message: fmt.Sprintf("Response %d", i),
		}
		av.StartValidation("+1234567890", fmt.Sprintf("Request %d", i), response, fmt.Sprintf("session-%d", i))
	}

	// Let validations start
	time.Sleep(50 * time.Millisecond)

	// Get active tasks
	tasks := av.GetActiveTasks()
	assert.Len(t, tasks, 3)

	// Verify task details - tasks may be returned in any order since they're stored in a map
	foundRequests := make(map[string]bool)
	foundSessions := make(map[string]bool)

	for _, task := range tasks {
		assert.Equal(t, "+1234567890", task.To)
		assert.NotZero(t, task.StartTime)
		assert.Contains(t, task.ID, task.SessionID)

		// Verify the request and session are from the expected set
		foundRequests[task.OriginalRequest] = true
		foundSessions[task.SessionID] = true
	}

	// Verify all expected requests and sessions were found
	for i := range 3 {
		expectedRequest := fmt.Sprintf("Request %d", i)
		expectedSession := fmt.Sprintf("session-%d", i)
		assert.True(t, foundRequests[expectedRequest], "Missing request: %s", expectedRequest)
		assert.True(t, foundSessions[expectedSession], "Missing session: %s", expectedSession)
	}

	// Unblock validations
	close(blockValidation)

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	require.NoError(t, av.Shutdown(ctx))
}

func TestAsyncValidator_ValidationContextCancellation(t *testing.T) {
	// Setup
	messenger := &asyncMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	validationStarted := make(chan struct{})
	validationCanceled := make(chan struct{})
	strategy := &asyncValidationStrategy{
		validateFunc: func(ctx context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			close(validationStarted)
			// Wait for context cancellation
			<-ctx.Done()
			close(validationCanceled)
			return agent.ValidationResult{Status: agent.ValidationStatusFailed}
		},
		recoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
			return "This should not be sent"
		},
	}

	logger := slog.Default()
	av := agent.NewAsyncValidator(messenger, strategy, logger)
	// Set a very short delay so validation actually starts
	av.SetCorrectionDelay(10 * time.Millisecond)

	// Start validation
	response := &claude.LLMResponse{Message: "Test"}
	av.StartValidation("+1234567890", "Test", response, "session-1")

	// Wait for validation to start or timeout
	select {
	case <-validationStarted:
		// Validation started, now shutdown to cancel it
	case <-time.After(200 * time.Millisecond):
		// Validation was canceled during delay, which is also fine
	}

	// Shutdown to cancel validation
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := av.Shutdown(ctx)
	require.NoError(t, err)

	// Verify no correction was sent
	sendCalls := messenger.GetSendCalls()
	assert.Empty(t, sendCalls, "No correction should be sent when validation is canceled")
}

func TestAsyncValidator_MessengerError(t *testing.T) {
	// Setup
	messenger := &asyncMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("network error")
		},
	}

	strategy := &asyncValidationStrategy{
		validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			return agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
			}
		},
		recoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
			return "Correction message"
		},
	}

	logger := slog.Default()
	av := agent.NewAsyncValidator(messenger, strategy, logger)
	av.SetCorrectionDelay(testCorrectionDelay)

	// Start validation
	response := &claude.LLMResponse{Message: "Test"}
	av.StartValidation("+1234567890", "Test", response, "session-1")

	// Wait for validation and correction attempt
	time.Sleep(testCorrectionDelay + 200*time.Millisecond)

	// Verify send was attempted
	sendCalls := messenger.GetSendCalls()
	require.Len(t, sendCalls, 1)

	// The error should be logged but not crash the validator
	assert.Equal(t, 0, av.ActiveCount())

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	require.NoError(t, av.Shutdown(ctx))
}

func TestAsyncValidator_ValidationDelay(t *testing.T) {
	// Setup
	messenger := &asyncMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	validationStartCh := make(chan time.Time, 1)
	strategy := &asyncValidationStrategy{
		validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			validationStartCh <- time.Now()
			return agent.ValidationResult{
				Status:     agent.ValidationStatusSuccess,
				Confidence: 1.0,
			}
		},
	}

	logger := slog.Default()
	av := agent.NewAsyncValidator(messenger, strategy, logger)

	// Set a specific delay for this test
	testDelay := 50 * time.Millisecond
	av.SetCorrectionDelay(testDelay)

	// Start validation
	response := &claude.LLMResponse{Message: "Test"}
	startTime := time.Now()
	av.StartValidation("+1234567890", "Test", response, "session-1")

	// Wait for validation to complete
	select {
	case validationStartTime := <-validationStartCh:
		// Verify delay was applied
		actualDelay := validationStartTime.Sub(startTime)
		assert.GreaterOrEqual(t, actualDelay, testDelay, "Validation should start after delay")
		assert.Less(t, actualDelay, testDelay+20*time.Millisecond, "Validation should start soon after delay")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Validation did not complete in time")
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	require.NoError(t, av.Shutdown(ctx))
}

func TestAsyncValidator_EmptyRecoveryMessage(t *testing.T) {
	// Setup
	messenger := &asyncMockMessenger{
		sendFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	strategy := &asyncValidationStrategy{
		validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			return agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
			}
		},
		recoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
			return "" // Empty recovery message
		},
	}

	logger := slog.Default()
	av := agent.NewAsyncValidator(messenger, strategy, logger)
	av.SetCorrectionDelay(testCorrectionDelay)

	// Start validation
	response := &claude.LLMResponse{Message: "Test"}
	av.StartValidation("+1234567890", "Test", response, "session-1")

	// Wait for validation
	time.Sleep(testCorrectionDelay + 200*time.Millisecond)

	// Verify no message was sent
	sendCalls := messenger.GetSendCalls()
	assert.Empty(t, sendCalls, "No message should be sent when recovery message is empty")

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	require.NoError(t, av.Shutdown(ctx))
}
