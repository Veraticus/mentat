package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Veraticus/mentat/internal/claude"
)

// MultiAgentValidator implements a thorough validation strategy where Claude validates Claude's responses.
// This provides the highest quality validation but requires additional LLM calls.
type MultiAgentValidator struct {
	// validationPromptTemplate is the template for validation queries
	validationPromptTemplate string
}

// NewMultiAgentValidator creates a new multi-agent validation strategy.
func NewMultiAgentValidator() *MultiAgentValidator {
	return &MultiAgentValidator{
		validationPromptTemplate: `Please validate the following response and determine if it adequately addresses the user's request.

User Request: %s

Assistant Response: %s

Analyze the response and provide your assessment in the following format:
STATUS: [SUCCESS|PARTIAL|FAILED|UNCLEAR|INCOMPLETE_SEARCH]
CONFIDENCE: [0.0-1.0]
ISSUES: [comma-separated list of issues, or "none"]
SUGGESTIONS: [comma-separated list of suggestions, or "none"]

Guidelines:
- SUCCESS: The response fully addresses the request with appropriate tool usage
- PARTIAL: The response addresses some but not all aspects of the request
- FAILED: The response does not address the request or contains errors
- UNCLEAR: Cannot determine if the response is adequate
- INCOMPLETE_SEARCH: The assistant should have used more tools (e.g., memory, calendar) but didn't

Be specific about any issues or missing elements.`,
	}
}

// Validate uses Claude to validate another Claude response.
func (v *MultiAgentValidator) Validate(ctx context.Context, request, response string, llm claude.LLM) ValidationResult {
	// Create validation prompt
	prompt := fmt.Sprintf(v.validationPromptTemplate, request, response)

	// Query Claude for validation
	llmResponse, err := llm.Query(ctx, prompt, "")
	if err != nil {
		return ValidationResult{
			Status:     ValidationStatusUnclear,
			Issues:     []string{fmt.Sprintf("validation query failed: %v", err)},
			Confidence: 0.0,
			Metadata:   map[string]string{"error": err.Error()},
		}
	}

	// Parse validation response
	return v.parseValidationResponse(llmResponse.Message)
}

// ShouldRetry determines if we should retry based on the validation result.
func (v *MultiAgentValidator) ShouldRetry(result ValidationResult) bool {
	// Retry for incomplete searches and unclear results with low confidence
	return result.Status == ValidationStatusIncompleteSearch ||
		(result.Status == ValidationStatusUnclear && result.Confidence < 0.3)
}

// GenerateRecovery creates a natural recovery message for validation failures.
func (v *MultiAgentValidator) GenerateRecovery(ctx context.Context, request, response string, result ValidationResult, llm claude.LLM) string {
	// For partial success, acknowledge what worked
	if result.Status == ValidationStatusPartial {
		prompt := fmt.Sprintf(`Generate a brief, natural message acknowledging partial completion of this request.
Request: %s
What was completed: %s
Issues: %s

Respond conversationally, mentioning what was done and what couldn't be completed.`,
			request, response, strings.Join(result.Issues, ", "))

		llmResponse, err := llm.Query(ctx, prompt, "")
		if err != nil {
			return "I was able to help with part of your request, but encountered some limitations."
		}
		return llmResponse.Message
	}

	// For failures, explain the issue naturally
	if result.Status == ValidationStatusFailed {
		issueList := "technical difficulties"
		if len(result.Issues) > 0 {
			issueList = strings.Join(result.Issues, " and ")
		}
		return fmt.Sprintf("I encountered %s with your request. Let me know if you'd like me to try a different approach.", issueList)
	}

	// Default message
	return "I'm having trouble with that request. Could you rephrase it or break it down into smaller steps?"
}

