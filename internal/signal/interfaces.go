// Package signal provides interfaces for Signal messenger integration.
package signal

import (
	"context"
	"time"
)

// Messenger abstracts Signal communication and future messaging channels.
type Messenger interface {
	// Send sends a message to the specified recipient
	Send(ctx context.Context, recipient string, message string) error

	// SendTypingIndicator sends a typing indicator to the recipient
	SendTypingIndicator(ctx context.Context, recipient string) error

	// Subscribe returns a channel of incoming messages
	Subscribe(ctx context.Context) (<-chan IncomingMessage, error)
}

// Manager orchestrates the complete Signal integration including
// registration, device management, and health monitoring.
type Manager interface {
	// Start initializes the Signal manager and starts the signal-cli process
	Start(ctx context.Context) error

	// Stop gracefully shuts down the Signal manager
	Stop(ctx context.Context) error

	// HealthCheck verifies Signal connectivity and process health
	HealthCheck(ctx context.Context) error

	// GetMessenger returns the messaging interface
	GetMessenger() Messenger

	// GetDeviceManager returns the device management interface
	GetDeviceManager() DeviceManager

	// GetPhoneNumber returns the configured phone number
	GetPhoneNumber() string

	// IsRegistered checks if the phone number is registered with Signal
	IsRegistered(ctx context.Context) (bool, error)

	// Register performs Signal registration for a new phone number
	Register(ctx context.Context, phoneNumber string, captchaToken string) error

	// VerifyCode completes registration with SMS verification code
	VerifyCode(ctx context.Context, code string) error
}

// ProcessManager abstracts subprocess lifecycle management for signal-cli.
type ProcessManager interface {
	// Start launches the managed process
	Start(ctx context.Context) error

	// Stop terminates the managed process gracefully
	Stop(ctx context.Context) error

	// Restart performs a graceful restart of the process
	Restart(ctx context.Context) error

	// IsRunning checks if the process is currently running
	IsRunning() bool

	// GetPID returns the process ID if running, 0 otherwise
	GetPID() int

	// WaitForReady waits for the process to be ready to accept connections
	WaitForReady(ctx context.Context) error
}

// DeviceManager handles Signal device operations including listing,
// removing, and linking devices.
type DeviceManager interface {
	// ListDevices returns all linked devices for the account
	ListDevices(ctx context.Context) ([]Device, error)

	// RemoveDevice removes a linked device by ID
	RemoveDevice(ctx context.Context, deviceID int) error

	// GenerateLinkingURI creates a linking URI for adding a new device
	GenerateLinkingURI(ctx context.Context, deviceName string) (string, error)

	// GetPrimaryDevice returns the primary device information
	GetPrimaryDevice(ctx context.Context) (*Device, error)

	// WaitForDeviceLink waits for a device to complete linking
	// Returns the linked device or an error if timeout/canceled
	WaitForDeviceLink(ctx context.Context, timeout time.Duration) (*Device, error)
}
