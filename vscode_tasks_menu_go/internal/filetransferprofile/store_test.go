package filetransferprofile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStoreRoundTripPreservesPrivateSecretRef(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file_transfer_profiles.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(Profile{
		ID:       "ftp-prod",
		Name:     "FTP Production",
		Protocol: ProtocolFTP,
		Host:     "ftp.example.com",
		Username: "deploy",
		SecretRef: "file-transfer/ftp-prod/password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.SecretRef == "" {
		t.Fatal("created profile lost secret ref")
	}
	got, err := store.Get("ftp-prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretRef != created.SecretRef {
		t.Fatalf("secret ref = %q, want %q", got.SecretRef, created.SecretRef)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("store mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestStoreCreateReplaceDelete(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "file_transfer_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(Profile{
		ID:           "sftp-prod",
		Name:         "SFTP",
		Protocol:     ProtocolSFTP,
		SSHProfileID: "ssh-prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.Replace("sftp-prod", Profile{
		Name:         "SFTP Updated",
		Protocol:     ProtocolSFTP,
		SSHProfileID: "ssh-prod",
		InitialPath:  "/srv/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.InitialPath != "/srv/app" {
		t.Fatalf("updated path = %q", updated.InitialPath)
	}
	deleted, err := store.Delete("sftp-prod")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != "sftp-prod" {
		t.Fatalf("deleted ID = %q", deleted.ID)
	}
	if _, err := store.Get("sftp-prod"); err != ErrProfileNotFound {
		t.Fatalf("Get after delete error = %v", err)
	}
}


func TestStoreSaveReplacesAllProfilesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file_transfer_profiles.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(Profile{
		ID: "old-profile", Name: "Old", Protocol: ProtocolFTP, Host: "old.example.com",
	}); err != nil {
		t.Fatal(err)
	}

	want := []Profile{
		{
			ID: "ftp-one", Name: "FTP One", Protocol: ProtocolFTP, Host: "ftp1.example.com",
			Username: "deploy", SecretRef: "file-transfer/ftp-one/password",
		},
		{
			ID: "sftp-two", Name: "SFTP Two", Protocol: ProtocolSFTP, SSHProfileID: "ssh-two",
			InitialPath: "/srv/app",
		},
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("profile count = %d, want 2", len(got))
	}
	if got[0].ID != "ftp-one" || got[0].SecretRef != "file-transfer/ftp-one/password" {
		t.Fatalf("first profile = %+v", got[0])
	}
	if got[1].ID != "sftp-two" || got[1].InitialPath != "/srv/app" {
		t.Fatalf("second profile = %+v", got[1])
	}
	if _, err := store.Get("old-profile"); err != ErrProfileNotFound {
		t.Fatalf("old profile still exists: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("store mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestStoreSaveRejectsDuplicateProfileIDs(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "file_transfer_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Save([]Profile{
		{ID: "dup", Name: "One", Protocol: ProtocolFTP, Host: "one.example.com"},
		{ID: "dup", Name: "Two", Protocol: ProtocolFTP, Host: "two.example.com"},
	})
	if err == nil {
		t.Fatal("Save unexpectedly accepted duplicate profile IDs")
	}
}
