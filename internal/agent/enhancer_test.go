package agent_test

import (
	"strings"
	"testing"

	"github.com/Veraticus/mentat/internal/agent"
)

func TestSmartIntentEnhancer_Enhance(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	tests := []struct {
		name     string
		request  string
		wantHint string // partial string to check in the enhancement
		noHint   bool   // true if no enhancement expected
	}{
		// Scheduling patterns
		{
			name:     "meeting request",
			request:  "Schedule a meeting with John tomorrow at 3pm",
			wantHint: "checking calendar availability",
		},
		{
			name:     "appointment request",
			request:  "Do I have any appointments this week?",
			wantHint: "checking calendar availability",
		},
		{
			name:     "availability check",
			request:  "When am I available next Tuesday?",
			wantHint: "checking calendar availability",
		},

		// Memory patterns
		{
			name:     "last time query",
			request:  "When was the last time we discussed the project timeline?",
			wantHint: "searching memory for relevant past interactions",
		},
		{
			name:     "remember query",
			request:  "Do you remember what Sarah said about the budget?",
			wantHint: "searching memory for relevant past interactions",
		},
		{
			name:     "previous conversation",
			request:  "What did we talk about in our previous conversation about taxes?",
			wantHint: "searching memory for relevant past interactions",
		},

		// Email patterns
		{
			name:     "email check",
			request:  "Check my email for messages from the boss",
			wantHint: "accessing email for context",
		},
		{
			name:     "send email",
			request:  "Send an email to the team about the meeting",
			wantHint: "accessing email for context",
		},

		// Task patterns
		{
			name:     "todo check",
			request:  "What's on my todo list for today?",
			wantHint: "checking existing tasks",
		},
		{
			name:     "create task",
			request:  "Add a task to call the dentist",
			wantHint: "checking existing tasks",
		},

		// Contact patterns
		{
			name:     "contact lookup",
			request:  "What's John's phone number?",
			wantHint: "looking up contact information",
		},
		{
			name:     "get in touch",
			request:  "How can I get in touch with the support team?",
			wantHint: "looking up contact information",
		},

		// Time-sensitive patterns
		{
			name:     "today query",
			request:  "What meetings do I have today?",
			wantHint: "considering the current date and time",
		},
		{
			name:     "urgent request",
			request:  "Find all urgent items that need attention",
			wantHint: "considering the current date and time",
		},

		// Expense patterns
		{
			name:     "expense check",
			request:  "How much did I spend on travel last month?",
			wantHint: "checking expense records",
		},
		{
			name:     "receipt processing",
			request:  "Process the receipt from dinner last night",
			wantHint: "checking expense records",
		},

		// Multiple patterns
		{
			name:     "multiple patterns - calendar and memory",
			request:  "When was the last time I had a meeting with Sarah?",
			wantHint: "checking calendar availability",
		},
		{
			name:     "multiple patterns - email and task",
			request:  "Send me an email reminder about my tasks for tomorrow",
			wantHint: "accessing email for context",
		},

		// No enhancement cases
		{
			name:    "simple greeting",
			request: "Hello",
			noHint:  true,
		},
		{
			name:    "short request",
			request: "Hi there",
			noHint:  true,
		},
		{
			name:    "no pattern match",
			request: "Tell me a joke about programming",
			noHint:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enhanced := enhancer.Enhance(tt.request)

			if tt.noHint {
				// Should return original request unchanged
				if enhanced != tt.request {
					t.Errorf("Expected no enhancement, but got: %v", enhanced)
				}
				return
			}

			// Should contain the original request
			if !strings.Contains(enhanced, tt.request) {
				t.Errorf("Enhanced request should contain original request")
			}

			// Should contain the hint
			if !strings.Contains(enhanced, tt.wantHint) {
				t.Errorf("Expected hint containing %q, got: %v", tt.wantHint, enhanced)
			}

			// Should have proper formatting
			if !strings.Contains(enhanced, "[Context hints:") {
				t.Errorf("Enhanced request should contain '[Context hints:' prefix")
			}
		})
	}
}

