package mocks

import (
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/Veraticus/mentat/internal/storage"
)

// MessageBuilder creates queue.Message instances for testing.
type MessageBuilder struct {
	msg *queue.Message
}

// NewMessageBuilder creates a new message builder with sensible defaults.
func NewMessageBuilder() *MessageBuilder {
	now := time.Now()
	return &MessageBuilder{
		msg: &queue.Message{
			ID:             "test-msg-123",
			ConversationID: "+15551234567",
			Sender:         "Test User",
			SenderNumber:   "+15551234567",
			Text:           "Test message content",
			State:          queue.StateQueued,
			Attempts:       0,
			MaxAttempts:    3,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
}

// MessageOption is a functional option for MessageBuilder.
type MessageOption func(*MessageBuilder)

// WithID sets the message ID.
func WithID(id string) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.ID = id
	}
}

// WithConversationID sets the conversation ID.
func WithConversationID(conversationID string) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.ConversationID = conversationID
	}
}

// WithText sets the message text.
func WithText(text string) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.Text = text
	}
}

// WithState sets the message state.
func WithState(state queue.State) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.State = state
	}
}

// WithSender sets the message sender.
func WithSender(sender string) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.Sender = sender
	}
}

// WithAttempts sets the number of attempts.
func WithAttempts(attempts int) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.Attempts = attempts
	}
}

// WithMaxAttempts sets the maximum number of attempts.
func WithMaxAttempts(maxAttempts int) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.MaxAttempts = maxAttempts
	}
}

// WithCreatedAt sets when the message was created.
func WithCreatedAt(t time.Time) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.CreatedAt = t
		b.msg.UpdatedAt = t
	}
}

// WithResponse sets the message response.
func WithResponse(response string) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.Response = response
		now := time.Now()
		b.msg.ProcessedAt = &now
	}
}

// WithError sets the error.
func WithError(err error) MessageOption {
	return func(b *MessageBuilder) {
		b.msg.Error = err
	}
}

// Build creates the message with all options applied.
func (b *MessageBuilder) Build(opts ...MessageOption) *queue.Message {
	// Create a new message with the same values (avoiding mutex copy)
	msg := queue.NewMessage(b.msg.ID, b.msg.ConversationID, b.msg.Sender, b.msg.SenderNumber, b.msg.Text)
	msg.State = b.msg.State
	msg.Attempts = b.msg.Attempts
	msg.MaxAttempts = b.msg.MaxAttempts
	msg.CreatedAt = b.msg.CreatedAt
	msg.UpdatedAt = b.msg.UpdatedAt
	msg.Error = b.msg.Error
	msg.Response = b.msg.Response
	if b.msg.ProcessedAt != nil {
		t := *b.msg.ProcessedAt
		msg.ProcessedAt = &t
	}
	
	// Create a temporary builder with the new message
	tempBuilder := &MessageBuilder{msg: msg}
	
	// Apply all options to the copy
	for _, opt := range opts {
		opt(tempBuilder)
	}
	
	return tempBuilder.msg
}

// QueuedMessageBuilder creates queue.QueuedMessage instances for testing.
type QueuedMessageBuilder struct {
	msg *queue.QueuedMessage
}

// NewQueuedMessageBuilder creates a new queued message builder.
func NewQueuedMessageBuilder() *QueuedMessageBuilder {
	return &QueuedMessageBuilder{
		msg: &queue.QueuedMessage{
			ID:             "queued-msg-123",
			ConversationID: "+15551234567",
			From:           "+15551234567",
			Text:           "Test message",
			State:          queue.MessageStateQueued,
			Priority:       queue.PriorityNormal,
			Attempts:       0,
			QueuedAt:       time.Now(),
		},
	}
}

// QueuedMessageOption is a functional option for QueuedMessageBuilder.
type QueuedMessageOption func(*QueuedMessageBuilder)

// WithQueuedMessageID sets the message ID.
func WithQueuedMessageID(id string) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.ID = id
	}
}

// WithQueuedConversationID sets the conversation ID.
func WithQueuedConversationID(conversationID string) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.ConversationID = conversationID
	}
}

// WithQueuedFrom sets the sender.
func WithQueuedFrom(from string) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.From = from
	}
}

// WithQueuedText sets the message text.
func WithQueuedText(text string) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.Text = text
	}
}

// WithQueuedState sets the message state.
func WithQueuedState(state queue.MessageState) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.State = state
	}
}

