package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMCPConfig(t *testing.T) {
	config := GenerateMCPConfig()

	// Test that all required servers are present
	requiredServers := []string{
		"google-calendar",
		"google-contacts",
		"gmail",
		"todoist",
		"memory",
		"expensify",
	}

	if len(config.MCPServers) != len(requiredServers) {
		t.Errorf("expected %d servers, got %d", len(requiredServers), len(config.MCPServers))
	}

	// Check each server configuration
	for _, serverName := range requiredServers {
		server, exists := config.MCPServers[serverName]
		if !exists {
			t.Errorf("missing required server: %s", serverName)
			continue
		}

		// All servers should use HTTP transport
		if server.Transport.Type != "http" {
			t.Errorf("server %s: expected transport type 'http', got '%s'", serverName, server.Transport.Type)
		}

		// Check URL format
		if !strings.HasPrefix(server.Transport.URL, "http://localhost:") {
			t.Errorf("server %s: URL should start with 'http://localhost:', got '%s'", serverName, server.Transport.URL)
		}
	}

	// Verify specific port assignments
	expectedPorts := map[string]string{
		"google-calendar": "http://localhost:3000",
		"google-contacts": "http://localhost:3001",
		"gmail":           "http://localhost:3002",
		"todoist":         "http://localhost:3003",
		"memory":          "http://localhost:3004",
		"expensify":       "http://localhost:3005",
	}

	for serverName, expectedURL := range expectedPorts {
		if server, exists := config.MCPServers[serverName]; exists {
			if server.Transport.URL != expectedURL {
				t.Errorf("server %s: expected URL '%s', got '%s'", serverName, expectedURL, server.Transport.URL)
			}
		}
	}
}

func TestMCPConfigJSONSerialization(t *testing.T) {
	config := GenerateMCPConfig()

	// Test that it serializes to valid JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	// Test that it can be deserialized
	var decoded MCPConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	// Verify structure after round-trip
	if len(decoded.MCPServers) != len(config.MCPServers) {
		t.Errorf("server count changed after serialization: expected %d, got %d",
			len(config.MCPServers), len(decoded.MCPServers))
	}

	// Check JSON field names are correct (mcpServers not MCPServers)
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"mcpServers"`) {
		t.Error("JSON should contain 'mcpServers' field, not 'MCPServers'")
	}
}

func TestWriteMCPConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-mcp-config.json")

	config := GenerateMCPConfig()

	// Test writing config
	err := WriteMCPConfig(config, configPath)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Verify file exists
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		t.Fatal("config file was not created")
	}

	// Test that written file contains valid JSON
	data, err := os.ReadFile(configPath) //nolint:gosec // test path is controlled
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var decoded MCPConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("written file contains invalid JSON: %v", err)
	}

	// Verify content matches
	if len(decoded.MCPServers) != len(config.MCPServers) {
		t.Errorf("written config has different server count: expected %d, got %d",
			len(config.MCPServers), len(decoded.MCPServers))
	}
}

func TestWriteMCPConfigCreatesDirectory(t *testing.T) {
	// Test that WriteMCPConfig creates parent directories if needed
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nested", "dir", "mcp-config.json")

	config := GenerateMCPConfig()

	err := WriteMCPConfig(config, configPath)
	if err != nil {
		t.Fatalf("failed to write config with nested dirs: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created in nested directory")
	}
}

func TestLoadMCPConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-mcp-config.json")

	// Write a test config
	originalConfig := GenerateMCPConfig()
	if err := WriteMCPConfig(originalConfig, configPath); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Test loading
	loadedConfig, err := LoadMCPConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify loaded config matches original
	if len(loadedConfig.MCPServers) != len(originalConfig.MCPServers) {
		t.Errorf("loaded config has different server count: expected %d, got %d",
			len(originalConfig.MCPServers), len(loadedConfig.MCPServers))
	}

	// Check specific server
	if calendar, exists := loadedConfig.MCPServers["google-calendar"]; exists {
		if calendar.Transport.URL != "http://localhost:3000" {
			t.Errorf("loaded config has wrong URL for google-calendar: %s", calendar.Transport.URL)
		}
	} else {
		t.Error("loaded config missing google-calendar server")
	}
}

func TestLoadMCPConfigFileNotFound(t *testing.T) {
	_, err := LoadMCPConfig("/non/existent/path/config.json")
	if err == nil {
		t.Error("expected error when loading non-existent file")
	}
}

func TestLoadMCPConfigInvalidJSON(t *testing.T) {
	// Create a temporary file with invalid JSON
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(configPath, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadMCPConfig(configPath)
	if err == nil {
		t.Error("expected error when loading invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse config") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestWriteMCPConfigInvalidConfig(t *testing.T) {
	// Test writing an invalid config
	invalidConfig := MCPConfig{
		MCPServers: map[string]ServerConfig{
			"test": {
				Transport: TransportConfig{
					Type: "invalid",
				},
			},
		},
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid-config.json")

	err := WriteMCPConfig(invalidConfig, configPath)
	if err == nil {
		t.Error("expected error when writing invalid config")
	}
	if !strings.Contains(err.Error(), "invalid config") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestValidateMCPConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    MCPConfig
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config",
			config:    GenerateMCPConfig(),
			wantError: false,
		},
		{
			name: "empty servers",
			config: MCPConfig{
				MCPServers: map[string]ServerConfig{},
			},
			wantError: true,
			errorMsg:  "no MCP servers configured",
		},
		{
			name: "invalid transport type",
			config: MCPConfig{
				MCPServers: map[string]ServerConfig{
					"test": {
						Transport: TransportConfig{
							Type: "invalid",
							URL:  "http://localhost:3000",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "invalid transport type",
		},
		{
			name: "missing URL for HTTP transport",
			config: MCPConfig{
				MCPServers: map[string]ServerConfig{
					"test": {
						Transport: TransportConfig{
							Type: "http",
							URL:  "",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "HTTP transport requires URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMCPConfig(tt.config)
			if tt.wantError {
				if err == nil {
					t.Error("expected validation error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%v'", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected validation error: %v", err)
				}
			}
		})
	}
}
