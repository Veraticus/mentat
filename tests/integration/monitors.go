//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// ResourceMonitor tracks resource usage
type ResourceMonitor struct {
	startGoroutines int
	startTime       time.Time
	samples         []ResourceSample
	mu              sync.Mutex
}

// ResourceSample represents a point-in-time resource measurement
type ResourceSample struct {
	Timestamp  time.Time
	Goroutines int
	HeapAlloc  uint64
	HeapInUse  uint64
	StackInUse uint64
	NumGC      uint32
}

// NewResourceMonitor creates a resource monitor
func NewResourceMonitor() *ResourceMonitor {
	return &ResourceMonitor{
		startGoroutines: runtime.NumGoroutine(),
		startTime:       time.Now(),
		samples:         make([]ResourceSample, 0),
	}
}

// Sample takes a resource measurement
func (rm *ResourceMonitor) Sample() ResourceSample {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	sample := ResourceSample{
		Timestamp:  time.Now(),
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.HeapAlloc,
		HeapInUse:  m.HeapInuse,
		StackInUse: m.StackInuse,
		NumGC:      m.NumGC,
	}

	rm.mu.Lock()
	rm.samples = append(rm.samples, sample)
	rm.mu.Unlock()

	return sample
}

// CheckLeaks verifies no resource leaks
func (rm *ResourceMonitor) CheckLeaks() error {
	current := runtime.NumGoroutine()
	if current > rm.startGoroutines+5 { // Allow some variance
		return fmt.Errorf("goroutine leak detected: started with %d, now have %d",
			rm.startGoroutines, current)
	}
	return nil
}

// QueueMonitor provides detailed queue monitoring
type QueueMonitor struct {
	mu      sync.RWMutex
	samples []QueueSample
	alerts  []QueueAlert
	ticker  *time.Ticker
	stopCh  chan struct{}
}

// QueueSample represents queue metrics at a point in time
type QueueSample struct {
	Timestamp      time.Time
	QueueDepth     int
	Processing     int
	WaitingWorkers int
	Conversations  int
}

// QueueAlert represents a queue condition alert
type QueueAlert struct {
	Timestamp   time.Time
	Type        string
	Description string
	Metrics     map[string]int
}

// NewQueueMonitor creates a queue monitor
func NewQueueMonitor() *QueueMonitor {
	return &QueueMonitor{
		samples: make([]QueueSample, 0),
		alerts:  make([]QueueAlert, 0),
		stopCh:  make(chan struct{}),
	}
}

// Start begins monitoring
func (qm *QueueMonitor) Start(ctx context.Context, interval time.Duration, statsFn func() map[string]int) {
	qm.ticker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-qm.stopCh:
				return
			case <-qm.ticker.C:
				stats := statsFn()
				qm.recordSample(stats)
				qm.checkAlerts(stats)
			}
		}
	}()
}

// Stop halts monitoring
func (qm *QueueMonitor) Stop() {
	if qm.ticker != nil {
		qm.ticker.Stop()
	}
	close(qm.stopCh)
}

// recordSample records queue metrics
func (qm *QueueMonitor) recordSample(stats map[string]int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	sample := QueueSample{
		Timestamp:      time.Now(),
		QueueDepth:     stats["queued_messages"],
		Processing:     stats["processing_messages"],
		WaitingWorkers: stats["waiting_workers"],
		Conversations:  stats["conversations"],
	}

	qm.samples = append(qm.samples, sample)
}

// checkAlerts checks for alert conditions
func (qm *QueueMonitor) checkAlerts(stats map[string]int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	// Alert on high queue depth
	if stats["queued_messages"] > 100 {
		qm.alerts = append(qm.alerts, QueueAlert{
			Timestamp:   time.Now(),
			Type:        "HIGH_QUEUE_DEPTH",
			Description: fmt.Sprintf("Queue depth exceeded 100: %d", stats["queued_messages"]),
			Metrics:     stats,
		})
	}

	// Alert on no processing
	if stats["processing_messages"] == 0 && stats["queued_messages"] > 0 {
		qm.alerts = append(qm.alerts, QueueAlert{
			Timestamp:   time.Now(),
			Type:        "STALLED_PROCESSING",
			Description: "Messages queued but none processing",
			Metrics:     stats,
		})
	}
}

// GetSamples returns all recorded samples
func (qm *QueueMonitor) GetSamples() []QueueSample {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	samples := make([]QueueSample, len(qm.samples))
	copy(samples, qm.samples)
	return samples
}

// GetAlerts returns all alerts
func (qm *QueueMonitor) GetAlerts() []QueueAlert {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	alerts := make([]QueueAlert, len(qm.alerts))
	copy(alerts, qm.alerts)
	return alerts
}
