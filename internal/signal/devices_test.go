package signal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// MockTransport for testing device operations.
type MockDeviceTransport struct {
	mu        sync.Mutex
	responses map[string]any
	callLog   []CallLog
	err       error
}

type CallLog struct {
	Method string
	Params any
}

func NewMockDeviceTransport() *MockDeviceTransport {
	return &MockDeviceTransport{
		responses: make(map[string]any),
		callLog:   make([]CallLog, 0),
	}
}

func (m *MockDeviceTransport) Call(_ context.Context, method string, params any) (*json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callLog = append(m.callLog, CallLog{Method: method, Params: params})

	if m.err != nil {
		return nil, m.err
	}

	if response, ok := m.responses[method]; ok {
		// Convert response to json.RawMessage
		data, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}
		rawMsg := json.RawMessage(data)
		return &rawMsg, nil
	}

	return nil, fmt.Errorf("no mock response configured for method: %s", method)
}

func (m *MockDeviceTransport) Subscribe(_ context.Context) (<-chan *signal.Notification, error) {
	// Not needed for device tests
	return nil, fmt.Errorf("subscribe not implemented in mock")
}

func (m *MockDeviceTransport) Close() error {
	return nil
}

func (m *MockDeviceTransport) SetResponse(method string, response any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[method] = response
}

func (m *MockDeviceTransport) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func TestDeviceManager_ListDevices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		phoneNumber   string
		mockResponse  any
		mockError     error
		expectedCount int
		expectedError bool
	}{
		{
			name:        "successful device list",
			phoneNumber: "+1234567890",
			mockResponse: []any{
				map[string]any{
					"id":        float64(1),
					"name":      "Primary Device",
					"created":   float64(1609459200000), // 2021-01-01 in millis
					"lastSeen":  float64(1609545600000), // 2021-01-02 in millis
					"isPrimary": true,
				},
				map[string]any{
					"id":        float64(2),
					"name":      "Desktop",
					"created":   float64(1609459200000),
					"lastSeen":  float64(1609545600000),
					"isPrimary": false,
				},
			},
			expectedCount: 2,
			expectedError: false,
		},
		{
			name:          "empty device list",
			phoneNumber:   "+1234567890",
			mockResponse:  []any{},
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:          "transport error",
			phoneNumber:   "+1234567890",
			mockError:     fmt.Errorf("transport failed"),
			expectedCount: 0,
			expectedError: true,
		},
		{
			name:          "invalid response format",
			phoneNumber:   "+1234567890",
			mockResponse:  "invalid",
			expectedCount: 0,
			expectedError: true, // Parser should error on invalid format
		},
		{
			name:        "missing fields in device",
			phoneNumber: "+1234567890",
			mockResponse: []any{
				map[string]any{
					"id": float64(1),
					// Missing other fields
				},
			},
			expectedCount: 1,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			transport := NewMockDeviceTransport()
			if tt.mockError != nil {
				transport.SetError(tt.mockError)
			} else {
				transport.SetResponse("listDevices", tt.mockResponse)
			}

			dm := signal.NewDeviceManager(transport, tt.phoneNumber)

			// Execute
			devices, err := dm.ListDevices(context.Background())

			// Verify
			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(devices) != tt.expectedCount {
					t.Errorf("expected %d devices, got %d", tt.expectedCount, len(devices))
				}
			}

			// Check transport was called correctly
			if len(transport.callLog) != 1 {
				t.Errorf("expected 1 call, got %d", len(transport.callLog))
				return
			}

			call := transport.callLog[0]
			if call.Method != "listDevices" {
				t.Errorf("expected method 'listDevices', got '%s'", call.Method)
			}

			params, ok := call.Params.(map[string]any)
			if !ok {
				t.Error("expected params to be map[string]any")
				return
			}

			if account, accountOk := params["account"].(string); !accountOk || account != tt.phoneNumber {
				t.Errorf("expected account '%s', got '%v'", tt.phoneNumber, params["account"])
			}
		})
	}
}

