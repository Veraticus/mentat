//go:build integration
// +build integration

package signal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// TestIntegration_ClientWithMockTransport tests the full client flow with mock transport
func TestIntegration_ClientWithMockTransport(t *testing.T) {
	// Create mock transport
	transport := signal.NewMockTransport()

	// Set up response for send method
	transport.SetResponse("send", func() *json.RawMessage {
		msg := json.RawMessage(`{"timestamp": 1699564800000}`)
		return &msg
	}(), nil)

	// Create client
	client := signal.NewClient(transport)

	// Test sending a message
	ctx := context.Background()
	resp, err := client.Send(ctx, &signal.SendRequest{
		Recipients: []string{"+1234567890"},
		Message:    "Integration test message",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if resp.Timestamp != 1699564800000 {
		t.Errorf("Expected timestamp 1699564800000, got %d", resp.Timestamp)
	}

	// Verify transport was called
	calls := transport.GetCalls("send")
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call to send, got %d", len(calls))
	}

	// Verify params structure
	params := calls[0].(map[string]interface{})
	recipients := params["recipient"].([]string)
	if len(recipients) != 1 || recipients[0] != "+1234567890" {
		t.Errorf("Expected recipient [+1234567890], got %v", recipients)
	}
	if params["message"] != "Integration test message" {
		t.Errorf("Expected message 'Integration test message', got %v", params["message"])
	}
}

// TestIntegration_SubscriptionFlow tests the full subscription flow
func TestIntegration_SubscriptionFlow(t *testing.T) {
	// Create mock transport
	transport := signal.NewMockTransport()

	// Create client
	client := signal.NewClient(transport)

	// Subscribe
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	envelopes, err := client.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Simulate incoming message
	go func() {
		time.Sleep(100 * time.Millisecond)
		transport.SimulateNotification(&signal.Notification{
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
						"message": "Hello from integration test",
						"expiresInSeconds": 0,
						"viewOnce": false,
						"attachments": [
							{
								"contentType": "image/jpeg",
								"filename": "test.jpg",
								"id": "attachment-123",
								"size": 12345,
								"width": 800,
								"height": 600,
								"uploadTimestamp": 1699564800000
							}
						]
					}
				}
			}`),
		})
	}()

	// Receive message
	select {
	case envelope := <-envelopes:
		if envelope.Source != "+9876543210" {
			t.Errorf("Expected source '+9876543210', got %s", envelope.Source)
		}
		if envelope.signal.DataMessage == nil {
			t.Fatal("Expected signal.DataMessage to be non-nil")
		}
		if envelope.signal.DataMessage.Message != "Hello from integration test" {
			t.Errorf("Expected message 'Hello from integration test', got %s", envelope.signal.DataMessage.Message)
		}
		if len(envelope.signal.DataMessage.Attachments) != 1 {
			t.Errorf("Expected 1 attachment, got %d", len(envelope.signal.DataMessage.Attachments))
		} else {
			att := envelope.signal.DataMessage.Attachments[0]
			if att.ContentType != "image/jpeg" {
				t.Errorf("Expected content type 'image/jpeg', got %s", att.ContentType)
			}
			if att.ID != "attachment-123" {
				t.Errorf("Expected attachment ID 'attachment-123', got %s", att.ID)
			}
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

// TestIntegration_ErrorHandling tests error propagation through the stack
func TestIntegration_ErrorHandling(t *testing.T) {
	// Create mock transport
	transport := signal.NewMockTransport()

	// Set up error response
	transport.SetError("send", fmt.Errorf("network error"))

	// Create client
	client := signal.NewClient(transport)

	// Try to send a message
	ctx := context.Background()
	_, err := client.Send(ctx, &signal.SendRequest{
		Recipients: []string{"+1234567890"},
		Message:    "This should fail",
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Error() != "send failed: network error" {
		t.Errorf("Expected 'send failed: network error', got %v", err)
	}
}

// TestIntegration_ConcurrentOperations tests concurrent client operations
func TestIntegration_ConcurrentOperations(t *testing.T) {
	// Create mock transport
	transport := signal.NewMockTransport()
	transport.SetResponse("send", func() *json.RawMessage {
		msg := json.RawMessage(`{"timestamp": 1699564800000}`)
		return &msg
	}(), nil)
	transport.SetResponse("sendTyping", nil, nil)

	// Create client
	client := signal.NewClient(transport)

	// Launch concurrent operations
	ctx := context.Background()
	errors := make(chan error, 20)
	done := make(chan struct{}, 20)

	// 10 sends
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			_, err := client.Send(ctx, &signal.SendRequest{
				Recipients: []string{fmt.Sprintf("+123456789%d", id)},
				Message:    fmt.Sprintf("Message %d", id),
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// 10 typing indicators
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			err := client.SendTypingIndicator(ctx, fmt.Sprintf("+123456789%d", id), false)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// Wait for all operations
	for i := 0; i < 20; i++ {
		<-done
	}
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify all calls were made
	sendCalls := transport.GetCalls("send")
	if len(sendCalls) != 10 {
		t.Errorf("Expected 10 send calls, got %d", len(sendCalls))
	}

	typingCalls := transport.GetCalls("sendTyping")
	if len(typingCalls) != 10 {
		t.Errorf("Expected 10 sendTyping calls, got %d", len(typingCalls))
	}
}

// TestIntegration_TypingIndicatorFlow tests typing indicator flow
func TestIntegration_TypingIndicatorFlow(t *testing.T) {
	// Create mock transport
	transport := signal.NewMockTransport()
	transport.SetResponse("sendTyping", nil, nil)

	// Create client
	client := signal.NewClient(transport)

	// Send typing start
	ctx := context.Background()
	err := client.SendTypingIndicator(ctx, "+1234567890", false)
	if err != nil {
		t.Fatalf("SendTypingIndicator (start) failed: %v", err)
	}

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Send typing stop
	err = client.SendTypingIndicator(ctx, "+1234567890", true)
	if err != nil {
		t.Fatalf("SendTypingIndicator (stop) failed: %v", err)
	}

	// Verify calls
	calls := transport.GetCalls("sendTyping")
	if len(calls) != 2 {
		t.Fatalf("Expected 2 calls, got %d", len(calls))
	}

	// Verify first call (start)
	params1 := calls[0].(map[string]interface{})
	if params1["stop"] != false {
		t.Error("First call should have stop=false")
	}

	// Verify second call (stop)
	params2 := calls[1].(map[string]interface{})
	if params2["stop"] != true {
		t.Error("Second call should have stop=true")
	}
}

// TestIntegration_RealSignalCLI tests with real signal-cli if available
func TestIntegration_RealSignalCLI(t *testing.T) {
	// Skip if not in integration mode
	socketPath := os.Getenv("SIGNAL_TEST_SOCKET")
	if socketPath == "" {
		t.Skip("SIGNAL_TEST_SOCKET not set, skipping real signal-cli test")
	}

	recipient := os.Getenv("SIGNAL_TEST_RECIPIENT")
	if recipient == "" {
		t.Skip("SIGNAL_TEST_RECIPIENT not set, skipping real signal-cli test")
	}

	// Create real Unix socket transport
	transport, err := signal.NewUnixSocketTransport(socketPath)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Create client
	client := signal.NewClient(transport)

	// Test sending a message
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Send(ctx, &signal.SendRequest{
		Recipients: []string{recipient},
		Message:    fmt.Sprintf("Integration test at %s", time.Now().Format(time.RFC3339)),
	})

	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.Timestamp == 0 {
		t.Error("Expected non-zero timestamp")
	}

	t.Logf("Message sent successfully with timestamp: %d", resp.Timestamp)
}
