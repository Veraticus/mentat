// Package signal provides Signal messenger integration via signal-cli.
package signal

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// countryUnknown is returned when country code is not recognized.
	countryUnknown = "Unknown"

	// e164MinLength is the minimum length of an E.164 number (excluding +).
	e164MinLength = 7

	// e164MaxLength is the maximum length of an E.164 number (excluding +).
	e164MaxLength = 15

	// positionOffset is added to character index for user-friendly position.
	positionOffset = 2

	// Common phone number lengths.
	phoneLength9  = 9
	phoneLength10 = 10
	phoneLength11 = 11
	phoneLength12 = 12

	// maxCountryCodeLength is the maximum length of a country code.
	maxCountryCodeLength = 3

	// minCountryCodeLength is the minimum length of a country code.
	minCountryCodeLength = 1

	// countryCodeLength2 is the length of a 2-digit country code.
	countryCodeLength2 = 2

	// phoneDigitsMin is the minimum phone digits after country code.
	phoneDigitsMin = 6
)

// countryFormat holds formatting information for a country.
type countryFormat struct {
	code      string
	minLength int
	maxLength int
	name      string
}

// getCountryCodeFormats returns country code formatting information.
// This function avoids global variable issues.
func getCountryCodeFormats() map[string]countryFormat {
	return map[string]countryFormat{
		"1":  {code: "1", minLength: phoneLength10, maxLength: phoneLength10, name: "US/Canada"},
		"44": {code: "44", minLength: phoneLength10, maxLength: phoneLength11, name: "UK"},
		"49": {code: "49", minLength: phoneLength10, maxLength: phoneLength12, name: "Germany"},
		"33": {code: "33", minLength: phoneLength9, maxLength: phoneLength9, name: "France"},
		"81": {code: "81", minLength: phoneLength10, maxLength: phoneLength11, name: "Japan"},
		"86": {code: "86", minLength: phoneLength11, maxLength: phoneLength11, name: "China"},
		"91": {code: "91", minLength: phoneLength10, maxLength: phoneLength10, name: "India"},
		"61": {code: "61", minLength: phoneLength9, maxLength: phoneLength9, name: "Australia"},
		"7":  {code: "7", minLength: phoneLength10, maxLength: phoneLength10, name: "Russia"},
		"55": {code: "55", minLength: phoneLength10, maxLength: phoneLength11, name: "Brazil"},
		"52": {code: "52", minLength: phoneLength10, maxLength: phoneLength10, name: "Mexico"},
		"34": {code: "34", minLength: phoneLength9, maxLength: phoneLength9, name: "Spain"},
		"39": {code: "39", minLength: phoneLength9, maxLength: phoneLength10, name: "Italy"},
		"31": {code: "31", minLength: phoneLength9, maxLength: phoneLength9, name: "Netherlands"},
		"82": {code: "82", minLength: phoneLength9, maxLength: phoneLength10, name: "South Korea"},
	}
}

// e164Regex validates the basic E.164 format.
var e164Regex = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// ValidatePhoneNumber validates that a phone number follows E.164 format.
// E.164 format: +[country code][phone number]
// - Must start with '+'
// - Country code: 1-3 digits (cannot start with 0)
// - Total length: 7-15 digits (excluding the +)
// - Returns nil if valid, error with clear message if invalid.
func ValidatePhoneNumber(phoneNumber string) error {
	// Check for empty string
	if phoneNumber == "" {
		return fmt.Errorf("phone number cannot be empty")
	}

	// Check for + prefix
	if !strings.HasPrefix(phoneNumber, "+") {
		return fmt.Errorf("phone number must start with '+' (E.164 format required)")
	}

	// Basic E.164 regex validation
	if !e164Regex.MatchString(phoneNumber) {
		return validatePhoneNumberDetails(phoneNumber)
	}

	return nil
}

// validatePhoneNumberDetails provides detailed validation errors.
func validatePhoneNumberDetails(phoneNumber string) error {
	// Provide specific error messages
	if len(phoneNumber) == 1 {
		return fmt.Errorf("phone number must include country code and number after '+'")
	}

	// Check if it contains non-digit characters
	for i, r := range phoneNumber[1:] {
		if r < '0' || r > '9' {
			return fmt.Errorf("phone number contains invalid character '%c' at position %d", r, i+positionOffset)
		}
	}

	// Check length
	digitCount := len(phoneNumber) - 1
	if digitCount < e164MinLength {
		return fmt.Errorf("phone number too short: %d digits (minimum %d required)", digitCount, e164MinLength)
	}
	if digitCount > e164MaxLength {
		return fmt.Errorf("phone number too long: %d digits (maximum %d allowed)", digitCount, e164MaxLength)
	}

	// Check for leading zero in country code
	if phoneNumber[1] == '0' {
		return fmt.Errorf("country code cannot start with 0")
	}

	return fmt.Errorf("invalid phone number format")
}

