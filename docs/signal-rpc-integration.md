# Signal-CLI JSON-RPC Integration Guide

This document provides a comprehensive guide for integrating signal-cli's JSON-RPC interface into the Mentat application, following Go idiomatic best practices.

## Table of Contents

1. [Overview](#overview)
2. [Signal-CLI JSON-RPC Protocol](#signal-cli-json-rpc-protocol)
3. [Message Formats](#message-formats)
4. [Go Implementation Design](#go-implementation-design)
5. [Error Handling](#error-handling)
6. [Testing Strategy](#testing-strategy)
7. [Security Considerations](#security-considerations)

## Overview

Signal-cli provides a JSON-RPC 2.0 interface that allows external applications to send and receive Signal messages programmatically. The Mentat application integrates with signal-cli running in daemon mode to provide bidirectional communication with Signal users.

### Key Features

- **Bidirectional Communication**: Send and receive messages, including attachments
- **Multiple Connection Types**: UNIX socket, TCP socket, or HTTP endpoint
- **Automatic Message Reception**: Messages are automatically received as JSON-RPC notifications
- **Multi-Account Support**: Can handle multiple Signal accounts (though Mentat uses single-account mode)
- **Attachment Support**: Full support for images, documents, and other file types

## Signal-CLI JSON-RPC Protocol

### Connection Modes

1. **UNIX Socket (Recommended for Mentat)**
   ```bash
   signal-cli -a +1234567890 daemon --socket /var/run/signal-cli/socket
   ```

2. **TCP Socket**
   ```bash
   signal-cli -a +1234567890 daemon --tcp 127.0.0.1:7583
   ```

3. **HTTP Endpoint**
   ```bash
   signal-cli -a +1234567890 daemon --http 127.0.0.1:8080
   ```

### Protocol Specification

All communication follows JSON-RPC 2.0 specification:

- Each request must be a JSON object on a single line
- Requests require: `jsonrpc: "2.0"`, unique `id`, `method`, optional `params`
- Responses include the matching `id` and either `result` or `error`
- Notifications (like incoming messages) have no `id` field

## Message Formats

### Sending Messages

#### Text Message
```json
{
  "jsonrpc": "2.0",
  "method": "send",
  "params": {
    "recipient": ["+1234567890"],
    "message": "Hello from Mentat!"
  },
  "id": "msg-001"
}
```

#### Message with Single Attachment
```json
{
  "jsonrpc": "2.0",
  "method": "send",
  "params": {
    "recipient": ["+1234567890"],
    "message": "Here's the report you requested",
    "attachment": "/tmp/report.pdf"
  },
  "id": "msg-002"
}
```

#### Message with Multiple Attachments
```json
{
  "jsonrpc": "2.0",
  "method": "send",
  "params": {
    "recipient": ["+1234567890"],
    "message": "Meeting photos",
    "attachments": [
      "/tmp/photo1.jpg",
      "/tmp/photo2.jpg",
      "/tmp/photo3.jpg"
    ]
  },
  "id": "msg-003"
}
```

#### Group Message
```json
{
  "jsonrpc": "2.0",
  "method": "send",
  "params": {
    "groupId": "VGhpcyBpcyBhIGdyb3VwIGlkIGluIGJhc2U2NA==",
    "message": "Team update",
    "attachment": "/tmp/update.docx"
  },
  "id": "msg-004"
}
```

### Sending Typing Indicators
```json
{
  "jsonrpc": "2.0",
  "method": "sendTyping",
  "params": {
    "recipient": ["+1234567890"],
    "stop": false
  },
  "id": "typing-001"
}
```

### Receiving Messages

Incoming messages arrive as JSON-RPC notifications (no `id` field):

#### Standard Message
```json
{
  "jsonrpc": "2.0",
  "method": "receive",
  "params": {
    "envelope": {
      "source": "+1234567890",
      "sourceNumber": "+1234567890",
      "sourceUuid": "abcd1234-5678-90ab-cdef-1234567890ab",
      "sourceName": "John Doe",
      "sourceDevice": 1,
      "timestamp": 1699564800000,
      "dataMessage": {
        "timestamp": 1699564800000,
        "message": "Hey, can you schedule a meeting for tomorrow?",
        "expiresInSeconds": 0,
        "viewOnce": false,
        "mentions": [],
        "attachments": [],
        "contacts": []
      }
    }
  }
}
```

#### Message with Attachments

**Important**: signal-cli automatically downloads attachments to `~/.local/share/signal-cli/attachments/` when messages are received. The attachment metadata in the JSON notification includes information about the file, but not the local path where it was saved.

```json
{
  "jsonrpc": "2.0",
  "method": "receive",
  "params": {
    "envelope": {
      "source": "+1234567890",
      "sourceNumber": "+1234567890",
      "sourceUuid": "abcd1234-5678-90ab-cdef-1234567890ab",
      "sourceName": "Jane Smith",
      "sourceDevice": 1,
      "timestamp": 1699564900000,
      "dataMessage": {
        "timestamp": 1699564900000,
        "message": "Here's my expense receipt",
        "attachments": [
          {
            "contentType": "image/jpeg",
            "filename": "receipt.jpg",
            "id": "attachment-id-123",
            "size": 245789,
            "width": 1024,
            "height": 768,
            "caption": null,
            "uploadTimestamp": 1699564890000
          }
        ]
      }
    }
  }
}
```

To access the downloaded attachment file, you'll need to:
1. Use the attachment ID to locate the file in the attachments directory
2. Or configure signal-cli with `--no-receive-attachments` and implement your own download logic
3. Or monitor the attachments directory for new files based on timestamp

#### Typing Indicator
```json
{
  "jsonrpc": "2.0",
  "method": "receive",
  "params": {
    "envelope": {
      "source": "+1234567890",
      "sourceNumber": "+1234567890",
      "sourceUuid": "abcd1234-5678-90ab-cdef-1234567890ab",
      "sourceName": "John Doe",
      "sourceDevice": 1,
      "timestamp": 1699564850000,
      "typingMessage": {
        "action": "STARTED",
        "timestamp": 1699564850000
      }
    }
  }
}
```

#### Read Receipt
```json
{
  "jsonrpc": "2.0",
  "method": "receive",
  "params": {
    "envelope": {
      "source": "+1234567890",
      "timestamp": 1699565000000,
      "receiptMessage": {
        "when": 1699564800000,
        "isDelivery": false,
        "isRead": true,
        "isViewed": false,
        "timestamps": [1699564800000]
      }
    }
  }
}
```

### Response Formats

#### Success Response
```json
{
  "jsonrpc": "2.0",
  "result": {
    "timestamp": 1699565100000
  },
  "id": "msg-001"
}
```

#### Error Response
```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32001,
    "message": "Failed to send message",
    "data": {
      "exception": "org.whispersystems.signalservice.api.push.exceptions.UnregisteredUserException",
      "message": "Unregistered user"
    }
  },
  "id": "msg-001"
}
```

## Go Implementation Design

### Core Interfaces

```go
// Package signal provides Signal messenger integration via signal-cli JSON-RPC
package signal

import (
    "context"
    "encoding/json"
    "time"
)

// Client represents a Signal client that communicates via JSON-RPC
type Client interface {
    // Send sends a message to one or more recipients
    Send(ctx context.Context, req *SendRequest) (*SendResponse, error)
    
    // SendTypingIndicator sends a typing indicator
    SendTypingIndicator(ctx context.Context, recipient string, stop bool) error
    
    // Subscribe starts receiving incoming messages
    Subscribe(ctx context.Context) (<-chan *Envelope, error)
    
    // Close closes the client connection
    Close() error
}

// Transport represents the underlying connection to signal-cli
type Transport interface {
    // Call makes a JSON-RPC call
    Call(ctx context.Context, method string, params interface{}) (*json.RawMessage, error)
    
    // Subscribe starts receiving notifications
    Subscribe(ctx context.Context) (<-chan *Notification, error)
    
    // Close closes the transport
    Close() error
}

// SendRequest represents a request to send a message
type SendRequest struct {
    Recipients  []string     `json:"recipient,omitempty"`
    GroupID     string       `json:"groupId,omitempty"`
    Message     string       `json:"message"`
    Attachments []string     `json:"attachments,omitempty"`
    Mentions    []Mention    `json:"mentions,omitempty"`
}

// Envelope represents an incoming message envelope
type Envelope struct {
    Source       string       `json:"source"`
    SourceNumber string       `json:"sourceNumber"`
    SourceUUID   string       `json:"sourceUuid"`
    SourceName   string       `json:"sourceName"`
    SourceDevice int          `json:"sourceDevice"`
    Timestamp    int64        `json:"timestamp"`
    
    // Message types (only one will be non-nil)
    DataMessage    *DataMessage    `json:"dataMessage,omitempty"`
    SyncMessage    *SyncMessage    `json:"syncMessage,omitempty"`
    TypingMessage  *TypingMessage  `json:"typingMessage,omitempty"`
    ReceiptMessage *ReceiptMessage `json:"receiptMessage,omitempty"`
}

// DataMessage represents a standard message
type DataMessage struct {
    Timestamp        int64        `json:"timestamp"`
    Message          string       `json:"message"`
    ExpiresInSeconds int          `json:"expiresInSeconds"`
    ViewOnce         bool         `json:"viewOnce"`
    Attachments      []Attachment `json:"attachments"`
    Mentions         []Mention    `json:"mentions"`
    Contacts         []Contact    `json:"contacts"`
    GroupInfo        *GroupInfo   `json:"groupInfo,omitempty"`
}

// Attachment represents a file attachment
type Attachment struct {
    ContentType     string `json:"contentType"`
    Filename        string `json:"filename"`
    ID              string `json:"id"`
    Size            int64  `json:"size"`
    Width           int    `json:"width,omitempty"`
    Height          int    `json:"height,omitempty"`
    Caption         string `json:"caption,omitempty"`
    UploadTimestamp int64  `json:"uploadTimestamp"`
}

// GetLocalPath returns the likely local path for a downloaded attachment
// Note: This is based on signal-cli's default behavior
func (a *Attachment) GetLocalPath(dataDir string) string {
    if dataDir == "" {
        dataDir = filepath.Join(os.Getenv("HOME"), ".local/share/signal-cli")
    }
    // Attachments are stored by ID in the attachments directory
    return filepath.Join(dataDir, "attachments", a.ID)
}
```

### Transport Implementations

#### UNIX Socket Transport

```go
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

// UnixSocketTransport implements Transport using a UNIX socket
type UnixSocketTransport struct {
    socketPath string
    conn       net.Conn
    
    // Request tracking
    requestID  atomic.Uint64
    pending    map[string]chan *rpcResponse
    pendingMu  sync.Mutex
    
    // Notification handling
    notifications chan *Notification
    
    // Lifecycle
    ctx    context.Context
    cancel context.CancelFunc
    done   chan struct{}
}

// NewUnixSocketTransport creates a new UNIX socket transport
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

// Call implements Transport.Call
func (t *UnixSocketTransport) Call(ctx context.Context, method string, params interface{}) (*json.RawMessage, error) {
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

// readLoop continuously reads from the socket
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
        // Log error but don't crash
        // In production, use proper logging
    }
}

// Subscribe implements Transport.Subscribe
func (t *UnixSocketTransport) Subscribe(ctx context.Context) (<-chan *Notification, error) {
    return t.notifications, nil
}

// Close implements Transport.Close
func (t *UnixSocketTransport) Close() error {
    t.cancel()
    err := t.conn.Close()
    <-t.done
    return err
}

// Internal types
type rpcRequest struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      string      `json:"id"`
    Method  string      `json:"method"`
    Params  interface{} `json:"params,omitempty"`
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

// Notification represents a JSON-RPC notification
type Notification struct {
    JSONRPC string          `json:"jsonrpc"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params"`
}
```

### Signal Client Implementation

```go
package signal

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
)

// client implements the Client interface
type client struct {
    transport Transport
    account   string // Optional account for multi-account mode
}

// NewClient creates a new Signal client
func NewClient(transport Transport, opts ...ClientOption) Client {
    c := &client{
        transport: transport,
    }
    
    for _, opt := range opts {
        opt(c)
    }
    
    return c
}

// ClientOption configures the client
type ClientOption func(*client)

// WithAccount sets the account for multi-account mode
func WithAccount(account string) ClientOption {
    return func(c *client) {
        c.account = account
    }
}

// Send implements Client.Send
func (c *client) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
    params := make(map[string]interface{})
    
    // Add account if in multi-account mode
    if c.account != "" {
        params["account"] = c.account
    }
    
    // Add recipients or group
    if len(req.Recipients) > 0 {
        params["recipient"] = req.Recipients
    } else if req.GroupID != "" {
        params["groupId"] = req.GroupID
    } else {
        return nil, fmt.Errorf("either recipients or groupId must be specified")
    }
    
    // Add message
    params["message"] = req.Message
    
    // Add attachments
    if len(req.Attachments) == 1 {
        params["attachment"] = req.Attachments[0]
    } else if len(req.Attachments) > 1 {
        params["attachments"] = req.Attachments
    }
    
    // Add mentions if any
    if len(req.Mentions) > 0 {
        params["mentions"] = req.Mentions
    }
    
    // Make the call
    result, err := c.transport.Call(ctx, "send", params)
    if err != nil {
        return nil, fmt.Errorf("send failed: %w", err)
    }
    
    // Parse response
    var resp SendResponse
    if err := json.Unmarshal(*result, &resp); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }
    
    return &resp, nil
}

// SendTypingIndicator implements Client.SendTypingIndicator
func (c *client) SendTypingIndicator(ctx context.Context, recipient string, stop bool) error {
    params := map[string]interface{}{
        "recipient": []string{recipient},
        "stop":      stop,
    }
    
    if c.account != "" {
        params["account"] = c.account
    }
    
    _, err := c.transport.Call(ctx, "sendTyping", params)
    return err
}

// Subscribe implements Client.Subscribe
func (c *client) Subscribe(ctx context.Context) (<-chan *Envelope, error) {
    // Get notification channel from transport
    notifications, err := c.transport.Subscribe(ctx)
    if err != nil {
        return nil, err
    }
    
    // Create envelope channel
    envelopes := make(chan *Envelope, 10)
    
    // Start processing notifications
    go func() {
        defer close(envelopes)
        
        for {
            select {
            case <-ctx.Done():
                return
            case notif, ok := <-notifications:
                if !ok {
                    return
                }
                
                // Only process "receive" notifications
                if notif.Method != "receive" {
                    continue
                }
                
                // Parse params
                var params struct {
                    Envelope *Envelope `json:"envelope"`
                }
                
                if err := json.Unmarshal(notif.Params, &params); err != nil {
                    // Log error in production
                    continue
                }
                
                if params.Envelope != nil {
                    select {
                    case envelopes <- params.Envelope:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }
    }()
    
    return envelopes, nil
}

// Close implements Client.Close
func (c *client) Close() error {
    return c.transport.Close()
}

// SendResponse represents the response from a send operation
type SendResponse struct {
    Timestamp int64 `json:"timestamp"`
}
```

### Mock Implementation for Testing

```go
package signal

import (
    "context"
    "sync"
)

// MockClient implements Client for testing
type MockClient struct {
    mu sync.Mutex
    
    // Sent messages
    SentMessages []SentMessage
    
    // Typing indicators
    TypingIndicators []TypingIndicator
    
    // Incoming messages to simulate
    incomingMessages chan *Envelope
    
    // Error to return
    SendError error
    
    // Closed flag
    closed bool
}

// SentMessage records a sent message
type SentMessage struct {
    Recipients  []string
    GroupID     string
    Message     string
    Attachments []string
}

// TypingIndicator records a typing indicator
type TypingIndicator struct {
    Recipient string
    Stop      bool
}

// NewMockClient creates a new mock client
func NewMockClient() *MockClient {
    return &MockClient{
        incomingMessages: make(chan *Envelope, 100),
    }
}

// Send implements Client.Send
func (m *MockClient) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if m.SendError != nil {
        return nil, m.SendError
    }
    
    m.SentMessages = append(m.SentMessages, SentMessage{
        Recipients:  req.Recipients,
        GroupID:     req.GroupID,
        Message:     req.Message,
        Attachments: req.Attachments,
    })
    
    return &SendResponse{
        Timestamp: time.Now().UnixMilli(),
    }, nil
}

// SendTypingIndicator implements Client.SendTypingIndicator
func (m *MockClient) SendTypingIndicator(ctx context.Context, recipient string, stop bool) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.TypingIndicators = append(m.TypingIndicators, TypingIndicator{
        Recipient: recipient,
        Stop:      stop,
    })
    
    return nil
}

// Subscribe implements Client.Subscribe
func (m *MockClient) Subscribe(ctx context.Context) (<-chan *Envelope, error) {
    return m.incomingMessages, nil
}

// SimulateIncomingMessage simulates an incoming message
func (m *MockClient) SimulateIncomingMessage(msg *Envelope) {
    m.incomingMessages <- msg
}

// Close implements Client.Close
func (m *MockClient) Close() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if !m.closed {
        m.closed = true
        close(m.incomingMessages)
    }
    
    return nil
}
```

## Error Handling

### Error Types

```go
// RPCError represents a JSON-RPC error
type RPCError struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
    return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Common error codes
const (
    ErrorCodeInvalidRequest  = -32600
    ErrorCodeMethodNotFound  = -32601
    ErrorCodeInvalidParams   = -32602
    ErrorCodeInternalError   = -32603
    
    // Signal-specific errors
    ErrorCodeUnregisteredUser = -32001
    ErrorCodeRateLimited      = -32002
    ErrorCodeNetworkFailure   = -32003
)

// IsUnregisteredUserError checks if the error is due to an unregistered user
func IsUnregisteredUserError(err error) bool {
    var rpcErr *RPCError
    if errors.As(err, &rpcErr) {
        return rpcErr.Code == ErrorCodeUnregisteredUser
    }
    return false
}

// IsRateLimitError checks if the error is due to rate limiting
func IsRateLimitError(err error) bool {
    var rpcErr *RPCError
    if errors.As(err, &rpcErr) {
        return rpcErr.Code == ErrorCodeRateLimited
    }
    return false
}
```

### Retry Logic

```go
// RetryableClient wraps a Client with retry logic
type RetryableClient struct {
    client     Client
    maxRetries int
    backoff    BackoffStrategy
}

// BackoffStrategy defines how to calculate retry delays
type BackoffStrategy interface {
    NextDelay(attempt int) time.Duration
}

// ExponentialBackoff implements exponential backoff
type ExponentialBackoff struct {
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Multiplier   float64
}

func (e *ExponentialBackoff) NextDelay(attempt int) time.Duration {
    delay := float64(e.InitialDelay) * math.Pow(e.Multiplier, float64(attempt-1))
    if delay > float64(e.MaxDelay) {
        return e.MaxDelay
    }
    return time.Duration(delay)
}

// Send with retry logic
func (r *RetryableClient) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
    var lastErr error
    
    for attempt := 1; attempt <= r.maxRetries; attempt++ {
        resp, err := r.client.Send(ctx, req)
        if err == nil {
            return resp, nil
        }
        
        lastErr = err
        
        // Don't retry on certain errors
        if IsUnregisteredUserError(err) {
            return nil, err
        }
        
        // Check if we should retry
        if attempt < r.maxRetries {
            delay := r.backoff.NextDelay(attempt)
            
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
                // Continue to next attempt
            }
        }
    }
    
    return nil, fmt.Errorf("failed after %d attempts: %w", r.maxRetries, lastErr)
}
```

## Testing Strategy

### Unit Tests

```go
package signal_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    "mentat/internal/signal"
)

