package queue

import (
	"context"
	"fmt"
	"strconv"
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
	Manager     *Manager
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

	// Create the Manager
	manager := NewManager(sysCtx)
	// Start the manager
	go manager.Start(sysCtx)

	// Create worker pool configuration
	poolConfig := PoolConfig{
		InitialSize:  config.WorkerPoolSize,
		MinSize:      config.MinWorkers,
		MaxSize:      config.MaxWorkers,
		LLM:          config.LLM,
		Messenger:    config.Messenger,
		QueueManager: manager,
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
		Manager:     manager,
		WorkerPool:  workerPool,
		RateLimiter: rateLimiter,
		cancel:      sysCancel,
	}

	return system, nil
}

// GetDetailedStats returns comprehensive queue system statistics.
func (qs *System) GetDetailedStats() DetailedStats {
	// For now, return empty stats as Manager doesn't have detailed stats
	return DetailedStats{}
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
	// Shutdown the manager with a timeout
	if err := qs.Manager.Shutdown(defaultShutdownTimeout); err != nil {
		return fmt.Errorf("failed to stop manager: %w", err)
	}

	// Cancel the context to signal shutdown
	qs.cancel()

	// Stop the worker pool
	qs.WorkerPool.Stop(context.Background())

	// Wait for workers to finish processing
	qs.WorkerPool.Wait()

	return nil
}

// Enqueue adds a message to the queue.
func (qs *System) Enqueue(msg signal.IncomingMessage) error {
	// Generate ID from timestamp and sender
	msgID := strconv.FormatInt(msg.Timestamp.UnixNano(), 10) + "-" + msg.From
	conversationID := msg.From // Use sender as conversation ID

	// Create internal Message from IncomingMessage
	internalMsg := NewMessage(msgID, conversationID, msg.From, msg.FromNumber, msg.Text)

	// Submit to manager
	return qs.Manager.Submit(internalMsg)
}

// Stats returns queue system statistics.
func (qs *System) Stats() Stats {
	// Get basic stats from manager
	managerStats := qs.Manager.Stats()

	// Build Stats struct
	stats := Stats{
		TotalQueued:       managerStats["queued"],
		TotalProcessing:   managerStats["processing"],
		ConversationCount: managerStats["conversations"],
		ActiveWorkers:     qs.WorkerPool.Size(),
		HealthyWorkers:    qs.WorkerPool.Size(), // All active workers are considered healthy
		// Note: Other fields like TotalCompleted, TotalFailed, timing stats are not available from Manager
	}

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
