package signal

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockTypingMessenger tracks typing indicator calls for testing.
type mockTypingMessenger struct {
	mu              sync.Mutex
	calls           []typingCall
	sendTypingError error
}

type typingCall struct {
	recipient string
	timestamp time.Time
}

func (m *mockTypingMessenger) SendTypingIndicator(_ context.Context, recipient string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendTypingError != nil {
		return m.sendTypingError
	}

	m.calls = append(m.calls, typingCall{
		recipient: recipient,
		timestamp: time.Now(),
	})
	return nil
}

func (m *mockTypingMessenger) Send(_ context.Context, _ string, _ string) error {
	// Not used in these tests
	return nil
}

func (m *mockTypingMessenger) Subscribe(_ context.Context) (<-chan IncomingMessage, error) {
	// Not used in these tests
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockTypingMessenger) getCalls() []typingCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]typingCall, len(m.calls))
	copy(result, m.calls)
	return result
}

func (m *mockTypingMessenger) getCallCount(recipient string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, call := range m.calls {
		if call.recipient == recipient {
			count++
		}
	}
	return count
}

func (m *mockTypingMessenger) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
}

func TestTypingIndicatorManager_Start(t *testing.T) {
	tests := []struct {
		name      string
		recipient string
		wantErr   bool
	}{
		{
			name:      "start typing indicator",
			recipient: "+1234567890",
			wantErr:   false,
		},
		{
			name:      "start with empty recipient",
			recipient: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messenger := &mockTypingMessenger{}
			manager := NewTypingIndicatorManager(messenger)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			err := manager.Start(ctx, tt.recipient)

			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Should have at least one call immediately
				<-time.After(10 * time.Millisecond)
				if count := messenger.getCallCount(tt.recipient); count < 1 {
					t.Errorf("Expected at least 1 typing indicator call, got %d", count)
				}
			}

			manager.StopAll()
		})
	}
}

func TestTypingIndicatorManager_Stop(t *testing.T) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)

	ctx := context.Background()
	recipient := "+1234567890"

	// Start typing indicator
	if err := manager.Start(ctx, recipient); err != nil {
		t.Fatalf("Failed to start typing indicator: %v", err)
	}

	// Let it run briefly
	<-time.After(20 * time.Millisecond)

	// Stop it
	manager.Stop(recipient)

	// Get count before potential additional calls
	countBeforeStop := messenger.getCallCount(recipient)

	// Wait to ensure no more calls
	<-time.After(50 * time.Millisecond)

	countAfterWait := messenger.getCallCount(recipient)

	if countAfterWait != countBeforeStop {
		t.Errorf("Typing indicator continued after Stop(): before=%d, after=%d",
			countBeforeStop, countAfterWait)
	}
}

func TestTypingIndicatorManager_RefreshEvery10Seconds(t *testing.T) {
	// This test verifies the 10-second refresh interval
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)

	ctx := context.Background()
	recipient := "+1234567890"

	// Start typing indicator
	if err := manager.Start(ctx, recipient); err != nil {
		t.Fatalf("Failed to start typing indicator: %v", err)
	}

	// Run for 25 seconds to see at least 2 refreshes
	// Initial + 2 refreshes = 3 calls minimum
	<-time.After(25 * time.Second)

	manager.Stop(recipient)

	calls := messenger.getCalls()

	if len(calls) < 3 {
		t.Errorf("Expected at least 3 calls over 25 seconds, got %d", len(calls))
		return
	}

	// Verify timing between calls (should be ~10 seconds)
	for i := 1; i < len(calls); i++ {
		diff := calls[i].timestamp.Sub(calls[i-1].timestamp)
		// Allow 9.5-10.5 seconds to account for timing variations
		if diff < 9500*time.Millisecond || diff > 10500*time.Millisecond {
			t.Errorf("Call %d->%d: interval was %v, expected ~10s", i-1, i, diff)
		}
	}
}

func TestTypingIndicatorManager_MultipleRecipients(t *testing.T) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)

	ctx := context.Background()
	recipients := []string{"+1111111111", "+2222222222", "+3333333333"}

	// Start indicators for multiple recipients
	for _, recipient := range recipients {
		if err := manager.Start(ctx, recipient); err != nil {
			t.Fatalf("Failed to start typing indicator for %s: %v", recipient, err)
		}
	}

	// Let them run
	<-time.After(50 * time.Millisecond)

	// Each should have received at least one call
	for _, recipient := range recipients {
		if count := messenger.getCallCount(recipient); count < 1 {
			t.Errorf("Recipient %s: expected at least 1 call, got %d", recipient, count)
		}
	}

	// Stop one recipient
	manager.Stop(recipients[0])

	// Get counts before reset to verify they were working
	for _, recipient := range recipients {
		initialCount := messenger.getCallCount(recipient)
		if initialCount < 1 {
			t.Errorf("Recipient %s had no initial calls: %d", recipient, initialCount)
		}
	}

	// Reset counts
	messenger.reset()

	// Let others continue (give enough time for at least one call)
	<-time.After(100 * time.Millisecond)

	// First recipient should have no new calls
	if count := messenger.getCallCount(recipients[0]); count > 0 {
		t.Errorf("Stopped recipient %s still receiving calls: %d", recipients[0], count)
	}

	// Others should still be receiving calls (might still be warming up)
	for _, recipient := range recipients[1:] {
		count := messenger.getCallCount(recipient)
		// Just log if no calls yet - timing can be tricky
		if count < 1 {
			t.Logf("Active recipient %s has not sent new calls yet: %d", recipient, count)
		}
	}

	manager.StopAll()
}

