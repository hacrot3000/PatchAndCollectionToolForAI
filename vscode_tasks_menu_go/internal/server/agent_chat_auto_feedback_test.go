package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
)

func autoFeedbackReadySession(t *testing.T) (*Server, string) {
	t.Helper()
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].Status = "done"
	x.ToolCalls[0].Result = `{"status":"PASS","run_id":"run-auto"}`
	until := time.Now().UTC().Add(10 * time.Minute)
	x.AutoFeedback = true
	x.AutoFeedbackUntil = &until
	x, err = store.Save(x, x.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return s, id
}

func TestAgentAutoFeedbackRequiresConsentAndDeliversOnce(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	noConsent := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/auto-feedback",
		map[string]any{"enabled": true, "expected_revision": x.Revision})
	if noConsent.Code != 403 {
		t.Fatalf("auto feedback enabled without consent: %d", noConsent.Code)
	}
	ok := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/auto-feedback",
		map[string]any{"enabled": true, "confirm_external": true, "expected_revision": x.Revision})
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), `"auto_feedback":true`) {
		t.Fatalf("auto feedback opt-in failed: %d %s", ok.Code, ok.Body)
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.AutoFeedbackUntil == nil || y.AutoFeedbackUntil.After(time.Now().UTC().Add(agentAutoFeedbackMaxAge+time.Minute)) {
		t.Fatalf("trust window not bounded: %+v", y.AutoFeedbackUntil)
	}
	// A pending (not final) patch must never be delivered by the scheduler.
	if _, ready := agentAutoFeedbackCandidate(y); ready {
		t.Fatal("pending patch treated as ready")
	}
	y.ToolCalls[0].Status = "done"
	y.ToolCalls[0].Result = `{"status":"PASS","run_id":"run-auto"}`
	idx, ready := agentAutoFeedbackCandidate(y)
	if !ready || idx != 0 {
		t.Fatalf("final result not eligible: %d %v", idx, ready)
	}
}

func TestAgentAutoFeedbackSchedulerDeliversVerifiedResultOnce(t *testing.T) {
	s, id := autoFeedbackReadySession(t)
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		if !strings.Contains(string(payload), `"call_id":"call_agent_001"`) {
			t.Fatal("wrong call_id delivered")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_auto", Text: "ack"}, nil
	}
	s.runAgentAutoFeedback(context.Background())
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !x.ToolCalls[0].FeedbackSent || x.ToolCalls[0].FeedbackStatus != "acknowledged" {
		t.Fatalf("auto delivery not applied once: calls=%d %+v", calls, x.ToolCalls[0])
	}
	// A later pass must not resend.
	s.runAgentAutoFeedback(context.Background())
	if calls != 1 {
		t.Fatalf("auto feedback resent the same result: %d", calls)
	}
}

func TestAgentAutoFeedbackStopsAfterExpiryAndTransportFailure(t *testing.T) {
	s, id := autoFeedbackReadySession(t)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	x.AutoFeedbackUntil = &past
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, nil
	}
	s.runAgentAutoFeedback(context.Background())
	if calls != 0 {
		t.Fatal("expired consent still delivered")
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(10 * time.Minute)
	y.AutoFeedbackUntil = &future
	y.ToolCalls[0].FeedbackSent = false
	y.ToolCalls[0].FeedbackStatus = ""
	y.Status = "waiting_tool"
	if y, err = store.Save(y, y.Revision); err != nil {
		t.Fatal(err)
	}
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, context.Canceled
	}
	s.runAgentAutoFeedback(context.Background())
	z, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if z.ToolCalls[0].FeedbackStatus != "unknown" {
		t.Fatalf("failed delivery not marked unknown: %q", z.ToolCalls[0].FeedbackStatus)
	}
	before := calls
	s.runAgentAutoFeedback(context.Background())
	if calls != before {
		t.Fatal("ambiguous delivery was retried automatically")
	}
}

func TestAgentAutoFeedbackSharedPermission(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions/id/auto-feedback", nil)
	perms := sharedRoutePermissions(req)
	if len(perms) != 3 || perms[2] != identity.PermissionFilesRead {
		t.Fatalf("auto-feedback permission mapping: %+v", perms)
	}
}

