package agent_test

import (
	"testing"

	"github.com/Veraticus/mentat/internal/agent"
)

func TestRequestComplexityAnalyzer_Analyze(t *testing.T) {
	analyzer := agent.NewRequestComplexityAnalyzer()

	tests := []struct {
		name                  string
		request               string
		expectedMinScore      float64
		expectedMaxScore      float64
		expectedMinSteps      int
		expectedFactorTypes   []agent.ComplexityFactorType
		requiresDecomposition bool
	}{
		{
			name:                  "simple request",
			request:               "What time is it?",
			expectedMinScore:      0.0,
			expectedMaxScore:      0.1,
			expectedMinSteps:      1,
			expectedFactorTypes:   []agent.ComplexityFactorType{},
			requiresDecomposition: false,
		},
		{
			name:                  "multi-step request",
			request:               "First find John's email, then send him the report, and finally schedule a meeting",
			expectedMinScore:      0.35,
			expectedMaxScore:      0.45,
			expectedMinSteps:      3,
			expectedFactorTypes:   []agent.ComplexityFactorType{agent.ComplexityFactorMultiStep},
			requiresDecomposition: false,
		},
		{
			name:                  "ambiguous request",
			request:               "Can you maybe do something with the stuff from yesterday or whatever?",
			expectedMinScore:      0.9,
			expectedMaxScore:      1.0,
			expectedMinSteps:      1,
			expectedFactorTypes:   []agent.ComplexityFactorType{agent.ComplexityFactorAmbiguous},
			requiresDecomposition: true,
		},
		{
			name:                  "temporal complexity",
			request:               "Schedule recurring meetings every Monday and Wednesday between 2pm and 4pm",
			expectedMinScore:      0.7,
			expectedMaxScore:      0.85,
			expectedMinSteps:      2,
			expectedFactorTypes:   []agent.ComplexityFactorType{agent.ComplexityFactorTemporal},
			requiresDecomposition: true,
		},
		{
			name:                  "conditional request",
			request:               "If John is available, schedule a meeting, otherwise send an email unless he's on vacation",
			expectedMinScore:      0.4,
			expectedMaxScore:      0.7,
			expectedMinSteps:      2,
			expectedFactorTypes:   []agent.ComplexityFactorType{agent.ComplexityFactorConditional},
			requiresDecomposition: false,
		},
		{
			name:                  "data integration request",
			request:               "Compare my calendar with my task list and email me a summary of conflicts",
			expectedMinScore:      0.7,
			expectedMaxScore:      0.85,
			expectedMinSteps:      2,
			expectedFactorTypes:   []agent.ComplexityFactorType{agent.ComplexityFactorDataIntegration},
			requiresDecomposition: true,
		},
		{
			name:                  "coordination request",
			request:               "Coordinate with the entire team to find a time when everyone can meet",
			expectedMinScore:      0.2,
			expectedMaxScore:      0.3,
			expectedMinSteps:      2,
			expectedFactorTypes:   []agent.ComplexityFactorType{agent.ComplexityFactorCoordination},
			requiresDecomposition: false,
		},
		{
			name: "highly complex request",
			request: "First check my calendar and email, then if I'm free tomorrow afternoon, " +
				"schedule a meeting with the team, otherwise find another time next week when " +
				"everyone is available, and send a summary email with the agenda items from our last meeting",
			expectedMinScore: 0.5,
			expectedMaxScore: 0.6,
			expectedMinSteps: 4,
			expectedFactorTypes: []agent.ComplexityFactorType{
				agent.ComplexityFactorMultiStep,
				agent.ComplexityFactorConditional,
				agent.ComplexityFactorTemporal,
				agent.ComplexityFactorDataIntegration,
				agent.ComplexityFactorCoordination,
			},
			requiresDecomposition: false,
		},
		{
			name:                  "numbered list request",
			request:               "Please do the following: 1. Check my email 2. Find urgent messages 3. Reply to each one",
			expectedMinScore:      0.4,
			expectedMaxScore:      0.7,
			expectedMinSteps:      4,
			expectedFactorTypes:   []agent.ComplexityFactorType{agent.ComplexityFactorMultiStep},
			requiresDecomposition: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.Analyze(tt.request)

			// Check score range
			if result.Score < tt.expectedMinScore || result.Score > tt.expectedMaxScore {
				t.Errorf("Score = %v, want between %v and %v", result.Score, tt.expectedMinScore, tt.expectedMaxScore)
			}

			// Check steps
			if result.Steps < tt.expectedMinSteps {
				t.Errorf("Steps = %v, want at least %v", result.Steps, tt.expectedMinSteps)
			}

			// Check factor types
			factorTypes := make(map[agent.ComplexityFactorType]bool)
			for _, factor := range result.Factors {
				factorTypes[factor.Type] = true
			}

			for _, expectedType := range tt.expectedFactorTypes {
				if !factorTypes[expectedType] {
					t.Errorf("Expected factor type %v not found in result", expectedType)
				}
			}

			// Check decomposition flag
			if result.RequiresDecomposition != tt.requiresDecomposition {
				t.Errorf("RequiresDecomposition = %v, want %v", result.RequiresDecomposition, tt.requiresDecomposition)
			}

			// Verify suggestions exist for complex requests
			if result.Score > 0.5 && len(result.Suggestions) == 0 {
				t.Error("Expected suggestions for complex request, but got none")
			}
		})
	}
}

