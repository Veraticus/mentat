package conversation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionPersistence handles saving and loading session data.
type SessionPersistence interface {
	SaveSessions(sessions map[string]*PersistedSession) error
	LoadSessions() (map[string]*PersistedSession, error)
}

// PersistedSession is the structure used for persistence.
// It includes all necessary data to reconstruct a session.
type PersistedSession struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	StartTime      time.Time `json:"start_time"`
	LastActivity   time.Time `json:"last_activity"`
	Messages       []Message `json:"messages"`
}

// FilePersistence implements SessionPersistence using JSON files.
type FilePersistence struct {
	directory string
}

// NewFilePersistence creates a new file-based persistence handler.
func NewFilePersistence(directory string) *FilePersistence {
	return &FilePersistence{
		directory: directory,
	}
}

// SaveSessions saves all sessions to a JSON file.
func (f *FilePersistence) SaveSessions(sessions map[string]*PersistedSession) error {
	// Ensure directory exists
	err := os.MkdirAll(f.directory, 0750)
	if err != nil {
		return fmt.Errorf("failed to create persistence directory: %w", err)
	}

	// Marshal sessions to JSON
	data, err := json.MarshalIndent(sessions, "", "  ") //nolint:musttag // marshaling map values
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}

	// Write to file atomically
	filename := filepath.Join(f.directory, "sessions.json")
	tempFile := filename + ".tmp"

	err = os.WriteFile(tempFile, data, 0600)
	if err != nil {
		return fmt.Errorf("failed to write sessions file: %w", err)
	}

	// Atomic rename
	err = os.Rename(tempFile, filename)
	if err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tempFile)
		return fmt.Errorf("failed to save sessions: %w", err)
	}

	return nil
}

// LoadSessions loads sessions from the JSON file.
func (f *FilePersistence) LoadSessions() (map[string]*PersistedSession, error) {
	filename := filepath.Join(f.directory, "sessions.json")

	// Check if file exists
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		// No sessions file, return empty map
		return make(map[string]*PersistedSession), nil
	}

	// Read file
	data, err := os.ReadFile(filename) // #nosec G304 - filename is constructed from configured directory
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions file: %w", err)
	}

	// Unmarshal sessions
	var sessions map[string]*PersistedSession
	err = json.Unmarshal(data, &sessions) //nolint:musttag // unmarshaling into map
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal sessions: %w", err)
	}

	return sessions, nil
}

// NoopPersistence implements SessionPersistence but does nothing.
// This allows persistence to be optional.
type NoopPersistence struct{}

// NewNoopPersistence creates a new no-op persistence handler.
func NewNoopPersistence() *NoopPersistence {
	return &NoopPersistence{}
}

// SaveSessions does nothing for noop persistence.
func (n *NoopPersistence) SaveSessions(_ map[string]*PersistedSession) error {
	// Intentionally does nothing
	return nil
}

// LoadSessions returns an empty map for noop persistence.
func (n *NoopPersistence) LoadSessions() (map[string]*PersistedSession, error) {
	// Always return empty map
	return make(map[string]*PersistedSession), nil
}

// ManagerWithPersistence extends Manager with persistence capabilities.
type ManagerWithPersistence struct {
	*Manager

	persistence SessionPersistence
}

// NewManagerWithPersistence creates a new manager with persistence support.
func NewManagerWithPersistence(window time.Duration, persistence SessionPersistence) *ManagerWithPersistence {
	if persistence == nil {
		persistence = NewNoopPersistence()
	}

	return &ManagerWithPersistence{
		Manager:     NewManager(window),
		persistence: persistence,
	}
}

// SaveSessions persists all current sessions.
func (m *ManagerWithPersistence) SaveSessions() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Convert internal sessions to persisted format
	persisted := make(map[string]*PersistedSession)

	for convID, sess := range m.sessions {
		persisted[convID] = &PersistedSession{
			ID:             sess.id,
			ConversationID: convID,
			StartTime:      sess.startTime,
			LastActivity:   sess.lastActivity,
			Messages:       sess.messages,
		}
	}

	err := m.persistence.SaveSessions(persisted)
	if err != nil {
		return fmt.Errorf("failed to save sessions: %w", err)
	}
	return nil
}

// RestoreSessions loads sessions from persistence.
func (m *ManagerWithPersistence) RestoreSessions() error {
	persisted, err := m.persistence.LoadSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear existing sessions
	m.sessions = make(map[string]*session)
	m.sessionByID = make(map[string]*session)
	m.identifierToSID = make(map[string]string)

	// Restore sessions
	now := time.Now()
	for convID, p := range persisted {
		// Only restore sessions that haven't expired
		if now.Sub(p.LastActivity) > m.window {
			continue
		}

		sess := &session{
			id:           p.ID,
			startTime:    p.StartTime,
			lastActivity: p.LastActivity,
			messages:     p.Messages,
		}

		m.sessions[convID] = sess
		m.sessionByID[p.ID] = sess
		m.identifierToSID[convID] = p.ID
	}

	return nil
}

// WithPeriodicSave starts a goroutine that periodically saves sessions.
// The returned function should be called to stop the periodic saves.
func (m *ManagerWithPersistence) WithPeriodicSave(interval time.Duration) func() {
	if interval <= 0 {
		interval = 1 * time.Minute
	}

	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				err := m.SaveSessions()
				if err != nil {
					// In production, log this error
					_ = err
				}
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	// Return stop function
	return func() {
		close(done)
	}
}
