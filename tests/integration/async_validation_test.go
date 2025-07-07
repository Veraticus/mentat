//go:build integration
// +build integration

// Package integration provides integration tests for the mentat system
package integration

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/mocks"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// ErrTimeout is returned when an operation times out
	ErrTimeout = errors.New("operation timed out")
)

// validationEvent tracks when validation events occur
type validationEvent struct {
	eventType string // "launched", "completed", "failed"
	timestamp time.Time
	msgID     string
	result    *agent.ValidationResult
	err       error
}

// asyncValidationTestHarness provides a test environment for async validation
type asyncValidationTestHarness struct {
	// Mocks
	mockLLM       *mocks.MockLLM
	mockMessenger *mocks.MockMessenger

	// Validation tracking
	validationEvents chan validationEvent
	validationMu     sync.Mutex
	validationMap    map[string][]validationEvent // msgID -> events

	// Message tracking
	messagesSent []mocks.SentMessage
	messagesMu   sync.Mutex

	// Timing tracking
	responseTimes   map[string]time.Duration // msgID -> response time
	correctionTimes map[string]time.Duration // msgID -> time from response to correction

	// Cleanup
	cleanupFuncs []func()
	closed       bool
	closedMu     sync.Mutex
	done         chan struct{} // Signal when event collector finishes
}

// newAsyncValidationTestHarness creates a test harness for async validation testing
func newAsyncValidationTestHarness() *asyncValidationTestHarness {
	h := &asyncValidationTestHarness{
		mockLLM:          mocks.NewMockLLM(),
		mockMessenger:    mocks.NewMockMessenger(),
		validationEvents: make(chan validationEvent, 100),
		validationMap:    make(map[string][]validationEvent),
		responseTimes:    make(map[string]time.Duration),
		correctionTimes:  make(map[string]time.Duration),
		messagesSent:     []mocks.SentMessage{},
		cleanupFuncs:     []func(){},
		done:             make(chan struct{}),
	}

	// Start event collector
	ctx, cancel := context.WithCancel(context.Background())
	go h.collectValidationEvents(ctx)
	h.cleanupFuncs = append(h.cleanupFuncs, cancel)

	return h
}

// collectValidationEvents collects validation events for analysis
func (h *asyncValidationTestHarness) collectValidationEvents(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-h.validationEvents:
			if !ok {
				return
			}
			h.validationMu.Lock()
			h.validationMap[event.msgID] = append(h.validationMap[event.msgID], event)
			h.validationMu.Unlock()
		}
	}
}

// recordValidationLaunch records when validation is launched
func (h *asyncValidationTestHarness) recordValidationLaunch(msgID string) {
	h.closedMu.Lock()
	if h.closed {
		h.closedMu.Unlock()
		return
	}
	// Send while holding the lock to prevent race with cleanup
	select {
	case h.validationEvents <- validationEvent{
		eventType: "launched",
		timestamp: time.Now(),
		msgID:     msgID,
	}:
	default:
		// Channel might be full, ignore
	}
	h.closedMu.Unlock()
}

// recordValidationComplete records when validation completes
func (h *asyncValidationTestHarness) recordValidationComplete(msgID string, result *agent.ValidationResult, err error) {
	h.closedMu.Lock()
	if h.closed {
		h.closedMu.Unlock()
		return
	}
	// Send while holding the lock to prevent race with cleanup
	select {
	case h.validationEvents <- validationEvent{
		eventType: "completed",
		timestamp: time.Now(),
		msgID:     msgID,
		result:    result,
		err:       err,
	}:
	default:
		// Channel might be full, ignore
	}
	h.closedMu.Unlock()
}

// getValidationEvents returns all validation events for a message
func (h *asyncValidationTestHarness) getValidationEvents(msgID string) []validationEvent {
	h.validationMu.Lock()
	defer h.validationMu.Unlock()

	events := h.validationMap[msgID]
	result := make([]validationEvent, len(events))
	copy(result, events)
	return result
}