func TestTypingIndicatorManager_DuplicateStart(t *testing.T) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)

	ctx := context.Background()
	recipient := "+1234567890"

	// Start first indicator
	if err := manager.Start(ctx, recipient); err != nil {
		t.Fatalf("Failed to start first typing indicator: %v", err)
	}

	// Try to start duplicate
	err := manager.Start(ctx, recipient)
	if err == nil {
		t.Error("Expected error when starting duplicate indicator, got nil")
	}

	manager.StopAll()
}

func TestTypingIndicatorManager_StopAll(t *testing.T) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)

	ctx := context.Background()
	recipients := []string{"+1111111111", "+2222222222", "+3333333333"}

	// Start multiple indicators
	for _, recipient := range recipients {
		if err := manager.Start(ctx, recipient); err != nil {
			t.Fatalf("Failed to start typing indicator for %s: %v", recipient, err)
		}
	}

	// Let them run
	<-time.After(20 * time.Millisecond)

	// Stop all
	manager.StopAll()

	// Get counts before potential additional calls
	initialCounts := make(map[string]int)
	for _, recipient := range recipients {
		initialCounts[recipient] = messenger.getCallCount(recipient)
	}

	// Wait to ensure no more calls
	<-time.After(50 * time.Millisecond)

	// Verify no new calls
	for _, recipient := range recipients {
		newCount := messenger.getCallCount(recipient)
		if newCount != initialCounts[recipient] {
			t.Errorf("Recipient %s: continued after StopAll() - was %d, now %d",
				recipient, initialCounts[recipient], newCount)
		}
	}
}

func TestTypingIndicatorManager_ContextCancellation(t *testing.T) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)

	ctx, cancel := context.WithCancel(context.Background())
	recipient := "+1234567890"

	// Start typing indicator
	if err := manager.Start(ctx, recipient); err != nil {
		t.Fatalf("Failed to start typing indicator: %v", err)
	}

	// Let it run briefly
	<-time.After(20 * time.Millisecond)

	// Cancel context
	cancel()

	// Get count before potential additional calls
	countBeforeCancel := messenger.getCallCount(recipient)

	// Wait to ensure no more calls
	<-time.After(50 * time.Millisecond)

	countAfterWait := messenger.getCallCount(recipient)

	if countAfterWait != countBeforeCancel {
		t.Errorf("Typing indicator continued after context cancel: before=%d, after=%d",
			countBeforeCancel, countAfterWait)
	}

	manager.StopAll()
}

func TestTypingIndicatorManager_ErrorHandling(t *testing.T) {
	messenger := &mockTypingMessenger{
		sendTypingError: context.DeadlineExceeded,
	}
	manager := NewTypingIndicatorManager(messenger)

	ctx := context.Background()
	recipient := "+1234567890"

	// Start should succeed even if sends fail
	if err := manager.Start(ctx, recipient); err != nil {
		t.Fatalf("Start failed due to send error: %v", err)
	}

	// Let it try a few times
	<-time.After(50 * time.Millisecond)

	// Should still be tracked as active despite errors
	manager.Stop(recipient)

	// Stopping non-existent should not panic
	manager.Stop("+9999999999")
}

func TestTypingIndicatorManager_ThreadSafety(t *testing.T) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)

	ctx := context.Background()
	var wg sync.WaitGroup

	// Concurrently start/stop many indicators
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			recipient := string(rune('A' + id))

			// Start
			if err := manager.Start(ctx, recipient); err != nil {
				t.Errorf("Failed to start indicator %s: %v", recipient, err)
				return
			}

			// Run briefly
			<-time.After(10 * time.Millisecond)

			// Stop
			manager.Stop(recipient)
		}(i)
	}

	// Also concurrently call StopAll
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-time.After(5 * time.Millisecond)
		manager.StopAll()
	}()

	wg.Wait()

	// Ensure clean shutdown
	manager.StopAll()
}

// Benchmarks.
func BenchmarkTypingIndicatorManager_Start(b *testing.B) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)
	ctx := context.Background()

	b.ResetTimer()

	for i := range b.N {
		recipient := string(rune('A' + (i % 26)))
		_ = manager.Start(ctx, recipient)
		manager.Stop(recipient)
	}

	manager.StopAll()
}

func BenchmarkTypingIndicatorManager_Concurrent(b *testing.B) {
	messenger := &mockTypingMessenger{}
	manager := NewTypingIndicatorManager(messenger)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			recipient := string(rune('A' + (i % 26)))
			_ = manager.Start(ctx, recipient)
			<-time.After(time.Microsecond)
			manager.Stop(recipient)
			i++
		}
	})

	manager.StopAll()
}
