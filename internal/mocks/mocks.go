// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/scheduler"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/Veraticus/mentat/internal/storage"
)

// Mock constants.
const (
	// defaultMockLatencyMS is the default latency for mock LLM responses.
	defaultMockLatencyMS = 10
	// incomingChannelSize is the buffer size for incoming message channels.
	incomingChannelSize = 100
	// mockProcessPID is the default PID for mock processes.
	mockProcessPID = 12345
	// secondaryDeviceID is the ID for the secondary test device.
	secondaryDeviceID = 2
)

// Compile-time checks to ensure mocks implement their interfaces.
var (
	_ claude.LLM                  = (*MockLLM)(nil)
	_ signal.Messenger            = (*MockMessenger)(nil)
	_ signal.Manager              = (*MockSignalManager)(nil)
	_ signal.ProcessManager       = (*MockProcessManager)(nil)
	_ signal.DeviceManager        = (*MockDeviceManager)(nil)
	_ conversation.SessionManager = (*MockSessionManager)(nil)
	_ queue.RateLimiter           = (*MockRateLimiter)(nil)
	_ agent.ValidationStrategy    = (*MockValidationStrategy)(nil)
	_ agent.IntentEnhancer        = (*MockIntentEnhancer)(nil)
	_ agent.Handler               = (*MockAgentHandler)(nil)
	_ queue.Worker                = (*MockWorker)(nil)
	_ queue.StateMachine          = (*MockStateMachine)(nil)
	_ storage.Storage             = (*MockStorage)(nil)
	_ scheduler.Scheduler         = (*MockScheduler)(nil)
	_ scheduler.JobHandler        = (*MockJobHandler)(nil)
)

// MockLLM is a test implementation of the LLM interface.
type MockLLM struct {
	err       error
	responses map[string]*claude.LLMResponse
	calls     []LLMCall
	mu        sync.Mutex

	// QueryFunc allows tests to provide custom query behavior
	QueryFunc func(ctx context.Context, prompt, sessionID string) (*claude.LLMResponse, error)
}

// LLMCall records a call to the LLM.
type LLMCall struct {
	Timestamp time.Time
	Prompt    string
	SessionID string
}

// NewMockLLM creates a new mock LLM.
func NewMockLLM() *MockLLM {
	return &MockLLM{
		responses: make(map[string]*claude.LLMResponse),
		calls:     make([]LLMCall, 0),
	}
}

// Query implements the LLM interface.
func (m *MockLLM) Query(
	ctx context.Context,
	prompt string,
	sessionID string,
) (*claude.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, LLMCall{
		Prompt:    prompt,
		SessionID: sessionID,
		Timestamp: time.Now(),
	})

	// If QueryFunc is provided, use it
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, prompt, sessionID)
	}

	if m.err != nil {
		return nil, m.err
	}

	// Check for specific response
	key := fmt.Sprintf("%s:%s", sessionID, prompt)
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}

	// Check for session-wide response
	if resp, ok := m.responses[sessionID]; ok {
		return resp, nil
	}

	// Check for empty sessionID (default response)
	if resp, ok := m.responses[""]; ok {
		return resp, nil
	}

	// Default response
	return &claude.LLMResponse{
		Message: "Mock response for: " + prompt,
		Metadata: claude.ResponseMetadata{
			Latency: defaultMockLatencyMS * time.Millisecond,
		},
	}, nil
}

// SetResponse sets a mock response for a specific prompt and session.
func (m *MockLLM) SetResponse(sessionID, prompt string, response *claude.LLMResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", sessionID, prompt)
	m.responses[key] = response
}

// SetSessionResponse sets a mock response for all prompts in a session.
func (m *MockLLM) SetSessionResponse(sessionID string, response *claude.LLMResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[sessionID] = response
}

// SetError sets an error to be returned on all queries.
func (m *MockLLM) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// GetCalls returns all recorded calls.
func (m *MockLLM) GetCalls() []LLMCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]LLMCall, len(m.calls))
	copy(calls, m.calls)
	return calls
}

// MockMessenger is a test implementation of the Messenger interface.
type MockMessenger struct {
	sendErr          error
	subscribeErr     error
	incomingChan     chan signal.IncomingMessage
	sentMessages     []SentMessage
	typingIndicators []TypingIndicator
	mu               sync.Mutex
}

