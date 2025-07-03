package conversation_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/conversation"
)

func TestSessionPersistence_SaveAndLoad(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test data
	sessions := map[string]*conversation.PersistedSession{
		"conv-123": {
			ID:             "session-001",
			ConversationID: "conv-123",
			StartTime:      time.Now().Add(-10 * time.Minute),
			LastActivity:   time.Now().Add(-5 * time.Minute),
			Messages: []conversation.Message{
				{
					Timestamp: time.Now().Add(-8 * time.Minute),
					ID:        "msg-1",
					SessionID: "session-001",
					From:      "user",
					Text:      "Hello",
					Response:  "Hi there!",
				},
				{
					Timestamp: time.Now().Add(-6 * time.Minute),
					ID:        "msg-2",
					SessionID: "session-001",
					From:      "user",
					Text:      "How are you?",
					Response:  "I'm doing well, thanks!",
				},
			},
		},
		"conv-456": {
			ID:             "session-002",
			ConversationID: "conv-456",
			StartTime:      time.Now().Add(-3 * time.Minute),
			LastActivity:   time.Now().Add(-1 * time.Minute),
			Messages: []conversation.Message{
				{
					Timestamp: time.Now().Add(-2 * time.Minute),
					ID:        "msg-3",
					SessionID: "session-002",
					From:      "user",
					Text:      "What's the weather?",
					Response:  "Let me check that for you.",
				},
			},
		},
	}

	// Test file-based persistence
	t.Run("FilePersistence", func(t *testing.T) {
		persistence := conversation.NewFilePersistence(tmpDir)

		// Save sessions
		saveErr := persistence.SaveSessions(sessions)
		if saveErr != nil {
			t.Fatalf("Failed to save sessions: %v", saveErr)
		}

		// Load sessions
		loaded, err := persistence.LoadSessions()
		if err != nil {
			t.Fatalf("Failed to load sessions: %v", err)
		}

		// Verify loaded data
		if len(loaded) != len(sessions) {
			t.Errorf("Expected %d sessions, got %d", len(sessions), len(loaded))
		}

		for convID, original := range sessions {
			loaded, exists := loaded[convID]
			if !exists {
				t.Errorf("Session for conversation %s not found", convID)
				continue
			}

			// Compare session data
			if loaded.ID != original.ID {
				t.Errorf("Session ID mismatch: expected %s, got %s", original.ID, loaded.ID)
			}
			if loaded.ConversationID != original.ConversationID {
				t.Errorf("Conversation ID mismatch: expected %s, got %s",
					original.ConversationID, loaded.ConversationID)
			}
			if len(loaded.Messages) != len(original.Messages) {
				t.Errorf("Message count mismatch: expected %d, got %d", len(original.Messages), len(loaded.Messages))
			}

			// Compare messages
			for i, msg := range original.Messages {
				if i >= len(loaded.Messages) {
					break
				}
				loadedMsg := loaded.Messages[i]
				if loadedMsg.ID != msg.ID {
					t.Errorf("Message ID mismatch: expected %s, got %s", msg.ID, loadedMsg.ID)
				}
				if loadedMsg.Text != msg.Text {
					t.Errorf("Message text mismatch: expected %s, got %s", msg.Text, loadedMsg.Text)
				}
				if loadedMsg.Response != msg.Response {
					t.Errorf("Message response mismatch: expected %s, got %s", msg.Response, loadedMsg.Response)
				}
			}
		}
	})

	// Test noop persistence
	t.Run("NoopPersistence", func(t *testing.T) {
		persistence := conversation.NewNoopPersistence()

		// Save should succeed but do nothing
		saveErr := persistence.SaveSessions(sessions)
		if saveErr != nil {
			t.Errorf("Noop save should not return error: %v", saveErr)
		}

		// Load should return empty map
		loaded, err := persistence.LoadSessions()
		if err != nil {
			t.Errorf("Noop load should not return error: %v", err)
		}
		if len(loaded) != 0 {
			t.Errorf("Noop load should return empty map, got %d sessions", len(loaded))
		}
	})
}

