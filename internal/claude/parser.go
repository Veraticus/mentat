package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseResponse attempts to parse Claude's response from various output formats.
// It handles both JSON and plain text responses, ensuring we never return empty messages.
func parseResponse(output string) (*jsonResponse, error) {
	// Trim whitespace
	output = strings.TrimSpace(output)

	// First try to parse as JSON
	resp, jsonErr := tryParseJSON(output)
	if jsonErr == nil {
		return resp, nil
	}

	// If it looks like JSON but failed to parse, return the error
	if strings.HasPrefix(output, "{") {
		return nil, fmt.Errorf("failed to parse JSON response: %w", jsonErr)
	}

	// If JSON parsing fails, try to extract error message
	if err := tryExtractError(output); err != nil {
		return nil, err
	}

	// If we have non-empty output that's not JSON, treat it as plain text
	if plainResp := tryParsePlainText(output); plainResp != nil {
		return plainResp, nil
	}

	// If we get here, we have empty or unusable output
	return nil, fmt.Errorf("parse failed: claude returned empty or unparseable response")
}

// tryParseJSON attempts to parse the output as JSON.
func tryParseJSON(output string) (*jsonResponse, error) {
	// Check if the JSON is wrapped in markdown code blocks
	if strings.HasPrefix(output, "```json") && strings.HasSuffix(output, "```") {
		// Extract JSON from markdown code block
		jsonStart := strings.Index(output, "\n") + 1
		jsonEnd := strings.LastIndex(output, "\n```")
		if jsonStart > 0 && jsonEnd > jsonStart {
			output = output[jsonStart:jsonEnd]
		}
	} else if strings.HasPrefix(output, "```") && strings.HasSuffix(output, "```") {
		// Handle case without json language tag
		jsonStart := strings.Index(output, "\n") + 1
		jsonEnd := strings.LastIndex(output, "\n```")
		if jsonStart > 0 && jsonEnd > jsonStart {
			output = output[jsonStart:jsonEnd]
		}
	}

	var jsonResp jsonResponse
	if err := json.Unmarshal([]byte(output), &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON response: %w", err)
	}

	// Check if the message field itself contains JSON (nested JSON from Claude)
	if jsonResp.Message != "" && strings.HasPrefix(strings.TrimSpace(jsonResp.Message), "{") {
		// Try to parse the message as our expected JSON format
		var innerResp struct {
			Message  string            `json:"message"`
			Progress *jsonProgressInfo `json:"progress,omitempty"`
		}
		if err := json.Unmarshal([]byte(jsonResp.Message), &innerResp); err == nil {
			// Successfully parsed inner JSON - use it
			jsonResp.Message = innerResp.Message
			jsonResp.Progress = innerResp.Progress
		}
	}

	return &jsonResp, nil
}

// tryExtractError checks for error patterns and extracts error messages.
func tryExtractError(output string) error {
	if !strings.Contains(output, "error:") && !strings.Contains(output, "Error:") {
		return nil
	}

	errorMsg := extractClaudeErrorMessage(output)
	if errorMsg != "" {
		return fmt.Errorf("%s", errorMsg)
	}
	return nil
}

// extractClaudeErrorMessage extracts error message from output for internal use.
func extractClaudeErrorMessage(output string) string {
	lines := strings.Split(output, "\n")
	var errorMsg strings.Builder
	errorMsg.WriteString("claude CLI error: ")

	foundError := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "error:") {
			errorMsg.WriteString(strings.TrimPrefix(line, "error:"))
			errorMsg.WriteString(" ")
			foundError = true
		} else if foundError && line != "" {
			// Include subsequent lines as part of error context
			errorMsg.WriteString(line)
			errorMsg.WriteString(" ")
		}
	}

	if foundError {
		return strings.TrimSpace(errorMsg.String())
	}
	return ""
}

// tryParsePlainText attempts to parse output as plain text.
func tryParsePlainText(output string) *jsonResponse {
	if output == "" {
		return nil
	}

	message := buildPlainTextMessage(output)
	if message != "" {
		return &jsonResponse{
			Message: message,
			Metadata: jsonResponseMetadata{
				Model: "unknown", // We don't know the model from plain text
			},
		}
	}
	return nil
}

// buildPlainTextMessage builds a message from multiline plain text.
func buildPlainTextMessage(output string) string {
	var msgBuilder strings.Builder
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			if i > 0 && msgBuilder.Len() > 0 {
				msgBuilder.WriteString(" ")
			}
			msgBuilder.WriteString(line)
		}
	}

	return msgBuilder.String()
}

// extractErrorMessage attempts to extract a user-friendly error message from command output.
// This is used when the command execution fails.
func extractErrorMessage(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "command produced no output"
	}

	var msgBuilder strings.Builder
	lines := strings.Split(output, "\n")

	// Look for specific error patterns
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Common error indicators
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "error") ||
			strings.Contains(lowerLine, "failed") ||
			strings.Contains(lowerLine, "invalid") ||
			strings.Contains(lowerLine, "not found") ||
			strings.Contains(lowerLine, "permission denied") ||
			strings.Contains(lowerLine, "timeout") ||
			strings.Contains(lowerLine, "timed out") {
			if msgBuilder.Len() > 0 {
				msgBuilder.WriteString("; ")
			}
			msgBuilder.WriteString(line)
		}
	}

	// If we found specific errors, return them
	if msgBuilder.Len() > 0 {
		return msgBuilder.String()
	}

	// Otherwise, return the first non-empty line
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}

	return "command failed with unspecified error"
}
