package signal_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/mocks"
	"github.com/Veraticus/mentat/internal/signal"
)

// TestNewSignalManager tests the manager constructor.
func TestNewSignalManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  signal.Config
		options []signal.ManagerOption
		check   func(t *testing.T, sm *signal.ManagerImpl)
	}{
		{
			name: "default configuration",
			config: signal.Config{
				PhoneNumber: "+1234567890",
				SocketPath:  "/tmp/signal.sock",
			},
			check: func(t *testing.T, sm *signal.ManagerImpl) {
				t.Helper()
				if sm.GetPhoneNumber() != "+1234567890" {
					t.Errorf("GetPhoneNumber() = %v, want %v", sm.GetPhoneNumber(), "+1234567890")
				}
			},
		},
		{
			name: "with process manager option",
			config: signal.Config{
				PhoneNumber: "+1234567890",
			},
			options: []signal.ManagerOption{
				signal.WithProcessManager(mocks.NewMockProcessManager()),
			},
			check: func(_ *testing.T, _ *signal.ManagerImpl) {
				// Process manager should be set
			},
		},
		{
			name: "with messenger option",
			config: signal.Config{
				PhoneNumber: "+1234567890",
			},
			options: []signal.ManagerOption{
				signal.WithMessenger(mocks.NewMockMessenger()),
			},
			check: func(t *testing.T, sm *signal.ManagerImpl) {
				t.Helper()
				if sm.GetMessenger() == nil {
					t.Error("GetMessenger() returned nil")
				}
			},
		},
		{
			name: "with data path option",
			config: signal.Config{
				PhoneNumber: "+1234567890",
			},
			options: []signal.ManagerOption{
				signal.WithDataPath("/custom/path"),
			},
			check: func(_ *testing.T, _ *signal.ManagerImpl) {
				// Data path is internal, can't directly verify
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sm := signal.NewSignalManager(tt.config, tt.options...)
			if sm == nil {
				t.Fatal("NewSignalManager returned nil")
			}

			if tt.check != nil {
				tt.check(t, sm)
			}
		})
	}
}

// TestManagerLifecycle tests Start/Stop operations.
func TestManagerLifecycle(t *testing.T) {
	tests := []struct {
		name         string
		setupMocks   func() (*mocks.MockProcessManager, *signal.MockTransport)
		wantStartErr bool
		wantStopErr  bool
	}{
		{
			name: "successful start and stop",
			setupMocks: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				pm := mocks.NewMockProcessManager()
				pm.SetRunning(true)
				transport := signal.NewMockTransport()
				return pm, transport
			},
		},
		{
			name: "start fails when already started",
			setupMocks: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				pm := mocks.NewMockProcessManager()
				pm.SetRunning(true)
				return pm, nil
			},
			wantStartErr: false, // First start should succeed, second should fail
		},
		{
			name: "process start failure",
			setupMocks: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				pm := mocks.NewMockProcessManager()
				pm.SetStartError(fmt.Errorf("failed to start"))
				return pm, nil
			},
			wantStartErr: true,
		},
		{
			name: "process ready timeout",
			setupMocks: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				pm := mocks.NewMockProcessManager()
				pm.SetReadyError(fmt.Errorf("timeout"))
				return pm, nil
			},
			wantStartErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for testing
			tmpDir := t.TempDir()
			dataPath := filepath.Join(tmpDir, "signal-data")

			// Create signal-cli executable for testing
			signalCLI := filepath.Join(tmpDir, "signal-cli")
			if err := os.WriteFile(signalCLI, []byte("#!/bin/sh\necho test"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

			pm, transport := tt.setupMocks()

			config := signal.Config{
				PhoneNumber: "+1234567890",
				SocketPath:  "/tmp/test.sock",
			}

			opts := []signal.ManagerOption{
				signal.WithProcessManager(pm),
				signal.WithDataPath(dataPath),
			}
			if transport != nil {
				opts = append(opts, signal.WithTransport(transport))
			}

			sm := signal.NewSignalManager(config, opts...)

			ctx := context.Background()

			// Test Start
			err := sm.Start(ctx)
			if (err != nil) != tt.wantStartErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantStartErr)
			}

			// If testing double start
			if tt.name == "start fails when already started" && err == nil {
				err = sm.Start(ctx)
				if err == nil {
					t.Error("Second Start() should have failed")
				}
			}

			// Test Stop
			err = sm.Stop(ctx)
			if (err != nil) != tt.wantStopErr {
				t.Errorf("Stop() error = %v, wantErr %v", err, tt.wantStopErr)
			}

			// Stop should be idempotent
			err = sm.Stop(ctx)
			if err != nil {
				t.Errorf("Second Stop() failed: %v", err)
			}
		})
	}
}

