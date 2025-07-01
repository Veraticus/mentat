package mocks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/scheduler"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/Veraticus/mentat/internal/storage"
)

func TestMockLLM(t *testing.T) {
	ctx := context.Background()
	mock := NewMockLLM()

	// Test default response
	resp, err := mock.Query(ctx, "test prompt", "session1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "Mock response for: test prompt" {
		t.Errorf("unexpected message: %s", resp.Message)
	}

	// Test specific response
	expectedResp := &claude.LLMResponse{
		Message: "specific response",
		Metadata: claude.ResponseMetadata{
			Latency: 5 * time.Millisecond,
		},
	}
	mock.SetResponse("session2", "prompt2", expectedResp)
	resp, err = mock.Query(ctx, "prompt2", "session2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != expectedResp.Message {
		t.Errorf("expected %s, got %s", expectedResp.Message, resp.Message)
	}

	// Test session-wide response
	sessionResp := &claude.LLMResponse{Message: "session response"}
	mock.SetSessionResponse("session3", sessionResp)
	resp, err = mock.Query(ctx, "any prompt", "session3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != sessionResp.Message {
		t.Errorf("expected %s, got %s", sessionResp.Message, resp.Message)
	}

	// Test error
	expectedErr := fmt.Errorf("llm error")
	mock.SetError(expectedErr)
	_, err = mock.Query(ctx, "prompt", "session")
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Test call recording
	mock.SetError(nil)
	_, _ = mock.Query(ctx, "call1", "session1")
	_, _ = mock.Query(ctx, "call2", "session2")
	calls := mock.GetCalls()
	// Should have 6 calls total (3 before error, 1 error, 2 after)
	if len(calls) != 6 {
		t.Errorf("expected 6 calls, got %d", len(calls))
	}
}