// waitForValidationLaunch waits for validation to be launched
func (h *asyncValidationTestHarness) waitForValidationLaunch(msgID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			events := h.getValidationEvents(msgID)
			for _, e := range events {
				if e.eventType == "launched" {
					return nil
				}
			}
			if time.Now().After(deadline) {
				return ErrTimeout
			}
		}
	}
}

// waitForValidationComplete waits for validation to complete
func (h *asyncValidationTestHarness) waitForValidationComplete(msgID string, timeout time.Duration) (*agent.ValidationResult, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			events := h.getValidationEvents(msgID)
			for _, e := range events {
				if e.eventType == "completed" {
					return e.result, e.err
				}
			}
			if time.Now().After(deadline) {
				return nil, ErrTimeout
			}
		}
	}
}

// assertNoValidation asserts that no validation was launched for a message
func (h *asyncValidationTestHarness) assertNoValidation(t *testing.T, msgID string) {
	// Wait a bit to ensure no validation is launched
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	<-timer.C

	events := h.getValidationEvents(msgID)
	for _, e := range events {
		if e.eventType == "launched" {
			t.Errorf("expected no validation for message %s, but validation was launched", msgID)
		}
	}
}

// waitForMessage waits for a message to be sent
func (h *asyncValidationTestHarness) waitForMessage(timeout time.Duration) (mocks.SentMessage, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.messagesMu.Lock()
			sent := h.mockMessenger.GetSentMessages()
			if len(sent) > len(h.messagesSent) {
				// New message sent
				newMsg := sent[len(h.messagesSent)]
				h.messagesSent = append(h.messagesSent, newMsg)
				h.messagesMu.Unlock()
				return newMsg, nil
			}
			h.messagesMu.Unlock()
			if time.Now().After(deadline) {
				return mocks.SentMessage{}, ErrTimeout
			}
		}
	}
}

// generateMessageID generates a unique message ID
func (h *asyncValidationTestHarness) generateMessageID() string {
	return fmt.Sprintf("test-msg-%d", time.Now().UnixNano())
}

// cleanup runs all cleanup functions
func (h *asyncValidationTestHarness) cleanup() {
	// First cancel contexts to stop goroutines
	for _, fn := range h.cleanupFuncs {
		fn()
	}

	// Wait for validation goroutines to finish their current operations
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	<-timer.C

	// Mark as closed and close channel under lock to prevent races
	h.closedMu.Lock()
	h.closed = true
	close(h.validationEvents)
	h.closedMu.Unlock()

	// Wait for the event collector goroutine to finish
	select {
	case <-h.done:
		// Event collector finished
	case <-time.After(100 * time.Millisecond):
		// Timeout waiting for event collector
	}
}

// createValidationStrategy creates a mock validation strategy that tracks calls
func createValidationStrategy(h *asyncValidationTestHarness) agent.ValidationStrategy {
	return &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status:     agent.ValidationStatusSuccess,
			Confidence: 0.95,
		},
	}
}

// trackingValidationStrategy tracks validation calls
type trackingValidationStrategy struct {
	harness *asyncValidationTestHarness
	result  *agent.ValidationResult
	err     error
	delay   time.Duration
}

func (s *trackingValidationStrategy) Validate(ctx context.Context, request, response, recoveryContext string, llm claude.LLM) agent.ValidationResult {
	// Use recovery context (session ID) as message ID for tracking
	msgID := recoveryContext
	if msgID == "" {
		msgID = "test-message-id"
	}

	s.harness.recordValidationLaunch(msgID)

	// Simulate validation work
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{ctx.Err().Error()},
			}
		}
	}

	if s.err != nil {
		// For timeout errors, return a special result that won't trigger correction
		if errors.Is(s.err, context.DeadlineExceeded) || errors.Is(s.err, context.Canceled) {
			result := agent.ValidationResult{
				Status: agent.ValidationStatusSuccess, // Return success to avoid correction
				Issues: []string{"Validation timed out"},
			}
			s.harness.recordValidationComplete(msgID, &result, s.err)
			return result
		}
		result := agent.ValidationResult{
			Status: agent.ValidationStatusFailed,
			Issues: []string{s.err.Error()},
		}
		s.harness.recordValidationComplete(msgID, &result, s.err)
		return result
	}

	s.harness.recordValidationComplete(msgID, s.result, nil)
	return *s.result
}

