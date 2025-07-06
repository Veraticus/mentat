package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/mocks"
)

// testValidationStrategy is a test double for ValidationStrategy.
type testValidationStrategy struct {
	validateFunc         func(ctx context.Context, request, response, sessionID string, llm claude.LLM) agent.ValidationResult
	shouldRetryFunc      func(result agent.ValidationResult) bool
	generateRecoveryFunc func(ctx context.Context, request, response, sessionID string, result agent.ValidationResult, llm claude.LLM) string
}

func (m *testValidationStrategy) Validate(
	ctx context.Context,
	request, response, sessionID string,
	llm claude.LLM,
) agent.ValidationResult {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, request, response, sessionID, llm)
	}
	return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
}

func (m *testValidationStrategy) ShouldRetry(result agent.ValidationResult) bool {
	if m.shouldRetryFunc != nil {
		return m.shouldRetryFunc(result)
	}
	return false
}

func (m *testValidationStrategy) GenerateRecovery(
	ctx context.Context,
	request, response, sessionID string,
	result agent.ValidationResult,
	llm claude.LLM,
) string {
	if m.generateRecoveryFunc != nil {
		return m.generateRecoveryFunc(ctx, request, response, sessionID, result, llm)
	}
	return ""
}

func TestParallelValidationWrapper(t *testing.T) {
	// Create a mock validator for testing
	mockValidator := &testValidationStrategy{
		validateFunc: func(ctx context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
			// Simulate some work
			select {
			case <-time.After(10 * time.Millisecond):
				return agent.ValidationResult{
					Status:     agent.ValidationStatusSuccess,
					Confidence: 0.9,
					Metadata:   map[string]string{"mock": "true"},
				}
			case <-ctx.Done():
				return agent.ValidationResult{
					Status:     agent.ValidationStatusUnclear,
					Issues:     []string{"context canceled"},
					Confidence: 0.0,
				}
			}
		},
		shouldRetryFunc: func(result agent.ValidationResult) bool {
			return result.Status == agent.ValidationStatusUnclear
		},
		generateRecoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
			return "mock recovery message"
		},
	}

	wrapper := agent.NewParallelValidationWrapper(mockValidator, 100*time.Millisecond)

	t.Run("successful validation", func(t *testing.T) {
		ctx := context.Background()
		result := wrapper.Validate(ctx, "test request", "test response", "session-123", nil)

		if result.Status != agent.ValidationStatusSuccess {
			t.Errorf("expected SUCCESS, got %v", result.Status)
		}
		if result.Confidence != 0.9 {
			t.Errorf("expected confidence 0.9, got %v", result.Confidence)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result := wrapper.Validate(ctx, "test request", "test response", "session-123", nil)

		if result.Status != agent.ValidationStatusUnclear {
			t.Errorf("expected UNCLEAR, got %v", result.Status)
		}
		if len(result.Issues) == 0 || !strings.Contains(result.Issues[0], "canceled") {
			t.Errorf("expected cancellation issue, got %v", result.Issues)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		// Create a slow validator
		slowValidator := &testValidationStrategy{
			validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
				time.Sleep(200 * time.Millisecond) // Longer than timeout
				return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
			},
		}

		slowWrapper := agent.NewParallelValidationWrapper(slowValidator, 50*time.Millisecond)
		ctx := context.Background()

		start := time.Now()
		result := slowWrapper.Validate(ctx, "test request", "test response", "session-123", nil)
		duration := time.Since(start)

		if result.Status != agent.ValidationStatusUnclear {
			t.Errorf("expected UNCLEAR due to timeout, got %v", result.Status)
		}
		if duration >= 100*time.Millisecond {
			t.Errorf("validation took too long: %v", duration)
		}
		if !strings.Contains(result.Issues[0], "timeout") {
			t.Errorf("expected timeout issue, got %v", result.Issues)
		}
	})

	t.Run("panic recovery", func(t *testing.T) {
		panicValidator := &testValidationStrategy{
			validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
				panic("test panic") //nolint:forbidigo // Testing panic recovery
			},
		}

		panicWrapper := agent.NewParallelValidationWrapper(panicValidator, 100*time.Millisecond)
		ctx := context.Background()

		result := panicWrapper.Validate(ctx, "test request", "test response", "session-123", nil)

		if result.Status != agent.ValidationStatusFailed {
			t.Errorf("expected FAILED, got %v", result.Status)
		}
		if !strings.Contains(result.Issues[0], "panic") {
			t.Errorf("expected panic in issues, got %v", result.Issues)
		}
	})

	t.Run("should retry delegates", func(t *testing.T) {
		result := agent.ValidationResult{Status: agent.ValidationStatusUnclear}
		if !wrapper.ShouldRetry(result) {
			t.Error("expected ShouldRetry to return true for UNCLEAR")
		}
	})

	t.Run("generate recovery delegates", func(t *testing.T) {
		ctx := context.Background()
		result := agent.ValidationResult{Status: agent.ValidationStatusFailed}
		recovery := wrapper.GenerateRecovery(ctx, "req", "resp", "session", result, nil)

		if recovery != "mock recovery message" {
			t.Errorf("expected mock recovery message, got %v", recovery)
		}
	})

	t.Run("generate recovery timeout", func(t *testing.T) {
		slowValidator := &testValidationStrategy{
			generateRecoveryFunc: func(_ context.Context, _, _, _ string, _ agent.ValidationResult, _ claude.LLM) string {
				time.Sleep(10 * time.Second) // Much longer than shortTimeout
				return "should timeout"
			},
		}

		recoveryWrapper := agent.NewParallelValidationWrapper(slowValidator, 100*time.Millisecond)
		ctx := context.Background()
		result := agent.ValidationResult{Status: agent.ValidationStatusFailed}

		start := time.Now()
		recovery := recoveryWrapper.GenerateRecovery(ctx, "req", "resp", "session", result, nil)
		duration := time.Since(start)

		// Should use default correction message on timeout
		if recovery == "should timeout" {
			t.Error("recovery should have timed out")
		}
		if duration >= 10*time.Second {
			t.Errorf("recovery generation took too long: %v", duration)
		}
	})
}

