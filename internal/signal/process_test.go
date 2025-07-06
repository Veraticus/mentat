package signal_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/mentat/internal/signal"
)

// TestProcessManagerLifecycle tests basic start/stop/restart functionality.
func TestProcessManagerLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		command    string
		args       []string
		wantErr    bool
		skipReason string
	}{
		{
			name:    "successful start and stop",
			command: "echo",
			args:    []string{"test"},
		},
		{
			name:    "default command signal-cli",
			command: "",
			// Empty command defaults to signal-cli, which won't exist in test env
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}

			pm := signal.NewProcessManager(signal.ProcessConfig{
				Command:         tt.command,
				Args:            tt.args,
				StartupTimeout:  100 * time.Millisecond,
				ShutdownTimeout: 100 * time.Millisecond,
			})

			ctx := context.Background()

			// Test Start
			err := pm.Start(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			// Verify process is running
			if !pm.IsRunning() {
				t.Error("Process should be running after Start()")
			}

			pid := pm.GetPID()
			if pid == 0 {
				t.Error("GetPID() should return non-zero PID when running")
			}

			// Test Stop
			if stopErr := pm.Stop(ctx); stopErr != nil {
				t.Errorf("Stop() error = %v", stopErr)
			}

			// Verify process is stopped
			if pm.IsRunning() {
				t.Error("Process should not be running after Stop()")
			}

			if pm.GetPID() != 0 {
				t.Error("GetPID() should return 0 when stopped")
			}
		})
	}
}

// TestProcessManagerStartErrors tests various start error conditions.
func TestProcessManagerStartErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		command   string
		wantError string
	}{
		{
			name:      "nonexistent command",
			command:   "/nonexistent/command/that/should/not/exist",
			wantError: "failed to start process",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pm := signal.NewProcessManager(signal.ProcessConfig{
				Command:         tt.command,
				StartupTimeout:  1 * time.Second,
				ShutdownTimeout: 1 * time.Second,
			})

			ctx := context.Background()
			err := pm.Start(ctx)

			if err == nil {
				t.Fatal("Expected error but got none")
			}

			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("Expected error containing %q, got %q", tt.wantError, err.Error())
			}
		})
	}
}