func TestDeviceManager_RemoveDevice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		phoneNumber   string
		deviceID      int
		mockError     error
		expectedError bool
	}{
		{
			name:          "successful removal",
			phoneNumber:   "+1234567890",
			deviceID:      2,
			expectedError: false,
		},
		{
			name:          "transport error",
			phoneNumber:   "+1234567890",
			deviceID:      2,
			mockError:     fmt.Errorf("removal failed"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			transport := NewMockDeviceTransport()
			if tt.mockError != nil {
				transport.SetError(tt.mockError)
			} else {
				transport.SetResponse("removeDevice", map[string]any{"success": true})
			}

			dm := signal.NewDeviceManager(transport, tt.phoneNumber)

			// Execute
			err := dm.RemoveDevice(context.Background(), tt.deviceID)

			// Verify
			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// Check transport was called correctly
			if len(transport.callLog) != 1 {
				t.Errorf("expected 1 call, got %d", len(transport.callLog))
				return
			}

			call := transport.callLog[0]
			if call.Method != "removeDevice" {
				t.Errorf("expected method 'removeDevice', got '%s'", call.Method)
			}

			params, ok := call.Params.(map[string]any)
			if !ok {
				t.Error("expected params to be map[string]any")
				return
			}

			if deviceID, deviceOk := params["deviceId"].(int); !deviceOk || deviceID != tt.deviceID {
				t.Errorf("expected deviceId %d, got %v", tt.deviceID, params["deviceId"])
			}
		})
	}
}

func TestDeviceManager_GenerateLinkingURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		phoneNumber   string
		deviceName    string
		mockResponse  any
		mockError     error
		expectedURI   string
		expectedError bool
	}{
		{
			name:        "successful URI generation",
			phoneNumber: "+1234567890",
			deviceName:  "My Tablet",
			mockResponse: map[string]any{
				"uri": "signal://link?uuid=123&pub_key=456",
			},
			expectedURI:   "signal://link?uuid=123&pub_key=456",
			expectedError: false,
		},
		{
			name:          "transport error",
			phoneNumber:   "+1234567890",
			deviceName:    "My Tablet",
			mockError:     fmt.Errorf("linking failed"),
			expectedError: true,
		},
		{
			name:          "invalid response format",
			phoneNumber:   "+1234567890",
			deviceName:    "My Tablet",
			mockResponse:  []any{}, // Wrong format
			expectedError: true,
		},
		{
			name:        "missing URI in response",
			phoneNumber: "+1234567890",
			deviceName:  "My Tablet",
			mockResponse: map[string]any{
				"status": "started",
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			transport := NewMockDeviceTransport()
			if tt.mockError != nil {
				transport.SetError(tt.mockError)
			} else {
				transport.SetResponse("startLink", tt.mockResponse)
			}

			dm := signal.NewDeviceManager(transport, tt.phoneNumber)

			// Execute
			uri, err := dm.GenerateLinkingURI(context.Background(), tt.deviceName)

			// Verify
			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if uri != tt.expectedURI {
					t.Errorf("expected URI '%s', got '%s'", tt.expectedURI, uri)
				}
			}

			// Check transport was called correctly
			if len(transport.callLog) != 1 {
				t.Errorf("expected 1 call, got %d", len(transport.callLog))
				return
			}

			call := transport.callLog[0]
			if call.Method != "startLink" {
				t.Errorf("expected method 'startLink', got '%s'", call.Method)
			}

			params, ok := call.Params.(map[string]any)
			if !ok {
				t.Error("expected params to be map[string]any")
				return
			}

			if name, nameOk := params["name"].(string); !nameOk || name != tt.deviceName {
				t.Errorf("expected name '%s', got '%v'", tt.deviceName, params["name"])
			}
		})
	}
}

