package agent

import (
	"context"
	"fmt"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// handler implements the Handler interface for processing messages.
type handler struct {
	llm claude.LLM
}

// NewHandler creates a new agent handler with the given LLM.
// Panics if llm is nil.
func NewHandler(llm claude.LLM) Handler {
	if llm == nil {
		panic("llm cannot be nil")
	}
	
	return &handler{
		llm: llm,
	}
}

// Process handles an incoming message through the agent pipeline.
// For the MVP implementation, this is a simple pass-through to Claude.
func (h *handler) Process(ctx context.Context, msg signal.IncomingMessage) error {
	// Generate session ID based on conversation
	sessionID := fmt.Sprintf("signal-%s", msg.From)
	
	// Query Claude with the message
	response, err := h.llm.Query(ctx, msg.Text, sessionID)
	if err != nil {
		return fmt.Errorf("LLM query failed: %w", err)
	}
	
	// For MVP, we just log the response - the actual sending happens elsewhere
	// In a full implementation, this would coordinate with validation and recovery
	_ = response.Message
	
	return nil
}