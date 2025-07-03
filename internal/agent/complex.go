package agent

import (
	"math"
	"strings"
)

// Configuration constants for complexity analysis.
const (
	defaultComplexityThreshold    = 0.5
	defaultDecompositionThreshold = 0.7
	verbCountThreshold            = 2
	verbWeightMultiplier          = 0.2
	timeCountMultiplier           = 0.2
	dataSourceMultiplier          = 0.3
	personCountThreshold          = 2
	diminishingReturnsFactor      = 0.9
	factorWeightThreshold         = 0.3
	maxEstimatedSteps             = 10
	highFactorWeight              = 0.5
	mediumFactorWeight            = 0.3
)

// RequestComplexityAnalyzer analyzes requests to identify multi-step operations
// and other complexity factors that may require special handling.
type RequestComplexityAnalyzer struct {
	// Thresholds for complexity scoring
	complexityThreshold    float64
	decompositionThreshold float64
}

// NewRequestComplexityAnalyzer creates a new complexity analyzer with default thresholds.
func NewRequestComplexityAnalyzer() *RequestComplexityAnalyzer {
	return &RequestComplexityAnalyzer{
		complexityThreshold:    defaultComplexityThreshold,
		decompositionThreshold: defaultDecompositionThreshold,
	}
}

// Analyze examines a request and returns detailed complexity information.
func (r *RequestComplexityAnalyzer) Analyze(request string) ComplexityResult {
	lowerRequest := strings.ToLower(request)

	// Collect all complexity factors
	var factors []ComplexityFactor

	// Analyze different aspects of complexity
	multiStepFactor := r.analyzeMultiStep(lowerRequest)
	if multiStepFactor.Weight > 0 {
		factors = append(factors, multiStepFactor)
	}

	ambiguityFactor := r.analyzeAmbiguity(lowerRequest)
	if ambiguityFactor.Weight > 0 {
		factors = append(factors, ambiguityFactor)
	}

	temporalFactor := r.analyzeTemporal(lowerRequest)
	if temporalFactor.Weight > 0 {
		factors = append(factors, temporalFactor)
	}

	conditionalFactor := r.analyzeConditional(lowerRequest)
	if conditionalFactor.Weight > 0 {
		factors = append(factors, conditionalFactor)
	}

	dataIntegrationFactor := r.analyzeDataIntegration(lowerRequest)
	if dataIntegrationFactor.Weight > 0 {
		factors = append(factors, dataIntegrationFactor)
	}

	coordinationFactor := r.analyzeCoordination(lowerRequest)
	if coordinationFactor.Weight > 0 {
		factors = append(factors, coordinationFactor)
	}

	// Calculate overall complexity score
	score := r.calculateScore(factors)
	steps := r.estimateSteps(factors, lowerRequest)

	// Generate suggestions based on complexity
	suggestions := r.generateSuggestions(factors, score)

	// Determine if decomposition is needed
	requiresDecomposition := score >= r.decompositionThreshold

	return ComplexityResult{
		Score:                 score,
		Steps:                 steps,
		Factors:               factors,
		Suggestions:           suggestions,
		RequiresDecomposition: requiresDecomposition,
	}
}

// IsComplex determines if a request requires multi-step processing.
func (r *RequestComplexityAnalyzer) IsComplex(request string) bool {
	result := r.Analyze(request)
	return result.Score >= r.complexityThreshold
}

// analyzeMultiStep detects requests that involve multiple sequential actions.
func (r *RequestComplexityAnalyzer) analyzeMultiStep(lowerRequest string) ComplexityFactor {
	weight := 0.0
	descriptions := []string{}

	// Multi-step connectors
	connectors := []struct {
		pattern string
		weight  float64
		desc    string
	}{
		{"and then", 0.3, "sequential actions"},
		{"after that", 0.3, "sequential actions"},
		{"first", 0.2, "ordered steps"},
		{"next", 0.2, "ordered steps"},
		{"finally", 0.2, "ordered steps"},
		{"followed by", 0.3, "sequential actions"},
		{"once", 0.2, "conditional sequence"},
		{"when", 0.15, "conditional sequence"},
		{"before", 0.2, "ordering constraint"},
		{"after", 0.2, "ordering constraint"},
	}

	// Check for numbered lists
	if strings.Contains(lowerRequest, "1.") || strings.Contains(lowerRequest, "1)") {
		weight += 0.4
		descriptions = append(descriptions, "numbered list detected")
	}

	// Check for multiple verbs indicating different actions
	actionVerbs := []string{
		"schedule", "find", "email", "remind", "check", "book",
		"cancel", "move", "send", "create", "update", "delete",
	}
	verbCount := 0
	for _, verb := range actionVerbs {
		if strings.Contains(lowerRequest, verb) {
			verbCount++
		}
	}
	if verbCount > verbCountThreshold {
		weight += float64(verbCount-verbCountThreshold) * verbWeightMultiplier
		descriptions = append(descriptions, "multiple actions detected")
	}

	// Check for connectors
	for _, connector := range connectors {
		if strings.Contains(lowerRequest, connector.pattern) {
			weight += connector.weight
			descriptions = append(descriptions, connector.desc)
		}
	}

	// Cap weight at 1.0
	if weight > 1.0 {
		weight = 1.0
	}

	desc := "Request involves multiple steps"
	if len(descriptions) > 0 {
		desc += ": " + strings.Join(descriptions, ", ")
	}

	return ComplexityFactor{
		Type:        ComplexityFactorMultiStep,
		Description: desc,
		Weight:      weight,
	}
}