// SentMessage records a sent message.
type SentMessage struct {
	Timestamp time.Time
	Recipient string
	Message   string
}

// TypingIndicator records a typing indicator.
type TypingIndicator struct {
	Timestamp time.Time
	Recipient string
}

// NewMockMessenger creates a new mock messenger.
func NewMockMessenger() *MockMessenger {
	return &MockMessenger{
		sentMessages:     make([]SentMessage, 0),
		incomingChan:     make(chan signal.IncomingMessage, incomingChannelSize),
		typingIndicators: make([]TypingIndicator, 0),
	}
}

// Send implements the Messenger interface.
func (m *MockMessenger) Send(_ context.Context, recipient string, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}

	m.sentMessages = append(m.sentMessages, SentMessage{
		Recipient: recipient,
		Message:   message,
		Timestamp: time.Now(),
	})

	return nil
}

// Subscribe implements the Messenger interface.
func (m *MockMessenger) Subscribe(_ context.Context) (<-chan signal.IncomingMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}

	return m.incomingChan, nil
}

// SendTypingIndicator implements the Messenger interface.
func (m *MockMessenger) SendTypingIndicator(_ context.Context, recipient string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.typingIndicators = append(m.typingIndicators, TypingIndicator{
		Recipient: recipient,
		Timestamp: time.Now(),
	})

	return nil
}

// InjectMessage adds a message to the incoming channel.
func (m *MockMessenger) InjectMessage(msg signal.IncomingMessage) {
	select {
	case m.incomingChan <- msg:
		// Message sent successfully
	default:
		// Channel is full, drop the message to avoid blocking
		// This prevents deadlocks in tests
	}
}

// GetSentMessages returns all sent messages.
func (m *MockMessenger) GetSentMessages() []SentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	messages := make([]SentMessage, len(m.sentMessages))
	copy(messages, m.sentMessages)
	return messages
}

// SetSendError sets an error to be returned on send.
func (m *MockMessenger) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

// MockSessionManager is a test implementation of the SessionManager interface.
type MockSessionManager struct {
	sessions map[string]string
	history  map[string][]conversation.Message
	mu       sync.Mutex
}

// NewMockSessionManager creates a new mock session manager.
func NewMockSessionManager() *MockSessionManager {
	return &MockSessionManager{
		sessions: make(map[string]string),
		history:  make(map[string][]conversation.Message),
	}
}

// GetOrCreateSession implements the SessionManager interface.
func (m *MockSessionManager) GetOrCreateSession(identifier string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[identifier]; ok {
		return session
	}

	session := fmt.Sprintf("session-%s-%d", identifier, time.Now().Unix())
	m.sessions[identifier] = session
	return session
}

// GetSessionHistory implements the SessionManager interface.
func (m *MockSessionManager) GetSessionHistory(sessionID string) []conversation.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	if history, ok := m.history[sessionID]; ok {
		return history
	}
	return []conversation.Message{}
}

// ExpireSessions implements the SessionManager interface.
func (m *MockSessionManager) ExpireSessions(_ time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id := range m.sessions {
		delete(m.sessions, id)
		delete(m.history, id)
		count++
	}
	return count
}

// GetLastSessionID implements the SessionManager interface.
func (m *MockSessionManager) GetLastSessionID(identifier string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[identifier]; ok {
		return session
	}
	return ""
}

// MockRateLimiter is a test implementation of the queue.RateLimiter interface.
type MockRateLimiter struct {
	allowedConvs map[string]bool
	records      map[string][]time.Time
	mu           sync.Mutex
}

// NewMockRateLimiter creates a new mock rate limiter.
func NewMockRateLimiter() *MockRateLimiter {
	return &MockRateLimiter{
		allowedConvs: make(map[string]bool),
		records:      make(map[string][]time.Time),
	}
}

// Allow implements the queue.RateLimiter interface.
func (m *MockRateLimiter) Allow(conversationID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if allowed, ok := m.allowedConvs[conversationID]; ok {
		return allowed
	}
	return true
}

// Record implements the queue.RateLimiter interface.
func (m *MockRateLimiter) Record(conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records[conversationID] = append(m.records[conversationID], time.Now())
}

