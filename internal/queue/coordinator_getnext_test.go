package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// TestCoordinator_GetNextOldestFirst verifies that within each conversation,
// messages are processed in chronological order (oldest first).
func TestCoordinator_GetNextOldestFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coordinator := NewCoordinator(ctx)
	defer func() {
		if err := coordinator.Stop(); err != nil {
			t.Errorf("Failed to stop coordinator: %v", err)
		}
	}()

	// Create messages with different timestamps for multiple conversations
	conversations := []string{"+1111111111", "+2222222222", "+3333333333"}
	messageTimestamps := enqueueTestMessages(t, coordinator, conversations, 3)

	// Process all messages and verify they come out in order within each conversation
	processedOrder := processAllMessages(t, coordinator, messageTimestamps, "worker1", 9)

	// Verify each conversation's messages were processed in chronological order
	verifyChronologicalOrder(t, processedOrder, messageTimestamps)
}

// enqueueTestMessages creates and enqueues test messages for the given conversations
func enqueueTestMessages(t *testing.T, coordinator *Coordinator, conversations []string, messagesPerConv int) map[string][]time.Time {
	t.Helper()
	messageTimestamps := make(map[string][]time.Time)

	for _, from := range conversations {
		messageTimestamps[from] = []time.Time{}

		for i := 0; i < messagesPerConv; i++ {
			timestamp := time.Now().Add(time.Duration(i) * time.Minute)
			msg := signal.IncomingMessage{
				Timestamp: timestamp,
				From:      from,
				Text:      fmt.Sprintf("Message %d for %s", i, from),
			}
			messageTimestamps[from] = append(messageTimestamps[from], timestamp)

			if err := coordinator.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue message: %v", err)
			}
		}
	}

	return messageTimestamps
}

// processAllMessages processes messages and tracks their order
func processAllMessages(t *testing.T, coordinator *Coordinator, messageTimestamps map[string][]time.Time, workerID string, totalMessages int) map[string][]time.Time {
	t.Helper()
	processedOrder := make(map[string][]time.Time)

	for i := 0; i < totalMessages; i++ {
		msg, err := coordinator.GetNext(workerID)
		if err != nil {
			t.Fatalf("Failed to get next message: %v", err)
		}
		if msg == nil {
			t.Fatal("Expected message but got nil")
		}

		timestamp := extractMessageTimestamp(msg, messageTimestamps[msg.From])
		if processedOrder[msg.From] == nil {
			processedOrder[msg.From] = []time.Time{}
		}
		processedOrder[msg.From] = append(processedOrder[msg.From], timestamp)

		// Mark as completed to allow next message from same conversation
		if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
			t.Fatalf("Failed to update state: %v", err)
		}
	}

	return processedOrder
}

// extractMessageTimestamp extracts the timestamp from a message ID
func extractMessageTimestamp(msg *QueuedMessage, timestamps []time.Time) time.Time {
	for _, ts := range timestamps {
		if fmt.Sprintf("%d-%s", ts.UnixNano(), msg.From) == msg.ID {
			return ts
		}
	}
	return time.Time{}
}

// verifyChronologicalOrder verifies messages were processed in chronological order
func verifyChronologicalOrder(t *testing.T, processedOrder, messageTimestamps map[string][]time.Time) {
	t.Helper()
	for from, timestamps := range processedOrder {
		for i := 1; i < len(timestamps); i++ {
			if timestamps[i].Before(timestamps[i-1]) {
				t.Errorf("Messages for conversation %s not in chronological order: %v before %v",
					from, timestamps[i], timestamps[i-1])
			}
		}

		// Also verify they match the original order
		expectedTimestamps := messageTimestamps[from]
		if len(timestamps) != len(expectedTimestamps) {
			t.Errorf("Conversation %s: expected %d messages, got %d",
				from, len(expectedTimestamps), len(timestamps))
		}
	}
}

// TestCoordinator_GetNextFairScheduling verifies round-robin scheduling across conversations.
func TestCoordinator_GetNextFairScheduling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coordinator := NewCoordinator(ctx)
	defer func() {
		if err := coordinator.Stop(); err != nil {
			t.Errorf("Failed to stop coordinator: %v", err)
		}
	}()

	// Create 5 conversations with different message counts
	conversationMessages := map[string]int{
		"+1111111111": 5,
		"+2222222222": 3,
		"+3333333333": 7,
		"+4444444444": 2,
		"+5555555555": 4,
	}

	// Enqueue messages
	enqueueConversationMessages(t, coordinator, conversationMessages)

	// Process all messages and track rounds
	conversationRounds := processAndTrackRounds(t, coordinator, conversationMessages, "worker1")

	// Verify fair scheduling
	verifyFairScheduling(t, conversationRounds, conversationMessages)
}