// analyzeAmbiguity detects vague or unclear requirements.
func (r *RequestComplexityAnalyzer) analyzeAmbiguity(lowerRequest string) ComplexityFactor {
	weight := 0.0
	indicators := []string{}

	// Vague terms
	vagueTerms := []struct {
		term   string
		weight float64
	}{
		{"something", 0.3},
		{"sometime", 0.3},
		{"maybe", 0.2},
		{"probably", 0.2},
		{"might", 0.2},
		{"could", 0.15},
		{"should", 0.15},
		{"any", 0.1},
		{"whatever", 0.3},
		{"stuff", 0.3},
		{"things", 0.2},
		{"etc", 0.2},
		{"and so on", 0.2},
		{"or something", 0.3},
		{"kind of", 0.2},
		{"sort of", 0.2},
	}

	for _, vague := range vagueTerms {
		if strings.Contains(lowerRequest, vague.term) {
			weight += vague.weight
			indicators = append(indicators, vague.term)
		}
	}

	// Check for questions within the request
	if strings.Count(lowerRequest, "?") > 1 {
		weight += 0.2
		indicators = append(indicators, "multiple questions")
	}

	// Check for alternatives
	if strings.Contains(lowerRequest, " or ") {
		weight += 0.15
		indicators = append(indicators, "alternatives present")
	}

	// Cap weight at 1.0
	if weight > 1.0 {
		weight = 1.0
	}

	desc := "Request contains ambiguous elements"
	if len(indicators) > 0 {
		desc += ": " + strings.Join(indicators, ", ")
	}

	return ComplexityFactor{
		Type:        ComplexityFactorAmbiguous,
		Description: desc,
		Weight:      weight,
	}
}

// analyzeTemporal detects time-based complexity.
func (r *RequestComplexityAnalyzer) analyzeTemporal(lowerRequest string) ComplexityFactor {
	weight := 0.0
	timeFactors := []string{}

	// Time ranges
	hasTimeBetween := strings.Contains(lowerRequest, "between")
	hasTimeDelimiter := strings.Contains(lowerRequest, "and") ||
		strings.Contains(lowerRequest, "to")
	if hasTimeBetween && hasTimeDelimiter {
		weight += 0.3
		timeFactors = append(timeFactors, "time range")
	}

	// Recurring patterns
	recurringTerms := []string{"every", "daily", "weekly", "monthly", "recurring", "repeat"}
	for _, term := range recurringTerms {
		if strings.Contains(lowerRequest, term) {
			weight += 0.3
			timeFactors = append(timeFactors, "recurring event")
			break
		}
	}

	// Multiple time references
	timeTerms := []string{
		"today", "tomorrow", "yesterday", "morning", "afternoon", "evening",
		"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday",
	}
	timeCount := 0
	for _, term := range timeTerms {
		if strings.Contains(lowerRequest, term) {
			timeCount++
		}
	}
	if timeCount > 1 {
		weight += float64(timeCount-1) * timeCountMultiplier
		timeFactors = append(timeFactors, "multiple time references")
	}

	// Duration calculations
	if strings.Contains(lowerRequest, "how long") || strings.Contains(lowerRequest, "duration") {
		weight += 0.2
		timeFactors = append(timeFactors, "duration calculation")
	}

	// Cap weight at 1.0
	if weight > 1.0 {
		weight = 1.0
	}

	desc := "Request involves temporal complexity"
	if len(timeFactors) > 0 {
		desc += ": " + strings.Join(timeFactors, ", ")
	}

	return ComplexityFactor{
		Type:        ComplexityFactorTemporal,
		Description: desc,
		Weight:      weight,
	}
}

