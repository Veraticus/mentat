// Package storage provides persistent storage interfaces.
package storage

import (
	"encoding/json"
	"time"

	"github.com/Veraticus/mentat/internal/queue"
)

// Storage provides persistent state management across all components.
type Storage interface {
	// Message lifecycle
	SaveMessage(msg *StoredMessage) error
	GetMessage(messageID string) (*StoredMessage, error)
	GetConversationHistory(userID string, limit int) ([]*StoredMessage, error)

	// Queue persistence
	SaveQueueItem(item *queue.QueuedMessage) error
	UpdateQueueItemState(itemID string, state queue.MessageState, metadata map[string]any) error
	GetPendingQueueItems() ([]*queue.QueuedMessage, error)
	GetQueueItemHistory(itemID string) ([]*StateTransition, error)

	// LLM interaction tracking
	SaveLLMCall(call *LLMCall) error
	GetLLMCallsForMessage(messageID string) ([]*LLMCall, error)
	GetLLMCostReport(userID string, start, end time.Time) (*CostReport, error)

	// Session management
	SaveSession(session *Session) error
	UpdateSessionActivity(sessionID string, lastActivity time.Time) error
	GetActiveSession(userID string) (*Session, error)
	ExpireSessions(before time.Time) (int, error)

	// System state
	SaveRateLimitState(userID string, state *RateLimitState) error
	GetRateLimitState(userID string) (*RateLimitState, error)
	SaveCircuitBreakerState(service string, state *CircuitBreakerState) error
	GetCircuitBreakerState(service string) (*CircuitBreakerState, error)

	// Analytics and monitoring
	RecordMetric(metric *SystemMetric) error
	GetMetricsReport(start, end time.Time) (*MetricsReport, error)

	// Maintenance
	PurgeOldData(before time.Time, dataType string) (int64, error)
	Vacuum() error
}

// MessageDirection indicates if a message is inbound or outbound.
type MessageDirection string

const (
	// INBOUND indicates a message from a user.
	INBOUND MessageDirection = "INBOUND"
	// OUTBOUND indicates a message to a user.
	OUTBOUND MessageDirection = "OUTBOUND"
)

// StoredMessage represents a persisted message.
type StoredMessage struct {
	ID              string
	ConversationID  string
	From            string
	To              string
	Content         string
	Direction       MessageDirection
	Timestamp       time.Time
	ProcessingState string
	Metadata        json.RawMessage
}

// LLMCall represents a tracked LLM interaction.
type LLMCall struct {
	Timestamp  time.Time
	Prompt     string
	ID         string
	MessageID  string
	SessionID  string
	CallType   string
	Response   string
	Error      string
	ToolCalls  json.RawMessage
	Cost       float64
	TokensUsed int
	Latency    time.Duration
	Success    bool
}

// StateTransition represents a queue item state change.
type StateTransition struct {
	Timestamp time.Time
	ID        string
	ItemID    string
	Reason    string
	Metadata  json.RawMessage
	FromState queue.MessageState
	ToState   queue.MessageState
}

// Session represents a conversation session.
type Session struct {
	StartedAt    time.Time
	LastActivity time.Time
	ExpiresAt    time.Time
	ID           string
	UserID       string
	Metadata     json.RawMessage
	MessageCount int
}

// RateLimitState tracks rate limiting per user.
type RateLimitState struct {
	LastRefill   time.Time
	WindowStart  time.Time
	UserID       string
	Tokens       int
	RequestCount int
}

// CircuitBreakerState tracks service health.
type CircuitBreakerState struct {
	LastFailure time.Time
	LastSuccess time.Time
	NextRetry   time.Time
	Service     string
	State       string
	Failures    int
}

// SystemMetric represents a tracked metric.
type SystemMetric struct {
	Timestamp time.Time
	Tags      map[string]string
	Name      string
	Value     float64
}

// CostReport summarizes LLM costs.
type CostReport struct {
	StartTime  time.Time
	EndTime    time.Time
	ByCallType map[string]float64
	UserID     string
	TotalCost  float64
	CallCount  int
	TokensUsed int
}

// MetricsReport summarizes system metrics.
type MetricsReport struct {
	StartTime      time.Time
	EndTime        time.Time
	TopMetrics     map[string]float64
	MessageCount   int
	SuccessRate    float64
	AverageLatency time.Duration
	QueueDepth     int
	ActiveUsers    int
}
