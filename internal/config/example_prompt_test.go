package config_test

import (
	"fmt"
	"log"
	"os"

	"github.com/Veraticus/mentat/internal/claude"
	"github.com/Veraticus/mentat/internal/config"
)

func ExampleLoadSystemPrompt() {
	// Create a temporary prompt file for the example
	tmpFile, err := os.CreateTemp("", "system_prompt_*.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	// Write a system prompt to the file
	promptContent := "You are Mentat, a helpful personal assistant specialized in scheduling and task management."
	if _, writeErr := tmpFile.WriteString(promptContent); writeErr != nil {
		_ = tmpFile.Close()
		log.Fatal(writeErr)
	}
	if err := tmpFile.Close(); err != nil {
		log.Fatal(err)
	}

	// Load the system prompt
	prompt, err := config.LoadSystemPrompt(tmpFile.Name())
	if err != nil {
		log.Fatalf("Failed to load system prompt: %v", err)
	}

	fmt.Println("Loaded system prompt successfully")
	fmt.Printf("First 50 chars: %.50s...\n", prompt)

	// Output:
	// Loaded system prompt successfully
	// First 50 chars: You are Mentat, a helpful personal assistant speci...
}

func ExampleLoadSystemPrompt_withClaude() {
	// Create a temporary prompt file
	tmpFile, err := os.CreateTemp("", "mentat_prompt_*.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	// Write the system prompt
	if _, writeErr := tmpFile.WriteString("You are a scheduling assistant."); writeErr != nil {
		_ = tmpFile.Close()
		log.Fatal(writeErr)
	}
	if err := tmpFile.Close(); err != nil {
		log.Fatal(err)
	}

	// Load the system prompt
	systemPrompt, err := config.LoadSystemPrompt(tmpFile.Name())
	if err != nil {
		log.Fatalf("Failed to load system prompt: %v", err)
	}

	// Use with Claude client
	claudeConfig := claude.Config{
		SystemPrompt: systemPrompt, // Use the loaded system prompt
	}

	fmt.Printf("Claude config with %d char system prompt\n", len(claudeConfig.SystemPrompt))

	// Output:
	// Claude config with 31 char system prompt
}