// analyzeConditional detects conditional logic requirements.
func (r *RequestComplexityAnalyzer) analyzeConditional(lowerRequest string) ComplexityFactor {
	weight := 0.0
	conditions := []string{}

	// Conditional keywords
	conditionalTerms := []struct {
		term   string
		weight float64
	}{
		{"if", 0.3},
		{"unless", 0.3},
		{"when", 0.2},
		{"in case", 0.3},
		{"depends", 0.3},
		{"based on", 0.2},
		{"assuming", 0.2},
		{"provided", 0.2},
		{"otherwise", 0.2},
		{"except", 0.2},
		{"but only", 0.3},
		{"as long as", 0.3},
	}

	for _, cond := range conditionalTerms {
		if strings.Contains(lowerRequest, cond.term) {
			weight += cond.weight
			conditions = append(conditions, cond.term)
		}
	}

	// Multiple conditions
	if strings.Count(lowerRequest, "if") > 1 {
		weight += 0.2
		conditions = append(conditions, "multiple conditions")
	}

	// Cap weight at 1.0
	if weight > 1.0 {
		weight = 1.0
	}

	desc := "Request contains conditional logic"
	if len(conditions) > 0 {
		desc += ": " + strings.Join(conditions, ", ")
	}

	return ComplexityFactor{
		Type:        ComplexityFactorConditional,
		Description: desc,
		Weight:      weight,
	}
}

// analyzeDataIntegration detects when multiple data sources are needed.
func (r *RequestComplexityAnalyzer) analyzeDataIntegration(lowerRequest string) ComplexityFactor {
	weight := 0.0
	dataSources := []string{}

	// Check for multiple data source keywords
	sources := map[string]string{
		"calendar": "calendar data",
		"email":    "email data",
		"task":     "task data",
		"todo":     "task data",
		"contact":  "contact data",
		"expense":  "expense data",
		"memory":   "memory data",
		"file":     "file data",
		"document": "document data",
	}

	foundSources := map[string]bool{}
	for keyword, source := range sources {
		if strings.Contains(lowerRequest, keyword) {
			if !foundSources[source] {
				foundSources[source] = true
				dataSources = append(dataSources, source)
			}
		}
	}

	// Weight based on number of unique data sources
	if len(dataSources) > 1 {
		weight = float64(len(dataSources)-1) * dataSourceMultiplier
	}

	// Check for comparison or aggregation terms
	aggregationTerms := []string{"compare", "total", "sum", "average", "combine", "merge", "together", "all"}
	for _, term := range aggregationTerms {
		if strings.Contains(lowerRequest, term) && len(dataSources) > 0 {
			weight += 0.2
			break
		}
	}

	// Cap weight at 1.0
	if weight > 1.0 {
		weight = 1.0
	}

	desc := "Request requires multiple data sources"
	if len(dataSources) > 0 {
		desc += ": " + strings.Join(dataSources, ", ")
	}

	return ComplexityFactor{
		Type:        ComplexityFactorDataIntegration,
		Description: desc,
		Weight:      weight,
	}
}

// analyzeCoordination detects when coordination between multiple entities is needed.
func (r *RequestComplexityAnalyzer) analyzeCoordination(lowerRequest string) ComplexityFactor {
	weight := 0.0
	coordinationFactors := []string{}

	// Group activities
	groupTerms := []string{"everyone", "all", "team", "group", "multiple people", "attendees", "participants"}
	for _, term := range groupTerms {
		if strings.Contains(lowerRequest, term) {
			weight += 0.3
			coordinationFactors = append(coordinationFactors, "group coordination")
			break
		}
	}

	// Back-and-forth activities
	backForthTerms := []string{"coordinate", "sync", "align", "negotiate", "back and forth", "follow up"}
	for _, term := range backForthTerms {
		if strings.Contains(lowerRequest, term) {
			weight += 0.3
			coordinationFactors = append(coordinationFactors, "coordination required")
			break
		}
	}

	// Multiple person references
	personCount := strings.Count(lowerRequest, "with") + strings.Count(lowerRequest, "and")
	if personCount > personCountThreshold {
		weight += 0.2
		coordinationFactors = append(coordinationFactors, "multiple people involved")
	}

	// Cap weight at 1.0
	if weight > 1.0 {
		weight = 1.0
	}

	desc := "Request involves coordination"
	if len(coordinationFactors) > 0 {
		desc += ": " + strings.Join(coordinationFactors, ", ")
	}

	return ComplexityFactor{
		Type:        ComplexityFactorCoordination,
		Description: desc,
		Weight:      weight,
	}
}

