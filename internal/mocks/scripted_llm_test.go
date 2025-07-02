package mocks_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/mocks"
)

func TestScriptedLLM_SimpleSequence(t *testing.T) {
	// Test that responses are returned in order
	llm := mocks.NewScriptedLLM()

	// Add a sequence of responses
	llm.AddSimpleScript("First response").
		AddSimpleScript("Second response").
		AddSimpleScript("Third response")

	ctx := context.Background()

	// Query in sequence
	resp1, err := llm.Query(ctx, "prompt1", "session1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp1.Message != "First response" {
		t.Errorf("expected 'First response', got %q", resp1.Message)
	}

	resp2, err := llm.Query(ctx, "prompt2", "session1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.Message != "Second response" {
		t.Errorf("expected 'Second response', got %q", resp2.Message)
	}

	resp3, err := llm.Query(ctx, "prompt3", "session1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp3.Message != "Third response" {
		t.Errorf("expected 'Third response', got %q", resp3.Message)
	}

	// Verify call tracking
	calls := llm.GetCalls()
	if len(calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(calls))
	}
}

func TestScriptedLLM_PatternMatching(t *testing.T) {
	tests := []struct {
		scripts       []mocks.ScriptedResponse
		name          string
		prompt        string
		sessionID     string
		expectedMsg   string
		expectedError bool
	}{
		{
			name: "exact pattern match",
			scripts: []mocks.ScriptedResponse{
				{
					PromptPattern: "^Hello.*",
					Response: &claude.LLMResponse{
						Message: "Greeting detected",
					},
					Repeatable: true,
				},
			},
			prompt:      "Hello, how are you?",
			sessionID:   "test",
			expectedMsg: "Greeting detected",
		},
		{
			name: "session pattern match",
			scripts: []mocks.ScriptedResponse{
				{
					SessionPattern: "^user-.*",
					Response: &claude.LLMResponse{
						Message: "User session",
					},
					Repeatable: true,
				},
			},
			prompt:      "any prompt",
			sessionID:   "user-123",
			expectedMsg: "User session",
		},
		{
			name: "both patterns must match",
			scripts: []mocks.ScriptedResponse{
				{
					PromptPattern:  "schedule",
					SessionPattern: "^admin-.*",
					Response: &claude.LLMResponse{
						Message: "Admin scheduling",
					},
					Repeatable: true,
				},
			},
			prompt:      "I need to schedule a meeting",
			sessionID:   "admin-456",
			expectedMsg: "Admin scheduling",
		},
		{
			name: "no pattern match with fallback",
			scripts: []mocks.ScriptedResponse{
				{
					PromptPattern: "^specific.*",
					Response: &claude.LLMResponse{
						Message: "Specific response",
					},
				},
			},
			prompt:      "general query",
			sessionID:   "test",
			expectedMsg: "No script configured for this prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := mocks.NewScriptedLLM()
			for _, script := range tt.scripts {
				llm.AddScript(script)
			}

			ctx := context.Background()
			resp, err := llm.Query(ctx, tt.prompt, tt.sessionID)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Message != tt.expectedMsg {
				t.Errorf("expected message %q, got %q", tt.expectedMsg, resp.Message)
			}
		})
	}
}

func TestScriptedLLM_ErrorInjection(t *testing.T) {
	llm := mocks.NewScriptedLLM()

	testErr := fmt.Errorf("simulated LLM error")

	// Add mix of success and error responses
	llm.AddSimpleScript("Success 1").
		AddErrorScript(testErr).
		AddSimpleScript("Success 2")

	ctx := context.Background()

	// First call succeeds
	resp1, err := llm.Query(ctx, "prompt1", "session1")
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if resp1.Message != "Success 1" {
		t.Errorf("unexpected response: %q", resp1.Message)
	}

	// Second call returns error
	_, err = llm.Query(ctx, "prompt2", "session1")
	if err == nil {
		t.Error("expected error but got none")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("expected error %v, got %v", testErr, err)
	}

	// Third call succeeds
	resp3, err := llm.Query(ctx, "prompt3", "session1")
	if err != nil {
		t.Fatalf("unexpected error on third call: %v", err)
	}
	if resp3.Message != "Success 2" {
		t.Errorf("unexpected response: %q", resp3.Message)
	}
}