func TestMockMessenger(t *testing.T) {
	ctx := context.Background()
	mock := NewMockMessenger()

	// Test send
	err := mock.Send(ctx, "recipient1", "message1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	messages := mock.GetSentMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Recipient != "recipient1" || messages[0].Message != "message1" {
		t.Errorf("unexpected message: %+v", messages[0])
	}

	// Test send error
	expectedErr := fmt.Errorf("send error")
	mock.SetSendError(expectedErr)
	err = mock.Send(ctx, "recipient2", "message2")
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Test subscribe
	mock.SetSendError(nil)
	ch, err := mock.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test inject message
	testMsg := signal.IncomingMessage{
		From:      "sender",
		Text:      "test",
		Timestamp: time.Now(),
	}
	mock.InjectMessage(testMsg)

	select {
	case msg := <-ch:
		if msg.From != testMsg.From || msg.Text != testMsg.Text {
			t.Errorf("unexpected message: %+v", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for message")
	}

	// Test typing indicator
	err = mock.SendTypingIndicator(ctx, "recipient3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockSessionManager(t *testing.T) {
	mock := NewMockSessionManager()

	// Test get or create session
	session1 := mock.GetOrCreateSession("user1")
	if session1 == "" {
		t.Error("expected non-empty session ID")
	}

	// Test getting same session
	session2 := mock.GetOrCreateSession("user1")
	if session1 != session2 {
		t.Errorf("expected same session, got %s and %s", session1, session2)
	}

	// Test get last session ID
	lastSession := mock.GetLastSessionID("user1")
	if lastSession != session1 {
		t.Errorf("expected %s, got %s", session1, lastSession)
	}

	// Test session history (empty by default)
	history := mock.GetSessionHistory(session1)
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d messages", len(history))
	}

	// Test expire sessions
	count := mock.ExpireSessions(time.Now())
	if count != 1 {
		t.Errorf("expected 1 expired session, got %d", count)
	}

	// Test after expiration
	lastSession = mock.GetLastSessionID("user1")
	if lastSession != "" {
		t.Errorf("expected empty session after expiration, got %s", lastSession)
	}
}

func TestMockRateLimiter(t *testing.T) {
	mock := NewMockRateLimiter()

	// Test default allow
	if !mock.Allow("conv1") {
		t.Error("expected default to allow")
	}

	// Test set allowed
	mock.SetAllowed("conv2", false)
	if mock.Allow("conv2") {
		t.Error("expected not allowed")
	}

	// Test record
	mock.Record("conv1")
	mock.Record("conv1")
	// No way to verify records directly in current implementation,
	// but this tests the method doesn't panic
}

func TestMockValidationStrategy(t *testing.T) {
	ctx := context.Background()
	mock := NewMockValidationStrategy()
	llm := NewMockLLM()

	// Test default validation
	result := mock.Validate(ctx, "request", "response", llm)
	if result.Status != agent.ValidationStatusSuccess {
		t.Errorf("expected success, got %v", result.Status)
	}
	if result.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", result.Confidence)
	}

	// Test custom result
	customResult := agent.ValidationResult{
		Status:     agent.ValidationStatusFailed,
		Confidence: 0.5,
		Issues:     []string{"issue1", "issue2"},
	}
	mock.SetResult(customResult)
	result = mock.Validate(ctx, "request", "response", llm)
	if result.Status != customResult.Status {
		t.Errorf("expected %v, got %v", customResult.Status, result.Status)
	}

	// Test retry
	if mock.ShouldRetry(result) {
		t.Error("expected no retry by default")
	}
	mock.SetRetry(true)
	if !mock.ShouldRetry(result) {
		t.Error("expected retry")
	}

	// Test recovery
	if mock.GenerateRecovery(ctx, "req", "resp", result, llm) != "" {
		t.Error("expected empty recovery by default")
	}
	mock.SetRecovery("recovery message")
	if mock.GenerateRecovery(ctx, "req", "resp", result, llm) != "recovery message" {
		t.Error("unexpected recovery message")
	}
}

func TestMockIntentEnhancer(t *testing.T) {
	mock := NewMockIntentEnhancer()

	// Test default behavior
	if mock.Enhance("test") != "test" {
		t.Error("expected no enhancement by default")
	}
	if mock.ShouldEnhance("test") {
		t.Error("expected no enhancement by default")
	}

	// Test custom enhance function
	mock.SetEnhanceFunc(func(s string) string {
		return "enhanced: " + s
	})
	if mock.Enhance("test") != "enhanced: test" {
		t.Error("unexpected enhancement")
	}

	// Test custom should enhance function
	mock.SetShouldEnhanceFunc(func(s string) bool {
		return len(s) > 5
	})
	if mock.ShouldEnhance("test") {
		t.Error("expected no enhancement for short string")
	}
	if !mock.ShouldEnhance("long test string") {
		t.Error("expected enhancement for long string")
	}
}

func TestMockAgentHandler(t *testing.T) {
	ctx := context.Background()
	mock := NewMockAgentHandler()

	msg1 := signal.IncomingMessage{From: "user1", Text: "msg1"}
	msg2 := signal.IncomingMessage{From: "user2", Text: "msg2"}

	// Test process
	err := mock.Process(ctx, msg1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mock.Process(ctx, msg2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test error
	expectedErr := fmt.Errorf("process error")
	mock.SetProcessError(expectedErr)
	err = mock.Process(ctx, msg1)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Test call recording
	calls := mock.GetCalls()
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(calls))
	}
	if calls[0].From != "user1" || calls[0].Text != "msg1" {
		t.Errorf("unexpected first call: %+v", calls[0])
	}
}

func TestMockMessageQueue(t *testing.T) {
	mock := NewMockMessageQueue()

	// Test enqueue
	msg := signal.IncomingMessage{
		From:      "user1",
		Text:      "test message",
		Timestamp: time.Now(),
	}
	err := mock.Enqueue(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test get next
	queuedMsg, err := mock.GetNext("worker1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queuedMsg.From != msg.From || queuedMsg.Text != msg.Text {
		t.Errorf("unexpected message: %+v", queuedMsg)
	}

	// Test update state
	err = mock.UpdateState(queuedMsg.ID, queue.MessageStateProcessing, "processing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test no more messages
	_, err = mock.GetNext("worker1")
	if err == nil {
		t.Error("expected error for no messages")
	}

	// Test stats
	stats := mock.Stats()
	if stats.TotalQueued != 0 {
		t.Errorf("unexpected stats: %+v", stats)
	}

	// Test error
	expectedErr := fmt.Errorf("queue error")
	mock.SetError(expectedErr)
	err = mock.Enqueue(msg)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestMockWorker(t *testing.T) {
	mock := NewMockWorker("worker1")

	// Test ID
	if mock.ID() != "worker1" {
		t.Errorf("unexpected ID: %s", mock.ID())
	}

	// Test start
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan bool)
	go func() {
		started <- true
		_ = mock.Start(ctx)
	}()

	<-started
	cancel()

	// Test stop
	err := mock.Stop()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockStateMachine(t *testing.T) {
	mock := NewMockStateMachine()
	msg := queue.NewMessage("msg1", "conv1", "user1", "+1234567890", "test")

	// Test valid transition
	if !mock.CanTransition(queue.StateQueued, queue.StateProcessing) {
		t.Error("expected valid transition")
	}

	// Test invalid transition
	if mock.CanTransition(queue.StateQueued, queue.StateCompleted) {
		t.Error("expected invalid transition")
	}

	// Test transition
	err := mock.Transition(msg, queue.StateProcessing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test nil message
	err = mock.Transition(nil, queue.StateProcessing)
	if err == nil {
		t.Error("expected error for nil message")
	}
}

func TestMockStorageMessages(t *testing.T) {
	mock := NewMockStorage()

	// Test save and get message
	msg := &storage.StoredMessage{
		ID:      "msg1",
		From:    "user1",
		To:      "bot",
		Content: "test",
	}
	err := mock.SaveMessage(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, err := mock.GetMessage("msg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retrieved.ID != msg.ID || retrieved.Content != msg.Content {
		t.Errorf("unexpected message: %+v", retrieved)
	}

	// Test message not found
	_, err = mock.GetMessage("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent message")
	}

	// Test conversation history
	history, err := mock.GetConversationHistory("user1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 message in history, got %d", len(history))
	}
}

func TestMockStorageQueue(t *testing.T) {
	mock := NewMockStorage()

	// Test queue item
	queueItem := &queue.QueuedMessage{
		ID:    "queue1",
		State: queue.MessageStateQueued,
	}
	err := mock.SaveQueueItem(queueItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test pending queue items
	pending, err := mock.GetPendingQueueItems()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending item, got %d", len(pending))
	}
}

func TestMockStorageSessions(t *testing.T) {
	mock := NewMockStorage()

	// Test session
	session := &storage.Session{
		ID:     "session1",
		UserID: "user1",
	}
	err := mock.SaveSession(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeSession, err := mock.GetActiveSession("user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activeSession.ID != session.ID {
		t.Errorf("unexpected session: %+v", activeSession)
	}

	err = mock.UpdateSessionActivity("session1", time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = mock.ExpireSessions(time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockStorageOtherMethods(t *testing.T) {
	mock := NewMockStorage()

	// Test LLM methods
	err := mock.SaveLLMCall(&storage.LLMCall{MessageID: "msg1"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = mock.GetLLMCallsForMessage("msg1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = mock.GetLLMCostReport("user1", time.Now(), time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test rate limit
	err = mock.SaveRateLimitState("user1", &storage.RateLimitState{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = mock.GetRateLimitState("user1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test circuit breaker
	err = mock.SaveCircuitBreakerState("service1", &storage.CircuitBreakerState{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = mock.GetCircuitBreakerState("service1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test metrics
	err = mock.RecordMetric(&storage.SystemMetric{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = mock.GetMetricsReport(time.Now(), time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = mock.PurgeOldData(time.Now(), "messages")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = mock.Vacuum()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockScheduler(t *testing.T) {
	ctx := context.Background()
	mock := NewMockScheduler()

	// Test schedule
	job := scheduler.Job{
		ID:       "job1",
		Name:     "Test Job",
		CronExpr: "* * * * *",
		Handler:  NewMockJobHandler("handler1"),
	}
	err := mock.Schedule(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test list
	jobs := mock.List()
	if len(jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(jobs))
	}

	// Test start
	err = mock.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test remove
	err = mock.Remove("job1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jobs = mock.List()
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs after removal, got %d", len(jobs))
	}

	// Test stop
	err = mock.Stop()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockJobHandler(t *testing.T) {
	ctx := context.Background()
	mock := NewMockJobHandler("test job")

	// Test name
	if mock.Name() != "test job" {
		t.Errorf("unexpected name: %s", mock.Name())
	}

	// Test execute
	err := mock.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test error
	expectedErr := fmt.Errorf("execute error")
	mock.SetExecuteError(expectedErr)
	err = mock.Execute(ctx)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Test call recording
	calls := mock.GetCalls()
	if len(calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(calls))
	}
}
