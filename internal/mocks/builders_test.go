package mocks_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/mocks"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/Veraticus/mentat/internal/storage"
)

const (
	mentatRecipient = "mentat"
)

func TestMessageBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, msg *queue.Message)
		name     string
		options  []mocks.MessageOption
	}{
		{
			name:     "default message",
			options:  []mocks.MessageOption{},
			validate: validateDefaultMessage,
		},
		{
			name: "custom message",
			options: []mocks.MessageOption{
				mocks.WithID("custom-123"),
				mocks.WithText("Custom content"),
				mocks.WithState(queue.StateProcessing),
				mocks.WithSender("+15559999999"),
				mocks.WithAttempts(2),
			},
			validate: validateCustomMessage,
		},
		{
			name: "message with error",
			options: []mocks.MessageOption{
				mocks.WithState(queue.StateFailed),
				mocks.WithError(fmt.Errorf("Connection timeout")),
			},
			validate: validateMessageWithError,
		},
		{
			name: "message with response",
			options: []mocks.MessageOption{
				mocks.WithState(queue.StateCompleted),
				mocks.WithResponse("Task completed successfully"),
			},
			validate: validateMessageWithResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := mocks.NewMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

// validateDefaultMessage validates a default message.
func validateDefaultMessage(t *testing.T, msg *queue.Message) {
	t.Helper()
	if msg.ID != "test-msg-123" {
		t.Errorf("expected ID test-msg-123, got %s", msg.ID)
	}
	if msg.State != queue.StateQueued {
		t.Errorf("expected state Queued, got %s", msg.State)
	}
	if msg.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", msg.Attempts)
	}
	if msg.MaxAttempts != 3 {
		t.Errorf("expected 3 max attempts, got %d", msg.MaxAttempts)
	}
}

// validateCustomMessage validates a custom message.
func validateCustomMessage(t *testing.T, msg *queue.Message) {
	t.Helper()
	if msg.ID != "custom-123" {
		t.Errorf("expected ID custom-123, got %s", msg.ID)
	}
	if msg.Text != "Custom content" {
		t.Errorf("expected text 'Custom content', got %s", msg.Text)
	}
	if msg.State != queue.StateProcessing {
		t.Errorf("expected state Processing, got %s", msg.State)
	}
	if msg.Sender != "+15559999999" {
		t.Errorf("expected sender +15559999999, got %s", msg.Sender)
	}
	if msg.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", msg.Attempts)
	}
}

// validateMessageWithError validates a message with error.
func validateMessageWithError(t *testing.T, msg *queue.Message) {
	t.Helper()
	if msg.State != queue.StateFailed {
		t.Errorf("expected state Failed, got %s", msg.State)
	}
	if msg.Error == nil || msg.Error.Error() != "Connection timeout" {
		t.Errorf("expected error 'Connection timeout', got %v", msg.Error)
	}
}

// validateMessageWithResponse validates a message with response.
func validateMessageWithResponse(t *testing.T, msg *queue.Message) {
	t.Helper()
	if msg.State != queue.StateCompleted {
		t.Errorf("expected state Completed, got %s", msg.State)
	}
	if msg.Response != "Task completed successfully" {
		t.Errorf("expected response 'Task completed successfully', got %s", msg.Response)
	}
	if msg.ProcessedAt == nil {
		t.Error("expected ProcessedAt to be set")
	}
}

func TestLLMResponseBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, resp *claude.LLMResponse)
		name     string
		options  []mocks.LLMResponseOption
	}{
		{
			name:    "default response",
			options: []mocks.LLMResponseOption{},
			validate: func(t *testing.T, resp *claude.LLMResponse) {
				t.Helper()
				if resp.Message != "I can help you with that." {
					t.Errorf("unexpected message: %s", resp.Message)
				}
				if resp.Metadata.ModelVersion != "claude-3-opus-20240229" {
					t.Errorf("unexpected model version: %s", resp.Metadata.ModelVersion)
				}
				if resp.Metadata.TokensUsed != 100 {
					t.Errorf("expected 100 tokens used, got %d", resp.Metadata.TokensUsed)
				}
			},
		},
		{
			name: "response with tool calls",
			options: []mocks.LLMResponseOption{
				mocks.WithMessage("Checking your calendar..."),
				mocks.WithToolCalls(
					claude.ToolCall{
						Tool: "calendar_search",
						Parameters: map[string]claude.ToolParameter{
							"date": claude.NewStringParam("today"),
						},
					},
				),
			},
			validate: func(t *testing.T, resp *claude.LLMResponse) {
				t.Helper()
				if resp.Message != "Checking your calendar..." {
					t.Errorf("unexpected message: %s", resp.Message)
				}
				if len(resp.ToolCalls) != 1 {
					t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
				}
				if resp.ToolCalls[0].Tool != "calendar_search" {
					t.Errorf("expected tool name calendar_search, got %s", resp.ToolCalls[0].Tool)
				}
			},
		},
		{
			name: "response with error message",
			options: []mocks.LLMResponseOption{
				mocks.WithMessage("Error: Rate limit exceeded"),
			},
			validate: func(t *testing.T, resp *claude.LLMResponse) {
				t.Helper()
				if resp.Message != "Error: Rate limit exceeded" {
					t.Errorf("unexpected message: %s", resp.Message)
				}
			},
		},
		{
			name: "response with metrics",
			options: []mocks.LLMResponseOption{
				mocks.WithLatency(2500 * time.Millisecond),
				mocks.WithTokensUsed(350),
			},
			validate: func(t *testing.T, resp *claude.LLMResponse) {
				t.Helper()
				if resp.Metadata.Latency != 2500*time.Millisecond {
					t.Errorf("expected latency 2500ms, got %v", resp.Metadata.Latency)
				}
				if resp.Metadata.TokensUsed != 350 {
					t.Errorf("expected 350 tokens used, got %d", resp.Metadata.TokensUsed)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := mocks.NewLLMResponseBuilder().Build(tt.options...)
			tt.validate(t, resp)
		})
	}
}

func TestValidationResultBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, result *agent.ValidationResult)
		name     string
		options  []mocks.ValidationResultOption
	}{
		{
			name:    "default valid result",
			options: []mocks.ValidationResultOption{},
			validate: func(t *testing.T, result *agent.ValidationResult) {
				t.Helper()
				if result.Status != agent.ValidationStatusSuccess {
					t.Errorf("expected status Success, got %s", result.Status)
				}
				if result.Confidence != 0.95 {
					t.Errorf("expected confidence 0.95, got %f", result.Confidence)
				}
			},
		},
		{
			name: "incomplete search result",
			options: []mocks.ValidationResultOption{
				mocks.WithStatus(agent.ValidationStatusIncompleteSearch),
				mocks.WithConfidence(0.6),
				mocks.WithIssues("Did not check memory", "Did not use calendar tool"),
				mocks.WithMetadata(map[string]string{
					"requires_retry":   "true",
					"generated_prompt": "Please check memory and calendar tools",
				}),
			},
			validate: func(t *testing.T, result *agent.ValidationResult) {
				t.Helper()
				if result.Status != agent.ValidationStatusIncompleteSearch {
					t.Errorf("expected status IncompleteSearch, got %s", result.Status)
				}
				if result.Confidence != 0.6 {
					t.Errorf("expected confidence 0.6, got %f", result.Confidence)
				}
				if len(result.Issues) != 2 {
					t.Errorf("expected 2 issues, got %d", len(result.Issues))
				}
				if result.Metadata["requires_retry"] != "true" {
					t.Error("expected requires_retry metadata to be true")
				}
				if result.Metadata["generated_prompt"] != "Please check memory and calendar tools" {
					t.Errorf("unexpected generated prompt: %s", result.Metadata["generated_prompt"])
				}
			},
		},
		{
			name: "failed result with suggestions",
			options: []mocks.ValidationResultOption{
				mocks.WithStatus(agent.ValidationStatusFailed),
				mocks.WithConfidence(0.2),
				mocks.WithIssues("Response contains incorrect information"),
				mocks.WithSuggestions("Verify data sources", "Cross-check with memory"),
			},
			validate: func(t *testing.T, result *agent.ValidationResult) {
				t.Helper()
				if result.Status != agent.ValidationStatusFailed {
					t.Errorf("expected status Failed, got %s", result.Status)
				}
				if len(result.Suggestions) != 2 {
					t.Errorf("expected 2 suggestions, got %d", len(result.Suggestions))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mocks.NewValidationResultBuilder().Build(tt.options...)
			tt.validate(t, result)
		})
	}
}