func TestDeviceManager_GetPrimaryDevice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		phoneNumber   string
		mockResponse  any
		expectedName  string
		expectedID    int
		expectedError bool
	}{
		{
			name:        "primary device found",
			phoneNumber: "+1234567890",
			mockResponse: []any{
				map[string]any{
					"id":        float64(1),
					"name":      "Primary Phone",
					"isPrimary": true,
				},
				map[string]any{
					"id":        float64(2),
					"name":      "Desktop",
					"isPrimary": false,
				},
			},
			expectedName:  "Primary Phone",
			expectedID:    1,
			expectedError: false,
		},
		{
			name:        "no primary device",
			phoneNumber: "+1234567890",
			mockResponse: []any{
				map[string]any{
					"id":        float64(2),
					"name":      "Desktop",
					"isPrimary": false,
				},
			},
			expectedError: true,
		},
		{
			name:          "empty device list",
			phoneNumber:   "+1234567890",
			mockResponse:  []any{},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			transport := NewMockDeviceTransport()
			transport.SetResponse("listDevices", tt.mockResponse)

			dm := signal.NewDeviceManager(transport, tt.phoneNumber)

			// Execute
			device, err := dm.GetPrimaryDevice(context.Background())

			// Verify
			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if device.Name != tt.expectedName {
				t.Errorf("expected name '%s', got '%s'", tt.expectedName, device.Name)
			}
			if device.ID != tt.expectedID {
				t.Errorf("expected ID %d, got %d", tt.expectedID, device.ID)
			}
			if !device.Primary {
				t.Error("expected device to be primary")
			}
		})
	}
}

func TestDeviceManager_WaitForDeviceLink(t *testing.T) {
	// This test can't run in parallel due to timing
	tests := []struct {
		name           string
		phoneNumber    string
		timeout        time.Duration
		initialDevices []any
		newDevice      map[string]any
		pollDelay      time.Duration
		expectedName   string
		expectedError  bool
	}{
		{
			name:        "successful device link",
			phoneNumber: "+1234567890",
			timeout:     5 * time.Second,
			initialDevices: []any{
				map[string]any{
					"id":   float64(1),
					"name": "Primary",
				},
			},
			newDevice: map[string]any{
				"id":   float64(2),
				"name": "New Tablet",
			},
			pollDelay:     100 * time.Millisecond,
			expectedName:  "New Tablet",
			expectedError: false,
		},
		{
			name:        "timeout waiting for device",
			phoneNumber: "+1234567890",
			timeout:     200 * time.Millisecond,
			initialDevices: []any{
				map[string]any{
					"id":   float64(1),
					"name": "Primary",
				},
			},
			pollDelay:     300 * time.Millisecond, // Longer than timeout
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			transport := NewMockDeviceTransport()
			dm := signal.NewDeviceManager(transport, tt.phoneNumber)

			// Set initial response
			transport.SetResponse("listDevices", tt.initialDevices)

			// Start async operation to add new device after delay
			if tt.newDevice != nil {
				go func() {
					time.Sleep(tt.pollDelay)
					allDevices := make([]any, len(tt.initialDevices)+1)
					copy(allDevices, tt.initialDevices)
					allDevices[len(tt.initialDevices)] = tt.newDevice
					transport.SetResponse("listDevices", allDevices)
				}()
			}

			// Execute
			ctx := context.Background()
			device, err := dm.WaitForDeviceLink(ctx, tt.timeout)

			// Verify
			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if device.Name != tt.expectedName {
					t.Errorf("expected name '%s', got '%s'", tt.expectedName, device.Name)
				}
			}
		})
	}
}

func TestDeviceManager_NilTransport(t *testing.T) {
	t.Parallel()

	dm := signal.NewDeviceManager(nil, "+1234567890")

	t.Run("ListDevices", func(t *testing.T) {
		t.Parallel()
		_, err := dm.ListDevices(context.Background())
		if err == nil {
			t.Error("expected error for nil transport")
		}
	})

	t.Run("RemoveDevice", func(t *testing.T) {
		t.Parallel()
		err := dm.RemoveDevice(context.Background(), 1)
		if err == nil {
			t.Error("expected error for nil transport")
		}
	})

	t.Run("GenerateLinkingURI", func(t *testing.T) {
		t.Parallel()
		_, err := dm.GenerateLinkingURI(context.Background(), "test")
		if err == nil {
			t.Error("expected error for nil transport")
		}
	})
}
