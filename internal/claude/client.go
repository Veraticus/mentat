package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Veraticus/mentat/internal/command"
)

const (
	// DefaultTimeout is the default timeout for Claude API calls.
	DefaultTimeout = 30 * time.Second
)

// commandRunner interface for command execution (allows mocking in tests).
type commandRunner interface {
	RunCommandContext(ctx context.Context, name string, args ...string) (string, error)
}

// realCommandRunner wraps the command package for production use.
type realCommandRunner struct{}

func (r *realCommandRunner) RunCommandContext(ctx context.Context, name string, args ...string) (string, error) {
	output, err := command.RunCommandContext(ctx, name, args...)
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w", err)
	}
	return output, nil
}

// Client implements the LLM interface for Claude CLI.
type Client struct {
	config    Config
	cmdRunner commandRunner
}

// jsonResponse is the JSON structure returned by Claude CLI.
type jsonResponse struct {
	Message   string               `json:"message"`
	Result    string               `json:"result"`   // Claude Code uses "result" field
	Type      string               `json:"type"`     // Response type
	IsError   bool                 `json:"is_error"` // Whether it's an error
	ToolCalls []jsonToolCall       `json:"tool_calls,omitempty"`
	Metadata  jsonResponseMetadata `json:"metadata,omitempty"`
	Usage     jsonUsage            `json:"usage,omitempty"`          // Usage stats
	TotalCost float64              `json:"total_cost_usd,omitempty"` // Cost in USD
}

// jsonToolCall is the JSON structure for tool calls.
type jsonToolCall struct {
	Name       string                       `json:"name"`
	Parameters map[string]jsonToolParameter `json:"parameters,omitempty"`
}

// jsonToolParameter is the JSON structure for tool parameters.
type jsonToolParameter struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// jsonResponseMetadata is the JSON structure for metadata.
type jsonResponseMetadata struct {
	Model      string `json:"model"`
	LatencyMs  int    `json:"latency_ms"`
	TokensUsed int    `json:"tokens_used"`
}

// jsonUsage is the JSON structure for usage stats.
type jsonUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheReadTokens int `json:"cache_read_input_tokens"`
}

// NewClient creates a new Claude CLI client.
func NewClient(config Config) (*Client, error) {
	// Validate configuration
	if config.Command == "" {
		return nil, fmt.Errorf("command path cannot be empty")
	}

	// Set default timeout if not specified
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}

	return &Client{
		config:    config,
		cmdRunner: &realCommandRunner{},
	}, nil
}

// Query executes a Claude query with the given prompt and session ID.
func (c *Client) Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error) {
	// Validate inputs
	if err := validateQueryInputs(prompt, sessionID); err != nil {
		return nil, err
	}

	// Prepare context with timeout
	queryCtx, cancel := prepareQueryContext(ctx, c.config.Timeout)
	defer cancel()

	// Build command arguments
	args := buildCommandArgs(c.config, prompt)

	// Execute the command
	output, err := c.cmdRunner.RunCommandContext(queryCtx, c.config.Command, args...)
	if err != nil {
		return nil, handleCommandError(queryCtx, err, output)
	}

	// Parse and convert response
	return parseAndConvertResponse(output, prompt)
}

func validateQueryInputs(prompt, sessionID string) error {
	if prompt == "" {
		return fmt.Errorf("prompt cannot be empty")
	}
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}
	return nil
}

func prepareQueryContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return context.WithTimeout(ctx, timeout)
	}
	// Return a no-op cancel function if context already has deadline
	return ctx, func() {}
}

func buildCommandArgs(config Config, prompt string) []string {
	args := []string{
		"--print", // Non-interactive mode
		"--output-format", "json",
		"--model", "sonnet",
	}

	if config.MCPConfigPath != "" {
		args = append(args, "--mcp-config", config.MCPConfigPath)
	}

	finalPrompt := prepareFinalPrompt(config.SystemPrompt, prompt)
	args = append(args, finalPrompt)

	return args
}

func prepareFinalPrompt(systemPrompt, userPrompt string) string {
	if systemPrompt == "" {
		return userPrompt
	}
	return fmt.Sprintf("<system>\n%s\n</system>\n\n<user>\n%s\n</user>", systemPrompt, userPrompt)
}

func handleCommandError(ctx context.Context, err error, output string) error {
	// Check if it's a context error (timeout/cancellation)
	if ctx.Err() != nil {
		return fmt.Errorf("claude query timed out: %w", ctx.Err())
	}

	// Check for authentication error in JSON output
	if authErr := checkAuthenticationError(err); authErr != nil {
		return authErr
	}

	// Extract useful error information
	errMsg := extractErrorMessage(output)
	return fmt.Errorf("failed to execute claude CLI: %w (output: %s)", err, errMsg)
}

func checkAuthenticationError(err error) error {
	errorStr := err.Error()
	jsonStart := strings.Index(errorStr, "{")
	if jsonStart < 0 {
		return nil
	}

	jsonOutput := extractJSONFromError(errorStr[jsonStart:])
	if jsonOutput == "" {
		return nil
	}

	var jsonResp jsonResponse
	parseErr := json.Unmarshal([]byte(jsonOutput), &jsonResp)
	if parseErr != nil {
		// If we can't parse the JSON, it's not an authentication error
		//nolint:nilerr // Intentionally returning nil - not an auth error
		return nil
	}

	if isAuthenticationError(jsonResp) {
		return &AuthenticationError{
			Message: "Claude Code authentication required",
		}
	}

	return nil
}

