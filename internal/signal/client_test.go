package signal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// Helper functions to reduce complexity.
func createTimestampResponse(timestamp int64) *json.RawMessage {
	msg := json.RawMessage(fmt.Sprintf(`{"timestamp": %d}`, timestamp))
	return &msg
}

func validateSendParams(t *testing.T, params map[string]any, req *signal.SendRequest) {
	t.Helper()

	// Check recipient/group
	if req.Recipients != nil {
		if _, ok := params["recipient"]; !ok {
			t.Error("Expected 'recipient' in params")
		}
	}
	if req.GroupID != "" {
		if _, ok := params["groupId"]; !ok {
			t.Error("Expected 'groupId' in params")
		}
	}

	// Check message
	if _, ok := params["message"]; !ok {
		t.Error("Expected 'message' in params")
	}

	// Check attachments
	validateAttachmentParams(t, params, req.Attachments)
}

func validateAttachmentParams(t *testing.T, params map[string]any, attachments []string) {
	t.Helper()

	switch len(attachments) {
	case 0:
		// No attachments expected
	case 1:
		if _, ok := params["attachment"]; !ok {
			t.Error("Expected 'attachment' for single attachment")
		}
	default:
		if _, ok := params["attachments"]; !ok {
			t.Error("Expected 'attachments' for multiple attachments")
		}
	}
}

type sendTestCase struct {
	name             string
	request          *signal.SendRequest
	transportResult  *json.RawMessage
	transportError   error
	wantErr          bool
	validateResponse func(t *testing.T, resp *signal.SendResponse)
}

func TestClient_Send(t *testing.T) {
	tests := getSendTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transport
			transport := signal.NewMockTransport()
			transport.SetResponse("send", tt.transportResult, tt.transportError)

			// Create client
			client := signal.NewClient(transport)

			// Send message
			ctx := context.Background()
			resp, err := client.Send(ctx, tt.request)

			// Validate result
			validateSendResult(t, tt, resp, err, transport)
		})
	}
}

func getSendTestCases() []sendTestCase {
	cases := []sendTestCase{}
	cases = append(cases, getBasicSendCases()...)
	cases = append(cases, getAttachmentSendCases()...)
	cases = append(cases, getErrorSendCases()...)
	return cases
}

func getBasicSendCases() []sendTestCase {
	return []sendTestCase{
		{
			name: "send text message",
			request: &signal.SendRequest{
				Recipients: []string{"+1234567890"},
				Message:    "Hello, World!",
			},
			transportResult: createTimestampResponse(1699564800000),
			wantErr:         false,
			validateResponse: func(t *testing.T, resp *signal.SendResponse) {
				t.Helper()
				if resp.Timestamp != 1699564800000 {
					t.Errorf("Expected timestamp 1699564800000, got %d", resp.Timestamp)
				}
			},
		},
		{
			name: "send to group",
			request: &signal.SendRequest{
				GroupID: "group123",
				Message: "Team update",
			},
			transportResult: createTimestampResponse(1699564800003),
			wantErr:         false,
		},
		{
			name: "send with mentions",
			request: &signal.SendRequest{
				Recipients: []string{"+1234567890"},
				Message:    "Hey @john, check this out",
				Mentions: []signal.Mention{
					{Start: 4, Length: 5, Author: "+0987654321"},
				},
			},
			transportResult: createTimestampResponse(1699564800004),
			wantErr:         false,
		},
	}
}

func getAttachmentSendCases() []sendTestCase {
	return []sendTestCase{
		{
			name: "send with single attachment",
			request: &signal.SendRequest{
				Recipients:  []string{"+1234567890"},
				Message:     "Check this out",
				Attachments: []string{"/tmp/photo.jpg"},
			},
			transportResult: createTimestampResponse(1699564800001),
			wantErr:         false,
		},
		{
			name: "send with multiple attachments",
			request: &signal.SendRequest{
				Recipients: []string{"+1234567890"},
				Message:    "Meeting photos",
				Attachments: []string{
					"/tmp/photo1.jpg",
					"/tmp/photo2.jpg",
					"/tmp/photo3.jpg",
				},
			},
			transportResult: createTimestampResponse(1699564800002),
			wantErr:         false,
		},
	}
}