func TestSignalMessageBuilder(t *testing.T) {
	now := time.Now()

	tests := []struct {
		validate func(t *testing.T, msg *signal.Message)
		name     string
		options  []mocks.SignalMessageOption
	}{
		{
			name:     "default signal message",
			options:  []mocks.SignalMessageOption{},
			validate: validateDefaultSignalMessage,
		},
		{
			name: "custom signal message",
			options: []mocks.SignalMessageOption{
				mocks.WithSignalID("custom-signal-456"),
				mocks.WithSignalSender("+15559876543"),
				mocks.WithRecipient("+15551111111"),
				mocks.WithSignalText("Custom message"),
				mocks.WithSignalTimestamp(now),
				mocks.WithDeliveryStatus(true, false),
			},
			validate: createCustomSignalValidator(now),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := mocks.NewSignalMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

// validateDefaultSignalMessage validates a default signal message.
func validateDefaultSignalMessage(t *testing.T, msg *signal.Message) {
	t.Helper()
	if msg.ID != "signal-msg-123" {
		t.Errorf("expected ID signal-msg-123, got %s", msg.ID)
	}
	if msg.Sender != "+15551234567" {
		t.Errorf("expected sender +15551234567, got %s", msg.Sender)
	}
	if msg.Recipient != mentatRecipient {
		t.Errorf("expected recipient mentat, got %s", msg.Recipient)
	}
	if msg.Text != "Hello from Signal" {
		t.Errorf("unexpected text: %s", msg.Text)
	}
}

// createCustomSignalValidator creates a validator for custom signal messages.
func createCustomSignalValidator(expectedTime time.Time) func(t *testing.T, msg *signal.Message) {
	return func(t *testing.T, msg *signal.Message) {
		t.Helper()
		validateCustomSignalProperties(t, msg)
		validateCustomSignalTimestamp(t, msg, expectedTime)
		validateCustomSignalDeliveryStatus(t, msg)
	}
}

// validateCustomSignalProperties validates custom signal message properties.
func validateCustomSignalProperties(t *testing.T, msg *signal.Message) {
	t.Helper()
	if msg.ID != "custom-signal-456" {
		t.Errorf("expected ID custom-signal-456, got %s", msg.ID)
	}
	if msg.Sender != "+15559876543" {
		t.Errorf("expected sender +15559876543, got %s", msg.Sender)
	}
	if msg.Text != "Custom message" {
		t.Errorf("expected text 'Custom message', got %s", msg.Text)
	}
}

// validateCustomSignalTimestamp validates the timestamp.
func validateCustomSignalTimestamp(t *testing.T, msg *signal.Message, expectedTime time.Time) {
	t.Helper()
	if !msg.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, msg.Timestamp)
	}
}

// validateCustomSignalDeliveryStatus validates delivery status.
func validateCustomSignalDeliveryStatus(t *testing.T, msg *signal.Message) {
	t.Helper()
	if !msg.IsDelivered {
		t.Error("expected IsDelivered to be true")
	}
	if msg.IsRead {
		t.Error("expected IsRead to be false")
	}
}

func TestScenarioBuilder(t *testing.T) {
	t.Run("simple conversation scenario", testSimpleConversationScenario)
	t.Run("failure scenario", testFailureScenario)
	t.Run("retry scenario", testRetryScenario)
	t.Run("complex multi-message scenario", testComplexMultiMessageScenario)
	t.Run("custom scenario builder", testCustomScenarioBuilder)
}

// testSimpleConversationScenario tests a simple conversation scenario.
func testSimpleConversationScenario(t *testing.T) {
	scenario := mocks.CreateSimpleConversation("What's the weather?", "It's sunny and 72°F today.")

	verifyScenarioBasics(t, scenario, "Simple Conversation", 1, 1)

	exp := scenario.Expectations[0]
	if exp.FinalState != queue.StateCompleted {
		t.Errorf("expected final state Completed, got %s", exp.FinalState)
	}
	if exp.ResponseContains != "It's sunny and 72°F today." {
		t.Errorf("unexpected response expectation: %s", exp.ResponseContains)
	}
}

// testFailureScenario tests a failure scenario.
func testFailureScenario(t *testing.T) {
	scenario := mocks.CreateFailureScenario()

	if len(scenario.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(scenario.Messages))
	}

	exp := scenario.Expectations[0]
	if !exp.ShouldFail {
		t.Error("expected ShouldFail to be true")
	}
	if exp.FinalState != queue.StateFailed {
		t.Errorf("expected final state Failed, got %s", exp.FinalState)
	}
}

