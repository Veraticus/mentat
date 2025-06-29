// Package claude provides interfaces for Claude Code SDK integration.
package claude

import (
	"context"
)

// LLM interface abstracts all Claude interactions.
type LLM interface {
	// Query sends a prompt to Claude and returns the response
	Query(ctx context.Context, prompt string, sessionID string) (*LLMResponse, error)
}