// parseValidationResponse extracts structured data from Claude's validation response.
func (v *MultiAgentValidator) parseValidationResponse(response string) ValidationResult {
	result := ValidationResult{
		Status:      ValidationStatusUnclear,
		Confidence:  0.5,
		Issues:      []string{},
		Suggestions: []string{},
		Metadata:    map[string]string{"raw_response": response},
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split only on the first colon to handle values with colons
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "STATUS":
			result.Status = v.parseStatus(value)
		case "CONFIDENCE":
			confidence, err := v.parseConfidence(value)
			if err == nil {
				result.Confidence = confidence
			}
		case "ISSUES":
			if issues := v.parseIssues(value); len(issues) > 0 {
				result.Issues = issues
			}
		case "SUGGESTIONS":
			if suggestions := v.parseSuggestions(value); len(suggestions) > 0 {
				result.Suggestions = suggestions
			}
		}
	}

	return result
}

// parseStatus converts string status to ValidationStatus.
func (v *MultiAgentValidator) parseStatus(status string) ValidationStatus {
	// Take only the first word to handle cases like "SUCCESS: with extra info"
	status = strings.TrimSpace(status)
	if colonIndex := strings.Index(status, ":"); colonIndex != -1 {
		status = status[:colonIndex]
	}
	// Also handle space-separated extra info
	if spaceIndex := strings.Index(status, " "); spaceIndex != -1 {
		status = status[:spaceIndex]
	}

	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS":
		return ValidationStatusSuccess
	case "PARTIAL":
		return ValidationStatusPartial
	case "FAILED":
		return ValidationStatusFailed
	case "INCOMPLETE_SEARCH":
		return ValidationStatusIncompleteSearch
	default:
		return ValidationStatusUnclear
	}
}

// parseConfidence extracts and validates confidence score.
func (v *MultiAgentValidator) parseConfidence(value string) (float64, error) {
	// Handle values like "0.9: high" by taking only the first part
	value = strings.TrimSpace(value)
	if colonIndex := strings.Index(value, ":"); colonIndex != -1 {
		value = strings.TrimSpace(value[:colonIndex])
	}

	var confidence float64
	n, err := fmt.Sscanf(value, "%f", &confidence)
	if err != nil || n != 1 {
		return 0, fmt.Errorf("invalid confidence format: %s", value)
	}

	// Check if there are trailing characters (like %)
	var trailing string
	if _, err := fmt.Sscanf(value, "%f%s", &confidence, &trailing); err == nil && trailing != "" {
		return 0, fmt.Errorf("invalid confidence format: %s", value)
	}

	// Clamp confidence to valid range [0.0, 1.0]
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}
	return confidence, nil
}

// parseIssues extracts issues from the validation response.
func (v *MultiAgentValidator) parseIssues(value string) []string {
	if strings.ToLower(strings.TrimSpace(value)) == "none" || value == "" {
		return []string{}
	}
	return v.parseList(value)
}

// parseSuggestions extracts suggestions from the validation response.
func (v *MultiAgentValidator) parseSuggestions(value string) []string {
	if strings.ToLower(strings.TrimSpace(value)) == "none" || value == "" {
		return []string{}
	}
	return v.parseList(value)
}

