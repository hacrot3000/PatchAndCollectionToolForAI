package agentchat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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

// Ordinary journal saves must not silently change provider/model selection.
// A future owner-scoped admission path must not weaken this generic CAS guard.
func TestStoreRejectsUnscopedProviderSelectionChanges(t *testing.T) {
 store, err := NewStore(filepath.Join(t.TempDir(), "private", "agent"))
 if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless)
 if err != nil { t.Fatal(err) }
 x, err = store.Save(x, 0)
 if err != nil { t.Fatal(err) }
 for _, tc := range []struct{name, provider, model string}{
  {"provider", "other-provider", "model"}, {"model", "provider", "other-model"},
 } {
  t.Run(tc.name, func(t *testing.T) {
   changed := x
   changed.ProviderID, changed.ModelID = tc.provider, tc.model
   if _, err := store.Save(changed, x.Revision); !errors.Is(err, ErrInvalidSession) {
    t.Fatalf("unscoped selection change accepted: %v", err)
   }
   got, err := store.Get(x.ID, "user:alice")
   if err != nil { t.Fatal(err) }
   if got.Revision != x.Revision || got.ProviderID != x.ProviderID || got.ModelID != x.ModelID {
    t.Fatal("rejected selection change modified the journal")
   }
  })
 }
}

func TestPendingTurnConcurrentJournalCAS(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:time.Now().UTC()}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 expected := *x.PendingUserTurn
 results := make(chan error, 2)
 var wg sync.WaitGroup
 for i := 0; i < 2; i++ {
  wg.Add(1)
  go func() { defer wg.Done(); _, err := store.Save(x, x.Revision); results <- err }()
 }
 wg.Wait(); close(results)
 successes, stale := 0, 0
 for err := range results { if err == nil { successes++ } else if errors.Is(err, ErrStaleRevision) { stale++ } else { t.Fatalf("unexpected save error: %v", err) } }
 if successes != 1 || stale != 1 { t.Fatalf("successes=%d stale=%d", successes, stale) }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != x.Revision+1 || got.PendingUserTurn == nil || *got.PendingUserTurn != expected { t.Fatal("concurrent saves altered pending turn") }
}

func TestPendingTurnSurvivesUnrelatedJournalSave(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:time.Now().UTC()}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 expected := *x.PendingUserTurn
 x.AppendEvent("test_checkpoint", x.Status, "", "")
 saved, err := store.Save(x, x.Revision); if err != nil { t.Fatal(err) }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != saved.Revision || got.PendingUserTurn == nil || *got.PendingUserTurn != expected {
  t.Fatal("unrelated journal save lost or changed pending turn")
 }
}

func TestPendingAdmissionBindingMatches(t *testing.T) {
 now := time.Now().UTC()
 digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 p := PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:digest, ProviderID:"provider", ModelID:"model", ReasoningEffort:"low", ProviderConfigVersion:"v1", QueuedAt:now}
 r := ProviderRun{ID:"run", Kind:"message", Phase:"prepared", PayloadSHA256:digest, CreatedAt:now, UpdatedAt:now}
 b := TurnBinding{Version:TurnBindingVersion, TurnID:"run", AttemptID:"run", RequestID:"req", ProviderID:"provider", ModelID:"model", ReasoningEffort:"low", ProviderConfigVersion:"v1", ContextMode:Stateless, PayloadDigest:digest, AdmittedAt:now}
 if !pendingAdmissionBindingMatches(&p,&b,&r) { t.Fatal("valid binding rejected") }
 cases := []struct{name string; mutate func(*PendingUserTurn,*TurnBinding,*ProviderRun)}{
  {"request",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.RequestID="other"}},
  {"provider",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.ProviderID="other"}},
  {"model",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.ModelID="other"}},
  {"effort",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.ReasoningEffort="high"}},
  {"config",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.ProviderConfigVersion="v2"}},
  {"credential",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.CredentialFingerprint=digest}},
  {"run",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.TurnID="other"}},
  {"attempt",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.AttemptID="other"}},
  {"payload",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.PayloadDigest="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
  {"phase",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){r.Phase="sending"}},
  {"kind",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){r.Kind="collection"}},
  {"invalid pending digest",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){p.RequestDigest="not-a-sha256"}},
  {"invalid binding mode",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){b.ContextMode="invalid"}},
  {"invalid run timestamps",func(p *PendingUserTurn,b *TurnBinding,r *ProviderRun){r.CreatedAt=time.Time{}}},
 }
 for _, tc := range cases { t.Run(tc.name,func(t *testing.T){ pp,bb,rr:=p,b,r; tc.mutate(&pp,&bb,&rr); if pendingAdmissionBindingMatches(&pp,&bb,&rr){t.Fatal("mismatch accepted")} }) }
 if pendingAdmissionBindingMatches(nil,&b,&r)||pendingAdmissionBindingMatches(&p,nil,&r)||pendingAdmissionBindingMatches(&p,&b,nil){t.Fatal("nil binding accepted")}
}

