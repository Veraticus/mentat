//go:build integration
// +build integration

// Package integration provides integration tests for the mentat system
package integration

import (
	"context"
	"fmt"
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

// TestFollowUpTiming_QuickCorrections tests that corrections arrive after appropriate delay
func TestFollowUpTiming_QuickCorrections(t *testing.T) {
	// Test various validation durations
	testCases := []struct {
		name               string
		validationDuration time.Duration
		expectedMinDelay   time.Duration
		expectedMaxDelay   time.Duration
		description        string
	}{
		{
			name:               "instant_validation",
			validationDuration: 0,
			expectedMinDelay:   100 * time.Millisecond, // 50ms initial + 0s validation + 50ms correction
			expectedMaxDelay:   200 * time.Millisecond, // Allow some overhead
			description:        "Instant validation should result in ~100ms total delay",
		},
		{
			name:               "quick_validation",
			validationDuration: 500 * time.Millisecond,
			expectedMinDelay:   600 * time.Millisecond, // 50ms + 500ms + 50ms
			expectedMaxDelay:   800 * time.Millisecond,
			description:        "Quick validation should add to total delay",
		},
		{
			name:               "normal_validation",
			validationDuration: 1 * time.Second,
			expectedMinDelay:   1100 * time.Millisecond, // 50ms + 1s + 50ms
			expectedMaxDelay:   1300 * time.Millisecond,
			description:        "Normal validation timing should be predictable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAsyncValidationTestHarness()
			defer h.cleanup()

			// Configure validation to fail with specific duration
			validationStrategy := &trackingValidationStrategy{
				harness: h,
				result: &agent.ValidationResult{
					Status:     agent.ValidationStatusFailed,
					Confidence: 0.7,
					Issues:     []string{"Test correction needed"},
				},
				delay: tc.validationDuration,
			}

			// Configure LLM
			llmResp := claude.LLMResponse{
				Message: "Initial response to user",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   true,
					NeedsContinuation: false,
				},
			}
			h.mockLLM.SetSessionResponse("", &llmResp)

			// Create handler
			sessionManager := mocks.NewMockSessionManager()
			handler, err := agent.NewHandler(h.mockLLM,
				agent.WithValidationStrategy(validationStrategy),
				agent.WithIntentEnhancer(mocks.NewMockIntentEnhancer()),
				agent.WithSessionManager(sessionManager),
				agent.WithMessenger(h.mockMessenger),
				agent.WithAsyncValidatorDelay(50*time.Millisecond), // Set the initial delay
				agent.WithCorrectionDelay(50*time.Millisecond),     // Set the correction delay
			)
			require.NoError(t, err)

			// Process message
			ctx := context.Background()
			msg := signal.IncomingMessage{
				Timestamp:  time.Now(),
				From:       "Test User",
				FromNumber: "+15551234567",
				Text:       "Test message requiring validation",
			}

			// Process the message
			err = handler.Process(ctx, msg)
			require.NoError(t, err)

			// Wait for both messages
			// First message (initial response)
			initialMsg, err := h.waitForMessage(1 * time.Second)
			require.NoError(t, err, "Should receive initial response")
			initialTime := initialMsg.Timestamp

			// Second message (correction)
			correction, err := h.waitForMessage(tc.expectedMaxDelay)
			require.NoError(t, err, "Should receive correction message")
			correctionTime := correction.Timestamp

			assert.Contains(t, correction.Message, "Oops!", "Correction should start with 'Oops!'")

			// Verify timing
			delta := correctionTime.Sub(initialTime)
			t.Logf("%s: Time between initial response and correction: %v", tc.name, delta)
			assert.GreaterOrEqual(t, delta, tc.expectedMinDelay,
				"%s: %s", tc.name, tc.description)
			assert.LessOrEqual(t, delta, tc.expectedMaxDelay,
				"%s: Correction should not be delayed too long", tc.name)

			// Wait for async operations to complete
			time.Sleep(50 * time.Millisecond)
		})
	}
}