func TestClient_Send(t *testing.T) {
    tests := []struct {
        name    string
        request *signal.SendRequest
        wantErr bool
    }{
        {
            name: "send text message",
            request: &signal.SendRequest{
                Recipients: []string{"+1234567890"},
                Message:    "Hello, World!",
            },
            wantErr: false,
        },
        {
            name: "send with attachment",
            request: &signal.SendRequest{
                Recipients:  []string{"+1234567890"},
                Message:     "Check this out",
                Attachments: []string{"/tmp/photo.jpg"},
            },
            wantErr: false,
        },
        {
            name: "send to group",
            request: &signal.SendRequest{
                GroupID: "group123",
                Message: "Team update",
            },
            wantErr: false,
        },
        {
            name: "missing recipient and group",
            request: &signal.SendRequest{
                Message: "Orphan message",
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create mock client
            client := signal.NewMockClient()
            
            // Send message
            ctx := context.Background()
            resp, err := client.Send(ctx, tt.request)
            
            // Check error
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.NotZero(t, resp.Timestamp)
            
            // Verify message was recorded
            assert.Len(t, client.SentMessages, 1)
            sent := client.SentMessages[0]
            assert.Equal(t, tt.request.Recipients, sent.Recipients)
            assert.Equal(t, tt.request.Message, sent.Message)
        })
    }
}

