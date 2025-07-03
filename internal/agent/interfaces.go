// Package agent provides the multi-agent validation system for processing messages.
package agent

import (
	"context"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/signal"
)

// Handler processes incoming messages through the multi-agent system.
type Handler interface {
	// Process handles an incoming message through the agent pipeline
	Process(ctx context.Context, msg signal.IncomingMessage) error
}

// ValidationStrategy allows pluggable validation approaches.
type ValidationStrategy interface {
	// Validate checks if Claude's response adequately addresses the request
	Validate(ctx context.Context, request, response string, llm claude.LLM) ValidationResult

	// ShouldRetry determines if validation should be retried based on the result
	ShouldRetry(result ValidationResult) bool

	// GenerateRecovery creates a natural recovery message for validation failures
	GenerateRecovery(ctx context.Context, request, response string, result ValidationResult, llm claude.LLM) string
}

// IntentEnhancer provides gentle guidance without prescribing exact tools.
type IntentEnhancer interface {
	// Enhance adds helpful context to the original request
	Enhance(originalRequest string) string

	// ShouldEnhance determines if a request would benefit from enhancement
	ShouldEnhance(request string) bool
}

// ComplexityAnalyzer identifies multi-step requests and analyzes request complexity.
type ComplexityAnalyzer interface {
	// Analyze examines a request and returns complexity information
	Analyze(request string) ComplexityResult

	// IsComplex determines if a request requires multi-step processing
	IsComplex(request string) bool
}
