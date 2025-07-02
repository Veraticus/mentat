package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// Default configuration values.
const (
	// defaultHealthCheckPeriod is the default health check interval.
	defaultHealthCheckPeriod = 30 * time.Second
	// defaultUnhealthyThreshold is the default unhealthy threshold duration.
	defaultUnhealthyThreshold = 2 * time.Minute
	// defaultShutdownTimeout is the default shutdown timeout.
	defaultShutdownTimeout = 30 * time.Second
)

// SystemConfig holds configuration for the entire queue system.
type SystemConfig struct {
	// Worker pool configuration
	WorkerPoolSize     int
	MinWorkers         int
	MaxWorkers         int
	HealthCheckPeriod  time.Duration
	UnhealthyThreshold time.Duration

	// Rate limiting configuration
	RateLimitPerMinute int
	BurstLimit         int
	RateLimitWindow    time.Duration

	// Queue configuration
	ShutdownTimeout time.Duration

	// Dependencies
	LLM       claude.LLM
	Messenger signal.Messenger
}

// System represents the integrated queue system with all components.
type System struct {
	Coordinator *Coordinator
	WorkerPool  *DynamicWorkerPool
	RateLimiter RateLimiter
	cancel      context.CancelFunc
}

// NewSystem creates a new integrated queue system with all components wired together.
func NewSystem(ctx context.Context, config SystemConfig) (*System, error) {
	// Validate configuration
	if err := validateSystemConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create context for the system
	sysCtx, sysCancel := context.WithCancel(ctx)

	// Create rate limiter
	// For a rate of X per minute with burst Y:
	// - capacity = burst limit (Y)
	// - refillRate = rate per period (X/periods_per_minute)
	// - refillPeriod = window/periods
	// We'll refill every second to get smooth rate limiting
	refillPeriod := time.Second
	refillsPerWindow := int(config.RateLimitWindow / refillPeriod)
	refillRate := config.RateLimitPerMinute / refillsPerWindow
	if refillRate < 1 {
		refillRate = 1
	}

	rateLimiter := NewRateLimiter(
		config.BurstLimit, // capacity (burst)
		refillRate,        // tokens per refill
		refillPeriod,      // refill every second
	)

	// Create the Coordinator (which creates the Manager internally)
	coordinator := NewCoordinator(sysCtx)
	if config.ShutdownTimeout > 0 {
		coordinator.shutdownTimeout = config.ShutdownTimeout
	}

	// Create worker pool configuration
	poolConfig := PoolConfig{
		InitialSize:  config.WorkerPoolSize,
		MinSize:      config.MinWorkers,
		MaxSize:      config.MaxWorkers,
		LLM:          config.LLM,
		Messenger:    config.Messenger,
		QueueManager: coordinator.manager,
		MessageQueue: coordinator, // Pass the coordinator for state updates
		RateLimiter:  rateLimiter,
	}

	// Create the dynamic worker pool with a custom worker factory
	workerPool, err := NewDynamicWorkerPool(ctx, poolConfig)
	if err != nil {
		sysCancel()
		return nil, fmt.Errorf("failed to create worker pool: %w", err)
	}

	// Create the integrated system
	system := &System{
		Coordinator: coordinator,
		WorkerPool:  workerPool,
		RateLimiter: rateLimiter,
		cancel:      sysCancel,
	}

	return system, nil
}

// GetDetailedStats returns comprehensive queue system statistics.
func (qs *System) GetDetailedStats() DetailedStats {
	// Update worker stats first
	active := qs.WorkerPool.Size()
	total := active

	// Safely convert to int32 with bounds checking
	const maxInt32 = int(^uint32(0) >> 1)

	// Clamp values to maxInt32 before conversion
	if active > maxInt32 {
		active = maxInt32
	}
	if total > maxInt32 {
		total = maxInt32
	}

	// Safe to convert now - values are guaranteed to fit in int32
	//nolint:gosec // Values are bounded by maxInt32 check above
	qs.Coordinator.statsCollector.UpdateWorkerCount(int32(active), int32(active), int32(total))

	return qs.Coordinator.GetDetailedStats()
}