func TestScriptedLLM_Delays(t *testing.T) {
	llm := mocks.NewScriptedLLM()

	// Add response with 100ms delay
	llm.AddDelayedScript(&claude.LLMResponse{
		Message: "Delayed response",
	}, 100*time.Millisecond)

	ctx := context.Background()
	start := time.Now()

	resp, err := llm.Query(ctx, "prompt", "session")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Message != "Delayed response" {
		t.Errorf("unexpected response: %q", resp.Message)
	}

	// Check that delay was applied (with some tolerance)
	if elapsed < 90*time.Millisecond {
		t.Errorf("response too fast, expected ~100ms delay, got %v", elapsed)
	}
}

func TestScriptedLLM_ContextCancellation(t *testing.T) {
	llm := mocks.NewScriptedLLM()

	// Add response with long delay
	llm.AddDelayedScript(&claude.LLMResponse{
		Message: "Should not see this",
	}, 1*time.Second)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := llm.Query(ctx, "prompt", "session")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected context cancellation error")
	}

	// Should have returned quickly due to context cancellation
	if elapsed > 100*time.Millisecond {
		t.Errorf("took too long to cancel: %v", elapsed)
	}
}

func TestScriptedLLM_RepeatableScripts(t *testing.T) {
	llm := mocks.NewScriptedLLM()

	// Add repeatable pattern script
	llm.AddScript(mocks.ScriptedResponse{
		PromptPattern: "memory",
		Response: &claude.LLMResponse{
			Message: "Memory response",
		},
		Repeatable: true,
	})

	// Add non-repeatable script
	llm.AddScript(mocks.ScriptedResponse{
		Response: &claude.LLMResponse{
			Message: "One-time response",
		},
		Repeatable: false,
	})

	ctx := context.Background()

	// Pattern script can be used multiple times
	for i := range 3 {
		resp, err := llm.Query(ctx, "check memory", "session")
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
		if resp.Message != "Memory response" {
			t.Errorf("unexpected response on iteration %d: %q", i, resp.Message)
		}
	}

	// Non-repeatable script only works once
	resp, err := llm.Query(ctx, "other", "session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "One-time response" {
		t.Errorf("unexpected response: %q", resp.Message)
	}

	// Second call to non-repeatable gets fallback
	resp, err = llm.Query(ctx, "other", "session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "No script configured for this prompt" {
		t.Errorf("unexpected response: %q", resp.Message)
	}
}

func TestScriptedLLM_StrictMode(t *testing.T) {
	llm := mocks.NewScriptedLLM(mocks.WithStrictMode())

	// Add one script
	llm.AddSimpleScript("Only response")

	ctx := context.Background()

	// First call works
	resp, err := llm.Query(ctx, "prompt1", "session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "Only response" {
		t.Errorf("unexpected response: %q", resp.Message)
	}

	// Second call fails in strict mode
	_, err = llm.Query(ctx, "prompt2", "session")
	if err == nil {
		t.Error("expected error in strict mode")
	}
}

func TestScriptedLLM_Callbacks(t *testing.T) {
	llm := mocks.NewScriptedLLM()

	var callbackExecuted bool
	var capturedPrompt string
	var capturedSession string

	llm.AddScript(mocks.ScriptedResponse{
		Response: &claude.LLMResponse{
			Message: "Response with callback",
		},
		BeforeReturn: func(prompt, sessionID string) {
			callbackExecuted = true
			capturedPrompt = prompt
			capturedSession = sessionID
		},
	})

	ctx := context.Background()
	resp, err := llm.Query(ctx, "test prompt", "test session")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !callbackExecuted {
		t.Error("callback was not executed")
	}

	if capturedPrompt != "test prompt" {
		t.Errorf("callback received wrong prompt: %q", capturedPrompt)
	}

	if capturedSession != "test session" {
		t.Errorf("callback received wrong session: %q", capturedSession)
	}

	if resp.Message != "Response with callback" {
		t.Errorf("unexpected response: %q", resp.Message)
	}
}