func (s *trackingValidationStrategy) ShouldRetry(result agent.ValidationResult) bool {
	return result.Status == agent.ValidationStatusIncompleteSearch
}

func (s *trackingValidationStrategy) GenerateRecovery(ctx context.Context, request, response, reason string, result agent.ValidationResult, llm claude.LLM) string {
	return "Oops! I need to correct my previous response. " + strings.Join(result.Issues, " ")
}

// Test scenarios

func TestAsyncValidation_SimpleQueries(t *testing.T) {
	tests := []struct {
		name             string
		userMessage      string
		llmResponse      claude.LLMResponse
		expectValidation bool
		description      string
	}{
		{
			name:        "greeting",
			userMessage: "Hi!",
			llmResponse: claude.LLMResponse{
				Message: "Hello! How can I help you today?",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   false,
					NeedsContinuation: false,
				},
			},
			expectValidation: false,
			description:      "Simple greeting should not trigger validation",
		},
		{
			name:        "simple_question",
			userMessage: "What time is it?",
			llmResponse: claude.LLMResponse{
				Message: "I'll check the current time for you.",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   false,
					NeedsContinuation: false,
				},
			},
			expectValidation: false,
			description:      "Simple questions should complete without validation",
		},
		{
			name:        "complex_with_tools",
			userMessage: "Check my calendar and email",
			llmResponse: claude.LLMResponse{
				Message: "I'll check your calendar and email for you.",
				Progress: &claude.ProgressInfo{
					NeedsValidation:    true,
					NeedsContinuation:  true,
					EstimatedRemaining: 1,
				},
			},
			expectValidation: true,
			description:      "Complex queries with tools should trigger validation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAsyncValidationTestHarness()
			defer h.cleanup() // Ensure cleanup

			// Create handler with validation strategy
			validationStrategy := createValidationStrategy(h)
			enhancer := mocks.NewMockIntentEnhancer()
			sessionManager := mocks.NewMockSessionManager()

			// Get the session ID that will be used for this test
			sessionID := sessionManager.GetOrCreateSession("Test User")

			// Configure LLM to return specific response
			h.mockLLM.SetSessionResponse(sessionID, &tc.llmResponse)
			h.mockLLM.SetSessionResponse("", &tc.llmResponse) // Fallback

			// For continuation scenarios, ensure they complete
			if tc.llmResponse.Progress != nil && tc.llmResponse.Progress.NeedsContinuation {
				// Set up continuation to complete
				continuationResp := claude.LLMResponse{
					Message: "Completed the task.",
					Progress: &claude.ProgressInfo{
						NeedsValidation:   tc.expectValidation,
						NeedsContinuation: false,
					},
				}
				h.mockLLM.QueryFunc = func(ctx context.Context, prompt, sid string) (*claude.LLMResponse, error) {
					if strings.Contains(prompt, "continue") || strings.Contains(prompt, "Please continue") {
						return &continuationResp, nil
					}
					return &tc.llmResponse, nil
				}
			}

			handler, err := agent.NewHandler(h.mockLLM,
				agent.WithValidationStrategy(validationStrategy),
				agent.WithIntentEnhancer(enhancer),
				agent.WithSessionManager(sessionManager),
				agent.WithMessenger(h.mockMessenger),
			)
			require.NoError(t, err)

			// Process message
			ctx := context.Background()
			msg := signal.IncomingMessage{
				Timestamp:  time.Now(),
				From:       "Test User",
				FromNumber: "+15551234567",
				Text:       tc.userMessage,
			}

			startTime := time.Now()

			// Process the message
			err = handler.Process(ctx, msg)
			require.NoError(t, err)

			// Wait for response to be sent
			_, err = h.waitForMessage(1 * time.Second)
			require.NoError(t, err)

			// Measure response time
			responseTime := time.Since(startTime)
			t.Logf("%s: Response time: %v", tc.name, responseTime)

			// Verify response time meets target
			assert.Less(t, responseTime, 200*time.Millisecond,
				"%s: Response should be delivered within 200ms", tc.description)

			// Check validation behavior
			if tc.expectValidation {
				// Should launch validation
				// Use waitForValidationLaunch with the session ID
				err = h.waitForValidationLaunch(sessionID, 200*time.Millisecond)
				if err != nil {
					// Log the validation map to debug
					h.validationMu.Lock()
					t.Logf("Validation map contents: %+v", h.validationMap)
					t.Logf("Looking for session ID: %s", sessionID)
					h.validationMu.Unlock()
				}
				require.NoError(t, err, "%s: Expected validation to be launched", tc.description)

				// Wait for validation to complete
				result, err := h.waitForValidationComplete(sessionID, 1*time.Second)
				require.NoError(t, err, "%s: Validation should complete", tc.description)
				assert.Equal(t, agent.ValidationStatusSuccess, result.Status,
					"%s: Validation should succeed", tc.description)
			} else {
				// Should NOT launch validation
				h.assertNoValidation(t, sessionID)
			}

			// Wait for async operations to finish before test cleanup
			timer := time.NewTimer(50 * time.Millisecond)
			defer timer.Stop()
			<-timer.C
		})
	}
}

