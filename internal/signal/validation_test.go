package signal_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Veraticus/mentat/internal/signal"
)

func TestValidatePhoneNumber(t *testing.T) {
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
			name:        "valid German number",
			phoneNumber: "+491701234567",
			wantErr:     false,
		},
		{
			name:        "valid French number",
			phoneNumber: "+33123456789",
			wantErr:     false,
		},
		{
			name:        "valid minimum length",
			phoneNumber: "+1234567",
			wantErr:     false,
		},
		{
			name:        "valid maximum length",
			phoneNumber: "+123456789012345",
			wantErr:     false,
		},
		{
			name:        "valid single digit country code",
			phoneNumber: "+1234567890",
			wantErr:     false,
		},
		{
			name:        "valid three digit country code",
			phoneNumber: "+3581234567",
			wantErr:     false,
		},

		// Invalid cases
		{
			name:        "empty string",
			phoneNumber: "",
			wantErr:     true,
			errorMsg:    "phone number cannot be empty",
		},
		{
			name:        "missing plus sign",
			phoneNumber: "12125551234",
			wantErr:     true,
			errorMsg:    "phone number must start with '+' (E.164 format required)",
		},
		{
			name:        "just plus sign",
			phoneNumber: "+",
			wantErr:     true,
			errorMsg:    "phone number must include country code and number after '+'",
		},
		{
			name:        "contains letters",
			phoneNumber: "+1212ABC1234",
			wantErr:     true,
			errorMsg:    "phone number contains invalid character 'A' at position 6",
		},
		{
			name:        "contains special characters",
			phoneNumber: "+1(212)555-1234",
			wantErr:     true,
			errorMsg:    "phone number contains invalid character '(' at position 3",
		},
		{
			name:        "contains spaces",
			phoneNumber: "+1 212 555 1234",
			wantErr:     true,
			errorMsg:    "phone number contains invalid character ' ' at position 3",
		},
		{
			name:        "too short",
			phoneNumber: "+123456",
			wantErr:     true,
			errorMsg:    "phone number too short: 6 digits (minimum 7 required)",
		},
		{
			name:        "too long",
			phoneNumber: "+1234567890123456",
			wantErr:     true,
			errorMsg:    "phone number too long: 16 digits (maximum 15 allowed)",
		},
		{
			name:        "country code starts with zero",
			phoneNumber: "+0123456789",
			wantErr:     true,
			errorMsg:    "country code cannot start with 0",
		},
		{
			name:        "dash in number",
			phoneNumber: "+1-2125551234",
			wantErr:     true,
			errorMsg:    "phone number contains invalid character '-' at position 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := signal.ValidatePhoneNumber(tt.phoneNumber)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePhoneNumber() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errorMsg != "" && err.Error() != tt.errorMsg {
				t.Errorf("ValidatePhoneNumber() error message = %v, want %v", err.Error(), tt.errorMsg)
			}
		})
	}
}