func TestScriptedLLM_MultiTurnConversation(t *testing.T) {
	// Test a realistic multi-turn conversation scenario
	llm := mocks.NewScriptedLLM()

	// Script a conversation about scheduling
	llm.AddScript(mocks.ScriptedResponse{
		PromptPattern: ".*schedule.*meeting.*",
		Response: &claude.LLMResponse{
			Message: "I'll help you schedule a meeting. When would you like to meet?",
			ToolCalls: []claude.ToolCall{
				{
					Tool: "calendar_check",
					Parameters: map[string]claude.ToolParameter{
						"action": claude.NewStringParam("check_availability"),
					},
					Result: "Available slots: 2pm-3pm, 4pm-5pm",
				},
			},
		},
		Repeatable: false,
	}).AddScript(mocks.ScriptedResponse{
		PromptPattern: ".*2pm.*",
		Response: &claude.LLMResponse{
			Message: "I've scheduled the meeting for 2pm. I've sent calendar invites to all participants.",
			ToolCalls: []claude.ToolCall{
				{
					Tool: "calendar_create",
					Parameters: map[string]claude.ToolParameter{
						"time":     claude.NewStringParam("2pm"),
						"duration": claude.NewStringParam("1h"),
					},
					Result: "Meeting created successfully",
				},
			},
		},
		Repeatable: false,
	})

	ctx := context.Background()

	// First turn - request to schedule
	resp1, err := llm.Query(ctx, "I need to schedule a meeting with the team", "user-123")
	if err != nil {
		t.Fatalf("unexpected error on turn 1: %v", err)
	}

	if resp1.Message != "I'll help you schedule a meeting. When would you like to meet?" {
		t.Errorf("unexpected response on turn 1: %q", resp1.Message)
	}

	if len(resp1.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(resp1.ToolCalls))
	}

	// Second turn - select time
	resp2, err := llm.Query(ctx, "Let's do 2pm", "user-123")
	if err != nil {
		t.Fatalf("unexpected error on turn 2: %v", err)
	}

	if resp2.Message != "I've scheduled the meeting for 2pm. I've sent calendar invites to all participants." {
		t.Errorf("unexpected response on turn 2: %q", resp2.Message)
	}

	// Verify conversation was tracked
	calls := llm.GetCalls()
	if len(calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(calls))
	}

	// All calls should be from same session
	for i, call := range calls {
		if call.SessionID != "user-123" {
			t.Errorf("call %d has wrong session ID: %q", i, call.SessionID)
		}
	}
}

