package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// PoolConfig holds configuration for the WorkerPool.
type PoolConfig struct {
	InitialSize        int
	MinSize            int
	MaxSize            int
	HealthCheckPeriod  time.Duration
	UnhealthyThreshold time.Duration
	LLM                claude.LLM
	Messenger          signal.Messenger
	QueueManager       *Manager
	MessageQueue       MessageQueue   // For state updates and stats
	RateLimiter        RateLimiter
	PanicHandler       PanicHandler // Optional: defaults to logging with stack trace
}

// workerHealth tracks the health of a worker.
type workerHealth struct {
	worker       Worker
	lastActivity time.Time
	healthy      bool
}

// poolCommand represents commands sent to the pool manager.
type poolCommand struct {
	cmd      string // "add", "remove", "health", "stop"
	count    int
	response chan error
}

// DynamicWorkerPool manages a pool of workers with dynamic sizing and health checks.
type DynamicWorkerPool struct {
	config       PoolConfig
	workers      map[string]*workerHealth
	typingMgr    signal.TypingIndicatorManager
	panicHandler PanicHandler
	
	// Channels for coordination
	commands   chan poolCommand
	workerDone chan string
	
	// Synchronization
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	workerCtx  context.Context
	workerCancel context.CancelFunc
	
	// Metrics
	activeWorkers  atomic.Int32
	healthyWorkers atomic.Int32
	nextWorkerID   atomic.Int32
}

// NewDynamicWorkerPool creates a new dynamic worker pool.
func NewDynamicWorkerPool(config PoolConfig) (*DynamicWorkerPool, error) {
	if config.InitialSize < 1 {
		return nil, fmt.Errorf("initial size must be at least 1")
	}
	if config.MinSize < 1 {
		config.MinSize = 1
	}
	if config.MaxSize < config.MinSize {
		config.MaxSize = config.MinSize * 2
	}
	if config.HealthCheckPeriod == 0 {
		config.HealthCheckPeriod = 30 * time.Second
	}
	if config.UnhealthyThreshold == 0 {
		config.UnhealthyThreshold = 2 * time.Minute
	}
	if config.PanicHandler == nil {
		config.PanicHandler = NewDefaultPanicHandler()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	workerCtx, workerCancel := context.WithCancel(context.Background())
	
	pool := &DynamicWorkerPool{
		config:       config,
		workers:      make(map[string]*workerHealth),
		typingMgr:    signal.NewTypingIndicatorManager(config.Messenger),
		panicHandler: config.PanicHandler,
		commands:     make(chan poolCommand, 10),
		workerDone:   make(chan string, config.MaxSize),
		ctx:          ctx,
		cancel:       cancel,
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}
	
	// Create initial workers
	for i := 0; i < config.InitialSize; i++ {
		if err := pool.createWorker(); err != nil {
			// Clean up any created workers
			pool.Stop()
			return nil, fmt.Errorf("failed to create initial workers: %w", err)
		}
	}
	
	return pool, nil
}

// Start begins the worker pool operation.
func (p *DynamicWorkerPool) Start(ctx context.Context) error {
	// Start the pool manager goroutine
	p.wg.Add(1)
	go p.manage()
	
	// Start health check routine
	p.wg.Add(1)
	go p.healthCheck()
	
	// Start all initial workers
	for id, wh := range p.workers {
		p.startWorker(ctx, id, wh.worker)
	}
	
	return nil
}

// manage handles pool commands and worker lifecycle.
func (p *DynamicWorkerPool) manage() {
	defer p.wg.Done()
	
	for {
		select {
		case <-p.ctx.Done():
			return
			
		case cmd := <-p.commands:
			switch cmd.cmd {
			case "add":
				cmd.response <- p.addWorkers(cmd.count)
			case "remove":
				cmd.response <- p.removeWorkers(cmd.count)
			case "stop":
				p.stopAllWorkers()
				cmd.response <- nil
				return
			}
			
		case workerID := <-p.workerDone:
			p.handleWorkerDone(workerID)
		}
	}
}

// healthCheck monitors worker health.
func (p *DynamicWorkerPool) healthCheck() {
	defer p.wg.Done()
	
	ticker := time.NewTicker(p.config.HealthCheckPeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
			
		case <-ticker.C:
			p.checkWorkerHealth()
		}
	}
}