func TestDetectCountryCode(t *testing.T) {
	tests := []struct {
		name          string
		phoneNumber   string
		wantCode      string
		wantCountry   string
		wantErr       bool
		errorContains string
	}{
		// Known country codes
		{
			name:        "US number",
			phoneNumber: "+12125551234",
			wantCode:    "1",
			wantCountry: "US/Canada",
			wantErr:     false,
		},
		{
			name:        "UK number",
			phoneNumber: "+442071234567",
			wantCode:    "44",
			wantCountry: "UK",
			wantErr:     false,
		},
		{
			name:        "Germany number",
			phoneNumber: "+491701234567",
			wantCode:    "49",
			wantCountry: "Germany",
			wantErr:     false,
		},
		{
			name:        "France number",
			phoneNumber: "+33123456789",
			wantCode:    "33",
			wantCountry: "France",
			wantErr:     false,
		},
		{
			name:        "Japan number",
			phoneNumber: "+819012345678",
			wantCode:    "81",
			wantCountry: "Japan",
			wantErr:     false,
		},
		{
			name:        "China number",
			phoneNumber: "+8613812345678",
			wantCode:    "86",
			wantCountry: "China",
			wantErr:     false,
		},
		{
			name:        "Russia number",
			phoneNumber: "+79123456789",
			wantCode:    "7",
			wantCountry: "Russia",
			wantErr:     false,
		},

		// Unknown country codes
		{
			name:        "unknown 3-digit code",
			phoneNumber: "+9991234567",
			wantCode:    "999",
			wantCountry: "Unknown",
			wantErr:     false,
		},
		{
			name:        "unknown 2-digit code",
			phoneNumber: "+991234567",
			wantCode:    "99",
			wantCountry: "Unknown",
			wantErr:     false,
		},
		{
			name:        "unknown 1-digit code",
			phoneNumber: "+21234567",
			wantCode:    "2",
			wantCountry: "Unknown",
			wantErr:     false,
		},

		// Invalid number length for known countries
		{
			name:          "US number too short",
			phoneNumber:   "+1212555",
			wantCode:      "1",
			wantCountry:   "US/Canada",
			wantErr:       true,
			errorContains: "invalid number length for US/Canada: expected 10-10 digits after country code, got 6",
		},
		{
			name:          "UK number too long",
			phoneNumber:   "+44207123456789",
			wantCode:      "44",
			wantCountry:   "UK",
			wantErr:       true,
			errorContains: "invalid number length for UK: expected 10-11 digits after country code, got 12",
		},

		// Invalid phone numbers
		{
			name:        "invalid phone number",
			phoneNumber: "invalid",
			wantCode:    "",
			wantCountry: "",
			wantErr:     true,
		},
		{
			name:        "empty phone number",
			phoneNumber: "",
			wantCode:    "",
			wantCountry: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, country, err := signal.DetectCountryCode(tt.phoneNumber)

			if (err != nil) != tt.wantErr {
				t.Errorf("DetectCountryCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if code != tt.wantCode {
				t.Errorf("DetectCountryCode() code = %v, want %v", code, tt.wantCode)
			}

			if country != tt.wantCountry {
				t.Errorf("DetectCountryCode() country = %v, want %v", country, tt.wantCountry)
			}

			if err != nil && tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("DetectCountryCode() error = %v, should contain %v", err.Error(), tt.errorContains)
			}
		})
	}
}

