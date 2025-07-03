package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// Pool constants.
const (
	// workerSizeMultiplier is used to calculate default max size from min size.
	workerSizeMultiplier = 2
	// commandChannelSize is the buffer size for pool commands.
	commandChannelSize = 10
	// commandTimeout is the timeout for sending/receiving commands.
	commandTimeout = 2 * time.Second
)

// PoolConfig holds configuration for the WorkerPool.
type PoolConfig struct {
	InitialSize  int
	MinSize      int
	MaxSize      int
	LLM          claude.LLM
	Messenger    signal.Messenger
	QueueManager *Manager
	MessageQueue MessageQueue // For state updates and stats
	RateLimiter  RateLimiter
	PanicHandler PanicHandler // Optional: defaults to logging with stack trace
}

// workerInfo holds information about a worker.
type workerInfo struct {
	worker Worker
}

// poolCommand represents commands sent to the pool manager.
type poolCommand struct {
	cmd      string // "add", "remove", "stop"
	count    int
	response chan error
}

// DynamicWorkerPool manages a pool of workers with dynamic sizing.
type DynamicWorkerPool struct {
	config       PoolConfig
	workers      map[string]*workerInfo
	typingMgr    signal.TypingIndicatorManager
	panicHandler PanicHandler

	// Channels for coordination
	commands   chan poolCommand
	workerDone chan string

	// Synchronization
	mu           sync.RWMutex // Protects workers map
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	workerCancel context.CancelFunc
	stopCh       chan struct{}

	// Metrics
	activeWorkers atomic.Int32
	nextWorkerID  atomic.Int32
}

// NewDynamicWorkerPool creates a new dynamic worker pool.
func NewDynamicWorkerPool(ctx context.Context, config PoolConfig) (*DynamicWorkerPool, error) {
	if config.InitialSize < 1 {
		return nil, fmt.Errorf("initial size must be at least 1")
	}
	if config.MinSize < 1 {
		config.MinSize = 1
	}
	if config.MaxSize < config.MinSize {
		config.MaxSize = config.MinSize * workerSizeMultiplier
	}
	if config.PanicHandler == nil {
		config.PanicHandler = NewDefaultPanicHandler()
	}

	_, cancel := context.WithCancel(context.Background())
	_, workerCancel := context.WithCancel(context.Background())

	pool := &DynamicWorkerPool{
		config:       config,
		workers:      make(map[string]*workerInfo),
		typingMgr:    signal.NewTypingIndicatorManager(config.Messenger),
		panicHandler: config.PanicHandler,
		commands:     make(chan poolCommand, commandChannelSize),
		workerDone:   make(chan string, config.MaxSize),
		cancel:       cancel,
		workerCancel: workerCancel,
		stopCh:       make(chan struct{}),
	}

	// Create initial workers
	for range config.InitialSize {
		pool.createWorker(ctx)
	}

	return pool, nil
}

// Start begins the worker pool operation.
func (p *DynamicWorkerPool) Start(ctx context.Context) error {
	// Start the pool manager goroutine
	p.wg.Add(1)
	go p.manage(ctx)

	// Start all initial workers
	p.mu.RLock()
	for id, wi := range p.workers {
		p.startWorker(ctx, id, wi.worker)
	}
	p.mu.RUnlock()

	return nil
}

// manage handles pool commands and worker lifecycle.
func (p *DynamicWorkerPool) manage(ctx context.Context) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return

		case cmd := <-p.commands:
			switch cmd.cmd {
			case "add":
				cmd.response <- p.addWorkers(ctx, cmd.count)
			case "remove":
				cmd.response <- p.removeWorkers(ctx, cmd.count)
			case "stop":
				p.stopAllWorkers(ctx)
				cmd.response <- nil
				return
			}

		case workerID := <-p.workerDone:
			p.handleWorkerDone(ctx, workerID)
		}
	}
}

