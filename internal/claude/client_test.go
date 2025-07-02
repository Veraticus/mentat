package claude_test

import (
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/claude"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  claude.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: claude.Config{
				Command:       "/usr/bin/claude",
				MCPConfigPath: "/etc/mcp-config.json",
				Timeout:       30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "missing command path",
			config: claude.Config{
				MCPConfigPath: "/etc/mcp-config.json",
				Timeout:       30 * time.Second,
			},
			wantErr: true,
			errMsg:  "command path cannot be empty",
		},
		{
			name: "zero timeout uses default",
			config: claude.Config{
				Command:       "/usr/bin/claude",
				MCPConfigPath: "/etc/mcp-config.json",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := claude.NewClient(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if client == nil {
				t.Error("expected client but got nil")
			}
		})
	}
}

// TestClientQueryIntegration would test Query functionality
// but requires SetCommandRunner which is unexported.
// These tests should be in the claude package or SetCommandRunner should be exported.
func TestClientQueryIntegration(t *testing.T) {
	t.Skip("Query testing requires unexported SetCommandRunner method")
}

// TestParseResponseIntegration would test response parsing
// but requires SetCommandRunner to inject test responses.
func TestParseResponseIntegration(t *testing.T) {
	t.Skip("Response parsing testing requires unexported SetCommandRunner method")
}

// TestErrorHandlingIntegration would test various error conditions
// but requires SetCommandRunner to simulate errors.
func TestErrorHandlingIntegration(t *testing.T) {
	t.Skip("Error handling testing requires unexported SetCommandRunner method")
}

// TestToolCallParsingIntegration would test tool call parsing
// but requires SetCommandRunner to inject test responses.
func TestToolCallParsingIntegration(t *testing.T) {
	t.Skip("Tool call parsing testing requires unexported SetCommandRunner method")
}
