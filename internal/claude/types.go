package claude

import "time"

// Config holds configuration for Claude CLI execution.
type Config struct {
	MCPConfigPath string
	SystemPrompt  string
	Command       string
	Timeout       time.Duration
	MaxTokens     int
}

// LLMResponse contains Claude's response and metadata.
type LLMResponse struct {
	ToolCalls []ToolCall       // Tool invocations made by Claude
	Message   string           // Text response from Claude
	Metadata  ResponseMetadata // Response metadata
}

// ToolCall represents a tool invocation by Claude.
type ToolCall struct {
	Tool       string                   // Tool name (e.g., "mcp_memory_create")
	Parameters map[string]ToolParameter // Parameters passed to the tool
	Result     string                   // Result returned by the tool
}

// ToolParameter represents a parameter value for tool calls.
// Using a concrete type instead of any/interface{} to maintain type safety.
type ToolParameter struct {
	ObjectValue map[string]ToolParameter
	StringValue string
	ArrayValue  []ToolParameter
	FloatValue  float64
	IntValue    int
	Type        ToolParameterType
	BoolValue   bool
}

// ToolParameterType indicates the type of a tool parameter.
type ToolParameterType int

const (
	// ToolParamString indicates a string parameter.
	ToolParamString ToolParameterType = iota
	// ToolParamInt indicates an integer parameter.
	ToolParamInt
	// ToolParamBool indicates a boolean parameter.
	ToolParamBool
	// ToolParamFloat indicates a float parameter.
	ToolParamFloat
	// ToolParamArray indicates an array parameter.
	ToolParamArray
	// ToolParamObject indicates an object parameter.
	ToolParamObject
)

// ResponseMetadata contains metadata about the LLM response.
type ResponseMetadata struct {
	ModelVersion string        // Claude model version used
	Latency      time.Duration // Time taken to generate response
	TokensUsed   int           // Number of tokens consumed
}

// NewStringParam creates a string tool parameter.
func NewStringParam(value string) ToolParameter {
	return ToolParameter{
		StringValue: value,
		Type:        ToolParamString,
	}
}

// NewIntParam creates an integer tool parameter.
func NewIntParam(value int) ToolParameter {
	return ToolParameter{
		IntValue: value,
		Type:     ToolParamInt,
	}
}

// NewBoolParam creates a boolean tool parameter.
func NewBoolParam(value bool) ToolParameter {
	return ToolParameter{
		BoolValue: value,
		Type:      ToolParamBool,
	}
}

// NewFloatParam creates a float tool parameter.
func NewFloatParam(value float64) ToolParameter {
	return ToolParameter{
		FloatValue: value,
		Type:       ToolParamFloat,
	}
}

// NewArrayParam creates an array tool parameter.
func NewArrayParam(values []ToolParameter) ToolParameter {
	return ToolParameter{
		ArrayValue: values,
		Type:       ToolParamArray,
	}
}

// NewObjectParam creates an object tool parameter.
func NewObjectParam(values map[string]ToolParameter) ToolParameter {
	return ToolParameter{
		ObjectValue: values,
		Type:        ToolParamObject,
	}
}
