package server

import (
 "context"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Model the reported incident: auto selection was refused after native start,
// the owner then executed the same approved ZIP in the native Patch panel.
func agentManualPatchHistoryFixture(t *testing.T) (*Server,string,string) {
 t.Helper()
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 approve:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
  map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if approve.Code!=200{t.Fatalf("cannot queue approved patch: %d %s",approve.Code,approve.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 call:=&x.ToolCalls[0]
 if call.QueuePath==""||call.ApprovalExpiresAt==nil||len(call.PackageSHA256)!=64{
  t.Fatalf("missing immutable approval receipt: %+v",call)
 }
 call.Status="unknown"
 call.Result="Workspace changed after the native session started; selection refused. Inspect the existing Patch session."
 call.PatchSessionID="original-native-session-before-selection"
 call.SelectedIndex=0
 call.PromptID=""
 call.RunID=""
 x.Status="tool_unknown"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 original:=filepath.Join(s.Workspace,filepath.FromSlash(call.QueuePath))
 archive:=filepath.Join(s.Workspace,"patchs","patched")
 if err=os.MkdirAll(archive,0700);err!=nil{t.Fatal(err)}
 name:=filepath.Base(call.QueuePath)
 if err=os.Rename(original,filepath.Join(archive,name));err!=nil{t.Fatal(err)}
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 history:=filepath.Join(s.Workspace,"artifacts","patch_tool","history")
 if err=os.MkdirAll(history,0700);err!=nil{t.Fatal(err)}
 report:=map[string]any{
  "format":"python-patch-tool-last-run","format_version":2,
  "project_identity":identity,"run_id":"manual-native-run-20261123",
  "finished_at":time.Now().UTC().Format(time.RFC3339Nano),
  "selected":[]string{name},"execution_order":[]string{name},
  "results":[]map[string]any{{"name":name,"kind":"PATCH","status":"PASS","rc":0}},
 }
 payload,err:=json.Marshal(report);if err!=nil{t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(history,"manual-success.json"),payload,0600);err!=nil{t.Fatal(err)}
 return s,id,name
}

func TestAgentManualPatchSuccessReconcilesWithoutReExecution(t *testing.T) {
 s,id,_:=agentManualPatchHistoryFixture(t)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 beforeSHA:=x.ToolCalls[0].PackageSHA256
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/sync",map[string]any{})
 if w.Code!=200{t.Fatalf("sync: %d %s",w.Code,w.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="waiting_tool"||x.ToolCalls[0].Status!="done"||
   x.ToolCalls[0].RunID!="manual-native-run-20261123"||
   x.ToolCalls[0].PackageSHA256!=beforeSHA||
   !strings.Contains(x.ToolCalls[0].Result,`"source":"native_patch_history_manual"`)||
   !strings.Contains(x.ToolCalls[0].Result,`"agent_auto_selection_refused":true`){
  t.Fatalf("manual Native PASS was not reconciled with original approval: %+v",x.ToolCalls[0])
 }
 if x.ToolCalls[0].FeedbackSent{t.Fatal("sync sent provider data without consent")}
}

func TestAgentManualPatchGetLastResultSendsOnceToAI(t *testing.T) {
 s,id,_:=agentManualPatchHistoryFixture(t)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 requests:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  requests++
  if !strings.Contains(string(payload),`"call_id":"call_agent_001"`)||
     !strings.Contains(string(payload),"native_patch_history_manual")||
     !strings.Contains(string(payload),"manual-native-run-20261123"){
   t.Fatalf("wrong manual receipt sent to provider: %s",payload)
  }
  return agentchat.ResponsesResult{ResponseID:"manual-ack",Text:"Manual Patch completion received."},nil
 }
 url:="/api/agent/sessions/"+id+"/tools/call_agent_001/recover"
 sent:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url,
  map[string]any{"action":"send_existing","confirm_external":true,"expected_revision":x.Revision})
 if sent.Code!=200{t.Fatalf("Get last result failed: %d %s",sent.Code,sent.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if requests!=1||x.ToolCalls[0].Status!="done"||
    x.ToolCalls[0].FeedbackStatus!="acknowledged"{
  t.Fatalf("manual PASS was not delivered exactly once: requests=%d state=%+v",requests,x)
 }
 duplicate:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,url,
  map[string]any{"action":"send_existing","confirm_external":true,"expected_revision":x.Revision})
 if duplicate.Code!=409{t.Fatalf("manual success delivered twice: %d",duplicate.Code)}
 if requests!=1{t.Fatal("manual Native run was duplicated or feedback replayed")}
}

func TestAgentManualPatchAutoFeedbackWhenEnabled(t *testing.T){
 s,id,_:=agentManualPatchHistoryFixture(t)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoFeedback=true
 x.AutoFeedbackPersistent=true
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if !strings.Contains(string(payload),"native_patch_history_manual"){t.Fatal("missing verified manual outcome")}
  return agentchat.ResponsesResult{ResponseID:"manual-auto-ack",Text:"OK"},nil
 }
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=1||x.ToolCalls[0].Status!="done"||x.ToolCalls[0].FeedbackStatus!="acknowledged"{
  t.Fatalf("automatic manual-result reconciliation failed: calls=%d state=%+v",calls,x.ToolCalls)
 }
 s.runAgentAutoFeedback(context.Background())
 if calls!=1{t.Fatal("automatically resent manual result")}
}

func TestAgentManualPatchHistoryMustBeUniqueAndBounded(t *testing.T){
 for _,tc:=range []string{"ambiguous_second_run","missing_finished_timestamp","wrong_archived_zip"}{
  t.Run(tc,func(t *testing.T){
   s,id,_:=agentManualPatchHistoryFixture(t)
   history:=filepath.Join(s.Workspace,"artifacts","patch_tool","history","manual-success.json")
   raw,err:=os.ReadFile(history);if err!=nil{t.Fatal(err)}
   var report map[string]any
   if err=json.Unmarshal(raw,&report);err!=nil{t.Fatal(err)}
   switch tc {
   case "ambiguous_second_run":
    report["run_id"]="manual-duplicate-run"
    b,_:=json.Marshal(report)
    if err=os.WriteFile(filepath.Join(filepath.Dir(history),"second-run.json"),b,0600);err!=nil{t.Fatal(err)}
   case "missing_finished_timestamp":
    delete(report,"finished_at")
    b,_:=json.Marshal(report)
    if err=os.WriteFile(history,b,0600);err!=nil{t.Fatal(err)}
   case "wrong_archived_zip":
    store,_:=s.agentChatStore()
    x,_:=store.Get(id,"local")
    archive:=filepath.Join(s.Workspace,"patchs","patched",filepath.Base(x.ToolCalls[0].QueuePath))
    if err=os.WriteFile(archive,[]byte("wrong zip"),0600);err!=nil{t.Fatal(err)}
   }
   w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/sync",map[string]any{})
   if w.Code!=200{t.Fatalf("sync: %d %s",w.Code,w.Body)}
   store,_:=s.agentChatStore()
   x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
   if x.ToolCalls[0].Status!="unknown"||x.ToolCalls[0].FeedbackSent{
    t.Fatalf("unverifiable manual result was adopted: %+v",x.ToolCalls[0])
   }
  })
 }
}