// SetAllowed sets whether a conversation is rate limited.
func (m *MockRateLimiter) SetAllowed(conversationID string, allowed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowedConvs[conversationID] = allowed
}

// Wait implements the queue.RateLimiter interface.
func (m *MockRateLimiter) Wait(_ context.Context, conversationID string) error {
	if !m.Allow(conversationID) {
		return fmt.Errorf("rate limit exceeded for conversation %s", conversationID)
	}
	return nil
}

// CleanupStale implements the queue.RateLimiter interface.
func (m *MockRateLimiter) CleanupStale(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// For testing, just clear old records
	cutoff := time.Now().Add(-maxAge)
	for convID, records := range m.records {
		var kept []time.Time
		for _, t := range records {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(m.records, convID)
		} else {
			m.records[convID] = kept
		}
	}
}

// Stats implements the queue.RateLimiter interface.
func (m *MockRateLimiter) Stats() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]any{
		"allowed_conversations":  len(m.allowedConvs),
		"recorded_conversations": len(m.records),
	}
}

// MockValidationStrategy is a test implementation of ValidationStrategy.
type MockValidationStrategy struct {
	recovery string
	result   agent.ValidationResult
	retry    bool
}

// NewMockValidationStrategy creates a new mock validation strategy.
func NewMockValidationStrategy() *MockValidationStrategy {
	return &MockValidationStrategy{
		result: agent.ValidationResult{
			Status:     agent.ValidationStatusSuccess,
			Confidence: 1.0,
		},
		retry: false,
	}
}

// Validate implements the ValidationStrategy interface.
func (m *MockValidationStrategy) Validate(
	_ context.Context,
	_, _, _ string,
	_ claude.LLM,
) agent.ValidationResult {
	return m.result
}

// ShouldRetry implements the ValidationStrategy interface.
func (m *MockValidationStrategy) ShouldRetry(_ agent.ValidationResult) bool {
	return m.retry
}

// GenerateRecovery implements the ValidationStrategy interface.
func (m *MockValidationStrategy) GenerateRecovery(
	_ context.Context,
	_, _, _ string,
	_ agent.ValidationResult,
	_ claude.LLM,
) string {
	return m.recovery
}

// SetResult sets the validation result.
func (m *MockValidationStrategy) SetResult(result agent.ValidationResult) {
	m.result = result
}

// SetRetry sets whether retry is recommended.
func (m *MockValidationStrategy) SetRetry(retry bool) {
	m.retry = retry
}

// SetRecovery sets the recovery prompt.
func (m *MockValidationStrategy) SetRecovery(recovery string) {
	m.recovery = recovery
}

// MockIntentEnhancer is a test implementation of the IntentEnhancer interface.
type MockIntentEnhancer struct {
	enhanceFunc       func(string) string
	shouldEnhanceFunc func(string) bool
}

// NewMockIntentEnhancer creates a new mock intent enhancer.
func NewMockIntentEnhancer() *MockIntentEnhancer {
	return &MockIntentEnhancer{
		enhanceFunc:       func(s string) string { return s },
		shouldEnhanceFunc: func(_ string) bool { return false },
	}
}

// Enhance implements the IntentEnhancer interface.
func (m *MockIntentEnhancer) Enhance(originalRequest string) string {
	return m.enhanceFunc(originalRequest)
}

// ShouldEnhance implements the IntentEnhancer interface.
func (m *MockIntentEnhancer) ShouldEnhance(request string) bool {
	return m.shouldEnhanceFunc(request)
}

// SetEnhanceFunc sets the enhance function.
func (m *MockIntentEnhancer) SetEnhanceFunc(f func(string) string) {
	m.enhanceFunc = f
}

// SetShouldEnhanceFunc sets the should enhance function.
func (m *MockIntentEnhancer) SetShouldEnhanceFunc(f func(string) bool) {
	m.shouldEnhanceFunc = f
}

// MockAgentHandler is a test implementation of the agent.Handler interface.
type MockAgentHandler struct {
	processErr error
	queryResp  claude.LLMResponse
	queryErr   error
	calls      []signal.IncomingMessage
	queryCalls []QueryCall
	mu         sync.Mutex
}

// QueryCall records a call to Query.
type QueryCall struct {
	Request   string
	SessionID string
}