func TestAgentAutoFeedbackReconcilesNativeCompletionWithoutGUI(t *testing.T) {
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
	x.ToolCalls[0].QueuePath = "patchs/PATCH_agent_fixture.zip"
	queueDir, err := s.patchUploadDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatal(err)
	}
	packageBytes := []byte("approved background package")
	if err := os.WriteFile(filepath.Join(queueDir, "PATCH_agent_fixture.zip"), packageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(packageBytes)
	x.ToolCalls[0].PackageSHA256 = hex.EncodeToString(digest[:])
	x.ToolCalls[0].PackageSize = int64(len(packageBytes))
	x.ToolCalls[0].PatchSessionID = "native-auto"
	x.ToolCalls[0].PromptID = "prompt-auto"
	x.ToolCalls[0].SelectedIndex = 2
	x.Status = "waiting_tool"
	x.AutoFeedback = true
	until := time.Now().UTC().Add(10 * time.Minute)
	x.AutoFeedbackUntil = &until
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	rc := 0
	s.Sessions = &fakeAutoFeedbackNative{meta: session.Metadata{ID: "native-auto", Status: "running"}, fakeAgentPatchProtocol: &fakeAgentPatchProtocol{
		expectedID: "native-auto",
		state: session.ProtocolState{Available: true, Items: []session.ProtocolItemState{{
			Index: 2, Name: "PATCH_agent_fixture.zip", Kind: "PATCH", Status: "PASS", RC: &rc, RunID: "native-run-auto",
		}}},
	}}
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		if !strings.Contains(string(payload), `"call_id":"call_agent_001"`) {
			t.Fatal("feedback lost exact call ID")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_auto_background", Text: "Patch completed"}, nil
	}
	// No GUI /sync request occurs. The scheduler reads native ProtocolState,
	// persists the final run and sends one tool result.
	s.runAgentAutoFeedback(context.Background())
	after, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	call := after.ToolCalls[0]
	if call.Status != "done" || call.RunID != "native-run-auto" || !call.FeedbackSent || call.FeedbackStatus != "acknowledged" || calls != 1 {
		t.Fatalf("background completion/feedback failed: %+v calls=%d", call, calls)
	}
	if len(after.Events) == 0 {
		t.Fatal("background reconciliation did not append an event")
	}
	// A later pass cannot send a second result.
	s.runAgentAutoFeedback(context.Background())
	if calls != 1 {
		t.Fatalf("background feedback duplicated: %d", calls)
	}
}

func TestAgentAutoFeedbackMissingNativeSessionBecomesUnknown(t *testing.T) {
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
	x.ToolCalls[0].QueuePath = "patchs/PATCH_missing.zip"
	x.ToolCalls[0].PatchSessionID = "native-missing"
	x.ToolCalls[0].PromptID = "prompt-missing"
	x.ToolCalls[0].SelectedIndex = 1
	x.AutoFeedback = true
	until := time.Now().UTC().Add(10 * time.Minute)
	x.AutoFeedbackUntil = &until
	if _, err = store.Save(x, x.Revision); err != nil {
		t.Fatal(err)
	}
	s.Sessions = fakeMissingNativeSession{}
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		return agentchat.ResponsesResult{}, nil
	}
	s.runAgentAutoFeedback(context.Background())
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if y.ToolCalls[0].Status != "unknown" || calls != 0 {
		t.Fatalf("missing native run was retried/falsely reported: %+v calls=%d", y.ToolCalls[0], calls)
	}
}

// fakeMissingNativeSession implements the Session Service but has no linked
// native ID. Metadata absence is authoritative enough to park UNKNOWN.
type fakeMissingNativeSession struct{ session.Service }

func (fakeMissingNativeSession) Metadata(string) (session.Metadata, bool) {
	return session.Metadata{}, false
}

type fakeAutoFeedbackNative struct {
	*fakeAgentPatchProtocol
	meta session.Metadata
}

func (f *fakeAutoFeedbackNative) Metadata(id string) (session.Metadata, bool) {
	if id != f.expectedID {
		return session.Metadata{}, false
	}
	return f.meta, true
}

