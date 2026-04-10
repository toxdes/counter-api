package utils

import (
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxLabelLength is the maximum length for labels (tenant/counter)
	MaxLabelLength = 255

	// MinLabelLength is the minimum length for labels
	MinLabelLength = 1

	// MaxCounterValue is the maximum value for a counter to prevent overflow
	MaxCounterValue = 9223372036854775806 // int64 max - 1 to allow incrementing

	// MinCounterValue is the minimum value for a counter
	MinCounterValue = -9223372036854775808 // int64 min
)

var (
	// validLabelRegex matches valid label characters
	// Allows alphanumeric, spaces, hyphens, underscores, and dots
	validLabelRegex = regexp.MustCompile(`^[\w\s\-\.]+$`)

	// ErrLabelTooLong is returned when label exceeds maximum length
	ErrLabelTooLong = errors.New("label exceeds maximum length")

	// ErrLabelTooShort is returned when label is too short
	ErrLabelTooShort = errors.New("label is too short")

	// ErrLabelInvalidChars is returned when label contains invalid characters
	ErrLabelInvalidChars = errors.New("label contains invalid characters")

	// ErrInvalidUUID is returned when UUID is invalid
	ErrInvalidUUID = errors.New("invalid UUID format")

	// ErrCounterValueOverflow is returned when counter value would overflow
	ErrCounterValueOverflow = errors.New("counter value would cause overflow")

	// ErrCounterValueUnderflow is returned when counter value would underflow
	ErrCounterValueUnderflow = errors.New("counter value would cause underflow")
)

// ValidateLabel validates a label string
func ValidateLabel(label string) error {
	if label == "" {
		return ErrLabelTooShort
	}

	length := utf8.RuneCountInString(label)
	if length < MinLabelLength {
		return ErrLabelTooShort
	}

	if length > MaxLabelLength {
		return ErrLabelTooLong
	}

	if !validLabelRegex.MatchString(label) {
		return ErrLabelInvalidChars
	}

	return nil
}

// ValidateUUID validates a UUID string
func ValidateUUID(id string) error {
	if id == "" {
		return ErrInvalidUUID
	}

	_, err := uuid.Parse(id)
	if err != nil {
		return ErrInvalidUUID
	}

	return nil
}

// ValidateCounterValue validates a counter value for potential overflow/underflow
func ValidateCounterValue(value int64) error {
	if value > MaxCounterValue {
		return ErrCounterValueOverflow
	}

	if value < MinCounterValue {
		return ErrCounterValueUnderflow
	}

	return nil
}

// ValidateIncrement validates that incrementing by delta won't overflow
func ValidateIncrement(currentValue, delta int64) error {
	// Check for positive overflow
	if delta > 0 {
		if currentValue > MaxCounterValue-delta {
			return ErrCounterValueOverflow
		}
	}

	// Check for negative underflow
	if delta < 0 {
		if currentValue < MinCounterValue-delta {
			return ErrCounterValueUnderflow
		}
	}

	return nil
}

// SanitizeString sanitizes a string input by removing null bytes and trimming whitespace
func SanitizeString(s string) string {
	// Remove null bytes
	sanitized := regexp.MustCompile(`\x00`).ReplaceAllString(s, "")

	// Trim whitespace
	return sanitizeString(sanitized)
}

// SanitizeLabel sanitizes and validates a label
func SanitizeLabel(label string) (string, error) {
	sanitized := SanitizeString(label)
	err := ValidateLabel(sanitized)
	return sanitized, err
}

func sanitizeString(s string) string {
	// Trim leading and trailing whitespace
	// Using explicit loop to handle unicode properly
	runes := []rune(s)
	start, end := 0, len(runes)

	for start < end && isWhitespace(runes[start]) {
		start++
	}

	for end > start && isWhitespace(runes[end-1]) {
		end--
	}

	return string(runes[start:end])
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// FormatError formats a validation error with context
func FormatError(field string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", field, err)
}