// createWorker creates a new worker and adds it to the pool.
func (p *DynamicWorkerPool) createWorker(ctx context.Context) string {
	id := int(p.nextWorkerID.Add(1))

	config := WorkerConfig{
		ID:                 id,
		LLM:                p.config.LLM,
		Messenger:          p.config.Messenger,
		QueueManager:       p.config.QueueManager,
		MessageQueue:       p.config.MessageQueue,
		RateLimiter:        p.config.RateLimiter,
		TypingIndicatorMgr: p.typingMgr,
	}

	worker := NewWorker(config)
	workerID := worker.ID()

	p.mu.Lock()
	p.workers[workerID] = &workerInfo{
		worker: worker,
	}
	p.mu.Unlock()

	p.activeWorkers.Add(1)

	logger := slog.Default()
	logger.InfoContext(ctx, "Created worker", slog.String("worker_id", workerID))
	return workerID
}

// startWorker starts a worker in a new goroutine.
func (p *DynamicWorkerPool) startWorker(ctx context.Context, id string, worker Worker) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// Handle panic recovery and decide if worker should be replaced
		defer func() {
			if r := recover(); r != nil {
				// Panic occurred - let handler decide if we should replace
				if HandleRecoveredPanic(id, r, p.panicHandler) {
					select {
					case p.workerDone <- id:
					case <-p.stopCh:
					}
				}
			} else {
				// Normal exit - always replace worker
				select {
				case p.workerDone <- id:
				case <-p.stopCh:
				}
			}
		}()

		logger := slog.Default()
		logger.InfoContext(ctx, "Starting worker", slog.String("workerID", id))
		workerCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		if err := worker.Start(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(workerCtx, "Worker stopped with error",
				slog.String("workerID", id),
				slog.Any("error", err))
		}
	}()
}

// addWorkers adds the specified number of workers to the pool.
func (p *DynamicWorkerPool) addWorkers(ctx context.Context, count int) error {
	p.mu.RLock()
	currentSize := len(p.workers)
	p.mu.RUnlock()

	maxAdd := p.config.MaxSize - currentSize

	if maxAdd <= 0 {
		return fmt.Errorf("pool at maximum size (%d)", p.config.MaxSize)
	}

	if count > maxAdd {
		count = maxAdd
	}

	for range count {
		workerID := p.createWorker(ctx)

		// Start the new worker
		p.mu.RLock()
		wi, ok := p.workers[workerID]
		p.mu.RUnlock()

		if ok {
			p.startWorker(ctx, workerID, wi.worker)
		}
	}

	logger := slog.Default()
	p.mu.RLock()
	poolSize := len(p.workers)
	p.mu.RUnlock()
	logger.InfoContext(ctx, "Added workers", slog.Int("count", count), slog.Int("pool_size", poolSize))
	return nil
}

// removeWorkers removes the specified number of workers from the pool.
func (p *DynamicWorkerPool) removeWorkers(ctx context.Context, count int) error {
	p.mu.Lock()
	currentSize := len(p.workers)
	maxRemove := currentSize - p.config.MinSize

	if maxRemove <= 0 {
		p.mu.Unlock()
		return fmt.Errorf("pool at minimum size (%d)", p.config.MinSize)
	}

	if count > maxRemove {
		count = maxRemove
	}

	// Remove the least recently active workers
	removed := 0
	var toStop []*workerInfo
	var toRemove []string

	for id := range p.workers {
		if removed >= count {
			break
		}

		if wi, ok := p.workers[id]; ok {
			toStop = append(toStop, wi)
			toRemove = append(toRemove, id)
			delete(p.workers, id)
			p.activeWorkers.Add(-1)
			removed++
		}
	}
	poolSize := len(p.workers)
	p.mu.Unlock()

	// Stop workers outside the lock to avoid holding it during potentially blocking operations
	for i, wi := range toStop {
		if err := wi.worker.Stop(); err != nil {
			logger := slog.Default()
			logger.ErrorContext(ctx, "Failed to stop worker",
				slog.String("worker_id", toRemove[i]),
				slog.Any("error", err))
		}
		logger := slog.Default()
		logger.InfoContext(ctx, "Removed worker", slog.String("worker_id", toRemove[i]))
	}

	logger := slog.Default()
	logger.InfoContext(ctx, "Removed workers", slog.Int("count", removed), slog.Int("pool_size", poolSize))
	return nil
}