// TestProcessManagerRestart tests the restart functionality.
func TestProcessManagerRestart(t *testing.T) {
	t.Parallel()

	// Skip if sleep command not available (Windows)
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	pm := signal.NewProcessManager(signal.ProcessConfig{
		Command:         "sleep",
		Args:            []string{"0.1"},
		StartupTimeout:  200 * time.Millisecond,
		ShutdownTimeout: 200 * time.Millisecond,
	})

	ctx := context.Background()

	// Start the process
	if err := pm.Start(ctx); err != nil {
		t.Fatalf("Initial Start() failed: %v", err)
	}

	firstPID := pm.GetPID()
	if firstPID == 0 {
		t.Fatal("Expected non-zero PID after start")
	}

	// Restart the process
	if err := pm.Restart(ctx); err != nil {
		t.Fatalf("Restart() failed: %v", err)
	}

	// Verify new process is running
	if !pm.IsRunning() {
		t.Error("Process should be running after Restart()")
	}

	secondPID := pm.GetPID()
	if secondPID == 0 {
		t.Fatal("Expected non-zero PID after restart")
	}

	// Clean up
	if err := pm.Stop(ctx); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

// TestProcessManagerWaitForReady tests the ready waiting functionality.
func TestProcessManagerWaitForReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startProc bool
		wantError string
	}{
		{
			name:      "process not started",
			startProc: false,
			wantError: "process is not running",
		},
		{
			name:      "process started",
			startProc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pm := signal.NewProcessManager(signal.ProcessConfig{
				Command: "echo",
			})

			ctx := context.Background()

			if tt.startProc {
				if err := pm.Start(ctx); err != nil {
					t.Fatalf("Start() failed: %v", err)
				}
				// Use shorter timeout for stop in test
				stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				defer func() {
					_ = pm.Stop(stopCtx)
				}()
			}

			err := pm.WaitForReady(ctx)

			if tt.wantError != "" {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("Expected error containing %q, got %q", tt.wantError, err.Error())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestProcessManagerContextCancellation tests context cancellation handling.
func TestProcessManagerContextCancellation(t *testing.T) {
	t.Parallel()

	// Skip if sleep command not available (Windows)
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	pm := signal.NewProcessManager(signal.ProcessConfig{
		Command:         "sleep",
		Args:            []string{"10"},
		StartupTimeout:  200 * time.Millisecond,
		ShutdownTimeout: 200 * time.Millisecond,
	})

	// Test Start with canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := pm.Start(ctx)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected context canceled error, got: %v", err)
	}

	// Test Stop with timeout
	pm = signal.NewProcessManager(signal.ProcessConfig{
		Command:         "sleep",
		Args:            []string{"10"},
		StartupTimeout:  200 * time.Millisecond,
		ShutdownTimeout: 200 * time.Millisecond,
	})

	if startErr := pm.Start(context.Background()); startErr != nil {
		t.Fatalf("Start() failed: %v", startErr)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stopCancel()

	// This should succeed quickly since sleep is treated as a simple command
	err = pm.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

// TestProcessManagerBuildArgs tests command argument construction.
func TestProcessManagerBuildArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   signal.ProcessConfig
		wantArgs []string
	}{
		{
			name:     "default args",
			config:   signal.ProcessConfig{},
			wantArgs: []string{"daemon", "--json-rpc"},
		},
		{
			name: "with data path",
			config: signal.ProcessConfig{
				DataPath: "/path/to/data",
			},
			wantArgs: []string{"--config", "/path/to/data", "daemon", "--json-rpc"},
		},
		{
			name: "with phone number",
			config: signal.ProcessConfig{
				PhoneNumber: "+1234567890",
			},
			wantArgs: []string{"-a", "+1234567890", "daemon", "--json-rpc"},
		},
		{
			name: "with all options",
			config: signal.ProcessConfig{
				DataPath:    "/path/to/data",
				PhoneNumber: "+1234567890",
				Args:        []string{"--verbose"},
			},
			wantArgs: []string{
				"--config", "/path/to/data",
				"-a", "+1234567890",
				"--verbose",
				"daemon", "--json-rpc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pm := signal.NewProcessManager(tt.config)
			got := pm.BuildArgs()

			if len(got) != len(tt.wantArgs) {
				t.Errorf("BuildArgs() returned %d args, want %d", len(got), len(tt.wantArgs))
			}

			for i, arg := range got {
				if i >= len(tt.wantArgs) {
					break
				}
				if arg != tt.wantArgs[i] {
					t.Errorf("BuildArgs()[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

// TestProcessManagerDefaults tests default configuration.
func TestProcessManagerDefaults(t *testing.T) {
	t.Parallel()

	// Test with empty config
	pm := signal.NewProcessManager(signal.ProcessConfig{})

	config := pm.GetConfig()
	if config.Command != "signal-cli" {
		t.Errorf("Default command = %q, want %q", config.Command, "signal-cli")
	}

	if config.StartupTimeout != 30*time.Second {
		t.Errorf("Default startup timeout = %v, want %v", config.StartupTimeout, 30*time.Second)
	}

	if config.ShutdownTimeout != 10*time.Second {
		t.Errorf("Default shutdown timeout = %v, want %v", config.ShutdownTimeout, 10*time.Second)
	}

	// Test with partial config
	pm = signal.NewProcessManager(signal.ProcessConfig{
		Command: "custom-cli",
	})

	config = pm.GetConfig()
	if config.Command != "custom-cli" {
		t.Errorf("Custom command = %q, want %q", config.Command, "custom-cli")
	}

	if config.StartupTimeout != 30*time.Second {
		t.Errorf("Default startup timeout = %v, want %v", config.StartupTimeout, 30*time.Second)
	}
}

// TestProcessManagerConcurrency tests concurrent operations.
func TestProcessManagerConcurrency(t *testing.T) {
	t.Parallel()

	// Skip if echo command might not handle concurrent execution well
	if runtime.GOOS == "windows" {
		t.Skip("Skipping concurrency test on Windows")
	}

	pm := signal.NewProcessManager(signal.ProcessConfig{
		Command:         "echo",
		Args:            []string{"test"},
		StartupTimeout:  100 * time.Millisecond,
		ShutdownTimeout: 100 * time.Millisecond,
	})

	ctx := context.Background()

	// Start the process
	if err := pm.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Run multiple concurrent operations
	done := make(chan bool, 3)

	// Reader goroutine
	go func() {
		for range 100 {
			_ = pm.IsRunning()
			_ = pm.GetPID()
		}
		done <- true
	}()

	// State checker goroutine
	go func() {
		for range 100 {
			_ = pm.WaitForReady(ctx)
		}
		done <- true
	}()

	// Wait for goroutines
	for range 2 {
		<-done
	}

	// Stop the process
	if err := pm.Stop(ctx); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

// Benchmark tests.
func BenchmarkProcessManagerStart(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		pm := signal.NewProcessManager(signal.ProcessConfig{
			Command:         "echo",
			Args:            []string{"test"},
			StartupTimeout:  100 * time.Millisecond,
			ShutdownTimeout: 100 * time.Millisecond,
		})

		if err := pm.Start(ctx); err != nil {
			b.Fatal(err)
		}
		if err := pm.Stop(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildArgs(b *testing.B) {
	pm := signal.NewProcessManager(signal.ProcessConfig{
		DataPath:    "/path/to/data",
		PhoneNumber: "+1234567890",
		Args:        []string{"--verbose", "--debug"},
	})

	b.ResetTimer()
	for range b.N {
		_ = pm.BuildArgs()
	}
}

// Integration test helper.
func createTestSignalCLI(t *testing.T) string {
	t.Helper()

	// For integration test, we'll use a simple echo command that outputs the ready message
	// This avoids shell script buffering issues
	return "echo"
}

// TestProcessManagerIntegration tests with a mock signal-cli.
func TestProcessManagerIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping integration test on Windows")
	}

	// Create mock signal-cli
	mockCLI := createTestSignalCLI(t)

	pm := signal.NewProcessManager(signal.ProcessConfig{
		Command:         mockCLI,
		Args:            []string{"Started JSON-RPC server"}, // Echo this message
		DataPath:        t.TempDir(),
		PhoneNumber:     "+1234567890",
		StartupTimeout:  2 * time.Second,
		ShutdownTimeout: 1 * time.Second,
	})

	ctx := context.Background()

	// Start the process
	if startErr := pm.Start(ctx); startErr != nil {
		t.Fatalf("Start() failed: %v", startErr)
	}

	// Verify it's running
	if !pm.IsRunning() {
		t.Error("Process should be running")
	}

	// Wait for ready should succeed
	if readyErr := pm.WaitForReady(ctx); readyErr != nil {
		t.Errorf("WaitForReady() failed: %v", readyErr)
	}

	// Stop the process
	if stopErr := pm.Stop(ctx); stopErr != nil {
		t.Errorf("Stop() failed: %v", stopErr)
	}
}

// TestIsNormalExit tests normal exit detection.
func TestIsNormalExit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantNormal bool
	}{
		{
			name:       "nil error",
			err:        nil,
			wantNormal: true,
		},
		{
			name:       "exit code 0",
			err:        &exec.ExitError{ProcessState: &os.ProcessState{}},
			wantNormal: true,
		},
		{
			name:       "other error",
			err:        fmt.Errorf("some error"),
			wantNormal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := signal.IsNormalExit(tt.err)
			if got != tt.wantNormal {
				t.Errorf("IsNormalExit() = %v, want %v", got, tt.wantNormal)
			}
		})
	}
}