// NewMockAgentHandler creates a new mock agent handler.
func NewMockAgentHandler() *MockAgentHandler {
	return &MockAgentHandler{
		calls:      make([]signal.IncomingMessage, 0),
		queryCalls: make([]QueryCall, 0),
		queryResp:  claude.LLMResponse{Message: "mock response"},
	}
}

// Process implements the agent.Handler interface.
func (m *MockAgentHandler) Process(_ context.Context, msg signal.IncomingMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, msg)
	return m.processErr
}

// SetProcessError sets the error to return from Process.
func (m *MockAgentHandler) SetProcessError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processErr = err
}

// GetCalls returns all recorded calls.
func (m *MockAgentHandler) GetCalls() []signal.IncomingMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]signal.IncomingMessage, len(m.calls))
	copy(calls, m.calls)
	return calls
}

// Query implements the agent.Handler interface.
func (m *MockAgentHandler) Query(_ context.Context, request, sessionID string) (claude.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queryCalls = append(m.queryCalls, QueryCall{Request: request, SessionID: sessionID})
	return m.queryResp, m.queryErr
}

// SetQueryResponse sets the response to return from Query.
func (m *MockAgentHandler) SetQueryResponse(resp claude.LLMResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryResp = resp
}

// SetQueryError sets the error to return from Query.
func (m *MockAgentHandler) SetQueryError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryErr = err
}

// GetQueryCalls returns all recorded query calls.
func (m *MockAgentHandler) GetQueryCalls() []QueryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]QueryCall, len(m.queryCalls))
	copy(calls, m.queryCalls)
	return calls
}

// MockWorker is a test implementation of the queue.Worker interface.
type MockWorker struct {
	id      string
	started bool
	stopped bool
}

// NewMockWorker creates a new mock worker.
func NewMockWorker(id string) *MockWorker {
	return &MockWorker{id: id}
}

// Start implements the Worker interface.
func (m *MockWorker) Start(ctx context.Context) error {
	m.started = true
	<-ctx.Done()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context error: %w", err)
	}
	return nil
}

// Stop implements the Worker interface.
func (m *MockWorker) Stop() error {
	m.stopped = true
	return nil
}

// ID implements the Worker interface.
func (m *MockWorker) ID() string {
	return m.id
}

// MockStateMachine is a test implementation of the queue.StateMachine interface.
type MockStateMachine struct {
	allowedTransitions map[queue.State][]queue.State
	transitions        []StateTransition
}

// StateTransition records a state transition.
type StateTransition struct {
	Message *queue.Message
	From    queue.State
	To      queue.State
}

// NewMockStateMachine creates a new mock state machine.
func NewMockStateMachine() *MockStateMachine {
	return &MockStateMachine{
		allowedTransitions: map[queue.State][]queue.State{
			queue.StateQueued:     {queue.StateProcessing},
			queue.StateProcessing: {queue.StateValidating, queue.StateFailed, queue.StateRetrying},
			queue.StateValidating: {queue.StateCompleted, queue.StateFailed},
			queue.StateRetrying:   {queue.StateQueued},
			queue.StateCompleted:  {}, // Terminal state
			queue.StateFailed:     {}, // Terminal state
		},
		transitions: make([]StateTransition, 0),
	}
}

// CanTransition implements the StateMachine interface.
func (m *MockStateMachine) CanTransition(from, to queue.State) bool {
	allowed, ok := m.allowedTransitions[from]
	if !ok {
		return false
	}
	for _, state := range allowed {
		if state == to {
			return true
		}
	}
	return false
}

// Transition implements the StateMachine interface.
func (m *MockStateMachine) Transition(msg *queue.Message, to queue.State) error {
	if msg == nil {
		return fmt.Errorf("cannot transition nil message")
	}
	currentState := msg.GetState()
	if !m.CanTransition(currentState, to) {
		return fmt.Errorf("invalid transition from %v to %v", currentState, to)
	}

	m.transitions = append(m.transitions, StateTransition{
		Message: msg,
		From:    currentState,
		To:      to,
	})

	msg.SetState(to)
	return nil
}

// IsTerminal implements the StateMachine interface.
func (m *MockStateMachine) IsTerminal(state queue.State) bool {
	// Terminal states have no outgoing transitions
	_, hasTransitions := m.allowedTransitions[state]
	return !hasTransitions
}