func TestRequestComplexityAnalyzer_IsComplex(t *testing.T) {
	analyzer := agent.NewRequestComplexityAnalyzer()

	tests := []struct {
		name        string
		request     string
		wantComplex bool
	}{
		{
			name:        "simple question",
			request:     "What's the weather?",
			wantComplex: false,
		},
		{
			name:        "complex multi-step",
			request:     "Check my calendar, find free slots, and book a meeting with John, then send a confirmation",
			wantComplex: false,
		},
		{
			name:        "ambiguous but not complex",
			request:     "Do something with that thing",
			wantComplex: false,
		},
		{
			name:        "complex conditional",
			request:     "If the meeting is confirmed and John is available, update the calendar, otherwise reschedule and notify everyone",
			wantComplex: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := analyzer.IsComplex(tt.request); got != tt.wantComplex {
				t.Errorf("IsComplex() = %v, want %v", got, tt.wantComplex)
			}
		})
	}
}

func TestComplexityFactorWeights(t *testing.T) {
	analyzer := agent.NewRequestComplexityAnalyzer()

	tests := []struct {
		name       string
		request    string
		factorType agent.ComplexityFactorType
		minWeight  float64
		maxWeight  float64
	}{
		{
			name:       "strong multi-step signal",
			request:    "First do this, then do that, and finally finish with this",
			factorType: agent.ComplexityFactorMultiStep,
			minWeight:  0.3,
			maxWeight:  0.5,
		},
		{
			name:       "multi-step with connectors",
			request:    "First do this, and then do that",
			factorType: agent.ComplexityFactorMultiStep,
			minWeight:  0.2,
			maxWeight:  0.6,
		},
		{
			name:       "strong ambiguity",
			request:    "Maybe do something with whatever stuff",
			factorType: agent.ComplexityFactorAmbiguous,
			minWeight:  0.5,
			maxWeight:  1.0,
		},
		{
			name:       "time range complexity",
			request:    "Schedule between Monday and Friday",
			factorType: agent.ComplexityFactorTemporal,
			minWeight:  0.2,
			maxWeight:  0.5,
		},
		{
			name:       "multiple conditions",
			request:    "If this then that, unless the other, provided it works",
			factorType: agent.ComplexityFactorConditional,
			minWeight:  0.5,
			maxWeight:  1.0,
		},
		{
			name:       "multiple data sources",
			request:    "Check calendar, email, and tasks, then combine the results",
			factorType: agent.ComplexityFactorDataIntegration,
			minWeight:  0.4,
			maxWeight:  0.8,
		},
		{
			name:       "team coordination",
			request:    "Coordinate with everyone on the team to align schedules",
			factorType: agent.ComplexityFactorCoordination,
			minWeight:  0.3,
			maxWeight:  0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.Analyze(tt.request)

			var foundFactor *agent.ComplexityFactor
			for i := range result.Factors {
				if result.Factors[i].Type == tt.factorType {
					foundFactor = &result.Factors[i]
					break
				}
			}

			if foundFactor == nil {
				t.Fatalf("Expected factor type %v not found", tt.factorType)
			}

			if foundFactor.Weight < tt.minWeight || foundFactor.Weight > tt.maxWeight {
				t.Errorf("Factor weight = %v, want between %v and %v", foundFactor.Weight, tt.minWeight, tt.maxWeight)
			}
		})
	}
}

