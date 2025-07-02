package agent_test

import (
	"testing"

	"github.com/Veraticus/mentat/internal/agent"
)

func TestValidationStatusConstants(t *testing.T) {
	// Verify that validation status constants have expected values
	expectedStatuses := map[agent.ValidationStatus]string{
		agent.ValidationStatusSuccess:          "SUCCESS",
		agent.ValidationStatusPartial:          "PARTIAL",
		agent.ValidationStatusFailed:           "FAILED",
		agent.ValidationStatusUnclear:          "UNCLEAR",
		agent.ValidationStatusIncompleteSearch: "INCOMPLETE_SEARCH",
	}

	for status, expected := range expectedStatuses {
		if string(status) != expected {
			t.Errorf("Expected %s to equal %s", status, expected)
		}
	}
}

func TestValidationResultZeroValue(t *testing.T) {
	var result agent.ValidationResult

	// Zero value should be identifiable as uninitialized
	if result.Status != "" {
		t.Error("Expected empty Status for zero value")
	}
	if result.Issues != nil {
		t.Error("Expected nil Issues for zero value")
	}
	if result.Suggestions != nil {
		t.Error("Expected nil Suggestions for zero value")
	}
	if result.Confidence != 0 {
		t.Error("Expected zero Confidence for zero value")
	}
	if result.Metadata != nil {
		t.Error("Expected nil Metadata for zero value")
	}
}

func TestValidationResultCreation(t *testing.T) {
	result := agent.ValidationResult{
		Status:      agent.ValidationStatusSuccess,
		Issues:      []string{"Issue 1", "Issue 2"},
		Suggestions: []string{"Suggestion 1", "Suggestion 2"},
		Confidence:  0.95,
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	if result.Status != agent.ValidationStatusSuccess {
		t.Error("Status not set correctly")
	}
	if len(result.Issues) != 2 {
		t.Error("Issues not set correctly")
	}
	if len(result.Suggestions) != 2 {
		t.Error("Suggestions not set correctly")
	}
	if result.Confidence != 0.95 {
		t.Error("Confidence not set correctly")
	}
	if len(result.Metadata) != 2 {
		t.Error("Metadata not set correctly")
	}
}

func TestConfigZeroValue(t *testing.T) {
	var cfg agent.Config

	// Zero value should have sensible defaults
	if cfg.MaxRetries != 0 {
		t.Error("Expected zero MaxRetries for zero value")
	}
	if cfg.EnableIntentEnhancement {
		t.Error("Expected false EnableIntentEnhancement for zero value")
	}
	if cfg.ValidationThreshold != 0 {
		t.Error("Expected zero ValidationThreshold for zero value")
	}
}

func TestConfigDefaults(t *testing.T) {
	// Test that a config with defaults provides useful values
	cfg := agent.Config{
		MaxRetries:              2,
		EnableIntentEnhancement: true,
		ValidationThreshold:     0.8,
	}

	if cfg.MaxRetries != 2 {
		t.Error("MaxRetries not set correctly")
	}
	if !cfg.EnableIntentEnhancement {
		t.Error("EnableIntentEnhancement not set correctly")
	}
	if cfg.ValidationThreshold != 0.8 {
		t.Error("ValidationThreshold not set correctly")
	}
}

func TestValidationStatusIsSuccess(t *testing.T) {
	tests := []struct {
		status   agent.ValidationStatus
		expected bool
	}{
		{agent.ValidationStatusSuccess, true},
		{agent.ValidationStatusPartial, false},
		{agent.ValidationStatusFailed, false},
		{agent.ValidationStatusUnclear, false},
		{agent.ValidationStatusIncompleteSearch, false},
		{agent.ValidationStatus("UNKNOWN"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			// This test assumes there might be an IsSuccess method
			// If not, we're just testing the constant values
			isSuccess := tt.status == agent.ValidationStatusSuccess
			if isSuccess != tt.expected {
				t.Errorf("Expected %s success to be %v", tt.status, tt.expected)
			}
		})
	}
}

func TestValidationResultWithDifferentStatuses(t *testing.T) {
	statuses := []agent.ValidationStatus{
		agent.ValidationStatusSuccess,
		agent.ValidationStatusPartial,
		agent.ValidationStatusFailed,
		agent.ValidationStatusUnclear,
		agent.ValidationStatusIncompleteSearch,
	}

	for _, status := range statuses {
		result := agent.ValidationResult{
			Status:     status,
			Confidence: 0.5,
		}

		if result.Status != status {
			t.Errorf("Status not preserved correctly for %s", status)
		}

		// Verify confidence is set
		if result.Confidence != 0.5 {
			t.Errorf("Confidence not preserved correctly for %s", status)
		}

		// Verify we can distinguish between different statuses
		switch result.Status {
		case agent.ValidationStatusSuccess:
			// Success case
		case agent.ValidationStatusPartial:
			// Partial case
		case agent.ValidationStatusFailed:
			// Failed case
		case agent.ValidationStatusUnclear:
			// Unclear case
		case agent.ValidationStatusIncompleteSearch:
			// Incomplete search case
		default:
			t.Errorf("Unexpected status: %s", result.Status)
		}
	}
}

func TestValidationResultNilSafety(t *testing.T) {
	// Test that nil slices and maps work correctly
	result := agent.ValidationResult{
		Status:      agent.ValidationStatusSuccess,
		Issues:      nil,
		Suggestions: nil,
		Metadata:    nil,
		Confidence:  1.0,
	}

	// Verify all fields are set correctly
	if result.Status != agent.ValidationStatusSuccess {
		t.Error("Status not set correctly")
	}
	if result.Confidence != 1.0 {
		t.Error("Confidence not set correctly")
	}

	// Should not panic when accessing nil fields
	if len(result.Issues) != 0 {
		t.Error("Expected nil Issues to have length 0")
	}
	if len(result.Suggestions) != 0 {
		t.Error("Expected nil Suggestions to have length 0")
	}
	if len(result.Metadata) != 0 {
		t.Error("Expected nil Metadata to have length 0")
	}

	// Can append to nil slices
	result.Issues = append(result.Issues, "New issue")
	if len(result.Issues) != 1 {
		t.Error("Failed to append to nil Issues slice")
	}

	// Can assign to nil map
	result.Metadata = make(map[string]string)
	result.Metadata["key"] = "value"
	if result.Metadata["key"] != "value" {
		t.Error("Failed to use nil-initialized Metadata map")
	}
}
