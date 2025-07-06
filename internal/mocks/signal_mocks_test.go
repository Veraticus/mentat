package mocks_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/mocks"
	"github.com/Veraticus/mentat/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time checks to ensure mocks implement their interfaces.
var (
	_ signal.Manager        = (*mocks.MockSignalManager)(nil)
	_ signal.ProcessManager = (*mocks.MockProcessManager)(nil)
	_ signal.DeviceManager  = (*mocks.MockDeviceManager)(nil)
)

func TestMockSignalManager(t *testing.T) {
	ctx := context.Background()

	t.Run("Start and Stop", func(t *testing.T) {
		m := mocks.NewMockSignalManager()

		// Initially not started or stopped
		assert.False(t, m.IsStarted())
		assert.False(t, m.IsStopped())

		// Start the manager
		err := m.Start(ctx)
		require.NoError(t, err)
		assert.True(t, m.IsStarted())

		// Stop the manager
		err = m.Stop(ctx)
		require.NoError(t, err)
		assert.True(t, m.IsStopped())
	})

	t.Run("Health Check", func(t *testing.T) {
		m := mocks.NewMockSignalManager()

		// Health check succeeds by default
		err := m.HealthCheck(ctx)
		require.NoError(t, err)

		// Set health error
		healthErr := fmt.Errorf("health check failed")
		m.SetHealthError(healthErr)

		// Health check should now fail
		err = m.HealthCheck(ctx)
		require.Equal(t, healthErr, err)
	})

	t.Run("Registration", func(t *testing.T) {
		m := mocks.NewMockSignalManager()

		// Initially not registered
		registered, err := m.IsRegistered(ctx)
		require.NoError(t, err)
		assert.False(t, registered)

		// Register with phone number
		phoneNumber := "+1234567890"
		captchaToken := "test-token"
		err = m.Register(ctx, phoneNumber, captchaToken)
		require.NoError(t, err)

		// Should now be registered
		registered, err = m.IsRegistered(ctx)
		require.NoError(t, err)
		assert.True(t, registered)
		assert.Equal(t, phoneNumber, m.GetPhoneNumber())

		// Test registration failure
		m2 := mocks.NewMockSignalManager()
		registerErr := fmt.Errorf("captcha required")
		m2.SetRegisterError(registerErr)

		err = m2.Register(ctx, phoneNumber, captchaToken)
		assert.Equal(t, registerErr, err)
	})

	t.Run("Verification", func(t *testing.T) {
		m := mocks.NewMockSignalManager()

		// Verify code
		err := m.VerifyCode(ctx, "123456")
		require.NoError(t, err)

		// Should be registered after verification
		registered, err := m.IsRegistered(ctx)
		require.NoError(t, err)
		assert.True(t, registered)

		// Test verification failure
		m2 := mocks.NewMockSignalManager()
		verifyErr := fmt.Errorf("invalid code")
		m2.SetVerifyError(verifyErr)

		err = m2.VerifyCode(ctx, "123456")
		assert.Equal(t, verifyErr, err)
	})

	t.Run("GetMessenger and GetDeviceManager", func(t *testing.T) {
		m := mocks.NewMockSignalManager()

		// Get messenger
		messenger := m.GetMessenger()
		assert.NotNil(t, messenger)

		// Verify it's a mock messenger we can use
		err := messenger.Send(ctx, "+1234567890", "test message")
		require.NoError(t, err)

		// Get device manager
		deviceManager := m.GetDeviceManager()
		assert.NotNil(t, deviceManager)

		// Verify it's a mock device manager we can use
		devices, err := deviceManager.ListDevices(ctx)
		require.NoError(t, err)
		assert.Len(t, devices, 2) // Default mock has 2 devices
	})
}

func TestMockProcessManager(t *testing.T) {
	ctx := context.Background()

	t.Run("Start and Stop", func(t *testing.T) {
		m := mocks.NewMockProcessManager()

		// Initially not running
		assert.False(t, m.IsRunning())
		assert.Equal(t, 0, m.GetPID())

		// Start the process
		err := m.Start(ctx)
		require.NoError(t, err)
		assert.True(t, m.IsRunning())
		assert.NotEqual(t, 0, m.GetPID())
		assert.Equal(t, 1, m.GetStartCalls())

		// Stop the process
		err = m.Stop(ctx)
		require.NoError(t, err)
		assert.False(t, m.IsRunning())
		assert.Equal(t, 0, m.GetPID())
		assert.Equal(t, 1, m.GetStopCalls())
	})

	t.Run("Restart", func(t *testing.T) {
		m := mocks.NewMockProcessManager()
		m.SetRunning(true)

		// Restart the process
		err := m.Restart(ctx)
		require.NoError(t, err)
		assert.True(t, m.IsRunning())
	})

	t.Run("WaitForReady", func(t *testing.T) {
		m := mocks.NewMockProcessManager()

		// Process not running, should fail
		err := m.WaitForReady(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "process not running")

		// Start process and try again
		err = m.Start(ctx)
		require.NoError(t, err)

		err = m.WaitForReady(ctx)
		require.NoError(t, err)

		// Test ready error
		m2 := mocks.NewMockProcessManager()
		m2.SetRunning(true)
		readyErr := fmt.Errorf("not ready")
		m2.SetReadyError(readyErr)

		err = m2.WaitForReady(ctx)
		assert.Equal(t, readyErr, err)
	})

	t.Run("Error Injection", func(t *testing.T) {
		m := mocks.NewMockProcessManager()

		// Set start error
		startErr := fmt.Errorf("failed to start")
		m.SetStartError(startErr)
		err := m.Start(ctx)
		assert.Equal(t, startErr, err)

		// Set stop error
		m.SetRunning(true)
		stopErr := fmt.Errorf("failed to stop")
		m.SetStopError(stopErr)
		err = m.Stop(ctx)
		assert.Equal(t, stopErr, err)
	})
}

