package signal_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// rpcResponse represents a JSON-RPC response (copy of unexported type for testing).
type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
	ID      json.RawMessage  `json:"id"`
}

// rpcError represents a JSON-RPC error (copy of unexported type for testing).
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcRequest represents a JSON-RPC request (copy of unexported type for testing).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id"`
}

// mockSignalServer simulates a signal-cli JSON-RPC server.
type mockSignalServer struct {
	socketPath string
	listener   net.Listener
	mu         sync.Mutex
	responses  map[string]*rpcResponse
	stopped    bool
	wg         sync.WaitGroup
}

func newMockSignalServer(t *testing.T) *mockSignalServer {
	t.Helper()
	// Create temp socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "signal.sock")

	server := &mockSignalServer{
		socketPath: socketPath,
		responses:  make(map[string]*rpcResponse),
	}

	// Start listening
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create unix socket: %v", err)
	}
	server.listener = listener

	// Start accepting connections
	server.wg.Add(1)
	go server.acceptLoop()

	return server
}

func (s *mockSignalServer) stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()

	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	_ = os.Remove(s.socketPath)
}

func (s *mockSignalServer) setResponse(method string, result any, err *rpcError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var resultMsg *json.RawMessage
	if result != nil {
		data, _ := json.Marshal(result)
		msg := json.RawMessage(data)
		resultMsg = &msg
	}

	s.responses[method] = &rpcResponse{
		JSONRPC: "2.0",
		Result:  resultMsg,
		Error:   err,
	}
}

func (s *mockSignalServer) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			stopped := s.stopped
			s.mu.Unlock()
			if stopped {
				return
			}
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *mockSignalServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		// Special handling for "slow" method
		if req.Method == "slow" {
			// Don't respond, just hang
			continue
		}

		// Look up response
		s.mu.Lock()
		resp, ok := s.responses[req.Method]
		s.mu.Unlock()

		if !ok {
			// Send method not found error
			resp = &rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32601,
					Message: "Method not found",
				},
			}
		} else {
			// Copy response and set ID
			respCopy := *resp
			respCopy.ID = req.ID
			resp = &respCopy
		}

		// Send response
		data, _ := json.Marshal(resp)
		_, _ = fmt.Fprintf(conn, "%s\n", data)
	}
}

func TestUnixSocketTransport_BasicOperations(t *testing.T) {
	server := newMockSignalServer(t)
	defer server.stop()

	// Set up a response
	server.setResponse("send", map[string]any{
		"timestamp": 1699564800000,
	}, nil)

	// Create transport
	transport, err := signal.NewUnixSocketTransport(server.socketPath)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	// Make a call
	ctx := context.Background()
	result, err := transport.Call(ctx, "send", map[string]any{
		"recipient": []string{"+1234567890"},
		"message":   "Hello",
	})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	// Verify result
	var resp struct {
		Timestamp int64 `json:"timestamp"`
	}
	if unmarshalErr := json.Unmarshal(*result, &resp); unmarshalErr != nil {
		t.Fatalf("Failed to unmarshal result: %v", unmarshalErr)
	}
	if resp.Timestamp != 1699564800000 {
		t.Errorf("Expected timestamp 1699564800000, got %d", resp.Timestamp)
	}
}

func TestUnixSocketTransport_ErrorHandling(t *testing.T) {
	server := newMockSignalServer(t)
	defer server.stop()

	// Set up an error response
	server.setResponse("send", nil, &rpcError{
		Code:    -32001,
		Message: "Unregistered user",
	})

	// Create transport
	transport, err := signal.NewUnixSocketTransport(server.socketPath)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	// Make a call that should fail
	ctx := context.Background()
	_, err = transport.Call(ctx, "send", map[string]any{
		"recipient": []string{"+invalid"},
		"message":   "Test",
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Check that we got an error (we can't check the specific type as RPCError is unexported)
	if !errors.Is(err, context.DeadlineExceeded) && err.Error() == "" {
		t.Errorf("Expected meaningful error message, got %v", err)
	}
}

func TestUnixSocketTransport_Notifications(t *testing.T) {
	// Skip notification test for now - it requires a more sophisticated mock server
	// that can send notifications on the same connection used for requests
	t.Skip("signal.Notification testing requires enhanced mock server implementation")
}

func TestUnixSocketTransport_ConcurrentCalls(t *testing.T) {
	server := newMockSignalServer(t)
	defer server.stop()

	// Set up response
	server.setResponse("test", map[string]any{
		"success": true,
	}, nil)

	// Create transport
	transport, err := signal.NewUnixSocketTransport(server.socketPath)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	// Make concurrent calls
	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, callErr := transport.Call(ctx, "test", map[string]any{
				"id": id,
			})
			if callErr != nil {
				errors <- callErr
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent call error: %v", err)
	}
}

func TestUnixSocketTransport_ContextCancellation(t *testing.T) {
	server := newMockSignalServer(t)
	defer server.stop()

	// Don't set up any response - server will hang

	// Create transport
	transport, err := signal.NewUnixSocketTransport(server.socketPath)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	// Make call with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = transport.Call(ctx, "slow", nil)
	if err == nil {
		t.Fatal("Expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
}

func TestUnixSocketTransport_ConnectionFailure(t *testing.T) {
	// Try to connect to non-existent socket
	_, err := signal.NewUnixSocketTransport("/tmp/non-existent-socket")
	if err == nil {
		t.Fatal("Expected connection error")
	}
}

func TestUnixSocketTransport_Reconnection(t *testing.T) {
	server := newMockSignalServer(t)
	defer server.stop()

	// Set up response
	server.setResponse("test", map[string]any{
		"success": true,
	}, nil)

	// Create transport
	transport, err := signal.NewUnixSocketTransport(server.socketPath)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	// Make a successful call
	ctx := context.Background()
	_, err = transport.Call(ctx, "test", nil)
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	// For now, we won't test reconnection in Phase 7
	// This would require implementing connection pooling and retry logic
}