func TestClient_Subscribe(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Create mock client
    client := signal.NewMockClient()
    
    // Subscribe
    envelopes, err := client.Subscribe(ctx)
    require.NoError(t, err)
    
    // Simulate incoming message
    testEnvelope := &signal.Envelope{
        Source:       "+9876543210",
        SourceNumber: "+9876543210",
        SourceName:   "Test User",
        Timestamp:    time.Now().UnixMilli(),
        DataMessage: &signal.DataMessage{
            Message: "Test message",
        },
    }
    
    client.SimulateIncomingMessage(testEnvelope)
    
    // Receive message
    select {
    case env := <-envelopes:
        assert.Equal(t, testEnvelope.Source, env.Source)
        assert.Equal(t, testEnvelope.DataMessage.Message, env.DataMessage.Message)
    case <-ctx.Done():
        t.Fatal("Timeout waiting for message")
    }
}

func TestClient_TypingIndicator(t *testing.T) {
    client := signal.NewMockClient()
    
    // Send typing start
    err := client.SendTypingIndicator(context.Background(), "+1234567890", false)
    require.NoError(t, err)
    
    // Send typing stop
    err = client.SendTypingIndicator(context.Background(), "+1234567890", true)
    require.NoError(t, err)
    
    // Verify
    assert.Len(t, client.TypingIndicators, 2)
    assert.False(t, client.TypingIndicators[0].Stop)
    assert.True(t, client.TypingIndicators[1].Stop)
}
```

### Integration Tests

```go
// +build integration

