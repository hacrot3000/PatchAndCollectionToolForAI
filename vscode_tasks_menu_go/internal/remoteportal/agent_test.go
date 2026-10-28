package remoteportal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"sync/atomic"
	"syscall"
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
	key := filepath.Join(t.TempDir(), "private", "seed")
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

func TestPortalHeartbeatFailureClearsCachedDeviceLease(t *testing.T) {
	var reject atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reject.Load() {
			http.Error(w, "device unpaired", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"device_lease":"mock-signed-lease"}`))
	}))
	defer server.Close()

	agent, err := NewAgent(server.URL,
		filepath.Join(t.TempDir(), "private", "identity.seed"), server.Client())
	if err != nil {t.Fatal(err)}
	if err := agent.Heartbeat(context.Background()); err != nil {t.Fatal(err)}
	if agent.currentLease() != "mock-signed-lease" {
		t.Fatal("authenticated Portal lease not cached")
	}
	reject.Store(true)
	if err := agent.Heartbeat(context.Background()); err == nil {
		t.Fatal("Portal rejected heartbeat but Agent reported success")
	}
	if agent.currentLease() != "" {
		t.Fatal("Agent retained device lease after Portal revoked access")
	}
}

type forgedIdentityOwner struct {
	os.FileInfo
	uid uint32
}
func (f forgedIdentityOwner) Sys() any {return &syscall.Stat_t{Uid:f.uid}}

// A compromised shared-host/PHP user may produce owner-only 0600/0700
// *looking* files but cannot become this Agent's trusted OS owner.
func TestAgentIdentitySeedAndDirectoryRequireEffectiveOwner(t *testing.T) {
	d:=t.TempDir()
	if err:=os.Chmod(d,0700);err!=nil{t.Fatal(err)}
	p:=filepath.Join(d,"device.seed")
	if err:=os.WriteFile(p,make([]byte,ed25519.SeedSize),0600);err!=nil{t.Fatal(err)}
	dir,err:=os.Lstat(d)
	if err!=nil{t.Fatal(err)}
	file,err:=os.Lstat(p)
	if err!=nil{t.Fatal(err)}
	if !ownerOnlyIdentity(dir,true) || !ownerOnlyIdentity(file,false) {
		t.Fatal("current owner was unable to use private Agent identity")
	}
	other:=uint32(os.Geteuid())+1
	if ownerOnlyIdentity(forgedIdentityOwner{FileInfo:file,uid:other},false) {
		t.Fatal("0600 Agent key owned by another OS tenant was accepted")
	}
	if ownerOnlyIdentity(forgedIdentityOwner{FileInfo:dir,uid:other},true) {
		t.Fatal("0700 Agent identity directory owned by another tenant was accepted")
	}
	if ownerOnlyIdentity(dir,false) || ownerOnlyIdentity(file,true) {
		t.Fatal("Agent identity inode type check failed")
	}
	if err:=os.Chmod(p,0640);err!=nil{t.Fatal(err)}
	file,err=os.Lstat(p)
	if err!=nil{t.Fatal(err)}
	if ownerOnlyIdentity(file,false) {t.Fatal("group-readable Agent seed accepted")}
}

func TestAgentIdentityRejectsSymlinkedSeedAndMalformedInode(t *testing.T) {
	d:=filepath.Join(t.TempDir(),"private")
	if err:=os.Mkdir(d,0700);err!=nil{t.Fatal(err)}
	source:=filepath.Join(d,"original.seed")
	seed:=make([]byte,ed25519.SeedSize)
	if _,err:=rand.Read(seed);err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(source,seed,0600);err!=nil{t.Fatal(err)}
	link:=filepath.Join(d,"device.seed")
	if err:=os.Symlink(source,link);err!=nil{t.Skipf("symlink unavailable: %v",err)}
	if _,err:=readSeed(link);err==nil {
		t.Fatal("readSeed followed a swapped symlink to a private Agent seed")
	}
	if _,err:=LoadOrCreateIdentity(link);err==nil {
		t.Fatal("Agent trusted an existing symlink instead of its own seed inode")
	}
	if got,err:=readSeed(source);err!=nil || string(got)!=string(seed) {
		t.Fatalf("real 0600 owner-owned Agent seed was rejected: %v",err)
	}
	if err:=os.Remove(link);err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(link,append(seed,0x01),0600);err!=nil{t.Fatal(err)}
	if _,err:=LoadOrCreateIdentity(link);err==nil {
		t.Fatal("Agent accepted seed with extra trailing bytes")
	}
}

func TestAgentIdentityCreationSurvivesRestartAndRejectsSymlinkTarget(t *testing.T) {
	d:=filepath.Join(t.TempDir(),"private")
	key:=filepath.Join(d,"device.seed")
	first,err:=LoadOrCreateIdentity(key)
	if err!=nil{t.Fatal(err)}
	second,err:=LoadOrCreateIdentity(key)
	if err!=nil || first.DeviceID()!=second.DeviceID() {
		t.Fatalf("Agent identity changed across process restart: %v",err)
	}
	// An attacker cannot replace the 32-byte descriptor by a symlink,
	// even if the target file remains owner-only and contains that seed.
	target:=filepath.Join(d,"backup.seed")
	raw,err:=os.ReadFile(key)
	if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(target,raw,0600);err!=nil{t.Fatal(err)}
	if err:=os.Remove(key);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(target,key);err!=nil{t.Skipf("symlinks unavailable: %v",err)}
	if _,err:=LoadOrCreateIdentity(key);err==nil {
		t.Fatal("Agent accepted symlinked key during reboot")
	}
}
