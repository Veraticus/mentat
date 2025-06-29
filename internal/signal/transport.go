package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Transport represents the underlying connection to signal-cli.
type Transport interface {
	// Call makes a JSON-RPC call
	Call(ctx context.Context, method string, params any) (*json.RawMessage, error)

	// Subscribe starts receiving notifications
	Subscribe(ctx context.Context) (<-chan *Notification, error)

	// Close closes the transport
	Close() error
}

// Notification represents a JSON-RPC notification.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// MockTransport implements Transport for testing.
type MockTransport struct {
	mu sync.RWMutex

	// Responses to return for specific methods
	responses map[string]*mockResponse

	// Call tracking
	calls map[string][]any

	// Notifications channel
	notifications chan *Notification

	// Closed flag
	closed bool

	// Response delays for testing timeouts
	delays map[string]time.Duration
}

type mockResponse struct {
	result *json.RawMessage
	err    error
}

// NewMockTransport creates a new mock transport.
func NewMockTransport() *MockTransport {
	return &MockTransport{
		responses:     make(map[string]*mockResponse),
		calls:         make(map[string][]any),
		notifications: make(chan *Notification, 100),
		delays:        make(map[string]time.Duration),
	}
}

// SetResponse sets the response for a specific method.
func (m *MockTransport) SetResponse(method string, result *json.RawMessage, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[method] = &mockResponse{
		result: result,
		err:    err,
	}
}

// SetError sets an error response for a specific method.
func (m *MockTransport) SetError(method string, err error) {
	m.SetResponse(method, nil, err)
}

// SetResponseDelay sets a delay for a specific method (for timeout testing).
func (m *MockTransport) SetResponseDelay(method string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delays[method] = delay
}

// GetCalls returns all recorded calls for a specific method.
func (m *MockTransport) GetCalls(method string) []any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calls[method]
}

// Call implements Transport.Call.
func (m *MockTransport) Call(ctx context.Context, method string, params any) (*json.RawMessage, error) {
	m.mu.Lock()
	m.calls[method] = append(m.calls[method], params)
	response := m.responses[method]
	delay := m.delays[method]
	m.mu.Unlock()

	// Simulate delay if configured
	if delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	// Check context before returning
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if response == nil {
		return nil, fmt.Errorf("no mock response configured for method: %s", method)
	}

	return response.result, response.err
}

// Subscribe implements Transport.Subscribe.
func (m *MockTransport) Subscribe(_ context.Context) (<-chan *Notification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("transport is closed")
	}

	return m.notifications, nil
}

// SimulateNotification sends a notification to subscribers.
func (m *MockTransport) SimulateNotification(notif *Notification) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.closed {
		select {
		case m.notifications <- notif:
		default:
			// Channel full, drop notification (in tests this shouldn't happen with buffer of 100)
		}
	}
}

// Close implements Transport.Close.
func (m *MockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.closed {
		m.closed = true
		close(m.notifications)
	}

	return nil
}

// IsClosed returns whether the transport is closed.
func (m *MockTransport) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}