// WithQueuedPriority sets the priority.
func WithQueuedPriority(priority queue.Priority) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.Priority = priority
	}
}

// WithQueuedAttempts sets the number of attempts.
func WithQueuedAttempts(attempts int) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.Attempts = attempts
	}
}

// WithQueuedAt sets when the message was queued.
func WithQueuedAt(t time.Time) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.QueuedAt = t
	}
}

// WithQueuedError sets the last error.
func WithQueuedError(err error) QueuedMessageOption {
	return func(b *QueuedMessageBuilder) {
		b.msg.LastError = err
	}
}

// Build creates the queued message with all options applied.
func (b *QueuedMessageBuilder) Build(opts ...QueuedMessageOption) *queue.QueuedMessage {
	// Create a new message copying all fields except the mutex
	msg := queue.QueuedMessage{
		ID:             b.msg.ID,
		ConversationID: b.msg.ConversationID,
		From:           b.msg.From,
		Text:           b.msg.Text,
		Priority:       b.msg.Priority,
		State:          b.msg.State,
		StateHistory:   b.msg.StateHistory,
		QueuedAt:       b.msg.QueuedAt,
		ProcessedAt:    b.msg.ProcessedAt,
		CompletedAt:    b.msg.CompletedAt,
		LastError:      b.msg.LastError,
		ErrorHistory:   b.msg.ErrorHistory,
		Attempts:       b.msg.Attempts,
		MaxAttempts:    b.msg.MaxAttempts,
		NextRetryAt:    b.msg.NextRetryAt,
	}
	
	// Create a temporary builder with the copy
	tempBuilder := &QueuedMessageBuilder{msg: &msg}
	
	// Apply all options to the copy
	for _, opt := range opts {
		opt(tempBuilder)
	}
	
	return tempBuilder.msg
}

// LLMResponseBuilder creates claude.LLMResponse instances for testing.
type LLMResponseBuilder struct {
	resp *claude.LLMResponse
}

// NewLLMResponseBuilder creates a new LLM response builder.
func NewLLMResponseBuilder() *LLMResponseBuilder {
	return &LLMResponseBuilder{
		resp: &claude.LLMResponse{
			Message:   "I can help you with that.",
			ToolCalls: []claude.ToolCall{},
			Metadata: claude.ResponseMetadata{
				ModelVersion: "claude-3-opus-20240229",
				Latency:      1500 * time.Millisecond,
				TokensUsed:   100,
			},
		},
	}
}

// LLMResponseOption is a functional option for LLMResponseBuilder.
type LLMResponseOption func(*LLMResponseBuilder)

// WithMessage sets the response message.
func WithMessage(message string) LLMResponseOption {
	return func(b *LLMResponseBuilder) {
		b.resp.Message = message
	}
}

// WithToolCalls adds tool calls to the response.
func WithToolCalls(calls ...claude.ToolCall) LLMResponseOption {
	return func(b *LLMResponseBuilder) {
		b.resp.ToolCalls = calls
	}
}

// WithModelVersion sets the model version used.
func WithModelVersion(version string) LLMResponseOption {
	return func(b *LLMResponseBuilder) {
		b.resp.Metadata.ModelVersion = version
	}
}

// WithLatency sets the response latency.
func WithLatency(latency time.Duration) LLMResponseOption {
	return func(b *LLMResponseBuilder) {
		b.resp.Metadata.Latency = latency
	}
}

// WithTokensUsed sets token count.
func WithTokensUsed(tokens int) LLMResponseOption {
	return func(b *LLMResponseBuilder) {
		b.resp.Metadata.TokensUsed = tokens
	}
}

// Build creates the response with all options applied.
func (b *LLMResponseBuilder) Build(opts ...LLMResponseOption) *claude.LLMResponse {
	// Create a copy of the response
	resp := *b.resp
	resp.ToolCalls = make([]claude.ToolCall, len(b.resp.ToolCalls))
	copy(resp.ToolCalls, b.resp.ToolCalls)
	
	// Create a temporary builder with the copy
	tempBuilder := &LLMResponseBuilder{resp: &resp}
	
	// Apply all options to the copy
	for _, opt := range opts {
		opt(tempBuilder)
	}
	
	return tempBuilder.resp
}

// ValidationResultBuilder creates agent.ValidationResult instances.
type ValidationResultBuilder struct {
	result *agent.ValidationResult
}

