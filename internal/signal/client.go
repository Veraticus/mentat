package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Client represents a Signal client that communicates via JSON-RPC.
type Client interface {
	// Send sends a message to one or more recipients
	Send(ctx context.Context, req *SendRequest) (*SendResponse, error)

	// SendTypingIndicator sends a typing indicator
	SendTypingIndicator(ctx context.Context, recipient string, stop bool) error

	// SendReceipt sends a read or viewed receipt for a message
	SendReceipt(ctx context.Context, recipient string, timestamp int64, receiptType string) error

	// Subscribe starts receiving incoming messages
	Subscribe(ctx context.Context) (<-chan *Envelope, error)

	// Close closes the client connection
	Close() error
}

// SendRequest represents a request to send a message.
type SendRequest struct {
	Recipients  []string  `json:"recipient,omitempty"`
	GroupID     string    `json:"groupId,omitempty"`
	Message     string    `json:"message"`
	Attachments []string  `json:"attachments,omitempty"`
	Mentions    []Mention `json:"mentions,omitempty"`
}

// SendResponse represents the response from a send operation.
type SendResponse struct {
	Timestamp int64 `json:"timestamp"`
}

// Mention represents a mention in a message.
type Mention struct {
	Start  int    `json:"start"`
	Length int    `json:"length"`
	Author string `json:"author"`
}

// Envelope represents an incoming message envelope.
type Envelope struct {
	Source       string `json:"source"`
	SourceNumber string `json:"sourceNumber"`
	SourceUUID   string `json:"sourceUuid"`
	SourceName   string `json:"sourceName"`
	SourceDevice int    `json:"sourceDevice"`
	Timestamp    int64  `json:"timestamp"`

	// Message types (only one will be non-nil)
	DataMessage    *DataMessage    `json:"dataMessage,omitempty"`
	SyncMessage    *SyncMessage    `json:"syncMessage,omitempty"`
	TypingMessage  *TypingMessage  `json:"typingMessage,omitempty"`
	ReceiptMessage *ReceiptMessage `json:"receiptMessage,omitempty"`
}

// DataMessage represents a standard message.
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

// Attachment represents a file attachment.
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

// SyncMessage represents a sync message.
type SyncMessage struct {
	SentMessage *SentSyncMessage `json:"sentMessage,omitempty"`
}

// SentSyncMessage represents a sent sync message.
type SentSyncMessage struct {
	Destination string       `json:"destination"`
	Timestamp   int64        `json:"timestamp"`
	Message     *DataMessage `json:"message,omitempty"`
}

// TypingMessage represents a typing indicator.
type TypingMessage struct {
	Action    string `json:"action"`
	Timestamp int64  `json:"timestamp"`
}

// ReceiptMessage represents a read receipt.
type ReceiptMessage struct {
	When       int64   `json:"when"`
	IsDelivery bool    `json:"isDelivery"`
	IsRead     bool    `json:"isRead"`
	IsViewed   bool    `json:"isViewed"`
	Timestamps []int64 `json:"timestamps"`
}

// Contact represents a contact.
type Contact struct {
	Name   *ContactName `json:"name,omitempty"`
	Number string       `json:"number,omitempty"`
}

