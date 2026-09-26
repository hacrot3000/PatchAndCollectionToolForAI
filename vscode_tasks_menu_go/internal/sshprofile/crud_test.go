package sshprofile

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestStoreCreateReplaceDelete(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.Create(Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com", Username: "deploy", AuthMethod: AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Port != DefaultPort {
		t.Fatalf("created port = %d", created.Port)
	}
	if _, err := store.Create(created); err == nil {
		t.Fatal("expected duplicate create failure")
	}

	updated := created
	updated.Name = "Production 2"
	got, err := store.Replace("prod", updated)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Production 2" {
		t.Fatalf("updated name = %q", got.Name)
	}

	loaded, err := store.Get("prod")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Production 2" {
		t.Fatalf("loaded name = %q", loaded.Name)
	}

	deleted, err := store.Delete("prod")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != "prod" {
		t.Fatalf("deleted id = %q", deleted.ID)
	}
	if _, err := store.Get("prod"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Get after delete error = %v", err)
	}
	if _, err := store.Delete("prod"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
}