// createWorker creates a new worker and adds it to the pool.
func (p *DynamicWorkerPool) createWorker() error {
	id := int(p.nextWorkerID.Add(1))
	
	config := WorkerConfig{
		ID:                 id,
		LLM:                p.config.LLM,
		Messenger:          p.config.Messenger,
		QueueManager:       p.config.QueueManager,
		MessageQueue:       p.config.MessageQueue,
		RateLimiter:        p.config.RateLimiter,
		TypingIndicatorMgr: p.typingMgr,
		ActivityRecorder:   p,
	}
	
	worker := NewWorker(config)
	workerID := worker.ID()
	
	p.workers[workerID] = &workerHealth{
		worker:       worker,
		lastActivity: time.Now(),
		healthy:      true,
	}
	
	p.activeWorkers.Add(1)
	p.healthyWorkers.Add(1)
	
	log.Printf("Created worker %s", workerID)
	return nil
}

// startWorker starts a worker in a new goroutine.
func (p *DynamicWorkerPool) startWorker(_ context.Context, id string, worker Worker) {
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
					case <-p.ctx.Done():
					}
				}
			} else {
				// Normal exit - always replace worker
				select {
				case p.workerDone <- id:
				case <-p.ctx.Done():
				}
			}
		}()
		
		log.Printf("Starting worker %s", id)
		if err := worker.Start(p.workerCtx); err != nil && err != context.Canceled {
			log.Printf("Worker %s stopped with error: %v", id, err)
		}
	}()
}

// addWorkers adds the specified number of workers to the pool.
func (p *DynamicWorkerPool) addWorkers(count int) error {
	currentSize := len(p.workers)
	maxAdd := p.config.MaxSize - currentSize
	
	if maxAdd <= 0 {
		return fmt.Errorf("pool at maximum size (%d)", p.config.MaxSize)
	}
	
	if count > maxAdd {
		count = maxAdd
	}
	
	for i := 0; i < count; i++ {
		if err := p.createWorker(); err != nil {
			return fmt.Errorf("failed to add worker: %w", err)
		}
		
		// Start the new worker
		for id, wh := range p.workers {
			if wh.lastActivity.IsZero() { // New worker
				wh.lastActivity = time.Now()
				p.startWorker(p.ctx, id, wh.worker)
				break
			}
		}
	}
	
	log.Printf("Added %d workers, pool size now %d", count, len(p.workers))
	return nil
}

// removeWorkers removes the specified number of workers from the pool.
func (p *DynamicWorkerPool) removeWorkers(count int) error {
	currentSize := len(p.workers)
	maxRemove := currentSize - p.config.MinSize
	
	if maxRemove <= 0 {
		return fmt.Errorf("pool at minimum size (%d)", p.config.MinSize)
	}
	
	if count > maxRemove {
		count = maxRemove
	}
	
	// Remove the least recently active workers
	removed := 0
	for id := range p.workers {
		if removed >= count {
			break
		}
		
		if wh, ok := p.workers[id]; ok {
			if err := wh.worker.Stop(); err != nil {
				log.Printf("Failed to stop worker %s: %v", id, err)
			}
			delete(p.workers, id)
			p.activeWorkers.Add(-1)
			if wh.healthy {
				p.healthyWorkers.Add(-1)
			}
			removed++
			log.Printf("Removed worker %s", id)
		}
	}
	
	log.Printf("Removed %d workers, pool size now %d", removed, len(p.workers))
	return nil
}

// checkWorkerHealth checks the health of all workers.
func (p *DynamicWorkerPool) checkWorkerHealth() {
	now := time.Now()
	unhealthyCount := 0
	
	// Get queue stats to determine if workers should be active
	stats := p.config.QueueManager.Stats()
	shouldBeActive := stats["queued"] > 0 || stats["processing"] > 0
	
	for id, wh := range p.workers {
		wasHealthy := wh.healthy
		
		// A worker is unhealthy if it hasn't been active and there's work to do
		if shouldBeActive && now.Sub(wh.lastActivity) > p.config.UnhealthyThreshold {
			wh.healthy = false
			unhealthyCount++
			
			if wasHealthy {
				p.healthyWorkers.Add(-1)
				log.Printf("Worker %s marked unhealthy (inactive for %v)", id, now.Sub(wh.lastActivity))
			}
		} else if !wasHealthy {
			// Worker recovered
			wh.healthy = true
			p.healthyWorkers.Add(1)
			log.Printf("Worker %s recovered", id)
		}
	}
	
	// Replace unhealthy workers if needed
	if unhealthyCount > 0 && len(p.workers) < p.config.MaxSize {
		log.Printf("Detected %d unhealthy workers, attempting to replace", unhealthyCount)
		p.replaceUnhealthyWorkers()
	}
}