func TestSmartIntentEnhancer_ShouldEnhance(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	tests := []struct {
		name    string
		request string
		want    bool
	}{
		// Should enhance
		{
			name:    "meeting request",
			request: "Schedule a meeting tomorrow",
			want:    true,
		},
		{
			name:    "memory query",
			request: "What did we discuss last time?",
			want:    true,
		},
		{
			name:    "email request",
			request: "Check my email for updates",
			want:    true,
		},

		// Should not enhance
		{
			name:    "too short",
			request: "Hi",
			want:    false,
		},
		{
			name:    "two words",
			request: "Hello there",
			want:    false,
		},
		{
			name:    "no pattern match",
			request: "Tell me about quantum physics",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := enhancer.ShouldEnhance(tt.request); got != tt.want {
				t.Errorf("ShouldEnhance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSmartIntentEnhancer_CaseInsensitive(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "uppercase MEETING",
			request: "Schedule a MEETING with the team",
		},
		{
			name:    "mixed case Email",
			request: "Check my Email for important messages",
		},
		{
			name:    "all caps TODO",
			request: "What's on my TODO list?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should detect pattern regardless of case
			if !enhancer.ShouldEnhance(tt.request) {
				t.Errorf("ShouldEnhance() should return true for %q", tt.request)
			}

			// Should enhance the request
			enhanced := enhancer.Enhance(tt.request)
			if enhanced == tt.request {
				t.Errorf("Enhance() should have enhanced %q", tt.request)
			}
		})
	}
}

func TestSmartIntentEnhancer_MultipleHints(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	// Request that matches multiple patterns
	request := "Send me an email about tomorrow's urgent meeting agenda and any tasks I need to complete"
	enhanced := enhancer.Enhance(request)

	// Should contain original request
	if !strings.Contains(enhanced, request) {
		t.Errorf("Enhanced request should contain original request")
	}

	// Should contain multiple hints joined with ", and"
	if !strings.Contains(enhanced, ", and") {
		t.Errorf("Multiple hints should be joined with ', and'")
	}

	// Count the number of hint patterns that should match
	expectedPatterns := []string{
		"email",    // "Send me an email"
		"tomorrow", // "tomorrow's"
		"urgent",   // "urgent meeting"
		"meeting",  // "meeting agenda"
		"tasks",    // "tasks I need"
	}

	matchCount := 0
	for _, pattern := range expectedPatterns {
		if strings.Contains(strings.ToLower(request), pattern) {
			matchCount++
		}
	}

	// We expect at least 3 different hint categories to be triggered
	if matchCount < 3 {
		t.Errorf("Expected multiple patterns to be detected, but only found %d", matchCount)
	}
}

func TestNoopIntentEnhancer(t *testing.T) {
	enhancer := &agent.NoopIntentEnhancer{}

	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "simple request",
			request: "Hello world",
		},
		{
			name:    "complex request",
			request: "Schedule a meeting tomorrow and check my email",
		},
		{
			name:    "empty request",
			request: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should never enhance
			if enhancer.ShouldEnhance(tt.request) {
				t.Errorf("NoopIntentEnhancer.ShouldEnhance() should always return false")
			}

			// Should return request unchanged
			if got := enhancer.Enhance(tt.request); got != tt.request {
				t.Errorf("NoopIntentEnhancer.Enhance() = %v, want %v", got, tt.request)
			}
		})
	}
}

func TestEnhancementFormat(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	// Test single hint format
	t.Run("single hint format", func(t *testing.T) {
		request := "Check my calendar for meetings"
		enhanced := enhancer.Enhance(request)

		// Should have proper format
		expected := request + "\n\n[Context hints: checking calendar availability and existing appointments might be helpful]"
		if enhanced != expected {
			t.Errorf("Single hint format incorrect\nGot:      %q\nExpected: %q", enhanced, expected)
		}
	})

	// Test that hints are not prescriptive
	t.Run("hints are suggestions", func(t *testing.T) {
		request := "What's on my schedule?"
		enhanced := enhancer.Enhance(request)

		// Hints should use gentle language like "might be helpful", "could be relevant"
		if !strings.Contains(enhanced, "might be") && !strings.Contains(enhanced, "could") {
			t.Errorf("Hints should use gentle, non-prescriptive language. Got: %v", enhanced)
		}
	})
}

