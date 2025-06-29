package signal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// UnixSocketTransport implements Transport using a UNIX socket.
type UnixSocketTransport struct {
	socketPath string
	conn       net.Conn

	// Request tracking
	requestID atomic.Uint64
	pending   map[string]chan *rpcResponse
	pendingMu sync.Mutex

	// Notification handling
	notifications chan *Notification

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewUnixSocketTransport creates a new UNIX socket transport.
func NewUnixSocketTransport(socketPath string) (*UnixSocketTransport, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to signal-cli socket: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	t := &UnixSocketTransport{
		socketPath:    socketPath,
		conn:          conn,
		pending:       make(map[string]chan *rpcResponse),
		notifications: make(chan *Notification, 100),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}

	// Start reading loop
	go t.readLoop()

	return t, nil
}

// Call implements Transport.Call.
func (t *UnixSocketTransport) Call(ctx context.Context, method string, params any) (*json.RawMessage, error) {
	// Generate unique ID
	id := fmt.Sprintf("req-%d", t.requestID.Add(1))

	// Create request
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create response channel
	respChan := make(chan *rpcResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = respChan
	t.pendingMu.Unlock()

	// Clean up on exit
	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
	}()

	// Send request
	if _, err := fmt.Fprintf(t.conn, "%s\n", data); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, &RPCError{
				Code:    resp.Error.Code,
				Message: resp.Error.Message,
				Data:    resp.Error.Data,
			}
		}
		return resp.Result, nil
	}
}

// readLoop continuously reads from the socket.
func (t *UnixSocketTransport) readLoop() {
	defer close(t.done)

	scanner := bufio.NewScanner(t.conn)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max

	for scanner.Scan() {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		line := scanner.Bytes()

		// Try to parse as response first
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err == nil && resp.ID != "" {
			// This is a response
			t.pendingMu.Lock()
			if ch, ok := t.pending[resp.ID]; ok {
				ch <- &resp
			}
			t.pendingMu.Unlock()
			continue
		}

		// Try to parse as notification
		var notif Notification
		if err := json.Unmarshal(line, &notif); err == nil && notif.Method != "" {
			select {
			case t.notifications <- &notif:
			case <-t.ctx.Done():
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// Log error but don't crash - in production, use proper logging
		_ = err
	}
}

// Subscribe implements Transport.Subscribe.
func (t *UnixSocketTransport) Subscribe(_ context.Context) (<-chan *Notification, error) {
	return t.notifications, nil
}

// Close implements Transport.Close.
func (t *UnixSocketTransport) Close() error {
	t.cancel()
	err := t.conn.Close()
	<-t.done
	return err
}

// Internal types.
type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      string           `json:"id"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}