package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifySSHHostKeyFailure(t *testing.T) {
	if got := classifySSHHostKeyFailure("WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!"); got != "host_key_changed" {
		t.Fatalf("changed classification=%q", got)
	}
	if got := classifySSHHostKeyFailure("Host key verification failed."); got != "host_key_verification" {
		t.Fatalf("verification classification=%q", got)
	}
	if got := classifySSHHostKeyFailure("Permission denied (publickey)."); got != "" {
		t.Fatalf("unexpected classification=%q", got)
	}
}

func writeSSHHostKeyFixtureTools(t *testing.T, knownHosts string) {
	t.Helper()
	bin := t.TempDir()
	sshPath := filepath.Join(bin, "ssh")
	sshScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-G\" ]; then echo \"userknownhostsfile $TASKDECK_TEST_KNOWN_HOSTS\"; exit 0; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(sshPath, []byte(sshScript), 0o700); err != nil { t.Fatal(err) }
	keygenPath := filepath.Join(bin, "ssh-keygen")
	keygenScript := `#!/bin/sh
action="$1"
lookup="$2"
shift 2
file=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-f" ]; then file="$2"; shift 2; else shift; fi
done
case "$action" in
  -F)
    if [ -f "$file" ] && grep -q "$lookup" "$file"; then
      echo "# Host $lookup found: line 1"
      grep "$lookup" "$file"
      exit 0
    fi
    exit 1 ;;
  -R)
    if [ -f "$file" ]; then
      tmp="$file.tmp"
      grep -v "$lookup" "$file" > "$tmp" || true
      mv "$tmp" "$file"
      echo "# Host $lookup found: line 1"
      echo "$file updated."
      exit 0
    fi
    exit 1 ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(keygenPath, []byte(keygenScript), 0o700); err != nil { t.Fatal(err) }
	t.Setenv("TASKDECK_TEST_KNOWN_HOSTS", knownHosts)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSSHHostKeyRecoveryInspectAndRemove(t *testing.T) {
	s, _ := newSSHConnectionTestServer(t)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHosts, []byte("prod.example.com ssh-ed25519 AAAATESTKEY\n"), 0o600); err != nil { t.Fatal(err) }
	writeSSHHostKeyFixtureTools(t, knownHosts)

	inspectReq := httptest.NewRequest(http.MethodPost, "/api/ssh/host-key", strings.NewReader(`{"profile_id":"prod","action":"inspect"}`))
	inspectReq.Header.Set("Content-Type", "application/json")
	inspectRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(inspectRR, inspectReq)
	if inspectRR.Code != http.StatusOK { t.Fatalf("inspect status=%d body=%s", inspectRR.Code, inspectRR.Body.String()) }
	if !strings.Contains(inspectRR.Body.String(), `"can_remove":true`) || !strings.Contains(inspectRR.Body.String(), knownHosts) {
		t.Fatalf("inspect body=%s", inspectRR.Body.String())
	}

	unsafeReq := httptest.NewRequest(http.MethodPost, "/api/ssh/host-key", strings.NewReader(`{"profile_id":"prod","action":"remove"}`))
	unsafeReq.Header.Set("Content-Type", "application/json")
	unsafeRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(unsafeRR, unsafeReq)
	if unsafeRR.Code != http.StatusBadRequest { t.Fatalf("unconfirmed remove status=%d body=%s", unsafeRR.Code, unsafeRR.Body.String()) }

	removeReq := httptest.NewRequest(http.MethodPost, "/api/ssh/host-key", strings.NewReader(`{"profile_id":"prod","action":"remove","confirmed":true}`))
	removeReq.Header.Set("Content-Type", "application/json")
	removeRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(removeRR, removeReq)
	if removeRR.Code != http.StatusOK { t.Fatalf("remove status=%d body=%s", removeRR.Code, removeRR.Body.String()) }
	if !strings.Contains(removeRR.Body.String(), `"ok":true`) || !strings.Contains(removeRR.Body.String(), "Reconnect explicitly") {
		t.Fatalf("remove body=%s", removeRR.Body.String())
	}
	raw, err := os.ReadFile(knownHosts)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(raw), "prod.example.com") { t.Fatalf("known_hosts entry remains: %q", raw) }
}

func TestSSHConnectionTestClassifiesChangedHostKey(t *testing.T) {
	s, _ := newSSHConnectionTestServer(t)
	writeSSHStub(t, `
printf "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!\nThe fingerprint for the ED25519 key sent by the remote host is SHA256:Fixture123=.\n" >&2
exit 255
`)
	req := httptest.NewRequest(http.MethodPost, "/api/ssh/test", strings.NewReader(`{"profile_id":"prod"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
	body := rr.Body.String()
	if !strings.Contains(body, `"failure_code":"host_key_changed"`) || !strings.Contains(body, `"candidate_fingerprint":"SHA256:Fixture123="`) {
		t.Fatalf("body=%s", body)
	}
}
