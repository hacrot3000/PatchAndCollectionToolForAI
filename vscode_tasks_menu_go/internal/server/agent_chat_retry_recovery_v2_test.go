package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestAgentRecoverLatestOfRepeatedPreflightFailuresWithoutRerun(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 name:="PATCH_CLEAR_TEXT_Vietnamese-diacritics-and-phpinfo-output_20261120T072133_0a2531d7.zip"
 pkg:=[]byte("approved immutable package")
 digest:=sha256.Sum256(pkg)
 queue,err:=s.patchUploadDir()
 if err!=nil {t.Fatal(err)}
 if err:=os.MkdirAll(queue,0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(queue,name),pkg,0600);err!=nil {t.Fatal(err)}
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil {t.Fatal(err)}
 hist:=filepath.Join(s.Workspace,"artifacts","patch_tool","history")
 if err:=os.MkdirAll(hist,0700);err!=nil {t.Fatal(err)}
 for i,row:=range []struct{run,finished string}{
  {"old-preflight-run","2026-11-20T14:21:33+07:00"},
  {"latest-preflight-run","2026-11-20T14:23:20+07:00"},
 }{
  report:=map[string]any{
   "format":"python-patch-tool-last-run","format_version":2,
   "project_identity":identity,"run_id":row.run,"finished_at":row.finished,
   "selected":[]string{name},"execution_order":[]string{},
   "results":[]map[string]any{{
    "name":name,"kind":"PATCH","status":"PREFLIGHT_FAIL","rc":2,
    "diagnosis":map[string]any{"kind":"insufficient_disk_space","message":"batch preflight failed: insufficient_disk_space"},
   }},
  }
  b,err:=json.Marshal(report)
  if err!=nil {t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(hist,string(rune('a'+i))+".json"),b,0600);err!=nil {t.Fatal(err)}
 }
 x.ToolCalls[0].Status="running"
 x.ToolCalls[0].PatchSessionID="expired-run"
 x.ToolCalls[0].QueuePath="patchs/"+name
 x.ToolCalls[0].PackageSHA256=hex.EncodeToString(digest[:])
 x.ToolCalls[0].PackageSize=int64(len(pkg))
 x.ToolCalls[0].SelectedIndex=0
 x.Status="waiting_tool"
 x,err=store.Save(x,x.Revision)
 if err!=nil {t.Fatal(err)}
 // The ordinary background reconciler refuses ambiguous same-name runs.
 s.Sessions=&fakeAgentPatchProtocol{expectedID:"not-expired-run",state:session.ProtocolState{}}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/sync",map[string]any{})
 if w.Code!=200 {t.Fatalf("sync: %d %s",w.Code,w.Body)}
 before,err:=store.Get(id,"local")
 if err!=nil || before.ToolCalls[0].Status!="unknown" {
  t.Fatalf("automatic sync must not guess the latest run: %+v, %v",before.ToolCalls,err)
 }
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if !strings.Contains(string(payload),"latest-preflight-run") || strings.Contains(string(payload),"old-preflight-run") {
   t.Fatalf("wrong native History run sent to AI: %s",payload)
  }
  return agentchat.ResponsesResult{ResponseID:"recovered",Text:"Native patch failed due to insufficient disk space."},nil
 }
 w=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/tools/"+before.ToolCalls[0].CallID+"/recover",
  map[string]any{"action":"send_existing","expected_revision":before.Revision,"confirm_external":true})
 if w.Code!=200 {t.Fatalf("explicit latest result: %d %s",w.Code,w.Body)}
 after,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if calls!=1 || after.ToolCalls[0].RunID!="latest-preflight-run" ||
   !after.ToolCalls[0].FeedbackSent || after.ToolCalls[0].Status!="error" {
  t.Fatalf("latest failed run not delivered exactly once: calls=%d %+v",calls,after.ToolCalls[0])
 }
 if got,err:=os.ReadFile(filepath.Join(queue,name));err!=nil || string(got)!=string(pkg){
  t.Fatal("send_existing altered or re-executed the native Patch package")
 }
}

func TestAgentReapplyAndSendIsOneShotAndWorksWithAutoFeedbackOff(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 approve:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/tools/"+x.ToolCalls[0].CallID+"/decision",
  map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if approve.Code!=200 {t.Fatalf("initial native queue refused: %d %s",approve.Code,approve.Body)}
 x,err=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 x.ToolCalls[0].Status="error"
 x.ToolCalls[0].RunID="previous-failure"
 x.ToolCalls[0].Result=`{"status":"PREFLIGHT_FAIL","rc":2,"run_id":"previous-failure"}`
 x.Status="waiting_tool"
 x.AutoPatch=false
 x.AutoFeedback=false
 x,err=store.Save(x,x.Revision)
 if err!=nil {t.Fatal(err)}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
  "/api/agent/sessions/"+id+"/tools/"+x.ToolCalls[0].CallID+"/recover",
  map[string]any{"action":"rerun_and_send","expected_revision":x.Revision,
   "confirm_execution":true,"confirm_external":true})
 if w.Code!=200 {t.Fatalf("one-shot reapply not accepted: %d %s",w.Code,w.Body)}
 pending,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 call:=pending.ToolCalls[0]
 if call.Status!="pending" || !call.AutoRunOnce || !call.SendAfterRetry ||
    call.ManualRetryOnly || len(call.Attempts)!=1 || call.Attempts[0].RunID!="previous-failure" {
  t.Fatalf("incorrect one-shot retry state: %+v",call)
 }
 if id,ok:=agentAutoPatchCandidate(pending);!ok || id!=call.CallID {
  t.Fatal("one-shot retry not eligible for native approval with Auto Patch OFF")
 }
 // Simulate the FINAL native receipt: a failure must still auto-send once.
 pending.ToolCalls[0].Status="error"
 pending.ToolCalls[0].RunID="new-failure"
 pending.ToolCalls[0].Result=`{"status":"PREFLIGHT_FAIL","rc":2,"diagnosis_kind":"insufficient_disk_space"}`
 pending.ToolCalls[0].AutoRunOnce=false
 pending.Status="waiting_tool"
 pending,err=store.Save(pending,pending.Revision)
 if err!=nil {t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if !strings.Contains(string(payload),"insufficient_disk_space") {t.Fatalf("missing retry failure: %s",payload)}
  return agentchat.ResponsesResult{ResponseID:"after-retry",Text:"Patch retry also failed."},nil
 }
 s.runAgentAutoFeedback(context.Background())
 done,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if calls!=1 || !done.ToolCalls[0].FeedbackSent || done.ToolCalls[0].FeedbackStatus!="acknowledged" {
  t.Fatalf("final failed rerun not delivered: calls=%d %+v",calls,done.ToolCalls[0])
 }
 s.runAgentAutoFeedback(context.Background())
 if calls!=1 {t.Fatal("one-shot retry result was sent twice")}
}
