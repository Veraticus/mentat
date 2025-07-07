// Package signal provides Signal messenger integration via signal-cli.
package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// deviceLinkPollInterval is the interval for polling device link status.
	deviceLinkPollInterval = 2 * time.Second
	// millisPerSecond is the number of milliseconds in a second.
	millisPerSecond = 1000
)

// Ensure DeviceManagerImpl implements DeviceManager interface.
var _ DeviceManager = (*DeviceManagerImpl)(nil)

// DeviceManagerImpl manages Signal device operations using signal-cli.
type DeviceManagerImpl struct {
	transport   Transport
	phoneNumber string
}

// NewDeviceManager creates a new device manager.
func NewDeviceManager(transport Transport, phoneNumber string) *DeviceManagerImpl {
	return &DeviceManagerImpl{
		transport:   transport,
		phoneNumber: phoneNumber,
	}
}

// ListDevices returns all linked devices for the account.
func (dm *DeviceManagerImpl) ListDevices(ctx context.Context) ([]Device, error) {
	if dm.transport == nil {
		return nil, fmt.Errorf("transport not initialized")
	}

	// Call signal-cli listDevices RPC method
	params := map[string]any{
		"account": dm.phoneNumber,
	}

	response, err := dm.transport.Call(ctx, "listDevices", params)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	// Parse the response
	var devices []Device
	if parseErr := parseDeviceListResponse(response, &devices); parseErr != nil {
		return nil, fmt.Errorf("failed to parse device list: %w", parseErr)
	}

	return devices, nil
}

// RemoveDevice removes a linked device by ID.
func (dm *DeviceManagerImpl) RemoveDevice(ctx context.Context, deviceID int) error {
	if dm.transport == nil {
		return fmt.Errorf("transport not initialized")
	}

	// Call signal-cli removeDevice RPC method
	params := map[string]any{
		"account":  dm.phoneNumber,
		"deviceId": deviceID,
	}

	_, err := dm.transport.Call(ctx, "removeDevice", params)
	if err != nil {
		return fmt.Errorf("failed to remove device %d: %w", deviceID, err)
	}

	return nil
}

// GenerateLinkingURI creates a linking URI for adding a new device.
func (dm *DeviceManagerImpl) GenerateLinkingURI(ctx context.Context, deviceName string) (string, error) {
	if dm.transport == nil {
		return "", fmt.Errorf("transport not initialized")
	}

	// Call signal-cli startLink RPC method
	params := map[string]any{
		"account": dm.phoneNumber,
		"name":    deviceName,
	}

	response, err := dm.transport.Call(ctx, "startLink", params)
	if err != nil {
		return "", fmt.Errorf("failed to generate linking URI: %w", err)
	}

	// Extract the URI from the response
	uri, err := extractLinkingURI(response)
	if err != nil {
		return "", fmt.Errorf("failed to extract linking URI: %w", err)
	}

	return uri, nil
}

// GetPrimaryDevice returns the primary device information.
func (dm *DeviceManagerImpl) GetPrimaryDevice(ctx context.Context) (*Device, error) {
	devices, err := dm.ListDevices(ctx)
	if err != nil {
		return nil, err
	}

	for i := range devices {
		if devices[i].Primary {
			return &devices[i], nil
		}
	}

	return nil, fmt.Errorf("no primary device found")
}

// WaitForDeviceLink waits for a device to complete linking.
func (dm *DeviceManagerImpl) WaitForDeviceLink(ctx context.Context, timeout time.Duration) (*Device, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get initial device list
	initialDevices, err := dm.ListDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get initial device list: %w", err)
	}

	// Poll for new device
	ticker := time.NewTicker(deviceLinkPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for device link: %w", ctx.Err())
		case <-ticker.C:
			currentDevices, listErr := dm.ListDevices(ctx)
			if listErr != nil {
				// Continue polling on transient errors
				continue
			}

			// Check for new device
			newDevice := findNewDevice(initialDevices, currentDevices)
			if newDevice != nil {
				return newDevice, nil
			}
		}
	}
}

// parseDeviceListResponse parses the JSON-RPC response for device list.
func parseDeviceListResponse(response any, devices *[]Device) error {
	// The response format depends on signal-cli version
	// Typically it's an array of device objects
	data, ok := response.([]any)
	if !ok {
		// Try to marshal/unmarshal to handle different types
		jsonData, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}

		var tempData []any
		if unmarshalErr := json.Unmarshal(jsonData, &tempData); unmarshalErr != nil {
			return fmt.Errorf("unexpected response format: %v", response)
		}
		data = tempData
	}

	for _, item := range data {
		deviceMap, isMap := item.(map[string]any)
		if !isMap {
			continue
		}

		device := Device{}

		// Parse device ID
		if id, idOk := deviceMap["id"].(float64); idOk {
			device.ID = int(id)
		}

		// Parse device name
		if name, nameOk := deviceMap["name"].(string); nameOk {
			device.Name = name
		}

		// Parse creation time
		if created, createdOk := deviceMap["created"].(float64); createdOk {
			device.Created = time.Unix(int64(created/millisPerSecond), 0)
		}

		// Parse last seen time
		if lastSeen, lastSeenOk := deviceMap["lastSeen"].(float64); lastSeenOk {
			device.LastSeen = time.Unix(int64(lastSeen/millisPerSecond), 0)
		}

		// Parse primary flag
		if primary, primaryOk := deviceMap["isPrimary"].(bool); primaryOk {
			device.Primary = primary
		}

		*devices = append(*devices, device)
	}

	return nil
}

// extractLinkingURI extracts the linking URI from the response.
func extractLinkingURI(response any) (string, error) {
	// Handle *json.RawMessage
	if rawMsg, ok := response.(*json.RawMessage); ok {
		var responseMap map[string]any
		if err := json.Unmarshal(*rawMsg, &responseMap); err != nil {
			return "", fmt.Errorf("failed to unmarshal response: %w", err)
		}
		uri, uriOk := responseMap["uri"].(string)
		if !uriOk {
			return "", fmt.Errorf("uri not found in response")
		}
		return uri, nil
	}

	// Handle direct map[string]any
	responseMap, ok := response.(map[string]any)
	if !ok {
		return "", fmt.Errorf("unexpected response format")
	}

	uri, ok := responseMap["uri"].(string)
	if !ok {
		return "", fmt.Errorf("uri not found in response")
	}

	return uri, nil
}

// findNewDevice finds a device that exists in current but not in initial list.
func findNewDevice(initial, current []Device) *Device {
	// Create a map of initial device IDs
	initialIDs := make(map[int]bool)
	for _, d := range initial {
		initialIDs[d.ID] = true
	}

	// Find new device
	for i := range current {
		if !initialIDs[current[i].ID] {
			return &current[i]
		}
	}

	return nil
}
