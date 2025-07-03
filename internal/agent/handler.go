package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/conversation"
	"github.com/Veraticus/mentat/internal/signal"
)

const (
	// DefaultMaxRetries is the default number of retry attempts for validation.
	DefaultMaxRetries = 2

	// DefaultValidationThreshold is the default confidence threshold for validation success.
	DefaultValidationThreshold = 0.8
)

// handler implements the Handler interface for processing messages.
type handler struct {
	llm                claude.LLM
	validationStrategy ValidationStrategy
	intentEnhancer     IntentEnhancer
	messenger          signal.Messenger
	sessionManager     conversation.SessionManager
	config             Config
	logger             *slog.Logger
}

// HandlerOption is a functional option for configuring a handler.
type HandlerOption func(*handler) error

// NewHandler creates a new agent handler with the given LLM and options.
// Returns an error if required dependencies are missing.
func NewHandler(llm claude.LLM, opts ...HandlerOption) (Handler, error) {
	if llm == nil {
		return nil, fmt.Errorf("handler creation failed: llm is required")
	}

	// Initialize with defaults
	h := &handler{
		llm:    llm,
		logger: slog.Default(),
		config: Config{
			MaxRetries:              DefaultMaxRetries,
			EnableIntentEnhancement: true,
			ValidationThreshold:     DefaultValidationThreshold,
		},
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(h); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Validate required dependencies
	if h.validationStrategy == nil {
		return nil, fmt.Errorf("handler creation failed: validation strategy is required")
	}
	if h.messenger == nil {
		return nil, fmt.Errorf("handler creation failed: messenger is required")
	}
	if h.sessionManager == nil {
		return nil, fmt.Errorf("handler creation failed: session manager is required")
	}

	return h, nil
}

// WithValidationStrategy sets the validation strategy.
func WithValidationStrategy(strategy ValidationStrategy) HandlerOption {
	return func(h *handler) error {
		if strategy == nil {
			return fmt.Errorf("invalid option: validation strategy cannot be nil")
		}
		h.validationStrategy = strategy
		return nil
	}
}

// WithIntentEnhancer sets the intent enhancer.
func WithIntentEnhancer(enhancer IntentEnhancer) HandlerOption {
	return func(h *handler) error {
		if enhancer == nil {
			return fmt.Errorf("invalid option: intent enhancer cannot be nil")
		}
		h.intentEnhancer = enhancer
		return nil
	}
}

// WithMessenger sets the messenger for sending responses.
func WithMessenger(messenger signal.Messenger) HandlerOption {
	return func(h *handler) error {
		if messenger == nil {
			return fmt.Errorf("invalid option: messenger cannot be nil")
		}
		h.messenger = messenger
		return nil
	}
}

// WithConfig sets the configuration.
func WithConfig(cfg Config) HandlerOption {
	return func(h *handler) error {
		// Validate config
		if cfg.MaxRetries < 0 {
			return fmt.Errorf("invalid config: max retries cannot be negative")
		}
		if cfg.ValidationThreshold < 0 || cfg.ValidationThreshold > 1 {
			return fmt.Errorf("invalid config: validation threshold must be between 0 and 1")
		}
		h.config = cfg
		return nil
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) HandlerOption {
	return func(h *handler) error {
		if logger == nil {
			return fmt.Errorf("invalid option: logger cannot be nil")
		}
		h.logger = logger
		return nil
	}
}

// WithSessionManager sets the session manager.
func WithSessionManager(manager conversation.SessionManager) HandlerOption {
	return func(h *handler) error {
		if manager == nil {
			return fmt.Errorf("invalid option: session manager cannot be nil")
		}
		h.sessionManager = manager
		return nil
	}
}

// Process handles an incoming message through the agent pipeline.
// It manages sessions, queries the LLM, and handles errors gracefully.
func (h *handler) Process(ctx context.Context, msg signal.IncomingMessage) error {
	// Log the incoming message
	h.logger.DebugContext(ctx, "processing message",
		slog.String("from", msg.From),
		slog.Int("text_length", len(msg.Text)))

	// Get or create session for conversation continuity
	sessionID := h.sessionManager.GetOrCreateSession(msg.From)
	h.logger.DebugContext(ctx, "session determined",
		slog.String("session_id", sessionID),
		slog.String("from", msg.From))

	// Get session history for context
	history := h.sessionManager.GetSessionHistory(sessionID)
	h.logger.DebugContext(ctx, "retrieved session history",
		slog.String("session_id", sessionID),
		slog.Int("history_length", len(history)))

	// Build context from history for multi-turn conversations
	// Currently using single message context for simplicity

	// Query Claude with the message
	response, err := h.llm.Query(ctx, msg.Text, sessionID)
	if err != nil {
		h.logger.ErrorContext(ctx, "LLM query failed",
			slog.Any("error", err),
			slog.String("session_id", sessionID),
			slog.String("from", msg.From))
		return fmt.Errorf("processing message from %s: LLM query failed: %w", msg.From, err)
	}

	h.logger.DebugContext(ctx, "received LLM response",
		slog.String("session_id", sessionID),
		slog.Int("response_length", len(response.Message)))

	// Perform validation with retry logic
	finalResponse := response
	retryCount := 0

	for {
		// Validate the response
		validationResult := h.validationStrategy.Validate(ctx, msg.Text, finalResponse.Message, h.llm)
		h.logger.DebugContext(ctx, "validation result",
			slog.String("status", string(validationResult.Status)),
			slog.Float64("confidence", validationResult.Confidence),
			slog.Any("issues", validationResult.Issues))

		// Check if we should retry
		if !h.validationStrategy.ShouldRetry(validationResult) || retryCount >= h.config.MaxRetries {
			// No retry needed or max retries reached
			if validationResult.Status == ValidationStatusFailed ||
				(validationResult.Status == ValidationStatusIncompleteSearch && retryCount >= h.config.MaxRetries) {
				// Generate recovery message
				recoveryMsg := h.validationStrategy.GenerateRecovery(
					ctx, msg.Text, finalResponse.Message, validationResult, h.llm)
				if recoveryMsg != "" {
					finalResponse.Message = recoveryMsg
				}
			}
			break
		}

		// Perform retry with guided prompt
		retryCount++
		h.logger.InfoContext(ctx, "retrying with guided prompt",
			slog.Int("retry_count", retryCount),
			slog.String("reason", string(validationResult.Status)))

		guidedPrompt := h.createGuidedPrompt(msg.Text, finalResponse.Message, validationResult)
		retryResponse, retryErr := h.llm.Query(ctx, guidedPrompt, sessionID)
		if retryErr != nil {
			h.logger.ErrorContext(ctx, "retry query failed",
				slog.Any("error", retryErr),
				slog.Int("retry_count", retryCount))
			// Keep the original response if retry fails
			break
		}

		finalResponse = retryResponse
		h.logger.DebugContext(ctx, "received retry response",
			slog.Int("response_length", len(finalResponse.Message)))
	}

	// Send the response back via messenger
	if sendErr := h.messenger.Send(ctx, msg.From, finalResponse.Message); sendErr != nil {
		h.logger.ErrorContext(ctx, "failed to send response",
			slog.Any("error", sendErr),
			slog.String("to", msg.From))
		return fmt.Errorf("processing message from %s: failed to send response: %w", msg.From, sendErr)
	}

	h.logger.InfoContext(ctx, "successfully processed message",
		slog.String("session_id", sessionID),
		slog.String("to", msg.From))

	return nil
}

// createGuidedPrompt creates a prompt that guides Claude to use the appropriate tools
// based on the validation result.
func (h *handler) createGuidedPrompt(originalRequest, _ string, validationResult ValidationResult) string {
	// Extract expected tools from metadata
	expectedTools := ""
	if tools, ok := validationResult.Metadata["expected_tools"]; ok {
		expectedTools = tools
	}

	// Build a guided prompt that hints at tool usage without being prescriptive
	var promptBuilder strings.Builder
	promptBuilder.WriteString("The user asked: ")
	promptBuilder.WriteString(originalRequest)
	promptBuilder.WriteString("\n\n")

	if validationResult.Status == ValidationStatusIncompleteSearch && expectedTools != "" {
		promptBuilder.WriteString("Please help the user with their request. ")
		h.addToolHints(&promptBuilder, expectedTools)
		promptBuilder.WriteString("\n\nBe thorough in gathering all relevant information to provide a complete answer.")
	} else {
		// Generic retry prompt for other validation failures
		promptBuilder.WriteString("Your previous response may not have fully addressed the request. ")
		if len(validationResult.Issues) > 0 {
			promptBuilder.WriteString("Issues identified: ")
			promptBuilder.WriteString(strings.Join(validationResult.Issues, ", "))
			promptBuilder.WriteString(". ")
		}
		promptBuilder.WriteString("\n\nPlease provide a more complete response.")
	}

	return promptBuilder.String()
}

// addToolHints adds hints based on expected tools to avoid nested complexity.
func (h *handler) addToolHints(promptBuilder *strings.Builder, expectedTools string) {
	// Add hints based on expected tools
	if strings.Contains(expectedTools, "memory") {
		promptBuilder.WriteString("Consider checking if there's any relevant context from previous conversations. ")
	}
	if strings.Contains(expectedTools, "calendar") {
		promptBuilder.WriteString("The user may be asking about scheduled events or appointments. ")
	}
	if strings.Contains(expectedTools, "email") {
		promptBuilder.WriteString("This might involve checking for relevant correspondence. ")
	}
	if strings.Contains(expectedTools, "tasks") {
		promptBuilder.WriteString("The user may be asking about pending tasks or todos. ")
	}
}
