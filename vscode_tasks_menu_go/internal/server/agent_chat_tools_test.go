package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func agentPendingCallFixture(t *testing.T, valid bool) (*Server, string, string) {
	t.Helper()
	s, id := newAgentConversationFixture(t, "stateless")
	identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	code := "# TASKDECK_PROJECT_KEY: " + identity.Key + "\n# TASKDECK_PROJECT_ID: " + identity.ID + "\nprint('test patch')\n"
	if !valid {
		code = "# TASKDECK_PROJECT_KEY: invalid\n# TASKDECK_PROJECT_ID: invalid\nprint('test patch')\n"
	}
	args, _ := json.Marshal(map[string]string{"name": "agent-check", "patch_python": code})
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		var req map[string]any
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatal(err)
		}
		if _, ok := req["tools"]; !ok {
			t.Fatal("tool schema was not advertised")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_pending", Calls: []agentchat.FunctionCall{{
			CallID: "call_agent_001", Name: "taskdeck_submit_patch", Arguments: string(args),
		}}}, nil
	}
	first := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Propose patch", "confirm_external": true, "expected_revision": 1})
	if first.Code != 200 {
		t.Fatalf("provider call failed: %d %s", first.Code, first.Body)
	}
	return s, id, code
}
func TestAgentToolApprovalQueuesUsingNativePatchToolOnce(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, e := s.agentChatStore()
	if e != nil {
		t.Fatal(e)
	}
	before, e := store.Get(id, "local")
	if e != nil {
		t.Fatal(e)
	}
	if len(before.ToolCalls) != 1 || before.ToolCalls[0].Status != "pending" {
		t.Fatalf("tool intent not persisted: %+v", before.ToolCalls)
	}
	path := "/api/agent/sessions/" + id + "/tools/call_agent_001/decision"
	missing := invokeAI(t, s.agentChatSessionItemAPI, "POST", path, map[string]any{
		"action": "approve", "expected_revision": before.Revision,
	})
	if missing.Code != 403 {
		t.Fatalf("missing explicit execution approval accepted: %d", missing.Code)
	}
	approved := invokeAI(t, s.agentChatSessionItemAPI, "POST", path, map[string]any{
		"action": "approve", "confirm_execution": true, "expected_revision": before.Revision,
	})
	if approved.Code != 200 {
		t.Fatalf("native queue submission failed: %d %s", approved.Code, approved.Body)
	}
	after, e := store.Get(id, "local")
	if e != nil {
		t.Fatal(e)
	}
	if after.ToolCalls[0].Status != "queued" || after.ToolCalls[0].QueuePath == "" {
		t.Fatalf("missing native queue receipt: %+v", after.ToolCalls)
	}
	queueFile := filepath.Join(s.Workspace, filepath.FromSlash(after.ToolCalls[0].QueuePath))
	info, e := os.Stat(queueFile)
	if e != nil || !info.Mode().IsRegular() {
		t.Fatalf("native Patch queue package missing: %v", e)
	}
	again := invokeAI(t, s.agentChatSessionItemAPI, "POST", path, map[string]any{
		"action": "approve", "confirm_execution": true, "expected_revision": after.Revision,
	})
	if again.Code != 409 {
		t.Fatalf("duplicate patch approval accepted: %d %s", again.Code, again.Body)
	}
	files, e := os.ReadDir(filepath.Join(s.Workspace, "patchs"))
	if e != nil {
		t.Fatal(e)
	}
	if len(files) != 1 {
		t.Fatalf("repeat generated additional patches: %d", len(files))
	}
}
func TestAgentToolRejectDoesNotQueueAndIsImmutable(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, false)
	before, e := s.agentChatStore()
	if e != nil {
		t.Fatal(e)
	}
	x, e := before.Get(id, "local")
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/agent/sessions/" + id + "/tools/call_agent_001/decision"
	result := invokeAI(t, s.agentChatSessionItemAPI, "POST", path, map[string]any{"action": "reject", "expected_revision": x.Revision})
	if result.Code != 200 {
		t.Fatalf("reject failed: %d %s", result.Code, result.Body)
	}
	x, e = before.Get(id, "local")
	if e != nil {
		t.Fatal(e)
	}
	if x.ToolCalls[0].Status != "rejected" || x.Status != "tool_rejected" {
		t.Fatalf("rejection not persisted %+v", x)
	}
	if _, err := os.Stat(filepath.Join(s.Workspace, "patchs")); !os.IsNotExist(err) {
		t.Fatalf("reject touched queue unexpectedly: %v", err)
	}
	repeat := invokeAI(t, s.agentChatSessionItemAPI, "POST", path, map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": x.Revision})
	if repeat.Code != 409 {
		t.Fatalf("rejected patch accepted later: %d", repeat.Code)
	}
}
func TestAgentToolInvalidNativeProjectIdentityRefusedBeforeQueue(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, false)
	store, e := s.agentChatStore()
	if e != nil {
		t.Fatal(e)
	}
	x, e := store.Get(id, "local")
	if e != nil {
		t.Fatal(e)
	}
	result := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": x.Revision})
	if result.Code != 400 {
		t.Fatalf("invalid patch identity accepted: %d %s", result.Code, result.Body)
	}
	now, e := store.Get(id, "local")
	if e != nil {
		t.Fatal(e)
	}
	if now.Revision != x.Revision || now.ToolCalls[0].Status != "pending" {
		t.Fatalf("bad patch changed queue or journal: %+v", now.ToolCalls)
	}
}
func TestAgentToolPermissionMappedAndBoundedArguments(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/agent/sessions/id/tools/call/decision", nil)
	permissions := sharedRoutePermissions(r)
	if len(permissions) != 2 || permissions[0] != identity.PermissionSettingsRead || permissions[1] != identity.PermissionPatchRun {
		t.Fatalf("Agent tool bypasses patch.run permission: %+v", permissions)
	}
	for _, raw := range []string{
		"{}",
		`{"name":"patch","patch_python":"test","unknown":"field"}`,
		`{"name":"patch","patch_python":""}`,
		`{"name":"","patch_python":"print(1)"}`,
		strings.Repeat("a", 512<<10+1),
	} {
		if _, e := parseAgentPatchArguments(raw); e == nil {
			t.Fatalf("invalid arguments accepted (len=%d): %s", len(raw), raw[:func() int {
				if len(raw) < 150 {
					return len(raw)
				}
				return 150
			}()])
		}
	}
}

