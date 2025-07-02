package signal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMockTransport_Call(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		params         any
		mockResponse   *json.RawMessage
		mockError      error
		wantErr        bool
		validateResult func(t *testing.T, result *json.RawMessage)
	}{
		{
			name:   "successful call",
			method: "send",
			params: map[string]any{
				"recipient": []string{"+1234567890"},
				"message":   "Hello, World!",
			},
			mockResponse: func() *json.RawMessage {
				msg := json.RawMessage(`{"timestamp": 1699564800000}`)
				return &msg
			}(),
			wantErr: false,
			validateResult: func(t *testing.T, result *json.RawMessage) {
				t.Helper()
				var resp struct {
					Timestamp int64 `json:"timestamp"`
				}
				err := json.Unmarshal(*result, &resp)
				if err != nil {
					t.Errorf("Failed to unmarshal result: %v", err)
				}
				if resp.Timestamp != 1699564800000 {
					t.Errorf("Expected timestamp 1699564800000, got %d", resp.Timestamp)
				}
			},
		},
		{
			name:   "call with error",
			method: "send",
			params: map[string]any{
				"recipient": []string{"+invalid"},
				"message":   "Test",
			},
			mockError: fmt.Errorf("invalid phone number"),
			wantErr:   true,
		},
		{
			name:   "call with nil params",
			method: "sendTyping",
			params: nil,
			mockResponse: func() *json.RawMessage {
				msg := json.RawMessage(`{"success": true}`)
				return &msg
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transport
			transport := NewMockTransport()
			if tt.mockResponse != nil {
				transport.SetResponse(tt.method, tt.mockResponse, tt.mockError)
			} else {
				transport.SetError(tt.method, tt.mockError)
			}

			// Make call
			ctx := context.Background()
			result, err := transport.Call(ctx, tt.method, tt.params)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Call() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Validate result if no error
			if !tt.wantErr && tt.validateResult != nil {
				tt.validateResult(t, result)
			}

			// Verify call was recorded
			calls := transport.GetCalls(tt.method)
			if len(calls) != 1 {
				t.Errorf("Expected 1 call to %s, got %d", tt.method, len(calls))
			}
		})
	}
}

func TestMockTransport_Subscribe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transport := NewMockTransport()

	// Subscribe
	notifications, err := transport.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Simulate incoming notification
	testNotif := &Notification{
		JSONRPC: "2.0",
		Method:  "receive",
		Params: json.RawMessage(`{
			"envelope": {
				"source": "+1234567890",
				"sourceNumber": "+1234567890",
				"timestamp": 1699564800000,
				"dataMessage": {
					"message": "Test message"
				}
			}
		}`),
	}

	transport.SimulateNotification(testNotif)

	// Receive notification
	select {
	case notif := <-notifications:
		if notif.Method != "receive" {
			t.Errorf("Expected method 'receive', got %s", notif.Method)
		}
		// Verify params
		var params struct {
			Envelope struct {
				Source string `json:"source"`
			} `json:"envelope"`
		}
		if unmarshalErr := json.Unmarshal(notif.Params, &params); unmarshalErr != nil {
			t.Errorf("Failed to unmarshal params: %v", unmarshalErr)
		}
		if params.Envelope.Source != "+1234567890" {
			t.Errorf("Expected source '+1234567890', got %s", params.Envelope.Source)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for notification")
	}
}

func TestMockTransport_Close(t *testing.T) {
	transport := NewMockTransport()

	// Subscribe to get a channel
	ctx := context.Background()
	notifications, err := transport.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Close transport
	err = transport.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify channel is closed
	select {
	case _, ok := <-notifications:
		if ok {
			t.Error("Expected notifications channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Notifications channel not closed after Close()")
	}

	// Verify transport is marked as closed
	if !transport.IsClosed() {
		t.Error("Transport should be marked as closed")
	}

	// Verify calling Close again returns no error
	err = transport.Close()
	if err != nil {
		t.Errorf("Close() on already closed transport error = %v", err)
	}
}

func TestMockTransport_ConcurrentCalls(t *testing.T) {
	transport := NewMockTransport()
	transport.SetResponse("test", func() *json.RawMessage {
		msg := json.RawMessage(`{"success": true}`)
		return &msg
	}(), nil)

	ctx := context.Background()
	done := make(chan struct{})
	errors := make(chan error, 10)

	// Make 10 concurrent calls
	for i := range 10 {
		go func(id int) {
			defer func() { done <- struct{}{} }()

			_, err := transport.Call(ctx, "test", map[string]any{
				"id": id,
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent call error: %v", err)
	}

	// Verify all calls were recorded
	calls := transport.GetCalls("test")
	if len(calls) != 10 {
		t.Errorf("Expected 10 calls, got %d", len(calls))
	}
}

func TestMockTransport_ContextCancellation(t *testing.T) {
	transport := NewMockTransport()
	transport.SetResponseDelay("slow", 500*time.Millisecond)
	transport.SetResponse("slow", func() *json.RawMessage {
		msg := json.RawMessage(`{"success": true}`)
		return &msg
	}(), nil)

	// Create a context that will be canceled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Make call that should be canceled
	_, err := transport.Call(ctx, "slow", nil)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
}
