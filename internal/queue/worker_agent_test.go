package queue_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/mocks"
	"github.com/Veraticus/mentat/internal/queue"
)

// TestWorkerWithAgentHandler tests that the worker correctly uses the agent handler when provided.
func TestWorkerWithAgentHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mocks
	mockLLM := mocks.NewMockLLM()
	mockMessenger := mocks.NewMockMessenger()
	mockAgentHandler := mocks.NewMockAgentHandler()

	// Set up agent handler to return a specific response
	expectedResponse := "Agent handled response"
	mockAgentHandler.SetQueryResponse(claude.LLMResponse{Message: expectedResponse})

	// Create queue and manager
	manager := queue.NewManager(ctx)
	go manager.Start(ctx)

	// Wait for manager to start
	done := make(chan struct{})
	go func() {
		manager.WaitForReady()
		close(done)
	}()

	select {
	case <-done:
		// Manager is ready
	case <-time.After(1 * time.Second):
		t.Fatal("Manager failed to start within timeout")
	}

	defer func() {
		if err := manager.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker with agent handler
	config := queue.WorkerConfig{
		ID:           1,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: manager,
		RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
		AgentHandler: mockAgentHandler,
	}

	worker := queue.NewWorker(config)

	// Create and submit a message
	msg := queue.NewMessage("test-1", "conv-1", "TestUser", "+1234567890", "Hello, agent!")
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Start worker and process message
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Process just one message
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 2*time.Second)
		defer timeoutCancel()
		_ = worker.Start(timeoutCtx)
	}()

	// Wait for message to be processed using channel notification
	select {
	case <-mockMessenger.MessageSentCh():
		// Message was sent, processing complete
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message to be processed")
	}

	// Verify agent handler was called
	queryCalls := mockAgentHandler.GetQueryCalls()
	if len(queryCalls) != 1 {
		t.Errorf("Expected 1 query call to agent handler, got %d", len(queryCalls))
	} else {
		call := queryCalls[0]
		if call.Request != "Hello, agent!" {
			t.Errorf("Expected request 'Hello, agent!', got %q", call.Request)
		}
		if call.SessionID != "signal-conv-1" {
			t.Errorf("Expected session ID 'signal-conv-1', got %q", call.SessionID)
		}
	}

	// Verify message was sent with agent response
	sentMessages := mockMessenger.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Errorf("Expected 1 sent message, got %d", len(sentMessages))
	} else {
		sent := sentMessages[0]
		if sent.Recipient != "+1234567890" {
			t.Errorf("Expected recipient '+1234567890', got %q", sent.Recipient)
		}
		if sent.Message != expectedResponse {
			t.Errorf("Expected message %q, got %q", expectedResponse, sent.Message)
		}
	}

	// Verify LLM was NOT called directly
	llmCalls := mockLLM.GetCalls()
	if len(llmCalls) != 0 {
		t.Errorf("Expected 0 direct LLM calls when agent handler is present, got %d", len(llmCalls))
	}

	cancel()
	wg.Wait()
}

// TestWorkerFallbackToLLM tests that the worker falls back to direct LLM when no agent handler is provided.
func TestWorkerFallbackToLLM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mocks
	mockLLM := mocks.NewMockLLM()
	mockMessenger := mocks.NewMockMessenger()

	// Set up LLM to return a specific response
	expectedResponse := "Direct LLM response"
	mockLLM.SetResponse("signal-conv-2", "Hello, LLM!", &claude.LLMResponse{Message: expectedResponse})

	// Create queue and manager
	manager := queue.NewManager(ctx)
	go manager.Start(ctx)
	defer func() {
		if err := manager.Shutdown(time.Second); err != nil {
			t.Logf("Shutdown error: %v", err)
		}
	}()

	// Create worker WITHOUT agent handler
	config := queue.WorkerConfig{
		ID:           1,
		LLM:          mockLLM,
		Messenger:    mockMessenger,
		QueueManager: manager,
		RateLimiter:  queue.NewRateLimiter(10, 1, time.Second),
		AgentHandler: nil, // No agent handler
	}

	worker := queue.NewWorker(config)

	// Create and submit a message
	msg := queue.NewMessage("test-2", "conv-2", "TestUser", "+1234567890", "Hello, LLM!")
	if err := manager.Submit(msg); err != nil {
		t.Fatalf("Failed to submit message: %v", err)
	}

	// Start worker and process message
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Process just one message
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 2*time.Second)
		defer timeoutCancel()
		_ = worker.Start(timeoutCtx)
	}()

	// Wait for message to be processed using channel notification
	select {
	case <-mockMessenger.MessageSentCh():
		// Message was sent, processing complete
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message to be processed")
	}

	// Verify LLM was called directly
	llmCalls := mockLLM.GetCalls()
	if len(llmCalls) != 1 {
		t.Errorf("Expected 1 direct LLM call, got %d", len(llmCalls))
	} else {
		call := llmCalls[0]
		if call.Prompt != "Hello, LLM!" {
			t.Errorf("Expected prompt 'Hello, LLM!', got %q", call.Prompt)
		}
		if call.SessionID != "signal-conv-2" {
			t.Errorf("Expected session ID 'signal-conv-2', got %q", call.SessionID)
		}
	}

	// Verify message was sent with LLM response
	sentMessages := mockMessenger.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Errorf("Expected 1 sent message, got %d", len(sentMessages))
	} else {
		sent := sentMessages[0]
		if sent.Recipient != "+1234567890" {
			t.Errorf("Expected recipient '+1234567890', got %q", sent.Recipient)
		}
		if sent.Message != expectedResponse {
			t.Errorf("Expected message %q, got %q", expectedResponse, sent.Message)
		}
	}

	cancel()
	wg.Wait()
}
