// Package config provides configuration management for Mentat,
// including MCP (Model Context Protocol) configuration generation.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File permission constants.
const (
	// dirPermissions is used for directory creation (rwxr-x---).
	dirPermissions = 0750
	// filePermissions is used for config files (rw-------).
	filePermissions = 0600
)

// MCPConfig represents the top-level MCP configuration structure
// that Claude expects. Uses JSON tags to match Claude's format.
type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerConfig represents configuration for a single MCP server.
type ServerConfig struct {
	Transport TransportConfig `json:"transport"`
}

// TransportConfig represents the transport configuration for an MCP server.
type TransportConfig struct {
	Type string `json:"type"`           // "http" or "stdio"
	URL  string `json:"url,omitempty"`  // Required for HTTP transport
	Path string `json:"path,omitempty"` // Required for stdio transport
}

// GenerateMCPConfig creates a default MCP configuration with all
// required servers configured for HTTP transport on their standard ports.
func GenerateMCPConfig() MCPConfig {
	return MCPConfig{
		MCPServers: map[string]ServerConfig{
			"google-calendar": {
				Transport: TransportConfig{
					Type: "http",
					URL:  "http://localhost:3000",
				},
			},
			"google-contacts": {
				Transport: TransportConfig{
					Type: "http",
					URL:  "http://localhost:3001",
				},
			},
			"gmail": {
				Transport: TransportConfig{
					Type: "http",
					URL:  "http://localhost:3002",
				},
			},
			"todoist": {
				Transport: TransportConfig{
					Type: "http",
					URL:  "http://localhost:3003",
				},
			},
			"memory": {
				Transport: TransportConfig{
					Type: "http",
					URL:  "http://localhost:3004",
				},
			},
			"expensify": {
				Transport: TransportConfig{
					Type: "http",
					URL:  "http://localhost:3005",
				},
			},
		},
	}
}

// WriteMCPConfig writes the MCP configuration to a file.
// It creates parent directories if they don't exist.
func WriteMCPConfig(config MCPConfig, path string) error {
	// Validate config before writing
	if err := ValidateMCPConfig(config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create parent directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with restricted permissions
	if writeErr := os.WriteFile(path, data, filePermissions); writeErr != nil {
		return fmt.Errorf("failed to write file: %w", writeErr)
	}

	return nil
}

// LoadMCPConfig loads an MCP configuration from a file.
func LoadMCPConfig(path string) (MCPConfig, error) {
	var config MCPConfig

	// Sanitize path to prevent directory traversal
	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	if unmarshalErr := json.Unmarshal(data, &config); unmarshalErr != nil {
		return config, fmt.Errorf("failed to parse config: %w", unmarshalErr)
	}

	// Validate loaded config
	if validateErr := ValidateMCPConfig(config); validateErr != nil {
		return config, fmt.Errorf("loaded config is invalid: %w", validateErr)
	}

	return config, nil
}

// ValidateMCPConfig validates that an MCP configuration is well-formed.
func ValidateMCPConfig(config MCPConfig) error {
	if len(config.MCPServers) == 0 {
		return fmt.Errorf("no MCP servers configured")
	}

	for name, server := range config.MCPServers {
		// Validate transport type
		switch server.Transport.Type {
		case "http":
			if server.Transport.URL == "" {
				return fmt.Errorf("server %s: HTTP transport requires URL", name)
			}
		case "stdio":
			if server.Transport.Path == "" {
				return fmt.Errorf("server %s: stdio transport requires path", name)
			}
		default:
			return fmt.Errorf("server %s: invalid transport type '%s' (must be 'http' or 'stdio')",
				name, server.Transport.Type)
		}
	}

	return nil
}