func TestAgentAutoFeedbackWorkerPoolDoesNotStarveOtherConversations(t *testing.T) {
	pool := agentAutoFeedbackWorkerPool{active: map[string]struct{}{}}
	for i := 0; i < agentAutoFeedbackWorkerLimit; i++ {
		if !pool.acquire(fmt.Sprintf("workspace:session-%d", i)) {
			t.Fatalf("independent session %d did not acquire a background worker", i)
		}
	}
	if pool.acquire("workspace:session-0") {
		t.Fatal("same session scheduled twice during long-running continuation")
	}
	if pool.acquire("workspace:session-extra") {
		t.Fatal("background worker limit was bypassed")
	}
	pool.release("workspace:session-1")
	if !pool.acquire("workspace:session-extra") {
		t.Fatal("queued session was not admitted when a background slot became free")
	}
	pool.release("workspace:session-extra")
	if !pool.acquire("workspace:session-1") {
		t.Fatal("released worker was not reusable")
	}
}

func TestAgentAutoFeedbackNeedsWorkRecognizesLongRunningNativeAndPendingAutomation(t *testing.T) {
	x := agentchat.Session{Owner:"local",Status:"waiting_tool"}
	if agentAutoFeedbackNeedsWork(x) {t.Fatal("idle session incorrectly occupies background worker")}
	x.ToolCalls=[]agentchat.ToolCallRecord{{Status:"running"}}
	if !agentAutoFeedbackNeedsWork(x) {t.Fatal("running native patch must be reconciled without browser")}
	x.ToolCalls[0].Status="pending"
	if agentAutoFeedbackNeedsWork(x) {t.Fatal("pending patch without AutoPatch grant is not autonomous")}
	x.AutoPatch=true
	if !agentAutoFeedbackNeedsWork(x) {t.Fatal("authorized pending Patch should resume")}
	x.AutoPatch=false
	x.ToolCalls[0].Status="done"
	x.ToolCalls[0].Result=`{"status":"PASS"}`
	x.AutoFeedback=true
	x.AutoFeedbackPersistent=true
	if !agentAutoFeedbackNeedsWork(x) {t.Fatal("verified result with persistent auto-feedback should resume")}
	x.Archived=true
	if agentAutoFeedbackNeedsWork(x) {t.Fatal("archived session must never be resumed")}
}

func TestAgentInterruptedQueueReconcilesOnlyAfterTurnLease(t *testing.T) {
	s,id,_ := agentPendingCallFixture(t,true)
	store,err := s.agentChatStore()
	if err != nil { t.Fatal(err) }
	x,err := store.Get(id,"local")
	if err != nil { t.Fatal(err) }
	x.Status="tool_queuing"
	x.ToolCalls[0].Status="queuing"
	x.AppendEvent("tool_queuing",x.Status,x.ToolCalls[0].CallID,"")
	x,err=store.Save(x,x.Revision)
	if err != nil {t.Fatal(err)}
	if !agentAutoFeedbackNeedsWork(x) {t.Fatal("QUEUING omitted from background reconciliation")}
	unlock,ok:=lockAgentTurn(s.Workspace,"local",id)
	if !ok {t.Fatal("cannot acquire approval lease")}
	s.reconcileAgentSession(context.Background(),store,id)
	locked,err:=store.Get(id,"local")
	unlock()
	if err!=nil||locked.Status!="tool_queuing" {t.Fatalf("live approval changed: %+v %v",locked,err)}
	s.reconcileAgentSession(context.Background(),store,id)
	recovered,err:=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	if recovered.Status!="tool_unknown"||recovered.ToolCalls[0].Status!="unknown"||recovered.ToolCalls[0].FeedbackSent {
		t.Fatalf("interrupted queue not safely parked: %+v",recovered)
	}
	if _,ok:=agentAutoPatchCandidate(recovered);ok {t.Fatal("ambiguous queue became eligible for replay")}
	if _,ok:=agentAutoFeedbackCandidate(recovered);ok {t.Fatal("ambiguous queue became eligible for feedback")}
	s.reconcileAgentSession(context.Background(),store,id)
	again,err:=store.Get(id,"local")
	if err!=nil||again.Revision!=recovered.Revision {t.Fatal("reconciliation not idempotent")}
}


