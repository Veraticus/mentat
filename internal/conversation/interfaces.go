package conversation

import "time"

// SessionManager handles conversation session lifecycle.
// A session groups related messages within a time window.
type SessionManager interface {
	// GetOrCreateSession returns the session ID for a conversation.
	// Creates a new session if none exists or the current one expired.
	GetOrCreateSession(conversationID string) string
	
	// ShouldContinueSession checks if a message should use the existing session.
	// Returns true if the timestamp is within the session window.
	ShouldContinueSession(conversationID string, timestamp time.Time) bool
	
	// EndSession explicitly ends a session for a conversation.
	EndSession(conversationID string)
}