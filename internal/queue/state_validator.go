package queue

import (
	"fmt"
)

// TransitionRule defines a validation rule for state transitions.
type TransitionRule struct {
	From        State
	To          State
	Description string
	Validate    func(msg *Message) error
}

// stateValidator provides enhanced validation for state transitions.
type stateValidator struct {
	rules []TransitionRule
}

// newStateValidator creates a validator with comprehensive transition rules.
func newStateValidator() *stateValidator {
	return &stateValidator{
		rules: []TransitionRule{
			// Queued -> Processing
			{
				From:        StateQueued,
				To:          StateProcessing,
				Description: "Message must have valid ID and text",
				Validate: func(msg *Message) error {
					if msg.ID == "" {
						return fmt.Errorf("cannot process message without ID")
					}
					if msg.Text == "" {
						return fmt.Errorf("cannot process empty message")
					}
					return nil
				},
			},
			// Processing -> Validating
			{
				From:        StateProcessing,
				To:          StateValidating,
				Description: "Message must have a response to validate",
				Validate: func(msg *Message) error {
					if msg.Response == "" {
						return fmt.Errorf("cannot validate without response: message has not been processed")
					}
					return nil
				},
			},
			// Processing -> Failed
			{
				From:        StateProcessing,
				To:          StateFailed,
				Description: "Failure must have an error reason",
				Validate: func(msg *Message) error {
					// If we can't retry (at max attempts), we can fail without explicit error
					// If we can retry but have no error, that's invalid
					if msg.Error == nil && msg.CanRetry() {
						return fmt.Errorf("cannot fail without error when retries remain")
					}
					return nil
				},
			},
			// Processing -> Retrying
			{
				From:        StateProcessing,
				To:          StateRetrying,
				Description: "Can only retry if attempts remain",
				Validate: func(msg *Message) error {
					if !msg.CanRetry() {
						return fmt.Errorf("cannot retry: maximum attempts (%d) exceeded", msg.MaxAttempts)
					}
					return nil
				},
			},
			// Validating -> Completed
			{
				From:        StateValidating,
				To:          StateCompleted,
				Description: "Must have validated response",
				Validate: func(msg *Message) error {
					if msg.Response == "" {
						return fmt.Errorf("cannot complete without validated response")
					}
					if msg.ProcessedAt == nil {
						return fmt.Errorf("cannot complete without processing timestamp")
					}
					return nil
				},
			},
			// Validating -> Failed
			{
				From:        StateValidating,
				To:          StateFailed,
				Description: "Validation failure must have reason",
				Validate: func(msg *Message) error {
					// Same logic as Processing -> Failed
					if msg.Error == nil && msg.CanRetry() {
						return fmt.Errorf("cannot fail validation without error when retries remain")
					}
					return nil
				},
			},
			// Validating -> Retrying
			{
				From:        StateValidating,
				To:          StateRetrying,
				Description: "Can only retry validation if attempts remain",
				Validate: func(msg *Message) error {
					if !msg.CanRetry() {
						return fmt.Errorf("cannot retry validation: maximum attempts (%d) exceeded", msg.MaxAttempts)
					}
					return nil
				},
			},
			// Retrying -> Processing
			{
				From:        StateRetrying,
				To:          StateProcessing,
				Description: "Retry must not exceed maximum attempts",
				Validate: func(msg *Message) error {
					if msg.Attempts >= msg.MaxAttempts {
						return fmt.Errorf("cannot retry: already attempted %d times (max: %d)", msg.Attempts, msg.MaxAttempts)
					}
					return nil
				},
			},
		},
	}
}

// validateTransition checks if a transition is valid according to business rules.
func (v *stateValidator) validateTransition(msg *Message, from, to State) error {
	// Find applicable rule
	for _, rule := range v.rules {
		if rule.From == from && rule.To == to {
			if err := rule.Validate(msg); err != nil {
				return fmt.Errorf("transition %s→%s invalid: %w", from, to, err)
			}
			return nil
		}
	}
	
	// No specific rule means transition is allowed if structurally valid
	return nil
}

// explainInvalidTransition provides detailed explanation for why a transition is not allowed.
func (v *stateValidator) explainInvalidTransition(from, to State) string {
	// Terminal states
	if from == StateCompleted {
		return fmt.Sprintf("transition from %s is not allowed: message processing is already complete", from)
	}
	if from == StateFailed {
		return fmt.Sprintf("transition from %s is not allowed: message has permanently failed and cannot be reprocessed", from)
	}
	
	// Explain based on target state requirements
	switch to {
	case StateQueued:
		return fmt.Sprintf("cannot transition to %s: messages cannot be re-queued once processing has started", to)
	case StateProcessing:
		if from == StateValidating {
			return fmt.Sprintf("cannot transition from %s to %s: validation must complete before reprocessing", from, to)
		}
		return fmt.Sprintf("transition from %s to %s is not allowed: messages can only enter processing from queued or retrying states", from, to)
	case StateValidating:
		return fmt.Sprintf("cannot transition to %s from %s: validation can only follow successful processing", to, from)
	case StateCompleted:
		return fmt.Sprintf("cannot transition to %s from %s: messages can only be completed after successful validation", to, from)
	case StateFailed:
		if from == StateQueued {
			return fmt.Sprintf("cannot transition from %s to %s: messages must be processed before they can fail", from, to)
		}
		return fmt.Sprintf("transition from %s to %s requires an error condition", from, to)
	case StateRetrying:
		if from == StateQueued {
			return fmt.Sprintf("cannot transition from %s to %s: only failed processing or validation can be retried", from, to)
		}
		return fmt.Sprintf("transition from %s to %s is only allowed after processing or validation failures", from, to)
	default:
		return fmt.Sprintf("transition from %s to %s is not defined in the state machine", from, to)
	}
}