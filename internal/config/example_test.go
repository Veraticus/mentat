package config_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Veraticus/mentat/internal/config"
)

func ExampleGenerateMCPConfig() {
	// Generate the default MCP configuration
	mcpConfig := config.GenerateMCPConfig()

	// Access individual server configurations
	if calendarServer, exists := mcpConfig.MCPServers["google-calendar"]; exists {
		fmt.Printf("Calendar server URL: %s\n", calendarServer.Transport.URL)
	}

	// Output:
	// Calendar server URL: http://localhost:3000
}

func ExampleWriteMCPConfig() {
	// Create a temporary directory for the example
	tmpDir, err := os.MkdirTemp("", "mcp-example")
	if err != nil {
		fmt.Println("Error creating temp dir:", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Generate and write MCP config to a file
	mcpConfig := config.GenerateMCPConfig()
	configPath := filepath.Join(tmpDir, "mcp-config.json")
	if writeErr := config.WriteMCPConfig(mcpConfig, configPath); writeErr != nil {
		fmt.Println("Error writing config:", writeErr)
		return
	}

	// The config is now written to disk
	fmt.Println("MCP config written successfully")

	// Output:
	// MCP config written successfully
}

func ExampleLoadMCPConfig() {
	// Create a temporary directory and config for the example
	tmpDir, _ := os.MkdirTemp("", "mcp-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write a config first
	mcpConfig := config.GenerateMCPConfig()
	configPath := filepath.Join(tmpDir, "mcp-config.json")
	_ = config.WriteMCPConfig(mcpConfig, configPath)

	// Load the configuration
	loadedConfig, err := config.LoadMCPConfig(configPath)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	fmt.Printf("Loaded MCP config with %d servers\n", len(loadedConfig.MCPServers))

	// Output:
	// Loaded MCP config with 6 servers
}

func ExampleValidateMCPConfig() {
	// Create a custom config to validate
	customConfig := config.MCPConfig{
		MCPServers: map[string]config.ServerConfig{
			"custom": {
				Transport: config.TransportConfig{
					Type: "http",
					URL:  "http://localhost:9000",
				},
			},
		},
	}

	// Validate the configuration
	if err := config.ValidateMCPConfig(customConfig); err != nil {
		fmt.Printf("Invalid config: %v\n", err)
	} else {
		fmt.Println("Config is valid")
	}

	// Output:
	// Config is valid
}
