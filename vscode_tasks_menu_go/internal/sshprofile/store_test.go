package sshprofile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStoreRoundTripKeepsSecretRefPrivate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "private", "ssh_profiles.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "prod.example.com",
		Username:   "deploy",
		AuthMethod: AuthPassword,
		SecretRef:  "ssh/prod/password",
		PresetCommands: []PresetCommand{
			{ID: "logs", Name: "Logs", Command: "tail -f app.log", Cwd: "/srv/app"},
		},
	}
	if err := store.Save([]Profile{want}); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("profiles = %d, want 1", len(got))
	}
	if got[0].SecretRef != want.SecretRef {
		t.Fatalf("secret ref = %q, want %q", got[0].SecretRef, want.SecretRef)
	}

	publicJSON, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), want.SecretRef) || strings.Contains(string(publicJSON), "secret_ref") {
		t.Fatalf("secret reference leaked from public profile JSON: %s", publicJSON)
	}

	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(disk), want.SecretRef) {
		t.Fatalf("private store did not persist secret reference: %s", disk)
	}
}

func TestStoreUsesRestrictedPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "taskdeck")
	path := filepath.Join(dir, "ssh_profiles.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save([]Profile{{
		ID: "prod", Name: "Production", Host: "prod.example.com", Username: "deploy", AuthMethod: AuthAgent,
	}}); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode = %o, want 700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %o, want 600", got)
	}
}

func TestStoreRejectsDuplicateIDs(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com", Username: "deploy", AuthMethod: AuthAgent,
	}
	err = store.Save([]Profile{p, p})
	if err == nil || !strings.Contains(err.Error(), "duplicate ssh profile id") {
		t.Fatalf("error = %v", err)
	}
}

func TestStoreRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_profiles.json")
	data := `{
  "version": 1,
  "profiles": [],
  "unexpected": true
}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewStoreRequiresAbsolutePath(t *testing.T) {
	if _, err := NewStore("relative/ssh_profiles.json"); err == nil {
		t.Fatal("expected absolute path validation error")
	}
}
