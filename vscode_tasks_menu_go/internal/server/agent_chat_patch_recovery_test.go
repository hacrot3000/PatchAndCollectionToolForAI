package server

import (
 "context"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentRecoverPatchNeverReexecutesUnverifiedRunningCall(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,_:=s.agentChatStore()
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 x.ToolCalls[0].Status="running"
 x.ToolCalls[0].PatchSessionID="expired-without-evidence"
 x.ToolCalls[0].QueuePath="patchs/unknown.zip"
 x.Status="waiting_tool"
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/tools/call_agent_001/recover"
 for _,action:=range []string{"send_existing","rerun_patch"}{
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,
   map[string]any{"action":action,"expected_revision":x.Revision,
    "confirm_external":true,"confirm_execution":true})
  if w.Code!=409{t.Fatalf("ambiguous running Patch %s allowed: %d %s",action,w.Code,w.Body)}
 }
 latest,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 if latest.ToolCalls[0].Status!="running"||latest.ToolCalls[0].FeedbackSent||
  len(latest.ToolCalls[0].Attempts)!=0{
  t.Fatalf("unverified Patch unexpectedly retried: %+v",latest.ToolCalls[0])
 }
}

func TestAgentRerunPatchRequiresVerifiedPriorFailureAndNewManualApproval(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,_:=s.agentChatStore()
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/tools/call_agent_001/decision"
 approved:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,
  map[string]any{"action":"approve","confirm_execution":true,"expected_revision":x.Revision})
 if approved.Code!=200{t.Fatalf("cannot queue first attempt: %d %s",approved.Code,approved.Body)}
 x,err=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 if x.ToolCalls[0].QueuePath==""{t.Fatal("missing prior approved native package")}
 x.ToolCalls[0].Status="error"
 x.ToolCalls[0].RunID="native-failed-run"
 x.ToolCalls[0].Result=`{"status":"PREFLIGHT_FAIL","rc":2,"run_id":"native-failed-run"}`
 x.AutoPatch=true // must NOT auto-approve this manual retry
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 recoverURL:="/api/agent/sessions/"+id+"/tools/call_agent_001/recover"
 retry:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,recoverURL,
  map[string]any{"action":"rerun_patch","confirm_execution":true,"expected_revision":x.Revision})
 if retry.Code!=200{t.Fatalf("manual retry preparation failed: %d %s",retry.Code,retry.Body)}
 after,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 call:=after.ToolCalls[0]
 if call.Status!="pending"||!call.ManualRetryOnly||len(call.Attempts)!=1||
  call.Attempts[0].RunID!="native-failed-run"||call.QueuePath!=""||call.RunID!=""{
  t.Fatalf("original failure not preserved for fresh approval: %+v",call)
 }
 if _,ok:=agentAutoPatchCandidate(after);ok{t.Fatal("Auto Patch must never execute a manual retry")}
 // The server must not requeue anything until an explicit new approval.
 if strings.Contains(retry.Body.String(),`"patch_reexecuted":true`){t.Fatal("retry endpoint lied about execution")}
 again:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,recoverURL,
  map[string]any{"action":"rerun_patch","confirm_execution":true,"expected_revision":after.Revision})
 if again.Code!=409{t.Fatalf("replayed manual retry %d",again.Code)}
}

func TestAgentRecoverVerifiedOldFailureSendsResultWithoutNewPatch(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 store,_:=s.agentChatStore()
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{ResponseID:"continuation-1",Text:"Old Patch failed; I will revise the proposal."},nil
 }
 x.ToolCalls[0].Status="error"
 x.ToolCalls[0].Result=`{"status":"PREFLIGHT_FAIL","rc":2,"diagnosis_kind":"insufficient_disk_space"}`
 x.ToolCalls[0].RunID="failed-run"
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 recoverURL:="/api/agent/sessions/"+id+"/tools/call_agent_001/recover"
 response:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,recoverURL,
  map[string]any{"action":"send_existing","confirm_external":true,"expected_revision":x.Revision})
 if response.Code!=200{t.Fatalf("verified old failure not delivered: %d %s",response.Code,response.Body)}
 after,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 if !after.ToolCalls[0].FeedbackSent||calls!=1{
  t.Fatalf("old failure not sent exactly once: calls=%d feedback=%+v",calls,after.ToolCalls[0])
 }
 duplicate:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,recoverURL,
  map[string]any{"action":"send_existing","confirm_external":true,"expected_revision":after.Revision})
 if duplicate.Code!=409{t.Fatalf("duplicate old result sent: %d",duplicate.Code)}
}
