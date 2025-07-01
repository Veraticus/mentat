package config_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Veraticus/mentat/internal/config"
)

// Example showing how MCP config integrates with Claude client.
func Example_integrationWithClaude() {
	// Create a temporary directory for config
	tmpDir, err := os.MkdirTemp("", "mentat-config")
	if err != nil {
		fmt.Println("Error creating temp dir:", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Generate and write MCP config to a file
	mcpConfig := config.GenerateMCPConfig()
	mcpConfigPath := filepath.Join(tmpDir, "mcp-config.json")
	if err := config.WriteMCPConfig(mcpConfig, mcpConfigPath); err != nil {
		fmt.Println("Error writing MCP config:", err)
		return
	}

	// In actual usage, this path would be passed to Claude CLI:
	// claude --mcp-config-path mcpConfigPath ...

	fmt.Printf("Claude can now access %d MCP servers\n", len(mcpConfig.MCPServers))

	// Output:
	// Claude can now access 6 MCP servers
}

// Example demonstrating dynamic MCP config modification.
func Example_dynamicMCPConfig() {
	// Start with the default config
	mcpConfig := config.GenerateMCPConfig()

	// Add a custom server
	mcpConfig.MCPServers["custom-tool"] = config.ServerConfig{
		Transport: config.TransportConfig{
			Type: "stdio",
			Path: "/usr/local/bin/custom-mcp-server",
		},
	}

	// Remove a server we don't need
	delete(mcpConfig.MCPServers, "expensify")

	// Validate the modified config
	if err := config.ValidateMCPConfig(mcpConfig); err != nil {
		fmt.Printf("Invalid config: %v\n", err)
		return
	}

	fmt.Printf("Modified config has %d servers\n", len(mcpConfig.MCPServers))

	// Output:
	// Modified config has 6 servers
}
