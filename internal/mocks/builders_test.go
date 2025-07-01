package mocks

import (
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/agent"
	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/queue"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/Veraticus/mentat/internal/storage"
)

func TestMessageBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, msg *queue.Message)
		name     string
		options  []MessageOption
	}{
		{
			name:    "default message",
			options: []MessageOption{},
			validate: func(t *testing.T, msg *queue.Message) {
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
			},
		},
		{
			name: "custom message",
			options: []MessageOption{
				WithID("custom-123"),
				WithText("Custom content"),
				WithState(queue.StateProcessing),
				WithSender("+15559999999"),
				WithAttempts(2),
			},
			validate: func(t *testing.T, msg *queue.Message) {
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
			},
		},
		{
			name: "message with error",
			options: []MessageOption{
				WithState(queue.StateFailed),
				WithError(fmt.Errorf("Connection timeout")),
			},
			validate: func(t *testing.T, msg *queue.Message) {
				t.Helper()
				if msg.State != queue.StateFailed {
					t.Errorf("expected state Failed, got %s", msg.State)
				}
				if msg.Error == nil || msg.Error.Error() != "Connection timeout" {
					t.Errorf("expected error 'Connection timeout', got %v", msg.Error)
				}
			},
		},
		{
			name: "message with response",
			options: []MessageOption{
				WithState(queue.StateCompleted),
				WithResponse("Task completed successfully"),
			},
			validate: func(t *testing.T, msg *queue.Message) {
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
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

func TestQueuedMessageBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, msg *queue.QueuedMessage)
		name     string
		options  []QueuedMessageOption
	}{
		{
			name:    "default queued message",
			options: []QueuedMessageOption{},
			validate: func(t *testing.T, msg *queue.QueuedMessage) {
				t.Helper()
				if msg.ID != "queued-msg-123" {
					t.Errorf("expected ID queued-msg-123, got %s", msg.ID)
				}
				if msg.State != queue.MessageStateQueued {
					t.Errorf("expected state Queued, got %v", msg.State)
				}
				if msg.Priority != queue.PriorityNormal {
					t.Errorf("expected priority Normal, got %v", msg.Priority)
				}
			},
		},
		{
			name: "custom queued message",
			options: []QueuedMessageOption{
				WithQueuedMessageID("custom-queued-123"),
				WithQueuedText("Custom queued text"),
				WithQueuedState(queue.MessageStateProcessing),
				WithQueuedPriority(queue.PriorityHigh),
				WithQueuedAttempts(2),
			},
			validate: func(t *testing.T, msg *queue.QueuedMessage) {
				t.Helper()
				if msg.ID != "custom-queued-123" {
					t.Errorf("expected ID custom-queued-123, got %s", msg.ID)
				}
				if msg.Text != "Custom queued text" {
					t.Errorf("expected text 'Custom queued text', got %s", msg.Text)
				}
				if msg.State != queue.MessageStateProcessing {
					t.Errorf("expected state Processing, got %v", msg.State)
				}
				if msg.Priority != queue.PriorityHigh {
					t.Errorf("expected priority High, got %v", msg.Priority)
				}
				if msg.Attempts != 2 {
					t.Errorf("expected 2 attempts, got %d", msg.Attempts)
				}
			},
		},
		{
			name: "queued message with error",
			options: []QueuedMessageOption{
				WithQueuedState(queue.MessageStateFailed),
				WithQueuedError(fmt.Errorf("queue processing failed")),
			},
			validate: func(t *testing.T, msg *queue.QueuedMessage) {
				t.Helper()
				if msg.State != queue.MessageStateFailed {
					t.Errorf("expected state Failed, got %v", msg.State)
				}
				if msg.LastError == nil || msg.LastError.Error() != "queue processing failed" {
					t.Errorf("expected error 'queue processing failed', got %v", msg.LastError)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewQueuedMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

func TestLLMResponseBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, resp *claude.LLMResponse)
		name     string
		options  []LLMResponseOption
	}{
		{
			name:    "default response",
			options: []LLMResponseOption{},
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
			options: []LLMResponseOption{
				WithMessage("Checking your calendar..."),
				WithToolCalls(
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
			options: []LLMResponseOption{
				WithMessage("Error: Rate limit exceeded"),
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
			options: []LLMResponseOption{
				WithLatency(2500 * time.Millisecond),
				WithTokensUsed(350),
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
			resp := NewLLMResponseBuilder().Build(tt.options...)
			tt.validate(t, resp)
		})
	}
}

func TestValidationResultBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, result *agent.ValidationResult)
		name     string
		options  []ValidationResultOption
	}{
		{
			name:    "default valid result",
			options: []ValidationResultOption{},
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
			options: []ValidationResultOption{
				WithStatus(agent.ValidationStatusIncompleteSearch),
				WithConfidence(0.6),
				WithIssues("Did not check memory", "Did not use calendar tool"),
				WithMetadata(map[string]string{
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
			options: []ValidationResultOption{
				WithStatus(agent.ValidationStatusFailed),
				WithConfidence(0.2),
				WithIssues("Response contains incorrect information"),
				WithSuggestions("Verify data sources", "Cross-check with memory"),
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
			result := NewValidationResultBuilder().Build(tt.options...)
			tt.validate(t, result)
		})
	}
}

func TestSignalMessageBuilder(t *testing.T) {
	now := time.Now()
	
	tests := []struct {
		validate func(t *testing.T, msg *signal.Message)
		name     string
		options  []SignalMessageOption
	}{
		{
			name:    "default signal message",
			options: []SignalMessageOption{},
			validate: func(t *testing.T, msg *signal.Message) {
				t.Helper()
				if msg.ID != "signal-msg-123" {
					t.Errorf("expected ID signal-msg-123, got %s", msg.ID)
				}
				if msg.Sender != "+15551234567" {
					t.Errorf("expected sender +15551234567, got %s", msg.Sender)
				}
				if msg.Recipient != "mentat" {
					t.Errorf("expected recipient mentat, got %s", msg.Recipient)
				}
				if msg.Text != "Hello from Signal" {
					t.Errorf("unexpected text: %s", msg.Text)
				}
			},
		},
		{
			name: "custom signal message",
			options: []SignalMessageOption{
				WithSignalID("custom-signal-456"),
				WithSignalSender("+15559876543"),
				WithRecipient("+15551111111"),
				WithSignalText("Custom message"),
				WithSignalTimestamp(now),
				WithDeliveryStatus(true, false),
			},
			validate: func(t *testing.T, msg *signal.Message) {
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
				if !msg.Timestamp.Equal(now) {
					t.Errorf("expected timestamp %v, got %v", now, msg.Timestamp)
				}
				if !msg.IsDelivered {
					t.Error("expected IsDelivered to be true")
				}
				if msg.IsRead {
					t.Error("expected IsRead to be false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewSignalMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

func TestScenarioBuilder(t *testing.T) {
	t.Run("simple conversation scenario", func(t *testing.T) {
		scenario := CreateSimpleConversation("What's the weather?", "It's sunny and 72°F today.")
		
		if scenario.Name != "Simple Conversation" {
			t.Errorf("unexpected scenario name: %s", scenario.Name)
		}
		if len(scenario.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(scenario.Messages))
		}
		if len(scenario.Expectations) != 1 {
			t.Errorf("expected 1 expectation, got %d", len(scenario.Expectations))
		}
		
		exp := scenario.Expectations[0]
		if exp.FinalState != queue.StateCompleted {
			t.Errorf("expected final state Completed, got %s", exp.FinalState)
		}
		if exp.ResponseContains != "It's sunny and 72°F today." {
			t.Errorf("unexpected response expectation: %s", exp.ResponseContains)
		}
	})

	t.Run("failure scenario", func(t *testing.T) {
		scenario := CreateFailureScenario()
		
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
	})

	t.Run("retry scenario", func(t *testing.T) {
		scenario := CreateRetryScenario()
		
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
		
		exp := scenario.Expectations[0]
		if exp.ResponseContains != "2pm meeting with John" {
			t.Errorf("unexpected response expectation: %s", exp.ResponseContains)
		}
	})

	t.Run("complex multi-message scenario", func(t *testing.T) {
		scenario := CreateComplexScenario()
		
		if len(scenario.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(scenario.Messages))
		}
		if len(scenario.Expectations) != 2 {
			t.Errorf("expected 2 expectations, got %d", len(scenario.Expectations))
		}
		
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
	})

	t.Run("custom scenario builder", func(t *testing.T) {
		msg1 := NewMessageBuilder().Build(WithID("custom-1"))
		msg2 := NewMessageBuilder().Build(WithID("custom-2"))
		resp := NewLLMResponseBuilder().Build()
		validation := NewValidationResultBuilder().Build()
		
		scenario := NewScenarioBuilder("Custom Test").Build(
			WithMessages(msg1, msg2),
			WithLLMResponse("custom-1", resp),
			WithValidation("custom-1", validation),
			WithExpectation(Expectation{
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
	})
}

func TestConversationMessageBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, msg *conversation.Message)
		name     string
		options  []ConversationMessageOption
	}{
		{
			name:    "default conversation message",
			options: []ConversationMessageOption{},
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
			options: []ConversationMessageOption{
				WithConversationMessageID("conv-msg-456"),
				WithConversationFrom("mentat"),
				WithConversationText("What's the weather?"),
				WithConversationResponse("It's sunny and 72°F."),
				WithConversationSessionID("session-456"),
			},
			validate: func(t *testing.T, msg *conversation.Message) {
				t.Helper()
				if msg.ID != "conv-msg-456" {
					t.Errorf("expected ID conv-msg-456, got %s", msg.ID)
				}
				if msg.From != "mentat" {
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
			msg := NewConversationMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

func TestStoredMessageBuilder(t *testing.T) {
	tests := []struct {
		validate func(t *testing.T, msg *storage.StoredMessage)
		name     string
		options  []StoredMessageOption
	}{
		{
			name:    "default stored message",
			options: []StoredMessageOption{},
			validate: func(t *testing.T, msg *storage.StoredMessage) {
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
			},
		},
		{
			name: "custom stored message",
			options: []StoredMessageOption{
				WithStoredMessageID("custom-stored-42"),
				WithStoredContent("Custom stored content"),
				WithStoredDirection(storage.OUTBOUND),
				WithStoredProcessingState("failed"),
				WithStoredFrom("mentat"),
				WithStoredTo("+15559876543"),
			},
			validate: func(t *testing.T, msg *storage.StoredMessage) {
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
				if msg.From != "mentat" {
					t.Errorf("expected from mentat, got %s", msg.From)
				}
				if msg.To != "+15559876543" {
					t.Errorf("expected to +15559876543, got %s", msg.To)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewStoredMessageBuilder().Build(tt.options...)
			tt.validate(t, msg)
		})
	}
}

func TestBuildersConcurrency(t *testing.T) {
	// Test that builders are safe for concurrent use
	t.Run("concurrent message building", func(t *testing.T) {
		done := make(chan bool, 10)
		
		for i := 0; i < 10; i++ {
			go func(id int) {
				msg := NewMessageBuilder().Build(
					WithID(fmt.Sprintf("msg-%d", id)),
					WithText("Concurrent message"),
				)
				if msg.Text != "Concurrent message" {
					t.Errorf("unexpected text in goroutine %d", id)
				}
				done <- true
			}(i)
		}
		
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

func TestBuildersProduceUniqueInstances(t *testing.T) {
	// Ensure builders create new instances each time
	t.Run("message builder creates unique instances", func(t *testing.T) {
		builder := NewMessageBuilder()
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
		
		resp := NewLLMResponseBuilder().Build(
			WithToolCalls(claude.ToolCall{
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
		
		resp := NewLLMResponseBuilder().Build(
			WithToolCalls(claude.ToolCall{
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
		scenarios := []*Scenario{
			CreateSimpleConversation("test", "response"),
			CreateFailureScenario(),
			CreateRetryScenario(),
			CreateComplexScenario(),
		}
		
		for i, scenario := range scenarios {
			if scenario.Name == "" {
				t.Errorf("scenario %d has empty name", i)
			}
			if len(scenario.Messages) == 0 {
				t.Errorf("scenario %d has no messages", i)
			}
			if len(scenario.Expectations) == 0 {
				t.Errorf("scenario %d has no expectations", i)
			}
			
			// Verify all message IDs in expectations exist
			for _, exp := range scenario.Expectations {
				found := false
				for _, msg := range scenario.Messages {
					if msg.ID == exp.MessageID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("scenario %d: expectation references non-existent message %s", i, exp.MessageID)
				}
			}
		}
	})
}

func TestBuilderChaining(t *testing.T) {
	// Test that options can be chained fluently
	t.Run("fluent option chaining", func(t *testing.T) {
		msg := NewMessageBuilder().Build(
			WithID("chain-1"),
			WithText("Chained content"),
			WithState(queue.StateProcessing),
			WithSender("+15551234567"),
			WithAttempts(1),
			WithMaxAttempts(5),
			WithError(fmt.Errorf("Test error")),
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
		msg := NewMessageBuilder().Build(
			WithText(""),
			WithAttempts(0),
			WithError(nil),
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
	for i := 0; i < b.N; i++ {
		_ = NewMessageBuilder().Build(
			WithID("bench-id"),
			WithText("Benchmark content"),
			WithState(queue.StateProcessing),
			WithSender("+15551234567"),
		)
	}
}

func BenchmarkScenarioCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = CreateComplexScenario()
	}
}