package signal_test

import (
    "context"
    "os"
    "testing"
    "time"
    
    "github.com/stretchr/testify/require"
    
    "mentat/internal/signal"
)

func TestIntegration_RealSignalCLI(t *testing.T) {
    // Skip if not in integration mode
    if os.Getenv("SIGNAL_TEST_SOCKET") == "" {
        t.Skip("SIGNAL_TEST_SOCKET not set")
    }
    
    // Create real transport
    transport, err := signal.NewUnixSocketTransport(os.Getenv("SIGNAL_TEST_SOCKET"))
    require.NoError(t, err)
    defer transport.Close()
    
    // Create client
    client := signal.NewClient(transport)
    
    // Test sending a message
    ctx := context.Background()
    resp, err := client.Send(ctx, &signal.SendRequest{
        Recipients: []string{os.Getenv("SIGNAL_TEST_RECIPIENT")},
        Message:    "Integration test message",
    })
    
    require.NoError(t, err)
    require.NotZero(t, resp.Timestamp)
}
```

## Attachment Handling Details

### How Attachments Work

When signal-cli receives a message with attachments:

1. **Automatic Download**: By default, signal-cli automatically downloads all attachments to `~/.local/share/signal-cli/attachments/`
2. **Storage by ID**: Files are stored using their attachment ID (not the original filename)
3. **Metadata in JSON**: The JSON notification includes metadata about the attachment but not the local file path

### Processing Received Attachments

```go
// ProcessReceivedAttachment handles an attachment from an incoming message
func ProcessReceivedAttachment(attachment *Attachment, dataDir string) (string, error) {
    // Get the local path where signal-cli saved the file
    localPath := attachment.GetLocalPath(dataDir)
    
    // Verify the file exists
    if _, err := os.Stat(localPath); os.IsNotExist(err) {
        return "", fmt.Errorf("attachment file not found: %s", localPath)
    }
    
    // For Mentat, we might want to:
    // 1. Move the file to a permanent location
    // 2. Rename it to include the original filename
    // 3. Store metadata in the database
    
    // Example: Copy to Mentat's attachment directory with meaningful name
    mentatDir := "/var/lib/mentat/attachments"
    timestamp := time.Now().Format("2006-01-02-150405")
    ext := filepath.Ext(attachment.Filename)
    if ext == "" {
        // Guess extension from content type
        ext = getExtensionFromContentType(attachment.ContentType)
    }
    
    newPath := filepath.Join(mentatDir, fmt.Sprintf("%s-%s%s", timestamp, attachment.ID[:8], ext))
    
    if err := copyFile(localPath, newPath); err != nil {
        return "", fmt.Errorf("failed to copy attachment: %w", err)
    }
    
    return newPath, nil
}

