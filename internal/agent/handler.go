package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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

	// Validation reason constants for logging.
	validationReasonSimpleQuery       = "simple_query"
	validationReasonComplexQuery      = "complex_query"
	validationReasonNeedsValidation   = "needs_validation_flag_true"
	validationReasonProgressNil       = "progress_nil_defaulting_to_validation"
	validationReasonProgressNilLaunch = "progress_nil_requires_validation"
)

// handler implements the Handler interface for processing messages.
type handler struct {
	llm                claude.LLM
	validationStrategy ValidationStrategy
	intentEnhancer     IntentEnhancer
	messenger          signal.Messenger
	sessionManager     conversation.SessionManager
	complexityAnalyzer ComplexityAnalyzer
	asyncValidator     *AsyncValidator
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

	// Create async validator if we have the required dependencies
	if h.validationStrategy != nil && h.messenger != nil {
		h.asyncValidator = NewAsyncValidator(h.messenger, h.validationStrategy, h.logger)
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

		// Get initial response with progress check
		initialResponse, err := h.getInitialResponse(ctx, msg.Text, sessionID)
		if err != nil {
			h.logger.ErrorContext(ctx, "initial response failed",
				slog.Any("error", err),
				slog.String("session_id", sessionID),
				slog.String("from", msg.From))
			return fmt.Errorf("processing message from %s: initial response failed: %w", msg.From, err)
		}

		// Check if initial response needs continuation
		var finalResponse *claude.LLMResponse

		switch {
		case initialResponse.Progress != nil && initialResponse.Progress.NeedsContinuation:
			h.logger.InfoContext(ctx, "initial response needs continuation",
				slog.String("session_id", sessionID),
				slog.String("status", initialResponse.Progress.Status),
				slog.String("from", msg.From))
			// Handle multi-step processing
			continuationResponse, continueErr := h.continueWithProgress(ctx, initialResponse, &msg, sessionID)
			if continueErr != nil {
				h.logger.ErrorContext(ctx, "continuation failed",
					slog.Any("error", continueErr),
					slog.String("session_id", sessionID))
				return fmt.Errorf("processing message from %s: continuation failed: %w", msg.From, continueErr)
			}
			finalResponse = continuationResponse

		case initialResponse.Progress != nil && !initialResponse.Progress.NeedsValidation:
			// Simple query completed without validation
			h.logger.InfoContext(ctx, "validation decision: skipping validation for simple query",
				slog.String("session_id", sessionID),
				slog.String("validation_reason", validationReasonSimpleQuery),
				slog.Bool("progress_nil", false),
				slog.Bool("needs_validation_flag", initialResponse.Progress.NeedsValidation),
				slog.String("query_complexity", "simple"),
				slog.String("status", initialResponse.Progress.Status),
				slog.String("from", msg.From))
			finalResponse = initialResponse

		default:
			// For complex queries or when progress info is missing, we'll validate asynchronously
			validationReason, needsValidation, progressNil := h.determineValidationDecision(initialResponse.Progress)
			if validationReason == validationReasonSimpleQuery {
				validationReason = validationReasonComplexQuery // Override for default case
			}

			h.logger.InfoContext(ctx, "validation decision: will validate asynchronously",
				slog.String("session_id", sessionID),
				slog.String("validation_reason", validationReason),
				slog.Bool("progress_nil", progressNil),
				slog.Bool("needs_validation_flag", needsValidation),
				slog.String("query_complexity", "complex"),
				slog.String("from", msg.From))
			finalResponse = initialResponse
		}

		// Send the response back via messenger immediately
		if sendErr := h.messenger.Send(ctx, msg.From, finalResponse.Message); sendErr != nil {
			h.logger.ErrorContext(ctx, "failed to send response",
				slog.Any("error", sendErr),
				slog.String("to", msg.From))
			return fmt.Errorf("processing message from %s: failed to send response: %w", msg.From, sendErr)
		}

		// Launch async validation if needed
		if finalResponse.Progress == nil || finalResponse.Progress.NeedsValidation {
			launchReason := validationReasonNeedsValidation
			if finalResponse.Progress == nil {
				launchReason = validationReasonProgressNilLaunch
			}
			h.logger.InfoContext(ctx, "launching async validation",
				slog.String("session_id", sessionID),
				slog.String("launch_reason", launchReason),
				slog.Bool("progress_nil", finalResponse.Progress == nil),
				slog.String("from", msg.From))

			// Use AsyncValidator to handle background validation
			// Not passing request context intentionally - validation should continue independently
			//nolint:contextcheck // Validation needs independent context to avoid cancellation
			h.asyncValidator.StartValidation(msg.From, msg.Text, finalResponse, sessionID)
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

	// Get initial response with progress check
	initialResponse, err := h.getInitialResponse(ctx, request, sessionID)
	if err != nil {
		h.logger.ErrorContext(ctx, "initial response failed",
			slog.Any("error", err),
			slog.String("session_id", sessionID))
		// Return user-friendly error message in the response
		return claude.LLMResponse{
			Message: errorRecovery.GenerateContextualMessage(err, request),
		}, fmt.Errorf("query failed: %w", err)
	}

	// Check if initial response needs continuation
	var finalResponse *claude.LLMResponse

	switch {
	case initialResponse.Progress != nil && initialResponse.Progress.NeedsContinuation:
		h.logger.InfoContext(ctx, "initial response needs continuation",
			slog.String("session_id", sessionID),
			slog.String("status", initialResponse.Progress.Status))
		// Handle multi-step processing
		msg := &signal.IncomingMessage{Text: request}
		continuationResponse, continueErr := h.continueWithProgress(ctx, initialResponse, msg, sessionID)
		if continueErr != nil {
			h.logger.ErrorContext(ctx, "continuation failed",
				slog.Any("error", continueErr),
				slog.String("session_id", sessionID))
			return claude.LLMResponse{
				Message: errorRecovery.GenerateContextualMessage(continueErr, request),
			}, fmt.Errorf("query continuation failed: %w", continueErr)
		}
		finalResponse = continuationResponse

	case initialResponse.Progress != nil && !initialResponse.Progress.NeedsValidation:
		// Simple query completed without validation
		h.logger.InfoContext(ctx, "validation decision: skipping validation for simple query",
			slog.String("session_id", sessionID),
			slog.String("validation_reason", validationReasonSimpleQuery),
			slog.Bool("progress_nil", false),
			slog.Bool("needs_validation_flag", initialResponse.Progress.NeedsValidation),
			slog.String("query_complexity", "simple"),
			slog.String("status", initialResponse.Progress.Status))
		return *initialResponse, nil

	default:
		// For complex queries or when progress info is missing, return initial response
		// and schedule async validation if needed
		validationReason, needsValidation, progressNil := h.determineValidationDecision(initialResponse.Progress)
		if validationReason == validationReasonSimpleQuery {
			validationReason = validationReasonComplexQuery // Override for default case
		}

		h.logger.InfoContext(ctx, "validation decision: returning response immediately",
			slog.String("session_id", sessionID),
			slog.String("validation_reason", validationReason),
			slog.Bool("progress_nil", progressNil),
			slog.Bool("needs_validation_flag", needsValidation),
			slog.String("query_complexity", "complex"))

		// Return the initial response immediately
		// Note: Validation should be handled by the caller if needed
		finalResponse = initialResponse
	}

	h.logger.InfoContext(ctx, "successfully processed query",
		slog.String("session_id", sessionID))

	return *finalResponse, nil
}

// getInitialResponse queries Claude and returns the initial response with progress information.
// This method is optimized for simple queries that can complete quickly without validation.
func (h *handler) getInitialResponse(
	ctx context.Context,
	request string,
	sessionID string,
) (*claude.LLMResponse, error) {
	// Prepare the request with optional complexity guidance
	requestToSend := h.prepareRequest(ctx, request, sessionID)

	// Query Claude with the message (possibly enhanced with guidance)
	response, err := h.llm.Query(ctx, requestToSend, sessionID)
	if err != nil {
		return nil, fmt.Errorf("LLM query failed: %w", err)
	}

	// Log the full response for debugging
	h.logger.InfoContext(ctx, "received initial LLM response",
		slog.String("session_id", sessionID),
		slog.String("message", response.Message),
		slog.Int("response_length", len(response.Message)),
		slog.Bool("has_progress", response.Progress != nil),
		slog.String("model", response.Metadata.ModelVersion),
		slog.Duration("latency", response.Metadata.Latency),
		slog.Int("tokens_used", response.Metadata.TokensUsed))

	// Log progress information if available
	if response.Progress != nil {
		h.logger.InfoContext(ctx, "progress information",
			slog.String("session_id", sessionID),
			slog.Bool("needs_continuation", response.Progress.NeedsContinuation),
			slog.String("status", response.Progress.Status),
			slog.String("progress_message", response.Progress.Message),
			slog.Int("estimated_remaining", response.Progress.EstimatedRemaining))
	}

	return response, nil
}

// continueWithProgress handles multi-step processing when Claude needs to continue.
func (h *handler) continueWithProgress(
	ctx context.Context,
	initialResponse *claude.LLMResponse,
	msg *signal.IncomingMessage,
	sessionID string,
) (*claude.LLMResponse, error) {
	// Maximum number of continuations
	const maxContinuations = 5

	// Start with the initial response
	currentResponse := initialResponse
	continuationCount := 0

	// Build conversation history
	conversationHistory := []string{
		fmt.Sprintf("User: %s", msg.Text),
		fmt.Sprintf("Assistant: %s", currentResponse.Message),
	}

	// Continue while Claude needs to and we haven't hit the limit
	for currentResponse.Progress != nil &&
		currentResponse.Progress.NeedsContinuation &&
		continuationCount < maxContinuations {
		// Check context cancellation
		select {
		case <-ctx.Done():
			h.logger.WarnContext(ctx, "Continuation canceled by context",
				slog.String("session_id", sessionID),
				slog.Int("continuation", continuationCount),
			)
			return currentResponse, fmt.Errorf("context canceled: %w", ctx.Err())
		default:
		}

		continuationCount++

		// Build continuation prompt
		continuationPrompt := h.buildContinuationPrompt(conversationHistory, currentResponse.Progress)

		h.logger.InfoContext(ctx, "Continuing multi-step processing",
			slog.String("session_id", sessionID),
			slog.Int("continuation", continuationCount),
			slog.String("status", currentResponse.Progress.Status),
		)

		// Query Claude for continuation
		nextResponse, err := h.llm.Query(ctx, continuationPrompt, sessionID)
		if err != nil {
			h.logger.ErrorContext(ctx, "Failed to get continuation",
				slog.String("error", err.Error()),
				slog.Int("continuation", continuationCount),
			)
			// Return what we have so far on error
			return currentResponse, nil
		}

		// Update conversation history
		conversationHistory = append(conversationHistory, fmt.Sprintf("Assistant: %s", nextResponse.Message))

		// Log full continuation response
		h.logger.InfoContext(ctx, "received continuation response",
			slog.String("session_id", sessionID),
			slog.Int("continuation", continuationCount),
			slog.String("message", nextResponse.Message),
			slog.Int("response_length", len(nextResponse.Message)),
			slog.Bool("has_progress", nextResponse.Progress != nil))

		// Log progress if available
		if nextResponse.Progress != nil {
			h.logger.InfoContext(ctx, "continuation progress",
				slog.String("session_id", sessionID),
				slog.Int("continuation", continuationCount),
				slog.Bool("needs_continuation", nextResponse.Progress.NeedsContinuation),
				slog.String("status", nextResponse.Progress.Status),
				slog.String("progress_message", nextResponse.Progress.Message),
				slog.Int("estimated_remaining", nextResponse.Progress.EstimatedRemaining))
		}

		currentResponse = nextResponse
	}

	// Log completion
	if continuationCount > 0 {
		h.logger.InfoContext(ctx, "Multi-step processing completed",
			slog.String("session_id", sessionID),
			slog.Int("total_continuations", continuationCount),
		)
	}

	return currentResponse, nil
}

// buildContinuationPrompt creates a prompt for Claude to continue processing.
func (h *handler) buildContinuationPrompt(
	history []string,
	progress *claude.ProgressInfo,
) string {
	// Build context from conversation history
	context := "Here is our conversation so far:\n\n"
	for _, entry := range history {
		context += entry + "\n\n"
	}

	// Add current progress status
	context += fmt.Sprintf("You indicated you need to continue processing with status: %s\n", progress.Status)

	if progress.Message != "" {
		context += fmt.Sprintf("Progress message: %s\n", progress.Message)
	}

	// Add continuation instruction
	context += "\nPlease continue with the next steps. Remember to include a progress JSON block to indicate if further continuation is needed."

	return context
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

// determineValidationDecision analyzes the progress info and returns validation decision details.
func (h *handler) determineValidationDecision(
	progress *claude.ProgressInfo,
) (string, bool, bool) {
	if progress == nil {
		return validationReasonProgressNil, true, true
	}

	progressNil := false
	needsValidation := progress.NeedsValidation

	var reason string
	if needsValidation {
		reason = validationReasonNeedsValidation
	} else {
		reason = validationReasonSimpleQuery
	}

	return reason, needsValidation, progressNil
}

const (
	// validationTimeout is the maximum time allowed for validation.
	validationTimeout = 30 * time.Second
	// correctionDelay is the delay before sending a correction message.
	correctionDelay = 2 * time.Second
)