// Simulate restart at the durable-sidecar, pre-journal-commit boundary.
// This is a state-boundary test, not a process-kill or power-loss test.
func TestPendingAdmissionStagedSidecarBeforeCommit(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 now := time.Now().UTC()
 payload := []byte("prepared payload")
 digest := ProviderPayloadDigest(payload)
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:digest, ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:now}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 expected := *x.PendingUserTurn
 candidate := x
 candidate.PendingUserTurn = nil
 candidate.Status = "requesting_ai"
 candidate.ProviderRun = &ProviderRun{ID:"run", Kind:"message", Phase:"prepared", PayloadSHA256:digest, CreatedAt:now, UpdatedAt:now}
 candidate.TurnBinding = &TurnBinding{Version:TurnBindingVersion, TurnID:"run", AttemptID:"run", RequestID:"req", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", ContextMode:Stateless, PayloadDigest:digest, AdmittedAt:now}
 if err := store.StageProviderPayload(candidate, payload); err != nil { t.Fatal(err) }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != x.Revision || got.PendingUserTurn == nil || *got.PendingUserTurn != expected || got.ProviderRun != nil { t.Fatal("staged sidecar changed journal") }
 if _, _, err := reopened.PreparedProviderPayload(x.ID, "user:alice"); err == nil { t.Fatal("uncommitted sidecar was replayable") }
 if _, err := reopened.admitPendingUserTurn(candidate, "user:alice", x.Revision, payload); err != nil { t.Fatal(err) }
 _, recovered, err := reopened.PreparedProviderPayload(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if string(recovered) != string(payload) { t.Fatal("committed payload differs") }
}

func TestPendingAdmissionStagingFailurePreservesTurn(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 now := time.Now().UTC()
 payload := []byte("prepared payload")
 digest := ProviderPayloadDigest(payload)
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:digest, ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:now}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 expected := *x.PendingUserTurn
 candidate := x
 candidate.PendingUserTurn = nil
 candidate.Status = "requesting_ai"
 candidate.ProviderRun = &ProviderRun{ID:"run", Kind:"message", Phase:"prepared", PayloadSHA256:digest, CreatedAt:now, UpdatedAt:now}
 candidate.TurnBinding = &TurnBinding{Version:TurnBindingVersion, TurnID:"run", AttemptID:"run", RequestID:"req", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", ContextMode:Stateless, PayloadDigest:digest, AdmittedAt:now}
 // An existing directory makes the sidecar rename fail without changing the journal.
 if err := os.Mkdir(store.providerRunFile(x.ID), 0700); err != nil { t.Fatal(err) }
 if _, err := store.admitPendingUserTurn(candidate, "user:alice", x.Revision, payload); err == nil { t.Fatal("admission unexpectedly succeeded") }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != x.Revision || got.PendingUserTurn == nil || *got.PendingUserTurn != expected || got.ProviderRun != nil { t.Fatal("failed staging altered persisted admission") }
}

func TestPendingAdmissionRejectsOtherOwner(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 now := time.Now().UTC()
 payload := []byte("prepared payload")
 digest := ProviderPayloadDigest(payload)
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:digest, ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:now}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 expected := *x.PendingUserTurn
 candidate := x
 candidate.PendingUserTurn = nil
 candidate.Status = "requesting_ai"
 candidate.ProviderRun = &ProviderRun{ID:"run", Kind:"message", Phase:"prepared", PayloadSHA256:digest, CreatedAt:now, UpdatedAt:now}
 candidate.TurnBinding = &TurnBinding{Version:TurnBindingVersion, TurnID:"run", AttemptID:"run", RequestID:"req", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", ContextMode:Stateless, PayloadDigest:digest, AdmittedAt:now}
 if _, err := store.AdmitPendingUserTurn(candidate, "user:bob", x.Revision, payload); !errors.Is(err, ErrInvalidSession) { t.Fatalf("cross-owner admission: %v", err) }
 if _, err := os.Lstat(store.providerRunFile(x.ID)); !errors.Is(err, os.ErrNotExist) { t.Fatalf("cross-owner admission staged sidecar: %v", err) }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != x.Revision || got.PendingUserTurn == nil || *got.PendingUserTurn != expected || got.ProviderRun != nil { t.Fatal("failed staging altered persisted admission") }
}

func TestPendingAdmissionConcurrentCommit(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 now := time.Now().UTC()
 payload := []byte("prepared payload")
 digest := ProviderPayloadDigest(payload)
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:digest, ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:now}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 candidate := x
 candidate.PendingUserTurn = nil
 candidate.Status = "requesting_ai"
 candidate.ProviderRun = &ProviderRun{ID:"run", Kind:"message", Phase:"prepared", PayloadSHA256:digest, CreatedAt:now, UpdatedAt:now}
 candidate.TurnBinding = &TurnBinding{Version:TurnBindingVersion, TurnID:"run", AttemptID:"run", RequestID:"req", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", ContextMode:Stateless, PayloadDigest:digest, AdmittedAt:now}
 results := make(chan error, 2)
 var wg sync.WaitGroup
 for i := 0; i < 2; i++ { wg.Add(1); go func() { defer wg.Done(); _, err := store.admitPendingUserTurn(candidate, "user:alice", x.Revision, payload); results <- err }() }
 wg.Wait(); close(results)
 successes, stale := 0, 0
 for err := range results { if err == nil { successes++ } else if errors.Is(err, ErrStaleRevision) { stale++ } else { t.Fatalf("unexpected admission error: %v", err) } }
 if successes != 1 || stale != 1 { t.Fatalf("successes=%d stale=%d", successes, stale) }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != x.Revision+1 || got.PendingUserTurn != nil || got.ProviderRun == nil || got.ProviderRun.Phase != "prepared" || got.ProviderRun.PayloadSHA256 != digest { t.Fatal("committed admission not retained after reopen") }
 _, recovered, err := reopened.PreparedProviderPayload(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if string(recovered) != string(payload) { t.Fatal("concurrent admission persisted wrong payload") }
}

func TestPendingAdmissionRejectsEmptyOwner(t *testing.T) {
 var store *Store
 _, err := store.admitPendingUserTurn(Session{}, "", 0, nil)
 if !errors.Is(err, ErrInvalidSession) {
  t.Fatalf("empty owner admission: got %v, want ErrInvalidSession", err)
 }
}

func TestPreservesPendingUserTurnInvariant(t *testing.T) {
 original := &PendingUserTurn{Message: "queued"}
 same := *original
 changed := same
 changed.Message = "different"
 if !preservesPendingUserTurn(nil, nil) || !preservesPendingUserTurn(nil, original) || !preservesPendingUserTurn(original, &same) {
  t.Fatal("valid pending preservation rejected")
 }
 if preservesPendingUserTurn(original, nil) || preservesPendingUserTurn(original, &changed) {
  t.Fatal("pending removal or replacement accepted")
 }
}

func TestPendingAdmissionCandidateMatches(t *testing.T) {
 now := time.Now().UTC()
 payload := []byte("prepared payload")
 digest := ProviderPayloadDigest(payload)
 current, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless)
 if err != nil { t.Fatal(err) }
 current.Revision = 4
 current.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:digest, ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:now}
 candidate := current
 candidate.PendingUserTurn = nil
 candidate.Status = "requesting_ai"
 candidate.ProviderRun = &ProviderRun{ID:"run", Kind:"message", Phase:"prepared", PayloadSHA256:digest, CreatedAt:now, UpdatedAt:now}
 candidate.TurnBinding = &TurnBinding{Version:TurnBindingVersion, TurnID:"run", AttemptID:"run", RequestID:"req", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", ContextMode:Stateless, PayloadDigest:digest, AdmittedAt:now}
 if !pendingAdmissionCandidateMatches(current, candidate, "user:alice", 4, payload) { t.Fatal("valid candidate rejected") }
 completed := *candidate.ProviderRun
 completed.Phase = "completed"
 prior := current
 prior.ProviderRun = &completed
 if !pendingAdmissionCandidateMatches(prior, candidate, "user:alice", 4, payload) { t.Fatal("completed prior run rejected") }
 cases := []struct{name string; mutate func(*Session,*Session,*string,*uint64,*[]byte)}{
  {"owner", func(c,x *Session,o *string,r *uint64,p *[]byte){*o="user:bob"}},
  {"candidate owner", func(c,x *Session,o *string,r *uint64,p *[]byte){x.Owner="user:bob"}},
  {"revision", func(c,x *Session,o *string,r *uint64,p *[]byte){*r=5}},
  {"session ID", func(c,x *Session,o *string,r *uint64,p *[]byte){x.ID="other"}},
  {"candidate revision", func(c,x *Session,o *string,r *uint64,p *[]byte){x.Revision++}},
  {"workspace", func(c,x *Session,o *string,r *uint64,p *[]byte){x.Workspace="other"}},
  {"project", func(c,x *Session,o *string,r *uint64,p *[]byte){x.ProjectID="other"}},
  {"pending retained", func(c,x *Session,o *string,r *uint64,p *[]byte){x.PendingUserTurn=c.PendingUserTurn}},
  {"no pending", func(c,x *Session,o *string,r *uint64,p *[]byte){c.PendingUserTurn=nil}},
  {"prepared already", func(c,x *Session,o *string,r *uint64,p *[]byte){c.ProviderRun=x.ProviderRun}},
  {"sending unresolved", func(c,x *Session,o *string,r *uint64,p *[]byte){run:=*x.ProviderRun; run.Phase="sending"; c.ProviderRun=&run}},
  {"unknown unresolved", func(c,x *Session,o *string,r *uint64,p *[]byte){run:=*x.ProviderRun; run.Phase="unknown"; c.ProviderRun=&run}},
  {"acknowledged unresolved", func(c,x *Session,o *string,r *uint64,p *[]byte){run:=*x.ProviderRun; run.Phase="acknowledged"; c.ProviderRun=&run}},
  {"blocked unresolved", func(c,x *Session,o *string,r *uint64,p *[]byte){run:=*x.ProviderRun; run.Phase="blocked"; c.ProviderRun=&run}},
  {"missing run", func(c,x *Session,o *string,r *uint64,p *[]byte){x.ProviderRun=nil}},
  {"missing binding", func(c,x *Session,o *string,r *uint64,p *[]byte){x.TurnBinding=nil}},
  {"binding context mode drift", func(c,x *Session,o *string,r *uint64,p *[]byte){binding:=*x.TurnBinding; binding.ContextMode=Conversation; x.TurnBinding=&binding}},
  {"candidate context mode drift", func(c,x *Session,o *string,r *uint64,p *[]byte){x.Mode=Conversation}},
  {"status", func(c,x *Session,o *string,r *uint64,p *[]byte){x.Status="idle"}},
  {"payload", func(c,x *Session,o *string,r *uint64,p *[]byte){*p=[]byte("different")}},
 }
 for _, tc := range cases { t.Run(tc.name, func(t *testing.T) {
  c,x,o,r,p := current,candidate,"user:alice",uint64(4),payload
  tc.mutate(&c,&x,&o,&r,&p)
  if pendingAdmissionCandidateMatches(c,x,o,r,p) { t.Fatal("invalid candidate accepted") }
 }) }
}

func TestStoreRejectsPendingTurnReplacement(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:time.Now().UTC()}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 changed := x
 replacement := *x.PendingUserTurn
 replacement.Message = "different"
 changed.PendingUserTurn = &replacement
 if _, err = store.Save(changed, x.Revision); !errors.Is(err, ErrInvalidSession) { t.Fatalf("pending replacement accepted: %v", err) }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != x.Revision || got.PendingUserTurn == nil || got.PendingUserTurn.Message != "queued" || got.PendingUserTurn.RequestID != "req" { t.Fatal("rejected replacement altered durable pending turn") }
}

