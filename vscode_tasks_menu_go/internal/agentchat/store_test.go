package agentchat

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSessionStoreOwnerIsolationRevisionAndRecovery(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "custom", "model-a", PreviousResponse)
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.Save(x, 0)
	if err != nil {
		t.Fatal(err)
	}
	if x.Revision != 1 {
		t.Fatalf("first version %d", x.Revision)
	}
	if _, err = s.Get(x.ID, "user:bob"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner leak: %v", err)
	}
	x.History = append(x.History, HistoryItem{Type: "user", Content: "first message"})
	x.PreviousResponseID = "resp-001"
	x.Status = "ready"
	x, err = s.Save(x, 1)
	if err != nil {
		t.Fatal(err)
	}
	if x.Revision != 2 {
		t.Fatalf("revision %d", x.Revision)
	}
	if _, err = s.Save(x, 1); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("stale write accepted: %v", err)
	}
	// Open a fresh store to simulate server restart.
	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(x.ID, "user:alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviousResponseID != "resp-001" || len(got.History) != 1 || got.Revision != 2 {
		t.Fatalf("lost canonical context: %+v", got)
	}
	list, err := reopened.List("user:alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("missing session in owner list: %v / %d", err, len(list))
	}
	empty, err := reopened.List("user:bob")
	if err != nil || len(empty) != 0 {
		t.Fatalf("cross-owner session exposure: %v / %d", err, len(empty))
	}
	info, err := os.Stat(filepath.Join(dir, x.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("insecure session mode: %o", info.Mode().Perm())
	}
}

func TestSessionStoreInvalidIDAndDirectory(t *testing.T) {
	s, err := NewStore("relative")
	if err == nil || s != nil {
		t.Fatal("relative state path allowed")
	}
	root := t.TempDir()
	s, err = NewStore(filepath.Join(root, "agent"))
	if err != nil {
		t.Fatal(err)
	}
	x, err := NewSession("owner", "/project", "uuid", "provider", "model", Stateless)
	if err != nil {
		t.Fatal(err)
	}
	x.ID = "../file"
	if _, err := s.Save(x, 0); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("path traversal not rejected: %v", err)
	}
	if _, err := s.Get("../file", "owner"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("path traversal not rejected: %v", err)
	}
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List("owner"); err == nil {
		t.Fatal("world-readable state directory allowed")
	}
}

func TestAgentSessionConcurrentWritesHaveOneWinner(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	x, err := NewSession("owner", "/workspace", "project-id", "provider", "model", Stateless)
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.Save(x, 0)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := x
			local.Status = "ready"
			_, err := s.Save(local, x.Revision)
			if err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if !errors.Is(err, ErrStaleRevision) {
				t.Errorf("unexpected write failure: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("expected exactly one CAS winner, got %d", ok)
	}
}

func TestNewSessionRejectsInvalidOrUnscopedIdentity(t *testing.T) {
	if _, err := NewSession("", "/w", "id", "provider", "model", Stateless); err == nil {
		t.Fatal("missing owner")
	}
	if _, err := NewSession("owner", "/w", "", "provider", "model", Stateless); err == nil {
		t.Fatal("missing project identity")
	}
	if _, err := NewSession("owner", "/w", "id", "provider", "model", "unknown"); err == nil {
		t.Fatal("unknown context mode")
	}
}


func TestSessionLifecycleEventsPreserveChronologicalHistoryPosition(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "agent"))
	if err != nil { t.Fatal(err) }
	x, err := NewSession("local", "/workspace/test", "project-uuid", "provider", "model", Stateless)
	if err != nil { t.Fatal(err) }
	x.History = append(x.History, HistoryItem{Type:"user",Content:"Create helloworld.php"})
	x.Status = "requesting_ai"
	x.AppendEvent("turn_started", "requesting_ai", "", "")
	x.History = append(x.History,
		HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"collect-1",Arguments:"{}"},
		HistoryItem{Type:"function_call_output",CallID:"collect-1",Content:"{}"})
	x.AppendEvent("collect_observed", "requesting_ai", "collect-1", "")
	x.Status = "failed"
	x.AppendEvent("collect_failed", "failed", "", "collection_continuation")
	x, err = s.Save(x, 0)
	if err != nil { t.Fatal(err) }
	reloaded,err:=s.Get(x.ID,"local")
	if err!=nil{t.Fatal(err)}
	if len(reloaded.Events)!=3 {t.Fatalf("lost session phase transitions: %+v",reloaded.Events)}
	if reloaded.Events[0].HistoryIndex!=1 ||
		reloaded.Events[1].HistoryIndex!=3 ||
		reloaded.Events[2].HistoryIndex!=3 ||
		reloaded.Events[2].Detail!="collection_continuation"{
		t.Fatalf("history event anchors incorrect: %+v",reloaded.Events)
	}
}
