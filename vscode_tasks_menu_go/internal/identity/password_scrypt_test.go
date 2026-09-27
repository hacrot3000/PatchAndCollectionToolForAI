package identity

import (
	"context"
	"strings"
	"testing"
)

func TestPasswordScryptHashAndVerify(t *testing.T) {
	requirePythonSQLite(t)

	password := "Mật khẩu shared server 🔐"
	encoded, err := HashPassword(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, passwordScryptHashPrefix) {
		t.Fatalf("hash prefix=%q", encoded)
	}
	if strings.Contains(encoded, password) {
		t.Fatal("password hash contains plaintext password")
	}

	salt, key, err := parsePasswordScryptHash(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(salt) != passwordScryptSaltBytes {
		t.Fatalf("salt length=%d want=%d", len(salt), passwordScryptSaltBytes)
	}
	if len(key) != passwordScryptKeyBytes {
		t.Fatalf("key length=%d want=%d", len(key), passwordScryptKeyBytes)
	}

	ok, err := VerifyPassword(context.Background(), password, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("correct password did not verify")
	}
}

func TestPasswordScryptRejectsMalformedHashWithoutDerivation(t *testing.T) {
	for _, encoded := range []string{
		"",
		"$argon2id$example",
		"$scrypt$v=1,ln=16,r=8,p=1$bad$bad",
		"$scrypt$v=1,ln=17,r=8,p=1$bad$bad",
		"$scrypt$v=1,ln=17,r=8,p=1$AAAAAAAAAAAAAAAAAAAAAA$bad",
	} {
		if _, _, err := parsePasswordScryptHash(encoded); err == nil {
			t.Fatalf("malformed hash accepted: %q", encoded)
		}
	}
}

func TestPasswordScryptInputBounds(t *testing.T) {
	if _, err := HashPassword(context.Background(), ""); err == nil {
		t.Fatal("empty password accepted")
	}

	tooLong := strings.Repeat("x", passwordScryptMaxBytes+1)
	if _, err := HashPassword(context.Background(), tooLong); err == nil {
		t.Fatal("oversized password accepted")
	}

	ok, err := VerifyPassword(context.Background(), "", "$scrypt$invalid")
	if err != nil || ok {
		t.Fatalf("empty verification result ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(context.Background(), tooLong, "$scrypt$invalid")
	if err != nil || ok {
		t.Fatalf("oversized verification result ok=%v err=%v", ok, err)
	}
}
