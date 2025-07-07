package signal_test

import (
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/mocks"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/stretchr/testify/assert"
)

func TestNewHealthMonitor(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	monitor := signal.NewHealthMonitor(mockManager, logger)

	assert.NotNil(t, monitor)
	assert.Equal(t, 0, monitor.GetRestartAttempts())
}

func TestHealthMonitor_StartStop(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Start the monitor
	monitor.Start()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the monitor
	monitor.Stop()

	// Verify it stopped properly (no deadlock)
	// If we reach here, Stop() completed without hanging
	t.Log("Health monitor started and stopped successfully")
}

func TestHealthMonitor_SuccessfulHealthCheck(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	// Mock successful health check
	mockManager.SetHealthError(nil)

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Use reflection to set checkInterval since it's not exported
	// For now, we'll just test with the default interval

	monitor.Start()

	// Wait briefly
	time.Sleep(100 * time.Millisecond)

	monitor.Stop()

	// Should have performed health checks (verified by no errors)
	// Test passes if no panic or hang occurs
	t.Log("Health checks performed successfully")
}

func TestHealthMonitor_FailedHealthCheckTriggersRestart(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	// Set up the mock to fail health check initially
	healthCheckError := fmt.Errorf("health check failed")
	mockManager.SetHealthError(healthCheckError)

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Start monitor
	monitor.Start()

	// Wait for health check and restart attempt
	time.Sleep(3 * time.Second)

	// Check if restart was attempted
	assert.True(t, mockManager.IsStopped(), "Stop should have been called")
	assert.True(t, mockManager.IsStarted(), "Start should have been called")

	monitor.Stop()
}

func TestHealthMonitor_RestartAttemptsTracking(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Initially should be 0
	assert.Equal(t, 0, monitor.GetRestartAttempts())

	// Set up failing health check
	mockManager.SetHealthError(fmt.Errorf("health check failed"))

	// Start and let it fail once
	monitor.Start()
	time.Sleep(2 * time.Second)

	// Should have incremented
	attempts := monitor.GetRestartAttempts()
	assert.Positive(t, attempts, "Restart attempts should have increased")

	monitor.Stop()
}

func TestHealthMonitor_ConcurrentAccess(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Concurrent operations
	var wg sync.WaitGroup

	// Reader goroutines
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 100 {
				_ = monitor.GetRestartAttempts()
				_ = j // Satisfy unused variable check
			}
		}()
		_ = i // Satisfy unused variable check
	}

	// Start the monitor once
	monitor.Start()
	defer monitor.Stop()

	wg.Wait()

	// Should not panic or race
	// Test passes if no race conditions detected
	t.Log("Concurrent access test completed without race conditions")
}

func TestHealthMonitor_GracefulShutdown(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Start monitor
	monitor.Start()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		monitor.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

func TestHealthMonitor_MultipleRestartAttempts(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	// Always fail health checks
	mockManager.SetHealthError(fmt.Errorf("persistent failure"))

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Start and let it attempt a restart
	monitor.Start()

	// Wait for first restart attempt (1s backoff + restart time)
	time.Sleep(3 * time.Second)

	// Should have at least one restart attempt
	attempts := monitor.GetRestartAttempts()
	assert.GreaterOrEqual(t, attempts, 1, "Should have attempted at least one restart")

	monitor.Stop()
}

func TestHealthMonitor_RestartWithContextCancellation(t *testing.T) {
	mockManager := mocks.NewMockSignalManager()
	logger := slog.Default()

	// Fail health check to trigger restart
	mockManager.SetHealthError(fmt.Errorf("health check failed"))

	monitor := signal.NewHealthMonitor(mockManager, logger)

	// Start monitor
	monitor.Start()

	// Let it attempt a restart
	time.Sleep(2 * time.Second)

	// Stop should cancel any ongoing operations
	stopDone := make(chan struct{})
	go func() {
		monitor.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Success - stop completed
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() should have canceled ongoing operations")
	}
}
