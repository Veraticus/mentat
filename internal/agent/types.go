package agent

import "time"

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

	// AsyncValidatorDelay is the delay before async validation starts.
	// This is primarily used for testing to avoid timing issues.
	AsyncValidatorDelay time.Duration

	// CorrectionDelay is the delay before sending a correction message.
	// Defaults to 2 seconds if not specified.
	CorrectionDelay time.Duration

	// ValidationTimeout is the maximum time allowed for validation.
	// Defaults to 30 seconds if not specified.
	ValidationTimeout time.Duration
}

// ComplexityResult contains the analysis of request complexity.
type ComplexityResult struct {
	// Score represents the overall complexity (0.0 = simple, 1.0 = very complex)
	Score float64

	// Steps represents the estimated number of steps required
	Steps int

	// Factors lists the contributing complexity factors
	Factors []ComplexityFactor

	// Suggestions provides guidance for handling the complexity
	Suggestions []string

	// RequiresDecomposition indicates if the request should be broken down
	RequiresDecomposition bool
}

// ComplexityFactor represents a factor contributing to request complexity.
type ComplexityFactor struct {
	// Type identifies the type of complexity factor
	Type ComplexityFactorType

	// Description provides detail about this factor
	Description string

	// Weight indicates how much this factor contributes to overall complexity
	Weight float64
}

// ComplexityFactorType identifies different types of complexity factors.
type ComplexityFactorType string

const (
	// ComplexityFactorMultiStep indicates multiple sequential actions needed.
	ComplexityFactorMultiStep ComplexityFactorType = "multi_step"

	// ComplexityFactorAmbiguous indicates unclear or vague requirements.
	ComplexityFactorAmbiguous ComplexityFactorType = "ambiguous"

	// ComplexityFactorTemporal indicates time-based complexity.
	ComplexityFactorTemporal ComplexityFactorType = "temporal"

	// ComplexityFactorConditional indicates conditional logic required.
	ComplexityFactorConditional ComplexityFactorType = "conditional"

	// ComplexityFactorDataIntegration indicates multiple data sources needed.
	ComplexityFactorDataIntegration ComplexityFactorType = "data_integration"

	// ComplexityFactorCoordination indicates coordination between multiple entities.
	ComplexityFactorCoordination ComplexityFactorType = "coordination"
)
