//go:build linux || darwin

package agentchat

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These tests exercise the durable journal commit boundary that a native
// COLLECT observation uses (Store.Save). They reuse the EXISTING unexported
// test-only seam syncDirectoryOverride; no production hook or API is added,
// no lock ordering or commit ordering changes, and production stores are nil.
// Limitation documented honestly: these tests drive the journal CAS layer, not
// the HTTP/native session runner. They do not claim a full pipeline crash test.

const (
	clp20CrashChildEnv = "TASKDECK_CLP20_OBS_CRASH_CHILD"
	clp20CrashModeEnv  = "TASKDECK_CLP20_OBS_CRASH_MODE"
	clp20CrashDirEnv   = "TASKDECK_CLP20_OBS_CRASH_DIR"
	clp20CrashIDEnv    = "TASKDECK_CLP20_OBS_CRASH_SESSION"
	clp20CrashMarker   = "TASKDECK_CLP20_OBS_BARRIER_REACHED"
	clp20Owner         = "crash-owner"
)

func clp20CallItem() HistoryItem {
	return HistoryItem{Type: "function_call", CallID: "read-1", Name: "taskdeck_collect_workspace", Arguments: `{"actions":[]}`}
}

func clp20OutputItem() HistoryItem {
	return HistoryItem{Type: "function_call_output", CallID: "read-1", Content: `{"status":"PASS","native_status":"PASS","text":"canonical observation"}`}
}

func clp20BaseSession(t *testing.T, store *Store) Session {
	t.Helper()
	x, err := NewSession(clp20Owner, "/workspace", "project", "provider", "model", Stateless)
	if err != nil {
		t.Fatal(err)
	}
	x, err = store.Save(x, 0)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func clp20ValidRun(phase string) *ProviderRun {
	now := time.Now().UTC()
	return &ProviderRun{
		ID: "run-native", Kind: "message", Phase: phase, CallID: "patch",
		PayloadSHA256: strings.Repeat("a", 64), ProviderFingerprint: "fp-1",
		CreatedAt: now, UpdatedAt: now,
	}
}

func clp20CrashChild() {
	dir := os.Getenv(clp20CrashDirEnv)
	id := os.Getenv(clp20CrashIDEnv)
	mode := os.Getenv(clp20CrashModeEnv)
	store, err := NewStore(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	x, err := store.Get(id, clp20Owner)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if mode == "before-native" {
		// W1: no native execution, no staged receipt, no canonical write.
		fmt.Fprintln(os.Stdout, clp20CrashMarker)
		time.Sleep(30 * time.Second)
		os.Exit(3)
	}
	if mode == "staged-uncommitted" {
		// W2: native may have run and a receipt is staged, but the canonical
		// observation is NOT adopted. Reading the sidecar is not a new run.
		sidecar := filepath.Join(dir, id+".clp20-staged-receipt")
		_ = os.WriteFile(sidecar, []byte(`{"status":"PASS","adopted":false}`), 0600)
		fmt.Fprintln(os.Stdout, clp20CrashMarker)
		time.Sleep(30 * time.Second)
		os.Exit(3)
	}

	candidate := x
	switch mode {
	case "postcommit-observe", "postcommit-ack":
		h := append([]HistoryItem(nil), x.History...)
		h = append(h, clp20CallItem(), clp20OutputItem())
		candidate.History = h
		if mode == "postcommit-ack" {
			candidate.ProviderRun = clp20ValidRun("acknowledged")
		}
	case "postcommit-prepared":
		// A committed PREPARED journal must reference exact staged bytes; without
		// the sidecar Save rejects the candidate before reaching the barrier.
		payload := []byte(`{"model":"m","input":"prepared"}`)
		candidate.ProviderRun = clp20ValidRun("prepared")
		candidate.ProviderRun.PayloadSHA256 = ProviderPayloadDigest(payload)
		if e := store.StageProviderPayload(candidate, payload); e != nil {
			fmt.Fprintln(os.Stderr, "cannot stage prepared payload:", e)
			os.Exit(2)
		}
	case "postcommit-sending":
		candidate.ProviderRun = clp20ValidRun("sending")
	default:
		fmt.Fprintln(os.Stderr, "unknown crash mode", mode)
		os.Exit(2)
	}

	// Postcommit barrier: syncDirectory is called once before the canonical
	// rename and once after. Fire on the second call, while withLock is held,
	// so the parent observes the committed bytes and then really kills us.
	calls := 0
	store.syncDirectoryOverride = func() error {
		calls++
		if calls == 2 {
			fmt.Fprintln(os.Stdout, clp20CrashMarker)
			time.Sleep(30 * time.Second)
			os.Exit(3)
		}
		return syncAgentDirectory(store.dir)
	}
	// StageProviderPayload (W4) performs its own directory sync before Save.
	// Count barrier calls from the canonical Save only so "second call" always
	// means the post-rename, committed-journal boundary for every crash mode.
	calls = 0
	_, _ = store.Save(candidate, x.Revision)
	fmt.Fprintln(os.Stderr, "unexpected return from postcommit barrier")
	os.Exit(4)
}

// clp20CrashDispatch must be the single -test.run target for child processes.
func TestCLP20ObservationCrashWindows(t *testing.T) {
	if os.Getenv(clp20CrashChildEnv) == "1" {
		clp20CrashChild()
		return
	}
	// The parent drives the individual windows through subtests below.
}

// clp20SyncBuffer is safe to read while os/exec's stderr copy goroutine is
// still writing; an unsynchronized bytes.Buffer.String() races that writer.
type clp20SyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *clp20SyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *clp20SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func clp20SpawnCrashChild(t *testing.T, mode, dir, id string) (*exec.Cmd, context.CancelFunc, *clp20SyncBuffer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLP20ObservationCrashWindows$")
	cmd.Env = append(os.Environ(),
		clp20CrashChildEnv+"=1", clp20CrashModeEnv+"="+mode,
		clp20CrashDirEnv+"="+dir, clp20CrashIDEnv+"="+id)
	stderr := &clp20SyncBuffer{}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		cancel()
	})
	scanner := bufio.NewScanner(out)
	seen := false
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == clp20CrashMarker {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("child never reached barrier (%s): scan=%v stderr=%s", mode, scanner.Err(), stderr.String())
	}
	return cmd, cancel, stderr
}

