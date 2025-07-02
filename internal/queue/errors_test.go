package queue_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
)

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrRateLimited sentinel",
			err:      queue.ErrRateLimited,
			expected: true,
		},
		{
			name:     "RateLimitError type",
			err:      &queue.RateLimitError{Message: "rate limited", RetryAfter: 30 * time.Second},
			expected: true,
		},
		{
			name:     "wrapped RateLimitError",
			err:      fmt.Errorf("failed to query: %w", &queue.RateLimitError{Message: "rate limited"}),
			expected: true,
		},
		{
			name:     "error with 'rate limit' text",
			err:      fmt.Errorf("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "error with 'rate-limit' text",
			err:      fmt.Errorf("rate-limit: too many requests"),
			expected: true,
		},
		{
			name:     "error with '429' text",
			err:      fmt.Errorf("HTTP 429: Too Many Requests"),
			expected: true,
		},
		{
			name:     "error with 'too many requests' text",
			err:      fmt.Errorf("too many requests, please try again later"),
			expected: true,
		},
		{
			name:     "error with 'quota exceeded' text",
			err:      fmt.Errorf("API quota exceeded"),
			expected: true,
		},
		{
			name:     "error with 'throttled' text",
			err:      fmt.Errorf("request throttled"),
			expected: true,
		},
		{
			name:     "error with 'slow down' text",
			err:      fmt.Errorf("please slow down your requests"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      fmt.Errorf("connection timeout"),
			expected: false,
		},
		{
			name:     "case insensitive check",
			err:      fmt.Errorf("RATE LIMIT EXCEEDED"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := queue.IsRateLimitError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRateLimitError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestExtractRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected time.Duration
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: 0,
		},
		{
			name:     "RateLimitError with RetryAfter",
			err:      &queue.RateLimitError{Message: "rate limited", RetryAfter: 60 * time.Second},
			expected: 60 * time.Second,
		},
		{
			name:     "RateLimitError without RetryAfter",
			err:      &queue.RateLimitError{Message: "rate limited"},
			expected: 0,
		},
		{
			name:     "wrapped RateLimitError",
			err:      fmt.Errorf("failed: %w", &queue.RateLimitError{RetryAfter: 30 * time.Second}),
			expected: 30 * time.Second,
		},
		{
			name:     "other error types",
			err:      fmt.Errorf("rate limit exceeded"),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := queue.ExtractRetryAfter(tt.err)
			if result != tt.expected {
				t.Errorf("ExtractRetryAfter(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	baseErr := fmt.Errorf("underlying error")
	err := queue.NewRateLimitError("API limit reached", 45*time.Second, baseErr)

	// Test Error() method
	expectedMsg := "rate limited: retry after 45s: API limit reached"
	if err.Error() != expectedMsg {
		t.Errorf("Error() = %q, want %q", err.Error(), expectedMsg)
	}

	// Test Unwrap()
	if !errors.Is(err, baseErr) {
		t.Errorf("Unwrap() did not return the base error")
	}

	// Test without RetryAfter
	err2 := queue.NewRateLimitError("API limit", 0, nil)
	expectedMsg2 := "rate limited: API limit"
	if err2.Error() != expectedMsg2 {
		t.Errorf("Error() = %q, want %q", err2.Error(), expectedMsg2)
	}
}
