package testing

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/joshsymonds/mentat/internal/claude"
)

// ScriptedResponse represents a scripted response for the LLM.
type ScriptedResponse struct {
	// Response to return when patterns match
	Response *claude.LLMResponse
	
	// Optional callback to run before returning (for side effects in tests)
	BeforeReturn func(prompt string, sessionID string)
	
	// Error to return instead of response
	Error error
	
	// Pattern to match against the prompt (regex). If empty, matches any prompt.
	PromptPattern string
	
	// Session pattern to match (regex). If empty, matches any session.
	SessionPattern string
	
	// Delay before returning response (simulates processing time)
	Delay time.Duration
	
	// Whether this response can be used multiple times
	Repeatable bool
}

// ScriptedLLM implements the LLM interface with scripted responses for testing.
type ScriptedLLM struct {
	mu               sync.Mutex
	scripts          []ScriptedResponse
	calls            []LLMCall // Track all calls for verification
	fallbackResponse *claude.LLMResponse
	fallbackError    error
	currentIndex     int
	strictMode       bool // If true, fails when no script matches
}

// NewScriptedLLM creates a new ScriptedLLM with optional configuration.
func NewScriptedLLM(opts ...ScriptedLLMOption) *ScriptedLLM {
	s := &ScriptedLLM{
		scripts: make([]ScriptedResponse, 0),
		calls:   make([]LLMCall, 0),
	}
	
	for _, opt := range opts {
		opt(s)
	}
	
	return s
}

// ScriptedLLMOption configures a ScriptedLLM.
type ScriptedLLMOption func(*ScriptedLLM)

// WithStrictMode enables strict mode - fails if no script matches.
func WithStrictMode() ScriptedLLMOption {
	return func(s *ScriptedLLM) {
		s.strictMode = true
	}
}

// WithFallback sets a default response when no script matches.
func WithFallback(response *claude.LLMResponse) ScriptedLLMOption {
	return func(s *ScriptedLLM) {
		s.fallbackResponse = response
	}
}

// WithFallbackError sets a default error when no script matches.
func WithFallbackError(err error) ScriptedLLMOption {
	return func(s *ScriptedLLM) {
		s.fallbackError = err
	}
}

// AddScript adds a scripted response to the sequence.
func (s *ScriptedLLM) AddScript(script ScriptedResponse) *ScriptedLLM {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.scripts = append(s.scripts, script)
	return s
}

// AddSimpleScript adds a simple text response script.
func (s *ScriptedLLM) AddSimpleScript(message string) *ScriptedLLM {
	return s.AddScript(ScriptedResponse{
		Response: &claude.LLMResponse{
			Message: message,
			Metadata: claude.ResponseMetadata{
				ModelVersion: "claude-3.5-sonnet-20241022",
				Latency:      100 * time.Millisecond,
				TokensUsed:   50,
			},
		},
	})
}

// AddErrorScript adds an error response script.
func (s *ScriptedLLM) AddErrorScript(err error) *ScriptedLLM {
	return s.AddScript(ScriptedResponse{
		Error: err,
	})
}

// AddPatternScript adds a pattern-matched response script.
func (s *ScriptedLLM) AddPatternScript(promptPattern string, response *claude.LLMResponse) *ScriptedLLM {
	return s.AddScript(ScriptedResponse{
		PromptPattern: promptPattern,
		Response:      response,
		Repeatable:    true, // Pattern scripts are repeatable by default
	})
}

// AddDelayedScript adds a response with simulated delay.
func (s *ScriptedLLM) AddDelayedScript(response *claude.LLMResponse, delay time.Duration) *ScriptedLLM {
	return s.AddScript(ScriptedResponse{
		Response: response,
		Delay:    delay,
	})
}

// Query implements the LLM interface.
func (s *ScriptedLLM) Query(ctx context.Context, prompt string, sessionID string) (*claude.LLMResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Record the call
	call := LLMCall{
		Prompt:    prompt,
		SessionID: sessionID,
		Timestamp: time.Now(),
	}
	s.calls = append(s.calls, call)
	
	// Find matching script
	for i, script := range s.scripts {
		// Skip already used non-repeatable scripts
		if !script.Repeatable && i < s.currentIndex {
			continue
		}
		
		// Check prompt pattern
		if script.PromptPattern != "" {
			matched, err := regexp.MatchString(script.PromptPattern, prompt)
			if err != nil || !matched {
				continue
			}
		}
		
		// Check session pattern
		if script.SessionPattern != "" {
			matched, err := regexp.MatchString(script.SessionPattern, sessionID)
			if err != nil || !matched {
				continue
			}
		}
		
		// Found a match!
		if !script.Repeatable {
			s.currentIndex = i + 1
		}
		
		// Execute callback if provided
		if script.BeforeReturn != nil {
			script.BeforeReturn(prompt, sessionID)
		}
		
		// Simulate delay
		if script.Delay > 0 {
			select {
			case <-time.After(script.Delay):
				// Delay completed
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		
		// Return error if specified
		if script.Error != nil {
			return nil, script.Error
		}
		
		// Return response
		return script.Response, nil
	}
	
	// No matching script found
	if s.strictMode {
		return nil, fmt.Errorf("no script matches prompt: %q in session: %q", prompt, sessionID)
	}
	
	// Use fallback
	if s.fallbackError != nil {
		return nil, s.fallbackError
	}
	
	if s.fallbackResponse != nil {
		return s.fallbackResponse, nil
	}
	
	// Default fallback
	return &claude.LLMResponse{
		Message: "No script configured for this prompt",
		Metadata: claude.ResponseMetadata{
			ModelVersion: "test",
			Latency:      50 * time.Millisecond,
			TokensUsed:   10,
		},
	}, nil
}

// GetCalls returns all recorded calls.
func (s *ScriptedLLM) GetCalls() []LLMCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Return a copy to prevent external modification
	calls := make([]LLMCall, len(s.calls))
	copy(calls, s.calls)
	return calls
}

// GetCallCount returns the number of calls made.
func (s *ScriptedLLM) GetCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return len(s.calls)
}

// Reset clears all calls and resets the script index.
func (s *ScriptedLLM) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.calls = make([]LLMCall, 0)
	s.currentIndex = 0
}

// ExpectNCalls verifies that exactly n calls were made.
func (s *ScriptedLLM) ExpectNCalls(n int) error {
	count := s.GetCallCount()
	if count != n {
		return fmt.Errorf("expected %d calls, got %d", n, count)
	}
	return nil
}

// ExpectPromptContains verifies that a call contained the given substring.
func (s *ScriptedLLM) ExpectPromptContains(substring string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for _, call := range s.calls {
		if contains(call.Prompt, substring) {
			return nil
		}
	}
	
	return fmt.Errorf("no call contained prompt substring: %q", substring)
}

// contains is a simple string contains helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" || 
		(s != "" && substr != "" && findSubstring(s, substr) >= 0))
}

// findSubstring finds the index of substr in s, or -1 if not found.
func findSubstring(s, substr string) int {
	if substr == "" {
		return 0
	}
	if len(substr) > len(s) {
		return -1
	}
	
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}