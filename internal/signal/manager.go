// Package signal provides Signal messenger integration via signal-cli.
package signal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Ensure ManagerImpl implements Manager interface.
var _ Manager = (*ManagerImpl)(nil)

const (
	// managerStartupTimeout is the default timeout for process startup.
	managerStartupTimeout = 30 * time.Second
	// managerShutdownTimeout is the default timeout for process shutdown.
	managerShutdownTimeout = 10 * time.Second
)

// ManagerImpl implements the Manager interface using dependency injection.
type ManagerImpl struct {
	config         Config
	processManager ProcessManager
	messenger      Messenger
	deviceManager  DeviceManager
	transport      Transport
	phoneNumber    string
	mu             sync.RWMutex
	isStarted      bool
	dataPath       string
}

// ManagerOption allows customization of SignalManager.
type ManagerOption func(*ManagerImpl)

// WithProcessManager sets a custom process manager.
func WithProcessManager(pm ProcessManager) ManagerOption {
	return func(sm *ManagerImpl) {
		sm.processManager = pm
	}
}

// WithMessenger sets a custom messenger.
func WithMessenger(m Messenger) ManagerOption {
	return func(sm *ManagerImpl) {
		sm.messenger = m
	}
}

// WithDeviceManager sets a custom device manager.
func WithDeviceManager(dm DeviceManager) ManagerOption {
	return func(sm *ManagerImpl) {
		sm.deviceManager = dm
	}
}

// WithTransport sets a custom transport.
func WithTransport(t Transport) ManagerOption {
	return func(sm *ManagerImpl) {
		sm.transport = t
	}
}

// WithDataPath sets a custom data path.
func WithDataPath(path string) ManagerOption {
	return func(sm *ManagerImpl) {
		sm.dataPath = path
	}
}

// NewSignalManager creates a new SignalManager with dependency injection.
func NewSignalManager(cfg Config, opts ...ManagerOption) *ManagerImpl {
	sm := &ManagerImpl{
		config:      cfg,
		phoneNumber: cfg.PhoneNumber,
	}

	// Apply options
	for _, opt := range opts {
		opt(sm)
	}

	// Set defaults if not provided
	if sm.dataPath == "" {
		sm.dataPath = filepath.Join("/var/lib/mentat/signal/data", cfg.PhoneNumber)
	}

	// Create process manager if not injected
	if sm.processManager == nil {
		sm.processManager = NewProcessManager(ProcessConfig{
			Command:         "signal-cli",
			DataPath:        sm.dataPath,
			PhoneNumber:     cfg.PhoneNumber,
			SocketPath:      cfg.SocketPath,
			StartupTimeout:  managerStartupTimeout,
			ShutdownTimeout: managerShutdownTimeout,
		})
	}

	// Create transport if not injected
	if sm.transport == nil && cfg.SocketPath != "" {
		transport, err := NewUnixSocketTransport(cfg.SocketPath)
		if err == nil {
			sm.transport = transport
		}
		// If transport creation failed, we'll handle this in Start()
	}

	// Create messenger if not injected
	if sm.messenger == nil && sm.transport != nil {
		client := NewClient(sm.transport, WithAccount(cfg.PhoneNumber))
		sm.messenger = NewMessenger(client, cfg.PhoneNumber)
	}

	// Device manager will be created on demand since it requires transport

	return sm
}

// Start initializes the Signal manager and starts the signal-cli process.
func (sm *ManagerImpl) Start(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.isStarted {
		return fmt.Errorf("signal manager already started")
	}

	// Check if signal-cli is available
	if !sm.isSignalCLIAvailable() {
		return fmt.Errorf("signal-cli not found in PATH")
	}

	// Ensure data directory exists
	if err := sm.ensureDataDirectory(); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Start the process
	if err := sm.processManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start signal-cli process: %w", err)
	}

	// Wait for process to be ready
	if err := sm.processManager.WaitForReady(ctx); err != nil {
		// Try to stop the process if startup failed
		_ = sm.processManager.Stop(ctx)
		return fmt.Errorf("signal-cli process failed to become ready: %w", err)
	}

	// Transport connects automatically when used, no action needed

	sm.isStarted = true
	return nil
}