// enqueueConversationMessages enqueues test messages for multiple conversations
func enqueueConversationMessages(t *testing.T, coordinator *Coordinator, conversationMessages map[string]int) {
	t.Helper()
	for from, count := range conversationMessages {
		for i := 0; i < count; i++ {
			msg := signal.IncomingMessage{
				Timestamp: time.Now(),
				From:      from,
				Text:      fmt.Sprintf("Message %d for %s", i, from),
			}

			if err := coordinator.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue message: %v", err)
			}
		}
	}
}

// processAndTrackRounds processes messages and tracks which round each conversation was processed in
func processAndTrackRounds(t *testing.T, coordinator *Coordinator, conversationMessages map[string]int, workerID string) map[string][]int {
	t.Helper()
	conversationRounds := make(map[string][]int)
	round := 1
	processedInRound := make(map[string]bool)

	totalMessages := 0
	for _, count := range conversationMessages {
		totalMessages += count
	}

	for i := 0; i < totalMessages; i++ {
		msg, err := coordinator.GetNext(workerID)
		if err != nil {
			t.Fatalf("Failed to get next message: %v", err)
		}
		if msg == nil {
			t.Fatal("Expected message but got nil")
		}

		round = trackMessageRound(msg.From, processedInRound, round)
		processedInRound[msg.From] = true

		if conversationRounds[msg.From] == nil {
			conversationRounds[msg.From] = []int{}
		}
		conversationRounds[msg.From] = append(conversationRounds[msg.From], round)

		if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
			t.Fatalf("Failed to update state: %v", err)
		}
	}

	return conversationRounds
}

// trackMessageRound determines which round a message belongs to
func trackMessageRound(from string, processedInRound map[string]bool, currentRound int) int {
	if processedInRound[from] {
		// Start a new round
		currentRound++
		for k := range processedInRound {
			delete(processedInRound, k)
		}
	}
	return currentRound
}

// verifyFairScheduling verifies that conversations are scheduled fairly
func verifyFairScheduling(t *testing.T, conversationRounds map[string][]int, conversationMessages map[string]int) {
	t.Helper()

	// Each conversation should be processed once per round until it runs out
	for from, rounds := range conversationRounds {
		for i := 1; i < len(rounds); i++ {
			if rounds[i] <= rounds[i-1] {
				t.Errorf("Conversation %s not scheduled fairly: round %d came after round %d",
					from, rounds[i], rounds[i-1])
			}
		}
	}

	// Verify no conversation waited more than N rounds
	maxWait := len(conversationMessages)
	for from, rounds := range conversationRounds {
		for i := 1; i < len(rounds); i++ {
			wait := rounds[i] - rounds[i-1]
			if wait > maxWait {
				t.Errorf("Conversation %s waited too long between messages: %d rounds (max allowed: %d)",
					from, wait, maxWait)
			}
		}
	}
}

// TestCoordinator_GetNextNoStarvation verifies that no conversation is starved
// even when some conversations have many more messages.
func TestCoordinator_GetNextNoStarvation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coordinator := NewCoordinator(ctx)
	defer func() {
		if err := coordinator.Stop(); err != nil {
			t.Errorf("Failed to stop coordinator: %v", err)
		}
	}()

	// Setup test data
	heavyConv := "+9999999999"
	lightConvs := []string{"+1111111111", "+2222222222", "+3333333333"}

	// Enqueue messages
	enqueueHeavyMessages(t, coordinator, heavyConv, 50)
	lightMessageTimes := enqueueLightMessages(t, coordinator, lightConvs)

	// Process and track messages
	processingResults := processMessagesWithTracking(t, coordinator, lightConvs)

	// Verify no starvation
	verifyNoStarvation(t, lightConvs, lightMessageTimes, processingResults)
}

// Helper functions for TestCoordinator_GetNextNoStarvation

