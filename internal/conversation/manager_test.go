package conversation_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/conversation"
)

func TestManager_GetOrCreateSession(t *testing.T) {
	m := conversation.NewManager(5 * time.Minute)

	// Test new session creation
	sessionID1 := m.GetOrCreateSession("user123")
	if sessionID1 == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Test reusing existing session
	sessionID2 := m.GetOrCreateSession("user123")
	if sessionID1 != sessionID2 {
		t.Errorf("expected same session ID, got different: %s vs %s", sessionID1, sessionID2)
	}

	// Test different user gets different session
	sessionID3 := m.GetOrCreateSession("user456")
	if sessionID3 == sessionID1 {
		t.Error("expected different session ID for different user")
	}
}

func TestManager_GetSessionHistory(t *testing.T) {
	m := conversation.NewManager(5 * time.Minute)

	// Create a session
	sessionID := m.GetOrCreateSession("user123")

	// Initially should have no history
	history := m.GetSessionHistory(sessionID)
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d messages", len(history))
	}

	// Add some messages
	msg1 := conversation.Message{
		ID:        "msg1",
		SessionID: sessionID,
		From:      "user123",
		Text:      "Hello",
		Timestamp: time.Now(),
	}
	msg2 := conversation.Message{
		ID:        "msg2",
		SessionID: sessionID,
		From:      "assistant",
		Text:      "Hi there!",
		Timestamp: time.Now(),
	}

	m.AddMessage(sessionID, msg1)
	m.AddMessage(sessionID, msg2)

	// Should have 2 messages
	history = m.GetSessionHistory(sessionID)
	if len(history) != 2 {
		t.Errorf("expected 2 messages, got %d", len(history))
	}

	// Verify message order
	if history[0].ID != "msg1" {
		t.Errorf("expected first message to be msg1, got %s", history[0].ID)
	}
	if history[1].ID != "msg2" {
		t.Errorf("expected second message to be msg2, got %s", history[1].ID)
	}

	// Non-existent session should return nil
	history = m.GetSessionHistory("non-existent")
	if history != nil {
		t.Error("expected nil history for non-existent session")
	}
}

func TestManager_ExpireSessions(t *testing.T) {
	m := conversation.NewManager(100 * time.Millisecond)

	// Create a session
	sessionID1 := m.GetOrCreateSession("user1")
	m.AddMessage(sessionID1, conversation.Message{ID: "msg1"})

	// Wait for it to be old enough
	time.Sleep(150 * time.Millisecond)

	// Create another session
	sessionID2 := m.GetOrCreateSession("user2")
	m.AddMessage(sessionID2, conversation.Message{ID: "msg2"})

	// Expire sessions older than 50ms ago
	beforeTime := time.Now().Add(-50 * time.Millisecond)
	removed := m.ExpireSessions(beforeTime)

	// Should have removed 1 session (user1)
	if removed != 1 {
		t.Errorf("expected to remove 1 session, removed %d", removed)
	}

	// Verify sessions are actually removed
	history1 := m.GetSessionHistory(sessionID1)
	history2 := m.GetSessionHistory(sessionID2)

	if history1 != nil {
		t.Error("expected session1 to be expired")
	}
	if history2 == nil || len(history2) != 1 {
		t.Error("expected session2 to still exist with history")
	}
}

func TestManager_GetLastSessionID(t *testing.T) {
	m := conversation.NewManager(5 * time.Minute)

	// Initially should return empty
	lastID := m.GetLastSessionID("user123")
	if lastID != "" {
		t.Errorf("expected empty last session ID, got %s", lastID)
	}

	// Create a session
	sessionID1 := m.GetOrCreateSession("user123")
	lastID = m.GetLastSessionID("user123")
	if lastID != sessionID1 {
		t.Errorf("expected last session ID to be %s, got %s", sessionID1, lastID)
	}

	// Create another session for different user
	sessionID2 := m.GetOrCreateSession("user456")

	// Verify each user has correct last session
	if m.GetLastSessionID("user123") != sessionID1 {
		t.Error("user123's last session ID changed unexpectedly")
	}
	if m.GetLastSessionID("user456") != sessionID2 {
		t.Error("user456's last session ID incorrect")
	}
}

