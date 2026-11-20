package identity

import (
	"strings"
	"testing"
)

func TestValidateNewPassword(t *testing.T) {
	for _, value := range []string{
		"private-admin-password",
		"十二个字符以上的安全口令",
		strings.Repeat("x", MaxPasswordBytes),
	} {
		if err := ValidateNewPassword(value); err != nil {
			t.Fatalf("valid password rejected: %v", err)
		}
	}
	for _, value := range []string{
		"",
		"short",
		"private-password\nsecond-line",
		"private-password\rsecond-line",
		"private-password\x00",
		strings.Repeat("x", MaxPasswordBytes+1),
		string([]byte{'p','r','i','v','a','t','e','-','p','a','s','s',0xff}),
	} {
		if err := ValidateNewPassword(value); err == nil {
			t.Fatalf("invalid password accepted: %q", value)
		}
	}
}