func TestAgentApprovalRecordsGitSnapshotAndQueues(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	gitQuickRun(t, s.Workspace, "init", "-q")
	gitQuickRun(t, s.Workspace, "config", "user.email", "agent@example.invalid")
	gitQuickRun(t, s.Workspace, "config", "user.name", "Agent Test")
	if err := os.WriteFile(filepath.Join(s.Workspace, "tracked.txt"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, s.Workspace, "add", "tracked.txt")
	gitQuickRun(t, s.Workspace, "commit", "-q", "-m", "initial")
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": x.Revision})
	if w.Code != 200 {
		t.Fatalf("approval failed: %d %s", w.Code, w.Body)
	}
	y, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	call := y.ToolCalls[0]
	if !call.GitIsRepo || len(call.ExpectedHead) != 40 || call.ExpectedBranch == "" || len(call.WorkspaceStatusSHA256) != 64 || call.ApprovalExpiresAt == nil {
		t.Fatalf("approval snapshot missing: %+v", call)
	}
	if call.ModelTurnID == "" || len(call.ArgumentsSHA256) != 64 {
		t.Fatalf("turn/argument binding missing: %+v", call)
	}
}

func TestAgentApprovalRefusesWorkspaceDrift(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	gitQuickRun(t, s.Workspace, "init", "-q")
	gitQuickRun(t, s.Workspace, "config", "user.email", "agent@example.invalid")
	gitQuickRun(t, s.Workspace, "config", "user.name", "Agent Test")
	if err := os.WriteFile(filepath.Join(s.Workspace, "a.txt"), []byte("a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, s.Workspace, "add", "a.txt")
	gitQuickRun(t, s.Workspace, "commit", "-q", "-m", "first")
	snap, err := agentWorkspaceSnapshotOf(context.Background(), s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Minute)
	x.ToolCalls[0].ExpectedHead = snap.Head
	x.ToolCalls[0].ExpectedBranch = snap.Branch
	x.ToolCalls[0].WorkspaceStatusSHA256 = snap.StatusSHA256
	x.ToolCalls[0].GitIsRepo = true
	x.ToolCalls[0].ApprovalExpiresAt = &expires
	x, err = store.Save(x, x.Revision)
	if err != nil {
		t.Fatal(err)
	}
	// A dirty worktree change after review must fail closed.
	if err := os.WriteFile(filepath.Join(s.Workspace, "a.txt"), []byte("dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	blocked := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": x.Revision})
	// HTTP 200 acknowledges a persisted refusal, not execution approval.
	if blocked.Code != 200 || !strings.Contains(blocked.Body.String(), `"status":"rejected"`) ||
		!strings.Contains(blocked.Body.String(), `"execution_enabled":false`) {
		t.Fatalf("worktree drift did not return a definitive refusal: %d %s", blocked.Code, blocked.Body)
	}
	z, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if z.Status != "tool_rejected" || !agentRepairableToolDenial(z.ToolCalls[0]) ||
		!agentPatchHasNoNativeAttempt(z.ToolCalls[0]) {
		t.Fatalf("drifted approval not rejected without queueing: %+v", z.ToolCalls[0])
	}
	contents, err := os.ReadFile(filepath.Join(s.Workspace, "a.txt"))
	if err != nil || string(contents) != "dirty\n" {
		t.Fatal("stale Patch changed the worktree")
	}
}

func TestAgentApprovalExpiresBeforeQueue(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, s.Workspace, "init", "-q")
	past := time.Now().UTC().Add(-time.Minute)
	x.ToolCalls[0].ApprovalExpiresAt = &past
	x, err = store.Save(x, x.Revision)
	if err != nil {
		t.Fatal(err)
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": x.Revision})
	if w.Code != 409 {
		t.Fatalf("expired approval accepted: %d", w.Code)
	}
}

func TestAgentApprovalRefusesChangedToolArguments(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	x.ToolCalls[0].ArgumentsSHA256 = strings.Repeat("0", 64)
	x, err = store.Save(x, x.Revision)
	if err != nil {
		t.Fatal(err)
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
		map[string]any{"action": "approve", "confirm_execution": true, "expected_revision": x.Revision})
	if w.Code != 409 {
		t.Fatalf("edited arguments approved: %d %s", w.Code, w.Body)
	}
}

func TestAgentWorkspaceLeaseWaitsForConcurrentExecution(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 release,err:=agentReserveWorkspaceExecution(context.Background(),s.Workspace)
 if err!=nil{t.Fatal(err)}
 defer release()
 store,e:=s.agentChatStore();if e!=nil{t.Fatal(e)}
 x,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
 done:=make(chan *httptest.ResponseRecorder,1)
 go func(){
  path:="/api/agent/sessions/"+id+"/tools/call_agent_001/decision"
  body,_:=json.Marshal(map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
  req:=httptest.NewRequest(http.MethodPost,path,strings.NewReader(string(body)))
  req.Header.Set("Content-Type","application/json")
  w:=httptest.NewRecorder()
  s.agentChatSessionItemAPI(w,req)
  done<-w
 }()
 deadline:=time.Now().Add(3*time.Second)
 for agentNativeWaitSnapshot(s.Workspace,"local",id)!="waiting_for_workspace"{
  select{case w:=<-done:t.Fatalf("approval did not wait: %d %s",w.Code,w.Body);default:}
  if time.Now().After(deadline){t.Fatal("wait state not visible")}
  time.Sleep(10*time.Millisecond)
 }
 y,e:=store.Get(id,"local");if e!=nil||y.ToolCalls[0].Status!="pending"{t.Fatal("waiting approval changed ledger")}
 release()
 select{case w:=<-done:if w.Code!=200{t.Fatalf("approval did not resume: %d %s",w.Code,w.Body)}
 case <-time.After(5*time.Second):t.Fatal("approval did not resume")}
}

func TestAgentSessionStoreRejectsOrphanToolOutput(t *testing.T) {
	s, err := agentchat.NewStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	x, err := agentchat.NewSession("owner", "/workspace", "project", "provider", "model", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.Save(x, 0)
	if err != nil {
		t.Fatal(err)
	}
	x.History = []agentchat.HistoryItem{{Type: "function_call_output", CallID: "never-called", Content: "result"}}
	if _, err = s.Save(x, x.Revision); !errors.Is(err, agentchat.ErrInvalidSession) {
		t.Fatalf("orphan tool output accepted: %v", err)
	}
}

func TestAgentAskUserWaitsForOwnerAndContinuesExactCall(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 arguments:=`{"questions":[{"id":"platform","prompt":"Platform?","type":"single_choice","required":true,"options":[{"id":"ios","label":"iOS"},{"id":"android","label":"Android"}],"allow_comment":true},{"id":"features","prompt":"Features?","type":"multiple_choice","required":false,"options":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"allow_comment":false},{"id":"details","prompt":"Details?","type":"text","required":true,"options":[],"allow_comment":false}]}`
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==1{
   if !strings.Contains(string(payload),agentchat.AskUserToolName){t.Fatal("questionnaire tool not advertised")}
   return agentchat.ResponsesResult{ResponseID:"questions",Calls:[]agentchat.FunctionCall{{CallID:"q1",Name:agentchat.AskUserToolName,Arguments:arguments}}},nil
  }
  if calls!=2{t.Fatal("duplicate answer egress")}
  var wire struct{Input []struct{Type string;CallID string `json:"call_id"`;Output string}}
  if err:=json.Unmarshal(payload,&wire);err!=nil{t.Fatal(err)}
  found:=0
  for _,item:=range wire.Input{if item.Type=="function_call_output"&&item.CallID=="q1"{
   found++;var result agentchat.AskUserResult
   if json.Unmarshal([]byte(item.Output),&result)!=nil||result.Status!="ANSWERED"||len(result.Answers)!=3||result.Answers[0].Selected[0]!="ios"||len(result.Answers[1].Selected)!=2||result.Answers[2].Text!="Chi tiết"{t.Fatalf("invalid answer receipt: %s",item.Output)}
  }}
  if found!=1{t.Fatalf("wrong call output count: %d",found)}
  return agentchat.ResponsesResult{ResponseID:"continued",Text:"Proceeding from actual answers."},nil
 }
 path:="/api/agent/sessions/"+id
 first:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path+"/messages",map[string]any{"message":"Ask for missing choices","confirm_external":true,"expected_revision":1})
 if first.Code!=200{t.Fatalf("question request: %d %s",first.Code,first.Body)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="waiting_tool"||len(x.ToolCalls)!=1||x.ToolCalls[0].Name!=agentchat.AskUserToolName||x.ToolCalls[0].Status!="pending"{t.Fatalf("question not durably pending: %+v",x.ToolCalls)}
 x.AutoPatch=true;x.AutoFeedback=true;x.AutoFeedbackPersistent=true;x.AutoContinueText=true
 if _,ok:=agentAutoPatchCandidate(x);ok{t.Fatal("question incorrectly auto-approved")}
 if _,ok:=agentAutoFeedbackCandidate(x);ok||agentchat.StandaloneAutoContinueCandidate(x){t.Fatal("question incorrectly auto-answered")}
 answerPath:=path+"/tools/q1/answer"
 payload:=map[string]any{"confirm_external":true,"arguments_sha256":x.ToolCalls[0].ArgumentsSHA256,"answers":[]agentchat.AskUserAnswer{{QuestionID:"platform",Selected:[]string{"ios"},Text:"Extra"},{QuestionID:"features",Selected:[]string{"a","b"}},{QuestionID:"details",Text:"Chi tiết"}}}
 invalid:=invokeAI(t,s.agentChatSessionItemAPI,"POST",answerPath,map[string]any{"confirm_external":true,"arguments_sha256":x.ToolCalls[0].ArgumentsSHA256})
 if invalid.Code!=400||calls!=1{t.Fatalf("missing required answers accepted: %d",invalid.Code)}
 native:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path+"/tools/q1/decision",map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if native.Code!=409{t.Fatalf("question became native Patch: %d",native.Code)}
 sent:=invokeAI(t,s.agentChatSessionItemAPI,"POST",answerPath,payload)
 if sent.Code!=200{t.Fatalf("answer continuation: %d %s",sent.Code,sent.Body)}
 after,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if after.Status!="completed"||after.ToolCalls[0].FeedbackStatus!="acknowledged"||after.ToolCalls[0].QueuePath!=""||calls!=2{t.Fatalf("answer journal not acknowledged: %+v",after.ToolCalls)}
 again:=invokeAI(t,s.agentChatSessionItemAPI,"POST",answerPath,payload)
 if again.Code!=409||calls!=2{t.Fatal("duplicate answer was sent")}
}
