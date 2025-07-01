package claude

import (
	"testing"
	"time"
)

func TestConfigZeroValue(t *testing.T) {
	var cfg Config

	// Zero value should be identifiable as uninitialized
	if cfg.MCPConfigPath != "" {
		t.Error("Expected empty MCPConfigPath for zero value")
	}
	if cfg.SystemPrompt != "" {
		t.Error("Expected empty SystemPrompt for zero value")
	}
	if cfg.Command != "" {
		t.Error("Expected empty Command for zero value")
	}
	if cfg.Timeout != 0 {
		t.Error("Expected zero Timeout for zero value")
	}
	if cfg.MaxTokens != 0 {
		t.Error("Expected zero MaxTokens for zero value")
	}
}

func TestLLMResponseZeroValue(t *testing.T) {
	var resp LLMResponse

	// Zero value should be identifiable as uninitialized
	if resp.ToolCalls != nil {
		t.Error("Expected nil ToolCalls for zero value")
	}
	if resp.Message != "" {
		t.Error("Expected empty Message for zero value")
	}
	if resp.Metadata.ModelVersion != "" {
		t.Error("Expected empty ModelVersion for zero value")
	}
	if resp.Metadata.Latency != 0 {
		t.Error("Expected zero Latency for zero value")
	}
	if resp.Metadata.TokensUsed != 0 {
		t.Error("Expected zero TokensUsed for zero value")
	}
}

func TestToolCallZeroValue(t *testing.T) {
	var tc ToolCall

	// Zero value should be identifiable as uninitialized
	if tc.Tool != "" {
		t.Error("Expected empty Tool for zero value")
	}
	if tc.Parameters != nil {
		t.Error("Expected nil Parameters for zero value")
	}
	if tc.Result != "" {
		t.Error("Expected empty Result for zero value")
	}
}

func TestToolParameterZeroValue(t *testing.T) {
	var tp ToolParameter

	// Zero value should default to ToolParamString type with empty string
	if tp.Type != ToolParamString {
		t.Error("Expected ToolParamString as default Type")
	}
	if tp.StringValue != "" {
		t.Error("Expected empty StringValue for zero value")
	}
	if tp.IntValue != 0 {
		t.Error("Expected zero IntValue for zero value")
	}
	if tp.BoolValue != false {
		t.Error("Expected false BoolValue for zero value")
	}
	if tp.FloatValue != 0.0 {
		t.Error("Expected zero FloatValue for zero value")
	}
	if tp.ArrayValue != nil {
		t.Error("Expected nil ArrayValue for zero value")
	}
	if tp.ObjectValue != nil {
		t.Error("Expected nil ObjectValue for zero value")
	}
}

func TestToolParameterConstructors(t *testing.T) {
	t.Run("NewStringParam", func(t *testing.T) {
		tp := NewStringParam("test")
		validateStringParam(t, tp, "test")
	})

	t.Run("NewIntParam", func(t *testing.T) {
		tp := NewIntParam(42)
		validateIntParam(t, tp, 42)
	})

	t.Run("NewBoolParam", func(t *testing.T) {
		tp := NewBoolParam(true)
		validateBoolParam(t, tp, true)
	})

	t.Run("NewFloatParam", func(t *testing.T) {
		tp := NewFloatParam(3.14)
		validateFloatParam(t, tp, 3.14)
	})

	t.Run("NewArrayParam", func(t *testing.T) {
		tp := NewArrayParam([]ToolParameter{
			NewStringParam("item1"),
			NewIntParam(2),
		})
		validateArrayParam(t, tp, 2)
	})

	t.Run("NewObjectParam", func(t *testing.T) {
		tp := NewObjectParam(map[string]ToolParameter{
			"key1": NewStringParam("value1"),
			"key2": NewIntParam(2),
		})
		validateObjectParam(t, tp, 2)
	})
}

func validateStringParam(t *testing.T, tp ToolParameter, expected string) {
	t.Helper()
	if tp.Type != ToolParamString {
		t.Error("Expected ToolParamString type")
	}
	if tp.StringValue != expected {
		t.Errorf("Expected StringValue to be '%s'", expected)
	}
}

func validateIntParam(t *testing.T, tp ToolParameter, expected int) {
	t.Helper()
	if tp.Type != ToolParamInt {
		t.Error("Expected ToolParamInt type")
	}
	if tp.IntValue != expected {
		t.Errorf("Expected IntValue to be %d", expected)
	}
}

func validateBoolParam(t *testing.T, tp ToolParameter, expected bool) {
	t.Helper()
	if tp.Type != ToolParamBool {
		t.Error("Expected ToolParamBool type")
	}
	if tp.BoolValue != expected {
		t.Errorf("Expected BoolValue to be %v", expected)
	}
}

func validateFloatParam(t *testing.T, tp ToolParameter, expected float64) {
	t.Helper()
	if tp.Type != ToolParamFloat {
		t.Error("Expected ToolParamFloat type")
	}
	if tp.FloatValue != expected {
		t.Errorf("Expected FloatValue to be %f", expected)
	}
}

func validateArrayParam(t *testing.T, tp ToolParameter, expectedLen int) {
	t.Helper()
	if tp.Type != ToolParamArray {
		t.Error("Expected ToolParamArray type")
	}
	if len(tp.ArrayValue) != expectedLen {
		t.Errorf("Expected ArrayValue to have %d items", expectedLen)
	}
}

func validateObjectParam(t *testing.T, tp ToolParameter, expectedLen int) {
	t.Helper()
	if tp.Type != ToolParamObject {
		t.Error("Expected ToolParamObject type")
	}
	if len(tp.ObjectValue) != expectedLen {
		t.Errorf("Expected ObjectValue to have %d items", expectedLen)
	}
}

func TestResponseMetadataZeroValue(t *testing.T) {
	var meta ResponseMetadata

	// Zero value should be identifiable as uninitialized
	if meta.ModelVersion != "" {
		t.Error("Expected empty ModelVersion for zero value")
	}
	if meta.Latency != 0 {
		t.Error("Expected zero Latency for zero value")
	}
	if meta.TokensUsed != 0 {
		t.Error("Expected zero TokensUsed for zero value")
	}
}

func TestLLMResponseCreation(t *testing.T) {
	resp := LLMResponse{
		Message: "Test response",
		ToolCalls: []ToolCall{
			{
				Tool: "test_tool",
				Parameters: map[string]ToolParameter{
					"param1": NewStringParam("value1"),
				},
				Result: "Success",
			},
		},
		Metadata: ResponseMetadata{
			ModelVersion: "claude-3.5",
			Latency:      100 * time.Millisecond,
			TokensUsed:   42,
		},
	}

	if resp.Message != "Test response" {
		t.Error("Message not set correctly")
	}
	if len(resp.ToolCalls) != 1 {
		t.Error("ToolCalls not set correctly")
	}
	if resp.ToolCalls[0].Tool != "test_tool" {
		t.Error("Tool name not set correctly")
	}
	if resp.Metadata.ModelVersion != "claude-3.5" {
		t.Error("ModelVersion not set correctly")
	}
	if resp.Metadata.Latency != 100*time.Millisecond {
		t.Error("Latency not set correctly")
	}
	if resp.Metadata.TokensUsed != 42 {
		t.Error("TokensUsed not set correctly")
	}
}