func TestAsyncValidation_TimingVerification(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup() // Ensure cleanup

	// Configure validation to return a correction needed result
	validationStrategy := &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status:     agent.ValidationStatusFailed,
			Confidence: 0.8,
			Issues:     []string{"Missing information about user's location"},
		},
		delay: 50 * time.Millisecond, // Simulate validation work
	}

	// Configure LLM responses
	llmResp := claude.LLMResponse{
		Message: "I'll check the weather for you.",
		Progress: &claude.ProgressInfo{
			NeedsValidation:   true,
			NeedsContinuation: true,
		},
	}
	h.mockLLM.SetSessionResponse("", &llmResp)

	// Configure mock to generate recovery message
	h.mockLLM.QueryFunc = func(ctx context.Context, prompt, sid string) (*claude.LLMResponse, error) {
		// For continuation prompts
		if strings.Contains(prompt, "continue") || strings.Contains(prompt, "Please continue") {
			return &claude.LLMResponse{
				Message: "Task completed.",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   true,
					NeedsContinuation: false,
				},
			}, nil
		}
		// For initial query
		return &llmResp, nil
	}

	// Create handler
	enhancer := mocks.NewMockIntentEnhancer()
	sessionManager := mocks.NewMockSessionManager()

	// Get the session ID that will be used
	sessionID := sessionManager.GetOrCreateSession("Test User")

	handler, err := agent.NewHandler(h.mockLLM,
		agent.WithValidationStrategy(validationStrategy),
		agent.WithIntentEnhancer(enhancer),
		agent.WithSessionManager(sessionManager),
		agent.WithMessenger(h.mockMessenger),
		agent.WithAsyncValidatorDelay(50*time.Millisecond),
		agent.WithCorrectionDelay(50*time.Millisecond),
	)
	require.NoError(t, err)

	// Process message
	ctx := context.Background()
	msg := signal.IncomingMessage{
		Timestamp:  time.Now(),
		From:       "Test User",
		FromNumber: "+15551234567",
		Text:       "What's the weather like?",
	}

	// Track timing
	startTime := time.Now()

	// Process message
	err = handler.Process(ctx, msg)
	require.NoError(t, err)

	// Wait for initial response
	_, err = h.waitForMessage(1 * time.Second)
	require.NoError(t, err)
	responseTime := time.Since(startTime)

	t.Logf("Initial response time: %v", responseTime)
	assert.Less(t, responseTime, 200*time.Millisecond, "Initial response should be within 200ms")

	// Wait for validation to launch
	err = h.waitForValidationLaunch(sessionID, 3*time.Second)
	require.NoError(t, err, "Validation should be launched")

	// Wait for validation to complete
	result, err := h.waitForValidationComplete(sessionID, 5*time.Second)
	require.NoError(t, err, "Validation should complete")
	assert.Equal(t, agent.ValidationStatusFailed, result.Status, "Validation should fail")

	// Wait for correction message
	correction, err := h.waitForMessage(3 * time.Second)
	require.NoError(t, err, "Should receive correction message")
	correctionTime := time.Since(startTime)

	t.Logf("Correction arrived after: %v", correctionTime)

	// Verify correction timing
	assert.Contains(t, correction.Message, "Oops!", "Correction should start with 'Oops!'")
	assert.Greater(t, correctionTime, responseTime+50*time.Millisecond,
		"Correction should arrive after delay")
	assert.Less(t, correctionTime, responseTime+300*time.Millisecond,
		"Correction should not be delayed too long")
}