// NewValidationResultBuilder creates a new validation result builder.
func NewValidationResultBuilder() *ValidationResultBuilder {
	return &ValidationResultBuilder{
		result: &agent.ValidationResult{
			Status:      agent.ValidationStatusSuccess,
			Confidence:  0.95,
			Issues:      []string{},
			Suggestions: []string{},
			Metadata:    make(map[string]string),
		},
	}
}

// ValidationResultOption is a functional option for ValidationResultBuilder.
type ValidationResultOption func(*ValidationResultBuilder)

// WithStatus sets the validation status.
func WithStatus(status agent.ValidationStatus) ValidationResultOption {
	return func(b *ValidationResultBuilder) {
		b.result.Status = status
	}
}

// WithConfidence sets the confidence score.
func WithConfidence(confidence float64) ValidationResultOption {
	return func(b *ValidationResultBuilder) {
		b.result.Confidence = confidence
	}
}

// WithIssues adds validation issues.
func WithIssues(issues ...string) ValidationResultOption {
	return func(b *ValidationResultBuilder) {
		b.result.Issues = issues
	}
}

// WithSuggestions adds suggestions.
func WithSuggestions(suggestions ...string) ValidationResultOption {
	return func(b *ValidationResultBuilder) {
		b.result.Suggestions = suggestions
	}
}

// WithMetadata sets validation metadata.
func WithMetadata(metadata map[string]string) ValidationResultOption {
	return func(b *ValidationResultBuilder) {
		b.result.Metadata = metadata
	}
}

// Build creates the result with all options applied.
func (b *ValidationResultBuilder) Build(opts ...ValidationResultOption) *agent.ValidationResult {
	// Create a copy of the result
	result := *b.result
	result.Issues = make([]string, len(b.result.Issues))
	copy(result.Issues, b.result.Issues)
	result.Suggestions = make([]string, len(b.result.Suggestions))
	copy(result.Suggestions, b.result.Suggestions)
	result.Metadata = make(map[string]string)
	for k, v := range b.result.Metadata {
		result.Metadata[k] = v
	}
	
	// Create a temporary builder with the copy
	tempBuilder := &ValidationResultBuilder{result: &result}
	
	// Apply all options to the copy
	for _, opt := range opts {
		opt(tempBuilder)
	}
	
	return tempBuilder.result
}

// SignalMessageBuilder creates signal.Message instances.
type SignalMessageBuilder struct {
	msg *signal.Message
}

// NewSignalMessageBuilder creates a new Signal message builder.
func NewSignalMessageBuilder() *SignalMessageBuilder {
	return &SignalMessageBuilder{
		msg: &signal.Message{
			ID:        "signal-msg-123",
			Sender:    "+15551234567",
			Recipient: "mentat",
			Text:      "Hello from Signal",
			Timestamp: time.Now(),
		},
	}
}

// SignalMessageOption is a functional option for SignalMessageBuilder.
type SignalMessageOption func(*SignalMessageBuilder)

// WithSignalID sets the message ID.
func WithSignalID(id string) SignalMessageOption {
	return func(b *SignalMessageBuilder) {
		b.msg.ID = id
	}
}

// WithSignalSender sets the message sender.
func WithSignalSender(sender string) SignalMessageOption {
	return func(b *SignalMessageBuilder) {
		b.msg.Sender = sender
	}
}

// WithRecipient sets the message recipient.
func WithRecipient(recipient string) SignalMessageOption {
	return func(b *SignalMessageBuilder) {
		b.msg.Recipient = recipient
	}
}

// WithSignalText sets the message text.
func WithSignalText(text string) SignalMessageOption {
	return func(b *SignalMessageBuilder) {
		b.msg.Text = text
	}
}

// WithSignalTimestamp sets the message timestamp.
func WithSignalTimestamp(ts time.Time) SignalMessageOption {
	return func(b *SignalMessageBuilder) {
		b.msg.Timestamp = ts
	}
}

// WithDeliveryStatus sets delivery status.
func WithDeliveryStatus(delivered, read bool) SignalMessageOption {
	return func(b *SignalMessageBuilder) {
		b.msg.IsDelivered = delivered
		b.msg.IsRead = read
	}
}