func getErrorSendCases() []sendTestCase {
	return []sendTestCase{
		{
			name: "missing recipient and group",
			request: &signal.SendRequest{
				Message: "Orphan message",
			},
			wantErr: true,
		},
		{
			name: "transport error",
			request: &signal.SendRequest{
				Recipients: []string{"+1234567890"},
				Message:    "Test",
			},
			transportError: fmt.Errorf("network error"),
			wantErr:        true,
		},
		{
			name: "invalid response format",
			request: &signal.SendRequest{
				Recipients: []string{"+1234567890"},
				Message:    "Test",
			},
			transportResult: func() *json.RawMessage {
				msg := json.RawMessage(`{"invalid": "response"}`)
				return &msg
			}(),
			wantErr: true,
		},
	}
}

func TestClient_SendTypingIndicator(t *testing.T) {
	tests := []struct {
		name      string
		recipient string
		stop      bool
		wantErr   bool
	}{
		{
			name:      "start typing",
			recipient: "+1234567890",
			stop:      false,
			wantErr:   false,
		},
		{
			name:      "stop typing",
			recipient: "+1234567890",
			stop:      true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transport
			transport := signal.NewMockTransport()
			transport.SetResponse("sendTyping", nil, nil)

			// Create client
			client := signal.NewClient(transport)

			// Send typing indicator
			ctx := context.Background()
			err := client.SendTypingIndicator(ctx, tt.recipient, tt.stop)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("SendTypingIndicator() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify transport was called correctly
			calls := transport.GetCalls("sendTyping")
			if len(calls) != 1 {
				t.Errorf("Expected 1 call to sendTyping, got %d", len(calls))
				return
			}

			params, ok := calls[0].(map[string]any)
			if !ok {
				t.Fatal("Expected params to be map[string]any")
			}

			recipients, ok := params["recipient"].([]string)
			if !ok {
				t.Fatal("Expected recipient to be []string")
			}

			if len(recipients) != 1 || recipients[0] != tt.recipient {
				t.Errorf("Expected recipient %s, got %v", tt.recipient, recipients)
			}

			if params["stop"] != tt.stop {
				t.Errorf("Expected stop=%v, got %v", tt.stop, params["stop"])
			}
		})
	}
}

func TestClient_Subscribe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create mock transport
	transport := signal.NewMockTransport()

	// Create client
	client := signal.NewClient(transport)

	// Subscribe
	envelopes, err := client.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Test various notification types
	t.Run("data message", func(t *testing.T) {
		testDataMessage(t, transport, envelopes)
	})

	t.Run("typing message", func(t *testing.T) {
		testTypingMessage(t, transport, envelopes)
	})

	t.Run("non-receive notification ignored", func(t *testing.T) {
		testNonReceiveNotification(t, transport, envelopes)
	})
}