func TestAsyncValidation_ConcurrentValidations(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup() // Ensure cleanup

	// Track goroutines
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Configure validation with some delay
	validationStrategy := &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status:     agent.ValidationStatusSuccess,
			Confidence: 0.95,
		},
		delay: 30 * time.Millisecond,
	}

	// Configure LLM to return validation-needed responses that complete quickly
	llmResp := claude.LLMResponse{
		Message: "I'll help you with that complex request.",
		Progress: &claude.ProgressInfo{
			NeedsValidation:   true,
			NeedsContinuation: false, // No continuation to avoid loops
		},
	}
	h.mockLLM.SetSessionResponse("", &llmResp)

	// Override QueryFunc to handle any continuation attempts
	h.mockLLM.QueryFunc = func(ctx context.Context, prompt, sid string) (*claude.LLMResponse, error) {
		return &llmResp, nil
	}

	enhancer := mocks.NewMockIntentEnhancer()
	sessionManager := mocks.NewMockSessionManager()

	handler, err := agent.NewHandler(h.mockLLM,
		agent.WithValidationStrategy(validationStrategy),
		agent.WithIntentEnhancer(enhancer),
		agent.WithSessionManager(sessionManager),
		agent.WithMessenger(h.mockMessenger),
		agent.WithAsyncValidatorDelay(50*time.Millisecond),
		agent.WithCorrectionDelay(50*time.Millisecond),
	)
	require.NoError(t, err)

	// Process multiple messages concurrently
	const numMessages = 5
	var wg sync.WaitGroup
	responseTimes := make([]time.Duration, numMessages)

	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx := context.Background()
			msg := signal.IncomingMessage{
				Timestamp:  time.Now(),
				From:       "Test User",
				FromNumber: fmt.Sprintf("+1555123456%d", idx),
				Text:       fmt.Sprintf("Complex request %c", 'A'+idx),
			}

			startTime := time.Now()
			err := handler.Process(ctx, msg)
			responseTimes[idx] = time.Since(startTime)

			assert.NoError(t, err)
		}(i)
	}

	// Wait for all messages to be processed
	wg.Wait()

	// Wait for validations to complete with 50ms correction delay
	time.Sleep(200 * time.Millisecond) // Wait time for async operations with delay

	// Check response times
	for i, rt := range responseTimes {
		t.Logf("Message %d response time: %v", i, rt)
		assert.Less(t, rt, 200*time.Millisecond, "All responses should be within 200ms")
	}

	// Force cleanup before checking goroutines
	runtime.GC()
	runtime.Gosched()
	time.Sleep(20 * time.Millisecond)

	// Verify no goroutine leaks
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d", finalGoroutines)

	// Allow some variance but should be close to initial
	// The async validator may have some goroutines still cleaning up
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+10,
		"Should not leak goroutines (async validators may still be running)")
}