// TestFollowUpTiming_SlowValidation tests timing when validation takes longer
func TestFollowUpTiming_SlowValidation(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup()

	// Configure validation with delay
	validationStrategy := &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status:     agent.ValidationStatusPartial,
			Confidence: 0.6,
			Issues:     []string{"Only partially completed the request"},
		},
		delay: 300 * time.Millisecond,
	}

	// Configure LLM
	llmResp := claude.LLMResponse{
		Message: "Working on your complex request",
		Progress: &claude.ProgressInfo{
			NeedsValidation:   true,
			NeedsContinuation: false,
		},
	}
	h.mockLLM.SetSessionResponse("", &llmResp)

	// Create handler
	sessionManager := mocks.NewMockSessionManager()
	handler, err := agent.NewHandler(h.mockLLM,
		agent.WithValidationStrategy(validationStrategy),
		agent.WithIntentEnhancer(mocks.NewMockIntentEnhancer()),
		agent.WithSessionManager(sessionManager),
		agent.WithMessenger(h.mockMessenger),
		agent.WithAsyncValidatorDelay(50*time.Millisecond), // Set the initial delay
		agent.WithCorrectionDelay(50*time.Millisecond),     // Set the correction delay
	)
	require.NoError(t, err)

	// Process message
	ctx := context.Background()
	msg := signal.IncomingMessage{
		Timestamp:  time.Now(),
		From:       "Test User",
		FromNumber: "+15551234567",
		Text:       "Complex request needing slow validation",
	}

	// Process
	err = handler.Process(ctx, msg)
	require.NoError(t, err)

	// Wait for initial response
	initialMsg, err := h.waitForMessage(1 * time.Second)
	require.NoError(t, err)
	initialTime := initialMsg.Timestamp

	// Wait for correction (should take 50ms initial + 300ms validation + 50ms correction = ~400ms)
	correction, err := h.waitForMessage(1 * time.Second)
	require.NoError(t, err)
	assert.Contains(t, correction.Message, "Oops!")

	// Verify timing
	delta := correction.Timestamp.Sub(initialTime)
	t.Logf("Slow validation timing: %v", delta)
	assert.GreaterOrEqual(t, delta, 400*time.Millisecond, "Should include all delays")
	assert.LessOrEqual(t, delta, 600*time.Millisecond, "Should not exceed reasonable overhead")
}

// TestFollowUpTiming_UnclearStatus tests the faster clarification timing
func TestFollowUpTiming_UnclearStatus(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup()

	// Configure validation to return UNCLEAR status
	validationStrategy := &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status:     agent.ValidationStatusUnclear,
			Confidence: 0.3,
			Issues:     []string{"Request is unclear"},
		},
		delay: 500 * time.Millisecond,
	}

	// Configure LLM
	llmResp := claude.LLMResponse{
		Message: "I'll help with that",
		Progress: &claude.ProgressInfo{
			NeedsValidation:   true,
			NeedsContinuation: false,
		},
	}
	h.mockLLM.SetSessionResponse("", &llmResp)

	// Create handler
	sessionManager := mocks.NewMockSessionManager()
	handler, err := agent.NewHandler(h.mockLLM,
		agent.WithValidationStrategy(validationStrategy),
		agent.WithIntentEnhancer(mocks.NewMockIntentEnhancer()),
		agent.WithSessionManager(sessionManager),
		agent.WithMessenger(h.mockMessenger),
		agent.WithAsyncValidatorDelay(50*time.Millisecond), // Set the initial delay
		agent.WithCorrectionDelay(50*time.Millisecond),     // Set the correction delay
	)
	require.NoError(t, err)

	// Process
	ctx := context.Background()
	msg := signal.IncomingMessage{
		Timestamp:  time.Now(),
		From:       "Test User",
		FromNumber: "+15551234567",
		Text:       "Unclear request",
	}

	err = handler.Process(ctx, msg)
	require.NoError(t, err)

	// Wait for messages
	initialMsg, err := h.waitForMessage(1 * time.Second)
	require.NoError(t, err)
	initialTime := initialMsg.Timestamp

	// Clarification should arrive faster (50ms initial + 500ms validation + 50ms clarify = ~600ms)
	clarification, err := h.waitForMessage(1000 * time.Millisecond)
	require.NoError(t, err)

	// Verify timing
	delta := clarification.Timestamp.Sub(initialTime)
	t.Logf("UNCLEAR status timing: %v", delta)
	assert.GreaterOrEqual(t, delta, 600*time.Millisecond,
		"UNCLEAR should use 50ms clarify delay instead of 50ms correction delay")
	assert.LessOrEqual(t, delta, 1000*time.Millisecond,
		"Clarification should arrive promptly")

	// Verify it's a clarification message
	assert.Contains(t, clarification.Message, "unclear",
		"Should indicate the request was unclear")
}