// parseList splits comma-separated values and trims whitespace.
func (v *MultiAgentValidator) parseList(list string) []string {
	parts := strings.Split(list, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// SimpleValidator implements basic validation without LLM calls.
// It checks for common failure patterns and response structure.
type SimpleValidator struct {
	minResponseLength int
	errorKeywords     []string
	successKeywords   []string
}

// NewSimpleValidator creates a new simple validation strategy.
func NewSimpleValidator() *SimpleValidator {
	return &SimpleValidator{
		minResponseLength: 10,
		errorKeywords: []string{
			"error", "failed", "unable", "cannot", "couldn't",
			"sorry", "apologize", "not sure", "don't know",
		},
		successKeywords: []string{
			"completed", "done", "finished", "scheduled",
			"found", "here's", "retrieved", "updated", "sent",
		},
	}
}

// Validate performs basic validation without LLM calls.
func (v *SimpleValidator) Validate(_ context.Context, request, response string, _ claude.LLM) ValidationResult {
	result := ValidationResult{
		Status:      ValidationStatusSuccess,
		Confidence:  0.8,
		Issues:      []string{},
		Suggestions: []string{},
		Metadata:    map[string]string{"validator": "simple"},
	}

	// Check response length
	if len(strings.TrimSpace(response)) < v.minResponseLength {
		result.Status = ValidationStatusFailed
		result.Issues = append(result.Issues, "response too short")
		result.Confidence = 0.9
		return result
	}

	// Check for error indicators
	lowerResponse := strings.ToLower(response)
	errorCount := 0
	for _, keyword := range v.errorKeywords {
		if strings.Contains(lowerResponse, keyword) {
			errorCount++
		}
	}

	// Check for success indicators
	successCount := 0
	for _, keyword := range v.successKeywords {
		if strings.Contains(lowerResponse, keyword) {
			successCount++
		}
	}

	// Determine status based on keyword analysis
	switch {
	case errorCount > successCount && errorCount >= 2:
		result.Status = ValidationStatusFailed
		result.Issues = append(result.Issues, "response contains error indicators")
		result.Confidence = 0.7
	case successCount > 0 && errorCount == 0:
		result.Status = ValidationStatusSuccess
		result.Confidence = 0.8
	case successCount > 0 && errorCount > 0:
		// Both success and error indicators = partial
		result.Status = ValidationStatusPartial
		result.Confidence = 0.6
	case errorCount == 1:
		// Single error indicator = partial
		result.Status = ValidationStatusPartial
		result.Confidence = 0.6
	default:
		result.Status = ValidationStatusUnclear
		result.Confidence = 0.4
	}

	// Check for questions in response (might indicate incomplete processing)
	if strings.Contains(response, "?") && !strings.Contains(request, "?") {
		result.Issues = append(result.Issues, "response contains questions")
		// Downgrade status if questions are present
		switch result.Status {
		case ValidationStatusSuccess:
			result.Status = ValidationStatusPartial
			result.Confidence = 0.64 // 0.8 * 0.8
		default:
			// For all other statuses, reduce confidence
			result.Confidence *= 0.8
		}
	}

	return result
}

// ShouldRetry for simple validator never suggests retries.
func (v *SimpleValidator) ShouldRetry(_ ValidationResult) bool {
	return false
}

// GenerateRecovery provides simple recovery messages without LLM calls.
func (v *SimpleValidator) GenerateRecovery(_ context.Context, _, _ string, result ValidationResult, _ claude.LLM) string {
	switch result.Status {
	case ValidationStatusFailed:
		return "I encountered an issue with that request. Please try again or rephrase your question."
	case ValidationStatusPartial:
		return "I was able to partially complete your request. Let me know if you need anything else."
	case ValidationStatusUnclear:
		return "I'm not certain I fully addressed your request. Could you clarify what you need?"
	default:
		return ""
	}
}

// NoopValidator is a pass-through validator that always returns success.
// Useful for testing and development when validation should be disabled.
type NoopValidator struct{}

// NewNoopValidator creates a new no-op validation strategy.
func NewNoopValidator() *NoopValidator {
	return &NoopValidator{}
}

// Validate always returns success with high confidence.
func (v *NoopValidator) Validate(_ context.Context, _, _ string, _ claude.LLM) ValidationResult {
	return ValidationResult{
		Status:      ValidationStatusSuccess,
		Confidence:  1.0,
		Issues:      []string{},
		Suggestions: []string{},
		Metadata:    map[string]string{"validator": "noop"},
	}
}

// ShouldRetry always returns false.
func (v *NoopValidator) ShouldRetry(_ ValidationResult) bool {
	return false
}

// GenerateRecovery always returns empty string.
func (v *NoopValidator) GenerateRecovery(_ context.Context, _, _ string, _ ValidationResult, _ claude.LLM) string {
	return ""
}