func TestAsyncValidation_FailureScenarios(t *testing.T) {
	tests := []struct {
		name             string
		validationErr    error
		validationResult *agent.ValidationResult
		messengerErr     error
		expectCorrection bool
		description      string
	}{
		{
			name:             "validation_timeout",
			validationErr:    context.DeadlineExceeded,
			expectCorrection: false,
			description:      "Validation timeout should not send correction",
		},
		{
			name: "validation_failed",
			validationResult: &agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"Response was incomplete"},
			},
			expectCorrection: true,
			description:      "Failed validation should send correction",
		},
		{
			name: "messenger_error",
			validationResult: &agent.ValidationResult{
				Status: agent.ValidationStatusFailed,
				Issues: []string{"Correction needed"},
			},
			messengerErr:     errors.New("connection lost"),
			expectCorrection: false, // Can't send if messenger fails
			description:      "Messenger errors should be handled gracefully",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAsyncValidationTestHarness()
			defer h.cleanup() // Ensure cleanup

			// Configure validation
			validationStrategy := &trackingValidationStrategy{
				harness: h,
				result:  tc.validationResult,
				err:     tc.validationErr,
				delay:   10 * time.Millisecond,
			}

			// Configure messenger error if needed
			if tc.messengerErr != nil {
				h.mockMessenger.SetSendError(tc.messengerErr)
			}

			// Configure LLM
			llmResp := claude.LLMResponse{
				Message: "Processing your request.",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   true,
					NeedsContinuation: true,
				},
			}
			h.mockLLM.SetSessionResponse("", &llmResp)

			// Configure recovery generation
			h.mockLLM.QueryFunc = func(ctx context.Context, prompt, sid string) (*claude.LLMResponse, error) {
				// For continuation prompts, complete without continuation
				if strings.Contains(prompt, "continue") || strings.Contains(prompt, "Please continue") {
					return &claude.LLMResponse{
						Message: "Task completed.",
						Progress: &claude.ProgressInfo{
							NeedsValidation:   true,
							NeedsContinuation: false,
						},
					}, nil
				}
				return &llmResp, nil
			}

			enhancer := mocks.NewMockIntentEnhancer()
			sessionManager := mocks.NewMockSessionManager()
			handler, err := agent.NewHandler(h.mockLLM,
				agent.WithValidationStrategy(validationStrategy),
				agent.WithIntentEnhancer(enhancer),
				agent.WithSessionManager(sessionManager),
				agent.WithMessenger(h.mockMessenger),
			)
			require.NoError(t, err)

			// Process message
			ctx := context.Background()
			msg := signal.IncomingMessage{
				Timestamp:  time.Now(),
				From:       "Test User",
				FromNumber: "+15551234567",
				Text:       "Test message",
			}

			err = handler.Process(ctx, msg)
			if tc.messengerErr != nil {
				// For messenger error test, the initial send will fail
				assert.Error(t, err, "Processing should fail when messenger errors")
				return
			}
			require.NoError(t, err, "Initial processing should succeed")

			// Wait for initial message
			_, err = h.waitForMessage(1 * time.Second)
			require.NoError(t, err)

			if tc.expectCorrection && tc.messengerErr == nil {
				// Get session ID for validation tracking
				sessionID := sessionManager.GetOrCreateSession("Test User")

				// Wait for validation to launch
				err = h.waitForValidationLaunch(sessionID, 3*time.Second)
				require.NoError(t, err, "Validation should be launched")

				// Wait for correction
				correction, err := h.waitForMessage(300 * time.Millisecond)
				require.NoError(t, err, "Should receive correction message")
				assert.Contains(t, correction.Message, "Oops!")
			} else {
				// Ensure no correction is sent
				_, err := h.waitForMessage(100 * time.Millisecond)
				assert.Error(t, err, "Should not send correction for %s", tc.description)
			}

			// Wait for async operations to finish before test cleanup
			timer := time.NewTimer(50 * time.Millisecond)
			defer timer.Stop()
			<-timer.C
		})
	}
}

