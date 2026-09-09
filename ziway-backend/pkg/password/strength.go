package password

import (
	"errors"
	"unicode"
)

// ValidateStrength enforces the SEC-3 password policy:
// at least 8 characters and 3 of 4 character classes (lower/upper/digit/symbol).
func ValidateStrength(pw string) error {
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var lower, upper, digit, symbol bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	classes := 0
	for _, b := range []bool{lower, upper, digit, symbol} {
		if b {
			classes++
		}
	}
	if classes < 3 {
		return errors.New("password must contain at least 3 of: lowercase, uppercase, digit, symbol")
	}
	return nil
}