// MockStorage is a test implementation of the storage.Storage interface.
type MockStorage struct {
	err        error
	messages   map[string]*storage.StoredMessage
	queueItems map[string]*queue.Message
	llmCalls   map[string][]*storage.LLMCall
	sessions   map[string]*storage.Session
	mu         sync.Mutex
}

// NewMockStorage creates a new mock storage.
func NewMockStorage() *MockStorage {
	return &MockStorage{
		messages:   make(map[string]*storage.StoredMessage),
		queueItems: make(map[string]*queue.Message),
		llmCalls:   make(map[string][]*storage.LLMCall),
		sessions:   make(map[string]*storage.Session),
	}
}

// SaveMessage implements the Storage interface.
func (m *MockStorage) SaveMessage(msg *storage.StoredMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	m.messages[msg.ID] = msg
	return nil
}

// GetMessage implements the Storage interface.
func (m *MockStorage) GetMessage(messageID string) (*storage.StoredMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	msg, ok := m.messages[messageID]
	if !ok {
		return nil, fmt.Errorf("message not found")
	}
	return msg, nil
}

// GetConversationHistory implements the Storage interface.
func (m *MockStorage) GetConversationHistory(
	userID string,
	limit int,
) ([]*storage.StoredMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var history []*storage.StoredMessage
	for _, msg := range m.messages {
		if msg.From == userID || msg.To == userID {
			history = append(history, msg)
		}
	}

	if len(history) > limit {
		history = history[len(history)-limit:]
	}

	return history, nil
}

// SaveQueueItem implements the Storage interface.
func (m *MockStorage) SaveQueueItem(item *queue.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queueItems[item.ID] = item
	return nil
}

// UpdateQueueItemState implements the Storage interface.
func (m *MockStorage) UpdateQueueItemState(
	itemID string,
	state queue.State,
	_ map[string]any,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if item, ok := m.queueItems[itemID]; ok {
		item.SetState(state)
	}
	return nil
}

// GetPendingQueueItems implements the Storage interface.
func (m *MockStorage) GetPendingQueueItems() ([]*queue.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var pending []*queue.Message
	for _, item := range m.queueItems {
		if item.GetState() == queue.StateQueued || item.GetState() == queue.StateProcessing {
			pending = append(pending, item)
		}
	}
	return pending, nil
}

// GetQueueItemHistory implements the Storage interface.
func (m *MockStorage) GetQueueItemHistory(_ string) ([]*storage.StateTransition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return []*storage.StateTransition{}, nil
}

// SaveLLMCall implements the Storage interface.
func (m *MockStorage) SaveLLMCall(call *storage.LLMCall) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.llmCalls[call.MessageID] = append(m.llmCalls[call.MessageID], call)
	return nil
}

// GetLLMCallsForMessage implements the Storage interface.
func (m *MockStorage) GetLLMCallsForMessage(messageID string) ([]*storage.LLMCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.llmCalls[messageID], nil
}

// GetLLMCostReport implements the Storage interface.
func (m *MockStorage) GetLLMCostReport(
	userID string,
	start, end time.Time,
) (*storage.CostReport, error) {
	return &storage.CostReport{
		UserID:    userID,
		StartTime: start,
		EndTime:   end,
	}, nil
}

// SaveSession implements the Storage interface.
func (m *MockStorage) SaveSession(session *storage.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[session.ID] = session
	return nil
}

// UpdateSessionActivity implements the Storage interface.
func (m *MockStorage) UpdateSessionActivity(sessionID string, lastActivity time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[sessionID]; ok {
		session.LastActivity = lastActivity
	}
	return nil
}

// GetActiveSession implements the Storage interface.
func (m *MockStorage) GetActiveSession(userID string) (*storage.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, session := range m.sessions {
		if session.UserID == userID {
			return session, nil
		}
	}
	return nil, fmt.Errorf("no active session")
}

// ExpireSessions implements the Storage interface.
func (m *MockStorage) ExpireSessions(before time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id, session := range m.sessions {
		if session.ExpiresAt.Before(before) {
			delete(m.sessions, id)
			count++
		}
	}
	return count, nil
}