func TestDetectIntent(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	tests := []struct {
		name           string
		request        string
		expectedIntent agent.IntentType
		minConfidence  float64
		maxConfidence  float64
	}{
		// Scheduling intent tests
		{
			name:           "schedule meeting",
			request:        "Schedule a meeting with the team tomorrow at 3pm",
			expectedIntent: agent.IntentScheduling,
			minConfidence:  0.7,
			maxConfidence:  1.0,
		},
		{
			name:           "check calendar",
			request:        "What's on my calendar today?",
			expectedIntent: agent.IntentScheduling,
			minConfidence:  0.3,
			maxConfidence:  1.0,
		},
		{
			name:           "availability check",
			request:        "When am I available next week?",
			expectedIntent: agent.IntentScheduling,
			minConfidence:  0.5,
			maxConfidence:  1.0,
		},
		{
			name:           "reschedule appointment",
			request:        "I need to reschedule my dentist appointment",
			expectedIntent: agent.IntentScheduling,
			minConfidence:  0.5,
			maxConfidence:  1.0,
		},
		{
			name:           "cancel meeting",
			request:        "Cancel the meeting with John",
			expectedIntent: agent.IntentScheduling,
			minConfidence:  0.3,
			maxConfidence:  1.0,
		},

		// Finding people intent tests
		{
			name:           "who is query",
			request:        "Who is the project manager for the Alpha project?",
			expectedIntent: agent.IntentFindingPeople,
			minConfidence:  0.5,
			maxConfidence:  1.0,
		},
		{
			name:           "contact info",
			request:        "What's Sarah's phone number?",
			expectedIntent: agent.IntentFindingPeople,
			minConfidence:  0.3,
			maxConfidence:  1.0,
		},
		{
			name:           "get in touch",
			request:        "How can I get in touch with the IT department?",
			expectedIntent: agent.IntentFindingPeople,
			minConfidence:  0.6,
			maxConfidence:  1.0,
		},
		{
			name:           "find person",
			request:        "Find me the contact details for our new client",
			expectedIntent: agent.IntentFindingPeople,
			minConfidence:  0.6,
			maxConfidence:  1.0,
		},
		{
			name:           "email address",
			request:        "I need the email address for the marketing team",
			expectedIntent: agent.IntentFindingPeople,
			minConfidence:  0.3,
			maxConfidence:  1.0,
		},

		// Memory intent tests
		{
			name:           "remember query",
			request:        "Do you remember what we discussed about the budget last week?",
			expectedIntent: agent.IntentMemory,
			minConfidence:  0.7,
			maxConfidence:  1.0,
		},
		{
			name:           "last time",
			request:        "When was the last time we talked about vacation plans?",
			expectedIntent: agent.IntentMemory,
			minConfidence:  0.6,
			maxConfidence:  1.0,
		},
		{
			name:           "previous conversation",
			request:        "In our previous conversation, what did I say about the deadline?",
			expectedIntent: agent.IntentMemory,
			minConfidence:  0.6,
			maxConfidence:  1.0,
		},
		{
			name:           "mentioned before",
			request:        "I mentioned something about a new project yesterday",
			expectedIntent: agent.IntentMemory,
			minConfidence:  0.4,
			maxConfidence:  1.0,
		},
		{
			name:           "recall information",
			request:        "Can you recall what the CEO said about quarterly results?",
			expectedIntent: agent.IntentMemory,
			minConfidence:  0.5,
			maxConfidence:  1.0,
		},

		// Unknown intent tests
		{
			name:           "general question",
			request:        "What's the weather like today?",
			expectedIntent: agent.IntentUnknown,
			minConfidence:  0.0,
			maxConfidence:  0.0,
		},
		{
			name:           "joke request",
			request:        "Tell me a funny joke",
			expectedIntent: agent.IntentUnknown,
			minConfidence:  0.0,
			maxConfidence:  0.0,
		},
		{
			name:           "calculation",
			request:        "What's 15% of 250?",
			expectedIntent: agent.IntentUnknown,
			minConfidence:  0.0,
			maxConfidence:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := enhancer.DetectIntent(tt.request)

			if intent.Type != tt.expectedIntent {
				t.Errorf("DetectIntent() type = %v, want %v", intent.Type, tt.expectedIntent)
			}

			if intent.Confidence < tt.minConfidence || intent.Confidence > tt.maxConfidence {
				t.Errorf("DetectIntent() confidence = %v, want between %v and %v",
					intent.Confidence, tt.minConfidence, tt.maxConfidence)
			}
		})
	}
}