func TestScriptedLLM_ConcurrentAccess(t *testing.T) {
	// Test thread safety
	llm := mocks.NewScriptedLLM()

	// Add many repeatable scripts
	for range 10 {
		llm.AddScript(mocks.ScriptedResponse{
			Response: &claude.LLMResponse{
				Message: "Concurrent response",
			},
			Repeatable: true,
		})
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Launch many concurrent queries
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			resp, err := llm.Query(ctx, "concurrent prompt", "session")
			if err != nil {
				errors <- err
				return
			}

			if resp.Message != "Concurrent response" {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}

	// Verify all calls were tracked
	if llm.GetCallCount() != 100 {
		t.Errorf("expected 100 calls, got %d", llm.GetCallCount())
	}
}

func TestScriptedLLM_HelperMethods(t *testing.T) {
	llm := mocks.NewScriptedLLM()

	ctx := context.Background()

	// Make some calls
	_, _ = llm.Query(ctx, "first prompt with keyword", "session1")
	_, _ = llm.Query(ctx, "second prompt", "session2")
	_, _ = llm.Query(ctx, "third prompt with keyword", "session3")

	// Test ExpectNCalls
	if err := llm.ExpectNCalls(3); err != nil {
		t.Errorf("ExpectNCalls failed: %v", err)
	}

	if err := llm.ExpectNCalls(5); err == nil {
		t.Error("ExpectNCalls should have failed for wrong count")
	}

	// Test ExpectPromptContains
	if err := llm.ExpectPromptContains("keyword"); err != nil {
		t.Errorf("ExpectPromptContains failed: %v", err)
	}

	if err := llm.ExpectPromptContains("missing"); err == nil {
		t.Error("ExpectPromptContains should have failed for missing substring")
	}

	// Test Reset
	llm.Reset()
	if llm.GetCallCount() != 0 {
		t.Error("Reset did not clear calls")
	}
}

func TestScriptedLLM_ComplexScenario(t *testing.T) {
	// Test a complex scenario with validation and retries
	llm := mocks.NewScriptedLLM()

	// Initial response with incomplete search
	llm.AddScript(mocks.ScriptedResponse{
		PromptPattern: ".*find.*information.*Josh.*",
		Response: mocks.NewLLMResponseBuilder().Build(
			mocks.WithMessage("I found some information about Josh."),
			mocks.WithToolCalls(claude.ToolCall{
				Tool: "search",
				Parameters: map[string]claude.ToolParameter{
					"query": claude.NewStringParam("Josh"),
				},
				Result: "Found 1 result",
			}),
		),
		Repeatable: false,
	})

	// Validation response - incomplete
	llm.AddScript(mocks.ScriptedResponse{
		PromptPattern: ".*VALIDATION.*",
		Response: mocks.NewLLMResponseBuilder().Build(
			mocks.WithMessage("Status: INCOMPLETE_SEARCH\nThe search was incomplete. Memory was not checked."),
		),
		Repeatable: false,
	})

	// Retry with more thorough search
	llm.AddScript(mocks.ScriptedResponse{
		PromptPattern: ".*find.*information.*Josh.*memory.*",
		Response: mocks.NewLLMResponseBuilder().Build(
			mocks.WithMessage("I've done a thorough search including memory."),
			mocks.WithToolCalls(
				claude.ToolCall{
					Tool: "search",
					Parameters: map[string]claude.ToolParameter{
						"query": claude.NewStringParam("Josh"),
					},
					Result: "Found 3 results",
				},
				claude.ToolCall{
					Tool: "memory_check",
					Parameters: map[string]claude.ToolParameter{
						"topic": claude.NewStringParam("Josh"),
					},
					Result: "Found previous context",
				},
			),
		),
		Repeatable: false,
	})

	// Final validation - success
	llm.AddScript(mocks.ScriptedResponse{
		PromptPattern: ".*VALIDATION.*",
		Response: mocks.NewLLMResponseBuilder().Build(
			mocks.WithMessage("Status: VALID\nThe response is complete and accurate."),
		),
		Repeatable: false,
	})

	ctx := context.Background()

	// Simulate the full flow
	// 1. Initial query
	resp1, _ := llm.Query(ctx, "Can you find information about Josh?", "session")
	if resp1.Message != "I found some information about Josh." {
		t.Errorf("unexpected initial response: %q", resp1.Message)
	}

	// 2. Validation finds it incomplete
	resp2, _ := llm.Query(ctx, "VALIDATION: Check if the response was thorough", "validator")
	if !strings.Contains(resp2.Message, "INCOMPLETE_SEARCH") {
		t.Errorf("expected incomplete search status, got: %q", resp2.Message)
	}

	// 3. Retry with guidance
	resp3, _ := llm.Query(ctx, "Can you find information about Josh? Make sure to check memory.", "session")
	if !strings.Contains(resp3.Message, "thorough search") {
		t.Errorf("unexpected retry response: %q", resp3.Message)
	}
	if len(resp3.ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls in retry, got %d", len(resp3.ToolCalls))
	}

	// 4. Final validation passes
	resp4, _ := llm.Query(ctx, "VALIDATION: Check if the response was thorough", "validator")
	if !strings.Contains(resp4.Message, "VALID") {
		t.Errorf("expected valid status, got: %q", resp4.Message)
	}

	// Verify full conversation flow
	if llm.GetCallCount() != 4 {
		t.Errorf("expected 4 calls for full flow, got %d", llm.GetCallCount())
	}
}