func TestSessionPersistence_JSONSerialization(t *testing.T) {
	// Create test session
	session := &conversation.PersistedSession{
		ID:             "test-session-001",
		ConversationID: "test-conv-001",
		StartTime:      time.Now().Add(-30 * time.Minute),
		LastActivity:   time.Now().Add(-5 * time.Minute),
		Messages: []conversation.Message{
			{
				Timestamp: time.Now().Add(-20 * time.Minute),
				ID:        "msg-test-1",
				SessionID: "test-session-001",
				From:      "user",
				Text:      "Test message with special chars: \"quotes\" and 'apostrophes'",
				Response:  "Response with newline\nand tab\tcharacters",
			},
		},
	}

	// Serialize to JSON
	data, err := json.MarshalIndent(session, "", "  ") //nolint:musttag // marshaling pointer to struct
	if err != nil {
		t.Fatalf("Failed to serialize session: %v", err)
	}

	// Deserialize from JSON
	var loaded conversation.PersistedSession
	err = json.Unmarshal(data, &loaded) //nolint:musttag // unmarshaling into struct
	if err != nil {
		t.Fatalf("Failed to deserialize session: %v", err)
	}

	// Verify data integrity
	if loaded.ID != session.ID {
		t.Errorf("ID mismatch after serialization: expected %s, got %s", session.ID, loaded.ID)
	}
	if loaded.ConversationID != session.ConversationID {
		t.Errorf("ConversationID mismatch: expected %s, got %s", session.ConversationID, loaded.ConversationID)
	}
	if len(loaded.Messages) != len(session.Messages) {
		t.Errorf("Message count mismatch: expected %d, got %d", len(session.Messages), len(loaded.Messages))
	}

	// Verify special characters are preserved
	if len(loaded.Messages) > 0 {
		if loaded.Messages[0].Text != session.Messages[0].Text {
			t.Errorf("Special characters not preserved in text: expected %q, got %q",
				session.Messages[0].Text, loaded.Messages[0].Text)
		}
		if loaded.Messages[0].Response != session.Messages[0].Response {
			t.Errorf("Special characters not preserved in response: expected %q, got %q",
				session.Messages[0].Response, loaded.Messages[0].Response)
		}
	}
}

func TestSessionPersistence_ErrorHandling(t *testing.T) {
	t.Run("InvalidDirectory", func(t *testing.T) {
		// Use a non-existent directory
		persistence := conversation.NewFilePersistence("/non/existent/directory")

		sessions := map[string]*conversation.PersistedSession{
			"test": {
				ID:             "test-session",
				ConversationID: "test",
				StartTime:      time.Now(),
				LastActivity:   time.Now(),
			},
		}

		// Save should fail
		saveErr := persistence.SaveSessions(sessions)
		if saveErr == nil {
			t.Error("Expected error when saving to invalid directory")
		}
	})

	t.Run("CorruptedFile", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Write corrupted JSON
		corruptFile := filepath.Join(tmpDir, "sessions.json")
		err := os.WriteFile(corruptFile, []byte("{invalid json"), 0644)
		if err != nil {
			t.Fatalf("Failed to write corrupt file: %v", err)
		}

		persistence := conversation.NewFilePersistence(tmpDir)

		// Load should fail
		_, loadErr := persistence.LoadSessions()
		if loadErr == nil {
			t.Error("Expected error when loading corrupted file")
		}
	})

	t.Run("EmptyFile", func(t *testing.T) {
		tmpDir := t.TempDir()

		persistence := conversation.NewFilePersistence(tmpDir)

		// Load from non-existent file should return empty map
		loaded, err := persistence.LoadSessions()
		if err != nil {
			t.Errorf("Load from non-existent file should not error: %v", err)
		}
		if len(loaded) != 0 {
			t.Errorf("Load from non-existent file should return empty map, got %d sessions", len(loaded))
		}
	})
}

func TestSessionPersistence_Integration(t *testing.T) {
	// This test verifies that persistence works with the Manager
	tmpDir := t.TempDir()

	// Create manager with persistence
	persistence := conversation.NewFilePersistence(tmpDir)
	manager := conversation.NewManagerWithPersistence(5*time.Minute, persistence)

	// Create some sessions
	sessionID1 := manager.GetOrCreateSession("user1")
	manager.AddMessage(sessionID1, conversation.Message{
		Timestamp: time.Now(),
		ID:        "msg-1",
		SessionID: sessionID1,
		From:      "user1",
		Text:      "Hello",
		Response:  "Hi there!",
	})

	sessionID2 := manager.GetOrCreateSession("user2")
	manager.AddMessage(sessionID2, conversation.Message{
		Timestamp: time.Now(),
		ID:        "msg-2",
		SessionID: sessionID2,
		From:      "user2",
		Text:      "Hey",
		Response:  "Hello!",
	})

	// Save sessions
	err := manager.SaveSessions()
	if err != nil {
		t.Fatalf("Failed to save sessions: %v", err)
	}

	// Create new manager and restore
	manager2 := conversation.NewManagerWithPersistence(5*time.Minute, persistence)
	err2 := manager2.RestoreSessions()
	if err2 != nil {
		t.Fatalf("Failed to restore sessions: %v", err2)
	}

	// Verify sessions were restored
	history1 := manager2.GetSessionHistory(sessionID1)
	if len(history1) != 1 {
		t.Errorf("Expected 1 message in session 1, got %d", len(history1))
	}

	history2 := manager2.GetSessionHistory(sessionID2)
	if len(history2) != 1 {
		t.Errorf("Expected 1 message in session 2, got %d", len(history2))
	}

	// Verify we can get last session IDs
	lastID1 := manager2.GetLastSessionID("user1")
	if lastID1 != sessionID1 {
		t.Errorf("Expected last session ID for user1 to be %s, got %s", sessionID1, lastID1)
	}

	lastID2 := manager2.GetLastSessionID("user2")
	if lastID2 != sessionID2 {
		t.Errorf("Expected last session ID for user2 to be %s, got %s", sessionID2, lastID2)
	}
}
