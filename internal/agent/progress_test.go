package agent_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Veraticus/mentat/internal/agent"
)

// mockProgressReporter is a mock implementation of ProgressReporter for testing.
type mockProgressReporter struct {
	mu sync.Mutex

	reportErrorCalls []reportErrorCall
	reportErrorFunc  func(ctx context.Context, err error, userFriendlyMessage string) error

	shouldContinueCalls []error
	shouldContinueFunc  func(err error) bool
}

type reportErrorCall struct {
	ctx                 context.Context
	err                 error
	userFriendlyMessage string
}

func (m *mockProgressReporter) ReportError(ctx context.Context, err error, userFriendlyMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reportErrorCalls = append(m.reportErrorCalls, reportErrorCall{
		ctx:                 ctx,
		err:                 err,
		userFriendlyMessage: userFriendlyMessage,
	})

	if m.reportErrorFunc != nil {
		return m.reportErrorFunc(ctx, err, userFriendlyMessage)
	}

	return nil
}

func (m *mockProgressReporter) ShouldContinue(err error) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shouldContinueCalls = append(m.shouldContinueCalls, err)

	if m.shouldContinueFunc != nil {
		return m.shouldContinueFunc(err)
	}

	return false
}

func (m *mockProgressReporter) getReportErrorCalls() []reportErrorCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	calls := make([]reportErrorCall, len(m.reportErrorCalls))
	copy(calls, m.reportErrorCalls)
	return calls
}

func (m *mockProgressReporter) getShouldContinueCalls() []error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	calls := make([]error, len(m.shouldContinueCalls))
	copy(calls, m.shouldContinueCalls)
	return calls
}

func TestProgressReporterInterface(_ *testing.T) {
	// Verify that our mock implements the interface
	var _ agent.ProgressReporter = (*mockProgressReporter)(nil)
}

func TestProgressReporterReportError(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		userFriendlyMsg   string
		reportErrorFunc   func(ctx context.Context, err error, userFriendlyMessage string) error
		expectedReportErr error
		expectedCallCount int
	}{
		{
			name:              "successful error report",
			err:               fmt.Errorf("internal error"),
			userFriendlyMsg:   "Something went wrong, please try again",
			reportErrorFunc:   nil, // Use default behavior
			expectedReportErr: nil,
			expectedCallCount: 1,
		},
		{
			name:            "error report fails",
			err:             fmt.Errorf("internal error"),
			userFriendlyMsg: "Something went wrong",
			reportErrorFunc: func(_ context.Context, _ error, _ string) error {
				return fmt.Errorf("failed to send message")
			},
			expectedReportErr: fmt.Errorf("failed to send message"),
			expectedCallCount: 1,
		},
		{
			name:              "nil error with message",
			err:               nil,
			userFriendlyMsg:   "Operation completed with warnings",
			reportErrorFunc:   nil,
			expectedReportErr: nil,
			expectedCallCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			reporter := &mockProgressReporter{
				reportErrorFunc: tt.reportErrorFunc,
			}

			err := reporter.ReportError(ctx, tt.err, tt.userFriendlyMsg)

			// Check return error
			if tt.expectedReportErr != nil {
				if err == nil || err.Error() != tt.expectedReportErr.Error() {
					t.Errorf("Expected error %v, got %v", tt.expectedReportErr, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}

			// Check calls were recorded
			calls := reporter.getReportErrorCalls()
			if len(calls) != tt.expectedCallCount {
				t.Errorf("Expected %d calls, got %d", tt.expectedCallCount, len(calls))
			}

			if tt.expectedCallCount > 0 {
				call := calls[0]
				if !errors.Is(call.err, tt.err) {
					t.Errorf("Expected error %v, got %v", tt.err, call.err)
				}
				if call.userFriendlyMessage != tt.userFriendlyMsg {
					t.Errorf("Expected message %q, got %q", tt.userFriendlyMsg, call.userFriendlyMessage)
				}
			}
		})
	}
}

