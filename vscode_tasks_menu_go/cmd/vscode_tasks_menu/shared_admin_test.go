package main

import (
	"strings"
	"testing"
)

func TestBootstrapPasswordStdinIsBoundedAndSingleLine(t *testing.T) {
	for _, suffix := range []string{"", "\n", "\r\n"} {
		want := "private-admin-password"
		got, err := readBootstrapPassword(strings.NewReader(want + suffix))
		if err != nil || got != want {
			t.Fatalf("read password: %v", err)
		}
	}
	for _, value := range []string{"short", "private-password\nsecond-line", strings.Repeat("x", 4097), "private-password\x00", "private-password\xff"} {
		if _, err := readBootstrapPassword(strings.NewReader(value)); err == nil {
			t.Fatal("invalid password input accepted")
		}
	}
}
