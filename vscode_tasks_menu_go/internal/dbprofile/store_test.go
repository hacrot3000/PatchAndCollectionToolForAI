package dbprofile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStoreRoundTripKeepsSecretReferencePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taskdeck", "db_profiles.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Profile{
		ID:        "mysql-prod",
		Name:      "MySQL Production",
		AdapterID: "mysql-cli",
		Host:      "db.example.com",
		Port:      3306,
		Username:  "app",
		Database:  "main",
		SecretRef: "db/mysql-prod/password",
	}
	if _, err := store.Create(want); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretRef != want.SecretRef {
		t.Fatalf("secret ref=%q want=%q", got.SecretRef, want.SecretRef)
	}
	publicJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), want.SecretRef) || strings.Contains(string(publicJSON), "secret_ref") {
		t.Fatalf("secret reference leaked into public JSON: %s", publicJSON)
	}

	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(disk), want.SecretRef) {
		t.Fatalf("private store did not persist secret reference: %s", disk)
	}
}

func TestStoreCreateReplaceDelete(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "db_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(Profile{
		ID: "db", Name: "DB", AdapterID: "mysql-cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(created); err == nil {
		t.Fatal("expected duplicate create to fail")
	}

	created.Name = "DB renamed"
	updated, err := store.Replace(created.ID, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "DB renamed" {
		t.Fatalf("updated=%+v", updated)
	}

	deleted, err := store.Delete(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != created.ID {
		t.Fatalf("deleted=%+v", deleted)
	}
	if _, err := store.Get(created.ID); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Get after delete error=%v", err)
	}
}

func TestStoreUsesRestrictedPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	dir := filepath.Join(t.TempDir(), "taskdeck")
	path := filepath.Join(dir, "db_profiles.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(Profile{ID: "db", Name: "DB", AdapterID: "mysql-cli"}); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode=%o want 700", got)
	}
	for _, file := range []string{path, path + ".lock"} {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode=%o want 600", file, got)
		}
	}
}

func TestStoreRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db_profiles.json")
	data := "{\n  \"version\": 1,\n  \"profiles\": [],\n  \"unexpected\": true\n}\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error=%v", err)
	}
}
