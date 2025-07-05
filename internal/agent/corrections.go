// Package agent provides the multi-agent validation system for processing messages.
package agent

import (
	"fmt"
	"strings"
)

// Correction message templates for different validation scenarios.
// These messages are designed to feel conversational and natural.
const (
	// PartialCompletionTemplate is used when the request was partially completed.
	PartialCompletionTemplate = "Oops! I was only able to partially complete that. %s"

	// PartialCompletionWithDetailsTemplate includes what worked and what didn't.
	PartialCompletionWithDetailsTemplate = "Oops! I managed to %s, but I couldn't %s. %s"

	// GeneralFailureTemplate is used for general failures.
	GeneralFailureTemplate = "Oops! I ran into trouble with that request. %s"

	// TechnicalFailureTemplate is used for technical/system failures.
	TechnicalFailureTemplate = "Oops! I encountered a technical issue: %s. Let me know if you'd like me to try again."

	// IncompleteSearchTemplate is used when required tools weren't used.
	IncompleteSearchTemplate = "Oops! I think I should have checked %s for that. Let me try again with a more thorough search."

	// MultipleToolsMissedTemplate is used when multiple tools were missed.
	MultipleToolsMissedTemplate = "Oops! I realized I should have checked %s to give you a complete answer. Would you like me to do a more thorough search?"

	// UnclearRequestTemplate is used when the request intent is unclear.
	UnclearRequestTemplate = "Hmm, I'm not quite sure I understood what you need. Could you tell me more about what you're looking for?"

	// RetryableFailureTemplate suggests the user can retry.
	RetryableFailureTemplate = "Oops! That didn't work as expected. %s. Feel free to ask again and I'll give it another shot!"

	// PermissionIssueTemplate is used for permission-related failures.
	PermissionIssueTemplate = "Oops! I don't have permission to access %s. You might need to check the permissions or try a different approach."

	// TimeoutTemplate is used when operations timeout.
	TimeoutTemplate = "Oops! That took longer than expected and timed out. The %s might be taking extra time today. Want me to try again?"

	// NoIssuesButFailedTemplate is used when validation failed but no specific issues were identified.
	NoIssuesButFailedTemplate = "Oops! Something went wrong, but I'm not sure exactly what. Could you try rephrasing your request?"
)

const (
	// listSizePair represents a list with exactly 2 items.
	listSizePair = 2
)

// GenerateCorrectionMessage creates a natural correction message based on the validation result.
// This returns a static fallback message when LLM generation is not available.
func GenerateCorrectionMessage(result ValidationResult) string {
	switch result.Status {
	case ValidationStatusPartial:
		return generatePartialCompletionMessage(result)
	case ValidationStatusFailed:
		return generateFailureMessage(result)
	case ValidationStatusIncompleteSearch:
		return generateIncompleteSearchMessage(result)
	case ValidationStatusUnclear:
		return UnclearRequestTemplate
	case ValidationStatusSuccess:
		// No correction needed for success
		return ""
	default:
		// Fallback for any future status values
		return ""
	}
}

// GenerateCorrectionPrompt creates a prompt for the LLM to generate a natural correction message.
// The prompt includes a template message and context to help the LLM create a more natural response.
func GenerateCorrectionPrompt(result ValidationResult, request, response string) string {
	templateMessage := GenerateCorrectionMessage(result)

	switch result.Status {
	case ValidationStatusPartial:
		return fmt.Sprintf(
			`Generate a brief, natural message acknowledging partial completion of this request.
User request: %s
What was completed: %s
Issues preventing full completion: %s

Template style: "%s"

Create a clear, conversational response that:
1. Starts with a natural acknowledgment like "Oops!" or similar
2. Acknowledges what was successfully completed
3. Explains what couldn't be done and why (be specific but user-friendly)
4. Maintains a helpful tone without being overly apologetic
5. Keeps the message concise (2-3 sentences max)`,
			request,
			response,
			strings.Join(result.Issues, ", "),
			templateMessage,
		)

	case ValidationStatusFailed:
		issueDescription := "technical difficulties"
		if len(result.Issues) > 0 {
			issueDescription = strings.Join(result.Issues, ", ")
		}

		return fmt.Sprintf(
			`Generate a brief, natural message explaining that the request failed.
User request: %s
Issues encountered: %s

Template style: "%s"

Create a clear, conversational response that:
1. Starts with a natural acknowledgment like "Oops!" 
2. Explains the issue in user-friendly terms
3. Suggests alternatives or next steps if appropriate
4. Maintains a helpful tone
5. Keeps the message concise (1-2 sentences max)`,
			request,
			issueDescription,
			templateMessage,
		)

	case ValidationStatusIncompleteSearch:
		missingTools := ""
		if tools, hasTools := result.Metadata["expected_tools"]; hasTools {
			missingTools = tools
		}

		return fmt.Sprintf(
			`Generate a brief, natural message explaining that you should have used additional tools.
User request: %s
Tools that should have been used: %s

Template style: "%s"

Create a clear, conversational response that:
1. Starts with a natural acknowledgment like "Oops!"
2. Mentions which tools/resources should have been checked
3. Offers to do a more thorough search
4. Keeps the message concise (1-2 sentences max)`,
			request,
			missingTools,
			templateMessage,
		)

	case ValidationStatusUnclear:
		return fmt.Sprintf(
			`Generate a brief, natural message asking for clarification.
User request: %s

Template style: "%s"

Create a clear, conversational response that:
1. Starts with a gentle acknowledgment like "Hmm," or "I'm not quite sure"
2. Asks for clarification in a friendly way
3. Suggests how the user might rephrase or what additional details would help
4. Keeps the message concise (1-2 sentences max)`,
			request,
			templateMessage,
		)

	case ValidationStatusSuccess:
		// No correction needed for success
		return ""

	default:
		// For any other status, just return the template
		return templateMessage
	}
}

