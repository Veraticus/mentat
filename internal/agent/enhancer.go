package agent

import (
	"math"
	"strings"
)

// patternHint represents a pattern to match and the enhancement hint to add.
type patternHint struct {
	keywords []string
	hint     string
}

// SmartIntentEnhancer enhances user requests with helpful context hints
// based on pattern matching. It guides Claude without being prescriptive.
type SmartIntentEnhancer struct {
	patterns []patternHint
}

// NewSmartIntentEnhancer creates a new SmartIntentEnhancer with default patterns.
func NewSmartIntentEnhancer() *SmartIntentEnhancer {
	return &SmartIntentEnhancer{
		patterns: []patternHint{
			// Scheduling patterns
			{
				keywords: []string{"meeting", "appointment", "schedule", "calendar", "book", "available"},
				hint:     "checking calendar availability and existing appointments might be helpful",
			},
			// Memory patterns
			{
				keywords: []string{
					"last time", "discussed", "remember", "previous",
					"mentioned", "told me", "conversation about",
				},
				hint: "searching memory for relevant past interactions could provide context",
			},
			// Email patterns
			{
				keywords: []string{"email", "message", "correspondence", "mail", "inbox", "send email"},
				hint:     "accessing email for context or composing messages might be needed",
			},
			// Task patterns
			{
				keywords: []string{"todo", "task", "reminder", "to do", "to-do", "checklist"},
				hint:     "checking existing tasks or creating new ones could be relevant",
			},
			// Contact patterns
			{
				keywords: []string{"contact", "phone", "address", "reach", "get in touch"},
				hint:     "looking up contact information might be useful",
			},
			// Time-sensitive patterns
			{
				keywords: []string{"today", "tomorrow", "this week", "next week", "urgent", "deadline"},
				hint:     "considering the current date and time context would be important",
			},
			// Expense patterns
			{
				keywords: []string{"expense", "receipt", "reimburse", "spend", "spent", "cost", "price"},
				hint:     "checking expense records or processing receipts might be relevant",
			},
		},
	}
}

// Enhance adds helpful context hints to the original request based on detected patterns.
// The hints are designed to guide without being prescriptive.
func (e *SmartIntentEnhancer) Enhance(originalRequest string) string {
	// Find all matching patterns
	var hints []string
	lowerRequest := strings.ToLower(originalRequest)

	for _, pattern := range e.patterns {
		if matchesPattern(lowerRequest, pattern.keywords) {
			hints = append(hints, pattern.hint)
		}
	}

	// If no patterns matched, return the original request
	if len(hints) == 0 {
		return originalRequest
	}

	// Build the enhanced request with hints
	var builder strings.Builder
	builder.WriteString(originalRequest)
	builder.WriteString("\n\n[Context hints: ")

	// Add hints in a natural way
	if len(hints) == 1 {
		builder.WriteString(hints[0])
	} else {
		for i, hint := range hints {
			if i > 0 {
				builder.WriteString(", and ")
			}
			builder.WriteString(hint)
		}
	}

	builder.WriteString("]")

	return builder.String()
}

// ShouldEnhance determines if a request would benefit from enhancement.
// Returns true if the request matches any known patterns.
func (e *SmartIntentEnhancer) ShouldEnhance(request string) bool {
	lowerRequest := strings.ToLower(request)

	// Check if any pattern matches
	for _, pattern := range e.patterns {
		if matchesPattern(lowerRequest, pattern.keywords) {
			return true
		}
	}

	// Don't enhance very short requests
	const minWordsForEnhancement = 3
	if len(strings.Fields(request)) < minWordsForEnhancement {
		return false
	}

	return false
}

// matchesPattern checks if the request contains any of the keywords.
func matchesPattern(lowerRequest string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(lowerRequest, keyword) {
			return true
		}
	}
	return false
}

// NoopIntentEnhancer is a no-op implementation of IntentEnhancer.
type NoopIntentEnhancer struct{}

// Enhance returns the original request unchanged.
func (n *NoopIntentEnhancer) Enhance(originalRequest string) string {
	return originalRequest
}

// ShouldEnhance always returns false.
func (n *NoopIntentEnhancer) ShouldEnhance(_ string) bool {
	return false
}

// Intent represents a detected user intent with confidence score.
type Intent struct {
	Type       IntentType
	Confidence float64
}

// IntentType represents the type of intent detected.
type IntentType string

const (
	// IntentScheduling represents scheduling-related requests.
	IntentScheduling IntentType = "scheduling"

	// IntentFindingPeople represents requests to find or contact people.
	IntentFindingPeople IntentType = "finding_people"

	// IntentMemory represents requests about past conversations or information.
	IntentMemory IntentType = "memory"

	// IntentUnknown represents requests that don't match known patterns.
	IntentUnknown IntentType = "unknown"
)

// Constants for intent detection configuration.
const (
	phraseWeight           = 2.0
	contextWeight          = 0.5
	minWordsForNormalize   = 5
	normalizeMultiplier    = 10.0
	minConfidenceThreshold = 0.1
	confidenceScaleFactor  = 3.0
	highScoreThreshold     = 2.0
	confidenceBoost        = 0.2
)

