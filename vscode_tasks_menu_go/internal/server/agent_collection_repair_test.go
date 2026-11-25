package server

import (
 "context"
 "fmt"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentCollectionRepairBudgetChangingRequests(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 calls,reads:=0,0
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{
  reads++
  return runCollectionDiagnostic("COLLECT_SCHEMA_INVALID","action[1].paths must be a non-empty array of project-relative paths",true)
 }
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,body []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls>1&&(!strings.Contains(string(body),"repair_attempt")||!strings.Contains(string(body),"error_fingerprint")){t.Fatal("missing structured repair guidance")}
  if calls>agentCollectionRepairBudget+1{t.Fatal("unbounded repair loop")}
  return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("response-%d",calls),Calls:[]agentchat.FunctionCall{{CallID:fmt.Sprintf("call-%d",calls),Name:"taskdeck_collect_workspace",Arguments:fmt.Sprintf(`{"title":"attempt-%d","actions":[{"type":"pack","paths":"bad"}]}`,calls)}}},nil
 }
 res:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",map[string]any{"message":"inspect source files","confirm_external":true,"expected_revision":1})
 if res.Code==200||calls!=agentCollectionRepairBudget+1||reads!=calls{t.Fatalf("repair loop: status=%d calls=%d reads=%d body=%s",res.Code,calls,reads,res.Body)}
 store,_:=s.agentChatStore();x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="failed"||x.ProviderRun==nil||x.ProviderRun.Phase!="acknowledged"{t.Fatalf("local error lost acknowledged boundary: %+v",x.ProviderRun)}
 last:=x.History[len(x.History)-1]
 if last.Type!="function_call_output"||last.CallID!=fmt.Sprintf("call-%d",calls)||!strings.Contains(last.Content,`"retry_allowed":false`){t.Fatal("terminal rejection not bound to original call")}
}

func TestAgentProviderAcknowledgementCannotRegressToUnknown(t *testing.T){
 x:=agentchat.Session{ProviderRun:&agentchat.ProviderRun{Phase:"acknowledged"}}
 markAgentProviderOutcome(&x,"unknown")
 if x.ProviderRun.Phase!="acknowledged"{t.Fatal("acknowledgement regressed")}
 x.ProviderRun.Phase="sending";markAgentProviderOutcome(&x,"unknown")
 if x.ProviderRun.Phase!="unknown"{t.Fatal("ambiguous delivery was not preserved")}
}

func TestAgentRunCollectionRepairCountScopedToParentAndProgress(t *testing.T){
 bad:=agentCollectionRepairFeedback("COLLECT_SCHEMA_INVALID","action[1].paths must be a non-empty array of project-relative paths",1)
 h:=[]agentchat.HistoryItem{{Type:"user",Content:"task"},{Type:"function_call",CallID:"a",Name:"taskdeck_collect_workspace",ParentTurnID:"p"},{Type:"function_call_output",CallID:"a",Content:bad},{Type:"assistant",Content:"repairing"},{Type:"function_call",CallID:"b",Name:"taskdeck_collect_workspace",ParentTurnID:"p"}}
 if n:=agentRunCollectionRepairCount(h,"p");n!=1{t.Fatalf("count=%d",n)}
 if n:=agentRunCollectionRepairCount(h,"other");n!=0{t.Fatalf("cross-parent count=%d",n)}
 h=append(h,agentchat.HistoryItem{Type:"function_call_output",CallID:"b",Content:`{"status":"COMPLETE"}`})
 if n:=agentRunCollectionRepairCount(h,"p");n!=0{t.Fatalf("progress did not reset count=%d",n)}
}

func TestAgentRecoverLegacyCollectionAcknowledgementRequiresExactJournal(t *testing.T){
 fixture:=func()agentchat.Session{
  x:=agentchat.Session{Owner:"local",Status:"failed",EventSeq:60,ProviderRun:&agentchat.ProviderRun{ID:"r50-e61",Kind:"collection",Phase:"unknown",CallID:"call-8",PayloadSHA256:strings.Repeat("a",64),UsageRecorded:true}}
  x.AppendEvent("provider_egress","requesting_ai","call-8","collection")
  x.AppendEvent("collect_request_rejected","failed","","invalid project identity: json: cannot unmarshal string")
  x.AppendEvent("collect_failed","failed","","collection_continuation")
  return x
 }
 x:=fixture()
 if !agentRecoverAcknowledgedCollection(&x)||x.ProviderRun.Phase!="acknowledged"||x.Status!="failed"{t.Fatal("confirmed local failure not recovered")}
 if agentRecoverAcknowledgedCollection(&x){t.Fatal("recovery not idempotent")}
 for _,mutate:=range []func(*agentchat.Session){
  func(x *agentchat.Session){x.ProviderRun.UsageRecorded=false},
  func(x *agentchat.Session){x.ProviderRun.ID="r50-e60"},
  func(x *agentchat.Session){x.ProviderRun.CallID="different"},
  func(x *agentchat.Session){x.Events[1].Detail="network timeout"},
  func(x *agentchat.Session){x.AppendEvent("turn_interrupted","failed","","provider_egress_after_restart_unknown")},
 }{
  x:=fixture();mutate(&x)
  if agentRecoverAcknowledgedCollection(&x)||x.ProviderRun.Phase!="unknown"{t.Fatal("ambiguous request incorrectly reconciled")}
 }
}
