package queue_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
)

func TestTokenBucket_Allow(t *testing.T) {
	// Create bucket with capacity 3, refill 1 token per 100ms
	tb := queue.NewTokenBucket(3, 1, 100*time.Millisecond)

	// Should allow 3 requests immediately (full bucket)
	for i := range 3 {
		if !tb.Allow() {
			t.Errorf("Expected Allow() to return true for request %d", i+1)
		}
	}

	// 4th request should fail (bucket empty)
	if tb.Allow() {
		t.Error("Expected Allow() to return false when bucket empty")
	}

	// Wait for refill
	<-time.After(150 * time.Millisecond)

	// Should allow 1 more request after refill
	if !tb.Allow() {
		t.Error("Expected Allow() to return true after refill")
	}

	// Should fail again
	if tb.Allow() {
		t.Error("Expected Allow() to return false after consuming refilled token")
	}
}

func TestTokenBucket_Wait(t *testing.T) {
	// Create bucket with capacity 1, refill 1 token per 50ms
	tb := queue.NewTokenBucket(1, 1, 50*time.Millisecond)

	// First request should succeed immediately
	ctx := context.Background()
	start := time.Now()
	if err := tb.Wait(ctx); err != nil {
		t.Fatalf("First Wait() failed: %v", err)
	}
	duration := time.Since(start)
	if duration > 10*time.Millisecond {
		t.Errorf("First Wait() took too long: %v", duration)
	}

	// Second request should wait for refill
	start = time.Now()
	if err := tb.Wait(ctx); err != nil {
		t.Fatalf("Second Wait() failed: %v", err)
	}
	duration = time.Since(start)
	if duration < 40*time.Millisecond || duration > 100*time.Millisecond {
		t.Errorf("Second Wait() duration unexpected: %v", duration)
	}
}

func TestTokenBucket_WaitCancel(t *testing.T) {
	// Create bucket with no tokens and slow refill
	tb := queue.NewTokenBucket(1, 1, 10*time.Second)
	tb.Allow() // Consume the initial token

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start waiting in goroutine
	done := make(chan error, 1)
	go func() {
		done <- tb.Wait(ctx)
	}()

	// Cancel after short delay
	<-time.After(50 * time.Millisecond)
	cancel()

	// Should return context error
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Wait() did not return after context cancel")
	}
}

