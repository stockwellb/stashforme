package auth

import (
	"errors"
	"regexp"
	"strings"
)

var (
	// ErrInvalidPhoneNumber indicates the phone number is not in valid E.164 format
	ErrInvalidPhoneNumber = errors.New("invalid phone number: must be in E.164 format (e.g., +14155551234)")

	// e164Regex matches E.164 phone numbers: + followed by 1-15 digits
	e164Regex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

	// digitsOnly matches strings of only digits
	digitsOnly = regexp.MustCompile(`^[1-9]\d{1,14}$`)
)

// NormalizePhoneNumber cleans up a phone number, adds + and country code 1 if missing
func NormalizePhoneNumber(phone string) string {
	// Remove spaces, dashes, parentheses
	phone = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '+' {
			return r
		}
		return -1
	}, phone)

	// Strip leading + for processing
	hasPlus := strings.HasPrefix(phone, "+")
	if hasPlus {
		phone = phone[1:]
	}

	// If 10 digits, assume US number missing country code
	if len(phone) == 10 {
		phone = "1" + phone
	}

	return "+" + phone
}

// ValidatePhoneNumber checks if the phone number is in valid E.164 format
func ValidatePhoneNumber(phone string) error {
	if !e164Regex.MatchString(phone) {
		return ErrInvalidPhoneNumber
	}
	return nil
}