// Build creates the message with all options applied.
func (b *SignalMessageBuilder) Build(opts ...SignalMessageOption) *signal.Message {
	// Create a copy of the message
	msg := *b.msg
	
	// Create a temporary builder with the copy
	tempBuilder := &SignalMessageBuilder{msg: &msg}
	
	// Apply all options to the copy
	for _, opt := range opts {
		opt(tempBuilder)
	}
	
	return tempBuilder.msg
}

// ScenarioBuilder creates complete test scenarios.
type ScenarioBuilder struct {
	name          string
	messages      []*queue.Message
	responses     map[string]*claude.LLMResponse
	validations   map[string]*agent.ValidationResult
	expectations  []Expectation
}

// Expectation represents an expected outcome in a scenario.
type Expectation struct {
	ResponseContains string
	MessageID        string
	FailReason       string
	FinalState       queue.State
	ShouldFail       bool
}

// NewScenarioBuilder creates a new scenario builder.
func NewScenarioBuilder(name string) *ScenarioBuilder {
	return &ScenarioBuilder{
		name:        name,
		messages:    []*queue.Message{},
		responses:   make(map[string]*claude.LLMResponse),
		validations: make(map[string]*agent.ValidationResult),
		expectations: []Expectation{},
	}
}

// ScenarioOption is a functional option for ScenarioBuilder.
type ScenarioOption func(*ScenarioBuilder)

// WithMessages adds messages to the scenario.
func WithMessages(messages ...*queue.Message) ScenarioOption {
	return func(b *ScenarioBuilder) {
		b.messages = append(b.messages, messages...)
	}
}

// WithLLMResponse maps a message ID to an expected LLM response.
func WithLLMResponse(messageID string, response *claude.LLMResponse) ScenarioOption {
	return func(b *ScenarioBuilder) {
		b.responses[messageID] = response
	}
}

// WithValidation maps a message ID to a validation result.
func WithValidation(messageID string, result *agent.ValidationResult) ScenarioOption {
	return func(b *ScenarioBuilder) {
		b.validations[messageID] = result
	}
}

// WithExpectation adds an expected outcome.
func WithExpectation(exp Expectation) ScenarioOption {
	return func(b *ScenarioBuilder) {
		b.expectations = append(b.expectations, exp)
	}
}

// Build creates the scenario with all options applied.
func (b *ScenarioBuilder) Build(opts ...ScenarioOption) *Scenario {
	for _, opt := range opts {
		opt(b)
	}
	
	return &Scenario{
		Name:         b.name,
		Messages:     b.messages,
		Responses:    b.responses,
		Validations:  b.validations,
		Expectations: b.expectations,
	}
}

// Scenario represents a complete test scenario.
type Scenario struct {
	Name         string
	Messages     []*queue.Message
	Responses    map[string]*claude.LLMResponse
	Validations  map[string]*agent.ValidationResult
	Expectations []Expectation
}

// ConversationMessageBuilder creates conversation.Message instances.
type ConversationMessageBuilder struct {
	msg *conversation.Message
}

// NewConversationMessageBuilder creates a new conversation message builder.
func NewConversationMessageBuilder() *ConversationMessageBuilder {
	return &ConversationMessageBuilder{
		msg: &conversation.Message{
			ID:        "conv-msg-123",
			From:      "+15551234567",
			Text:      "Test conversation message",
			Response:  "",
			Timestamp: time.Now(),
			SessionID: "session-123",
		},
	}
}

// ConversationMessageOption is a functional option for ConversationMessageBuilder.
type ConversationMessageOption func(*ConversationMessageBuilder)

// WithConversationMessageID sets the message ID.
func WithConversationMessageID(id string) ConversationMessageOption {
	return func(b *ConversationMessageBuilder) {
		b.msg.ID = id
	}
}

// WithConversationFrom sets who the message is from.
func WithConversationFrom(from string) ConversationMessageOption {
	return func(b *ConversationMessageBuilder) {
		b.msg.From = from
	}
}

// WithConversationText sets the message text.
func WithConversationText(text string) ConversationMessageOption {
	return func(b *ConversationMessageBuilder) {
		b.msg.Text = text
	}
}

// WithConversationResponse sets the response.
func WithConversationResponse(response string) ConversationMessageOption {
	return func(b *ConversationMessageBuilder) {
		b.msg.Response = response
	}
}

// WithConversationTimestamp sets when the message was sent.
func WithConversationTimestamp(t time.Time) ConversationMessageOption {
	return func(b *ConversationMessageBuilder) {
		b.msg.Timestamp = t
	}
}

