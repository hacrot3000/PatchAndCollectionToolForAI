package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
)

type fakeAgentPatchProtocol struct {
	session.Service
	state      session.ProtocolState
	expectedID string
}

// List exposes the same synthetic session as Metadata. Queue retirement now
// inspects this Service method; the embedded nil Service must not be invoked.
func (f *fakeAgentPatchProtocol) List() []session.Metadata {
	if f.expectedID == "" {
		return nil
	}
	meta, _ := f.Metadata(f.expectedID)
	return []session.Metadata{meta}
}

func (f *fakeAgentPatchProtocol) Metadata(id string) (session.Metadata, bool) {
	if id != f.expectedID {
		return session.Metadata{}, false
	}
	return session.Metadata{ID: id, Status: "running"}, true
}

func (f *fakeAgentPatchProtocol) ProtocolState(id string) (session.ProtocolState, error) {
	if id != f.expectedID {
		return session.ProtocolState{}, errors.New("unknown patch session")
	}
	return f.state, nil
}
func TestAgentChatSyncFinalNativePatchEvidence(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].Status = "running"
	x.ToolCalls[0].QueuePath = "patchs/agent_fixture.zip"
	x.ToolCalls[0].PatchSessionID = "patch-session-1"
	x.ToolCalls[0].PromptID = "prompt-1"
	x.ToolCalls[0].SelectedIndex = 1
	queueDir, err := s.patchUploadDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(queueDir, "agent_fixture.zip")
	packageBytes := []byte("approved test package")
	if err := os.WriteFile(packagePath, packageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(packageBytes)
	x.ToolCalls[0].PackageSHA256 = hex.EncodeToString(digest[:])
	x.ToolCalls[0].PackageSize = int64(len(packageBytes))
	x, err = store.Save(x, x.Revision)
	if err != nil {
		t.Fatal(err)
	}
	rc := 0
	s.Sessions = &fakeAgentPatchProtocol{
		expectedID: "patch-session-1",
		state: session.ProtocolState{Available: true, Items: []session.ProtocolItemState{{
			Index: 1, Name: "agent_fixture.zip", Kind: "PATCH", Status: "PASS", RC: &rc, RunID: "run-1", OutputTail: "All tests passed",
		}}},
	}
	result := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/sync", map[string]any{})
	if result.Code != 200 {
		t.Fatalf("sync failed: %d %s", result.Code, result.Body)
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.ToolCalls[0].Status != "done" || !strings.Contains(y.ToolCalls[0].Result, `"PASS"`) {
		t.Fatalf("verified native result was not saved: %+v", y.ToolCalls[0])
	}
	oldRevision := y.Revision
	again := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/sync", map[string]any{})
	if again.Code != 200 {
		t.Fatalf("second poll failed: %d", again.Code)
	}
	z, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if z.Revision != oldRevision {
		t.Fatal("no-op sync unexpectedly mutated journal")
	}
}
func TestAgentChatSyncDoesNotInventSuccess(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].Status = "running"
	x.ToolCalls[0].QueuePath = "patchs/agent_fixture.zip"
	x.ToolCalls[0].PatchSessionID = "patch-session-2"
	x.ToolCalls[0].PromptID = "prompt-2"
	x.ToolCalls[0].SelectedIndex = 1
	x, err = store.Save(x, x.Revision)
	if err != nil {
		t.Fatal(err)
	}
	s.Sessions = &fakeAgentPatchProtocol{
		expectedID: "patch-session-2",
		state: session.ProtocolState{Available: true, Items: []session.ProtocolItemState{{
			Index: 1, Name: "other.zip", Kind: "PATCH", Status: "PASS", RunID: "run-other",
		}}},
	}
	result := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/sync", map[string]any{})
	if result.Code != 200 {
		t.Fatalf("sync failed: %d %s", result.Code, result.Body)
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.ToolCalls[0].Status != "running" || y.Revision != x.Revision {
		t.Fatal("unrelated native PASS was attributed to agent")
	}
}
func TestAgentChatSyncSharedPermission(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/agent/sessions/id/sync", nil)
	perms := sharedRoutePermissions(request)
	if len(perms) != 2 || perms[0] != identity.PermissionSettingsRead || perms[1] != identity.PermissionPatchView {
		t.Fatalf("native sync missing patch.view permission: %v", perms)
	}
}

func TestAgentStartupReconciliationNeverReexecutes(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].Status = "running"
	x.ToolCalls[0].QueuePath = "patchs/agent_fixture.zip"
	x.ToolCalls[0].PatchSessionID = "patch-session-startup"
	x.ToolCalls[0].PromptID = "prompt-startup"
	x.ToolCalls[0].SelectedIndex = 1
	queueDir, err := s.patchUploadDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatal(err)
	}
	packageBytes := []byte("startup package")
	if err := os.WriteFile(filepath.Join(queueDir, "agent_fixture.zip"), packageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(packageBytes)
	x.ToolCalls[0].PackageSHA256 = hex.EncodeToString(digest[:])
	x.ToolCalls[0].PackageSize = int64(len(packageBytes))
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	s.Sessions = &fakeAgentPatchProtocol{
		expectedID: "patch-session-startup",
		state: session.ProtocolState{Available: true, Items: []session.ProtocolItemState{{
			Index: 1, Name: "agent_fixture.zip", Kind: "PATCH", Status: "PASS", RunID: "run-startup",
		}}},
	}
	s.agentReconcilePendingOnStartup(context.Background())
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.ToolCalls[0].Status != "done" || y.ToolCalls[0].RunID != "run-startup" {
		t.Fatalf("startup did not adopt unambiguous final evidence: %+v", y.ToolCalls[0])
	}
	if y.ToolCalls[0].QueuePath == "" || y.ToolCalls[0].PatchSessionID == "patch-session-replaced" {
		t.Fatal("startup reconciliation queued/replaced a native session")
	}
}

