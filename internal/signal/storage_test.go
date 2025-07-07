package signal_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Veraticus/mentat/internal/signal"
)

func TestNewStorageManager(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		wantBasePath string
	}{
		{
			name:         "with custom base path",
			basePath:     "/custom/path",
			wantBasePath: "/custom/path",
		},
		{
			name:         "with empty base path uses default",
			basePath:     "",
			wantBasePath: "/var/lib/mentat/signal/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := signal.NewStorageManager(tt.basePath)
			gotPath := sm.GetDataPath("+12125551234")
			if !strings.HasPrefix(gotPath, tt.wantBasePath) {
				t.Errorf("expected path to start with %s, got %s", tt.wantBasePath, gotPath)
			}
		})
	}
}

func TestGetDataPath(t *testing.T) {
	sm := signal.NewStorageManager("/test/base")

	tests := []struct {
		name        string
		phoneNumber string
		want        string
	}{
		{
			name:        "formats US number",
			phoneNumber: "+1-212-555-1234",
			want:        "/test/base/+12125551234",
		},
		{
			name:        "handles spaces",
			phoneNumber: "+1 212 555 1234",
			want:        "/test/base/+12125551234",
		},
		{
			name:        "already clean number",
			phoneNumber: "+12125551234",
			want:        "/test/base/+12125551234",
		},
		{
			name:        "UK number",
			phoneNumber: "+44-20-7123-4567",
			want:        "/test/base/+442071234567",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sm.GetDataPath(tt.phoneNumber)
			if got != tt.want {
				t.Errorf("GetDataPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSubPath(t *testing.T) {
	sm := signal.NewStorageManager("/test/base")

	tests := []struct {
		name        string
		phoneNumber string
		subdir      string
		want        string
	}{
		{
			name:        "avatars subdir",
			phoneNumber: "+12125551234",
			subdir:      "avatars",
			want:        "/test/base/+12125551234/avatars",
		},
		{
			name:        "attachments subdir",
			phoneNumber: "+12125551234",
			subdir:      "attachments",
			want:        "/test/base/+12125551234/attachments",
		},
		{
			name:        "nested subdir",
			phoneNumber: "+12125551234",
			subdir:      "foo/bar",
			want:        "/test/base/+12125551234/foo/bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sm.GetSubPath(tt.phoneNumber, tt.subdir)
			if got != tt.want {
				t.Errorf("GetSubPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitializeDataPath(t *testing.T) {
	// Use temp directory for testing
	tempDir := t.TempDir()
	sm := signal.NewStorageManager(tempDir)

	tests := []struct {
		name        string
		phoneNumber string
		wantErr     bool
		errorMsg    string
	}{
		{
			name:        "valid phone number",
			phoneNumber: "+12125551234",
			wantErr:     false,
		},
		{
			name:        "invalid phone number",
			phoneNumber: "not-a-phone",
			wantErr:     true,
			errorMsg:    "must be in E.164 format",
		},
		{
			name:        "empty phone number",
			phoneNumber: "",
			wantErr:     true,
			errorMsg:    "phone number cannot be empty",
		},
		{
			name:        "phone number with formatting",
			phoneNumber: "+1-212-555-1234",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.InitializeDataPath(tt.phoneNumber)
			if (err != nil) != tt.wantErr {
				t.Errorf("InitializeDataPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("error message = %v, want to contain %v", err.Error(), tt.errorMsg)
				}
			}

			// If successful, verify directory structure
			if err == nil {
				dataPath := sm.GetDataPath(tt.phoneNumber)

				// Check base directory
				if !dirExists(dataPath) {
					t.Errorf("base directory not created: %s", dataPath)
				}

				// Check subdirectories
				subdirs := []string{"avatars", "attachments", "stickers"}
				for _, subdir := range subdirs {
					path := filepath.Join(dataPath, subdir)
					if !dirExists(path) {
						t.Errorf("subdirectory not created: %s", path)
					}

					// Check permissions
					info, statErr := os.Stat(path)
					if statErr != nil {
						t.Errorf("failed to stat directory: %v", statErr)
						continue
					}
					mode := info.Mode().Perm()
					if mode != 0700 {
						t.Errorf("incorrect permissions on %s: got %o, want 0700", path, mode)
					}
				}
			}
		})
	}
}

func TestValidateDataPath(t *testing.T) {
	tempDir := t.TempDir()
	sm := signal.NewStorageManager(tempDir)

	// Create a valid directory structure
	phoneNumber := "+12125551234"
	err := sm.InitializeDataPath(phoneNumber)
	if err != nil {
		t.Fatalf("failed to initialize test directory: %v", err)
	}

	tests := []struct {
		name        string
		phoneNumber string
		setup       func()
		wantErr     bool
		errorMsg    string
	}{
		{
			name:        "valid directory structure",
			phoneNumber: phoneNumber,
			wantErr:     false,
		},
		{
			name:        "invalid phone number",
			phoneNumber: "invalid",
			wantErr:     true,
			errorMsg:    "invalid phone number",
		},
		{
			name:        "non-existent directory",
			phoneNumber: "+19999999999",
			wantErr:     true,
			errorMsg:    "directory does not exist",
		},
		{
			name:        "missing subdirectory",
			phoneNumber: "+13333333333",
			setup: func() {
				// Create base directory but not subdirectories
				dataPath := sm.GetDataPath("+13333333333")
				_ = os.MkdirAll(dataPath, 0700)
			},
			wantErr:  true,
			errorMsg: "directory validation failed",
		},
		{
			name:        "wrong permissions",
			phoneNumber: "+14444444444",
			setup: func() {
				dataPath := sm.GetDataPath("+14444444444")
				_ = os.MkdirAll(dataPath, 0755)
			},
			wantErr:  true,
			errorMsg: "incorrect permissions",
		},
		{
			name:        "file instead of directory",
			phoneNumber: "+15555555555",
			setup: func() {
				dataPath := sm.GetDataPath("+15555555555")
				dir := filepath.Dir(dataPath)
				_ = os.MkdirAll(dir, 0700)
				_ = os.WriteFile(dataPath, []byte("not a directory"), 0600)
			},
			wantErr:  true,
			errorMsg: "not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			validateErr := sm.ValidateDataPath(tt.phoneNumber)
			if (validateErr != nil) != tt.wantErr {
				t.Errorf("ValidateDataPath() error = %v, wantErr %v", validateErr, tt.wantErr)
				return
			}
			if validateErr != nil && tt.errorMsg != "" {
				if !strings.Contains(validateErr.Error(), tt.errorMsg) {
					t.Errorf("error message = %v, want to contain %v", validateErr.Error(), tt.errorMsg)
				}
			}
		})
	}
}

func TestHasExistingConfig(t *testing.T) {
	tempDir := t.TempDir()
	sm := signal.NewStorageManager(tempDir)

	tests := []struct {
		name        string
		phoneNumber string
		setup       func()
		want        bool
	}{
		{
			name:        "no config exists",
			phoneNumber: "+12125551234",
			want:        false,
		},
		{
			name:        "config exists with content",
			phoneNumber: "+13333333333",
			setup: func() {
				dataPath := sm.GetDataPath("+13333333333")
				_ = os.MkdirAll(dataPath, 0700)
				accountFile := filepath.Join(dataPath, "account.db")
				_ = os.WriteFile(accountFile, []byte("test data"), 0600)
			},
			want: true,
		},
		{
			name:        "empty config file",
			phoneNumber: "+14444444444",
			setup: func() {
				dataPath := sm.GetDataPath("+14444444444")
				_ = os.MkdirAll(dataPath, 0700)
				accountFile := filepath.Join(dataPath, "account.db")
				_ = os.WriteFile(accountFile, []byte{}, 0600)
			},
			want: false,
		},
		{
			name:        "account.db is directory",
			phoneNumber: "+15555555555",
			setup: func() {
				dataPath := sm.GetDataPath("+15555555555")
				_ = os.MkdirAll(dataPath, 0700)
				accountFile := filepath.Join(dataPath, "account.db")
				_ = os.MkdirAll(accountFile, 0700)
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			got := sm.HasExistingConfig(tt.phoneNumber)
			if got != tt.want {
				t.Errorf("HasExistingConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPhoneNumberValidation(t *testing.T) {
	tempDir := t.TempDir()
	sm := signal.NewStorageManager(tempDir)

	tests := []struct {
		name        string
		phoneNumber string
		wantErr     bool
		errorMsg    string
	}{
		// Valid cases
		{
			name:        "valid US number",
			phoneNumber: "+12125551234",
			wantErr:     false,
		},
		{
			name:        "valid UK number",
			phoneNumber: "+442071234567",
			wantErr:     false,
		},
		{
			name:        "valid with spaces",
			phoneNumber: "+1 212 555 1234",
			wantErr:     false,
		},
		{
			name:        "valid with dashes",
			phoneNumber: "+1-212-555-1234",
			wantErr:     false,
		},
		{
			name:        "valid with parentheses",
			phoneNumber: "+1 (212) 555-1234",
			wantErr:     false,
		},
		{
			name:        "minimum valid length",
			phoneNumber: "+1234567",
			wantErr:     false,
		},
		// Invalid cases
		{
			name:        "empty phone number",
			phoneNumber: "",
			wantErr:     true,
			errorMsg:    "phone number cannot be empty",
		},
		{
			name:        "missing + prefix",
			phoneNumber: "12125551234",
			wantErr:     true,
			errorMsg:    "must be in E.164 format",
		},
		{
			name:        "too short",
			phoneNumber: "+123456",
			wantErr:     true,
			errorMsg:    "phone number too short",
		},
		{
			name:        "contains letters",
			phoneNumber: "+1212555ABCD",
			wantErr:     true,
			errorMsg:    "contains non-digit characters",
		},
		{
			name:        "special characters",
			phoneNumber: "+1212555!@#$",
			wantErr:     true,
			errorMsg:    "contains non-digit characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.InitializeDataPath(tt.phoneNumber)
			if (err != nil) != tt.wantErr {
				t.Errorf("phone validation error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("error message = %v, want to contain %v", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

func TestPermissionErrors(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission tests when running as root")
	}

	tempDir := t.TempDir()
	// Create a directory with no write permissions
	restrictedDir := filepath.Join(tempDir, "restricted")
	err := os.MkdirAll(restrictedDir, 0500)
	if err != nil {
		t.Fatalf("failed to create restricted directory: %v", err)
	}

	sm := signal.NewStorageManager(restrictedDir)
	err = sm.InitializeDataPath("+12125551234")
	if err == nil {
		t.Error("expected permission error, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected permission denied error, got: %v", err)
	}
}

// Helper functions

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