// Stop gracefully shuts down the Signal manager.
func (sm *ManagerImpl) Stop(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.isStarted {
		return nil
	}

	var errs []error

	// Close transport
	if sm.transport != nil {
		if err := sm.transport.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close transport: %w", err))
		}
	}

	// Stop the process
	if err := sm.processManager.Stop(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop process: %w", err))
	}

	sm.isStarted = false

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
}

// HealthCheck verifies Signal connectivity and process health.
func (sm *ManagerImpl) HealthCheck(ctx context.Context) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if !sm.isStarted {
		return fmt.Errorf("signal manager not started")
	}

	// Check process health
	if !sm.processManager.IsRunning() {
		return fmt.Errorf("signal-cli process not running")
	}

	// Check transport connectivity if available
	if sm.transport != nil {
		// Try a simple RPC call to verify connectivity
		// We'll use listAccounts as a simple health check
		_, err := sm.transport.Call(ctx, "listAccounts", nil)
		if err != nil {
			return fmt.Errorf("RPC health check failed: %w", err)
		}
	}

	return nil
}

// GetMessenger returns the messaging interface.
func (sm *ManagerImpl) GetMessenger() Messenger {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.messenger
}

// GetDeviceManager returns the device management interface.
//
//nolint:ireturn // Required by interface
func (sm *ManagerImpl) GetDeviceManager() DeviceManager {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Device manager will be implemented in a future phase
	return sm.deviceManager
}

// GetPhoneNumber returns the configured phone number.
func (sm *ManagerImpl) GetPhoneNumber() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.phoneNumber
}

// IsRegistered checks if the phone number is registered with Signal.
func (sm *ManagerImpl) IsRegistered(ctx context.Context) (bool, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if !sm.isStarted {
		return false, fmt.Errorf("signal manager not started")
	}

	// Check if account data exists
	accountFile := filepath.Join(sm.dataPath, "account.db")
	if _, err := os.Stat(accountFile); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to check account file: %w", err)
	}

	// Also verify with signal-cli if transport is available
	if sm.transport != nil {
		_, err := sm.transport.Call(ctx, "listAccounts", nil)
		if err != nil {
			// If we get an error, it might mean not registered
			// For now, return false without error
			return false, nil
		}
		// Check if our phone number is in the response
		// This would require parsing the response properly
		// For now, assume registered if no error
		return true, nil
	}

	return true, nil
}

// Register performs Signal registration for a new phone number.
func (sm *ManagerImpl) Register(_ context.Context, phoneNumber string, _ string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.isStarted {
		return fmt.Errorf("signal manager not started")
	}

	// Update phone number if different
	if phoneNumber != sm.phoneNumber {
		sm.phoneNumber = phoneNumber
		sm.config.PhoneNumber = phoneNumber
	}

	// Registration would be handled by signal-cli commands
	// This is a placeholder for the actual implementation
	return fmt.Errorf("registration not yet implemented")
}

// VerifyCode completes registration with SMS verification code.
func (sm *ManagerImpl) VerifyCode(_ context.Context, _ string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.isStarted {
		return fmt.Errorf("signal manager not started")
	}

	// Verification would be handled by signal-cli commands
	// This is a placeholder for the actual implementation
	return fmt.Errorf("verification not yet implemented")
}

// isSignalCLIAvailable checks if signal-cli binary is available.
func (sm *ManagerImpl) isSignalCLIAvailable() bool {
	// First check if signal-cli is in PATH
	if _, err := exec.LookPath("signal-cli"); err == nil {
		return true
	}

	// Check common locations
	locations := []string{
		"/usr/bin/signal-cli",
		"/usr/local/bin/signal-cli",
		"/opt/signal-cli/bin/signal-cli",
	}

	for _, loc := range locations {
		if _, statErr := os.Stat(loc); statErr == nil {
			return true
		}
	}

	return false
}

// ensureDataDirectory creates the data directory if it doesn't exist.
func (sm *ManagerImpl) ensureDataDirectory() error {
	// Create directory with appropriate permissions
	if err := os.MkdirAll(sm.dataPath, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Verify we can write to it
	testFile := filepath.Join(sm.dataPath, ".test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return fmt.Errorf("data directory not writable: %w", err)
	}
	_ = os.Remove(testFile)

	return nil
}