func TestManager_MessageHistoryBounds(t *testing.T) {
	m := conversation.NewManager(5 * time.Minute)

	sessionID := m.GetOrCreateSession("user123")

	// Add more messages than the default limit (100)
	for i := range 105 {
		m.AddMessage(sessionID, conversation.Message{
			ID:   string(rune('a' + (i % 26))),
			Text: "Message " + string(rune('a'+(i%26))),
		})
	}

	// Should only have the last 100 messages (default limit)
	history := m.GetSessionHistory(sessionID)
	if len(history) != 100 {
		t.Errorf("expected 100 messages, got %d", len(history))
	}
}

func TestManager_ThreadSafety(t *testing.T) {
	m := conversation.NewManager(5 * time.Minute)

	var wg sync.WaitGroup
	concurrency := 10
	iterations := 100

	// Concurrent session creation
	wg.Add(concurrency)
	for i := range concurrency {
		go func(userID int) {
			defer wg.Done()
			for range iterations {
				sessionID := m.GetOrCreateSession("user" + string(rune('0'+userID)))
				m.AddMessage(sessionID, conversation.Message{
					ID:   "msg",
					Text: "Test message",
				})
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(concurrency)
	for i := range concurrency {
		go func(userID int) {
			defer wg.Done()
			for range iterations {
				m.GetLastSessionID("user" + string(rune('0'+userID)))
				sessionID := m.GetOrCreateSession("user" + string(rune('0'+userID)))
				m.GetSessionHistory(sessionID)
			}
		}(i)
	}

	// Concurrent cleanup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations / 10 {
			m.CleanupExpired()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Verify state is consistent
	stats := m.Stats()
	if stats["total"] < 0 || stats["active"] < 0 {
		t.Error("negative stats indicate race condition")
	}
}

func TestManager_SlidingWindow(t *testing.T) {
	// Use short window for testing
	m := conversation.NewManager(200 * time.Millisecond)

	sessionID := m.GetOrCreateSession("user123")

	// Make multiple requests within the window
	for i := range 3 {
		time.Sleep(50 * time.Millisecond)
		newID := m.GetOrCreateSession("user123")
		if newID != sessionID {
			t.Errorf("expected same session ID within window, got new ID on iteration %d", i)
		}
		// Add a message to update activity
		m.AddMessage(sessionID, conversation.Message{ID: "msg" + string(rune('0'+i))})
	}

	// Wait for window to expire
	time.Sleep(250 * time.Millisecond)
	expiredID := m.GetOrCreateSession("user123")
	if expiredID == sessionID {
		t.Error("expected new session after window expired")
	}
}

func TestManager_CleanupExpired(t *testing.T) {
	m := conversation.NewManager(100 * time.Millisecond)

	// Create sessions
	m.GetOrCreateSession("user1")
	time.Sleep(150 * time.Millisecond)
	m.GetOrCreateSession("user2")

	// Cleanup should remove user1's session
	removed := m.CleanupExpired()
	if removed != 1 {
		t.Errorf("expected 1 session removed, got %d", removed)
	}

	// Verify only user2 remains
	stats := m.Stats()
	if stats["total"] != 1 {
		t.Errorf("expected 1 total session, got %d", stats["total"])
	}
}

func TestManager_SessionHistory_ReturnsCopy(t *testing.T) {
	m := conversation.NewManager(5 * time.Minute)
	sessionID := m.GetOrCreateSession("user123")

	// Add a message
	original := conversation.Message{
		ID:   "msg1",
		Text: "Original text",
	}
	m.AddMessage(sessionID, original)

	// Get history and modify it
	history := m.GetSessionHistory(sessionID)
	history[0].Text = "Modified text"

	// Get history again - should still have original text
	history2 := m.GetSessionHistory(sessionID)
	if history2[0].Text != "Original text" {
		t.Error("GetSessionHistory did not return a copy - external modification affected internal state")
	}
}

func TestManager_AddMessage_UpdatesActivity(t *testing.T) {
	m := conversation.NewManager(100 * time.Millisecond)
	sessionID := m.GetOrCreateSession("user123")

	// Wait a bit
	time.Sleep(80 * time.Millisecond)

	// Add a message - this should update the activity time
	m.AddMessage(sessionID, conversation.Message{ID: "msg1"})

	// Wait a bit more - total time > 100ms but message was recent
	time.Sleep(50 * time.Millisecond)

	// Session should still be active due to sliding window
	newID := m.GetOrCreateSession("user123")
	if newID != sessionID {
		t.Error("AddMessage should have updated activity time")
	}
}