// detectSchedulingIntent scores scheduling-related patterns in the request.
func detectSchedulingIntent(lowerRequest string) float64 {
	score := 0.0

	// Scheduling keywords
	schedulingKeywords := []string{
		"schedule", "meeting", "appointment", "calendar", "book", "available",
		"availability", "free time", "busy", "conflict", "reschedule",
		"cancel meeting", "move meeting", "when can", "when is",
		"set up time", "arrange", "plan meeting", "organize meeting",
		"free on", "free at", "sync", "catch up", "discuss", "review",
		"conference", "call", "session", "briefing", "standup",
	}

	// Scheduling phrases (stronger signals)
	schedulingPhrases := []string{
		"am i free", "are you free", "find time", "set up", "pencil in",
		"block time", "reserve time", "schedule time", "book time",
		"meeting room", "conference room", "last meeting", "next meeting",
	}

	for _, keyword := range schedulingKeywords {
		if strings.Contains(lowerRequest, keyword) {
			score += 1.0
		}
	}

	for _, phrase := range schedulingPhrases {
		if strings.Contains(lowerRequest, phrase) {
			score += phraseWeight
		}
	}

	return score
}

// detectFindingPeopleIntent scores people-finding patterns in the request.
func detectFindingPeopleIntent(lowerRequest string) float64 {
	score := 0.0

	// Finding people keywords
	findingPeopleKeywords := []string{
		"who is", "who's", "contact", "phone", "email address", "reach",
		"get in touch", "find", "locate", "number for", "how to contact",
		"where is", "person", "someone", "anybody", "team member",
		"colleague", "coworker", "boss", "manager", "report to",
		"connect with", "speak to", "talk to", "email for", "support",
		"developer", "designer", "engineer", "director", "assistant",
	}

	// Finding people phrases (stronger signals)
	findingPeoplePhrases := []string{
		"reports to", "in touch with", "contact info", "contact details",
		"get me", "put me in touch", "who handles", "who manages",
		"person responsible", "point of contact", "reach out to",
	}

	for _, keyword := range findingPeopleKeywords {
		if strings.Contains(lowerRequest, keyword) {
			score += 1.0
		}
	}

	for _, phrase := range findingPeoplePhrases {
		if strings.Contains(lowerRequest, phrase) {
			score += phraseWeight
		}
	}

	// Context adjustment
	if strings.HasPrefix(lowerRequest, "who") && !strings.Contains(lowerRequest, "meeting") {
		score += contextWeight
	}

	return score
}

// detectMemoryIntent scores memory-related patterns in the request.
func detectMemoryIntent(lowerRequest string) float64 {
	score := 0.0

	// Memory keywords
	memoryKeywords := []string{
		"remember", "last time", "previously", "mentioned", "told me",
		"discussed", "said about", "conversation about", "talked about",
		"earlier", "before", "past", "history", "recall", "forget",
		"did i", "did you", "have i", "have we", "what was",
		"remind me", "agreed on", "decided", "decision", "outcome",
	}

	// Memory phrases (stronger signals)
	memoryPhrases := []string{
		"what did", "what was", "last week", "last month", "last year",
		"remind me what", "tell me what", "said last", "say about",
		"past discussion", "previous meeting", "earlier today",
	}

	for _, keyword := range memoryKeywords {
		if strings.Contains(lowerRequest, keyword) {
			score += 1.0
		}
	}

	for _, phrase := range memoryPhrases {
		if strings.Contains(lowerRequest, phrase) {
			score += phraseWeight
		}
	}

	// Context adjustment
	if strings.Contains(lowerRequest, "last meeting") && strings.Contains(lowerRequest, "decision") {
		score += 1.0
	}

	return score
}

// DetectIntent analyzes a request and returns the primary intent with confidence.
// It focuses on scheduling, finding people, and memory patterns as required.
func (e *SmartIntentEnhancer) DetectIntent(request string) Intent {
	lowerRequest := strings.ToLower(request)

	// Score each intent type
	schedulingScore := detectSchedulingIntent(lowerRequest)
	findingPeopleScore := detectFindingPeopleIntent(lowerRequest)
	memoryScore := detectMemoryIntent(lowerRequest)

	// Normalize scores for longer requests
	wordCount := float64(len(strings.Fields(request)))
	if wordCount > minWordsForNormalize {
		schedulingScore = schedulingScore / wordCount * normalizeMultiplier
		findingPeopleScore = findingPeopleScore / wordCount * normalizeMultiplier
		memoryScore = memoryScore / wordCount * normalizeMultiplier
	}

	// Determine primary intent
	maxScore := schedulingScore
	primaryIntent := IntentScheduling

	if findingPeopleScore > maxScore {
		maxScore = findingPeopleScore
		primaryIntent = IntentFindingPeople
	}

	if memoryScore > maxScore {
		maxScore = memoryScore
		primaryIntent = IntentMemory
	}

	// Check if intent is significant
	if maxScore < minConfidenceThreshold {
		return Intent{
			Type:       IntentUnknown,
			Confidence: 0.0,
		}
	}

	// Calculate confidence
	confidence := math.Min(maxScore/confidenceScaleFactor, 1.0)

	// Boost confidence for very clear signals
	if maxScore > highScoreThreshold {
		confidence = math.Min(confidence+confidenceBoost, 1.0)
	}

	return Intent{
		Type:       primaryIntent,
		Confidence: confidence,
	}
}

// Ensure implementations satisfy the interface.
var (
	_ IntentEnhancer = (*SmartIntentEnhancer)(nil)
	_ IntentEnhancer = (*NoopIntentEnhancer)(nil)
)