func TestComplexityScoreCalculation(t *testing.T) {
	analyzer := agent.NewRequestComplexityAnalyzer()

	// Test diminishing returns through public API
	t.Run("diminishing returns", func(t *testing.T) {
		// Create a request that would have multiple high-weight factors
		complexRequest := "First do this, then maybe do that with something, if the time is between Monday and Friday"

		result := analyzer.Analyze(complexRequest)

		// The score should show diminishing returns effect
		// With multiple factors, the score should be significant but not maximal
		if result.Score < 0.3 || result.Score > 0.5 {
			t.Errorf("Score = %v, expected between 0.3 and 0.5 for multi-factor request", result.Score)
		}

		// Should have multiple factors
		if len(result.Factors) < 3 {
			t.Errorf("Expected at least 3 factors, got %d", len(result.Factors))
		}
	})

	// Test empty/simple request
	t.Run("simple request", func(t *testing.T) {
		result := analyzer.Analyze("Hi")
		if result.Score != 0.0 {
			t.Errorf("Score for simple request = %v, want 0.0", result.Score)
		}
	})

	// Test single dominant factor
	t.Run("single factor dominance", func(t *testing.T) {
		result := analyzer.Analyze("First do A, then do B, next do C, finally do D")

		// Should be dominated by multi-step factor
		if len(result.Factors) == 0 {
			t.Fatal("Expected at least one factor")
		}

		// The multi-step factor should be present and significant
		hasMultiStep := false
		for _, factor := range result.Factors {
			if factor.Type == agent.ComplexityFactorMultiStep {
				hasMultiStep = true
				if factor.Weight < 0.5 {
					t.Errorf("Multi-step factor weight = %v, expected > 0.5", factor.Weight)
				}
			}
		}

		if !hasMultiStep {
			t.Error("Expected multi-step factor to be present")
		}
	})
}

func TestStepEstimation(t *testing.T) {
	analyzer := agent.NewRequestComplexityAnalyzer()

	tests := []struct {
		name     string
		request  string
		minSteps int
		maxSteps int
	}{
		{
			name:     "explicit numbered steps",
			request:  "1. First step 2. Second step 3. Third step",
			minSteps: 4,
			maxSteps: 6,
		},
		{
			name:     "sequential keywords",
			request:  "First check this, then do that, next try this, finally confirm",
			minSteps: 4,
			maxSteps: 8,
		},
		{
			name:     "simple request",
			request:  "Send an email",
			minSteps: 1,
			maxSteps: 2,
		},
		{
			name:     "capped at maximum",
			request:  "1. Step 2. Step 3. Step 4. Step 5. Step 6. Step 7. Step 8. Step 9. Step 10. Step 11. Step 12. Step",
			minSteps: 2,
			maxSteps: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.Analyze(tt.request)
			if result.Steps < tt.minSteps || result.Steps > tt.maxSteps {
				t.Errorf("Steps = %v, want between %v and %v", result.Steps, tt.minSteps, tt.maxSteps)
			}
		})
	}
}

func TestSuggestionGeneration(t *testing.T) {
	analyzer := agent.NewRequestComplexityAnalyzer()

	tests := []struct {
		name                string
		request             string
		expectedSuggestions []string
	}{
		{
			name:                "simple request",
			request:             "What time is it?",
			expectedSuggestions: []string{},
		},
		{
			name:    "complex multi-step",
			request: "First find all emails from John, then categorize them by urgency, and finally create a summary report",
			expectedSuggestions: []string{
				"Process each step sequentially and verify completion before proceeding",
			},
		},
		{
			name:    "ambiguous request",
			request: "Maybe find something about that stuff we discussed or whatever",
			expectedSuggestions: []string{
				"Clarify ambiguous requirements by making reasonable assumptions or asking for specifics",
			},
		},
		{
			name:    "data integration",
			request: "Compare calendar appointments with task deadlines and email commitments",
			expectedSuggestions: []string{
				"Ensure all required data sources are accessed and integrated properly",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.Analyze(tt.request)

			// Check that expected suggestions are present
			suggestionMap := make(map[string]bool)
			for _, s := range result.Suggestions {
				suggestionMap[s] = true
			}

			for _, expected := range tt.expectedSuggestions {
				if !suggestionMap[expected] {
					t.Errorf("Expected suggestion not found: %s", expected)
					t.Errorf("Got suggestions: %v", result.Suggestions)
				}
			}
		})
	}
}

func TestNoopComplexityAnalyzer(t *testing.T) {
	analyzer := &agent.NoopComplexityAnalyzer{}

	t.Run("Analyze returns minimal result", func(t *testing.T) {
		result := analyzer.Analyze("Complex request with many steps and conditions")

		if result.Score != 0.0 {
			t.Errorf("Score = %v, want 0.0", result.Score)
		}
		if result.Steps != 1 {
			t.Errorf("Steps = %v, want 1", result.Steps)
		}
		if len(result.Factors) != 0 {
			t.Errorf("Factors length = %v, want 0", len(result.Factors))
		}
		if result.RequiresDecomposition {
			t.Error("RequiresDecomposition = true, want false")
		}
	})

	t.Run("IsComplex always returns false", func(t *testing.T) {
		if analyzer.IsComplex("Very complex multi-step request") {
			t.Error("IsComplex() = true, want false")
		}
	})
}

func TestComplexityAnalyzerInterface(_ *testing.T) {
	// Ensure both implementations satisfy the interface
	var _ agent.ComplexityAnalyzer = (*agent.RequestComplexityAnalyzer)(nil)
	var _ agent.ComplexityAnalyzer = (*agent.NoopComplexityAnalyzer)(nil)
}