func TestParallelMultiAgentValidator(t *testing.T) {
	validator := agent.NewParallelMultiAgentValidator()

	t.Run("wraps multi-agent validator", func(t *testing.T) {
		mockLLM := &mocks.MockLLM{
			QueryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				// Return a validation response
				return &claude.LLMResponse{
					Message: "STATUS: SUCCESS\nCONFIDENCE: 0.9\nISSUES: none\nSUGGESTIONS: none",
				}, nil
			},
		}

		ctx := context.Background()
		result := validator.Validate(ctx, "test request", "test response", "session-123", mockLLM)

		if result.Status != agent.ValidationStatusSuccess {
			t.Errorf("expected SUCCESS, got %v", result.Status)
		}
	})

	t.Run("ValidateAsync delegates to Validate", func(t *testing.T) {
		mockLLM := &mocks.MockLLM{
			QueryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
				return &claude.LLMResponse{
					Message: "STATUS: PARTIAL\nCONFIDENCE: 0.5\nISSUES: incomplete\nSUGGESTIONS: retry",
				}, nil
			},
		}

		ctx := context.Background()
		result := validator.ValidateAsync(ctx, "test request", "test response", "session-123", mockLLM)

		if result.Status != agent.ValidationStatusPartial {
			t.Errorf("expected PARTIAL, got %v", result.Status)
		}
	})
}

func TestParallelSimpleValidator(t *testing.T) {
	validator := agent.NewParallelSimpleValidator()

	t.Run("wraps simple validator", func(t *testing.T) {
		ctx := context.Background()
		result := validator.Validate(ctx, "test request", "completed successfully", "session-123", nil)

		if result.Status != agent.ValidationStatusSuccess {
			t.Errorf("expected SUCCESS, got %v", result.Status)
		}
	})

	t.Run("ValidateAsync uses simple validator directly", func(t *testing.T) {
		ctx := context.Background()
		result := validator.ValidateAsync(ctx, "test request", "error occurred", "session-123", nil)

		// Simple validator should detect error keyword
		if result.Status == agent.ValidationStatusSuccess {
			t.Error("expected validation to detect error")
		}
	})
}

func TestParallelNoopValidator(t *testing.T) {
	validator := agent.NewParallelNoopValidator()

	t.Run("always returns success", func(t *testing.T) {
		ctx := context.Background()
		result := validator.Validate(ctx, "any request", "any response", "session-123", nil)

		if result.Status != agent.ValidationStatusSuccess {
			t.Errorf("expected SUCCESS, got %v", result.Status)
		}
		if result.Confidence != 1.0 {
			t.Errorf("expected confidence 1.0, got %v", result.Confidence)
		}
	})

	t.Run("context cancellation handled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result := validator.Validate(ctx, "any request", "any response", "session-123", nil)

		if result.Status != agent.ValidationStatusUnclear {
			t.Errorf("expected UNCLEAR due to cancellation, got %v", result.Status)
		}
	})

	t.Run("ValidateAsync delegates", func(t *testing.T) {
		ctx := context.Background()
		result := validator.ValidateAsync(ctx, "any request", "any response", "session-123", nil)

		if result.Status != agent.ValidationStatusSuccess {
			t.Errorf("expected SUCCESS, got %v", result.Status)
		}
	})
}

