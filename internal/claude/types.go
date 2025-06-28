package claude

// Config holds configuration for Claude CLI execution.
type Config struct {
	// MCPConfigPath is the path to the MCP configuration file.
	MCPConfigPath string
	
	// Timeout is the maximum time to wait for a response.
	Timeout int
	
	// MaxTokens limits the response length.
	MaxTokens int
}