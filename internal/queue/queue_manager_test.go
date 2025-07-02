package queue_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
)

// Helper function to create test IncomingMessage.
func createIncomingMessage(from, text string) signal.IncomingMessage {
	return signal.IncomingMessage{
		Timestamp: time.Now(),
		From:      from,
		Text:      text,
	}
}

// Helper function to get expected message ID.
func getExpectedMessageID(msg signal.IncomingMessage) string {
	return fmt.Sprintf("%d-%s", msg.Timestamp.UnixNano(), msg.From)
}

func TestQueueManager_ImplementsInterface(_ *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// This test ensures QueueManager implements queue.MessageQueue interface
	var _ queue.MessageQueue = qm
}

func TestQueueManager_EnqueueAndGetNext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Test enqueue
	msg := createIncomingMessage("sender1", "Hello world")
	expectedID := getExpectedMessageID(msg)

	if err := qm.Enqueue(msg); err != nil {
		t.Fatalf("Failed to enqueue message: %v", err)
	}

	// Test get next
	queuedMsg, err := qm.GetNext("worker-1")
	if err != nil {
		t.Fatalf("Failed to get next message: %v", err)
	}

	if queuedMsg == nil {
		t.Fatal("Expected a message but got nil")
	}

	if queuedMsg.ID != expectedID {
		t.Errorf("Expected message ID %s, got %s", expectedID, queuedMsg.ID)
	}

	if queuedMsg.State != queue.StateProcessing {
		t.Errorf("Expected state %s (processing), got %s", queue.StateProcessing, queuedMsg.State)
	}
}

func TestQueueManager_UpdateState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Enqueue a message
	msg := createIncomingMessage("sender1", "Test message")
	expectedID := getExpectedMessageID(msg)

	if err := qm.Enqueue(msg); err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Get the message to put it in processing state
	queuedMsg, err := qm.GetNext("worker-1")
	if err != nil || queuedMsg == nil {
		t.Fatalf("Failed to get message: %v", err)
	}

	// Update state to completed
	if updateErr := qm.UpdateState(expectedID, queue.StateCompleted, "Processing completed"); updateErr != nil {
		t.Fatalf("Failed to update state: %v", updateErr)
	}

	// Verify stats reflect the state change
	stats := qm.Stats()
	if stats.TotalCompleted != 1 {
		t.Errorf("Expected 1 completed message, got %d", stats.TotalCompleted)
	}
}

func TestQueueManager_Stats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Initial stats should be zero
	stats := qm.Stats()
	if stats.TotalQueued != 0 || stats.TotalProcessing != 0 || stats.TotalCompleted != 0 {
		t.Error("Expected initial stats to be zero")
	}

	// Add messages from different conversations
	for i := range 5 {
		msg := createIncomingMessage(fmt.Sprintf("sender-%d", i%3), "Test") // 3 conversations
		if err := qm.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Check stats
	stats = qm.Stats()
	if stats.TotalQueued != 5 {
		t.Errorf("Expected 5 queued messages, got %d", stats.TotalQueued)
	}
	if stats.ConversationCount != 3 {
		t.Errorf("Expected 3 conversations, got %d", stats.ConversationCount)
	}

	// Get one message to process
	if _, err := qm.GetNext("worker-1"); err != nil {
		t.Fatalf("Failed to get next: %v", err)
	}

	stats = qm.Stats()
	if stats.TotalQueued != 4 {
		t.Errorf("Expected 4 queued after processing one, got %d", stats.TotalQueued)
	}
	if stats.TotalProcessing != 1 {
		t.Errorf("Expected 1 processing, got %d", stats.TotalProcessing)
	}
}