func getExtensionFromContentType(contentType string) string {
    switch contentType {
    case "image/jpeg":
        return ".jpg"
    case "image/png":
        return ".png"
    case "image/gif":
        return ".gif"
    case "application/pdf":
        return ".pdf"
    case "video/mp4":
        return ".mp4"
    default:
        return ""
    }
}
```

### Configuring Attachment Behavior

You can control signal-cli's attachment handling:

```bash
# Disable automatic download
signal-cli -a +1234567890 daemon --socket /var/run/signal-cli/socket --no-receive-attachments

# With custom data directory
signal-cli --config /var/lib/mentat/signal-data -a +1234567890 daemon --socket
```

### Sending Attachments

For sending, signal-cli requires absolute paths to files:

```go
// PrepareAttachmentForSending ensures an attachment is ready to send
func PrepareAttachmentForSending(filePath string) (string, error) {
    // Ensure absolute path
    absPath, err := filepath.Abs(filePath)
    if err != nil {
        return "", fmt.Errorf("failed to get absolute path: %w", err)
    }
    
    // Verify file exists and is readable
    info, err := os.Stat(absPath)
    if err != nil {
        return "", fmt.Errorf("cannot access file: %w", err)
    }
    
    // Check size limits (Signal has a ~100MB limit)
    if info.Size() > 100*1024*1024 {
        return "", fmt.Errorf("file too large: %d bytes", info.Size())
    }
    
    // For images, you might want to:
    // 1. Resize if too large
    // 2. Convert to a supported format
    // 3. Generate a thumbnail for preview
    
    return absPath, nil
}
```

## Security Considerations

### Input Validation

```go
// ValidateSendRequest validates a send request
func ValidateSendRequest(req *SendRequest) error {
    // Check recipients or group
    if len(req.Recipients) == 0 && req.GroupID == "" {
        return fmt.Errorf("either recipients or groupId must be specified")
    }
    
    // Validate phone numbers
    for _, recipient := range req.Recipients {
        if !isValidPhoneNumber(recipient) {
            return fmt.Errorf("invalid phone number: %s", recipient)
        }
    }
    
    // Check message length
    if len(req.Message) > 5000 {
        return fmt.Errorf("message too long: %d characters (max 5000)", len(req.Message))
    }
    
    // Validate attachments
    for _, path := range req.Attachments {
        if err := validateAttachmentPath(path); err != nil {
            return fmt.Errorf("invalid attachment: %w", err)
        }
    }
    
    return nil
}

