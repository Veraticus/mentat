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

	// Allow coordinator to initialize
	time.Sleep(10 * time.Millisecond)

	// Create messages with different timestamps for multiple conversations
	conversations := []string{"+1111111111", "+2222222222", "+3333333333"}
	messageTimestamps := make(map[string][]time.Time)
	
	// Enqueue 3 messages per conversation with different timestamps
	for _, from := range conversations {
		messageTimestamps[from] = []time.Time{}
		
		for i := 0; i < 3; i++ {
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
			
			// Small delay to ensure distinct ordering
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Process all messages and verify they come out in order within each conversation
	processedOrder := make(map[string][]time.Time)
	workerID := "worker1"
	
	for i := 0; i < 9; i++ { // 3 conversations * 3 messages each
		msg, err := coordinator.GetNext(workerID)
		if err != nil {
			t.Fatalf("Failed to get next message: %v", err)
		}
		if msg == nil {
			t.Fatal("Expected message but got nil")
		}
		
		from := msg.From
		// Extract timestamp from message ID (format: timestamp-sender)
		var timestamp time.Time
		for _, ts := range messageTimestamps[from] {
			if fmt.Sprintf("%d-%s", ts.UnixNano(), from) == msg.ID {
				timestamp = ts
				break
			}
		}
		
		if processedOrder[from] == nil {
			processedOrder[from] = []time.Time{}
		}
		processedOrder[from] = append(processedOrder[from], timestamp)
		
		// Mark as completed to allow next message from same conversation
		if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
			t.Fatalf("Failed to update state: %v", err)
		}
	}
	
	// Verify each conversation's messages were processed in chronological order
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

	// Allow coordinator to initialize
	time.Sleep(10 * time.Millisecond)

	// Create 5 conversations with different message counts
	conversationMessages := map[string]int{
		"+1111111111": 5,
		"+2222222222": 3,
		"+3333333333": 7,
		"+4444444444": 2,
		"+5555555555": 4,
	}
	
	// Enqueue messages
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
	
	// Track which conversations have been processed in each round
	workerID := "worker1"
	conversationRounds := make(map[string][]int)
	round := 1
	processedInRound := make(map[string]bool)
	
	// Process all messages
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
		
		from := msg.From
		
		// Check if we've seen this conversation in the current round
		if processedInRound[from] {
			// Start a new round
			round++
			processedInRound = make(map[string]bool)
		}
		
		processedInRound[from] = true
		
		if conversationRounds[from] == nil {
			conversationRounds[from] = []int{}
		}
		conversationRounds[from] = append(conversationRounds[from], round)
		
		// Mark as completed
		if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
			t.Fatalf("Failed to update state: %v", err)
		}
	}
	
	// Verify fair scheduling: each conversation should be processed once per round
	// until it runs out of messages
	for from, rounds := range conversationRounds {
		for i := 1; i < len(rounds); i++ {
			if rounds[i] <= rounds[i-1] {
				t.Errorf("Conversation %s not scheduled fairly: round %d came after round %d",
					from, rounds[i], rounds[i-1])
			}
		}
	}
	
	// Verify no conversation waited more than N rounds where N is the number of active conversations
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

	// Allow coordinator to initialize
	time.Sleep(10 * time.Millisecond)

	// Create one conversation with many messages and several with few
	heavyConv := "+9999999999"
	lightConvs := []string{"+1111111111", "+2222222222", "+3333333333"}
	
	// Enqueue 50 messages for heavy conversation
	for i := 0; i < 50; i++ {
		msg := signal.IncomingMessage{
			Timestamp: time.Now(),
			From:      heavyConv,
			Text:      fmt.Sprintf("Heavy message %d", i),
		}
		
		if err := coordinator.Enqueue(msg); err != nil {
			t.Fatalf("Failed to enqueue message: %v", err)
		}
	}
	
	// Enqueue 2 messages for each light conversation
	lightMessageTimes := make(map[string]time.Time)
	for _, from := range lightConvs {
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
	
	// Process messages and track when light conversations get processed
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
		
		// Track when light conversations get their first message processed
		for _, lightConv := range lightConvs {
			if msg.From == lightConv && lightProcessedTimes[lightConv].IsZero() {
				lightProcessedTimes[lightConv] = time.Now()
				if messageCount > maxMessagesBeforeLightProcessed {
					maxMessagesBeforeLightProcessed = messageCount
				}
			}
		}
		
		// Mark as completed
		if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
			t.Fatalf("Failed to update state: %v", err)
		}
		
		// Stop if we've processed all messages from light conversations
		allLightProcessed := true
		for _, from := range lightConvs {
			if lightProcessedTimes[from].IsZero() {
				allLightProcessed = false
				break
			}
		}
		if allLightProcessed && messageCount > 20 { // Ensure we process enough to test
			break
		}
	}
	
	// Verify all light conversations got processed reasonably quickly
	for _, from := range lightConvs {
		if lightProcessedTimes[from].IsZero() {
			t.Errorf("Light conversation %s was never processed (starvation)", from)
			continue
		}
		
		waitTime := lightProcessedTimes[from].Sub(lightMessageTimes[from])
		if waitTime > 2*time.Second {
			t.Errorf("Light conversation %s waited too long: %v", from, waitTime)
		}
	}
	
	// Verify light conversations weren't starved by the heavy conversation
	// They should be processed within the first N messages where N is reasonable
	expectedMaxMessages := len(lightConvs) + 5 // Some buffer for fair scheduling
	if maxMessagesBeforeLightProcessed > expectedMaxMessages {
		t.Errorf("Light conversations had to wait for %d messages before being processed (expected < %d)",
			maxMessagesBeforeLightProcessed, expectedMaxMessages)
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

	// Allow coordinator to initialize
	time.Sleep(10 * time.Millisecond)

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

	// Allow coordinator to initialize
	time.Sleep(10 * time.Millisecond)

	// Enqueue messages from multiple conversations
	numConversations := 10
	messagesPerConv := 5
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
	
	// Create multiple workers that will process messages concurrently
	numWorkers := 5
	var wg sync.WaitGroup
	processedMessages := sync.Map{}
	duplicates := atomic.Int32{}
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker%d", w)
		
		go func(wID string) {
			defer wg.Done()
			
			for {
				msg, err := coordinator.GetNext(wID)
				if err != nil {
					if err == context.DeadlineExceeded {
						return
					}
					t.Errorf("Worker %s failed to get message: %v", wID, err)
					return
				}
				if msg == nil {
					// No more messages
					return
				}
				
				// Check if this message was already processed
				if _, loaded := processedMessages.LoadOrStore(msg.ID, wID); loaded {
					duplicates.Add(1)
					t.Errorf("Message %s was processed by multiple workers", msg.ID)
				}
				
				// Simulate processing time
				time.Sleep(10 * time.Millisecond)
				
				// Mark as completed
				if err := coordinator.UpdateState(msg.ID, MessageStateCompleted, "test completed"); err != nil {
					t.Errorf("Worker %s failed to update state: %v", wID, err)
				}
			}
		}(workerID)
	}
	
	// Wait for all workers to finish
	wg.Wait()
	
	// Verify all messages were processed exactly once
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
		name              string
		numConversations  int
		messagesPerConv   int
		numWorkers        int
	}{
		{"Small_10Conv_10Msg_1Worker", 10, 10, 1},
		{"Medium_50Conv_20Msg_5Workers", 50, 20, 5},
		{"Large_100Conv_50Msg_10Workers", 100, 50, 10},
		{"ManyConv_1000Conv_5Msg_20Workers", 1000, 5, 20},
	}
	
	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			
			coordinator := NewCoordinator(ctx)
			defer func() {
				_ = coordinator.Stop()
			}()
			
			// Allow coordinator to initialize
			time.Sleep(10 * time.Millisecond)
			
			// Pre-populate the queue
			for i := 0; i < scenario.numConversations; i++ {
				from := fmt.Sprintf("+%010d", i)
				for j := 0; j < scenario.messagesPerConv; j++ {
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
			
			b.ResetTimer()
			
			// Measure GetNext performance
			var wg sync.WaitGroup
			for w := 0; w < scenario.numWorkers; w++ {
				wg.Add(1)
				workerID := fmt.Sprintf("worker%d", w)
				
				go func(wID string) {
					defer wg.Done()
					
					for i := 0; i < b.N/scenario.numWorkers; i++ {
						msg, err := coordinator.GetNext(wID)
						if err != nil || msg == nil {
							// Re-enqueue a message to keep the queue full
							newMsg := signal.IncomingMessage{
								Timestamp: time.Now(),
								From:      fmt.Sprintf("+%010d", i%scenario.numConversations),
								Text:      "Benchmark message",
							}
							_ = coordinator.Enqueue(newMsg)
							continue
						}
						
						// Immediately complete to allow more messages
						_ = coordinator.UpdateState(msg.ID, MessageStateCompleted, "benchmark")
					}
				}(workerID)
			}
			
			wg.Wait()
		})
	}
}