func TestTokenBucket_RefillCap(t *testing.T) {
	// Create bucket with capacity 2, refill 3 tokens per period
	tb := queue.NewTokenBucket(2, 3, 50*time.Millisecond)

	// Consume all tokens
	tb.Allow()
	tb.Allow()

	// Wait for refill
	<-time.After(60 * time.Millisecond)

	// Should only have 2 tokens (capped at capacity)
	count := 0
	for range 5 {
		if tb.Allow() {
			count++
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 tokens after refill, got %d", count)
	}
}

func TestRateLimiter_PerConversation(t *testing.T) {
	rl := queue.NewRateLimiter(2, 1, 50*time.Millisecond)

	// Each conversation should have its own bucket
	// Should allow 2 requests for conv1
	if !rl.Allow("conv1") {
		t.Error("Expected first allow for conv1")
	}
	if !rl.Allow("conv1") {
		t.Error("Expected second allow for conv1")
	}
	if rl.Allow("conv1") {
		t.Error("Expected conv1 to be rate limited")
	}

	// conv2 should still have tokens
	// Should allow 2 requests for conv2
	if !rl.Allow("conv2") {
		t.Error("Expected first allow for conv2")
	}
	if !rl.Allow("conv2") {
		t.Error("Expected second allow for conv2")
	}
	if rl.Allow("conv2") {
		t.Error("Expected conv2 to be rate limited")
	}
}

// TestRateLimiter_Wait is disabled as Wait is no longer part of the interface
// The rate limiting is now handled synchronously via Allow/Record
/*
func TestRateLimiter_Wait(t *testing.T) {
	rl := queue.NewRateLimiter(1, 1, 50*time.Millisecond)
	ctx := context.Background()

	// Use up token for conv1
	rl.Allow("conv1")

	// Wait should succeed after refill
	start := time.Now()
	if err := rl.Wait(ctx, "conv1"); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	duration := time.Since(start)

	if duration < 40*time.Millisecond || duration > 100*time.Millisecond {
		t.Errorf("Wait duration unexpected: %v", duration)
	}
}
*/

func TestRateLimiter_CleanupStale(t *testing.T) {
	rl := queue.NewRateLimiter(1, 1, 50*time.Millisecond)

	// Create buckets for multiple conversations
	rl.Allow("old1")
	rl.Allow("old2")
	<-time.After(100 * time.Millisecond)
	rl.Allow("new1")

	// Should have 3 buckets
	if count := queue.GetRateLimiterBucketCount(rl); count != 3 {
		t.Errorf("Expected 3 buckets, got %d", count)
	}

	// Cleanup buckets older than 80ms
	rl.CleanupStale(80 * time.Millisecond)

	// Should only have new1 left
	if count := queue.GetRateLimiterBucketCount(rl); count != 1 {
		t.Errorf("Expected 1 bucket after cleanup, got %d", count)
	}

	if !queue.RateLimiterHasBucket(rl, "new1") {
		t.Error("Expected new1 bucket to remain")
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	// Use smaller capacity to make rate limiting more apparent
	rl := queue.NewRateLimiter(5, 1, 100*time.Millisecond)

	var allowed int32
	var denied int32
	var wg sync.WaitGroup

	// Run 10 goroutines trying to consume tokens
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine tries 3 times quickly
			for range 3 {
				convID := fmt.Sprintf("conv%d", id%2) // 2 conversations

				if rl.Allow(convID) {
					atomic.AddInt32(&allowed, 1)
				} else {
					atomic.AddInt32(&denied, 1)
				}

				// Small delay between attempts
				<-time.After(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Should have rate limited some requests
	allowedCount := atomic.LoadInt32(&allowed)
	deniedCount := atomic.LoadInt32(&denied)
	total := allowedCount + deniedCount

	t.Logf("Allowed %d, Denied %d out of %d total requests", allowedCount, deniedCount, total)

	// With 2 conversations, 5 capacity each, we should allow at most 10 initially
	// Plus maybe a few more from refills during the test
	if allowedCount > 15 {
		t.Errorf("Too many requests allowed: %d (expected <= 15)", allowedCount)
	}

	if deniedCount < 5 {
		t.Errorf("Too few requests denied: %d (expected >= 5)", deniedCount)
	}
}

func TestRateLimiter_Stats(t *testing.T) {
	rl := queue.NewRateLimiter(3, 1, 50*time.Millisecond)

	// Create some buckets with different token counts
	rl.Allow("conv1") // 2 tokens left
	rl.Allow("conv2")
	rl.Allow("conv2") // 1 token left

	stats := rl.Stats()

	convCount, ok := stats["conversations"].(int)
	if !ok {
		t.Fatal("conversations stat is not an int")
	}
	if convCount != 2 {
		t.Errorf("Expected 2 conversations, got %v", stats["conversations"])
	}

	totalTokens, ok := stats["total_tokens"].(int)
	if !ok {
		t.Fatal("total_tokens stat is not an int")
	}
	if totalTokens != 3 { // 2 + 1
		t.Errorf("Expected 3 total tokens, got %d", totalTokens)
	}
}

func TestDefaultRateLimiter(t *testing.T) {
	rl := queue.DefaultRateLimiter()

	// Should allow burst of 5
	for i := range 5 {
		if !rl.Allow("test") {
			t.Errorf("Expected Allow() to return true for request %d", i+1)
		}
	}

	// 6th should fail
	if rl.Allow("test") {
		t.Error("Expected 6th request to be rate limited")
	}
}