func TestAgentAutoFeedbackHistoricalUnknownDoesNotBlockNewNativeFail(t *testing.T){
 s,id:=autoFeedbackReadySession(t)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 old:=x.ToolCalls[0]
 old.FeedbackSent=true;old.FeedbackStatus="unknown"
 current:=old
 current.CallID="new-failed-patch";current.Status="error";current.Result=`{"status":"FAIL","run_id":"1795589464744451_575193","rc":2,"project_unchanged":false,"automatic_retry":false}`
 current.FeedbackSent=false;current.FeedbackStatus=""
 var source agentchat.HistoryItem
 for _,it:=range x.History{if it.Type=="function_call"&&it.CallID==old.CallID{source=it}}
 if source.CallID==""{t.Fatal("missing fixture source")}
 next:=source;next.CallID=current.CallID
 x.History=append(x.History,agentchat.HistoryItem{Type:"function_call_output",CallID:old.CallID,Content:old.Result},
  agentchat.HistoryItem{Type:"user",Content:"tiếp"},agentchat.HistoryItem{Type:"assistant",Content:"Continue with a new proposal."},next)
 x.ToolCalls=[]agentchat.ToolCallRecord{old,current}
 // The new assistant call came from a completed provider request.
 if x.ProviderRun==nil{t.Fatal("missing provider run")}
 x.ProviderRun.Phase="completed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if idx,ok:=agentAutoFeedbackCandidate(x);!ok||idx!=1{t.Fatalf("new native FAIL blocked: %d %v",idx,ok)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if !strings.Contains(string(payload),"new-failed-patch")||!strings.Contains(string(payload),"1795589464744451_575193"){t.Fatal("wrong receipt")}
  return agentchat.ResponsesResult{ResponseID:"after-fail",Text:"Inspect workspace before a corrected patch."},nil
 }
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=1{t.Fatalf("provider calls=%d",calls)}
 for _,call:=range x.ToolCalls{
  if call.CallID==old.CallID&&call.FeedbackStatus!="unknown"{t.Fatal("historical acknowledgement manufactured")}
  if call.CallID==current.CallID&&call.FeedbackStatus!="acknowledged"{t.Fatal("native FAIL not delivered")}
 }
 s.runAgentAutoFeedback(context.Background())
 if calls!=1{t.Fatal("receipt resent")}
}

func TestAgentAutoFeedbackCurrentUnknownStillBlocks(t *testing.T){
 old:=agentchat.ToolCallRecord{CallID:"old",Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS"}`,FeedbackSent:true,FeedbackStatus:"unknown"}
 fresh:=agentchat.ToolCallRecord{CallID:"new",Name:"taskdeck_submit_patch",Status:"error",Result:`{"status":"FAIL"}`}
 x:=agentchat.Session{Owner:"local",Status:"waiting_tool",AutoFeedback:true,AutoFeedbackPersistent:true,ProviderRun:&agentchat.ProviderRun{Phase:"completed"},
 ToolCalls:[]agentchat.ToolCallRecord{fresh,old},History:[]agentchat.HistoryItem{{Type:"function_call",Name:old.Name,CallID:old.CallID},{Type:"function_call_output",CallID:old.CallID,Content:old.Result},{Type:"function_call",Name:fresh.Name,CallID:fresh.CallID}}}
 if _,reason:=agentAutoFeedbackAdmission(x);reason!="current_feedback_unconfirmed"{t.Fatalf("unresolved feedback bypassed: %s",reason)}
 x.History=append(x.History[:2],agentchat.HistoryItem{Type:"user",Content:"continue"},x.History[2])
 if idx,ok:=agentAutoFeedbackCandidate(x);!ok||idx!=0{t.Fatal("historical feedback blocked current result")}
 x.ProviderRun.Phase="unknown"
 if _,ok:=agentAutoFeedbackCandidate(x);ok{t.Fatal("active provider ambiguity bypassed")}
}