func TestFormatPhoneNumberForDisplay(t *testing.T) {
	tests := []struct {
		name        string
		phoneNumber string
		want        string
	}{
		// Known country formatting
		{
			name:        "US number",
			phoneNumber: "+12125551234",
			want:        "+1 (212) 555-1234",
		},
		{
			name:        "UK number 10 digits",
			phoneNumber: "+442071234567",
			want:        "+44 2071 234567",
		},
		{
			name:        "UK number 11 digits",
			phoneNumber: "+4420712345678",
			want:        "+44 2071 2345678",
		},
		{
			name:        "Germany number",
			phoneNumber: "+491701234567",
			want:        "+49 170 1234567",
		},
		{
			name:        "France number",
			phoneNumber: "+33123456789",
			want:        "+33 123456789",
		},

		// Unknown country codes - default formatting
		{
			name:        "unknown country code",
			phoneNumber: "+9991234567",
			want:        "+999 1234567",
		},

		// Invalid numbers return as-is
		{
			name:        "invalid number",
			phoneNumber: "invalid",
			want:        "invalid",
		},
		{
			name:        "empty number",
			phoneNumber: "",
			want:        "",
		},
		{
			name:        "no plus sign",
			phoneNumber: "12125551234",
			want:        "12125551234",
		},

		// Numbers with wrong length for country - default formatting
		{
			name:        "US number wrong length",
			phoneNumber: "+121255512",
			want:        "+1 21255512",
		},
		{
			name:        "UK number too short",
			phoneNumber: "+44207123",
			want:        "+44 207123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signal.FormatPhoneNumberForDisplay(tt.phoneNumber)
			if got != tt.want {
				t.Errorf("FormatPhoneNumberForDisplay() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestE164EdgeCases tests edge cases for E.164 validation.
func TestE164EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		phoneNumber string
		valid       bool
		description string
	}{
		{
			name:        "minimum valid E.164",
			phoneNumber: "+1234567",
			valid:       true,
			description: "7 digits total is minimum",
		},
		{
			name:        "maximum valid E.164",
			phoneNumber: "+123456789012345",
			valid:       true,
			description: "15 digits total is maximum",
		},
		{
			name:        "country code can be single digit",
			phoneNumber: "+7234567",
			valid:       true,
			description: "Russia has single digit country code",
		},
		{
			name:        "country code can be three digits",
			phoneNumber: "+3581234567",
			valid:       true,
			description: "Finland has 3-digit country code",
		},
		{
			name:        "plus zero is invalid",
			phoneNumber: "+0",
			valid:       false,
			description: "country code cannot start with 0",
		},
		{
			name:        "unicode digits not allowed",
			phoneNumber: "+1२३४५६७८९०",
			valid:       false,
			description: "only ASCII digits allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := signal.ValidatePhoneNumber(tt.phoneNumber)
			isValid := err == nil
			if isValid != tt.valid {
				t.Errorf("ValidatePhoneNumber(%s) valid = %v, want %v (%s)",
					tt.phoneNumber, isValid, tt.valid, tt.description)
				if err != nil {
					t.Logf("Error: %v", err)
				}
			}
		})
	}
}

// BenchmarkValidatePhoneNumber benchmarks phone number validation.
func BenchmarkValidatePhoneNumber(b *testing.B) {
	phoneNumbers := []string{
		"+12125551234",
		"+442071234567",
		"+491701234567",
		"invalid",
		"+1234567890123456", // too long
	}

	b.ResetTimer()
	for range b.N {
		for _, num := range phoneNumbers {
			_ = signal.ValidatePhoneNumber(num)
		}
	}
}

// ExampleValidatePhoneNumber shows how to use ValidatePhoneNumber.
func ExampleValidatePhoneNumber() {
	// Valid US number
	err := signal.ValidatePhoneNumber("+12125551234")
	fmt.Println("Valid US number:", err == nil)

	// Invalid - missing plus
	err = signal.ValidatePhoneNumber("12125551234")
	fmt.Println("Missing plus error:", err != nil)

	// Invalid - too short
	err = signal.ValidatePhoneNumber("+12345")
	fmt.Println("Too short error:", err != nil)

	// Output:
	// Valid US number: true
	// Missing plus error: true
	// Too short error: true
}

// ExampleDetectCountryCode shows how to use DetectCountryCode.
func ExampleDetectCountryCode() {
	// US number
	code, country, _ := signal.DetectCountryCode("+12125551234")
	fmt.Printf("US: code=%s, country=%s\n", code, country)

	// UK number
	code, country, _ = signal.DetectCountryCode("+442071234567")
	fmt.Printf("UK: code=%s, country=%s\n", code, country)

	// Unknown country
	code, country, _ = signal.DetectCountryCode("+9991234567")
	fmt.Printf("Unknown: code=%s, country=%s\n", code, country)

	// Output:
	// US: code=1, country=US/Canada
	// UK: code=44, country=UK
	// Unknown: code=999, country=Unknown
}

// ExampleFormatPhoneNumberForDisplay shows how to format phone numbers.
func ExampleFormatPhoneNumberForDisplay() {
	// US number formatting
	formatted := signal.FormatPhoneNumberForDisplay("+12125551234")
	fmt.Println("US:", formatted)

	// UK number formatting
	formatted = signal.FormatPhoneNumberForDisplay("+442071234567")
	fmt.Println("UK:", formatted)

	// German number formatting
	formatted = signal.FormatPhoneNumberForDisplay("+491701234567")
	fmt.Println("Germany:", formatted)

	// Output:
	// US: +1 (212) 555-1234
	// UK: +44 2071 234567
	// Germany: +49 170 1234567
}