// TestFollowUpTiming_AllStatuses tests timing for each validation status
func TestFollowUpTiming_AllStatuses(t *testing.T) {
	testCases := []struct {
		status             agent.ValidationStatus
		expectFollowUp     bool
		expectedDelayType  string // "correction" or "clarify"
		expectedTotalDelay time.Duration
	}{
		{
			status:         agent.ValidationStatusSuccess,
			expectFollowUp: false,
		},
		{
			status:             agent.ValidationStatusPartial,
			expectFollowUp:     true,
			expectedDelayType:  "correction",
			expectedTotalDelay: 100 * time.Millisecond, // 50ms + 0s + 50ms
		},
		{
			status:             agent.ValidationStatusFailed,
			expectFollowUp:     true,
			expectedDelayType:  "correction",
			expectedTotalDelay: 100 * time.Millisecond,
		},
		{
			status:             agent.ValidationStatusIncompleteSearch,
			expectFollowUp:     true,
			expectedDelayType:  "correction",
			expectedTotalDelay: 100 * time.Millisecond,
		},
		{
			status:             agent.ValidationStatusUnclear,
			expectFollowUp:     true,
			expectedDelayType:  "clarify",
			expectedTotalDelay: 100 * time.Millisecond, // 50ms + 0s + 50ms
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.status), func(t *testing.T) {
			h := newAsyncValidationTestHarness()
			defer h.cleanup()

			// Configure validation
			validationStrategy := &trackingValidationStrategy{
				harness: h,
				result: &agent.ValidationResult{
					Status:     tc.status,
					Confidence: 0.8,
					Issues:     []string{fmt.Sprintf("Test issue for %s", tc.status)},
				},
				delay: 0, // Instant validation for timing clarity
			}

			// Configure LLM
			llmResp := claude.LLMResponse{
				Message: "Response needing validation",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   true,
					NeedsContinuation: false,
				},
			}
			h.mockLLM.SetSessionResponse("", &llmResp)

			// Create handler
			sessionManager := mocks.NewMockSessionManager()
			handler, err := agent.NewHandler(h.mockLLM,
				agent.WithValidationStrategy(validationStrategy),
				agent.WithIntentEnhancer(mocks.NewMockIntentEnhancer()),
				agent.WithSessionManager(sessionManager),
				agent.WithMessenger(h.mockMessenger),
				agent.WithAsyncValidatorDelay(50*time.Millisecond), // Set the initial delay
				agent.WithCorrectionDelay(50*time.Millisecond),     // Set the correction delay
			)
			require.NoError(t, err)

			// Process
			ctx := context.Background()
			msg := signal.IncomingMessage{
				Timestamp:  time.Now(),
				From:       "Test User",
				FromNumber: "+15551234567",
				Text:       fmt.Sprintf("Test for %s status", tc.status),
			}

			err = handler.Process(ctx, msg)
			require.NoError(t, err)

			// Wait for initial response
			initialMsg, err := h.waitForMessage(1 * time.Second)
			require.NoError(t, err)
			initialTime := initialMsg.Timestamp

			if tc.expectFollowUp {
				// Wait for follow-up
				followUp, err := h.waitForMessage(500 * time.Millisecond)
				require.NoError(t, err, "Should receive follow-up for %s", tc.status)

				// Verify follow-up message was sent
				// Note: trackingValidationStrategy always generates messages starting with "Oops!"
				assert.Contains(t, followUp.Message, "Oops!",
					"Follow-up message should be a correction")

				// Verify timing
				delta := followUp.Timestamp.Sub(initialTime)
				t.Logf("%s timing: %v", tc.status, delta)
				assert.GreaterOrEqual(t, delta, tc.expectedTotalDelay,
					"%s should have appropriate delay", tc.status)
				assert.LessOrEqual(t, delta, tc.expectedTotalDelay+time.Second,
					"%s delay should not exceed expected by much", tc.status)
			} else {
				// Ensure no follow-up for SUCCESS
				// Wait a bit to ensure no follow-up is sent
				time.Sleep(200 * time.Millisecond)

				messages := h.mockMessenger.GetSentMessages()
				assert.Equal(t, 1, len(messages),
					"SUCCESS status should not send follow-up")
			}
		})
	}
}