// TestManagerHealthCheck tests health check functionality.
func TestManagerHealthCheck(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() (*mocks.MockProcessManager, *signal.MockTransport)
		started   bool
		wantError string
	}{
		{
			name: "health check when not started",
			setup: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				return mocks.NewMockProcessManager(), nil
			},
			started:   false,
			wantError: "signal manager not started",
		},
		{
			name: "health check with running process",
			setup: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				pm := mocks.NewMockProcessManager()
				pm.SetRunning(true)
				transport := signal.NewMockTransport()
				transport.SetResponse("listAccounts", nil, nil)
				return pm, transport
			},
			started: true,
		},
		{
			name: "health check with stopped process",
			setup: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				pm := mocks.NewMockProcessManager()
				pm.SetRunning(false)
				return pm, nil
			},
			started:   true,
			wantError: "signal-cli process not running",
		},
		{
			name: "health check with RPC failure",
			setup: func() (*mocks.MockProcessManager, *signal.MockTransport) {
				pm := mocks.NewMockProcessManager()
				pm.SetRunning(true)
				transport := signal.NewMockTransport()
				transport.SetError("listAccounts", fmt.Errorf("RPC error"))
				return pm, transport
			},
			started:   true,
			wantError: "RPC health check failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			signalCLI := filepath.Join(tmpDir, "signal-cli")
			if err := os.WriteFile(signalCLI, []byte("#!/bin/sh\necho test"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

			pm, transport := tt.setup()

			config := signal.Config{
				PhoneNumber: "+1234567890",
			}

			opts := []signal.ManagerOption{
				signal.WithProcessManager(pm),
				signal.WithDataPath(filepath.Join(tmpDir, "data")),
			}
			if transport != nil {
				opts = append(opts, signal.WithTransport(transport))
			}

			sm := signal.NewSignalManager(config, opts...)

			ctx := context.Background()

			if tt.started {
				// For health check test, we need to manually mark as started
				// since we're using mocks
				pm.SetRunning(true)
				if err := sm.Start(ctx); err != nil {
					t.Fatalf("Start() failed: %v", err)
				}
				defer sm.Stop(ctx)

				// For stopped process test, simulate process crash after start
				if tt.name == "health check with stopped process" {
					pm.SetRunning(false)
				}
			}

			err := sm.HealthCheck(ctx)
			if tt.wantError != "" {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("HealthCheck() error = %v, want containing %v", err, tt.wantError)
				}
			} else if err != nil {
				t.Errorf("HealthCheck() unexpected error: %v", err)
			}
		})
	}
}