// ContactName represents a contact's name.
type ContactName struct {
	Display string `json:"display,omitempty"`
	Given   string `json:"given,omitempty"`
	Family  string `json:"family,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
	Suffix  string `json:"suffix,omitempty"`
	Middle  string `json:"middle,omitempty"`
}

// GroupInfo represents group information.
type GroupInfo struct {
	GroupID string   `json:"groupId"`
	Type    string   `json:"type"`
	Name    string   `json:"name,omitempty"`
	Members []string `json:"members,omitempty"`
}

// client implements the Client interface.
type client struct {
	transport Transport
	account   string // Optional account for multi-account mode
}

// NewClient creates a new Signal client.
func NewClient(transport Transport, opts ...ClientOption) Client {
	c := &client{
		transport: transport,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ClientOption configures the client.
type ClientOption func(*client)

// WithAccount sets the account for multi-account mode.
func WithAccount(account string) ClientOption {
	return func(c *client) {
		c.account = account
	}
}

// Send implements Client.Send.
func (c *client) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	params := make(map[string]any)

	// Add account if in multi-account mode
	if c.account != "" {
		params["account"] = c.account
	}

	// Add recipients or group
	switch {
	case len(req.Recipients) > 0:
		params["recipient"] = req.Recipients
	case req.GroupID != "":
		params["groupId"] = req.GroupID
	default:
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

	// Validate response has required field
	if resp.Timestamp == 0 {
		return nil, fmt.Errorf("invalid response: missing timestamp")
	}

	return &resp, nil
}

// SendTypingIndicator implements Client.SendTypingIndicator.
func (c *client) SendTypingIndicator(ctx context.Context, recipient string, stop bool) error {
	params := map[string]any{
		"recipient": []string{recipient},
		"stop":      stop,
	}

	if c.account != "" {
		params["account"] = c.account
	}

	_, err := c.transport.Call(ctx, "sendTyping", params)
	return err
}

// SendReceipt implements Client.SendReceipt.
func (c *client) SendReceipt(ctx context.Context, recipient string, timestamp int64, receiptType string) error {
	params := map[string]any{
		"recipient":       []string{recipient},
		"targetTimestamp": timestamp,
		"type":            receiptType,
	}

	if c.account != "" {
		params["account"] = c.account
	}

	_, err := c.transport.Call(ctx, "sendReceipt", params)
	return err
}

// Subscribe implements Client.Subscribe.
func (c *client) Subscribe(ctx context.Context) (<-chan *Envelope, error) {
	// Get notification channel from transport
	notifications, err := c.transport.Subscribe(ctx)
	if err != nil {
		return nil, err
	}

	// Create envelope channel
	envelopes := make(chan *Envelope, 10)

	// Start processing notifications
	go c.processNotifications(ctx, notifications, envelopes)

	return envelopes, nil
}

// processNotifications handles incoming notifications and converts them to envelopes.
func (c *client) processNotifications(ctx context.Context, notifications <-chan *Notification, envelopes chan<- *Envelope) {
	defer close(envelopes)

	for {
		select {
		case <-ctx.Done():
			return
		case notif, ok := <-notifications:
			if !ok {
				return
			}
			c.handleNotification(ctx, notif, envelopes)
		}
	}
}

// handleNotification processes a single notification.
func (c *client) handleNotification(ctx context.Context, notif *Notification, envelopes chan<- *Envelope) {
	// Only process "receive" notifications
	if notif.Method != "receive" {
		return
	}

	envelope := c.parseEnvelope(notif)
	if envelope == nil {
		return
	}

	// Send envelope or exit if context is done
	select {
	case envelopes <- envelope:
	case <-ctx.Done():
	}
}

// parseEnvelope extracts an envelope from a notification.
func (c *client) parseEnvelope(notif *Notification) *Envelope {
	var params struct {
		Envelope *Envelope `json:"envelope"`
	}

	if err := json.Unmarshal(notif.Params, &params); err != nil {
		return nil
	}

	return params.Envelope
}

// Close implements Client.Close.
func (c *client) Close() error {
	return c.transport.Close()
}

// MockClient implements Client for testing.
type MockClient struct {
	SendFunc                func(ctx context.Context, req *SendRequest) (*SendResponse, error)
	SendTypingIndicatorFunc func(ctx context.Context, recipient string, stop bool) error
	SendReceiptFunc         func(ctx context.Context, recipient string, timestamp int64, receiptType string) error
	SubscribeFunc           func(ctx context.Context) (<-chan *Envelope, error)
	CloseFunc               func() error

	// For simpler testing
	SentMessages     []SentMessage
	TypingIndicators []MockTypingIndicator
	Receipts         []MockReceipt
	incomingMessages chan *Envelope
	SendError        error
	closed           bool
}

// SentMessage records a sent message.
type SentMessage struct {
	Recipients  []string
	GroupID     string
	Message     string
	Attachments []string
}

// MockTypingIndicator records a typing indicator for testing.
type MockTypingIndicator struct {
	Recipient string
	Stop      bool
}

// MockReceipt records a receipt for testing.
type MockReceipt struct {
	Recipient   string
	Timestamp   int64
	ReceiptType string
}

// NewMockClient creates a new mock client.
func NewMockClient() *MockClient {
	m := &MockClient{
		incomingMessages: make(chan *Envelope, 100),
	}

	// Set default implementations
	m.SendFunc = func(_ context.Context, req *SendRequest) (*SendResponse, error) {
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

	m.SendTypingIndicatorFunc = func(_ context.Context, recipient string, stop bool) error {
		m.TypingIndicators = append(m.TypingIndicators, MockTypingIndicator{
			Recipient: recipient,
			Stop:      stop,
		})
		return nil
	}

	m.SendReceiptFunc = func(_ context.Context, recipient string, timestamp int64, receiptType string) error {
		m.Receipts = append(m.Receipts, MockReceipt{
			Recipient:   recipient,
			Timestamp:   timestamp,
			ReceiptType: receiptType,
		})
		return nil
	}

	m.SubscribeFunc = func(_ context.Context) (<-chan *Envelope, error) {
		return m.incomingMessages, nil
	}

	m.CloseFunc = func() error {
		if !m.closed {
			m.closed = true
			close(m.incomingMessages)
		}
		return nil
	}

	return m
}

// Send implements Client.Send.
func (m *MockClient) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	return m.SendFunc(ctx, req)
}

// SendTypingIndicator implements Client.SendTypingIndicator.
func (m *MockClient) SendTypingIndicator(ctx context.Context, recipient string, stop bool) error {
	return m.SendTypingIndicatorFunc(ctx, recipient, stop)
}

// SendReceipt implements Client.SendReceipt.
func (m *MockClient) SendReceipt(ctx context.Context, recipient string, timestamp int64, receiptType string) error {
	return m.SendReceiptFunc(ctx, recipient, timestamp, receiptType)
}

// Subscribe implements Client.Subscribe.
func (m *MockClient) Subscribe(ctx context.Context) (<-chan *Envelope, error) {
	return m.SubscribeFunc(ctx)
}

// SimulateIncomingMessage simulates an incoming message.
func (m *MockClient) SimulateIncomingMessage(msg *Envelope) {
	if !m.closed {
		m.incomingMessages <- msg
	}
}

// Close implements Client.Close.
func (m *MockClient) Close() error {
	return m.CloseFunc()
}