func enqueueHeavyMessages(t *testing.T, coordinator *Coordinator, from string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		msg := signal.IncomingMessage{
			Timestamp: time.Now(),
			From:      from,
			Text:      fmt.Sprintf("Heavy message %d", i),
		}
		if err := coordinator.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}
	}
}

func enqueueLightMessages(t *testing.T, coordinator *Coordinator, conversations []string) map[string]time.Time {
	t.Helper()
	lightMessageTimes := make(map[string]time.Time)
	for _, from := range conversations {
		for i := 0; i < 2; i++ {
			enqueueTime := time.Now()
			if i == 0 {
				lightMessageTimes[from] = enqueueTime
			}
			msg := signal.IncomingMessage{
				Timestamp: enqueueTime,
				From:      from,
				Text:      fmt.Sprintf("Light message %d for %s", i, from),
			}
			if err := coordinator.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue message: %v", err)
			}
		}
	}
	return lightMessageTimes
}

type processingResult struct {
	lightProcessedTimes             map[string]time.Time
	maxMessagesBeforeLightProcessed int
}

func processMessagesWithTracking(t *testing.T, coordinator *Coordinator, lightConvs []string) processingResult {
	t.Helper()
	workerID := "worker1"
	lightProcessedTimes := make(map[string]time.Time)
	messageCount := 0
	maxMessagesBeforeLightProcessed := 0

	for {
		msg, err := coordinator.GetNext(workerID)
		if err != nil {
			if err == context.DeadlineExceeded {
				break
			}
			t.Fatalf("Failed to get next message: %v", err)
		}
		if msg == nil {
			break
		}

		messageCount++
		trackLightConversationProcessing(msg, lightConvs, lightProcessedTimes, messageCount, &maxMessagesBeforeLightProcessed)

		if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
			t.Fatalf("Failed to update state: %v", err)
		}

		if shouldStopProcessing(lightConvs, lightProcessedTimes, messageCount) {
			break
		}
	}

	return processingResult{
		lightProcessedTimes:             lightProcessedTimes,
		maxMessagesBeforeLightProcessed: maxMessagesBeforeLightProcessed,
	}
}

func trackLightConversationProcessing(msg *QueuedMessage, lightConvs []string, lightProcessedTimes map[string]time.Time, messageCount int, maxMessages *int) {
	for _, lightConv := range lightConvs {
		if msg.From == lightConv && lightProcessedTimes[lightConv].IsZero() {
			lightProcessedTimes[lightConv] = time.Now()
			if messageCount > *maxMessages {
				*maxMessages = messageCount
			}
		}
	}
}

func shouldStopProcessing(lightConvs []string, lightProcessedTimes map[string]time.Time, messageCount int) bool {
	allLightProcessed := true
	for _, from := range lightConvs {
		if lightProcessedTimes[from].IsZero() {
			allLightProcessed = false
			break
		}
	}
	return allLightProcessed && messageCount > 20
}

func verifyNoStarvation(t *testing.T, lightConvs []string, lightMessageTimes map[string]time.Time, results processingResult) {
	t.Helper()
	// Verify all light conversations got processed reasonably quickly
	for _, from := range lightConvs {
		if results.lightProcessedTimes[from].IsZero() {
			t.Errorf("Light conversation %s was never processed (starvation)", from)
			continue
		}

		waitTime := results.lightProcessedTimes[from].Sub(lightMessageTimes[from])
		if waitTime > 2*time.Second {
			t.Errorf("Light conversation %s waited too long: %v", from, waitTime)
		}
	}

	// Verify light conversations weren't starved by the heavy conversation
	expectedMaxMessages := len(lightConvs) + 5 // Some buffer for fair scheduling
	if results.maxMessagesBeforeLightProcessed > expectedMaxMessages {
		t.Errorf("Light conversations had to wait for %d messages before being processed (expected < %d)",
			results.maxMessagesBeforeLightProcessed, expectedMaxMessages)
	}
}