// calculateScore computes the overall complexity score from factors.
func (r *RequestComplexityAnalyzer) calculateScore(factors []ComplexityFactor) float64 {
	if len(factors) == 0 {
		return 0.0
	}

	// Calculate weighted average with diminishing returns for multiple factors
	totalWeight := 0.0
	for i, factor := range factors {
		// Apply diminishing returns for each additional factor
		discount := math.Pow(diminishingReturnsFactor, float64(i))
		totalWeight += factor.Weight * discount
	}

	// Normalize to 0-1 range
	score := totalWeight / float64(len(factors))
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// estimateSteps estimates the number of steps required based on complexity factors.
func (r *RequestComplexityAnalyzer) estimateSteps(factors []ComplexityFactor, lowerRequest string) int {
	baseSteps := 1

	// Add steps for each significant factor
	for _, factor := range factors {
		if factor.Type == ComplexityFactorMultiStep && factor.Weight > 0.5 {
			// Count explicit step indicators
			stepIndicators := []string{"first", "then", "next", "finally", "1.", "2.", "3."}
			for _, indicator := range stepIndicators {
				if strings.Contains(lowerRequest, indicator) {
					baseSteps++
				}
			}
		} else if factor.Weight > factorWeightThreshold {
			baseSteps++
		}
	}

	// Cap at reasonable maximum
	if baseSteps > maxEstimatedSteps {
		baseSteps = maxEstimatedSteps
	}

	return baseSteps
}

// generateSuggestions creates helpful guidance based on complexity analysis.
func (r *RequestComplexityAnalyzer) generateSuggestions(factors []ComplexityFactor, score float64) []string {
	suggestions := []string{}

	// Add general suggestions
	suggestions = append(suggestions, r.generateGeneralSuggestions(score)...)

	// Add factor-specific suggestions
	suggestions = append(suggestions, r.generateFactorSuggestions(factors)...)

	return suggestions
}

// generateGeneralSuggestions creates suggestions based on overall score.
func (r *RequestComplexityAnalyzer) generateGeneralSuggestions(score float64) []string {
	var suggestions []string

	if score >= r.decompositionThreshold {
		suggestions = append(suggestions, "Consider breaking this request into smaller, sequential tasks")
	} else if score >= r.complexityThreshold {
		suggestions = append(suggestions, "This request may require multiple steps to complete fully")
	}

	return suggestions
}

// generateFactorSuggestions creates suggestions based on specific complexity factors.
func (r *RequestComplexityAnalyzer) generateFactorSuggestions(factors []ComplexityFactor) []string {
	var suggestions []string

	for _, factor := range factors {
		suggestion := r.getSuggestionForFactor(factor)
		if suggestion != "" {
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions
}

// getSuggestionForFactor returns a suggestion for a specific complexity factor.
func (r *RequestComplexityAnalyzer) getSuggestionForFactor(factor ComplexityFactor) string {
	switch factor.Type {
	case ComplexityFactorMultiStep:
		if factor.Weight > highFactorWeight {
			return "Process each step sequentially and verify completion before proceeding"
		}
	case ComplexityFactorAmbiguous:
		if factor.Weight > mediumFactorWeight {
			return "Clarify ambiguous requirements by making reasonable assumptions or asking for specifics"
		}
	case ComplexityFactorTemporal:
		if factor.Weight > mediumFactorWeight {
			return "Pay careful attention to time constraints and scheduling conflicts"
		}
	case ComplexityFactorConditional:
		if factor.Weight > mediumFactorWeight {
			return "Evaluate conditions carefully and handle all possible cases"
		}
	case ComplexityFactorDataIntegration:
		if factor.Weight > mediumFactorWeight {
			return "Ensure all required data sources are accessed and integrated properly"
		}
	case ComplexityFactorCoordination:
		if factor.Weight > mediumFactorWeight {
			return "Consider availability and preferences of all parties involved"
		}
	}
	return ""
}

// NoopComplexityAnalyzer is a no-op implementation of ComplexityAnalyzer.
type NoopComplexityAnalyzer struct{}

// Analyze returns a minimal complexity result.
func (n *NoopComplexityAnalyzer) Analyze(_ string) ComplexityResult {
	return ComplexityResult{
		Score:                 0.0,
		Steps:                 1,
		Factors:               []ComplexityFactor{},
		Suggestions:           []string{},
		RequiresDecomposition: false,
	}
}

// IsComplex always returns false.
func (n *NoopComplexityAnalyzer) IsComplex(_ string) bool {
	return false
}

// Ensure implementations satisfy the interface.
var (
	_ ComplexityAnalyzer = (*RequestComplexityAnalyzer)(nil)
	_ ComplexityAnalyzer = (*NoopComplexityAnalyzer)(nil)
)