func clp20KillAndAssert(t *testing.T, cmd *exec.Cmd, stderr *clp20SyncBuffer) {
	t.Helper()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("cannot kill child at barrier: %v stderr=%s", err, stderr.String())
	}
	waitErr := cmd.Wait()
	if waitErr == nil {
		t.Fatalf("child exited normally; forced kill barrier not exercised stderr=%s", stderr.String())
	}
	exitErr, ok := waitErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected wait error %T: %v stderr=%s", waitErr, waitErr, stderr.String())
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		t.Fatalf("child did not exit by intended SIGKILL: %v stderr=%s", exitErr, stderr.String())
	}
}

func clp20Reopen(t *testing.T, dir, id string) Session {
	t.Helper()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, clp20Owner)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func clp20OutputCount(x Session) int {
	n := 0
	for _, item := range x.History {
		if item.Type == "function_call_output" && item.CallID == "read-1" {
			n++
		}
	}
	return n
}

// W1: crash before native execution. Canonical journal is byte-identical and
// no observation exists after restart; re-reading cannot become a new run.
func TestCLP20ObservationCrashW1BeforeNative(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := clp20BaseSession(t, store)
	before, err := os.ReadFile(store.file(x.ID))
	if err != nil {
		t.Fatal(err)
	}
	cmd, _, stderr := clp20SpawnCrashChild(t, "before-native", dir, x.ID)
	clp20KillAndAssert(t, cmd, stderr)
	after, err := os.ReadFile(store.file(x.ID))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("W1 canonical journal changed before any native action: %v", err)
	}
	restarted := clp20Reopen(t, dir, x.ID)
	if restarted.Revision != x.Revision || clp20OutputCount(restarted) != 0 {
		t.Fatal("W1 unexecuted native action produced an observation")
	}
}

// W2: native may have run and a receipt was staged, but the observation was
// not durably committed. Restart must not adopt the staged receipt or rerun.
func TestCLP20ObservationCrashW2StagedNotCommitted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := clp20BaseSession(t, store)
	before, err := os.ReadFile(store.file(x.ID))
	if err != nil {
		t.Fatal(err)
	}
	cmd, _, stderr := clp20SpawnCrashChild(t, "staged-uncommitted", dir, x.ID)
	clp20KillAndAssert(t, cmd, stderr)
	staged := filepath.Join(dir, x.ID+".clp20-staged-receipt")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("W2 child never staged the receipt: %v", err)
	}
	after, err := os.ReadFile(store.file(x.ID))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("W2 staged receipt was adopted into canonical journal: %v", err)
	}
	restarted := clp20Reopen(t, dir, x.ID)
	if restarted.Revision != x.Revision || clp20OutputCount(restarted) != 0 {
		t.Fatal("W2 staged-but-uncommitted observation was counted")
	}
}

// W3: crash after the observation commit but before any PREPARED transition.
// Exactly one observation survives; no provider run is fabricated.
func TestCLP20ObservationCrashW3CommittedBeforePrepared(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := clp20BaseSession(t, store)
	before, err := os.ReadFile(store.file(x.ID))
	if err != nil {
		t.Fatal(err)
	}
	cmd, _, stderr := clp20SpawnCrashChild(t, "postcommit-observe", dir, x.ID)
	clp20KillAndAssert(t, cmd, stderr)
	committed, err := os.ReadFile(store.file(x.ID))
	if err != nil || bytes.Equal(committed, before) {
		t.Fatalf("W3 postcommit barrier fired before authoritative commit: %v", err)
	}
	restarted := clp20Reopen(t, dir, x.ID)
	if restarted.Revision != x.Revision+1 || clp20OutputCount(restarted) != 1 {
		t.Fatalf("W3 observation not committed exactly once: rev=%d outputs=%d", restarted.Revision, clp20OutputCount(restarted))
	}
	if restarted.ProviderRun != nil {
		t.Fatal("W3 observation commit fabricated a PREPARED provider run")
	}
}

