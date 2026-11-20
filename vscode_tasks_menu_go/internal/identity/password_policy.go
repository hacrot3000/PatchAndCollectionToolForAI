package identity

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MinPasswordRunes = 12
	MaxPasswordBytes = 4096
)

// ValidateNewPassword is the single policy for newly assigned shared identity
// passwords. It intentionally avoids composition rules that encourage
// predictable substitutions; length and transport safety are enforced here.
func ValidateNewPassword(password string) error {
	if !utf8.ValidString(password) ||
		utf8.RuneCountInString(password) < MinPasswordRunes ||
		len(password) > MaxPasswordBytes ||
		strings.ContainsAny(password, "\r\n\x00") {
		return fmt.Errorf("password must contain at least %d characters, at most %d bytes, and no line breaks or NUL", MinPasswordRunes, MaxPasswordBytes)
	}
	return nil
}