// SaveRateLimitState implements the Storage interface.
func (m *MockStorage) SaveRateLimitState(_ string, _ *storage.RateLimitState) error {
	return nil
}

// GetRateLimitState implements the Storage interface.
func (m *MockStorage) GetRateLimitState(userID string) (*storage.RateLimitState, error) {
	return &storage.RateLimitState{UserID: userID}, nil
}

// SaveCircuitBreakerState implements the Storage interface.
func (m *MockStorage) SaveCircuitBreakerState(_ string, _ *storage.CircuitBreakerState) error {
	return nil
}

// GetCircuitBreakerState implements the Storage interface.
func (m *MockStorage) GetCircuitBreakerState(service string) (*storage.CircuitBreakerState, error) {
	return &storage.CircuitBreakerState{Service: service, State: "CLOSED"}, nil
}

// RecordMetric implements the Storage interface.
func (m *MockStorage) RecordMetric(_ *storage.SystemMetric) error {
	return nil
}

// GetMetricsReport implements the Storage interface.
func (m *MockStorage) GetMetricsReport(start, end time.Time) (*storage.MetricsReport, error) {
	return &storage.MetricsReport{
		StartTime: start,
		EndTime:   end,
	}, nil
}

// PurgeOldData implements the Storage interface.
func (m *MockStorage) PurgeOldData(_ time.Time, _ string) (int64, error) {
	return 0, nil
}

// Vacuum implements the Storage interface.
func (m *MockStorage) Vacuum() error {
	return nil
}

// MockScheduler is a test implementation of the scheduler.Scheduler interface.
type MockScheduler struct {
	jobs    map[string]scheduler.Job
	mu      sync.Mutex
	started bool
	stopped bool
}

// NewMockScheduler creates a new mock scheduler.
func NewMockScheduler() *MockScheduler {
	return &MockScheduler{
		jobs: make(map[string]scheduler.Job),
	}
}

// Schedule implements the Scheduler interface.
func (m *MockScheduler) Schedule(job scheduler.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.jobs[job.ID] = job
	return nil
}

// Remove implements the Scheduler interface.
func (m *MockScheduler) Remove(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.jobs, jobID)
	return nil
}

// Start implements the Scheduler interface.
func (m *MockScheduler) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.started = true
	return nil
}

// Stop implements the Scheduler interface.
func (m *MockScheduler) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopped = true
	return nil
}

// List implements the Scheduler interface.
func (m *MockScheduler) List() []scheduler.Job {
	m.mu.Lock()
	defer m.mu.Unlock()

	jobs := make([]scheduler.Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// MockJobHandler is a test implementation of the scheduler.JobHandler interface.
type MockJobHandler struct {
	name    string
	execErr error
	calls   []time.Time
	mu      sync.Mutex
}

// NewMockJobHandler creates a new mock job handler.
func NewMockJobHandler(name string) *MockJobHandler {
	return &MockJobHandler{
		name:  name,
		calls: make([]time.Time, 0),
	}
}

// Execute implements the JobHandler interface.
func (m *MockJobHandler) Execute(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, time.Now())
	return m.execErr
}

// Name implements the JobHandler interface.
func (m *MockJobHandler) Name() string {
	return m.name
}

// SetExecuteError sets the error to return from Execute.
func (m *MockJobHandler) SetExecuteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.execErr = err
}

// GetCalls returns all recorded execution times.
func (m *MockJobHandler) GetCalls() []time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]time.Time, len(m.calls))
	copy(calls, m.calls)
	return calls
}

// MockSignalManager is a test implementation of the signal.Manager interface.
type MockSignalManager struct {
	started       bool
	stopped       bool
	registered    bool
	phoneNumber   string
	healthErr     error
	registerErr   error
	verifyErr     error
	messenger     *MockMessenger
	deviceManager *MockDeviceManager
	mu            sync.Mutex
}

// NewMockSignalManager creates a new mock Signal manager.
func NewMockSignalManager() *MockSignalManager {
	return &MockSignalManager{
		phoneNumber:   "+1234567890",
		messenger:     NewMockMessenger(),
		deviceManager: NewMockDeviceManager(),
	}
}

// Start implements the Manager interface.
func (m *MockSignalManager) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	return nil
}

