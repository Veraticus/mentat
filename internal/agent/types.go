package agent

// ValidationStatus represents the outcome of validation.
type ValidationStatus string

const (
	// ValidationStatusSuccess indicates the request was fully completed.
	ValidationStatusSuccess ValidationStatus = "SUCCESS"

	// ValidationStatusPartial indicates the request was partially completed.
	ValidationStatusPartial ValidationStatus = "PARTIAL"

	// ValidationStatusFailed indicates the request failed.
	ValidationStatusFailed ValidationStatus = "FAILED"

	// ValidationStatusUnclear indicates the validation couldn't determine status.
	ValidationStatusUnclear ValidationStatus = "UNCLEAR"

	// ValidationStatusIncompleteSearch indicates more thorough search needed.
	ValidationStatusIncompleteSearch ValidationStatus = "INCOMPLETE_SEARCH"
)

// ValidationResult contains the outcome of agent validation.
type ValidationResult struct {
	Metadata    map[string]string // Changed from map[string]any to concrete type
	Status      ValidationStatus
	Issues      []string
	Suggestions []string
	Confidence  float64
}

// Config holds configuration for the agent handler.
type Config struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

	// EnableIntentEnhancement enables the intent enhancement feature.
	EnableIntentEnhancement bool

	// ValidationThreshold is the minimum confidence for success.
	ValidationThreshold float64
}