func isValidPhoneNumber(number string) bool {
    // Must start with + and contain only digits
    if len(number) < 2 || number[0] != '+' {
        return false
    }
    
    for _, r := range number[1:] {
        if r < '0' || r > '9' {
            return false
        }
    }
    
    return true
}

func validateAttachmentPath(path string) error {
    // Ensure absolute path
    if !filepath.IsAbs(path) {
        return fmt.Errorf("attachment path must be absolute")
    }
    
    // Check file exists and is readable
    info, err := os.Stat(path)
    if err != nil {
        return fmt.Errorf("cannot access attachment: %w", err)
    }
    
    // Check file size
    if info.Size() > 100*1024*1024 { // 100MB limit
        return fmt.Errorf("attachment too large: %d bytes", info.Size())
    }
    
    // Check file type
    if !isAllowedFileType(path) {
        return fmt.Errorf("file type not allowed")
    }
    
    return nil
}
```

### Rate Limiting

```go
// RateLimitedClient wraps a Client with rate limiting
type RateLimitedClient struct {
    client  Client
    limiter *rate.Limiter
}

// NewRateLimitedClient creates a rate-limited client
func NewRateLimitedClient(client Client, rps float64) *RateLimitedClient {
    return &RateLimitedClient{
        client:  client,
        limiter: rate.NewLimiter(rate.Limit(rps), int(rps)),
    }
}