func TestAgentStartupReconciliationLeavesAmbiguousUnknown(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].Status = "running"
	x.ToolCalls[0].QueuePath = "patchs/agent_fixture.zip"
	x.ToolCalls[0].PatchSessionID = "patch-session-ambiguous"
	x.ToolCalls[0].PromptID = "prompt-x"
	x.ToolCalls[0].SelectedIndex = 1
	x.ToolCalls[0].RunID = "run-original"
	queueDir, err := s.patchUploadDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatal(err)
	}
	packageBytes := []byte("ambiguous package")
	if err := os.WriteFile(filepath.Join(queueDir, "agent_fixture.zip"), packageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(packageBytes)
	x.ToolCalls[0].PackageSHA256 = hex.EncodeToString(digest[:])
	x.ToolCalls[0].PackageSize = int64(len(packageBytes))
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	s.Sessions = &fakeAgentPatchProtocol{
		expectedID: "patch-session-ambiguous",
		state: session.ProtocolState{Available: true, Items: []session.ProtocolItemState{{
			Index: 1, Name: "agent_fixture.zip", Kind: "PATCH", Status: "PASS", RunID: "run-different",
		}}},
	}
	s.agentReconcilePendingOnStartup(context.Background())
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.ToolCalls[0].Status != "running" || y.ToolCalls[0].RunID != "run-original" {
		t.Fatalf("mismatched run was adopted: %+v", y.ToolCalls[0])
	}
}