// Start begins the queue system operation.
func (qs *System) Start(ctx context.Context) error {
	// Start the worker pool
	if err := qs.WorkerPool.Start(ctx); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the queue system.
func (qs *System) Stop() error {
	// Cancel the context to signal shutdown
	qs.cancel()

	// Stop the worker pool first
	qs.WorkerPool.Stop(context.Background())

	// Wait for workers to finish processing
	qs.WorkerPool.Wait()

	// Stop the coordinator (which stops the manager)
	if err := qs.Coordinator.Stop(); err != nil {
		return fmt.Errorf("failed to stop coordinator: %w", err)
	}

	return nil
}

// Enqueue adds a message to the queue.
func (qs *System) Enqueue(msg signal.IncomingMessage) error {
	return qs.Coordinator.Enqueue(msg)
}

// Stats returns queue system statistics.
func (qs *System) Stats() Stats {
	// Update worker stats in the StatsCollector
	active := qs.WorkerPool.Size()
	total := active // For now, total equals active

	// Safely convert to int32 with bounds checking
	const maxInt32 = int(^uint32(0) >> 1)

	// Clamp values to maxInt32 before conversion
	if active > maxInt32 {
		active = maxInt32
	}
	if total > maxInt32 {
		total = maxInt32
	}

	// Safe to convert now - values are guaranteed to fit in int32
	//nolint:gosec // Values are bounded by maxInt32 check above
	qs.Coordinator.statsCollector.UpdateWorkerCount(int32(active), int32(active), int32(total))

	stats := qs.Coordinator.Stats()

	// Add worker pool stats
	stats.ActiveWorkers = active
	stats.HealthyWorkers = active // All active workers are considered healthy

	return stats
}

// ScaleWorkers adjusts the number of workers in the pool.
func (qs *System) ScaleWorkers(delta int) error {
	if delta > 0 {
		return qs.WorkerPool.ScaleUp(delta)
	} else if delta < 0 {
		return qs.WorkerPool.ScaleDown(-delta)
	}
	return nil
}

// WorkerCount returns the current number of workers.
func (qs *System) WorkerCount() int {
	return qs.WorkerPool.Size()
}

// validateSystemConfig validates the system configuration.
func validateSystemConfig(config *SystemConfig) error {
	// Apply defaults
	if config.WorkerPoolSize == 0 {
		config.WorkerPoolSize = 3
	}
	if config.MinWorkers == 0 {
		config.MinWorkers = 1
	}
	if config.MaxWorkers == 0 {
		config.MaxWorkers = 10
	}
	if config.HealthCheckPeriod == 0 {
		config.HealthCheckPeriod = defaultHealthCheckPeriod
	}
	if config.UnhealthyThreshold == 0 {
		config.UnhealthyThreshold = defaultUnhealthyThreshold
	}
	if config.RateLimitPerMinute == 0 {
		config.RateLimitPerMinute = 10
	}
	if config.BurstLimit == 0 {
		config.BurstLimit = 3
	}
	if config.RateLimitWindow == 0 {
		config.RateLimitWindow = time.Minute
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = defaultShutdownTimeout
	}

	// Validate required dependencies
	if config.LLM == nil {
		return fmt.Errorf("LLM is required")
	}
	if config.Messenger == nil {
		return fmt.Errorf("messenger is required")
	}

	// Validate configuration consistency
	if config.MinWorkers > config.MaxWorkers {
		return fmt.Errorf("MinWorkers (%d) cannot be greater than MaxWorkers (%d)",
			config.MinWorkers, config.MaxWorkers)
	}
	if config.WorkerPoolSize < config.MinWorkers {
		config.WorkerPoolSize = config.MinWorkers
	}
	if config.WorkerPoolSize > config.MaxWorkers {
		config.WorkerPoolSize = config.MaxWorkers
	}

	return nil
}
