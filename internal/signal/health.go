package signal

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// defaultCheckInterval is the default health check interval.
	defaultCheckInterval = 30 * time.Second
	// defaultMaxBackoff is the maximum backoff duration for restarts.
	defaultMaxBackoff = 5 * time.Minute
	// healthCheckTimeout is the timeout for individual health checks.
	healthCheckTimeout = 10 * time.Second
	// restartTimeout is the timeout for restart operations.
	restartTimeout = 30 * time.Second
	// shutdownWaitTime is the wait time after stopping before restarting.
	shutdownWaitTime = 2 * time.Second
	// maxRestartAttempts is the number of attempts before alerting.
	maxRestartAttempts = 5
)

// HealthMonitor monitors Signal health and manages restarts.
type HealthMonitor struct {
	manager         Manager
	logger          *slog.Logger
	checkInterval   time.Duration
	maxBackoff      time.Duration
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	restartAttempts int
	mu              sync.Mutex
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(manager Manager, logger *slog.Logger) *HealthMonitor {
	return &HealthMonitor{
		manager:       manager,
		logger:        logger,
		checkInterval: defaultCheckInterval,
		maxBackoff:    defaultMaxBackoff,
	}
}

// Start begins health monitoring in a background goroutine.
func (h *HealthMonitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.cancel = cancel
	h.mu.Unlock()

	h.wg.Add(1)
	go h.monitor(ctx)
}

// Stop gracefully shuts down the health monitor.
func (h *HealthMonitor) Stop() {
	h.mu.Lock()
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Unlock()
	h.wg.Wait()
}

// monitor runs the main health check loop.
func (h *HealthMonitor) monitor(ctx context.Context) {
	defer h.wg.Done()

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	// Initial health check
	h.performHealthCheck(ctx)

	for {
		select {
		case <-ctx.Done():
			h.logger.InfoContext(ctx, "Health monitor stopping")
			return
		case <-ticker.C:
			h.performHealthCheck(ctx)
		}
	}
}

// performHealthCheck executes a health check and handles failures.
func (h *HealthMonitor) performHealthCheck(ctx context.Context) {
	healthCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	err := h.manager.HealthCheck(healthCtx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Health check failed", "error", err)
		h.handleFailure(ctx)
	} else {
		// Reset restart attempts on successful check
		h.mu.Lock()
		h.restartAttempts = 0
		h.mu.Unlock()
		h.logger.DebugContext(ctx, "Health check passed")
	}
}

// handleFailure manages restart attempts with exponential backoff.
func (h *HealthMonitor) handleFailure(ctx context.Context) {
	h.mu.Lock()
	attempts := h.restartAttempts
	h.restartAttempts++
	h.mu.Unlock()

	// Calculate backoff duration
	backoff := h.calculateBackoff(attempts)

	h.logger.InfoContext(ctx, "Scheduling Signal restart",
		"attempt", attempts+1,
		"backoff", backoff)

	// Use a timer channel for backoff delay
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		h.attemptRestart(ctx)
	}
}

// calculateBackoff returns exponential backoff duration.
func (h *HealthMonitor) calculateBackoff(attempts int) time.Duration {
	if attempts == 0 {
		return time.Second
	}

	backoff := time.Second * time.Duration(1<<attempts) // 2^attempts seconds
	if backoff > h.maxBackoff {
		backoff = h.maxBackoff
	}

	return backoff
}

// attemptRestart tries to restart the Signal service.
func (h *HealthMonitor) attemptRestart(ctx context.Context) {
	h.logger.InfoContext(ctx, "Attempting Signal restart")

	// First, ensure it's stopped
	restartCtx, cancel := context.WithTimeout(ctx, restartTimeout)
	defer cancel()

	// Stop the service
	if err := h.manager.Stop(restartCtx); err != nil {
		h.logger.ErrorContext(restartCtx, "Failed to stop Signal", "error", err)
	}

	// Wait a moment for clean shutdown
	select {
	case <-ctx.Done():
		return
	case <-time.After(shutdownWaitTime):
	}

	// Start the service
	if err := h.manager.Start(restartCtx); err != nil {
		h.logger.ErrorContext(restartCtx, "Failed to restart Signal", "error", err)

		// Alert on repeated failures
		h.mu.Lock()
		attempts := h.restartAttempts
		h.mu.Unlock()

		if attempts >= maxRestartAttempts {
			h.logger.ErrorContext(restartCtx, "Signal restart failed multiple times",
				"attempts", attempts,
				"alert", "manual intervention required")
		}
	} else {
		h.logger.InfoContext(restartCtx, "Signal restarted successfully")
	}
}

// GetRestartAttempts returns the current restart attempt count.
func (h *HealthMonitor) GetRestartAttempts() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.restartAttempts
}
