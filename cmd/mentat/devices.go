// Package main provides device management CLI commands for Mentat.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	signalpkg "github.com/Veraticus/mentat/internal/signal"
)

const (
	// tabwriterPadding is the padding for tabwriter.
	tabwriterPadding = 2
	// deviceLinkTimeout is the timeout for device linking.
	deviceLinkTimeout = 2 * time.Minute
	// signalManagerTimeout is the timeout for Signal manager operations.
	signalManagerTimeout = 30 * time.Second
	// minArgsForCommand is the minimum number of arguments for a command.
	minArgsForCommand = 2
	// minArgsForSubcommand is the minimum number of arguments for subcommands that take a parameter.
	minArgsForSubcommand = 3
)

// DeviceCommands handles device management operations.
type DeviceCommands struct {
	manager       signalpkg.Manager
	deviceManager signalpkg.DeviceManager
	phoneNumber   string
}

// NewDeviceCommands creates a new device command handler.
func NewDeviceCommands(manager signalpkg.Manager) *DeviceCommands {
	return &DeviceCommands{
		manager:       manager,
		deviceManager: manager.GetDeviceManager(),
		phoneNumber:   manager.GetPhoneNumber(),
	}
}

// ListDevices lists all linked devices.
func (dc *DeviceCommands) ListDevices(ctx context.Context) error {
	devices, err := dc.deviceManager.ListDevices(ctx)
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	if len(devices) == 0 {
		_, _ = io.WriteString(os.Stdout, "No devices found.\n")
		return nil
	}

	// Create a tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, tabwriterPadding, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tPRIMARY\tCREATED\tLAST SEEN")
	_, _ = fmt.Fprintln(w, "--\t----\t-------\t-------\t---------")

	for _, device := range devices {
		primary := ""
		if device.Primary {
			primary = "Yes"
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			device.ID,
			device.Name,
			primary,
			device.Created.Format("2006-01-02"),
			device.LastSeen.Format("2006-01-02 15:04"),
		)
	}

	if flushErr := w.Flush(); flushErr != nil {
		return fmt.Errorf("failed to flush output: %w", flushErr)
	}
	return nil
}

// RemoveDevice removes a linked device.
func (dc *DeviceCommands) RemoveDevice(ctx context.Context, deviceIDStr string) error {
	deviceID, err := strconv.Atoi(deviceIDStr)
	if err != nil {
		return fmt.Errorf("invalid device ID: %w", err)
	}

	// Check if device exists and is not primary
	devices, listErr := dc.deviceManager.ListDevices(ctx)
	if listErr != nil {
		return fmt.Errorf("failed to list devices: %w", listErr)
	}

	var found bool
	for _, device := range devices {
		if device.ID == deviceID {
			found = true
			if device.Primary {
				return fmt.Errorf("cannot remove primary device")
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("device with ID %d not found", deviceID)
	}

	// Remove the device
	if removeErr := dc.deviceManager.RemoveDevice(ctx, deviceID); removeErr != nil {
		return fmt.Errorf("failed to remove device: %w", removeErr)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Device %d removed successfully.\n", deviceID)
	return nil
}

// LinkDevice generates a linking URI and waits for device to be linked.
func (dc *DeviceCommands) LinkDevice(ctx context.Context, deviceName string) error {
	if deviceName == "" {
		return fmt.Errorf("device name cannot be empty")
	}

	// Generate linking URI
	uri, err := dc.deviceManager.GenerateLinkingURI(ctx, deviceName)
	if err != nil {
		return fmt.Errorf("failed to generate linking URI: %w", err)
	}

	_, _ = io.WriteString(os.Stdout, "Open Signal on your device and scan this QR code:\n\n")

	// Generate a simple ASCII QR code placeholder
	// In a real implementation, you'd use a QR code library
	qrPlaceholder := `╔════════════════════════════════════════╗
║                                        ║
║        [QR CODE PLACEHOLDER]           ║
║                                        ║
║  Install a QR code generator to see    ║
║  the actual QR code, or use this URI:  ║
║                                        ║
╚════════════════════════════════════════╝

Linking URI:
`
	_, _ = io.WriteString(os.Stdout, qrPlaceholder)
	_, _ = io.WriteString(os.Stdout, uri)
	_, _ = io.WriteString(os.Stdout, "\n\nWaiting for device to link (timeout: 2 minutes)...\n")

	// Wait for device to be linked
	linkCtx, cancel := context.WithTimeout(ctx, deviceLinkTimeout)
	defer cancel()

	device, err := dc.deviceManager.WaitForDeviceLink(linkCtx, deviceLinkTimeout)
	if err != nil {
		return fmt.Errorf("device linking failed: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "\nDevice '%s' (ID: %d) linked successfully!\n", device.Name, device.ID)
	return nil
}

// ExecuteDeviceCommand handles device management subcommands.
func ExecuteDeviceCommand(args []string) error {
	if len(args) < minArgsForCommand {
		printDeviceUsage()
		return fmt.Errorf("insufficient arguments")
	}

	subcommand := args[1]

	// Initialize Signal manager with injected dependencies for testing
	ctx := context.Background()

	// Read phone number
	phoneNumber, err := readPhoneNumber()
	if err != nil {
		return fmt.Errorf("failed to read phone number: %w", err)
	}

	// Create Signal manager
	cfg := signalpkg.Config{
		SocketPath:  signalSocketPath,
		PhoneNumber: phoneNumber,
		Timeout:     signalManagerTimeout,
	}

	// Create transport
	transport, err := signalpkg.NewUnixSocketTransport(cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}

	// Create device manager
	deviceManager := signalpkg.NewDeviceManager(transport, phoneNumber)

	// Create Signal manager with device manager
	manager := signalpkg.NewSignalManager(cfg,
		signalpkg.WithDeviceManager(deviceManager),
		signalpkg.WithTransport(transport),
	)

	// Start the manager
	if startErr := manager.Start(ctx); startErr != nil {
		return fmt.Errorf("failed to start Signal manager: %w", startErr)
	}
	defer func() {
		if stopErr := manager.Stop(ctx); stopErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error stopping Signal manager: %v\n", stopErr)
		}
	}()

	// Create device commands handler
	dc := NewDeviceCommands(manager)

	switch subcommand {
	case "list":
		return dc.ListDevices(ctx)

	case "remove":
		if len(args) < minArgsForSubcommand {
			return fmt.Errorf("device ID required for remove command")
		}
		return dc.RemoveDevice(ctx, args[2])

	case "link":
		if len(args) < minArgsForSubcommand {
			return fmt.Errorf("device name required for link command")
		}
		return dc.LinkDevice(ctx, args[2])

	default:
		printDeviceUsage()
		return fmt.Errorf("unknown subcommand: %s", subcommand)
	}
}

func printDeviceUsage() {
	usageText := `Usage: mentat devices <command> [args]

Commands:
  list              List all linked devices
  remove <id>       Remove a linked device
  link <name>       Link a new device with the given name

Examples:
  mentat devices list
  mentat devices remove 2
  mentat devices link "Mom's iPad"
`
	_, _ = io.WriteString(os.Stdout, usageText)
}
