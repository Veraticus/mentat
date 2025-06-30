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
	var jsonResp jsonResponse
	if err := json.Unmarshal([]byte(output), &jsonResp); err == nil {
		// Successfully parsed JSON
		// Check for message in either Message or Result field (Claude Code uses Result)
		if jsonResp.Message == "" && jsonResp.Result == "" {
			return nil, fmt.Errorf("claude response missing message/result field")
		}
		return &jsonResp, nil
	}
	
	// If JSON parsing fails, try to extract useful information from the output
	// This handles cases where Claude CLI might return errors or non-JSON output
	
	// Check for common error patterns
	if strings.Contains(output, "error:") || strings.Contains(output, "Error:") {
		// Extract error message
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
			return nil, fmt.Errorf("%s", strings.TrimSpace(errorMsg.String()))
		}
	}
	
	// If we have non-empty output that's not JSON, treat it as a plain text response
	if output != "" {
		// Build a response with the raw output as the message
		var msgBuilder strings.Builder
		
		// Process multiline output
		lines := strings.Split(output, "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				if i > 0 {
					msgBuilder.WriteString(" ")
				}
				msgBuilder.WriteString(line)
			}
		}
		
		message := msgBuilder.String()
		if message != "" {
			// Create a minimal response with just the message
			return &jsonResponse{
				Message: message,
				Metadata: jsonResponseMetadata{
					Model: "unknown", // We don't know the model from plain text
				},
			}, nil
		}
	}
	
	// If we get here, we have empty or unusable output
	return nil, fmt.Errorf("claude returned empty or unparseable response")
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