func TestStoreRejectsPendingTurnRemoval(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "private", "agent")
 store, err := NewStore(dir); if err != nil { t.Fatal(err) }
 x, err := NewSession("user:alice", "/workspace/a", "project-uuid", "provider", "model", Stateless); if err != nil { t.Fatal(err) }
 x.PendingUserTurn = &PendingUserTurn{Message:"queued", RequestID:"req", RequestDigest:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ProviderID:"provider", ModelID:"model", ProviderConfigVersion:"v1", QueuedAt:time.Now().UTC()}
 x, err = store.Save(x, 0); if err != nil { t.Fatal(err) }
 changed := x; changed.PendingUserTurn = nil
 if _, err = store.Save(changed, x.Revision); !errors.Is(err, ErrInvalidSession) { t.Fatalf("pending removal accepted: %v", err) }
 reopened, err := NewStore(dir); if err != nil { t.Fatal(err) }
 got, err := reopened.Get(x.ID, "user:alice"); if err != nil { t.Fatal(err) }
 if got.Revision != x.Revision || got.PendingUserTurn == nil || got.PendingUserTurn.Message != "queued" || got.PendingUserTurn.RequestID != "req" { t.Fatal("rejected removal altered durable pending turn") }
}

func TestStoreListIgnoresThousandsOfCollectionArchiveSegments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private", "agent")
	store, err := NewStore(dir)
	if err != nil { t.Fatal(err) }
	x, err := NewSession("local", "/workspace/project", "project-uuid", "provider", "model", Stateless)
	if err != nil { t.Fatal(err) }
	x, err = store.Save(x, 0)
	if err != nil { t.Fatal(err) }
	// Thousands of archive segments can accumulate over multiple days.
	// They are not separate conversations and must not disable the
	// auto-Patch/auto-feedback scheduler, which relies on Store.List.
	for i := 1; i <= 1050; i++ {
		name := filepath.Join(dir, fmt.Sprintf("%s.history.%d.json", x.ID, i))
		if err := os.WriteFile(name, []byte("{}"), 0600); err != nil { t.Fatal(err) }
	}
	sessions, err := store.List("local")
	if err != nil { t.Fatalf("Collection archives prevented session enumeration: %v", err) }
	if len(sessions) != 1 || sessions[0].ID != x.ID {
		t.Fatalf("unexpected active sessions with archives: %+v", sessions)
	}
}