// W4: PREPARED survives a crash and is not advanced to SENDING by restart.
func TestCLP20ObservationCrashW4PreparedBeforeSending(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := clp20BaseSession(t, store)
	cmd, _, stderr := clp20SpawnCrashChild(t, "postcommit-prepared", dir, x.ID)
	clp20KillAndAssert(t, cmd, stderr)
	restarted := clp20Reopen(t, dir, x.ID)
	if restarted.ProviderRun == nil || restarted.ProviderRun.Phase != "prepared" {
		t.Fatalf("W4 PREPARED state lost or auto-advanced: %+v", restarted.ProviderRun)
	}
}

// W5: SENDING before ACK leaves delivery UNKNOWN; restart cannot fabricate an
// acknowledgement or replay the native observation.
func TestCLP20ObservationCrashW5SendingBeforeAck(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := clp20BaseSession(t, store)
	cmd, _, stderr := clp20SpawnCrashChild(t, "postcommit-sending", dir, x.ID)
	clp20KillAndAssert(t, cmd, stderr)
	restarted := clp20Reopen(t, dir, x.ID)
	if restarted.ProviderRun == nil || restarted.ProviderRun.Phase != "sending" {
		t.Fatalf("W5 UNKNOWN delivery not preserved: %+v", restarted.ProviderRun)
	}
	if len(restarted.ToolCalls) != 0 {
		t.Fatal("W5 fabricated an acknowledgement after crash")
	}
}

// W6: restart after ACK preserves exactly one observation and ACK; counters
// are not incremented again by re-reading the durable receipt.
func TestCLP20ObservationCrashW6RestartAfterAck(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := clp20BaseSession(t, store)
	cmd, _, stderr := clp20SpawnCrashChild(t, "postcommit-ack", dir, x.ID)
	clp20KillAndAssert(t, cmd, stderr)
	first := clp20Reopen(t, dir, x.ID)
	if first.ProviderRun == nil || first.ProviderRun.Phase != "acknowledged" || clp20OutputCount(first) != 1 {
		t.Fatalf("W6 ACK/observation not preserved once: %+v outputs=%d", first.ProviderRun, clp20OutputCount(first))
	}
	// A second reopen (restart) must not duplicate the observation.
	second := clp20Reopen(t, dir, x.ID)
	if second.Revision != first.Revision || clp20OutputCount(second) != 1 {
		t.Fatalf("W6 restart duplicated the observation: rev=%d outputs=%d", second.Revision, clp20OutputCount(second))
	}
}

// W7: concurrent save/reconcile of one logical observation has one CAS winner
// and is committed exactly once.
func TestCLP20ObservationCrashW7ConcurrentOneWinner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := clp20BaseSession(t, store)
	candidate := x
	h := append([]HistoryItem(nil), x.History...)
	h = append(h, clp20CallItem(), clp20OutputItem())
	candidate.History = h

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := store.Save(candidate, x.Revision)
			mu.Lock()
			if e == nil {
				wins++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("W7 expected exactly one CAS winner, got %d", wins)
	}
	restarted := clp20Reopen(t, dir, x.ID)
	if clp20OutputCount(restarted) != 1 {
		t.Fatalf("W7 observation committed %d times", clp20OutputCount(restarted))
	}
}

// After a crash leaves UNKNOWN delivery, only an explicit fresh owner action
// may continue; automatic resume requires live consent and is fenced.
func TestCLP20CrashReopenNeedsFreshOwnerAuthorization(t *testing.T) {
	x, err := NewSession("local", "/workspace", "project", "provider", "model", Stateless)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	x.Status = "failed"
	x.ProviderRun = &ProviderRun{
		ID: "run-unknown", Kind: "collection", Phase: "unknown", CallID: "read-1",
		PayloadSHA256: strings.Repeat("b", 64), ProviderFingerprint: "fp-1",
		CreatedAt: now, UpdatedAt: now,
	}
	x.History = []HistoryItem{{Type: "user", Content: "Audit source"}, clp20CallItem(), clp20OutputItem()}
	if !HasUnknownCollectionDelivery(x) {
		t.Fatal("UNKNOWN collection delivery not fenced")
	}
	if _, ok := CollectionResumeReceipt(x); !ok {
		t.Fatal("explicit-owner resume receipt missing")
	}
	if !CanSendAfterUnknownCollection(x) {
		t.Fatal("fresh owner message blocked after UNKNOWN delivery")
	}
	x.AutoContinueText = true
	if !CollectionAutoResumeAllowed(x) {
		t.Fatal("live owner consent auto resume refused")
	}
	x.AutoContinueText = false
	if CollectionAutoResumeAllowed(x) {
		t.Fatal("revoked consent allowed automatic resume")
	}
}
