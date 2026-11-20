package sshprofile

import "testing"

func TestNewIDProducesValidDistinctIDs(t *testing.T) {
	first, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("duplicate generated id %q", first)
	}
	if err := validateID("profile id", first); err != nil {
		t.Fatalf("first id invalid: %v", err)
	}
	if err := validateID("profile id", second); err != nil {
		t.Fatalf("second id invalid: %v", err)
	}
}