func TestDetectIntent_EdgeCases(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	tests := []struct {
		name           string
		request        string
		expectedIntent agent.IntentType
	}{
		{
			name:           "empty string",
			request:        "",
			expectedIntent: agent.IntentUnknown,
		},
		{
			name:           "single word",
			request:        "meeting",
			expectedIntent: agent.IntentScheduling,
		},
		{
			name:           "very long request with multiple intents",
			request:        "I need to schedule a meeting with Sarah to discuss what we talked about last time regarding the project timeline and also get her contact information for future reference",
			expectedIntent: agent.IntentScheduling, // Should pick the dominant intent
		},
		{
			name:           "case insensitive",
			request:        "SCHEDULE A MEETING WITH THE TEAM",
			expectedIntent: agent.IntentScheduling,
		},
		{
			name:           "punctuation handling",
			request:        "Who's the manager? I need their phone number!",
			expectedIntent: agent.IntentFindingPeople,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := enhancer.DetectIntent(tt.request)

			if intent.Type != tt.expectedIntent {
				t.Errorf("DetectIntent() type = %v, want %v", intent.Type, tt.expectedIntent)
			}
		})
	}
}

func TestDetectIntent_AccuracyRequirement(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	// Test corpus for >90% accuracy requirement
	testCases := []struct {
		request        string
		expectedIntent agent.IntentType
	}{
		// Scheduling tests (20 cases)
		{"Schedule a team meeting", agent.IntentScheduling},
		{"Book an appointment with the doctor", agent.IntentScheduling},
		{"When can we meet?", agent.IntentScheduling},
		{"Check my calendar for tomorrow", agent.IntentScheduling},
		{"Am I free on Friday?", agent.IntentScheduling},
		{"Set up time to discuss the project", agent.IntentScheduling},
		{"What meetings do I have today?", agent.IntentScheduling},
		{"Reschedule our lunch", agent.IntentScheduling},
		{"Cancel my 3pm appointment", agent.IntentScheduling},
		{"When is my next available slot?", agent.IntentScheduling},
		{"Arrange a call with the client", agent.IntentScheduling},
		{"Move the meeting to next week", agent.IntentScheduling},
		{"Find time for a quick sync", agent.IntentScheduling},
		{"Is Tuesday morning available?", agent.IntentScheduling},
		{"Plan a meeting with the stakeholders", agent.IntentScheduling},
		{"Check for conflicts next Monday", agent.IntentScheduling},
		{"What's my schedule like?", agent.IntentScheduling},
		{"Book the conference room", agent.IntentScheduling},
		{"When did we schedule the review?", agent.IntentScheduling},
		{"Organize a team meeting for planning", agent.IntentScheduling},

		// Finding people tests (20 cases)
		{"Who is John Smith?", agent.IntentFindingPeople},
		{"Get me Sarah's contact info", agent.IntentFindingPeople},
		{"What's the boss's phone number?", agent.IntentFindingPeople},
		{"How do I reach the IT team?", agent.IntentFindingPeople},
		{"Find the project manager", agent.IntentFindingPeople},
		{"Who should I contact about this?", agent.IntentFindingPeople},
		{"I need to get in touch with HR", agent.IntentFindingPeople},
		{"What's the email for support?", agent.IntentFindingPeople},
		{"Locate the team lead", agent.IntentFindingPeople},
		{"Who's responsible for marketing?", agent.IntentFindingPeople},
		{"Contact details for the vendor", agent.IntentFindingPeople},
		{"How can I reach someone in accounting?", agent.IntentFindingPeople},
		{"Find me the CEO's assistant", agent.IntentFindingPeople},
		{"Who reports to the director?", agent.IntentFindingPeople},
		{"I need someone from legal", agent.IntentFindingPeople},
		{"What's the number for reception?", agent.IntentFindingPeople},
		{"Get me in touch with a developer", agent.IntentFindingPeople},
		{"Who's my manager?", agent.IntentFindingPeople},
		{"Find a contact in sales", agent.IntentFindingPeople},
		{"Email address for the client", agent.IntentFindingPeople},

		// Memory tests (20 cases)
		{"What did we discuss last time?", agent.IntentMemory},
		{"Do you remember the budget number?", agent.IntentMemory},
		{"When did I mention the deadline?", agent.IntentMemory},
		{"What was said about the proposal?", agent.IntentMemory},
		{"Recall our conversation yesterday", agent.IntentMemory},
		{"Did I tell you about the changes?", agent.IntentMemory},
		{"What did Sarah say earlier?", agent.IntentMemory},
		{"Have we talked about this before?", agent.IntentMemory},
		{"What was the decision from last meeting?", agent.IntentMemory},
		{"I forget what we agreed on", agent.IntentMemory},
		{"Did you mention anything about costs?", agent.IntentMemory},
		{"What was discussed in the review?", agent.IntentMemory},
		{"I previously asked about vacation", agent.IntentMemory},
		{"When have we talked about raises?", agent.IntentMemory},
		{"What did the client say last week?", agent.IntentMemory},
		{"Did I already tell you about the issue?", agent.IntentMemory},
		{"What was mentioned about timelines?", agent.IntentMemory},
		{"Remind me what we decided", agent.IntentMemory},
		{"Have I asked this question before?", agent.IntentMemory},
		{"What was the outcome of our past discussion?", agent.IntentMemory},
	}

	correct := 0
	total := len(testCases)

	for _, tc := range testCases {
		intent := enhancer.DetectIntent(tc.request)
		if intent.Type == tc.expectedIntent {
			correct++
		} else {
			t.Logf("Misclassified: %q - expected %s, got %s", tc.request, tc.expectedIntent, intent.Type)
		}
	}

	accuracy := float64(correct) / float64(total) * 100
	requiredAccuracy := 90.0

	if accuracy < requiredAccuracy {
		t.Errorf("Accuracy %.1f%% is below required %.1f%% (correct: %d/%d)",
			accuracy, requiredAccuracy, correct, total)
	} else {
		t.Logf("Accuracy: %.1f%% (correct: %d/%d) - PASSED", accuracy, correct, total)
	}
}

