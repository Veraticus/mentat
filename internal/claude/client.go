package claude

import (
	"context"
	"fmt"
	"time"

	"github.com/Veraticus/mentat/internal/command"
)

// commandRunner interface for command execution (allows mocking in tests).
type commandRunner interface {
	RunCommandContext(ctx context.Context, name string, args ...string) (string, error)
}

// realCommandRunner wraps the command package for production use.
type realCommandRunner struct{}

func (r *realCommandRunner) RunCommandContext(ctx context.Context, name string, args ...string) (string, error) {
	return command.RunCommandContext(ctx, name, args...)
}

// Client implements the LLM interface for Claude CLI.
type Client struct {
	config    Config
	cmdRunner commandRunner
}

// jsonResponse is the JSON structure returned by Claude CLI.
type jsonResponse struct {
	Message   string                `json:"message"`
	ToolCalls []jsonToolCall        `json:"tool_calls,omitempty"`
	Metadata  jsonResponseMetadata  `json:"metadata,omitempty"`
}

// jsonToolCall is the JSON structure for tool calls.
type jsonToolCall struct {
	Name       string                        `json:"name"`
	Parameters map[string]jsonToolParameter  `json:"parameters,omitempty"`
}

// jsonToolParameter is the JSON structure for tool parameters.
type jsonToolParameter struct {
	Type  string      `json:"type"`
	Value any `json:"value"`
}

// jsonResponseMetadata is the JSON structure for metadata.
type jsonResponseMetadata struct {
	Model      string `json:"model"`
	LatencyMs  int    `json:"latency_ms"`
	TokensUsed int    `json:"tokens_used"`
}

// NewClient creates a new Claude CLI client.
func NewClient(config Config) (*Client, error) {
	// Validate configuration
	if config.Command == "" {
		return nil, fmt.Errorf("command path is required")
	}
	if config.MCPConfigPath == "" {
		return nil, fmt.Errorf("MCP config path is required")
	}

	// Set default timeout if not specified
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &Client{
		config:    config,
		cmdRunner: &realCommandRunner{},
	}, nil
}

// Query executes a Claude query with the given prompt and session ID.
func (c *Client) Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error) {
	// Validate inputs
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Apply timeout to context if not already present
	queryCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		queryCtx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	// Build command arguments
	args := []string{
		"chat",
		"--mcp-config", c.config.MCPConfigPath,
		"--session-id", sessionID,
		"--format", "json",
		prompt,
	}

	// Execute the command
	output, err := c.cmdRunner.RunCommandContext(queryCtx, c.config.Command, args...)
	if err != nil {
		// Check if it's a context error (timeout/cancellation)
		if queryCtx.Err() != nil {
			return nil, fmt.Errorf("claude query timed out: %w", queryCtx.Err())
		}
		// Try to extract useful error information from output
		errMsg := extractErrorMessage(output)
		return nil, fmt.Errorf("failed to execute claude CLI: %w (output: %s)", err, errMsg)
	}

	// Parse the response using our robust parser
	jsonResp, err := parseResponse(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse claude response: %w", err)
	}

	// Convert to LLMResponse
	response := &LLMResponse{
		Message: jsonResp.Message,
		Metadata: ResponseMetadata{
			ModelVersion: jsonResp.Metadata.Model,
			Latency:      time.Duration(jsonResp.Metadata.LatencyMs) * time.Millisecond,
			TokensUsed:   jsonResp.Metadata.TokensUsed,
		},
	}

	// Convert tool calls
	for _, tc := range jsonResp.ToolCalls {
		toolCall := ToolCall{
			Tool:       tc.Name,
			Parameters: make(map[string]ToolParameter),
		}
		
		// Convert parameters
		for name, param := range tc.Parameters {
			toolCall.Parameters[name] = convertJSONParameter(param)
		}
		
		response.ToolCalls = append(response.ToolCalls, toolCall)
	}

	return response, nil
}

// SetCommandRunner allows injecting a mock command runner for testing.
// This method is not part of the LLM interface and is only used in tests.
func (c *Client) SetCommandRunner(runner commandRunner) {
	c.cmdRunner = runner
}

// convertJSONParameter converts a JSON parameter to a ToolParameter.
func convertJSONParameter(param jsonToolParameter) ToolParameter {
	switch param.Type {
	case "string":
		if str, ok := param.Value.(string); ok {
			return NewStringParam(str)
		}
	case "int":
		if val, ok := param.Value.(float64); ok {
			return NewIntParam(int(val))
		}
	case "bool":
		if val, ok := param.Value.(bool); ok {
			return NewBoolParam(val)
		}
	case "float":
		if val, ok := param.Value.(float64); ok {
			return NewFloatParam(val)
		}
	case "array":
		if arr, ok := param.Value.([]any); ok {
			var params []ToolParameter
			for _, item := range arr {
				if jsonParam, ok := item.(map[string]any); ok {
					if typeStr, ok := jsonParam["type"].(string); ok {
						p := jsonToolParameter{
							Type:  typeStr,
							Value: jsonParam["value"],
						}
						params = append(params, convertJSONParameter(p))
					}
				}
			}
			return NewArrayParam(params)
		}
	case "object":
		if obj, ok := param.Value.(map[string]any); ok {
			params := make(map[string]ToolParameter)
			for k, v := range obj {
				if jsonParam, ok := v.(map[string]any); ok {
					if typeStr, ok := jsonParam["type"].(string); ok {
						p := jsonToolParameter{
							Type:  typeStr,
							Value: jsonParam["value"],
						}
						params[k] = convertJSONParameter(p)
					}
				}
			}
			return NewObjectParam(params)
		}
	}
	
	// Default to string if type is unknown
	return NewStringParam(fmt.Sprintf("%v", param.Value))
}