func TestAsyncValidation_ResponseTimeTargets(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup() // Ensure cleanup

	// Track response times for P99 calculation
	var responseTimes []time.Duration
	var mu sync.Mutex

	// Configure validation with no delay for P99 testing
	validationStrategy := &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status:     agent.ValidationStatusSuccess,
			Confidence: 0.95,
		},
		delay: 0, // No delay for performance testing
	}

	// Configure LLM with varying response times
	var requestCount int
	var requestMu sync.Mutex
	h.mockLLM.QueryFunc = func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error) {
		requestMu.Lock()
		requestCount++
		localCount := requestCount
		requestMu.Unlock()

		// Simulate varying Claude response times (5-15ms)
		delay := time.Duration(5+int(time.Now().UnixNano()%10)) * time.Millisecond
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C

		// Check if this is a continuation prompt
		isContinuation := strings.Contains(prompt, "Please continue")

		// Always complete continuations to avoid infinite loops
		if isContinuation {
			return &claude.LLMResponse{
				Message: "Continuation complete",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   false,
					NeedsContinuation: false,
				},
			}, nil
		}

		// Mix of simple and complex responses
		if localCount%3 == 0 {
			// Simple response - no validation needed
			return &claude.LLMResponse{
				Message: "Simple response",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   false,
					NeedsContinuation: false,
				},
			}, nil
		}

		// Complex response - needs validation but no continuation
		return &claude.LLMResponse{
			Message: "Complex response needing validation",
			Progress: &claude.ProgressInfo{
				NeedsValidation:   true,
				NeedsContinuation: false,
			},
		}, nil
	}

	enhancer := mocks.NewMockIntentEnhancer()
	sessionManager := mocks.NewMockSessionManager()

	handler, err := agent.NewHandler(h.mockLLM,
		agent.WithValidationStrategy(validationStrategy),
		agent.WithIntentEnhancer(enhancer),
		agent.WithSessionManager(sessionManager),
		agent.WithMessenger(h.mockMessenger),
		agent.WithAsyncValidatorDelay(50*time.Millisecond),
		agent.WithCorrectionDelay(50*time.Millisecond),
	)
	require.NoError(t, err)

	// Process many messages to calculate P99
	const numRequests = 30 // Balance between good P99 calculation and test speed
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx := context.Background()
			// Use different users to avoid session serialization
			userName := fmt.Sprintf("User%d", idx)
			msg := signal.IncomingMessage{
				Timestamp:  time.Now(),
				From:       userName,
				FromNumber: fmt.Sprintf("+1555123%04d", idx),
				Text:       fmt.Sprintf("Request %c", 'A'+idx%26),
			}

			startTime := time.Now()
			err := handler.Process(ctx, msg)
			responseTime := time.Since(startTime)

			if err != nil {
				t.Logf("Error processing message %d: %v", idx, err)
			}

			mu.Lock()
			responseTimes = append(responseTimes, responseTime)
			mu.Unlock()
		}(i)

		// Smaller stagger to reduce total test time
		if i%10 == 0 {
			timer := time.NewTimer(10 * time.Millisecond)
			defer timer.Stop()
			<-timer.C
		}
	}

	wg.Wait()

	// Wait a bit for all async operations to complete
	time.Sleep(100 * time.Millisecond)

	// Calculate P99
	mu.Lock()
	defer mu.Unlock()

	// Sort response times
	for i := 0; i < len(responseTimes)-1; i++ {
		for j := i + 1; j < len(responseTimes); j++ {
			if responseTimes[i] > responseTimes[j] {
				responseTimes[i], responseTimes[j] = responseTimes[j], responseTimes[i]
			}
		}
	}

	p99Index := int(float64(len(responseTimes)) * 0.99)
	p99Latency := responseTimes[p99Index]

	t.Logf("Response time statistics:")
	t.Logf("  Min: %v", responseTimes[0])
	t.Logf("  Median: %v", responseTimes[len(responseTimes)/2])
	t.Logf("  P99: %v", p99Latency)
	t.Logf("  Max: %v", responseTimes[len(responseTimes)-1])

	// With different users to avoid session serialization, P99 should be under 300ms
	// Note: AsyncValidator has a 50ms correction delay before validation starts
	// Allow 300ms for P99 due to the correction delay affecting some requests
	assert.Less(t, p99Latency, 300*time.Millisecond, "P99 latency should be under 300ms (includes 50ms correction delay)")
}
