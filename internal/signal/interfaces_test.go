package signal_test

import (
	"context"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// Verify that all interfaces can be mocked.
// This test ensures interface definitions are properly designed.

// testMessenger implements the Messenger interface for interface compilation tests.
type testMessenger struct {
	sendFunc       func(ctx context.Context, recipient string, message string) error
	sendTypingFunc func(ctx context.Context, recipient string) error
	subscribeFunc  func(ctx context.Context) (<-chan signal.IncomingMessage, error)
}

func (m *testMessenger) Send(ctx context.Context, recipient string, message string) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, recipient, message)
	}
	return nil
}

func (m *testMessenger) SendTypingIndicator(ctx context.Context, recipient string) error {
	if m.sendTypingFunc != nil {
		return m.sendTypingFunc(ctx, recipient)
	}
	return nil
}

func (m *testMessenger) Subscribe(ctx context.Context) (<-chan signal.IncomingMessage, error) {
	if m.subscribeFunc != nil {
		return m.subscribeFunc(ctx)
	}
	ch := make(chan signal.IncomingMessage)
	close(ch)
	return ch, nil
}

// testManager implements the Manager interface.
type testManager struct {
	startFunc            func(ctx context.Context) error
	stopFunc             func(ctx context.Context) error
	healthCheckFunc      func(ctx context.Context) error
	getMessengerFunc     func() signal.Messenger
	getDeviceManagerFunc func() signal.DeviceManager
	getPhoneNumberFunc   func() string
	isRegisteredFunc     func(ctx context.Context) (bool, error)
	registerFunc         func(ctx context.Context, phoneNumber string, captchaToken string) error
	verifyCodeFunc       func(ctx context.Context, code string) error
}

func (m *testManager) Start(ctx context.Context) error {
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}
	return nil
}

func (m *testManager) Stop(ctx context.Context) error {
	if m.stopFunc != nil {
		return m.stopFunc(ctx)
	}
	return nil
}

func (m *testManager) HealthCheck(ctx context.Context) error {
	if m.healthCheckFunc != nil {
		return m.healthCheckFunc(ctx)
	}
	return nil
}

func (m *testManager) GetMessenger() signal.Messenger {
	if m.getMessengerFunc != nil {
		return m.getMessengerFunc()
	}
	return &testMessenger{}
}

func (m *testManager) GetDeviceManager() signal.DeviceManager { //nolint:ireturn // test mock returns interface by design
	if m.getDeviceManagerFunc != nil {
		return m.getDeviceManagerFunc()
	}
	return &testDeviceManager{}
}

func (m *testManager) GetPhoneNumber() string {
	if m.getPhoneNumberFunc != nil {
		return m.getPhoneNumberFunc()
	}
	return "+1234567890"
}

func (m *testManager) IsRegistered(ctx context.Context) (bool, error) {
	if m.isRegisteredFunc != nil {
		return m.isRegisteredFunc(ctx)
	}
	return true, nil
}

func (m *testManager) Register(ctx context.Context, phoneNumber string, captchaToken string) error {
	if m.registerFunc != nil {
		return m.registerFunc(ctx, phoneNumber, captchaToken)
	}
	return nil
}

func (m *testManager) VerifyCode(ctx context.Context, code string) error {
	if m.verifyCodeFunc != nil {
		return m.verifyCodeFunc(ctx, code)
	}
	return nil
}

// testProcessManager implements the ProcessManager interface.
type testProcessManager struct {
	startFunc        func(ctx context.Context) error
	stopFunc         func(ctx context.Context) error
	restartFunc      func(ctx context.Context) error
	isRunningFunc    func() bool
	getPIDFunc       func() int
	waitForReadyFunc func(ctx context.Context) error
}

func (m *testProcessManager) Start(ctx context.Context) error {
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}
	return nil
}

func (m *testProcessManager) Stop(ctx context.Context) error {
	if m.stopFunc != nil {
		return m.stopFunc(ctx)
	}
	return nil
}

func (m *testProcessManager) Restart(ctx context.Context) error {
	if m.restartFunc != nil {
		return m.restartFunc(ctx)
	}
	return nil
}

func (m *testProcessManager) IsRunning() bool {
	if m.isRunningFunc != nil {
		return m.isRunningFunc()
	}
	return true
}

func (m *testProcessManager) GetPID() int {
	if m.getPIDFunc != nil {
		return m.getPIDFunc()
	}
	return 12345
}