// Send implements Client.Send with rate limiting
func (r *RateLimitedClient) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
    if err := r.limiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limit exceeded: %w", err)
    }
    
    return r.client.Send(ctx, req)
}
```

### Audit Logging

```go
// AuditedClient wraps a Client with audit logging
type AuditedClient struct {
    client  Client
    storage Storage
}

// Send with audit logging
func (a *AuditedClient) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
    start := time.Now()
    
    // Call underlying client
    resp, err := a.client.Send(ctx, req)
    
    // Log the attempt
    audit := &AuditLog{
        Timestamp:   start,
        Method:      "send",
        Recipients:  req.Recipients,
        GroupID:     req.GroupID,
        MessageSize: len(req.Message),
        Attachments: len(req.Attachments),
        Success:     err == nil,
        Duration:    time.Since(start),
    }
    
    if err != nil {
        audit.Error = err.Error()
    }
    
    // Save audit log (don't fail the request if logging fails)
    go a.storage.SaveAuditLog(audit)
    
    return resp, err
}
```

## Best Practices

1. **Always use context for cancellation**: All operations should accept a context to support timeouts and cancellation.

2. **Handle connection failures gracefully**: Implement reconnection logic for long-running connections.

3. **Validate all inputs**: Never trust external input, especially file paths and phone numbers.

4. **Rate limit outgoing messages**: Respect Signal's rate limits to avoid getting blocked.

5. **Log but don't leak**: Log enough for debugging but avoid logging message contents or personal information.

6. **Test with mocks**: Use the mock client for unit tests to avoid depending on signal-cli.

7. **Handle all message types**: Even if you only care about text messages, handle other types gracefully.

8. **Implement proper shutdown**: Ensure all resources are cleaned up on shutdown.

## Existing Go Libraries

During research, we found several Go libraries related to Signal:

1. **[DerLukas15/signalgo](https://github.com/DerLukas15/signalgo)**: A Go interface for signal-cli, but primarily uses DBus (which is experimental) rather than JSON-RPC over Unix sockets.

2. **[signal-golang/textsecure](https://github.com/signal-golang/textsecure)**: A native Go implementation of the Signal protocol, doesn't use signal-cli.

3. **[ybbus/jsonrpc](https://github.com/ybbus/jsonrpc)**: A general-purpose JSON-RPC 2.0 client that could be adapted for signal-cli communication.

None of these libraries provide exactly what Mentat needs (JSON-RPC over Unix socket to signal-cli), so we're implementing our own client following the design in this document.

## Conclusion

This integration provides a robust, testable, and idiomatic Go implementation for interacting with signal-cli's JSON-RPC interface. The design follows Go best practices with clear interfaces, proper error handling, and comprehensive testing support.

The implementation is specifically tailored for Mentat's needs:
- Unix socket transport for reliable local communication
- Full support for attachments and group messages
- Comprehensive error handling and retry logic
- Mock implementations for testing
- Security measures including input validation and rate limiting