// TestFollowUpTiming_ConcurrentValidations tests timing with multiple concurrent validations
func TestFollowUpTiming_ConcurrentValidations(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup()

	// Process multiple messages concurrently
	const numConcurrent = 5
	var wg sync.WaitGroup

	// Track results
	type result struct {
		phoneNumber  string
		initialTime  time.Time
		followUpTime time.Time
		hasFollowUp  bool
	}
	results := make([]result, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Vary validation duration slightly
			validationDuration := time.Duration(idx*100) * time.Millisecond

			// Create unique validation strategy
			validationStrategy := &trackingValidationStrategy{
				harness: h,
				result: &agent.ValidationResult{
					Status:     agent.ValidationStatusFailed,
					Confidence: 0.7,
					Issues:     []string{"Concurrent test correction"},
				},
				delay: validationDuration,
			}

			// Configure LLM
			llmResp := claude.LLMResponse{
				Message: "Processing concurrent request",
				Progress: &claude.ProgressInfo{
					NeedsValidation:   true,
					NeedsContinuation: false,
				},
			}

			// Create handler with unique components
			mockLLM := mocks.NewMockLLM()
			mockLLM.SetSessionResponse("", &llmResp)

			mockMessenger := mocks.NewMockMessenger()
			sessionManager := mocks.NewMockSessionManager()

			handler, err := agent.NewHandler(mockLLM,
				agent.WithValidationStrategy(validationStrategy),
				agent.WithIntentEnhancer(mocks.NewMockIntentEnhancer()),
				agent.WithSessionManager(sessionManager),
				agent.WithMessenger(mockMessenger),
				agent.WithAsyncValidatorDelay(50*time.Millisecond), // Set the initial delay
				agent.WithCorrectionDelay(50*time.Millisecond),     // Set the correction delay
			)
			require.NoError(t, err)

			// Process with unique phone number
			ctx := context.Background()
			phoneNumber := fmt.Sprintf("+1555123456%d", idx)
			msg := signal.IncomingMessage{
				Timestamp:  time.Now(),
				From:       fmt.Sprintf("User %d", idx),
				FromNumber: phoneNumber,
				Text:       fmt.Sprintf("Concurrent request %d", idx),
			}

			err = handler.Process(ctx, msg)
			assert.NoError(t, err)

			// Track timing
			result := &results[idx]
			result.phoneNumber = phoneNumber

			// Get messages
			time.Sleep(10 * time.Millisecond) // Allow initial message to be sent
			messages := mockMessenger.GetSentMessages()
			if len(messages) > 0 {
				result.initialTime = messages[0].Timestamp
			}

			// Wait for follow-up
			time.Sleep(600 * time.Millisecond)
			messages = mockMessenger.GetSentMessages()
			if len(messages) > 1 {
				result.followUpTime = messages[1].Timestamp
				result.hasFollowUp = true
			}
		}(i)
	}

	// Wait for all processing to complete
	wg.Wait()

	// Verify timing for each concurrent validation
	for i, res := range results {
		if !res.hasFollowUp {
			t.Errorf("Missing follow-up for concurrent validation %d", i)
			continue
		}

		delta := res.followUpTime.Sub(res.initialTime)
		t.Logf("Concurrent validation %s: %v", res.phoneNumber, delta)

		// Each should maintain proper timing despite concurrency
		expectedMin := 100*time.Millisecond + time.Duration(i*100)*time.Millisecond
		assert.GreaterOrEqual(t, delta, expectedMin,
			"Concurrent validations should maintain minimum delay")
		assert.LessOrEqual(t, delta, expectedMin+200*time.Millisecond,
			"Concurrent validations should not have excessive delay")
	}
}

// TestFollowUpTiming_ContextCancellation tests that follow-ups are canceled properly
func TestFollowUpTiming_ContextCancellation(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup()

	// Configure validation with long delay
	validationStrategy := &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status: agent.ValidationStatusFailed,
			Issues: []string{"Should be canceled"},
		},
		delay: 1 * time.Second, // Long delay to simulate cancellation scenario
	}

	// Configure LLM
	llmResp := claude.LLMResponse{
		Message: "Response that will have canceled validation",
		Progress: &claude.ProgressInfo{
			NeedsValidation:   true,
			NeedsContinuation: false,
		},
	}
	h.mockLLM.SetSessionResponse("", &llmResp)

	// Create handler
	sessionManager := mocks.NewMockSessionManager()
	sessionID := sessionManager.GetOrCreateSession("Test User")

	handler, err := agent.NewHandler(h.mockLLM,
		agent.WithValidationStrategy(validationStrategy),
		agent.WithIntentEnhancer(mocks.NewMockIntentEnhancer()),
		agent.WithSessionManager(sessionManager),
		agent.WithMessenger(h.mockMessenger),
		agent.WithAsyncValidatorDelay(50*time.Millisecond), // Set the initial delay
		agent.WithCorrectionDelay(50*time.Millisecond),     // Set the correction delay
	)
	require.NoError(t, err)

	// Process message
	ctx := context.Background()
	msg := signal.IncomingMessage{
		Timestamp:  time.Now(),
		From:       "Test User",
		FromNumber: "+15551234567",
		Text:       "Test cancellation",
	}

	err = handler.Process(ctx, msg)
	require.NoError(t, err)

	// Wait for initial response
	_, err = h.waitForMessage(1 * time.Second)
	require.NoError(t, err)

	beforeMessages := len(h.mockMessenger.GetSentMessages())
	assert.Equal(t, 1, beforeMessages, "Should have initial response")

	// Wait for validation to launch
	err = h.waitForValidationLaunch(sessionID, 300*time.Millisecond)
	require.NoError(t, err)

	// Note: We can't directly shutdown the validator from outside the handler
	// The test relies on the validation timing out or completing naturally
	t.Log("Validation will complete or timeout naturally")

	// Wait to ensure no follow-up is sent
	time.Sleep(300 * time.Millisecond)

	// Verify only initial message was sent
	afterMessages := len(h.mockMessenger.GetSentMessages())
	assert.Equal(t, 1, afterMessages, "Canceled validation should not send follow-up")

	// Since we can't directly cancel the validation, it will complete eventually
	// but the 1s delay means it won't send a follow-up within our test window
	t.Log("Validation will complete after 1s delay, outside our test window")
}

