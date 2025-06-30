package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"
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
			err:      ErrRateLimited,
			expected: true,
		},
		{
			name:     "RateLimitError type",
			err:      &RateLimitError{Message: "rate limited", RetryAfter: 30 * time.Second},
			expected: true,
		},
		{
			name:     "wrapped RateLimitError",
			err:      fmt.Errorf("failed to query: %w", &RateLimitError{Message: "rate limited"}),
			expected: true,
		},
		{
			name:     "error with 'rate limit' text",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "error with 'rate-limit' text",
			err:      errors.New("rate-limit: too many requests"),
			expected: true,
		},
		{
			name:     "error with '429' text",
			err:      errors.New("HTTP 429: Too Many Requests"),
			expected: true,
		},
		{
			name:     "error with 'too many requests' text",
			err:      errors.New("too many requests, please try again later"),
			expected: true,
		},
		{
			name:     "error with 'quota exceeded' text",
			err:      errors.New("API quota exceeded"),
			expected: true,
		},
		{
			name:     "error with 'throttled' text",
			err:      errors.New("request throttled"),
			expected: true,
		},
		{
			name:     "error with 'slow down' text",
			err:      errors.New("please slow down your requests"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "case insensitive check",
			err:      errors.New("RATE LIMIT EXCEEDED"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRateLimitError(tt.err)
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
			err:      &RateLimitError{Message: "rate limited", RetryAfter: 60 * time.Second},
			expected: 60 * time.Second,
		},
		{
			name:     "RateLimitError without RetryAfter",
			err:      &RateLimitError{Message: "rate limited"},
			expected: 0,
		},
		{
			name:     "wrapped RateLimitError",
			err:      fmt.Errorf("failed: %w", &RateLimitError{RetryAfter: 30 * time.Second}),
			expected: 30 * time.Second,
		},
		{
			name:     "other error types",
			err:      errors.New("rate limit exceeded"),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractRetryAfter(tt.err)
			if result != tt.expected {
				t.Errorf("ExtractRetryAfter(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	baseErr := errors.New("underlying error")
	err := NewRateLimitError("API limit reached", 45*time.Second, baseErr)

	// Test Error() method
	expectedMsg := "rate limited: retry after 45s: API limit reached"
	if err.Error() != expectedMsg {
		t.Errorf("Error() = %q, want %q", err.Error(), expectedMsg)
	}

	// Test Unwrap()
	if errors.Unwrap(err) != baseErr {
		t.Errorf("Unwrap() did not return the base error")
	}

	// Test without RetryAfter
	err2 := NewRateLimitError("API limit", 0, nil)
	expectedMsg2 := "rate limited: API limit"
	if err2.Error() != expectedMsg2 {
		t.Errorf("Error() = %q, want %q", err2.Error(), expectedMsg2)
	}
}