// TestCoordinator_GetNextConversationAffinity verifies that only one message
// per conversation is processed at a time (conversation affinity).
func TestCoordinator_GetNextConversationAffinity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coordinator := NewCoordinator(ctx)
	defer func() {
		if err := coordinator.Stop(); err != nil {
			t.Errorf("Failed to stop coordinator: %v", err)
		}
	}()

	// Coordinator starts immediately, no initialization delay needed

	from := "+1234567890"

	// Enqueue 5 messages for the same conversation
	for i := 0; i < 5; i++ {
		msg := signal.IncomingMessage{
			Timestamp: time.Now(),
			From:      from,
			Text:      fmt.Sprintf("Message %d", i),
		}

		if err := coordinator.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}
	}

	// Get first message with worker1
	worker1 := "worker1"
	msg1, err := coordinator.GetNext(worker1)
	if err != nil {
		t.Fatalf("Failed to get first message: %v", err)
	}
	if msg1 == nil {
		t.Fatal("Expected message but got nil")
	}

	// Try to get another message from the same conversation with worker2
	// This should fail or return a message from a different conversation
	worker2 := "worker2"
	msg2, err := coordinator.GetNext(worker2)
	if err == nil && msg2 != nil && msg2.From == from {
		t.Error("Second worker should not get message from same conversation while first is processing")
	}

	// Complete the first message
	if updateErr := coordinator.UpdateState(msg1.ID, MessageStateCompleted, "test completed"); updateErr != nil {
		t.Fatalf("Failed to update state: %v", updateErr)
	}

	// Now worker2 should be able to get a message from this conversation
	msg3, err := coordinator.GetNext(worker2)
	if err != nil {
		t.Fatalf("Failed to get message after first completed: %v", err)
	}
	if msg3 == nil {
		t.Fatal("Expected message but got nil")
	}
	if msg3.From != from {
		t.Errorf("Expected message from conversation %s, got %s", from, msg3.From)
	}
}

// TestCoordinator_GetNextConcurrentWorkers verifies that multiple workers
// can safely call GetNext concurrently without getting duplicate messages.
func TestCoordinator_GetNextConcurrentWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coordinator := NewCoordinator(ctx)
	defer func() {
		if err := coordinator.Stop(); err != nil {
			t.Errorf("Failed to stop coordinator: %v", err)
		}
	}()

	// Enqueue messages from multiple conversations
	numConversations := 10
	messagesPerConv := 5
	totalMessages := enqueueConcurrentTestMessages(t, coordinator, numConversations, messagesPerConv)

	// Create multiple workers that will process messages concurrently
	numWorkers := 5
	processedMessages, duplicates := runConcurrentWorkers(t, coordinator, numWorkers)

	// Verify all messages were processed exactly once
	verifyConcurrentProcessing(t, processedMessages, duplicates, totalMessages)
}

// enqueueConcurrentTestMessages enqueues test messages for concurrent processing
func enqueueConcurrentTestMessages(t *testing.T, coordinator *Coordinator, numConversations, messagesPerConv int) int {
	t.Helper()
	totalMessages := numConversations * messagesPerConv

	for i := 0; i < numConversations; i++ {
		from := fmt.Sprintf("+%010d", i)
		for j := 0; j < messagesPerConv; j++ {
			msg := signal.IncomingMessage{
				Timestamp: time.Now(),
				From:      from,
				Text:      fmt.Sprintf("Message %d for %s", j, from),
			}

			if err := coordinator.Enqueue(msg); err != nil {
				t.Fatalf("Failed to enqueue message: %v", err)
			}
		}
	}

	return totalMessages
}

// runConcurrentWorkers runs multiple workers concurrently and tracks processed messages
func runConcurrentWorkers(t *testing.T, coordinator *Coordinator, numWorkers int) (*sync.Map, *atomic.Int32) {
	t.Helper()
	var wg sync.WaitGroup
	processedMessages := &sync.Map{}
	duplicates := &atomic.Int32{}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker%d", w)
		go processWorkerMessages(t, coordinator, workerID, processedMessages, duplicates, &wg)
	}

	wg.Wait()
	return processedMessages, duplicates
}

// processWorkerMessages processes messages for a single worker
func processWorkerMessages(t *testing.T, coordinator *Coordinator, workerID string, processedMessages *sync.Map, duplicates *atomic.Int32, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		msg, err := coordinator.GetNext(workerID)
		if err != nil {
			if err == context.DeadlineExceeded {
				return
			}
			t.Errorf("Worker %s failed to get message: %v", workerID, err)
			return
		}
		if msg == nil {
			return // No more messages
		}

		// Check if this message was already processed
		if _, loaded := processedMessages.LoadOrStore(msg.ID, workerID); loaded {
			duplicates.Add(1)
			t.Errorf("Message %s was processed by multiple workers", msg.ID)
		}

		// Mark as completed
		if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
			t.Errorf("Worker %s failed to update state: %v", workerID, err)
		}
	}
}

