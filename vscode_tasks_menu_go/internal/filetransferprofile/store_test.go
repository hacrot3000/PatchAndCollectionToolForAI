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
