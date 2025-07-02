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
)

// Compile-time checks to ensure mocks implement their interfaces.
var (
	_ claude.LLM                  = (*MockLLM)(nil)
	_ signal.Messenger            = (*MockMessenger)(nil)
	_ conversation.SessionManager = (*MockSessionManager)(nil)
	_ queue.RateLimiter           = (*MockRateLimiter)(nil)
	_ agent.ValidationStrategy    = (*MockValidationStrategy)(nil)
	_ agent.IntentEnhancer        = (*MockIntentEnhancer)(nil)
	_ agent.Handler               = (*MockAgentHandler)(nil)
	_ queue.MessageQueue          = (*MockMessageQueue)(nil)
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
	m.incomingChan <- msg
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
	_, _ string,
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
	_, _ string,
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
	calls      []signal.IncomingMessage
	mu         sync.Mutex
}

// NewMockAgentHandler creates a new mock agent handler.
func NewMockAgentHandler() *MockAgentHandler {
	return &MockAgentHandler{
		calls: make([]signal.IncomingMessage, 0),
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

// MockMessageQueue is a test implementation of the queue.MessageQueue interface.
type MockMessageQueue struct {
	err      error
	states   map[string]queue.MessageState
	messages []*queue.QueuedMessage
	stats    queue.Stats
	mu       sync.Mutex
}

// NewMockMessageQueue creates a new mock message queue.
func NewMockMessageQueue() *MockMessageQueue {
	return &MockMessageQueue{
		messages: make([]*queue.QueuedMessage, 0),
		states:   make(map[string]queue.MessageState),
		stats:    queue.Stats{},
	}
}

// Enqueue implements the MessageQueue interface.
func (m *MockMessageQueue) Enqueue(msg signal.IncomingMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	queuedMsg := &queue.QueuedMessage{
		ID:             fmt.Sprintf("msg-%d", len(m.messages)),
		ConversationID: msg.From,
		From:           msg.From,
		Text:           msg.Text,
		State:          queue.MessageStateQueued,
		Priority:       queue.PriorityNormal,
		QueuedAt:       time.Now(),
	}

	m.messages = append(m.messages, queuedMsg)
	m.states[queuedMsg.ID] = queue.MessageStateQueued
	return nil
}

// GetNext implements the MessageQueue interface.
func (m *MockMessageQueue) GetNext(_ string) (*queue.QueuedMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, msg := range m.messages {
		if m.states[msg.ID] == queue.MessageStateQueued {
			return msg, nil
		}
	}
	return nil, fmt.Errorf("no messages available")
}

// UpdateState implements the MessageQueue interface.
func (m *MockMessageQueue) UpdateState(msgID string, state queue.MessageState, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.states[msgID] = state
	return nil
}

// Stats implements the MessageQueue interface.
func (m *MockMessageQueue) Stats() queue.Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
}

// SetError sets the error to return.
func (m *MockMessageQueue) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
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
	queueItems map[string]*queue.QueuedMessage
	llmCalls   map[string][]*storage.LLMCall
	sessions   map[string]*storage.Session
	mu         sync.Mutex
}

// NewMockStorage creates a new mock storage.
func NewMockStorage() *MockStorage {
	return &MockStorage{
		messages:   make(map[string]*storage.StoredMessage),
		queueItems: make(map[string]*queue.QueuedMessage),
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
func (m *MockStorage) SaveQueueItem(item *queue.QueuedMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queueItems[item.ID] = item
	return nil
}

// UpdateQueueItemState implements the Storage interface.
func (m *MockStorage) UpdateQueueItemState(
	itemID string,
	state queue.MessageState,
	_ map[string]any,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if item, ok := m.queueItems[itemID]; ok {
		item.State = state
	}
	return nil
}

// GetPendingQueueItems implements the Storage interface.
func (m *MockStorage) GetPendingQueueItems() ([]*queue.QueuedMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var pending []*queue.QueuedMessage
	for _, item := range m.queueItems {
		if item.State == queue.MessageStateQueued || item.State == queue.MessageStateProcessing {
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