func extractJSONFromError(errorStr string) string {
	jsonEnd := strings.LastIndex(errorStr, "}")
	if jsonEnd < 0 {
		return ""
	}
	return errorStr[:jsonEnd+1]
}

func isAuthenticationError(resp jsonResponse) bool {
	return resp.IsError && (strings.Contains(resp.Result, "Invalid API key") ||
		strings.Contains(resp.Result, "Please run /login"))
}

func parseAndConvertResponse(output, prompt string) (*LLMResponse, error) {
	jsonResp, err := parseResponse(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse claude response: %w\nPrompt: %s\nOutput: %s", err, prompt, output)
	}

	// Check for authentication error
	if isAuthenticationError(*jsonResp) {
		return nil, &AuthenticationError{
			Message: "Claude Code authentication required",
		}
	}

	return buildLLMResponse(jsonResp), nil
}

func buildLLMResponse(jsonResp *jsonResponse) *LLMResponse {
	message := extractMessage(jsonResp)
	totalTokens := calculateTotalTokens(jsonResp)

	response := &LLMResponse{
		Message: message,
		Metadata: ResponseMetadata{
			ModelVersion: jsonResp.Metadata.Model,
			Latency:      time.Duration(jsonResp.Metadata.LatencyMs) * time.Millisecond,
			TokensUsed:   totalTokens,
		},
	}

	// Convert tool calls
	convertToolCalls(jsonResp, response)

	return response
}

func extractMessage(jsonResp *jsonResponse) string {
	message := strings.TrimSpace(jsonResp.Message)
	if message == "" && jsonResp.Result != "" {
		message = strings.TrimSpace(jsonResp.Result)
	}
	return message
}

func calculateTotalTokens(jsonResp *jsonResponse) int {
	if jsonResp.Metadata.TokensUsed > 0 {
		return jsonResp.Metadata.TokensUsed
	}
	if jsonResp.Usage.InputTokens > 0 {
		return jsonResp.Usage.InputTokens + jsonResp.Usage.OutputTokens
	}
	return 0
}

func convertToolCalls(jsonResp *jsonResponse, response *LLMResponse) {
	for _, tc := range jsonResp.ToolCalls {
		toolCall := ToolCall{
			Tool:       tc.Name,
			Parameters: make(map[string]ToolParameter),
		}

		for name, param := range tc.Parameters {
			toolCall.Parameters[name] = convertJSONParameter(param)
		}

		response.ToolCalls = append(response.ToolCalls, toolCall)
	}
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
		return convertStringParam(param)
	case "int":
		return convertIntParam(param)
	case "bool":
		return convertBoolParam(param)
	case "float":
		return convertFloatParam(param)
	case "array":
		return convertArrayParam(param)
	case "object":
		return convertObjectParam(param)
	default:
		return NewStringParam(fmt.Sprintf("%v", param.Value))
	}
}

// Helper functions to reduce complexity

func convertStringParam(param jsonToolParameter) ToolParameter {
	if str, ok := param.Value.(string); ok {
		return NewStringParam(str)
	}
	return NewStringParam(fmt.Sprintf("%v", param.Value))
}

func convertIntParam(param jsonToolParameter) ToolParameter {
	if val, ok := param.Value.(float64); ok {
		return NewIntParam(int(val))
	}
	return NewStringParam(fmt.Sprintf("%v", param.Value))
}

func convertBoolParam(param jsonToolParameter) ToolParameter {
	if val, ok := param.Value.(bool); ok {
		return NewBoolParam(val)
	}
	return NewStringParam(fmt.Sprintf("%v", param.Value))
}

func convertFloatParam(param jsonToolParameter) ToolParameter {
	if val, ok := param.Value.(float64); ok {
		return NewFloatParam(val)
	}
	return NewStringParam(fmt.Sprintf("%v", param.Value))
}

func convertArrayParam(param jsonToolParameter) ToolParameter {
	arr, ok := param.Value.([]any)
	if !ok {
		return NewStringParam(fmt.Sprintf("%v", param.Value))
	}

	var params []ToolParameter
	for _, item := range arr {
		if p := parseArrayItem(item); p != nil {
			params = append(params, *p)
		}
	}
	return NewArrayParam(params)
}

func parseArrayItem(item any) *ToolParameter {
	jsonParam, ok := item.(map[string]any)
	if !ok {
		return nil
	}

	typeStr, ok := jsonParam["type"].(string)
	if !ok {
		return nil
	}

	p := jsonToolParameter{
		Type:  typeStr,
		Value: jsonParam["value"],
	}
	result := convertJSONParameter(p)
	return &result
}

func convertObjectParam(param jsonToolParameter) ToolParameter {
	obj, ok := param.Value.(map[string]any)
	if !ok {
		return NewStringParam(fmt.Sprintf("%v", param.Value))
	}

	params := make(map[string]ToolParameter)
	for k, v := range obj {
		if p := parseObjectValue(v); p != nil {
			params[k] = *p
		}
	}
	return NewObjectParam(params)
}

func parseObjectValue(v any) *ToolParameter {
	jsonParam, ok := v.(map[string]any)
	if !ok {
		return nil
	}

	typeStr, ok := jsonParam["type"].(string)
	if !ok {
		return nil
	}

	p := jsonToolParameter{
		Type:  typeStr,
		Value: jsonParam["value"],
	}
	result := convertJSONParameter(p)
	return &result
}