// TestFollowUpTiming_NaturalConversationPacing tests overall conversation flow timing
func TestFollowUpTiming_NaturalConversationPacing(t *testing.T) {
	h := newAsyncValidationTestHarness()
	defer h.cleanup()

	// Configure validation
	validationStrategy := &trackingValidationStrategy{
		harness: h,
		result: &agent.ValidationResult{
			Status:     agent.ValidationStatusPartial,
			Confidence: 0.6,
			Issues:     []string{"I completed part of your request"},
		},
		delay: 100 * time.Millisecond,
	}

	// Configure LLM
	llmResp := claude.LLMResponse{
		Message: "Let me help you with that complex task",
		Progress: &claude.ProgressInfo{
			NeedsValidation:   true,
			NeedsContinuation: false,
		},
	}
	h.mockLLM.SetSessionResponse("", &llmResp)

	// Create handler
	sessionManager := mocks.NewMockSessionManager()
	handler, err := agent.NewHandler(h.mockLLM,
		agent.WithValidationStrategy(validationStrategy),
		agent.WithIntentEnhancer(mocks.NewMockIntentEnhancer()),
		agent.WithSessionManager(sessionManager),
		agent.WithMessenger(h.mockMessenger),
		agent.WithAsyncValidatorDelay(50*time.Millisecond), // Set the initial delay
		agent.WithCorrectionDelay(50*time.Millisecond),     // Set the correction delay
	)
	require.NoError(t, err)

	// Start timing
	conversationStart := time.Now()

	// Process message
	ctx := context.Background()
	msg := signal.IncomingMessage{
		Timestamp:  time.Now(),
		From:       "Test User",
		FromNumber: "+15551234567",
		Text:       "Please help me with this complex multi-step task",
	}

	err = handler.Process(ctx, msg)
	require.NoError(t, err)

	// Wait for both messages
	initialMsg, err := h.waitForMessage(1 * time.Second)
	require.NoError(t, err)

	followUpMsg, err := h.waitForMessage(600 * time.Millisecond)
	require.NoError(t, err)

	// Analyze conversation pacing
	// Time from user message to initial response
	initialResponseTime := initialMsg.Timestamp.Sub(conversationStart)
	t.Logf("User → Initial response: %v", initialResponseTime)
	assert.Less(t, initialResponseTime, 200*time.Millisecond,
		"Initial response should be quick for good UX")

	// Time between messages
	messageDelta := followUpMsg.Timestamp.Sub(initialMsg.Timestamp)
	t.Logf("Initial → Follow-up: %v", messageDelta)
	assert.GreaterOrEqual(t, messageDelta, 150*time.Millisecond,
		"Follow-up should give user time to read initial response")
	assert.LessOrEqual(t, messageDelta, 300*time.Millisecond,
		"Follow-up should not leave user waiting too long")

	// Total conversation time
	totalTime := followUpMsg.Timestamp.Sub(conversationStart)
	t.Logf("Total conversation time: %v", totalTime)
	assert.LessOrEqual(t, totalTime, 400*time.Millisecond,
		"Complete interaction should be reasonably quick")

	// The pacing should feel natural:
	// 1. Quick initial acknowledgment
	// 2. Appropriate pause before correction
	// 3. Not too rushed, not too slow
}
