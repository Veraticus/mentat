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
	tests := []struct {
		validate func(t *testing.T, tp ToolParameter)
		create   func() ToolParameter
		name     string
	}{
		{
			name: "NewStringParam",
			create: func() ToolParameter {
				return NewStringParam("test")
			},
			validate: func(t *testing.T, tp ToolParameter) {
				t.Helper()
				if tp.Type != ToolParamString {
					t.Error("Expected ToolParamString type")
				}
				if tp.StringValue != "test" {
					t.Error("Expected StringValue to be 'test'")
				}
			},
		},
		{
			name: "NewIntParam",
			create: func() ToolParameter {
				return NewIntParam(42)
			},
			validate: func(t *testing.T, tp ToolParameter) {
				t.Helper()
				if tp.Type != ToolParamInt {
					t.Error("Expected ToolParamInt type")
				}
				if tp.IntValue != 42 {
					t.Error("Expected IntValue to be 42")
				}
			},
		},
		{
			name: "NewBoolParam",
			create: func() ToolParameter {
				return NewBoolParam(true)
			},
			validate: func(t *testing.T, tp ToolParameter) {
				t.Helper()
				if tp.Type != ToolParamBool {
					t.Error("Expected ToolParamBool type")
				}
				if !tp.BoolValue {
					t.Error("Expected BoolValue to be true")
				}
			},
		},
		{
			name: "NewFloatParam",
			create: func() ToolParameter {
				return NewFloatParam(3.14)
			},
			validate: func(t *testing.T, tp ToolParameter) {
				t.Helper()
				if tp.Type != ToolParamFloat {
					t.Error("Expected ToolParamFloat type")
				}
				if tp.FloatValue != 3.14 {
					t.Error("Expected FloatValue to be 3.14")
				}
			},
		},
		{
			name: "NewArrayParam",
			create: func() ToolParameter {
				return NewArrayParam([]ToolParameter{
					NewStringParam("item1"),
					NewIntParam(2),
				})
			},
			validate: func(t *testing.T, tp ToolParameter) {
				t.Helper()
				if tp.Type != ToolParamArray {
					t.Error("Expected ToolParamArray type")
				}
				if len(tp.ArrayValue) != 2 {
					t.Error("Expected ArrayValue to have 2 items")
				}
			},
		},
		{
			name: "NewObjectParam",
			create: func() ToolParameter {
				return NewObjectParam(map[string]ToolParameter{
					"key1": NewStringParam("value1"),
					"key2": NewIntParam(2),
				})
			},
			validate: func(t *testing.T, tp ToolParameter) {
				t.Helper()
				if tp.Type != ToolParamObject {
					t.Error("Expected ToolParamObject type")
				}
				if len(tp.ObjectValue) != 2 {
					t.Error("Expected ObjectValue to have 2 items")
				}
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp := tt.create()
			tt.validate(t, tp)
		})
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