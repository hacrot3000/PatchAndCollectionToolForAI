package agentrun

import (
	"os"
 "bytes"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestContextSnapshotPinsPromptPolicyWithoutSecrets(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	store, err := NewSnapshotStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	autoCollection := false
	feedbackUntil := time.Now().UTC().Add(time.Hour)
	policy := &ContextSnapshotPolicy{
		SystemInstructions: "Pinned safe system instructions",
		RetryMode:          agentchat.RetryLimited,
		ReasoningEffort:    "high",
		Automation: AutomationSnapshot{AutoPatch: true, AutoCollection: &autoCollection, AutoFeedback: true,
			AutoFeedbackPersistent: true, AutoFeedbackUntil: &feedbackUntil},
	}
	snapshot, err := store.CreateWithPolicy("owner", "conversation", "project",
		[]agentchat.HistoryItem{{Type: "user", Content: "prompt"}}, nil, policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.SystemInstructions = "changed after send"
	*policy.Automation.AutoCollection = true
	loaded, err := store.Get(snapshot.ID, "owner", "conversation", "project")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Policy == nil || loaded.Policy.SystemInstructions != "Pinned safe system instructions" ||
		loaded.Policy.RetryMode != agentchat.RetryLimited || loaded.Policy.ReasoningEffort != "high" ||
		loaded.Policy.Automation.AutoCollection == nil || *loaded.Policy.Automation.AutoCollection {
		t.Fatalf("snapshot policy changed after send: %+v", loaded.Policy)
	}
}

func TestContextSnapshotPolicyIsIncludedInIntegrityDigest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	store, err := NewSnapshotStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.CreateWithPolicy("owner", "conversation", "project",
		[]agentchat.HistoryItem{{Type: "user", Content: "prompt"}}, nil,
		&ContextSnapshotPolicy{SystemInstructions: "pinned instructions", RetryMode: agentchat.RetryOff})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, snapshot.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	modified := []byte(string(body))
	for i := 0; i+len("pinned instructions") <= len(modified); i++ {
		if string(modified[i:i+len("pinned instructions")]) == "pinned instructions" {
			copy(modified[i:], "changed instructions!!")
			break
		}
	}
	if err := os.WriteFile(path, modified, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(snapshot.ID, "owner", "conversation", "project"); err == nil {
		t.Fatal("modified prompt policy passed snapshot integrity validation")
	}
}

func TestCollectionGuardPolicySnapshotPinsThresholdsAndLegacyDigest(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"snapshots")
 store,err:=NewSnapshotStore(dir);if err!=nil{t.Fatal(err)}
 history:=[]agentchat.HistoryItem{{Type:"user",Content:"original goal"}}
 original:=NewCollectionGuardPolicySnapshot()
 original.Thresholds.DuplicateStreak=1
 policy:=&ContextSnapshotPolicy{CollectionGuardPolicy:original}
 snap,err:=store.CreateWithPolicy("owner","conversation","project",history,nil,policy);if err!=nil{t.Fatal(err)}
 original.Thresholds.DuplicateStreak=9
 if got,ok:=snap.Policy.CollectionPolicy();!ok||got.DuplicateStreak!=1{t.Fatal("snapshot aliases caller threshold pointer")}
 reopened,err:=NewSnapshotStore(dir);if err!=nil{t.Fatal(err)}
 loaded,err:=reopened.Get(snap.ID,"owner","conversation","project");if err!=nil{t.Fatal(err)}
 if got,ok:=loaded.Policy.CollectionPolicy();!ok||got.DuplicateStreak!=1{t.Fatal("policy changed after reopen")}
 next,err:=reopened.CreateWithPolicy("owner","conversation","project",history,nil,loaded.Policy);if err!=nil{t.Fatal(err)}
 if got,ok:=next.Policy.CollectionPolicy();!ok||got.DuplicateStreak!=1{t.Fatal("continuation changed pinned policy")}
 legacy,err:=store.CreateWithPolicy("owner","conversation","project",history,nil,&ContextSnapshotPolicy{});if err!=nil{t.Fatal(err)}
 legacy,err=reopened.Get(legacy.ID,"owner","conversation","project");if err!=nil{t.Fatal(err)}
 if legacy.Policy.CollectionGuardPolicy!=nil{t.Fatal("legacy snapshot was rewritten")}
 if got,ok:=legacy.Policy.CollectionPolicy();!ok||got.DuplicateStreak!=3||got.FirstReadAdvisory!=5||got.SecondReadAdvisory!=10{t.Fatal("legacy defaults changed")}
 for _,invalid:=range []*CollectionGuardPolicySnapshot{{Version:999,Thresholds:original.Thresholds},{Version:1}}{
  if _,ok:=(&ContextSnapshotPolicy{CollectionGuardPolicy:invalid}).CollectionPolicy();ok{t.Fatal("invalid explicit metadata enabled guard")}
 }
 path:=filepath.Join(dir,snap.ID+".json")
 raw,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 changed:=bytes.Replace(raw,[]byte(`"DuplicateStreak":1`),[]byte(`"DuplicateStreak":2`),1)
 if bytes.Equal(raw,changed){t.Fatal("missing pinned policy in payload")}
 if err=os.WriteFile(path,changed,0600);err!=nil{t.Fatal(err)}
 if _,err=reopened.Get(snap.ID,"owner","conversation","project");err==nil{t.Fatal("modified threshold passed digest")}
}