func (m *testProcessManager) WaitForReady(ctx context.Context) error {
	if m.waitForReadyFunc != nil {
		return m.waitForReadyFunc(ctx)
	}
	return nil
}

// testDeviceManager implements the DeviceManager interface.
type testDeviceManager struct {
	listDevicesFunc        func(ctx context.Context) ([]signal.Device, error)
	removeDeviceFunc       func(ctx context.Context, deviceID int) error
	generateLinkingURIFunc func(ctx context.Context, deviceName string) (string, error)
	getPrimaryDeviceFunc   func(ctx context.Context) (*signal.Device, error)
	waitForDeviceLinkFunc  func(ctx context.Context, timeout time.Duration) (*signal.Device, error)
}

func (m *testDeviceManager) ListDevices(ctx context.Context) ([]signal.Device, error) {
	if m.listDevicesFunc != nil {
		return m.listDevicesFunc(ctx)
	}
	return []signal.Device{}, nil
}

func (m *testDeviceManager) RemoveDevice(ctx context.Context, deviceID int) error {
	if m.removeDeviceFunc != nil {
		return m.removeDeviceFunc(ctx, deviceID)
	}
	return nil
}

func (m *testDeviceManager) GenerateLinkingURI(ctx context.Context, deviceName string) (string, error) {
	if m.generateLinkingURIFunc != nil {
		return m.generateLinkingURIFunc(ctx, deviceName)
	}
	return "signal://link?device=test", nil
}

func (m *testDeviceManager) GetPrimaryDevice(ctx context.Context) (*signal.Device, error) {
	if m.getPrimaryDeviceFunc != nil {
		return m.getPrimaryDeviceFunc(ctx)
	}
	return &signal.Device{
		ID:      1,
		Name:    "Primary",
		Primary: true,
	}, nil
}

func (m *testDeviceManager) WaitForDeviceLink(ctx context.Context, timeout time.Duration) (*signal.Device, error) {
	if m.waitForDeviceLinkFunc != nil {
		return m.waitForDeviceLinkFunc(ctx, timeout)
	}
	return &signal.Device{
		ID:   2,
		Name: "New Device",
	}, nil
}

// TestInterfaceCompilation verifies that all interfaces compile
// and can be implemented by mock types.
func TestInterfaceCompilation(_ *testing.T) {
	// Verify Messenger interface
	var _ signal.Messenger = (*testMessenger)(nil)

	// Verify Manager interface
	var _ signal.Manager = (*testManager)(nil)

	// Verify ProcessManager interface
	var _ signal.ProcessManager = (*testProcessManager)(nil)

	// Verify DeviceManager interface
	var _ signal.DeviceManager = (*testDeviceManager)(nil)
}

// TestInterfaceUsage verifies basic usage of the interfaces.
func TestInterfaceUsage(t *testing.T) {
	ctx := context.Background()

	// Test Messenger
	messenger := &testMessenger{
		sendFunc: func(_ context.Context, recipient string, message string) error {
			if recipient == "" || message == "" {
				t.Error("Expected non-empty recipient and message")
			}
			return nil
		},
	}

	err := messenger.Send(ctx, "+1234567890", "Test message")
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	// Test Manager
	manager := &testManager{
		getPhoneNumberFunc: func() string {
			return "+1234567890"
		},
	}

	phoneNumber := manager.GetPhoneNumber()
	if phoneNumber != "+1234567890" {
		t.Errorf("Expected phone number +1234567890, got %s", phoneNumber)
	}

	// Test ProcessManager
	processManager := &testProcessManager{
		isRunningFunc: func() bool {
			return true
		},
		getPIDFunc: func() int {
			return 12345
		},
	}

	if !processManager.IsRunning() {
		t.Error("Expected process to be running")
	}

	if pid := processManager.GetPID(); pid != 12345 {
		t.Errorf("Expected PID 12345, got %d", pid)
	}

	// Test DeviceManager
	deviceManager := &testDeviceManager{
		listDevicesFunc: func(_ context.Context) ([]signal.Device, error) {
			return []signal.Device{
				{ID: 1, Name: "Device 1", Primary: true},
				{ID: 2, Name: "Device 2", Primary: false},
			}, nil
		},
	}

	devices, err := deviceManager.ListDevices(ctx)
	if err != nil {
		t.Errorf("ListDevices failed: %v", err)
	}

	if len(devices) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(devices))
	}
}
