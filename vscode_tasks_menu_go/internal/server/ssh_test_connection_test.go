package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func newSSHConnectionTestServer(t *testing.T) (*Server, *sshprofile.Store) {
	t.Helper()
	store, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "prod.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Workspace: t.TempDir(), SSHProfiles: store}, store
}

func writeSSHStub(t *testing.T, body string) {
	t.Helper()
	binDir := t.TempDir()
	path := filepath.Join(binDir, "ssh")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
}

func TestSSHConnectionTestAPISucceedsWithoutOpeningSession(t *testing.T) {
	s, _ := newSSHConnectionTestServer(t)
	writeSSHStub(t, `
case "$1" in
  -V) echo "OpenSSH_fixture" >&2; exit 0 ;;
esac
exit 0
`)

	req := httptest.NewRequest(http.MethodPost, "/api/ssh/test", strings.NewReader(`{"profile_id":"prod"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "SSH connection succeeded") {
		t.Fatalf("unexpected response: %s", body)
	}
}

func TestSSHConnectionTestAPIReturnsBoundedFailureDetails(t *testing.T) {
	s, _ := newSSHConnectionTestServer(t)
	writeSSHStub(t, `
printf 'permission denied: fixture\n' >&2
exit 255
`)

	req := httptest.NewRequest(http.MethodPost, "/api/ssh/test", strings.NewReader(`{"profile_id":"prod"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"ok":false`) || !strings.Contains(body, "permission denied: fixture") {
		t.Fatalf("unexpected response: %s", body)
	}
}

func TestBoundedSSHCommandOutputTruncates(t *testing.T) {
	out := &boundedCommandOutput{remaining: 4}
	if n, err := out.Write([]byte("abcdefgh")); err != nil || n != 8 {
		t.Fatalf("Write n=%d err=%v", n, err)
	}
	if got := out.String(); got != "abcd\n[output truncated]" {
		t.Fatalf("output=%q", got)
	}
}


func TestSSHConnectionTestAPIUsesUnsavedProfileWithoutPersistingIt(t *testing.T) {
	s, store := newSSHConnectionTestServer(t)
	writeSSHStub(t, `
case "$1" in
  -V) echo "OpenSSH_fixture" >&2; exit 0 ;;
esac
exit 0
`)

	req := httptest.NewRequest(http.MethodPost, "/api/ssh/test", strings.NewReader(`{
		"profile":{
			"name":"",
			"host":"preview.example.com",
			"port":22,
			"username":"preview",
			"auth_method":"password",
			"secret":"temporary-password"
		}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "SSH connection succeeded") {
		t.Fatalf("unexpected response: %s", body)
	}

	profiles, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != "prod" {
		t.Fatalf("unsaved SSH test mutated profile store: %#v", profiles)
	}
}


func TestSSHConnectionTestAPIUsesEditedFieldsWithSavedSecret(t *testing.T) {
	s, store := newSSHConnectionTestServer(t)
	secrets, err := secretstore.NewFileStore(filepath.Join(t.TempDir(), "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	const secretRef = "ssh/saved/auth/test"
	if err := secrets.Put(secretRef, []byte("saved-password")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(sshprofile.Profile{
		ID:         "saved",
		Name:       "Saved password profile",
		Host:       "old.example.com",
		Username:   "olduser",
		AuthMethod: sshprofile.AuthPassword,
		SecretRef:  secretRef,
	}); err != nil {
		t.Fatal(err)
	}
	s.ConnectionSecrets = secrets
	writeSSHStub(t, `
case "$*" in
  *"preview@edited.example.com"*) exit 0 ;;
  *) echo "edited SSH target missing: $*" >&2; exit 2 ;;
esac
`)

	req := httptest.NewRequest(http.MethodPost, "/api/ssh/test", strings.NewReader(`{
		"profile_id":"saved",
		"profile":{
			"name":"Edited profile",
			"host":"edited.example.com",
			"port":22,
			"username":"preview",
			"auth_method":"password"
		}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "SSH connection succeeded") {
		t.Fatalf("unexpected response: %s", body)
	}
}