// DetectCountryCode attempts to identify the country code from an E.164 phone number.
// Returns the country code, country name, and nil error if detected.
// Returns empty strings and an error if the number is invalid or country code is unknown.
func DetectCountryCode(phoneNumber string) (string, string, error) {
	// First validate the phone number
	if err := ValidatePhoneNumber(phoneNumber); err != nil {
		return "", "", err
	}

	// Remove the + prefix
	number := phoneNumber[1:]

	// Check known country codes first
	code, name, err := checkKnownCountryCodes(number)
	if code != "" {
		return code, name, err
	}

	// For unknown codes, extract based on reasonable assumptions
	code = extractUnknownCountryCode(number)
	return code, countryUnknown, nil
}

// checkKnownCountryCodes checks if the number starts with a known country code.
func checkKnownCountryCodes(number string) (string, string, error) {
	countryCodeFormats := getCountryCodeFormats()

	// Try to match country codes, starting with longest possible (3 digits)
	for length := maxCountryCodeLength; length >= minCountryCodeLength; length-- {
		if len(number) >= length {
			possibleCode := number[:length]
			if format, exists := countryCodeFormats[possibleCode]; exists {
				// Validate the remaining number length for this country
				remainingDigits := len(number) - length
				if remainingDigits < format.minLength || remainingDigits > format.maxLength {
					return possibleCode, format.name, fmt.Errorf(
						"invalid number length for %s: expected %d-%d digits after country code, got %d",
						format.name,
						format.minLength,
						format.maxLength,
						remainingDigits,
					)
				}
				return possibleCode, format.name, nil
			}
		}
	}

	return "", "", nil
}

// extractUnknownCountryCode extracts country code for unknown numbers.
// For unknown codes, we make an intelligent guess based on total length.
func extractUnknownCountryCode(number string) string {
	if number[0] == '0' {
		return ""
	}

	numberLen := len(number)

	// Based on total length, choose most likely country code length
	// This heuristic tries to leave a reasonable phone number length (6-8 digits)
	switch {
	case numberLen >= 10 && numberLen <= 11:
		// For 10-11 digits total, try 3-digit country code first
		// e.g., +9991234567 -> 999 (leaves 7 digits)
		if numberLen >= maxCountryCodeLength+6 {
			return number[:maxCountryCodeLength]
		}
	case numberLen == phoneLength9:
		// For 9 digits total, try 2-digit country code
		// e.g., +991234567 -> 99 (leaves 7 digits)
		if numberLen >= countryCodeLength2+phoneDigitsMin {
			return number[:countryCodeLength2]
		}
	}

	// Default: prefer single digit country code
	if numberLen >= minCountryCodeLength+6 {
		return number[:minCountryCodeLength]
	}

	// For very short numbers, take what we can
	if numberLen >= maxCountryCodeLength {
		return number[:maxCountryCodeLength]
	}
	if numberLen >= countryCodeLength2 {
		return number[:countryCodeLength2]
	}

	// Last resort: single digit
	return number[:minCountryCodeLength]
}

// FormatPhoneNumberForDisplay formats a validated E.164 number for user display.
// For known country codes, it applies common formatting conventions.
// Returns the original number if formatting fails.
func FormatPhoneNumberForDisplay(phoneNumber string) string {
	// Validate first
	if err := ValidatePhoneNumber(phoneNumber); err != nil {
		return phoneNumber
	}

	code, country, err := DetectCountryCode(phoneNumber)
	if err != nil {
		// If there's an error (like wrong length for country), still format with space
		if code != "" {
			nationalNumber := phoneNumber[1+len(code):]
			return fmt.Sprintf("+%s %s", code, nationalNumber)
		}
		return phoneNumber
	}

	// Remove + and country code
	nationalNumber := phoneNumber[1+len(code):]

	// Apply country-specific formatting for known countries
	if country != countryUnknown {
		formatted := formatKnownCountry(code, nationalNumber)
		if formatted != "" {
			return formatted
		}
	}

	// Default formatting: add space after country code
	return fmt.Sprintf("+%s %s", code, nationalNumber)
}

// formatKnownCountry applies country-specific formatting rules.
func formatKnownCountry(code, nationalNumber string) string {
	switch code {
	case "1": // US/Canada: +1 (XXX) XXX-XXXX
		if len(nationalNumber) == phoneLength10 {
			return fmt.Sprintf("+1 (%s) %s-%s",
				nationalNumber[:3],
				nationalNumber[3:6],
				nationalNumber[6:])
		}
	case "44": // UK: +44 XXXX XXXXXX or similar
		if len(nationalNumber) >= phoneLength10 {
			return fmt.Sprintf("+44 %s %s",
				nationalNumber[:4],
				nationalNumber[4:])
		}
	case "49": // Germany: +49 XXX XXXXXXXX
		if len(nationalNumber) >= phoneLength10 {
			return fmt.Sprintf("+49 %s %s",
				nationalNumber[:3],
				nationalNumber[3:])
		}
	}
	return ""
}
