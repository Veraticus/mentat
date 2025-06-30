// Package config provides configuration loading and validation for Mentat.
package config

import (
	"fmt"
	"os"
	"strings"
)

// LoadSystemPrompt loads a system prompt from the specified file path.
// It validates that the file exists and contains a non-empty prompt.
// Returns an error if the file cannot be read or validation fails.
func LoadSystemPrompt(path string) (string, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("system prompt file not found: %s", path)
	}

	// Read the file
	content, err := os.ReadFile(path) // #nosec G304 - Path comes from config and is expected to be dynamic
	if err != nil {
		return "", fmt.Errorf("failed to read system prompt: %w", err)
	}

	prompt := string(content)

	// Validate the prompt
	if err := ValidateSystemPrompt(prompt); err != nil {
		return "", err
	}

	return prompt, nil
}

// ValidateSystemPrompt ensures the system prompt is valid.
// A valid prompt must be non-empty after trimming whitespace.
func ValidateSystemPrompt(prompt string) error {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return fmt.Errorf("system prompt is empty")
	}
	return nil
}