// WithConversationSessionID sets the session ID.
func WithConversationSessionID(sessionID string) ConversationMessageOption {
	return func(b *ConversationMessageBuilder) {
		b.msg.SessionID = sessionID
	}
}

// Build creates the message with all options applied.
func (b *ConversationMessageBuilder) Build(opts ...ConversationMessageOption) *conversation.Message {
	// Create a copy of the message
	msg := *b.msg
	
	// Create a temporary builder with the copy
	tempBuilder := &ConversationMessageBuilder{msg: &msg}
	
	// Apply all options to the copy
	for _, opt := range opts {
		opt(tempBuilder)
	}
	
	return tempBuilder.msg
}

// StoredMessageBuilder creates storage.StoredMessage instances.
type StoredMessageBuilder struct {
	msg *storage.StoredMessage
}

// NewStoredMessageBuilder creates a new stored message builder.
func NewStoredMessageBuilder() *StoredMessageBuilder {
	return &StoredMessageBuilder{
		msg: &storage.StoredMessage{
			ID:              "stored-msg-123",
			ConversationID:  "+15551234567",
			From:            "+15551234567",
			To:              "mentat",
			Content:         "Stored message",
			Direction:       storage.INBOUND,
			Timestamp:       time.Now(),
			ProcessingState: "completed",
			Metadata:        nil,
		},
	}
}

// StoredMessageOption is a functional option for StoredMessageBuilder.  
type StoredMessageOption func(*StoredMessageBuilder)

// WithStoredMessageID sets the message ID.
func WithStoredMessageID(id string) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.ID = id
	}
}

// WithStoredConversationID sets the conversation ID.
func WithStoredConversationID(conversationID string) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.ConversationID = conversationID
	}
}

// WithStoredFrom sets the sender.
func WithStoredFrom(from string) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.From = from
	}
}

// WithStoredTo sets the recipient.
func WithStoredTo(to string) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.To = to
	}
}

// WithStoredContent sets the message content.
func WithStoredContent(content string) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.Content = content
	}
}

// WithStoredDirection sets the message direction.
func WithStoredDirection(direction storage.MessageDirection) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.Direction = direction
	}
}

// WithStoredTimestamp sets when the message was stored.
func WithStoredTimestamp(t time.Time) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.Timestamp = t
	}
}

// WithStoredProcessingState sets the processing state.
func WithStoredProcessingState(state string) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.ProcessingState = state
	}
}

// WithStoredMetadata sets the metadata.
func WithStoredMetadata(metadata []byte) StoredMessageOption {
	return func(b *StoredMessageBuilder) {
		b.msg.Metadata = metadata
	}
}

// Build creates the message with all options applied.
func (b *StoredMessageBuilder) Build(opts ...StoredMessageOption) *storage.StoredMessage {
	// Create a copy of the message
	msg := *b.msg
	
	// Create a temporary builder with the copy
	tempBuilder := &StoredMessageBuilder{msg: &msg}
	
	// Apply all options to the copy
	for _, opt := range opts {
		opt(tempBuilder)
	}
	
	return tempBuilder.msg
}

// Helper functions for common test scenarios

// CreateSimpleConversation creates a basic conversation flow.
func CreateSimpleConversation(userMessage, assistantResponse string) *Scenario {
	msgID := "test-msg-" + time.Now().Format("20060102150405")
	
	msg := NewMessageBuilder().Build(
		WithID(msgID),
		WithText(userMessage),
	)
	
	resp := NewLLMResponseBuilder().Build(
		WithMessage(assistantResponse),
	)
	
	validation := NewValidationResultBuilder().Build(
		WithStatus(agent.ValidationStatusSuccess),
		WithConfidence(0.98),
	)
	
	return NewScenarioBuilder("Simple Conversation").Build(
		WithMessages(msg),
		WithLLMResponse(msgID, resp),
		WithValidation(msgID, validation),
		WithExpectation(Expectation{
			MessageID:        msgID,
			FinalState:      queue.StateCompleted,
			ResponseContains: assistantResponse,
		}),
	)
}