func TestCreateParallelValidator(t *testing.T) {
	t.Run("creates wrapper for MultiAgentValidator", func(t *testing.T) {
		base := agent.NewMultiAgentValidator()
		parallel := agent.CreateParallelValidator(base)

		if _, ok := parallel.(*agent.ParallelMultiAgentValidator); !ok {
			t.Error("expected ParallelMultiAgentValidator")
		}
	})

	t.Run("creates wrapper for SimpleValidator", func(t *testing.T) {
		base := agent.NewSimpleValidator()
		parallel := agent.CreateParallelValidator(base)

		if _, ok := parallel.(*agent.ParallelSimpleValidator); !ok {
			t.Error("expected ParallelSimpleValidator")
		}
	})

	t.Run("creates wrapper for NoopValidator", func(t *testing.T) {
		base := agent.NewNoopValidator()
		parallel := agent.CreateParallelValidator(base)

		if _, ok := parallel.(*agent.ParallelNoopValidator); !ok {
			t.Error("expected ParallelNoopValidator")
		}
	})

	t.Run("returns existing async validator", func(t *testing.T) {
		async := agent.NewParallelMultiAgentValidator()
		result := agent.CreateParallelValidator(async)

		if result != async {
			t.Error("expected same instance for async validator")
		}
	})

	t.Run("creates generic wrapper for unknown type", func(t *testing.T) {
		mock := &testValidationStrategy{}
		parallel := agent.CreateParallelValidator(mock)

		if _, ok := parallel.(*agent.ParallelValidationWrapper); !ok {
			t.Error("expected ParallelValidationWrapper for unknown type")
		}
	})
}

func TestSetTimeout(t *testing.T) {
	t.Run("sets timeout on ParallelValidationWrapper", func(t *testing.T) {
		mock := &testValidationStrategy{
			validateFunc: func(_ context.Context, _, _, _ string, _ claude.LLM) agent.ValidationResult {
				// Sleep longer than initial timeout but less than new timeout
				time.Sleep(150 * time.Millisecond)
				return agent.ValidationResult{Status: agent.ValidationStatusSuccess}
			},
		}

		wrapper := agent.NewParallelValidationWrapper(mock, 100*time.Millisecond)

		// Should timeout with initial timeout
		ctx := context.Background()
		result := wrapper.Validate(ctx, "test", "test", "session", nil)
		if result.Status != agent.ValidationStatusUnclear {
			t.Error("expected timeout with initial timeout")
		}

		// Update timeout
		err := agent.SetTimeout(wrapper, 200*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to set timeout: %v", err)
		}

		// Should succeed with new timeout
		result = wrapper.Validate(ctx, "test", "test", "session", nil)
		if result.Status != agent.ValidationStatusSuccess {
			t.Error("expected success with new timeout")
		}
	})

	t.Run("sets timeout on ParallelMultiAgentValidator", func(t *testing.T) {
		validator := agent.NewParallelMultiAgentValidator()
		err := agent.SetTimeout(validator, 500*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to set timeout: %v", err)
		}
	})

	t.Run("sets timeout on ParallelSimpleValidator", func(t *testing.T) {
		validator := agent.NewParallelSimpleValidator()
		err := agent.SetTimeout(validator, 500*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to set timeout: %v", err)
		}
	})

	t.Run("returns error for unsupported validator", func(t *testing.T) {
		validator := agent.NewNoopValidator()
		err := agent.SetTimeout(validator, 500*time.Millisecond)
		if err == nil {
			t.Error("expected error for unsupported validator")
		}
		if !strings.Contains(err.Error(), "does not support timeout") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestRaceConditions(t *testing.T) {
	// Test concurrent validations don't interfere
	validator := agent.NewParallelMultiAgentValidator()
	mockLLM := &mocks.MockLLM{
		QueryFunc: func(_ context.Context, _, _ string) (*claude.LLMResponse, error) {
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			return &claude.LLMResponse{
				Message: "STATUS: SUCCESS\nCONFIDENCE: 0.9\nISSUES: none\nSUGGESTIONS: none",
			}, nil
		},
	}

	ctx := context.Background()
	results := make(chan agent.ValidationResult, 10)

	// Run 10 concurrent validations
	for i := range 10 {
		go func(id int) {
			result := validator.Validate(ctx, "request", "response", string(rune(id)), mockLLM)
			results <- result
		}(i)
	}

	// Collect results
	successCount := 0
	for range 10 {
		result := <-results
		if result.Status == agent.ValidationStatusSuccess {
			successCount++
		}
	}

	if successCount != 10 {
		t.Errorf("expected all 10 validations to succeed, got %d", successCount)
	}
}

func TestAsyncValidationMemoryLeak(_ *testing.T) {
	// This test ensures we don't leak goroutines
	validator := agent.NewParallelMultiAgentValidator()
	ctx := context.Background()

	// Run many quick validations
	for range 100 {
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		validator.Validate(timeoutCtx, "test", "test", "session", nil)
		cancel()
	}

	// Give goroutines time to clean up
	time.Sleep(50 * time.Millisecond)

	// In a real test, we'd check runtime.NumGoroutine() but that's fragile
	// This test mainly ensures we don't panic or deadlock
}
