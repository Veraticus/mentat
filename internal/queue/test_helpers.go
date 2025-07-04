package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// TestMockLLM implements the LLM interface for testing.
// This is exported so it can be used by the queue_test package.
type TestMockLLM struct {
	Err       error
	Response  string
	Queries   []string
	Delay     time.Duration
	Mu        sync.Mutex
	QueryFunc func(ctx context.Context, prompt string, sessionID string) (*claude.LLMResponse, error)
}

// Query implements the LLM interface for testing.
func (m *TestMockLLM) Query(ctx context.Context, prompt string, sessionID string) (*claude.LLMResponse, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, prompt, sessionID)
	}

	m.Mu.Lock()
	m.Queries = append(m.Queries, prompt)
	m.Mu.Unlock()

	if m.Delay > 0 {
		select {
		case <-time.After(m.Delay):
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during mock LLM delay: %w", ctx.Err())
		}
	}

	if m.Err != nil {
		return nil, m.Err
	}

	return &claude.LLMResponse{
		Message:  m.Response,
		Metadata: claude.ResponseMetadata{},
	}, nil
}

// TestMockMessenger implements the Messenger interface for testing.
// This is exported so it can be used by the queue_test package.
type TestMockMessenger struct {
	SendErr      error
	TypingErr    error
	IncomingCh   chan signal.IncomingMessage
	SentMessages []TestSentMessage
	Mu           sync.Mutex
	TypingCount  int32
	SendFunc     func(ctx context.Context, recipient string, message string) error
}

// TestSentMessage represents a sent message in tests.
type TestSentMessage struct {
	Recipient string
	Message   string
}

// Send implements the Messenger interface for testing.
func (m *TestMockMessenger) Send(ctx context.Context, recipient string, message string) error {
	m.Mu.Lock()
	sendFunc := m.SendFunc
	m.Mu.Unlock()

	if sendFunc != nil {
		return sendFunc(ctx, recipient, message)
	}

	m.Mu.Lock()
	m.SentMessages = append(m.SentMessages, TestSentMessage{recipient, message})
	m.Mu.Unlock()
	return m.SendErr
}

// Subscribe implements the Messenger interface for testing.
func (m *TestMockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	if m.IncomingCh == nil {
		m.IncomingCh = make(chan signal.IncomingMessage)
	}
	return m.IncomingCh, nil
}

// SendTypingIndicator implements the Messenger interface for testing.
func (m *TestMockMessenger) SendTypingIndicator(_ context.Context, _ string) error {
	atomic.AddInt32(&m.TypingCount, 1)
	return m.TypingErr
}

// GetRateLimiterBucketCount is a test helper to get the number of rate limiter buckets.
// This is exported for use in black-box tests.
func GetRateLimiterBucketCount(r RateLimiter) int {
	if rl, ok := r.(*RateLimiterImpl); ok {
		rl.mu.RLock()
		defer rl.mu.RUnlock()
		return len(rl.buckets)
	}
	return -1
}

// RateLimiterHasBucket is a test helper to check if a bucket exists.
// This is exported for use in black-box tests.
func RateLimiterHasBucket(r RateLimiter, conversationID string) bool {
	if rl, ok := r.(*RateLimiterImpl); ok {
		rl.mu.RLock()
		defer rl.mu.RUnlock()
		_, exists := rl.buckets[conversationID]
		return exists
	}
	return false
}