func TestMockDeviceManager(t *testing.T) {
	ctx := context.Background()

	t.Run("ListDevices", func(t *testing.T) {
		m := mocks.NewMockDeviceManager()

		// List devices
		devices, err := m.ListDevices(ctx)
		require.NoError(t, err)
		assert.Len(t, devices, 2)

		// Verify devices
		assert.True(t, devices[0].Primary)
		assert.Equal(t, "Primary Device", devices[0].Name)
		assert.False(t, devices[1].Primary)
		assert.Equal(t, "Desktop", devices[1].Name)

		// Test list error
		m2 := mocks.NewMockDeviceManager()
		listErr := fmt.Errorf("failed to list")
		m2.SetListError(listErr)

		_, err = m2.ListDevices(ctx)
		assert.Equal(t, listErr, err)
	})

	t.Run("RemoveDevice", func(t *testing.T) {
		m := mocks.NewMockDeviceManager()

		// Initial device count
		devices, err := m.ListDevices(ctx)
		require.NoError(t, err)
		initialCount := len(devices)

		// Remove a device
		deviceID := 2
		err = m.RemoveDevice(ctx, deviceID)
		require.NoError(t, err)

		// Verify device was removed
		devices, err = m.ListDevices(ctx)
		require.NoError(t, err)
		assert.Len(t, devices, initialCount-1)

		// Verify removed device ID was tracked
		removedIDs := m.GetRemovedDeviceIDs()
		assert.Contains(t, removedIDs, deviceID)

		// Test remove error
		m2 := mocks.NewMockDeviceManager()
		removeErr := fmt.Errorf("cannot remove primary")
		m2.SetRemoveError(removeErr)

		err = m2.RemoveDevice(ctx, 1)
		assert.Equal(t, removeErr, err)
	})

	t.Run("GenerateLinkingURI", func(t *testing.T) {
		m := mocks.NewMockDeviceManager()

		// Generate linking URI
		deviceName := "Test Device"
		uri, err := m.GenerateLinkingURI(ctx, deviceName)
		require.NoError(t, err)
		assert.Contains(t, uri, "signal://link")
		assert.Contains(t, uri, deviceName)

		// Test link error
		m2 := mocks.NewMockDeviceManager()
		linkErr := fmt.Errorf("linking disabled")
		m2.SetLinkError(linkErr)

		_, err = m2.GenerateLinkingURI(ctx, deviceName)
		assert.Equal(t, linkErr, err)
	})

	t.Run("GetPrimaryDevice", func(t *testing.T) {
		m := mocks.NewMockDeviceManager()

		// Get primary device
		primary, err := m.GetPrimaryDevice(ctx)
		require.NoError(t, err)
		assert.NotNil(t, primary)
		assert.True(t, primary.Primary)
		assert.Equal(t, "Primary Device", primary.Name)
	})

	t.Run("WaitForDeviceLink", func(t *testing.T) {
		m := mocks.NewMockDeviceManager()

		// Initial device count
		devices, err := m.ListDevices(ctx)
		require.NoError(t, err)
		initialCount := len(devices)

		// Wait for device link (simulates successful linking)
		timeout := 30 * time.Second
		device, err := m.WaitForDeviceLink(ctx, timeout)
		require.NoError(t, err)
		assert.NotNil(t, device)
		assert.False(t, device.Primary)

		// Verify new device was added
		devices, err = m.ListDevices(ctx)
		require.NoError(t, err)
		assert.Len(t, devices, initialCount+1)

		// Test with preset linked device
		m2 := mocks.NewMockDeviceManager()
		presetDevice := &signal.Device{
			ID:      99,
			Name:    "Preset Device",
			Primary: false,
		}
		m2.SetLinkedDevice(presetDevice)

		device, err = m2.WaitForDeviceLink(ctx, timeout)
		require.NoError(t, err)
		assert.Equal(t, presetDevice.ID, device.ID)
		assert.Equal(t, presetDevice.Name, device.Name)

		// Test wait error
		m3 := mocks.NewMockDeviceManager()
		waitErr := fmt.Errorf("timeout waiting for device")
		m3.SetWaitError(waitErr)

		_, err = m3.WaitForDeviceLink(ctx, timeout)
		assert.Equal(t, waitErr, err)
	})

	t.Run("SetDevices", func(t *testing.T) {
		m := mocks.NewMockDeviceManager()

		// Set custom device list
		customDevices := []signal.Device{
			{ID: 10, Name: "Custom 1", Primary: true},
			{ID: 20, Name: "Custom 2", Primary: false},
			{ID: 30, Name: "Custom 3", Primary: false},
		}
		m.SetDevices(customDevices)

		// Verify custom devices
		devices, err := m.ListDevices(ctx)
		require.NoError(t, err)
		assert.Len(t, devices, len(customDevices))
		assert.Equal(t, customDevices[0].Name, devices[0].Name)
	})
}