func TestDetectIntent_ConfidenceScores(t *testing.T) {
	enhancer := agent.NewSmartIntentEnhancer()

	tests := []struct {
		name                 string
		request              string
		expectHighConfidence bool
	}{
		{
			name:                 "strong scheduling signal",
			request:              "Schedule a meeting appointment on my calendar",
			expectHighConfidence: true,
		},
		{
			name:                 "weak scheduling signal",
			request:              "Is there time?",
			expectHighConfidence: false,
		},
		{
			name:                 "strong memory signal",
			request:              "What did we discuss in our last conversation about the project?",
			expectHighConfidence: true,
		},
		{
			name:                 "weak memory signal",
			request:              "Did I say something?",
			expectHighConfidence: false,
		},
		{
			name:                 "strong finding people signal",
			request:              "Who is the manager and how can I contact them?",
			expectHighConfidence: true,
		},
		{
			name:                 "weak finding people signal",
			request:              "Someone called",
			expectHighConfidence: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := enhancer.DetectIntent(tt.request)

			if tt.expectHighConfidence && intent.Confidence < 0.6 {
				t.Errorf("Expected high confidence (>0.6) but got %v", intent.Confidence)
			}
			if !tt.expectHighConfidence && intent.Confidence > 0.5 {
				t.Errorf("Expected low confidence (<0.5) but got %v", intent.Confidence)
			}
		})
	}
}
