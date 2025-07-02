package queue

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Rate limiter constants.
const (
	// tickerCheckMultiplier is how many times to check per refill period.
	tickerCheckMultiplier = 10
	// defaultBurstSize is the default burst size for rate limiter.
	defaultBurstSize = 5
)

// TokenBucket represents a token bucket for rate limiting.
type TokenBucket struct {
	lastRefill   time.Time
	refillPeriod time.Duration
	capacity     int
	tokens       int
	refillRate   int
	mu           sync.Mutex
}

// NewTokenBucket creates a new token bucket.
func NewTokenBucket(capacity, refillRate int, refillPeriod time.Duration) *TokenBucket {
	return &TokenBucket{
		capacity:     capacity,
		tokens:       capacity, // Start full
		refillRate:   refillRate,
		refillPeriod: refillPeriod,
		lastRefill:   time.Now(),
	}
}

// Allow tries to consume a token, returns true if successful.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// Wait blocks until a token is available or context is canceled.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	// Fast path - check if token available
	if tb.Allow() {
		return nil
	}

	// Slow path - wait for token
	ticker := time.NewTicker(tb.refillPeriod / tickerCheckMultiplier) // Check 10x per refill period
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for rate limit: %w", ctx.Err())
		case <-ticker.C:
			if tb.Allow() {
				return nil
			}
		}
	}
}

// refill adds tokens based on elapsed time.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// How many refill periods have passed?
	periods := int(elapsed / tb.refillPeriod)
	if periods <= 0 {
		return
	}

	// Add tokens
	tokensToAdd := periods * tb.refillRate
	tb.tokens += tokensToAdd

	// Cap at capacity
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	// Update last refill time
	tb.lastRefill = tb.lastRefill.Add(time.Duration(periods) * tb.refillPeriod)
}

// RateLimiterImpl implements the RateLimiter interface using token buckets.
type RateLimiterImpl struct {
	buckets      map[string]*TokenBucket
	capacity     int
	refillRate   int
	refillPeriod time.Duration
	mu           sync.RWMutex
}

// NewRateLimiter creates a new rate limiter with per-conversation token buckets.
func NewRateLimiter(capacity, refillRate int, refillPeriod time.Duration) *RateLimiterImpl {
	return &RateLimiterImpl{
		buckets:      make(map[string]*TokenBucket),
		capacity:     capacity,
		refillRate:   refillRate,
		refillPeriod: refillPeriod,
	}
}

// Allow checks if the conversation can process a message now.
func (rl *RateLimiterImpl) Allow(conversationID string) bool {
	rl.mu.Lock()
	bucket, exists := rl.buckets[conversationID]
	if !exists {
		bucket = NewTokenBucket(rl.capacity, rl.refillRate, rl.refillPeriod)
		rl.buckets[conversationID] = bucket
	}
	rl.mu.Unlock()

	return bucket.Allow()
}

// Wait blocks until the conversation can process a message.
func (rl *RateLimiterImpl) Wait(ctx context.Context, conversationID string) error {
	rl.mu.Lock()
	bucket, exists := rl.buckets[conversationID]
	if !exists {
		bucket = NewTokenBucket(rl.capacity, rl.refillRate, rl.refillPeriod)
		rl.buckets[conversationID] = bucket
	}
	rl.mu.Unlock()

	return bucket.Wait(ctx)
}

// Record marks a conversation as active.
func (rl *RateLimiterImpl) Record(_ string) {
	// This implementation uses token buckets, so recording happens via Allow()
	// This method exists to satisfy the interface but doesn't need to do anything
}

// CleanupStale removes token buckets that haven't been used recently.
func (rl *RateLimiterImpl) CleanupStale(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	for convID, bucket := range rl.buckets {
		bucket.mu.Lock()
		if bucket.lastRefill.Before(cutoff) {
			delete(rl.buckets, convID)
		}
		bucket.mu.Unlock()
	}
}

// Stats returns rate limiter statistics.
func (rl *RateLimiterImpl) Stats() map[string]any {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	stats := make(map[string]any)
	stats["conversations"] = len(rl.buckets)

	totalTokens := 0
	for _, bucket := range rl.buckets {
		bucket.mu.Lock()
		bucket.refill()
		totalTokens += bucket.tokens
		bucket.mu.Unlock()
	}
	stats["total_tokens"] = totalTokens

	return stats
}

// DefaultRateLimiter creates a rate limiter with sensible defaults.
// Allows burst of 5 messages, then 1 message per second.
func DefaultRateLimiter() *RateLimiterImpl {
	return NewRateLimiter(defaultBurstSize, 1, time.Second)
}
