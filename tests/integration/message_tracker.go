// +build integration

package integration

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
)

// MessageTracker provides detailed tracking of message lifecycle
type MessageTracker struct {
	mu            sync.RWMutex
	messages      map[string]*TrackedMessage
	stateChanges  chan StateTransition
	completed     int32
	failed        int32
	retrying      int32
}

// TrackedMessage represents a message with full lifecycle tracking
type TrackedMessage struct {
	ID             string
	ConversationID string
	Text           string
	Response       string
	
	// State tracking
	CurrentState   queue.MessageState
	StateHistory   []StateTransition
	
	// Timing
	EnqueuedAt     time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	ProcessingTime time.Duration
	
	// Attempts
	Attempts       int
	LastError      error
	ErrorHistory   []ErrorRecord
}

// StateTransition represents a state change with metadata
type StateTransition struct {
	MessageID string
	From      queue.MessageState
	To        queue.MessageState
	Timestamp time.Time
	Reason    string
	Error     error
}

// ErrorRecord tracks an error occurrence
type ErrorRecord struct {
	Timestamp time.Time
	Error     error
	Attempt   int
	State     queue.MessageState
}

// NewMessageTracker creates a new message tracker
func NewMessageTracker() *MessageTracker {
	return &MessageTracker{
		messages:     make(map[string]*TrackedMessage),
		stateChanges: make(chan StateTransition, 1000),
	}
}

// TrackMessage starts tracking a message
func (mt *MessageTracker) TrackMessage(id, conversationID, text string) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	
	mt.messages[id] = &TrackedMessage{
		ID:             id,
		ConversationID: conversationID,
		Text:           text,
		CurrentState:   queue.MessageStateQueued,
		EnqueuedAt:     time.Now(),
		StateHistory:   []StateTransition{},
	}
}

// RecordStateChange records a state transition
func (mt *MessageTracker) RecordStateChange(msgID string, from, to queue.MessageState, reason string, err error) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	
	transition := StateTransition{
		MessageID: msgID,
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Reason:    reason,
		Error:     err,
	}
	
	if msg, ok := mt.messages[msgID]; ok {
		msg.StateHistory = append(msg.StateHistory, transition)
		msg.CurrentState = to
		
		// Update timing
		switch to {
		case queue.MessageStateProcessing:
			now := time.Now()
			msg.StartedAt = &now
		case queue.MessageStateCompleted:
			now := time.Now()
			msg.CompletedAt = &now
			if msg.StartedAt != nil {
				msg.ProcessingTime = now.Sub(*msg.StartedAt)
			}
			atomic.AddInt32(&mt.completed, 1)
		case queue.MessageStateFailed:
			atomic.AddInt32(&mt.failed, 1)
		case queue.MessageStateRetrying:
			atomic.AddInt32(&mt.retrying, 1)
			msg.Attempts++
		}
		
		// Track errors
		if err != nil {
			msg.LastError = err
			msg.ErrorHistory = append(msg.ErrorHistory, ErrorRecord{
				Timestamp: time.Now(),
				Error:     err,
				Attempt:   msg.Attempts,
				State:     to,
			})
		}
	}
	
	// Send to channel for async processing
	select {
	case mt.stateChanges <- transition:
	default:
		// Channel full, drop oldest
		<-mt.stateChanges
		mt.stateChanges <- transition
	}
}

// GetMessage returns tracking data for a message
func (mt *MessageTracker) GetMessage(id string) (*TrackedMessage, bool) {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	msg, ok := mt.messages[id]
	return msg, ok
}

// GetStats returns current statistics
func (mt *MessageTracker) GetStats() MessageStats {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	
	stats := MessageStats{
		Total:      len(mt.messages),
		Completed:  int(atomic.LoadInt32(&mt.completed)),
		Failed:     int(atomic.LoadInt32(&mt.failed)),
		Retrying:   int(atomic.LoadInt32(&mt.retrying)),
		ByState:    make(map[queue.MessageState]int),
	}
	
	for _, msg := range mt.messages {
		stats.ByState[msg.CurrentState]++
	}
	
	return stats
}

// MessageStats provides aggregate statistics
type MessageStats struct {
	Total     int
	Completed int
	Failed    int
	Retrying  int
	ByState   map[queue.MessageState]int
}