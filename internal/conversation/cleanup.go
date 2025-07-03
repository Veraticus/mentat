package conversation

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultCleanupInterval is the default interval at which expired sessions are cleaned up.
	DefaultCleanupInterval = 1 * time.Minute
)

// CleanupService manages periodic cleanup of expired sessions.
type CleanupService struct {
	manager  *Manager
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewCleanupService creates a new cleanup service with default interval.
func NewCleanupService(manager *Manager) *CleanupService {
	return NewCleanupServiceWithInterval(manager, DefaultCleanupInterval)
}

// NewCleanupServiceWithInterval creates a new cleanup service with custom interval.
func NewCleanupServiceWithInterval(manager *Manager, interval time.Duration) *CleanupService {
	return &CleanupService{
		manager:  manager,
		interval: interval,
	}
}

// Start begins the periodic cleanup process.
func (c *CleanupService) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil // Already running
	}

	// Create cancelable context
	cleanupCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.running = true

	// Start cleanup goroutine
	go c.runCleanup(cleanupCtx)

	return nil
}

// Stop gracefully stops the cleanup service.
func (c *CleanupService) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}

	cancel := c.cancel
	done := c.done
	c.mu.Unlock()

	// Cancel the context
	if cancel != nil {
		cancel()
	}

	// Wait for cleanup goroutine to finish
	if done != nil {
		<-done
	}
}

// runCleanup performs periodic cleanup of expired sessions.
func (c *CleanupService) runCleanup(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		c.running = false
		if c.done != nil {
			close(c.done)
		}
		c.mu.Unlock()
	}()

	logger := slog.Default().With(
		slog.String("component", "conversation.cleanup"),
	)

	// Create ticker for periodic cleanup
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Run initial cleanup
	c.performCleanup(ctx, logger)

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "Cleanup service stopping")
			return

		case <-ticker.C:
			c.performCleanup(ctx, logger)
		}
	}
}

// performCleanup executes a single cleanup operation.
func (c *CleanupService) performCleanup(ctx context.Context, logger *slog.Logger) {
	startTime := time.Now()
	removed := c.manager.CleanupExpired()
	duration := time.Since(startTime)

	if removed > 0 {
		logger.InfoContext(ctx, "Cleaned up expired sessions",
			slog.Int("removed", removed),
			slog.Duration("duration", duration),
		)
	}

	// Also log current stats for monitoring
	stats := c.manager.Stats()
	logger.DebugContext(ctx, "Session stats after cleanup",
		slog.Int("total", stats["total"]),
		slog.Int("active", stats["active"]),
	)
}

// IsRunning returns whether the cleanup service is currently running.
func (c *CleanupService) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}