// Stop implements the Manager interface.
func (m *MockSignalManager) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	return nil
}

// HealthCheck implements the Manager interface.
func (m *MockSignalManager) HealthCheck(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.healthErr
}

// GetMessenger implements the Manager interface.
func (m *MockSignalManager) GetMessenger() signal.Messenger {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messenger
}

// GetDeviceManager implements the Manager interface.
//
//nolint:ireturn // Must return interface to satisfy signal.Manager interface
func (m *MockSignalManager) GetDeviceManager() signal.DeviceManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deviceManager
}

// GetPhoneNumber implements the Manager interface.
func (m *MockSignalManager) GetPhoneNumber() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phoneNumber
}

// IsRegistered implements the Manager interface.
func (m *MockSignalManager) IsRegistered(_ context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registered, nil
}

// Register implements the Manager interface.
func (m *MockSignalManager) Register(_ context.Context, phoneNumber string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.registerErr != nil {
		return m.registerErr
	}

	m.phoneNumber = phoneNumber
	m.registered = true
	return nil
}

// VerifyCode implements the Manager interface.
func (m *MockSignalManager) VerifyCode(_ context.Context, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.verifyErr != nil {
		return m.verifyErr
	}

	m.registered = true
	return nil
}

// SetHealthError sets the error to return from HealthCheck.
func (m *MockSignalManager) SetHealthError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthErr = err
}

// SetRegistered sets the registration status.
func (m *MockSignalManager) SetRegistered(registered bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registered = registered
}

// SetRegisterError sets the error to return from Register.
func (m *MockSignalManager) SetRegisterError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registerErr = err
}

// SetVerifyError sets the error to return from VerifyCode.
func (m *MockSignalManager) SetVerifyError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifyErr = err
}

// IsStarted returns whether Start was called.
func (m *MockSignalManager) IsStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

// IsStopped returns whether Stop was called.
func (m *MockSignalManager) IsStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

// MockProcessManager is a test implementation of the signal.ProcessManager interface.
type MockProcessManager struct {
	running      bool
	pid          int
	startErr     error
	stopErr      error
	restartErr   error
	readyErr     error
	startCalls   int
	stopCalls    int
	restartCalls int
	mu           sync.Mutex
}

// NewMockProcessManager creates a new mock process manager.
func NewMockProcessManager() *MockProcessManager {
	return &MockProcessManager{
		pid: mockProcessPID,
	}
}

// Start implements the ProcessManager interface.
func (m *MockProcessManager) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.startCalls++
	if m.startErr != nil {
		return m.startErr
	}

	m.running = true
	return nil
}

// Stop implements the ProcessManager interface.
func (m *MockProcessManager) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopCalls++
	if m.stopErr != nil {
		return m.stopErr
	}

	m.running = false
	m.pid = 0
	return nil
}

// Restart implements the ProcessManager interface.
func (m *MockProcessManager) Restart(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.restartCalls++
	if m.restartErr != nil {
		return m.restartErr
	}

	// Simulate restart by toggling running state
	m.running = false
	m.running = true
	return nil
}

// IsRunning implements the ProcessManager interface.
func (m *MockProcessManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// GetPID implements the ProcessManager interface.
func (m *MockProcessManager) GetPID() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return m.pid
	}
	return 0
}

// WaitForReady implements the ProcessManager interface.
func (m *MockProcessManager) WaitForReady(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readyErr != nil {
		return m.readyErr
	}

	if !m.running {
		return fmt.Errorf("process not running")
	}

	return nil
}

// SetRunning sets the running state.
func (m *MockProcessManager) SetRunning(running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = running
}

// SetStartError sets the error to return from Start.
func (m *MockProcessManager) SetStartError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startErr = err
}

// SetStopError sets the error to return from Stop.
func (m *MockProcessManager) SetStopError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopErr = err
}

// SetReadyError sets the error to return from WaitForReady.
func (m *MockProcessManager) SetReadyError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readyErr = err
}

// GetStartCalls returns the number of times Start was called.
func (m *MockProcessManager) GetStartCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCalls
}

// GetStopCalls returns the number of times Stop was called.
func (m *MockProcessManager) GetStopCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopCalls
}