// handleWorkerDone handles a worker that has stopped.
func (p *DynamicWorkerPool) handleWorkerDone(ctx context.Context, workerID string) {
	p.mu.Lock()
	_, ok := p.workers[workerID]
	if !ok {
		p.mu.Unlock()
		return
	}

	delete(p.workers, workerID)
	p.activeWorkers.Add(-1)
	logger := slog.Default()
	logger.InfoContext(ctx, "Worker stopped", slog.String("worker_id", workerID))

	// Replace the worker if we're below minimum size
	if len(p.workers) >= p.config.MinSize {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	newWorkerID := p.createWorker(ctx)

	// Start the replacement worker
	p.mu.RLock()
	wi, ok := p.workers[newWorkerID]
	p.mu.RUnlock()

	if ok {
		p.startWorker(ctx, newWorkerID, wi.worker)
	}
}

// stopAllWorkers stops all workers in the pool.
func (p *DynamicWorkerPool) stopAllWorkers(ctx context.Context) {
	logger := slog.Default()

	p.mu.Lock()
	logger.InfoContext(ctx, "Stopping all workers", slog.Int("count", len(p.workers)))

	// Collect workers to stop
	toStop := make([]struct {
		id string
		wi *workerInfo
	}, 0, len(p.workers))
	for id, wi := range p.workers {
		toStop = append(toStop, struct {
			id string
			wi *workerInfo
		}{id: id, wi: wi})
		delete(p.workers, id)
	}
	p.mu.Unlock()

	// Stop workers outside the lock
	for _, item := range toStop {
		if err := item.wi.worker.Stop(); err != nil {
			logger.ErrorContext(ctx, "Failed to stop worker", slog.String("worker_id", item.id), slog.Any("error", err))
		}
	}

	p.activeWorkers.Store(0)

	// Stop typing indicators
	if p.typingMgr != nil {
		p.typingMgr.StopAll()
	}
}

// ScaleUp adds workers to the pool.
func (p *DynamicWorkerPool) ScaleUp(count int) error {
	cmd := poolCommand{
		cmd:      "add",
		count:    count,
		response: make(chan error, 1),
	}

	select {
	case p.commands <- cmd:
		return <-cmd.response
	case <-p.stopCh:
		return fmt.Errorf("pool is shutting down")
	}
}

// ScaleDown removes workers from the pool.
func (p *DynamicWorkerPool) ScaleDown(count int) error {
	cmd := poolCommand{
		cmd:      "remove",
		count:    count,
		response: make(chan error, 1),
	}

	select {
	case p.commands <- cmd:
		return <-cmd.response
	case <-p.stopCh:
		return fmt.Errorf("pool is shutting down")
	}
}

// Size returns the current number of workers in the pool.
func (p *DynamicWorkerPool) Size() int {
	return int(p.activeWorkers.Load())
}

// Stop gracefully stops the worker pool.
func (p *DynamicWorkerPool) Stop(ctx context.Context) {
	logger := slog.Default()
	logger.InfoContext(ctx, "Stopping worker pool")

	// Send stop command first (before canceling contexts)
	cmd := poolCommand{
		cmd:      "stop",
		response: make(chan error, 1),
	}

	select {
	case p.commands <- cmd:
		select {
		case <-cmd.response:
			// Command processed - workers have been stopped
		case <-time.After(commandTimeout):
			logger.WarnContext(ctx, "Timeout waiting for stop command response")
		}
	case <-time.After(commandTimeout):
		logger.WarnContext(ctx, "Timeout sending stop command")
	}

	// Now cancel contexts to ensure everything shuts down
	p.workerCancel()
	p.cancel()
	// Close stop channel if not already closed
	select {
	case <-p.stopCh:
		// Already closed
	default:
		close(p.stopCh)
	}
}

// Wait blocks until all workers have stopped.
func (p *DynamicWorkerPool) Wait() {
	p.wg.Wait()
}