func TestQueueManager_FairScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	conversations := []string{"sender-1", "sender-2", "sender-3"}
	messagesPerConv := 3

	// Enqueue messages
	for _, sender := range conversations {
		for i := range messagesPerConv {
			msg := createIncomingMessage(sender, fmt.Sprintf("Test message %d", i))
			if err := qm.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue: %v", err)
			}
		}
	}

	// Track which conversations we get messages from
	convCounts := make(map[string]int)
	messages := make([]*queue.Message, 0, len(conversations))

	// First round - should get one from each conversation
	// Get all messages first before completing any
	for i := range conversations {
		msg, err := qm.GetNext(fmt.Sprintf("worker-%d", i))
		if err != nil || msg == nil {
			t.Fatalf("Failed to get message %d: %v", i, err)
		}
		convCounts[msg.ConversationID]++
		messages = append(messages, msg)
	}

	// Now complete all messages
	for _, msg := range messages {
		if err := qm.UpdateState(msg.ID, queue.StateCompleted, "done"); err != nil {
			t.Errorf("Failed to complete message: %v", err)
		}
	}

	// Check fairness - each conversation should have been scheduled once
	for _, sender := range conversations {
		// ConversationID is the sender itself in the test helper
		if convCounts[sender] != 1 {
			t.Errorf(
				"Conversation %s scheduled %d times, expected 1 (counts: %v)",
				sender,
				convCounts[sender],
				convCounts,
			)
		}
	}
}

// testWorker represents a concurrent worker for testing.
type testWorker struct {
	id       string
	qm       queue.MessageQueue
	t        *testing.T
	procChan chan<- int32
}

func (w *testWorker) run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if !w.processNextMessage() {
				<-time.After(10 * time.Millisecond)
			}
		}
	}
}

func (w *testWorker) processNextMessage() bool {
	msg, err := w.qm.GetNext(w.id)
	if err != nil || msg == nil {
		return false
	}

	// Simulate processing
	<-time.After(time.Millisecond)

	// Mark as completed
	if updateErr := w.qm.UpdateState(msg.ID, queue.StateCompleted, "processed"); updateErr != nil {
		w.t.Errorf("Failed to update state: %v", updateErr)
		return false
	}

	w.procChan <- 1
	return true
}

// testConversationProducer generates messages for a conversation.
type testConversationProducer struct {
	convNum         int
	messagesPerConv int
	qm              queue.MessageQueue
	t               *testing.T
	enqueueChan     chan<- int32
}

func (p *testConversationProducer) run(wg *sync.WaitGroup) {
	defer wg.Done()

	for m := range p.messagesPerConv {
		msg := createIncomingMessage(
			fmt.Sprintf("sender-%d", p.convNum),
			fmt.Sprintf("Message %d from conversation %d", m, p.convNum),
		)
		if err := p.qm.Enqueue(msg); err != nil {
			p.t.Errorf("Failed to enqueue message: %v", err)
			return
		}
		p.enqueueChan <- 1
	}
}

func TestQueueManager_100ConcurrentConversations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	numConversations := 100
	messagesPerConv := 10
	numWorkers := 20
	expectedTotal := numConversations * messagesPerConv

	// Channels for counting
	enqueueChan := make(chan int32, expectedTotal)
	processChan := make(chan int32, expectedTotal)

	var wg sync.WaitGroup

	// Start workers
	startWorkers(ctx, t, qm, numWorkers, processChan, &wg)

	// Start producers
	startProducers(t, qm, numConversations, messagesPerConv, enqueueChan, &wg)

	// Verify enqueue count
	enqueueCount := waitForCount(enqueueChan, expectedTotal, 500*time.Millisecond)
	if enqueueCount != expectedTotal {
		t.Errorf("Expected %d messages enqueued, got %d", expectedTotal, enqueueCount)
	}

	// Wait for processing completion
	if !waitForProcessingComplete(t, processChan, expectedTotal, 30*time.Second) {
		return
	}

	// Clean shutdown
	cancel()
	wg.Wait()

	// Verify final stats
	verifyFinalStats(t, qm, expectedTotal)

	t.Logf(
		"Successfully processed %d messages from %d concurrent conversations",
		expectedTotal,
		numConversations,
	)
}

func startWorkers(
	ctx context.Context,
	t *testing.T,
	qm queue.MessageQueue,
	numWorkers int,
	processChan chan<- int32,
	wg *sync.WaitGroup,
) {
	t.Helper()
	for w := range numWorkers {
		wg.Add(1)
		worker := &testWorker{
			id:       fmt.Sprintf("worker-%d", w),
			qm:       qm,
			t:        t,
			procChan: processChan,
		}
		go worker.run(ctx, wg)
	}
}

