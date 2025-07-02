package queue

import (
	"time"
)

// Retry constants.
const (
	// jitterDivisor is used to calculate jitter (10% jitter).
	jitterDivisor = 10
	// halfDivisor is used to divide values by 2.
	halfDivisor = 2
)

// CalculateRetryDelay calculates exponential backoff for retries.
func CalculateRetryDelay(attempts int) time.Duration {
	const (
		baseDelay = time.Second
		maxDelay  = 5 * time.Minute
		maxShift  = 30 // Prevent overflow in bit shifting
	)

	// Handle edge cases
	if attempts <= 0 {
		return baseDelay
	}

	// Use multiplication instead of bit shifting to avoid gosec warnings
	delay := baseDelay
	for i := 0; i < attempts && i < maxShift; i++ {
		delay *= 2
		if delay > maxDelay {
			return maxDelay
		}
	}

	// Add 10% jitter
	jitterRange := delay / jitterDivisor
	if jitterRange > 0 {
		// Use modulo to get a value within jitter range
		jitter := time.Duration(time.Now().UnixNano() % int64(jitterRange))
		// Center the jitter around 0 by subtracting half the range
		halfJitterRange := jitterRange / halfDivisor
		delay += jitter - halfJitterRange
	}

	return delay
}