func TestProgressReporterShouldContinue(t *testing.T) {
	tests := []struct {
		name               string
		err                error
		shouldContinueFunc func(err error) bool
		expectedResult     bool
	}{
		{
			name: "continue on non-critical error",
			err:  fmt.Errorf("timeout error"),
			shouldContinueFunc: func(err error) bool {
				return err.Error() == "timeout error"
			},
			expectedResult: true,
		},
		{
			name: "stop on critical error",
			err:  fmt.Errorf("authentication failed"),
			shouldContinueFunc: func(_ error) bool {
				return false
			},
			expectedResult: false,
		},
		{
			name:               "default behavior stops",
			err:                fmt.Errorf("some error"),
			shouldContinueFunc: nil, // Use default
			expectedResult:     false,
		},
		{
			name: "nil error continues",
			err:  nil,
			shouldContinueFunc: func(err error) bool {
				return err == nil
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := &mockProgressReporter{
				shouldContinueFunc: tt.shouldContinueFunc,
			}

			result := reporter.ShouldContinue(tt.err)

			if result != tt.expectedResult {
				t.Errorf("Expected %v, got %v", tt.expectedResult, result)
			}

			// Check the call was recorded
			calls := reporter.getShouldContinueCalls()
			if len(calls) != 1 {
				t.Errorf("Expected 1 call, got %d", len(calls))
			}
			if !errors.Is(calls[0], tt.err) {
				t.Errorf("Expected error %v, got %v", tt.err, calls[0])
			}
		})
	}
}

func TestProgressReporterConcurrentAccess(t *testing.T) {
	reporter := &mockProgressReporter{
		reportErrorFunc: func(_ context.Context, _ error, _ string) error {
			return nil
		},
		shouldContinueFunc: func(_ error) bool {
			return true
		},
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	iterations := 100

	// Test concurrent ReportError calls
	wg.Add(iterations)
	for i := range iterations {
		go func(i int) {
			defer wg.Done()
			err := reporter.ReportError(ctx, fmt.Errorf("error"), "message")
			if err != nil {
				t.Errorf("Unexpected error in iteration %d: %v", i, err)
			}
		}(i)
	}

	// Test concurrent ShouldContinue calls
	wg.Add(iterations)
	for range iterations {
		go func() {
			defer wg.Done()
			_ = reporter.ShouldContinue(fmt.Errorf("error"))
		}()
	}

	wg.Wait()

	// Verify all calls were recorded
	reportCalls := reporter.getReportErrorCalls()
	if len(reportCalls) != iterations {
		t.Errorf("Expected %d ReportError calls, got %d", iterations, len(reportCalls))
	}

	continueCalls := reporter.getShouldContinueCalls()
	if len(continueCalls) != iterations {
		t.Errorf("Expected %d ShouldContinue calls, got %d", iterations, len(continueCalls))
	}
}

func TestProgressReporterWithContext(t *testing.T) {
	reporter := &mockProgressReporter{
		reportErrorFunc: func(ctx context.Context, _ error, _ string) error {
			// Check if context is canceled
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		},
	}

	t.Run("with active context", func(t *testing.T) {
		ctx := context.Background()
		err := reporter.ReportError(ctx, fmt.Errorf("test"), "Test message")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := reporter.ReportError(ctx, fmt.Errorf("test"), "Test message")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	})
}

func TestProgressReporterErrorCategories(t *testing.T) {
	// Test that the reporter can handle different error categories
	errorCategories := []struct {
		name           string
		err            error
		shouldContinue bool
	}{
		{
			name:           "network error",
			err:            fmt.Errorf("connection timeout"),
			shouldContinue: true,
		},
		{
			name:           "authentication error",
			err:            fmt.Errorf("invalid credentials"),
			shouldContinue: false,
		},
		{
			name:           "permission error",
			err:            fmt.Errorf("access denied"),
			shouldContinue: false,
		},
		{
			name:           "temporary error",
			err:            fmt.Errorf("service temporarily unavailable"),
			shouldContinue: true,
		},
	}

	reporter := &mockProgressReporter{
		shouldContinueFunc: func(err error) bool {
			if err == nil {
				return true
			}
			// Simple heuristic for testing
			msg := err.Error()
			return msg == "connection timeout" || msg == "service temporarily unavailable"
		},
	}

	for _, tc := range errorCategories {
		t.Run(tc.name, func(t *testing.T) {
			result := reporter.ShouldContinue(tc.err)
			if result != tc.shouldContinue {
				t.Errorf("For error %q, expected ShouldContinue=%v, got %v",
					tc.err, tc.shouldContinue, result)
			}
		})
	}
}