func startProducers(
	t *testing.T,
	qm queue.MessageQueue,
	numConversations, messagesPerConv int,
	enqueueChan chan<- int32,
	wg *sync.WaitGroup,
) {
	t.Helper()
	for c := range numConversations {
		wg.Add(1)
		producer := &testConversationProducer{
			convNum:         c,
			messagesPerConv: messagesPerConv,
			qm:              qm,
			t:               t,
			enqueueChan:     enqueueChan,
		}
		go producer.run(wg)
	}
}

func waitForCount(countChan <-chan int32, expected int, timeout time.Duration) int {
	count := 0
	deadline := time.After(timeout)

	for count < expected {
		select {
		case <-countChan:
			count++
		case <-deadline:
			return count
		}
	}
	return count
}

func waitForProcessingComplete(
	t *testing.T,
	processChan <-chan int32,
	expectedTotal int,
	timeout time.Duration,
) bool {
	t.Helper()
	processedCount := 0
	deadline := time.After(timeout)

	for processedCount < expectedTotal {
		select {
		case <-processChan:
			processedCount++
		case <-deadline:
			t.Fatalf("Timeout: only processed %d/%d messages", processedCount, expectedTotal)
			return false
		}
	}
	return true
}

func verifyFinalStats(t *testing.T, qm queue.MessageQueue, expectedTotal int) {
	t.Helper()
	finalStats := qm.Stats()
	if finalStats.TotalCompleted != expectedTotal {
		t.Errorf("Expected %d completed messages, got %d", expectedTotal, finalStats.TotalCompleted)
	}
}

func TestQueueManager_UpdateStateInvalidMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Try to update state of non-existent message
	err := qm.UpdateState("non-existent-id", queue.StateCompleted, "test")
	if err == nil {
		t.Error("Expected error when updating non-existent message")
	}
}

func TestQueueManager_GetNextTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	// Try to get message when queue is empty
	start := time.Now()
	msg, err := qm.GetNext("worker-1")
	duration := time.Since(start)

	if msg != nil {
		t.Error("Expected nil message when queue is empty")
	}

	if err == nil {
		t.Error("Expected error for empty queue, got nil")
	}

	// Should return quickly (not block indefinitely)
	if duration > 100*time.Millisecond {
		t.Errorf("GetNext took too long on empty queue: %v", duration)
	}
}

func TestQueueManager_ConcurrentEnqueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)
	defer func() {
		_ = qm.Stop()
	}()

	numGoroutines := 50
	messagesPerGoroutine := 20
	var wg sync.WaitGroup
	var successCount int32

	for g := range numGoroutines {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			for range messagesPerGoroutine {
				msg := createIncomingMessage(
					fmt.Sprintf("sender-%d", gID%10), // 10 conversations
					"Test",
				)
				if err := qm.Enqueue(msg); err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}
		}(g)
	}

	wg.Wait()

	expectedTotal := numGoroutines * messagesPerGoroutine
	if int(successCount) != expectedTotal {
		t.Errorf("Expected %d successful enqueues, got %d", expectedTotal, successCount)
	}

	stats := qm.Stats()
	if stats.TotalQueued != expectedTotal {
		t.Errorf("Expected %d queued messages, got %d", expectedTotal, stats.TotalQueued)
	}
}

func TestQueueManager_StopGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := queue.NewCoordinator(ctx)

	// Enqueue some messages
	for i := range 5 {
		msg := createIncomingMessage("sender", fmt.Sprintf("Test %d", i))
		if err := qm.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue: %v", err)
		}
	}

	// Stop should complete quickly
	done := make(chan bool)
	go func() {
		_ = qm.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop took too long")
	}

	// Should not be able to enqueue after stop
	err := qm.Enqueue(createIncomingMessage("sender", "Test after stop"))
	if err == nil {
		t.Error("Expected error when enqueuing after stop")
	}
}
