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

	// ComplexityThresholdMedium is the threshold for medium complexity requests.
	ComplexityThresholdMedium = 0.5

	// ComplexityThresholdHigh is the threshold for high complexity requests.
	ComplexityThresholdHigh = 0.7
)

// handler implements the Handler interface for processing messages.
type handler struct {
	llm                claude.LLM
	validationStrategy ValidationStrategy
	intentEnhancer     IntentEnhancer
	messenger          signal.Messenger
	sessionManager     conversation.SessionManager
	complexityAnalyzer ComplexityAnalyzer
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

// WithComplexityAnalyzer sets the complexity analyzer.
func WithComplexityAnalyzer(analyzer ComplexityAnalyzer) HandlerOption {
	return func(h *handler) error {
		if analyzer == nil {
			return fmt.Errorf("invalid option: complexity analyzer cannot be nil")
		}
		h.complexityAnalyzer = analyzer
		return nil
	}
}

// Process handles an incoming message through the agent pipeline.
// It manages sessions, queries the LLM, and handles errors gracefully.
func (h *handler) Process(ctx context.Context, msg signal.IncomingMessage) error {
	// Create error handler for this process
	errorHandler := NewProcessingErrorHandler(h.messenger)

	// Wrap the entire process in error recovery
	return errorHandler.WrapWithRecovery(ctx, msg.From, msg.Text, func() error {
		// Log the incoming message
		h.logger.DebugContext(ctx, "processing message",
			slog.String("from", msg.From),
			slog.Int("text_length", len(msg.Text)))

		// Get or create session for conversation continuity
		sessionID := h.sessionManager.GetOrCreateSession(msg.From)
		h.logger.DebugContext(ctx, "session determined",
			slog.String("session_id", sessionID),
			slog.String("from", msg.From))

		// Prepare the request with optional complexity guidance
		requestToSend := h.prepareRequest(ctx, msg.Text, sessionID)

		// Query Claude with the message (possibly enhanced with guidance)
		response, err := h.llm.Query(ctx, requestToSend, sessionID)
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
		finalResponse := h.validateAndRetry(ctx, msg.Text, response, sessionID)

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
	})
}

// Query processes a request and returns the response without sending it.
// This method is designed for queue-based systems that need to manage responses.
func (h *handler) Query(ctx context.Context, request string, sessionID string) (claude.LLMResponse, error) {
	// Create error recovery handler
	errorRecovery := NewErrorRecovery()

	// Log the query
	h.logger.DebugContext(ctx, "processing query",
		slog.String("session_id", sessionID),
		slog.Int("text_length", len(request)))

	// Prepare the request with optional complexity guidance
	requestToSend := h.prepareRequest(ctx, request, sessionID)

	// Query Claude with the message (possibly enhanced with guidance)
	response, err := h.llm.Query(ctx, requestToSend, sessionID)
	if err != nil {
		h.logger.ErrorContext(ctx, "LLM query failed",
			slog.Any("error", err),
			slog.String("session_id", sessionID))
		// Return user-friendly error message in the response
		return claude.LLMResponse{
			Message: errorRecovery.GenerateContextualMessage(err, request),
		}, fmt.Errorf("query failed: %w", err)
	}

	h.logger.DebugContext(ctx, "received LLM response",
		slog.String("session_id", sessionID),
		slog.Int("response_length", len(response.Message)))

	// Perform validation with retry logic
	finalResponse := h.validateAndRetry(ctx, request, response, sessionID)

	h.logger.InfoContext(ctx, "successfully processed query",
		slog.String("session_id", sessionID))

	return *finalResponse, nil
}

// prepareRequest prepares the request with optional complexity guidance.
func (h *handler) prepareRequest(ctx context.Context, originalText string, sessionID string) string {
	// Get session history for context
	history := h.sessionManager.GetSessionHistory(sessionID)
	h.logger.DebugContext(ctx, "retrieved session history",
		slog.String("session_id", sessionID),
		slog.Int("history_length", len(history)))

	// Build context from history for multi-turn conversations
	// Currently using single message context for simplicity

	// Analyze request complexity and add gentle guidance if needed
	if h.complexityAnalyzer == nil {
		return originalText
	}

	complexityResult := h.complexityAnalyzer.Analyze(originalText)
	// Consider complex if score > 0.5 or requires decomposition
	if complexityResult.Score <= ComplexityThresholdMedium && !complexityResult.RequiresDecomposition {
		return originalText
	}

	// Add gentle guidance based on complexity factors
	guidedRequest := h.addComplexityGuidance(originalText, complexityResult)

	h.logger.DebugContext(ctx, "added complexity guidance",
		slog.Float64("complexity_score", complexityResult.Score),
		slog.Int("estimated_steps", complexityResult.Steps),
		slog.Any("factors", complexityResult.Factors))

	return guidedRequest
}

// validateAndRetry performs validation with retry logic.
func (h *handler) validateAndRetry(
	ctx context.Context,
	originalRequest string,
	initialResponse *claude.LLMResponse,
	sessionID string,
) *claude.LLMResponse {
	finalResponse := initialResponse
	retryCount := 0

	for {
		// Validate the response
		validationResult := h.validationStrategy.Validate(ctx, originalRequest, finalResponse.Message, sessionID, h.llm)
		h.logger.DebugContext(ctx, "validation result",
			slog.String("status", string(validationResult.Status)),
			slog.Float64("confidence", validationResult.Confidence),
			slog.Any("issues", validationResult.Issues))

		// Check if we should retry
		if !h.validationStrategy.ShouldRetry(validationResult) || retryCount >= h.config.MaxRetries {
			// Handle terminal states
			if h.shouldGenerateRecovery(validationResult, retryCount) {
				recoveryMsg := h.validationStrategy.GenerateRecovery(
					ctx, originalRequest, finalResponse.Message, sessionID, validationResult, h.llm)
				if recoveryMsg != "" {
					finalResponse.Message = recoveryMsg
				}
			}
			break
		}

		// Perform retry
		retryResponse, ok := h.performRetry(
			ctx,
			originalRequest,
			finalResponse.Message,
			validationResult,
			sessionID,
			retryCount,
		)
		if !ok {
			break
		}

		finalResponse = retryResponse
		retryCount++
	}

	return finalResponse
}

// shouldGenerateRecovery determines if a recovery message should be generated.
func (h *handler) shouldGenerateRecovery(result ValidationResult, retryCount int) bool {
	return result.Status == ValidationStatusFailed ||
		result.Status == ValidationStatusPartial ||
		(result.Status == ValidationStatusIncompleteSearch && retryCount >= h.config.MaxRetries)
}

// performRetry attempts to retry with a guided prompt.
func (h *handler) performRetry(
	ctx context.Context,
	originalRequest, currentResponse string,
	validationResult ValidationResult,
	sessionID string,
	retryCount int,
) (*claude.LLMResponse, bool) {
	h.logger.InfoContext(ctx, "retrying with guided prompt",
		slog.Int("retry_count", retryCount+1),
		slog.String("reason", string(validationResult.Status)))

	guidedPrompt := h.createGuidedPrompt(originalRequest, currentResponse, validationResult)
	retryResponse, err := h.llm.Query(ctx, guidedPrompt, sessionID)
	if err != nil {
		h.logger.ErrorContext(ctx, "retry query failed",
			slog.Any("error", err),
			slog.Int("retry_count", retryCount+1))
		return nil, false
	}

	h.logger.DebugContext(ctx, "received retry response",
		slog.Int("response_length", len(retryResponse.Message)))

	return retryResponse, true
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

// addComplexityGuidance adds gentle guidance based on detected complexity factors.
// It provides thoroughness hints without micromanaging or prescribing specific tools.
func (h *handler) addComplexityGuidance(originalRequest string, complexity ComplexityResult) string {
	// Return early if not complex enough
	if complexity.Score <= ComplexityThresholdMedium && !complexity.RequiresDecomposition {
		return originalRequest
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString(originalRequest)
	promptBuilder.WriteString("\n\n")

	// Build guidance based on complexity factors
	guidance := h.buildComplexityGuidance(complexity)
	promptBuilder.WriteString(guidance)

	return promptBuilder.String()
}

// buildComplexityGuidance generates guidance text based on complexity factors.
func (h *handler) buildComplexityGuidance(complexity ComplexityResult) string {
	var guidanceBuilder strings.Builder

	// Analyze factors
	factorFlags := h.analyzeComplexityFactors(complexity.Factors)

	// Add contextual hints based on detected patterns
	if factorFlags.hasMultiStep && complexity.Steps > 3 {
		guidanceBuilder.WriteString("This request may involve multiple steps. ")
		guidanceBuilder.WriteString("Consider breaking it down into logical parts for thoroughness. ")
	}

	if factorFlags.hasTemporal {
		guidanceBuilder.WriteString("Time-related information might be relevant to this request. ")
	}

	if factorFlags.hasDataIntegration {
		guidanceBuilder.WriteString("Multiple sources of information may be helpful for a complete answer. ")
	}

	if factorFlags.hasCoordination {
		guidanceBuilder.WriteString("This might involve coordinating information about multiple people or events. ")
	}

	// Add a general thoroughness reminder for complex requests
	if complexity.Score > ComplexityThresholdHigh {
		guidanceBuilder.WriteString("\nPlease be thorough in addressing all aspects of this request.")
	}

	return guidanceBuilder.String()
}

// complexityFactorFlags holds flags for different complexity factors.
type complexityFactorFlags struct {
	hasMultiStep       bool
	hasTemporal        bool
	hasDataIntegration bool
	hasCoordination    bool
}

// analyzeComplexityFactors analyzes factors and returns flags.
func (h *handler) analyzeComplexityFactors(factors []ComplexityFactor) complexityFactorFlags {
	flags := complexityFactorFlags{}

	for _, factor := range factors {
		switch factor.Type {
		case ComplexityFactorMultiStep:
			flags.hasMultiStep = true
		case ComplexityFactorTemporal:
			flags.hasTemporal = true
		case ComplexityFactorDataIntegration:
			flags.hasDataIntegration = true
		case ComplexityFactorCoordination:
			flags.hasCoordination = true
		case ComplexityFactorAmbiguous, ComplexityFactorConditional:
			// These factors contribute to complexity but don't require specific guidance
		}
	}

	return flags
}
