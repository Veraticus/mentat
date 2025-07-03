package conversation

import (
	"fmt"
	"sync"
	"time"
)

// DefaultSessionWindow is the default session window duration.
const DefaultSessionWindow = 5 * time.Minute

// DefaultMaxHistory is the default maximum number of messages to keep per session.
const DefaultMaxHistory = 100

// session represents an active conversation session.
type session struct {
	startTime    time.Time // 8 bytes
	lastActivity time.Time // 8 bytes
	id           string    // 16 bytes
	messages     []Message // Message history for this session
}

// Manager implements the SessionManager interface.
type Manager struct {
	sessions        map[string]*session // conversationID -> session
	sessionByID     map[string]*session // sessionID -> session (for GetSessionHistory)
	identifierToSID map[string]string   // identifier -> most recent sessionID
	window          time.Duration
	mu              sync.RWMutex
	maxHistory      int // Maximum messages to keep per session
}

// NewManager creates a new conversation manager.
func NewManager(window time.Duration) *Manager {
	if window <= 0 {
		window = DefaultSessionWindow
	}

	return &Manager{
		sessions:        make(map[string]*session),
		sessionByID:     make(map[string]*session),
		identifierToSID: make(map[string]string),
		window:          window,
		maxHistory:      DefaultMaxHistory,
	}
}

// GetOrCreateSession returns the session ID for a conversation.
// Creates a new session if none exists or the current one expired.
func (m *Manager) GetOrCreateSession(conversationID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Check for existing session
	if sess, exists := m.sessions[conversationID]; exists {
		// Check if session is still valid
		if now.Sub(sess.lastActivity) <= m.window {
			sess.lastActivity = now
			m.identifierToSID[conversationID] = sess.id
			return sess.id
		}
		// Clean up expired session
		delete(m.sessionByID, sess.id)
	}

	// Create new session
	sessionID := fmt.Sprintf("conv-%s-%d", conversationID, now.UnixNano())
	newSession := &session{
		id:           sessionID,
		startTime:    now,
		lastActivity: now,
		messages:     make([]Message, 0, m.maxHistory),
	}
	m.sessions[conversationID] = newSession
	m.sessionByID[sessionID] = newSession
	m.identifierToSID[conversationID] = sessionID

	return sessionID
}

// ShouldContinueSession checks if a message should use the existing session.
// Returns true if the timestamp is within the session window.
func (m *Manager) ShouldContinueSession(conversationID string, timestamp time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, exists := m.sessions[conversationID]
	if !exists {
		return false
	}

	// Check if the timestamp is within the window
	return timestamp.Sub(sess.lastActivity) <= m.window
}

// EndSession explicitly ends a session for a conversation.
func (m *Manager) EndSession(conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, exists := m.sessions[conversationID]; exists {
		delete(m.sessionByID, sess.id)
		delete(m.sessions, conversationID)
	}
}

// CleanupExpired removes expired sessions.
func (m *Manager) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0

	for convID, sess := range m.sessions {
		if now.Sub(sess.lastActivity) > m.window {
			delete(m.sessionByID, sess.id)
			delete(m.sessions, convID)
			removed++
		}
	}

	return removed
}

// Stats returns current session statistics.
func (m *Manager) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := 0
	now := time.Now()

	for _, sess := range m.sessions {
		if now.Sub(sess.lastActivity) <= m.window {
			active++
		}
	}

	return map[string]int{
		"total":  len(m.sessions),
		"active": active,
	}
}

// GetSessionHistory returns the message history for a session.
func (m *Manager) GetSessionHistory(sessionID string) []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, exists := m.sessionByID[sessionID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	history := make([]Message, len(sess.messages))
	copy(history, sess.messages)
	return history
}

// ExpireSessions removes sessions that haven't been active since the given time.
func (m *Manager) ExpireSessions(before time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for convID, sess := range m.sessions {
		if sess.lastActivity.Before(before) {
			delete(m.sessionByID, sess.id)
			delete(m.sessions, convID)
			removed++
		}
	}

	return removed
}

// GetLastSessionID returns the most recent session ID for an identifier.
func (m *Manager) GetLastSessionID(identifier string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.identifierToSID[identifier]
}

// AddMessage adds a message to the session history.
func (m *Manager) AddMessage(sessionID string, msg Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, exists := m.sessionByID[sessionID]
	if !exists {
		return
	}

	// Add message to history
	sess.messages = append(sess.messages, msg)

	// Trim history if it exceeds max
	if len(sess.messages) > m.maxHistory {
		// Keep only the most recent messages
		sess.messages = sess.messages[len(sess.messages)-m.maxHistory:]
	}

	// Update last activity
	sess.lastActivity = time.Now()
}