// MockDeviceManager is a test implementation of the signal.DeviceManager interface.
type MockDeviceManager struct {
	devices          []signal.Device
	primaryDevice    *signal.Device
	listErr          error
	removeErr        error
	linkErr          error
	waitErr          error
	linkedDevice     *signal.Device
	linkingURI       string
	removedDeviceIDs []int
	mu               sync.Mutex
}

// NewMockDeviceManager creates a new mock device manager.
func NewMockDeviceManager() *MockDeviceManager {
	primary := &signal.Device{
		ID:       1,
		Name:     "Primary Device",
		Primary:  true,
		Created:  time.Now().Add(-24 * time.Hour),
		LastSeen: time.Now(),
	}

	return &MockDeviceManager{
		devices: []signal.Device{
			*primary,
			{
				ID:       secondaryDeviceID,
				Name:     "Desktop",
				Primary:  false,
				Created:  time.Now().Add(-12 * time.Hour),
				LastSeen: time.Now().Add(-1 * time.Hour),
			},
		},
		primaryDevice:    primary,
		removedDeviceIDs: make([]int, 0),
	}
}

// ListDevices implements the DeviceManager interface.
func (m *MockDeviceManager) ListDevices(_ context.Context) ([]signal.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listErr != nil {
		return nil, m.listErr
	}

	devices := make([]signal.Device, len(m.devices))
	copy(devices, m.devices)
	return devices, nil
}

// RemoveDevice implements the DeviceManager interface.
func (m *MockDeviceManager) RemoveDevice(_ context.Context, deviceID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.removeErr != nil {
		return m.removeErr
	}

	// Track removed device IDs
	m.removedDeviceIDs = append(m.removedDeviceIDs, deviceID)

	// Remove from devices list
	filtered := make([]signal.Device, 0, len(m.devices))
	for _, d := range m.devices {
		if d.ID != deviceID {
			filtered = append(filtered, d)
		}
	}
	m.devices = filtered

	return nil
}

// GenerateLinkingURI implements the DeviceManager interface.
func (m *MockDeviceManager) GenerateLinkingURI(_ context.Context, deviceName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.linkErr != nil {
		return "", m.linkErr
	}

	m.linkingURI = fmt.Sprintf("signal://link?device=%s&token=mock", deviceName)
	return m.linkingURI, nil
}

// GetPrimaryDevice implements the DeviceManager interface.
func (m *MockDeviceManager) GetPrimaryDevice(_ context.Context) (*signal.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.primaryDevice == nil {
		return nil, fmt.Errorf("no primary device")
	}

	return m.primaryDevice, nil
}

// WaitForDeviceLink implements the DeviceManager interface.
func (m *MockDeviceManager) WaitForDeviceLink(_ context.Context, _ time.Duration) (*signal.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.waitErr != nil {
		return nil, m.waitErr
	}

	if m.linkedDevice != nil {
		return m.linkedDevice, nil
	}

	// Simulate successful linking
	newDevice := &signal.Device{
		ID:       len(m.devices) + 1,
		Name:     "New Device",
		Primary:  false,
		Created:  time.Now(),
		LastSeen: time.Now(),
	}

	m.devices = append(m.devices, *newDevice)
	return newDevice, nil
}

// SetDevices sets the mock device list.
func (m *MockDeviceManager) SetDevices(devices []signal.Device) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices = devices
}

// SetListError sets the error to return from ListDevices.
func (m *MockDeviceManager) SetListError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listErr = err
}

// SetRemoveError sets the error to return from RemoveDevice.
func (m *MockDeviceManager) SetRemoveError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeErr = err
}

// SetLinkError sets the error to return from GenerateLinkingURI.
func (m *MockDeviceManager) SetLinkError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.linkErr = err
}

// SetWaitError sets the error to return from WaitForDeviceLink.
func (m *MockDeviceManager) SetWaitError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.waitErr = err
}

// SetLinkedDevice sets the device to return from WaitForDeviceLink.
func (m *MockDeviceManager) SetLinkedDevice(device *signal.Device) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.linkedDevice = device
}

// GetRemovedDeviceIDs returns the IDs of removed devices.
func (m *MockDeviceManager) GetRemovedDeviceIDs() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int, len(m.removedDeviceIDs))
	copy(ids, m.removedDeviceIDs)
	return ids
}