// testRetryScenario tests a retry scenario.
func testRetryScenario(t *testing.T) {
	scenario := mocks.CreateRetryScenario()

	verifyRetryScenarioStructure(t, scenario)

	exp := scenario.Expectations[0]
	if exp.ResponseContains != "2pm meeting with John" {
		t.Errorf("unexpected response expectation: %s", exp.ResponseContains)
	}
}

// testComplexMultiMessageScenario tests a complex multi-message scenario.
func testComplexMultiMessageScenario(t *testing.T) {
	scenario := mocks.CreateComplexScenario()

	verifyScenarioBasics(t, scenario, "", 2, 2)
	verifyComplexScenarioMessages(t, scenario)
}

// testCustomScenarioBuilder tests a custom scenario builder.
func testCustomScenarioBuilder(t *testing.T) {
	msg1 := mocks.NewMessageBuilder().Build(mocks.WithID("custom-1"))
	msg2 := mocks.NewMessageBuilder().Build(mocks.WithID("custom-2"))
	resp := mocks.NewLLMResponseBuilder().Build()
	validation := mocks.NewValidationResultBuilder().Build()

	scenario := mocks.NewScenarioBuilder("Custom Test").Build(
		mocks.WithMessages(msg1, msg2),
		mocks.WithLLMResponse("custom-1", resp),
		mocks.WithValidation("custom-1", validation),
		mocks.WithExpectation(mocks.Expectation{
			MessageID:  "custom-1",
			FinalState: queue.StateCompleted,
		}),
	)

	if scenario.Name != "Custom Test" {
		t.Errorf("unexpected scenario name: %s", scenario.Name)
	}
	if len(scenario.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(scenario.Messages))
	}
}

// verifyScenarioBasics verifies basic scenario properties.
func verifyScenarioBasics(
	t *testing.T,
	scenario *mocks.Scenario,
	expectedName string,
	expectedMessages, expectedExpectations int,
) {
	t.Helper()
	if expectedName != "" && scenario.Name != expectedName {
		t.Errorf("unexpected scenario name: %s", scenario.Name)
	}
	if len(scenario.Messages) != expectedMessages {
		t.Errorf("expected %d message(s), got %d", expectedMessages, len(scenario.Messages))
	}
	if len(scenario.Expectations) != expectedExpectations {
		t.Errorf(
			"expected %d expectation(s), got %d",
			expectedExpectations,
			len(scenario.Expectations),
		)
	}
}

// verifyRetryScenarioStructure verifies retry scenario structure.
func verifyRetryScenarioStructure(t *testing.T, scenario *mocks.Scenario) {
	t.Helper()
	if len(scenario.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(scenario.Messages))
	}
	// Should have 2 responses (initial + retry)
	if len(scenario.Responses) != 2 {
		t.Errorf("expected 2 responses, got %d", len(scenario.Responses))
	}
	// Should have 2 validations
	if len(scenario.Validations) != 2 {
		t.Errorf("expected 2 validations, got %d", len(scenario.Validations))
	}
}

