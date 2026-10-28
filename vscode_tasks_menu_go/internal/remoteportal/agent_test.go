package remoteportal

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"strconv"
	"testing"
)

func TestIdentityPersistenceAndFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "identity.seed")
	first, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateIdentity(path)
	if err != nil || second.DeviceID() != first.DeviceID() {
		t.Fatalf("identity changed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("key not owner-only: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateIdentity(path); err == nil {
		t.Fatal("insecure identity file was accepted")
	}
}

func TestAgentSignedEnrollmentAndHeartbeat(t *testing.T) {
	key := filepath.Join(t.TempDir(), "seed")
	var requests []signedMessage
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/agent/") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var p signedMessage
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("invalid JSON: %v", err)
		}
		action := "heartbeat"
		fields := []string{p.DeviceID}
		if r.URL.Path == "/api/agent/enroll" {
			action = "enroll"
			fields = append(fields, p.DisplayName, p.PairingHash)
		}
		key, err := base64.StdEncoding.DecodeString(p.PublicKey)
		if err != nil || len(key) != ed25519.PublicKeySize {
			t.Error("invalid public key")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		digest := sha256.Sum256(key)
		if hex.EncodeToString(digest[:]) != p.DeviceID {
			t.Error("fingerprint mismatch")
		}
		message := strings.Join(append(append([]string{protocol, action}, fields...), 
			intString(p.Timestamp), p.Nonce), "\n")
		sig, _ := base64.StdEncoding.DecodeString(p.Signature)
		if !ed25519.Verify(ed25519.PublicKey(key), []byte(message), sig) {
			t.Error("invalid signature on " + action)
		}
		requests = append(requests, p)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"ok\":true}"))
	}))
	defer srv.Close()
	agent, err := NewAgent(srv.URL, key, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	code, err := agent.Enroll(context.Background(), "Mac mini / Project")
	if err != nil || len(code) != 32 {
		t.Fatalf("enroll: %v", err)
	}
	raw, _ := hex.DecodeString(code)
	sum := sha256.Sum256(raw)
	if requests[0].PairingHash != hex.EncodeToString(sum[:]) {
		t.Fatal("pairing hash mismatch")
	}
	if err := agent.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests[0].Nonce == requests[1].Nonce {
		t.Fatal("nonce reused")
	}
	if _, err := NewAgent("http://example.com", key, nil); err == nil {
		t.Fatal("plaintext portal URL accepted")
	}
}

func intString(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestIdentityRejectsUnsafeParent(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "world-readable")
	if err := os.Mkdir(directory, 0755); err != nil { t.Fatal(err) }
	if _, err := LoadOrCreateIdentity(filepath.Join(directory, "key.seed")); err == nil {
		t.Fatal("identity accepted world-readable parent")
	}
	private := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(private, 0700); err != nil { t.Fatal(err) }
	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(private, symlink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := LoadOrCreateIdentity(filepath.Join(symlink, "key.seed")); err == nil {
		t.Fatal("identity accepted symlinked parent")
	}
}