func TestAgentExecutionViewIsOwnerScopedAndPathFree(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].Status = "running"
	x.ToolCalls[0].QueuePath = "patchs/agent_fixture.zip"
	x.ToolCalls[0].PatchSessionID = "patch-session-view"
	x.ToolCalls[0].PromptID = "p1"
	x.ToolCalls[0].SelectedIndex = 1
	x.ToolCalls[0].RunID = "run-view"
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	rc := 0
	s.Sessions = &fakeAgentPatchProtocol{
		expectedID: "patch-session-view",
		state: session.ProtocolState{
			Available: true,
			Items: []session.ProtocolItemState{{Index: 1, Name: "agent_fixture.zip", Kind: "PATCH", Status: "PASS", RunID: "run-view", RC: &rc,
				OutputTail: "secret-token=/private/path should never leak as a path",
				Commands:   []session.ProtocolCommandEvidenceState{{Index: 1, Stage: "apply", Status: "PASS", RC: &rc, Argv: []string{"--token=abc"}, LogPath: "/root/.secret/log"}}}},
			Artifacts: []session.ProtocolArtifactState{{RunID: "run-view", ArtifactKind: "log", Path: "/root/.secret/log", ItemName: "agent_fixture.zip", Primary: true}},
		},
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+id+"/tools/call_agent_001/execution", nil)
	if w.Code != 200 {
		t.Fatalf("execution view failed: %d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, forbidden := range []string{"/root/.secret/log", "--token=abc"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("execution view leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"native_status":"PASS"`) || !strings.Contains(body, `"kind":"log"`) {
		t.Fatalf("execution view missing evidence: %s", body)
	}
	missing := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+id+"/tools/nope/execution", nil)
	if missing.Code != 404 {
		t.Fatalf("unknown call: %d", missing.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/agent/sessions/s/tools/c/execution", nil)
	perms := sharedRoutePermissions(req)
	if len(perms) != 2 || perms[1] != identity.PermissionPatchView {
		t.Fatalf("execution permissions: %+v", perms)
	}
}

func TestAgentEventsJournalResyncsAfterCursor(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		return agentchat.ResponsesResult{ResponseID: "resp_ev", Text: "ok"}, nil
	}
	if w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "CANARY_PRIVATE_PROMPT_NOT_FOR_EVENTS_4917", "confirm_external": true, "expected_revision": 1}); w.Code != 200 {
		t.Fatalf("turn failed: %d %s", w.Code, w.Body)
	}
	all := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+id+"/events", nil)
	if all.Code != 200 || !strings.Contains(all.Body.String(), `"kind":"turn_started"`) || !strings.Contains(all.Body.String(), `"kind":"turn_finished"`) {
		t.Fatalf("journal missing transitions: %d %s", all.Code, all.Body)
	}
	var view struct {
		Events []agentchat.SessionEvent `json:"events"`
		Cursor uint64                   `json:"cursor"`
	}
	if err := json.Unmarshal(all.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Events) == 0 || view.Cursor == 0 {
		t.Fatalf("empty journal: %s", all.Body)
	}
	// A cursor at the current tail returns nothing; nothing is replayed.
	tail := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet,
		fmt.Sprintf("/api/agent/sessions/%s/events?since=%d", id, view.Cursor), nil)
	if tail.Code != 200 || !strings.Contains(tail.Body.String(), `"events":[]`) {
		t.Fatalf("cursor not honoured: %d %s", tail.Code, tail.Body)
	}
	// The journal must never carry prompt text.
	if strings.Contains(all.Body.String(), "CANARY_PRIVATE_PROMPT_NOT_FOR_EVENTS_4917") {
		t.Fatalf("journal leaked prompt text: %s", all.Body)
	}
	bad := httptest.NewRequest(http.MethodGet, "/api/agent/sessions/"+id+"/events?since=abc", nil)
	r2 := httptest.NewRecorder()
	s.agentChatSessionItemAPI(r2, bad)
	if r2.Code != 400 {
		t.Fatalf("invalid cursor accepted: %d", r2.Code)
	}
}

func TestAgentStartupMarksInterruptedProviderTurnRecoverableWithoutReplayingPatch(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.Status="requesting_ai"
 x.ToolCalls[0].Status="done"
 x.ToolCalls[0].Result=`{"status":"PASS","rc":0}`
 x.ToolCalls[0].FeedbackSent=true
 x.ToolCalls[0].FeedbackStatus="sending"
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 s.agentReconcilePendingOnStartup(context.Background())
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if got.Status!="failed"||got.ToolCalls[0].FeedbackStatus!="unknown"{
  t.Fatalf("provider egress was retried or not marked ambiguous: status=%s feedback=%s",
   got.Status,got.ToolCalls[0].FeedbackStatus)
 }
 if got.ToolCalls[0].Status!="done"||got.ToolCalls[0].Result==""{
  t.Fatal("native Patch receipt was lost/reexecuted")
 }
 var interrupted bool
 for _,e:=range got.Events{if e.Kind=="turn_interrupted"{interrupted=true;break}}
 if !interrupted{t.Fatal("restart interruption missing durable event")}
}