// generatePartialCompletionMessage creates a message for partial completions.
func generatePartialCompletionMessage(result ValidationResult) string {
	// If we have metadata about what was completed vs what wasn't, use the detailed template
	if completed, hasCompleted := result.Metadata["completed"]; hasCompleted {
		if failed, hasFailed := result.Metadata["failed"]; hasFailed {
			suggestion := ""
			if len(result.Suggestions) > 0 {
				suggestion = result.Suggestions[0]
			}
			return fmt.Sprintf(PartialCompletionWithDetailsTemplate, completed, failed, suggestion)
		}
	}

	// Fall back to issue-based message
	if len(result.Issues) == 0 {
		return fmt.Sprintf(PartialCompletionTemplate, "Some parts of your request couldn't be completed.")
	}

	// Format the issues into a natural explanation
	issueDescription := formatIssues(result.Issues)
	return fmt.Sprintf(PartialCompletionTemplate, issueDescription)
}

// generateFailureMessage creates a message for failed validations.
func generateFailureMessage(result ValidationResult) string {
	if len(result.Issues) == 0 {
		return NoIssuesButFailedTemplate
	}

	// Check for specific failure types
	firstIssue := strings.ToLower(result.Issues[0])

	// Permission issues
	if strings.Contains(firstIssue, "permission") || strings.Contains(firstIssue, "access") {
		resource := "that resource"
		if res, hasResource := result.Metadata["resource"]; hasResource {
			resource = res
		}
		return fmt.Sprintf(PermissionIssueTemplate, resource)
	}

	// Timeout issues
	if strings.Contains(firstIssue, "timeout") || strings.Contains(firstIssue, "time") {
		service := "service"
		if svc, hasService := result.Metadata["service"]; hasService {
			service = svc
		}
		return fmt.Sprintf(TimeoutTemplate, service)
	}

	// Technical/system errors
	if strings.Contains(firstIssue, "error") || strings.Contains(firstIssue, "failed") {
		return fmt.Sprintf(TechnicalFailureTemplate, result.Issues[0])
	}

	// General failure with retry suggestion
	if len(result.Suggestions) > 0 {
		return fmt.Sprintf(RetryableFailureTemplate, formatIssues(result.Issues))
	}

	// Default failure message
	return fmt.Sprintf(GeneralFailureTemplate, formatIssues(result.Issues))
}

// generateIncompleteSearchMessage creates a message for incomplete searches.
func generateIncompleteSearchMessage(result ValidationResult) string {
	// Check if we have information about which tools were missed
	if expectedTools, hasTools := result.Metadata["expected_tools"]; hasTools && expectedTools != "" {
		tools := strings.Split(expectedTools, ",")
		if len(tools) == 1 {
			toolDescription := getToolDescription(tools[0])
			return fmt.Sprintf(IncompleteSearchTemplate, toolDescription)
		}
		// Multiple tools missed
		toolDescriptions := make([]string, len(tools))
		for i, tool := range tools {
			toolDescriptions[i] = getToolDescription(strings.TrimSpace(tool))
		}
		return fmt.Sprintf(MultipleToolsMissedTemplate, formatList(toolDescriptions))
	}

	// Generic incomplete search message
	return fmt.Sprintf(IncompleteSearchTemplate, "some additional information")
}

// getToolDescription returns a user-friendly description of a tool.
func getToolDescription(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "calendar":
		return "your calendar"
	case "memory":
		return "our previous conversations"
	case "email":
		return "your emails"
	case "tasks":
		return "your task list"
	default:
		return tool
	}
}

// formatIssues converts a list of issues into a natural sentence.
func formatIssues(issues []string) string {
	if len(issues) == 0 {
		return "I encountered an issue"
	}
	if len(issues) == 1 {
		return issues[0]
	}
	// Multiple issues - combine them naturally
	return strings.Join(issues[:len(issues)-1], ", ") + " and " + issues[len(issues)-1]
}

// formatList formats a list of items into a natural sentence.
func formatList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case listSizePair:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}