// verifyConcurrentProcessing verifies that all messages were processed exactly once
func verifyConcurrentProcessing(t *testing.T, processedMessages *sync.Map, duplicates *atomic.Int32, totalMessages int) {
	t.Helper()
	processedCount := 0
	processedMessages.Range(func(_, _ any) bool {
		processedCount++
		return true
	})

	if processedCount != totalMessages {
		t.Errorf("Expected %d messages to be processed, got %d", totalMessages, processedCount)
	}

	if duplicates.Load() > 0 {
		t.Errorf("Found %d duplicate message assignments", duplicates.Load())
	}
}

// TestCoordinator_GetNextPriorityFairness verifies that high priority messages
// are processed first while still maintaining fairness across conversations.
func TestCoordinator_GetNextPriorityFairness(t *testing.T) {
	t.Skip("Priority handling not yet implemented in Manager")
	// This test would require modifying the Manager to handle priorities
	// within its fair scheduling algorithm
}

// BenchmarkCoordinator_GetNext measures GetNext performance under various loads.
func BenchmarkCoordinator_GetNext(b *testing.B) {
	scenarios := []struct {
		name             string
		numConversations int
		messagesPerConv  int
		numWorkers       int
	}{
		{"Small_10Conv_10Msg_1Worker", 10, 10, 1},
		{"Medium_50Conv_20Msg_5Workers", 50, 20, 5},
		{"Large_100Conv_50Msg_10Workers", 100, 50, 10},
		{"ManyConv_1000Conv_5Msg_20Workers", 1000, 5, 20},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			coordinator := setupBenchmarkCoordinator(b, scenario.numConversations, scenario.messagesPerConv)
			b.ResetTimer()
			runBenchmarkWorkers(b, coordinator, scenario.numWorkers, scenario.numConversations)
		})
	}
}

// setupBenchmarkCoordinator creates and populates a coordinator for benchmarking
func setupBenchmarkCoordinator(b *testing.B, numConversations, messagesPerConv int) *Coordinator {
	b.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(func() {
		cancel()
	})

	coordinator := NewCoordinator(ctx)
	b.Cleanup(func() {
		_ = coordinator.Stop()
	})

	// Pre-populate the queue
	populateBenchmarkQueue(b, coordinator, numConversations, messagesPerConv)
	return coordinator
}

// populateBenchmarkQueue fills the queue with test messages
func populateBenchmarkQueue(b *testing.B, coordinator *Coordinator, numConversations, messagesPerConv int) {
	b.Helper()
	for i := 0; i < numConversations; i++ {
		from := fmt.Sprintf("+%010d", i)
		for j := 0; j < messagesPerConv; j++ {
			msg := signal.IncomingMessage{
				Timestamp: time.Now(),
				From:      from,
				Text:      fmt.Sprintf("Message %d", j),
			}

			if err := coordinator.Enqueue(msg); err != nil {
				b.Fatalf("Failed to enqueue: %v", err)
			}
		}
	}
}

// runBenchmarkWorkers runs workers for the benchmark
func runBenchmarkWorkers(b *testing.B, coordinator *Coordinator, numWorkers, numConversations int) {
	b.Helper()
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker%d", w)
		go benchmarkWorker(b, coordinator, workerID, numWorkers, numConversations, &wg)
	}

	wg.Wait()
}

// benchmarkWorker processes messages for a single benchmark worker
func benchmarkWorker(b *testing.B, coordinator *Coordinator, workerID string, numWorkers, numConversations int, wg *sync.WaitGroup) {
	defer wg.Done()

	for i := 0; i < b.N/numWorkers; i++ {
		msg, err := coordinator.GetNext(workerID)
		if err != nil || msg == nil {
			// Re-enqueue a message to keep the queue full
			newMsg := signal.IncomingMessage{
				Timestamp: time.Now(),
				From:      fmt.Sprintf("+%010d", i%numConversations),
				Text:      "Benchmark message",
			}
			_ = coordinator.Enqueue(newMsg)
			continue
		}

		// Immediately complete to allow more messages
		_ = coordinator.UpdateState(msg.ID, MessageStateCompleted, "benchmark")
	}
}