// CreateFailureScenario creates a scenario where LLM fails.
func CreateFailureScenario() *Scenario {
	msgID := "fail-msg-" + time.Now().Format("20060102150405")
	
	msg := NewMessageBuilder().Build(
		WithID(msgID),
		WithText("This will fail"),
	)
	
	resp := NewLLMResponseBuilder().Build(
		WithMessage("Error: Service temporarily unavailable"),
	)
	
	return NewScenarioBuilder("Failure Scenario").Build(
		WithMessages(msg),
		WithLLMResponse(msgID, resp),
		WithExpectation(Expectation{
			MessageID:  msgID,
			FinalState: queue.StateFailed,
			ShouldFail: true,
			FailReason: "Service temporarily unavailable",
		}),
	)
}

// CreateRetryScenario creates a scenario requiring validation retry.
func CreateRetryScenario() *Scenario {
	msgID := "retry-msg-" + time.Now().Format("20060102150405")
	
	msg := NewMessageBuilder().Build(
		WithID(msgID),
		WithText("Find my meeting tomorrow"),
	)
	
	// First response - incomplete
	resp1 := NewLLMResponseBuilder().Build(
		WithMessage("You have a meeting tomorrow."),
	)
	
	// Validation says incomplete
	validation1 := NewValidationResultBuilder().Build(
		WithStatus(agent.ValidationStatusIncompleteSearch),
		WithConfidence(0.6),
		WithIssues("Did not check calendar MCP tool"),
		WithMetadata(map[string]string{
			"requires_retry": "true",
			"generated_prompt": "Please check the calendar MCP tool for tomorrow's meetings",
		}),
	)
	
	// Second response - complete
	resp2 := NewLLMResponseBuilder().Build(
		WithMessage("You have a 2pm meeting with John about Q4 planning."),
		WithToolCalls(claude.ToolCall{
			Tool: "calendar_search",
			Parameters: map[string]claude.ToolParameter{
				"date": claude.NewStringParam("tomorrow"),
			},
		}),
	)
	
	// Final validation passes
	validation2 := NewValidationResultBuilder().Build(
		WithStatus(agent.ValidationStatusSuccess),
		WithConfidence(0.95),
	)
	
	return NewScenarioBuilder("Retry Scenario").Build(
		WithMessages(msg),
		WithLLMResponse(msgID+"-attempt1", resp1),
		WithValidation(msgID+"-attempt1", validation1),
		WithLLMResponse(msgID+"-attempt2", resp2),
		WithValidation(msgID+"-attempt2", validation2),
		WithExpectation(Expectation{
			MessageID:        msgID,
			FinalState:      queue.StateCompleted,
			ResponseContains: "2pm meeting with John",
		}),
	)
}

// CreateComplexScenario creates a multi-message conversation scenario.
func CreateComplexScenario() *Scenario {
	// First message
	msg1 := NewMessageBuilder().Build(
		WithID("complex-1"),
		WithText("What's my schedule today?"),
		WithConversationID("+15551234567"),
	)
	
	resp1 := NewLLMResponseBuilder().Build(
		WithMessage("Let me check your calendar. You have 3 meetings today..."),
		WithToolCalls(claude.ToolCall{
			Tool: "calendar_list",
			Parameters: map[string]claude.ToolParameter{},
		}),
	)
	
	// Follow-up message within session window
	msg2 := NewMessageBuilder().Build(
		WithID("complex-2"),
		WithText("Cancel the 2pm one"),
		WithConversationID("+15551234567"),
		WithCreatedAt(time.Now().Add(30 * time.Second)),
	)
	
	resp2 := NewLLMResponseBuilder().Build(
		WithMessage("I've canceled your 2pm meeting with the product team."),
		WithToolCalls(claude.ToolCall{
			Tool: "calendar_cancel",
			Parameters: map[string]claude.ToolParameter{
				"meeting_id": claude.NewStringParam("12345"),
			},
		}),
	)
	
	// Both pass validation
	validation := NewValidationResultBuilder().Build(
		WithStatus(agent.ValidationStatusSuccess),
		WithConfidence(0.99),
	)
	
	return NewScenarioBuilder("Complex Multi-Message").Build(
		WithMessages(msg1, msg2),
		WithLLMResponse("complex-1", resp1),
		WithLLMResponse("complex-2", resp2),
		WithValidation("complex-1", validation),
		WithValidation("complex-2", validation),
		WithExpectation(Expectation{
			MessageID:        "complex-1",
			FinalState:      queue.StateCompleted,
			ResponseContains: "3 meetings today",
		}),
		WithExpectation(Expectation{
			MessageID:        "complex-2",
			FinalState:      queue.StateCompleted,
			ResponseContains: "canceled your 2pm meeting",
		}),
	)
}