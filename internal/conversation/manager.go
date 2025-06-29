package conversation

import (
	"fmt"
	"sync"
	"time"
)

// DefaultSessionWindow is the default session window duration.
const DefaultSessionWindow = 5 * time.Minute

// session represents an active conversation session.
type session struct {
	startTime    time.Time // 8 bytes
	lastActivity time.Time // 8 bytes
	id           string    // 16 bytes
}

// Manager implements the SessionManager interface.
type Manager struct {
	sessions map[string]*session
	window   time.Duration
	mu       sync.RWMutex
}

// NewManager creates a new conversation manager.
func NewManager(window time.Duration) *Manager {
	if window <= 0 {
		window = DefaultSessionWindow
	}
	
	return &Manager{
		sessions: make(map[string]*session),
		window:   window,
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
			return sess.id
		}
	}
	
	// Create new session
	sessionID := fmt.Sprintf("conv-%s-%d", conversationID, now.UnixNano())
	m.sessions[conversationID] = &session{
		id:           sessionID,
		startTime:    now,
		lastActivity: now,
	}
	
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
	
	delete(m.sessions, conversationID)
}

// CleanupExpired removes expired sessions.
func (m *Manager) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	removed := 0
	
	for convID, sess := range m.sessions {
		if now.Sub(sess.lastActivity) > m.window {
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