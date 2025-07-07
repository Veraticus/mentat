package conversation_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/conversation"
)

func TestCleanupService_Start(t *testing.T) {
	manager := conversation.NewManager(100 * time.Millisecond) // Short window for testing
	service := conversation.NewCleanupServiceWithInterval(manager, 50*time.Millisecond)

	ctx := context.Background()

	// Test starting the service
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cleanup service: %v", err)
	}

	if !service.IsRunning() {
		t.Error("Service should be running after Start")
	}

	// Test starting again (should not error)
	err = service.Start(ctx)
	if err != nil {
		t.Fatalf("Starting already running service should not error: %v", err)
	}

	// Stop the service
	service.Stop()

	if service.IsRunning() {
		t.Error("Service should not be running after Stop")
	}
}

func TestCleanupService_PeriodicCleanup(t *testing.T) {
	// Create manager with very short session window
	manager := conversation.NewManager(100 * time.Millisecond)
	service := conversation.NewCleanupServiceWithInterval(manager, 50*time.Millisecond)

	// Create some sessions
	session1 := manager.GetOrCreateSession("user1")
	session2 := manager.GetOrCreateSession("user2")
	session3 := manager.GetOrCreateSession("user3")

	// Verify sessions were created
	if session1 == "" || session2 == "" || session3 == "" {
		t.Fatal("Failed to create sessions")
	}

	// Start cleanup service
	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cleanup service: %v", err)
	}

	// Wait for sessions to expire and be cleaned up
	<-time.After(250 * time.Millisecond)

	// Check that expired sessions were removed
	stats := manager.Stats()
	if stats["total"] != 0 {
		t.Errorf("Expected 0 total sessions after cleanup, got %d", stats["total"])
	}

	// Stop the service
	service.Stop()
}

func TestCleanupService_StopDuringCleanup(t *testing.T) {
	manager := conversation.NewManager(1 * time.Hour) // Long window so sessions don't expire naturally
	service := conversation.NewCleanupServiceWithInterval(manager, 10*time.Millisecond)

	// Create many sessions
	for i := range 100 {
		manager.GetOrCreateSession(string(rune(i)))
	}

	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cleanup service: %v", err)
	}

	// Stop quickly
	time.Sleep(5 * time.Millisecond)
	service.Stop()

	// Verify it's stopped
	if service.IsRunning() {
		t.Error("Service should not be running after Stop")
	}

	// Try to stop again (should not panic)
	service.Stop()
}

func TestCleanupService_ConcurrentAccess(t *testing.T) {
	manager := conversation.NewManager(50 * time.Millisecond)
	service := conversation.NewCleanupServiceWithInterval(manager, 10*time.Millisecond)

	ctx := context.Background()

	var wg sync.WaitGroup

	// Start multiple goroutines that start/stop the service
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range 5 {
				if id%2 == 0 {
					_ = service.Start(ctx)
					<-time.After(5 * time.Millisecond)
				} else {
					service.Stop()
					<-time.After(5 * time.Millisecond)
				}
			}
		}(i)
	}

	// Create sessions concurrently
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range 10 {
				manager.GetOrCreateSession(string(rune(id*10 + j)))
				<-time.After(2 * time.Millisecond)
			}
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()

	// Final stop
	service.Stop()

	// Should not panic and service should be stopped
	if service.IsRunning() {
		t.Error("Service should not be running after final Stop")
	}
}

func TestCleanupService_ContextCancellation(t *testing.T) {
	manager := conversation.NewManager(1 * time.Hour) // Long window
	service := conversation.NewCleanupServiceWithInterval(manager, 10*time.Millisecond)

	// Create cancelable context
	ctx, cancel := context.WithCancel(context.Background())

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cleanup service: %v", err)
	}

	// Cancel the context
	cancel()

	// Give it time to stop
	<-time.After(50 * time.Millisecond)

	// Service should stop on its own due to context cancellation
	if service.IsRunning() {
		t.Error("Service should stop when context is canceled")
	}

	// Explicit stop should still work
	service.Stop()
}

func TestCleanupService_NoMemoryLeak(t *testing.T) {
	// This test verifies that the cleanup service doesn't leak memory
	// by ensuring goroutines are properly cleaned up

	manager := conversation.NewManager(50 * time.Millisecond)
	service := conversation.NewCleanupServiceWithInterval(manager, 10*time.Millisecond)

	// Count goroutines before
	initialGoroutines := runtime.NumGoroutine()

	ctx := context.Background()

	// Start and stop the service multiple times
	for range 5 {
		err := service.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start cleanup service: %v", err)
		}

		// Create some sessions
		for j := range 10 {
			manager.GetOrCreateSession(string(rune(j)))
		}

		<-time.After(30 * time.Millisecond)
		service.Stop()
	}

	// Give goroutines time to fully exit
	<-time.After(100 * time.Millisecond)

	// Count goroutines after
	finalGoroutines := runtime.NumGoroutine()

	// Allow for some variance but should be close to initial count
	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("Possible goroutine leak: started with %d, ended with %d goroutines",
			initialGoroutines, finalGoroutines)
	}
}

func TestCleanupService_ManagerIntegration(t *testing.T) {
	// Test that cleanup service properly integrates with manager's cleanup method
	manager := conversation.NewManager(100 * time.Millisecond)
	service := conversation.NewCleanupServiceWithInterval(manager, 50*time.Millisecond)

	// Add sessions with messages
	session1 := manager.GetOrCreateSession("user1")
	manager.AddMessage(session1, conversation.Message{
		ID:        "msg1",
		SessionID: session1,
		From:      "user1",
		Text:      "Hello",
		Timestamp: time.Now(),
	})

	session2 := manager.GetOrCreateSession("user2")
	manager.AddMessage(session2, conversation.Message{
		ID:        "msg2",
		SessionID: session2,
		From:      "user2",
		Text:      "Hi",
		Timestamp: time.Now(),
	})

	// Verify sessions exist
	history1 := manager.GetSessionHistory(session1)
	history2 := manager.GetSessionHistory(session2)
	if len(history1) != 1 || len(history2) != 1 {
		t.Fatal("Messages not properly added to sessions")
	}

	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cleanup service: %v", err)
	}

	// Wait for cleanup to run
	<-time.After(200 * time.Millisecond)

	// Sessions should be expired and cleaned up
	history1After := manager.GetSessionHistory(session1)
	history2After := manager.GetSessionHistory(session2)
	if history1After != nil || history2After != nil {
		t.Error("Expired sessions should have been cleaned up")
	}

	service.Stop()
}
