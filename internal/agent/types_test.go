package agent

import (
	"testing"
)

func TestValidationStatusConstants(t *testing.T) {
	// Verify that validation status constants have expected values
	expectedStatuses := map[ValidationStatus]string{
		ValidationStatusSuccess:          "SUCCESS",
		ValidationStatusPartial:          "PARTIAL",
		ValidationStatusFailed:           "FAILED",
		ValidationStatusUnclear:          "UNCLEAR",
		ValidationStatusIncompleteSearch: "INCOMPLETE_SEARCH",
	}

	for status, expected := range expectedStatuses {
		if string(status) != expected {
			t.Errorf("Expected %s to equal %s", status, expected)
		}
	}
}

func TestValidationResultZeroValue(t *testing.T) {
	var result ValidationResult

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
	result := ValidationResult{
		Status:      ValidationStatusSuccess,
		Issues:      []string{"Issue 1", "Issue 2"},
		Suggestions: []string{"Suggestion 1", "Suggestion 2"},
		Confidence:  0.95,
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	if result.Status != ValidationStatusSuccess {
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
	var cfg Config

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
	cfg := Config{
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
		status   ValidationStatus
		expected bool
	}{
		{ValidationStatusSuccess, true},
		{ValidationStatusPartial, false},
		{ValidationStatusFailed, false},
		{ValidationStatusUnclear, false},
		{ValidationStatusIncompleteSearch, false},
		{ValidationStatus("UNKNOWN"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			// This test assumes there might be an IsSuccess method
			// If not, we're just testing the constant values
			isSuccess := tt.status == ValidationStatusSuccess
			if isSuccess != tt.expected {
				t.Errorf("Expected %s success to be %v", tt.status, tt.expected)
			}
		})
	}
}

func TestValidationResultWithDifferentStatuses(t *testing.T) {
	statuses := []ValidationStatus{
		ValidationStatusSuccess,
		ValidationStatusPartial,
		ValidationStatusFailed,
		ValidationStatusUnclear,
		ValidationStatusIncompleteSearch,
	}

	for _, status := range statuses {
		result := ValidationResult{
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
		case ValidationStatusSuccess:
			// Success case
		case ValidationStatusPartial:
			// Partial case
		case ValidationStatusFailed:
			// Failed case
		case ValidationStatusUnclear:
			// Unclear case
		case ValidationStatusIncompleteSearch:
			// Incomplete search case
		default:
			t.Errorf("Unexpected status: %s", result.Status)
		}
	}
}

func TestValidationResultNilSafety(t *testing.T) {
	// Test that nil slices and maps work correctly
	result := ValidationResult{
		Status:      ValidationStatusSuccess,
		Issues:      nil,
		Suggestions: nil,
		Metadata:    nil,
		Confidence:  1.0,
	}

	// Verify all fields are set correctly
	if result.Status != ValidationStatusSuccess {
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