// TestManagerIsRegistered tests registration checking.
func TestManagerIsRegistered(t *testing.T) {
	tests := []struct {
		name          string
		accountExists bool
		transport     *signal.MockTransport
		started       bool
		want          bool
		wantError     bool
	}{
		{
			name:      "not started",
			started:   false,
			wantError: true,
		},
		{
			name:          "account file exists",
			accountExists: true,
			started:       true,
			want:          true,
		},
		{
			name:          "account file missing",
			accountExists: false,
			started:       true,
			want:          false,
		},
		{
			name:          "with transport success",
			accountExists: true,
			transport: func() *signal.MockTransport {
				t := signal.NewMockTransport()
				t.SetResponse("listAccounts", nil, nil)
				return t
			}(),
			started: true,
			want:    true,
		},
		{
			name:          "with transport error",
			accountExists: false,
			transport: func() *signal.MockTransport {
				t := signal.NewMockTransport()
				t.SetError("listAccounts", fmt.Errorf("not registered"))
				return t
			}(),
			started: true,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			dataPath := filepath.Join(tmpDir, "data")

			// Create signal-cli for testing
			signalCLI := filepath.Join(tmpDir, "signal-cli")
			if err := os.WriteFile(signalCLI, []byte("#!/bin/sh\necho test"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

			// Create account file if needed
			if tt.accountExists {
				os.MkdirAll(dataPath, 0755)
				accountFile := filepath.Join(dataPath, "account.db")
				os.WriteFile(accountFile, []byte("test"), 0644)
			}

			pm := mocks.NewMockProcessManager()
			pm.SetRunning(true)

			config := signal.Config{
				PhoneNumber: "+1234567890",
			}

			opts := []signal.ManagerOption{
				signal.WithProcessManager(pm),
				signal.WithDataPath(dataPath),
			}
			if tt.transport != nil {
				opts = append(opts, signal.WithTransport(tt.transport))
			}

			sm := signal.NewSignalManager(config, opts...)

			ctx := context.Background()

			if tt.started {
				if err := sm.Start(ctx); err != nil {
					t.Fatalf("Start() failed: %v", err)
				}
				defer sm.Stop(ctx)
			}

			got, err := sm.IsRegistered(ctx)
			if (err != nil) != tt.wantError {
				t.Errorf("IsRegistered() error = %v, wantErr %v", err, tt.wantError)
			}
			if got != tt.want {
				t.Errorf("IsRegistered() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestManagerConcurrency tests concurrent access to manager.
func TestManagerConcurrency(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	signalCLI := filepath.Join(tmpDir, "signal-cli")
	if err := os.WriteFile(signalCLI, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	pm := mocks.NewMockProcessManager()
	pm.SetRunning(true)

	transport := signal.NewMockTransport()
	transport.SetResponse("listAccounts", nil, nil)

	config := signal.Config{
		PhoneNumber: "+1234567890",
	}

	sm := signal.NewSignalManager(config,
		signal.WithProcessManager(pm),
		signal.WithTransport(transport),
		signal.WithDataPath(filepath.Join(tmpDir, "data")),
	)

	ctx := context.Background()
	if err := sm.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Run concurrent operations
	done := make(chan bool, 4)

	// Concurrent health checks
	go func() {
		for range 10 {
			_ = sm.HealthCheck(ctx)
		}
		done <- true
	}()

	// Concurrent getters
	go func() {
		for range 10 {
			_ = sm.GetPhoneNumber()
			_ = sm.GetMessenger()
			_ = sm.GetDeviceManager()
		}
		done <- true
	}()

	// Concurrent IsRegistered checks
	go func() {
		for range 10 {
			_, _ = sm.IsRegistered(ctx)
		}
		done <- true
	}()

	// Stop/Start cycle with slight delay
	go func() {
		// Use a timer for the delay
		timer := time.NewTimer(5 * time.Millisecond)
		<-timer.C
		_ = sm.Stop(ctx)
		done <- true
	}()

	// Wait for all goroutines
	for range 4 {
		<-done
	}
}

// TestManagerNoSignalCLI tests behavior when signal-cli is not available.
func TestManagerNoSignalCLI(t *testing.T) {
	// Create empty temp directory (no signal-cli)
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir)

	config := signal.Config{
		PhoneNumber: "+1234567890",
	}

	sm := signal.NewSignalManager(config)

	ctx := context.Background()
	err := sm.Start(ctx)
	if err == nil {
		t.Fatal("Expected error when signal-cli not found")
	}
	if !strings.Contains(err.Error(), "signal-cli not found") {
		t.Errorf("Start() error = %v, want containing 'signal-cli not found'", err)
	}
}

// TestManagerDataDirectory tests data directory handling.
func TestManagerDataDirectory(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	signalCLI := filepath.Join(tmpDir, "signal-cli")
	if err := os.WriteFile(signalCLI, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	// Test with read-only directory
	roDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(roDir, 0755)
	os.Chmod(roDir, 0444)
	defer os.Chmod(roDir, 0755) // Cleanup

	pm := mocks.NewMockProcessManager()

	config := signal.Config{
		PhoneNumber: "+1234567890",
	}

	sm := signal.NewSignalManager(config,
		signal.WithProcessManager(pm),
		signal.WithDataPath(roDir),
	)

	ctx := context.Background()
	err := sm.Start(ctx)
	if err == nil {
		t.Fatal("Expected error with read-only directory")
	}
	if !strings.Contains(err.Error(), "data directory") {
		t.Errorf("Start() error = %v, want containing 'data directory'", err)
	}
}

// TestManagerRegistrationMethods tests Register and VerifyCode placeholders.
func TestManagerRegistrationMethods(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	signalCLI := filepath.Join(tmpDir, "signal-cli")
	if err := os.WriteFile(signalCLI, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	pm := mocks.NewMockProcessManager()
	pm.SetRunning(true)

	config := signal.Config{
		PhoneNumber: "+1234567890",
	}

	sm := signal.NewSignalManager(config,
		signal.WithProcessManager(pm),
		signal.WithDataPath(filepath.Join(tmpDir, "data")),
	)

	ctx := context.Background()
	if err := sm.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer sm.Stop(ctx)

	// Test Register (placeholder)
	err := sm.Register(ctx, "+9876543210", "captcha123")
	if err == nil {
		t.Error("Register() should return not implemented error")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("Register() error = %v, want containing 'not yet implemented'", err)
	}

	// Test VerifyCode (placeholder)
	err = sm.VerifyCode(ctx, "123456")
	if err == nil {
		t.Error("VerifyCode() should return not implemented error")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("VerifyCode() error = %v, want containing 'not yet implemented'", err)
	}
}

// TestManagerGetters tests the getter methods.
func TestManagerGetters(t *testing.T) {
	t.Parallel()

	messenger := mocks.NewMockMessenger()
	deviceManager := mocks.NewMockDeviceManager()

	config := signal.Config{
		PhoneNumber: "+1234567890",
	}

	sm := signal.NewSignalManager(config,
		signal.WithMessenger(messenger),
		signal.WithDeviceManager(deviceManager),
	)

	// Test GetPhoneNumber
	if got := sm.GetPhoneNumber(); got != "+1234567890" {
		t.Errorf("GetPhoneNumber() = %v, want %v", got, "+1234567890")
	}

	// Test GetMessenger
	if got := sm.GetMessenger(); got != messenger {
		t.Error("GetMessenger() returned different instance")
	}

	// Test GetDeviceManager
	if got := sm.GetDeviceManager(); got != deviceManager {
		t.Error("GetDeviceManager() returned different instance")
	}
}