func testDataMessage(t *testing.T, transport *signal.MockTransport, envelopes <-chan *signal.Envelope) {
	t.Helper()
	// Simulate incoming data message
	notif := &signal.Notification{
		JSONRPC: "2.0",
		Method:  "receive",
		Params: json.RawMessage(`{
			"envelope": {
				"source": "+9876543210",
				"sourceNumber": "+9876543210",
				"sourceName": "Test User",
				"sourceDevice": 1,
				"timestamp": 1699564800000,
				"dataMessage": {
					"timestamp": 1699564800000,
					"message": "Test message",
					"expiresInSeconds": 0,
					"viewOnce": false,
					"attachments": []
				}
			}
		}`),
	}

	transport.SimulateNotification(notif)

	// Receive envelope
	select {
	case env := <-envelopes:
		if env.Source != "+9876543210" {
			t.Errorf("Expected source '+9876543210', got %s", env.Source)
		}
		if env.DataMessage == nil {
			t.Error("Expected DataMessage to be non-nil")
		} else if env.DataMessage.Message != "Test message" {
			t.Errorf("Expected message 'Test message', got %s", env.DataMessage.Message)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for envelope")
	}
}

func testTypingMessage(t *testing.T, transport *signal.MockTransport, envelopes <-chan *signal.Envelope) {
	t.Helper()
	// Simulate typing indicator
	notif := &signal.Notification{
		JSONRPC: "2.0",
		Method:  "receive",
		Params: json.RawMessage(`{
			"envelope": {
				"source": "+9876543210",
				"sourceNumber": "+9876543210",
				"timestamp": 1699564900000,
				"typingMessage": {
					"action": "STARTED",
					"timestamp": 1699564900000
				}
			}
		}`),
	}

	transport.SimulateNotification(notif)

	// Receive envelope
	select {
	case env := <-envelopes:
		if env.TypingMessage == nil {
			t.Error("Expected TypingMessage to be non-nil")
		} else if env.TypingMessage.Action != "STARTED" {
			t.Errorf("Expected action 'STARTED', got %s", env.TypingMessage.Action)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for typing indicator")
	}
}

func testNonReceiveNotification(t *testing.T, transport *signal.MockTransport, envelopes <-chan *signal.Envelope) {
	t.Helper()
	// Simulate non-receive notification
	notif := &signal.Notification{
		JSONRPC: "2.0",
		Method:  "status",
		Params:  json.RawMessage(`{"status": "connected"}`),
	}

	transport.SimulateNotification(notif)

	// Should not receive anything
	select {
	case <-envelopes:
		t.Error("Should not receive non-receive notifications")
	case <-time.After(100 * time.Millisecond):
		// Expected timeout
	}
}

func TestClient_Close(t *testing.T) {
	// Create mock transport
	transport := signal.NewMockTransport()

	// Create client
	client := signal.NewClient(transport)

	// Close client
	err := client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify transport was closed
	if !transport.IsClosed() {
		t.Error("Expected transport to be closed")
	}
}

func TestClient_WithAccount(t *testing.T) {
	// Create mock transport
	transport := signal.NewMockTransport()
	transport.SetResponse("send", func() *json.RawMessage {
		msg := json.RawMessage(`{"timestamp": 1699564800000}`)
		return &msg
	}(), nil)

	// Create client with account
	client := signal.NewClient(transport, signal.WithAccount("+1111111111"))

	// Send message
	ctx := context.Background()
	_, err := client.Send(ctx, &signal.SendRequest{
		Recipients: []string{"+2222222222"},
		Message:    "Test from account",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Verify account was included in params
	calls := transport.GetCalls("send")
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	params, ok := calls[0].(map[string]any)
	if !ok {
		t.Fatal("Expected params to be map[string]any")
	}
	if account, accountOk := params["account"]; !accountOk {
		t.Error("Expected 'account' in params")
	} else if account != "+1111111111" {
		t.Errorf("Expected account '+1111111111', got %v", account)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	// Create mock transport with delay
	transport := signal.NewMockTransport()
	transport.SetResponseDelay("send", 500*time.Millisecond)
	transport.SetResponse("send", func() *json.RawMessage {
		msg := json.RawMessage(`{"timestamp": 1699564800000}`)
		return &msg
	}(), nil)

	// Create client
	client := signal.NewClient(transport)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Send should timeout
	_, err := client.Send(ctx, &signal.SendRequest{
		Recipients: []string{"+1234567890"},
		Message:    "Test",
	})

	if err == nil {
		t.Error("Expected timeout error")
		return
	}

	// The error should contain the deadline exceeded error
	if !containsError(err, context.DeadlineExceeded) {
		t.Errorf("Expected error to contain DeadlineExceeded, got %v", err)
	}
}

func validateSendResult(
	t *testing.T,
	tt sendTestCase,
	resp *signal.SendResponse,
	err error,
	transport *signal.MockTransport,
) {
	t.Helper()

	// Check error
	if (err != nil) != tt.wantErr {
		t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
		return
	}

	// Validate response if no error expected
	if !tt.wantErr && tt.validateResponse != nil {
		tt.validateResponse(t, resp)
	}

	// Verify transport was called correctly (unless validation error)
	if tt.request.Recipients != nil || tt.request.GroupID != "" {
		validateTransportCall(t, transport, tt.request)
	}
}

func validateTransportCall(t *testing.T, transport *signal.MockTransport, request *signal.SendRequest) {
	t.Helper()

	calls := transport.GetCalls("send")
	if len(calls) != 1 {
		t.Errorf("Expected 1 call to send, got %d", len(calls))
		return
	}

	// Verify params structure
	params, ok := calls[0].(map[string]any)
	if !ok {
		t.Fatal("Expected params to be map[string]any")
	}
	validateSendParams(t, params, request)
}

// containsError checks if the error or any wrapped error matches the target.
func containsError(err, target error) bool {
	if errors.Is(err, target) {
		return true
	}
	// Check if error contains the target error string
	return err != nil && target != nil &&
		(err.Error() == target.Error() ||
			err.Error() == fmt.Sprintf("send failed: %v", target))
}
