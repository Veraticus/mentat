// Package conversation provides session management for maintaining conversation context.
package conversation

import "time"

// SessionManager handles conversation continuity.
type SessionManager interface {
	// GetOrCreateSession returns the session ID for a user identifier
	GetOrCreateSession(identifier string) string
	
	// GetSessionHistory returns the message history for a session
	GetSessionHistory(sessionID string) []Message
	
	// ExpireSessions removes sessions that haven't been active since the given time
	ExpireSessions(before time.Time) int
	
	// GetLastSessionID returns the most recent session ID for an identifier
	GetLastSessionID(identifier string) string
}

// Message represents a message within a conversation session.
type Message struct {
	Timestamp time.Time // 8 bytes
	ID        string    // 16 bytes
	SessionID string    // 16 bytes
	From      string    // 16 bytes
	Text      string    // 16 bytes
	Response  string    // 16 bytes
}