// verifyComplexScenarioMessages verifies complex scenario messages.
func verifyComplexScenarioMessages(t *testing.T, scenario *mocks.Scenario) {
	t.Helper()
	// Verify messages are in same conversation
	if scenario.Messages[0].ConversationID != scenario.Messages[1].ConversationID {
		t.Error("messages should be in same conversation")
	}

	// Verify both responses exist
	resp1 := scenario.Responses["complex-1"]
	resp2 := scenario.Responses["complex-2"]
	if resp1 == nil || resp2 == nil {
		t.Error("responses should exist for both messages")
	}
}

func TestConversationMessageBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, msg *conversation.Message)
		name     string
		options  []mocks.ConversationMessageOption
	}{
		{
			name:    "default conversation message",
			options: []mocks.ConversationMessageOption{},
			validate: func(t *testing.T, msg *conversation.Message) {
				t.Helper()
				if msg.ID != "conv-msg-123" {
					t.Errorf("expected ID conv-msg-123, got %s", msg.ID)
				}
				if msg.From != "+15551234567" {
					t.Errorf("expected from +15551234567, got %s", msg.From)
				}
				if msg.Text != "Test conversation message" {
					t.Errorf("unexpected text: %s", msg.Text)
				}
				if msg.SessionID != "session-123" {
					t.Errorf("expected session ID session-123, got %s", msg.SessionID)
				}
			},
		},
		{
			name: "message with response",
			options: []mocks.ConversationMessageOption{
				mocks.WithConversationMessageID("conv-msg-456"),
				mocks.WithConversationFrom(mentatRecipient),
				mocks.WithConversationText("What's the weather?"),
				mocks.WithConversationResponse("It's sunny and 72°F."),
				mocks.WithConversationSessionID("session-456"),
			},
			validate: func(t *testing.T, msg *conversation.Message) {
				t.Helper()
				if msg.ID != "conv-msg-456" {
					t.Errorf("expected ID conv-msg-456, got %s", msg.ID)
				}
				if msg.From != mentatRecipient {
					t.Errorf("expected from mentat, got %s", msg.From)
				}
				if msg.Text != "What's the weather?" {
					t.Errorf("unexpected text: %s", msg.Text)
				}
				if msg.Response != "It's sunny and 72°F." {
					t.Errorf("unexpected response: %s", msg.Response)
				}
				if msg.SessionID != "session-456" {
					t.Errorf("expected session ID session-456, got %s", msg.SessionID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := mocks.NewConversationMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

func TestStoredMessageBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, msg *storage.StoredMessage)
		name     string
		options  []mocks.StoredMessageOption
	}{
		{
			name:     "default stored message",
			options:  []mocks.StoredMessageOption{},
			validate: validateDefaultStoredMessage,
		},
		{
			name: "custom stored message",
			options: []mocks.StoredMessageOption{
				mocks.WithStoredMessageID("custom-stored-42"),
				mocks.WithStoredContent("Custom stored content"),
				mocks.WithStoredDirection(storage.OUTBOUND),
				mocks.WithStoredProcessingState("failed"),
				mocks.WithStoredFrom(mentatRecipient),
				mocks.WithStoredTo("+15559876543"),
			},
			validate: validateCustomStoredMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := mocks.NewStoredMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

// validateDefaultStoredMessage validates a default stored message.
func validateDefaultStoredMessage(t *testing.T, msg *storage.StoredMessage) {
	t.Helper()
	if msg.ID != "stored-msg-123" {
		t.Errorf("expected ID stored-msg-123, got %s", msg.ID)
	}
	if msg.ConversationID != "+15551234567" {
		t.Errorf("expected conversation ID +15551234567, got %s", msg.ConversationID)
	}
	if msg.Direction != storage.INBOUND {
		t.Errorf("expected direction INBOUND, got %s", msg.Direction)
	}
	if msg.ProcessingState != "completed" {
		t.Errorf("expected processing state completed, got %s", msg.ProcessingState)
	}
}

// validateCustomStoredMessage validates a custom stored message.
func validateCustomStoredMessage(t *testing.T, msg *storage.StoredMessage) {
	t.Helper()
	if msg.ID != "custom-stored-42" {
		t.Errorf("expected ID custom-stored-42, got %s", msg.ID)
	}
	if msg.Content != "Custom stored content" {
		t.Errorf("unexpected content: %s", msg.Content)
	}
	if msg.Direction != storage.OUTBOUND {
		t.Errorf("expected direction OUTBOUND, got %s", msg.Direction)
	}
	if msg.ProcessingState != "failed" {
		t.Errorf("expected processing state failed, got %s", msg.ProcessingState)
	}
	if msg.From != mentatRecipient {
		t.Errorf("expected from mentat, got %s", msg.From)
	}
	if msg.To != "+15559876543" {
		t.Errorf("expected to +15559876543, got %s", msg.To)
	}
}

func TestBuildersConcurrency(t *testing.T) {
	// Test that builders are safe for concurrent use
	t.Run("concurrent message building", func(t *testing.T) {
		done := make(chan bool, 10)

		for i := range 10 {
			go func(id int) {
				msg := mocks.NewMessageBuilder().Build(
					mocks.WithID(fmt.Sprintf("msg-%d", id)),
					mocks.WithText("Concurrent message"),
				)
				if msg.Text != "Concurrent message" {
					t.Errorf("unexpected text in goroutine %d", id)
				}
				done <- true
			}(i)
		}

		for range 10 {
			<-done
		}
	})
}

func TestBuildersProduceUniqueInstances(t *testing.T) {
	// Ensure builders create new instances each time
	t.Run("message builder creates unique instances", func(t *testing.T) {
		builder := mocks.NewMessageBuilder()
		msg1 := builder.Build()
		msg2 := builder.Build()

		if msg1 == msg2 {
			t.Error("builder should create new instances")
		}

		// Modify one shouldn't affect the other
		msg1.Text = "Modified"
		if msg2.Text == "Modified" {
			t.Error("modifying one instance affected another")
		}
	})
}

func TestToolParameterBuilding(t *testing.T) {
	// Test building complex tool parameters
	t.Run("object parameter", func(t *testing.T) {
		params := map[string]claude.ToolParameter{
			"date":     claude.NewStringParam("2024-01-15"),
			"duration": claude.NewIntParam(60),
			"attendees": claude.NewArrayParam([]claude.ToolParameter{
				claude.NewStringParam("alice"),
				claude.NewStringParam("bob"),
			}),
		}

		resp := mocks.NewLLMResponseBuilder().Build(
			mocks.WithToolCalls(claude.ToolCall{
				Tool:       "schedule_meeting",
				Parameters: params,
			}),
		)

		if len(resp.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
		}

		call := resp.ToolCalls[0]
		if dateParam, ok := call.Parameters["date"]; !ok {
			t.Error("expected date parameter")
		} else if dateParam.StringValue != "2024-01-15" {
			t.Errorf("unexpected date: %v", dateParam.StringValue)
		}
	})

	t.Run("array parameter", func(t *testing.T) {
		arrayParam := claude.NewArrayParam([]claude.ToolParameter{
			claude.NewStringParam("item1"),
			claude.NewStringParam("item2"),
			claude.NewStringParam("item3"),
		})

		resp := mocks.NewLLMResponseBuilder().Build(
			mocks.WithToolCalls(claude.ToolCall{
				Tool: "process_items",
				Parameters: map[string]claude.ToolParameter{
					"items": arrayParam,
				},
			}),
		)

		call := resp.ToolCalls[0]
		if itemsParam, ok := call.Parameters["items"]; !ok {
			t.Fatal("expected items parameter")
		} else if len(itemsParam.ArrayValue) != 3 {
			t.Errorf("expected 3 items, got %d", len(itemsParam.ArrayValue))
		}
	})
}

func TestScenarioHelpers(t *testing.T) {
	// Test that scenario helpers produce valid test data
	t.Run("all scenario helpers produce valid data", func(t *testing.T) {
		scenarios := []*mocks.Scenario{
			mocks.CreateSimpleConversation("test", "response"),
			mocks.CreateFailureScenario(),
			mocks.CreateRetryScenario(),
			mocks.CreateComplexScenario(),
		}

		for i, scenario := range scenarios {
			validateScenario(t, i, scenario)
		}
	})
}

// validateScenario validates a single scenario.
func validateScenario(t *testing.T, index int, scenario *mocks.Scenario) {
	t.Helper()

	validateScenarioBasicProperties(t, index, scenario)
	validateScenarioExpectations(t, index, scenario)
}

// validateScenarioBasicProperties validates basic properties of a scenario.
func validateScenarioBasicProperties(t *testing.T, index int, scenario *mocks.Scenario) {
	t.Helper()

	if scenario.Name == "" {
		t.Errorf("scenario %d has empty name", index)
	}
	if len(scenario.Messages) == 0 {
		t.Errorf("scenario %d has no messages", index)
	}
	if len(scenario.Expectations) == 0 {
		t.Errorf("scenario %d has no expectations", index)
	}
}

// validateScenarioExpectations validates that all expectations reference existing messages.
func validateScenarioExpectations(t *testing.T, index int, scenario *mocks.Scenario) {
	t.Helper()

	for _, exp := range scenario.Expectations {
		if !messageExistsInScenario(scenario, exp.MessageID) {
			t.Errorf(
				"scenario %d: expectation references non-existent message %s",
				index,
				exp.MessageID,
			)
		}
	}
}

// messageExistsInScenario checks if a message ID exists in the scenario.
func messageExistsInScenario(scenario *mocks.Scenario, messageID string) bool {
	for _, msg := range scenario.Messages {
		if msg.ID == messageID {
			return true
		}
	}
	return false
}

func TestBuilderChaining(t *testing.T) {
	// Test that options can be chained fluently
	t.Run("fluent option chaining", func(t *testing.T) {
		msg := mocks.NewMessageBuilder().Build(
			mocks.WithID("chain-1"),
			mocks.WithText("Chained content"),
			mocks.WithState(queue.StateProcessing),
			mocks.WithSender("+15551234567"),
			mocks.WithAttempts(1),
			mocks.WithMaxAttempts(5),
			mocks.WithError(fmt.Errorf("Test error")),
		)

		// Verify all options were applied
		if msg.ID != "chain-1" {
			t.Error("ID option not applied")
		}
		if msg.Text != "Chained content" {
			t.Error("Text option not applied")
		}
		if msg.State != queue.StateProcessing {
			t.Error("State option not applied")
		}
		if msg.Sender != "+15551234567" {
			t.Error("Sender option not applied")
		}
		if msg.Attempts != 1 {
			t.Error("Attempts option not applied")
		}
		if msg.MaxAttempts != 5 {
			t.Error("MaxAttempts option not applied")
		}
		if msg.Error == nil || msg.Error.Error() != "Test error" {
			t.Error("Error option not applied")
		}
	})
}

func TestZeroValues(t *testing.T) {
	// Ensure builders handle zero values correctly
	t.Run("zero values are handled properly", func(t *testing.T) {
		msg := mocks.NewMessageBuilder().Build(
			mocks.WithText(""),
			mocks.WithAttempts(0),
			mocks.WithError(nil),
		)

		if msg.Text != "" {
			t.Error("empty text should be allowed")
		}
		if msg.Attempts != 0 {
			t.Error("zero attempts should be allowed")
		}
		if msg.Error != nil {
			t.Error("nil error should be allowed")
		}
	})
}

// Benchmark to ensure builders are performant.
func BenchmarkMessageBuilder(b *testing.B) {
	for range b.N {
		_ = mocks.NewMessageBuilder().Build(
			mocks.WithID("bench-id"),
			mocks.WithText("Benchmark content"),
			mocks.WithState(queue.StateProcessing),
			mocks.WithSender("+15551234567"),
		)
	}
}

func BenchmarkScenarioCreation(b *testing.B) {
	for range b.N {
		_ = mocks.CreateComplexScenario()
	}
}
