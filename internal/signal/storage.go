// Package signal provides Signal messenger integration via signal-cli.
package signal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// dirPermissions is the permission mode for Signal data directories.
	// 0700 ensures only the owner can read, write, and execute.
	dirPermissions = 0700

	// minPhoneNumberLength is the minimum length for a valid E.164 phone number.
	// This includes the '+' prefix and at least 7 digits.
	minPhoneNumberLength = 8
)

// StorageManager handles Signal data directory management.
type StorageManager interface {
	// InitializeDataPath creates the complete directory structure for a phone number.
	InitializeDataPath(phoneNumber string) error

	// ValidateDataPath validates existing directories and permissions.
	ValidateDataPath(phoneNumber string) error

	// GetDataPath returns the base data path for a phone number.
	GetDataPath(phoneNumber string) string

	// GetSubPath returns a specific subdirectory path.
	GetSubPath(phoneNumber string, subdir string) string

	// HasExistingConfig checks if Signal configuration already exists.
	HasExistingConfig(phoneNumber string) bool
}

// StorageManagerImpl implements the StorageManager interface.
type StorageManagerImpl struct {
	basePath string
}

// NewStorageManager creates a new storage manager with the given base path.
func NewStorageManager(basePath string) *StorageManagerImpl {
	if basePath == "" {
		basePath = "/var/lib/mentat/signal/data"
	}
	return &StorageManagerImpl{
		basePath: basePath,
	}
}

// InitializeDataPath creates the complete directory structure for a phone number.
func (sm *StorageManagerImpl) InitializeDataPath(phoneNumber string) error {
	// Validate phone number format
	if err := sm.validatePhoneNumber(phoneNumber); err != nil {
		return fmt.Errorf("invalid phone number: %w", err)
	}

	dataPath := sm.GetDataPath(phoneNumber)

	// Create base directory
	if err := sm.createDirectory(dataPath); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}

	// Create subdirectories
	subdirs := []string{"avatars", "attachments", "stickers"}
	for _, subdir := range subdirs {
		path := filepath.Join(dataPath, subdir)
		if err := sm.createDirectory(path); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Validate permissions after creation
	if err := sm.ValidateDataPath(phoneNumber); err != nil {
		return fmt.Errorf("directory validation failed after creation: %w", err)
	}

	return nil
}

// ValidateDataPath validates existing directories and permissions.
func (sm *StorageManagerImpl) ValidateDataPath(phoneNumber string) error {
	// Validate phone number format
	if err := sm.validatePhoneNumber(phoneNumber); err != nil {
		return fmt.Errorf("invalid phone number: %w", err)
	}

	dataPath := sm.GetDataPath(phoneNumber)

	// Check base directory
	if err := sm.validateDirectory(dataPath); err != nil {
		return fmt.Errorf("base directory validation failed: %w", err)
	}

	// Check subdirectories
	subdirs := []string{"avatars", "attachments", "stickers"}
	for _, subdir := range subdirs {
		path := filepath.Join(dataPath, subdir)
		if err := sm.validateDirectory(path); err != nil {
			return fmt.Errorf("%s directory validation failed: %w", subdir, err)
		}
	}

	// Test write permissions
	testFile := filepath.Join(dataPath, ".permission_test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return fmt.Errorf("data directory not writable: %w", err)
	}
	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("failed to remove test file: %w", err)
	}

	return nil
}

// GetDataPath returns the base data path for a phone number.
func (sm *StorageManagerImpl) GetDataPath(phoneNumber string) string {
	// Normalize phone number for directory name
	cleanNumber := strings.ReplaceAll(phoneNumber, " ", "")
	cleanNumber = strings.ReplaceAll(cleanNumber, "-", "")
	return filepath.Join(sm.basePath, cleanNumber)
}

// GetSubPath returns a specific subdirectory path.
func (sm *StorageManagerImpl) GetSubPath(phoneNumber string, subdir string) string {
	return filepath.Join(sm.GetDataPath(phoneNumber), subdir)
}

// HasExistingConfig checks if Signal configuration already exists.
func (sm *StorageManagerImpl) HasExistingConfig(phoneNumber string) bool {
	dataPath := sm.GetDataPath(phoneNumber)
	accountFile := filepath.Join(dataPath, "account.db")

	// Check if account.db exists
	info, err := os.Stat(accountFile)
	if err != nil {
		return false
	}

	// Make sure it's a file and has non-zero size
	return !info.IsDir() && info.Size() > 0
}

// createDirectory creates a directory with proper permissions.
func (sm *StorageManagerImpl) createDirectory(path string) error {
	// Create directory with 0700 permissions (owner read/write/execute only)
	if err := os.MkdirAll(path, dirPermissions); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied creating directory %s: %w", path, err)
		}
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	// Ensure permissions are correct even if directory already existed
	// #nosec G302 -- directories need execute permission (0700) for traversal
	if err := os.Chmod(path, dirPermissions); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied setting permissions on %s: %w", path, err)
		}
		return fmt.Errorf("failed to set permissions on %s: %w", path, err)
	}

	return nil
}

// validateDirectory checks if a directory exists and has proper permissions.
func (sm *StorageManagerImpl) validateDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied accessing directory %s: %w", path, err)
		}
		return fmt.Errorf("failed to stat directory %s: %w", path, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// Check permissions (should be 0700)
	mode := info.Mode().Perm()
	if mode != dirPermissions {
		return fmt.Errorf("incorrect permissions on %s: got %o, want 0700", path, mode)
	}

	return nil
}

// validatePhoneNumber performs basic validation on phone number format.
func (sm *StorageManagerImpl) validatePhoneNumber(phoneNumber string) error {
	if phoneNumber == "" {
		return fmt.Errorf("phone number cannot be empty")
	}

	// Remove common formatting characters
	cleaned := strings.ReplaceAll(phoneNumber, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	// Check if it starts with + (E.164 format)
	if !strings.HasPrefix(cleaned, "+") {
		return fmt.Errorf("phone number must be in E.164 format (starting with +)")
	}

	// Check minimum length (+ followed by at least 7 digits)
	if len(cleaned) < minPhoneNumberLength {
		return fmt.Errorf("phone number too short")
	}

	// Check if remaining characters are digits
	for i := 1; i < len(cleaned); i++ {
		if cleaned[i] < '0' || cleaned[i] > '9' {
			return fmt.Errorf("phone number contains non-digit characters")
		}
	}

	return nil
}