// replaceUnhealthyWorkers replaces unhealthy workers with new ones.
func (p *DynamicWorkerPool) replaceUnhealthyWorkers() {
	for id, wh := range p.workers {
		if !wh.healthy {
			// Stop the unhealthy worker
			if err := wh.worker.Stop(); err != nil {
				log.Printf("Failed to stop worker %s: %v", id, err)
			}
			delete(p.workers, id)
			p.activeWorkers.Add(-1)
			
			// Create a replacement if we're not at max size
			if len(p.workers) < p.config.MaxSize {
				if err := p.createWorker(); err != nil {
					log.Printf("Failed to create replacement worker: %v", err)
				} else {
					// Start the new worker
					for newID, newWH := range p.workers {
						if newWH.lastActivity.IsZero() { // New worker
							newWH.lastActivity = time.Now()
							p.startWorker(p.ctx, newID, newWH.worker)
							break
						}
					}
				}
			}
		}
	}
}

// handleWorkerDone handles a worker that has stopped.
func (p *DynamicWorkerPool) handleWorkerDone(workerID string) {
	if wh, ok := p.workers[workerID]; ok {
		delete(p.workers, workerID)
		p.activeWorkers.Add(-1)
		if wh.healthy {
			p.healthyWorkers.Add(-1)
		}
		
		log.Printf("Worker %s stopped", workerID)
		
		// Replace the worker if we're below minimum size
		if len(p.workers) < p.config.MinSize {
			if err := p.createWorker(); err != nil {
				log.Printf("Failed to replace stopped worker: %v", err)
			} else {
				// Start the replacement
				for id, wh := range p.workers {
					if wh.lastActivity.IsZero() { // New worker
						wh.lastActivity = time.Now()
						p.startWorker(p.ctx, id, wh.worker)
						break
					}
				}
			}
		}
	}
}

// stopAllWorkers stops all workers in the pool.
func (p *DynamicWorkerPool) stopAllWorkers() {
	log.Printf("Stopping all %d workers", len(p.workers))
	
	for id, wh := range p.workers {
		if err := wh.worker.Stop(); err != nil {
			log.Printf("Failed to stop worker %s: %v", id, err)
		}
		delete(p.workers, id)
	}
	
	p.activeWorkers.Store(0)
	p.healthyWorkers.Store(0)
	
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
	case <-p.ctx.Done():
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
	case <-p.ctx.Done():
		return fmt.Errorf("pool is shutting down")
	}
}

// Size returns the current number of workers in the pool.
func (p *DynamicWorkerPool) Size() int {
	return int(p.activeWorkers.Load())
}

// HealthyWorkers returns the number of healthy workers.
func (p *DynamicWorkerPool) HealthyWorkers() int {
	return int(p.healthyWorkers.Load())
}

// Stop gracefully stops the worker pool.
func (p *DynamicWorkerPool) Stop() {
	log.Printf("Stopping worker pool")
	
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
		case <-time.After(2 * time.Second):
			log.Printf("Timeout waiting for stop command response")
		}
	case <-time.After(2 * time.Second):
		log.Printf("Timeout sending stop command")
	}
	
	// Now cancel contexts to ensure everything shuts down
	p.workerCancel()
	p.cancel()
}

// Wait blocks until all workers have stopped.
func (p *DynamicWorkerPool) Wait() {
	p.wg.Wait()
}

// RecordActivity marks a worker as active (called by workers when processing messages).
func (p *DynamicWorkerPool) RecordActivity(workerID string) {
	if wh, ok := p.workers[workerID]; ok {
		wh.lastActivity = time.Now()
	}
}