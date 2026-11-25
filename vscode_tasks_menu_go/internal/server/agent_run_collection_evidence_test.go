package server

import (
 "context"
 "encoding/json"
 "net/http"
 "reflect"
 "sync/atomic"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentRunNativeCollectionEvidenceOwnerPlanCAS(t *testing.T){
 s,id,_,spec:=providerToolRunFixture(t,"run-collection-proof")
 spec,history:=enableRunAutomationFixture(t,s,spec,true,false)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,160)
 for i:=range steps{steps[i]=agentchat.TaskPlanStep{ID:"step-"+formatPlanStepIndex(i),Title:"Inspect source",Acceptance:"Review successful native Collection"}}
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Existing conversation history must survive evidence adoption"})
 x.TaskPlan,err=agentchat.NewTaskPlan("Review native observations",steps);if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 var reads,requests atomic.Int32
 s.AgentCollectionCall=func(_ string,identity patchtool.ProjectIdentity,_ []byte)string{
  reads.Add(1)
  raw,_:=json.Marshal(map[string]any{
   "source":"native_patch_tool_collect","project":identity,"status":"PASS","native_status":"PASS","rc":0,
   "evidence_verified":true,"partial_evidence_available":false,"native_session_id":"native-session","run_id":"agent_collect_fixture",
   "result_zip":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_fixture.zip",
   "result_text":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_fixture.txt",
  })
  return string(raw)
 }
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  if requests.Add(1)==1{return agentchat.ResponsesResult{ResponseID:"response-1",Calls:[]agentchat.FunctionCall{{CallID:"collect-exact",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"main.go"}]}`}}},nil}
  return agentchat.ResponsesResult{ResponseID:"response-2",Text:"Native read finished."},nil
 }
 manager,err:=s.agentRunManager("local");if err!=nil{t.Fatal(err)}
 defer manager.Close()
 if _,err=manager.Submit(spec,history);err!=nil{t.Fatal(err)}
 waitForAgentRunStatus(t,manager,spec.RunID,agentrun.Completed)
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 before:=append([]agentchat.HistoryItem(nil),x.History...)
 path:="/api/agent/sessions/"+id+"/plan"
 ref:="run:"+spec.RunID+"/collect-exact"
 action:=func(target,step,receipt string,revision uint64)int{
  result:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,target,map[string]any{
   "action":"set_step","expected_revision":revision,"step_id":step,"status":"done",
   "evidence_kind":"collection","evidence_ref":receipt,"evidence_note":"Reviewed native wrapper PASS",
  })
  if result.Code!=200&&result.Code!=409{t.Fatalf("unexpected response %d %s",result.Code,result.Body)}
  return result.Code
 }
 if action(path,"step-0000","run:"+spec.RunID+"/invented",x.Revision)!=409{t.Fatal("invented call accepted")}
 if action(path,"step-0000","run:foreign/collect-exact",x.Revision)!=409{t.Fatal("foreign run accepted")}
 if action(path+"?offset=0&limit=24","step-0000",ref,x.Revision)!=200{t.Fatal("paged run Collection proof rejected")}
 oldRevision:=x.Revision
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.Steps[0].Evidence[0].Ref!=ref||x.TaskPlan.Steps[0].Status!="done"{t.Fatal("paged proof not persisted")}
 if action(path,"step-0001",ref,oldRevision)!=409{t.Fatal("stale revision accepted")}
 if action(path,"step-0001",ref,x.Revision)!=200{t.Fatal("inline run Collection proof rejected")}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.State!="active"||x.TaskPlan.Steps[1].Evidence[0].Ref!=ref{t.Fatalf("incorrect plan state or receipt: %s %+v",x.TaskPlan.State,x.TaskPlan.Steps[1].Evidence)}
 if !reflect.DeepEqual(before,x.History){t.Fatalf("proof adoption changed history: before=%+v after=%+v",before,x.History)}
 if reads.Load()!=1||requests.Load()!=2{t.Fatal("proof adoption replayed native work or provider request")}
 proof,err:=s.resolveRunCollectionEvidence("local",id,ref,"");if err!=nil{t.Fatal(err)}
 other:=x;other.Owner="other"
 if _,err=proof.forSession(other);err==nil{t.Fatal("proof crossed owner boundary")}
 if _,err=s.resolveRunCollectionEvidence("local","different-conversation",ref,"");err==nil{t.Fatal("proof crossed conversation boundary")}
}

func TestAgentRunCollectionEvidencePairRejectsForeignAndDuplicate(t *testing.T){
 spec:=agentchat.RunSpec{RunID:"run-a",ProviderID:"provider-a",ModelID:"model-a",ParentTurnID:"turn-a"}
 call:=agentchat.HistoryItem{Type:"function_call",CallID:"exact",Name:"taskdeck_collect_workspace",OriginRunID:spec.RunID,ProviderID:spec.ProviderID,ModelID:spec.ModelID,ParentTurnID:spec.ParentTurnID}
 output:=call;output.Type="function_call_output";output.Content=`{"source":"native_patch_tool_collect"}`
 if _,err:=runCollectionEvidencePair([]agentchat.HistoryItem{call,output},spec,"exact");err!=nil{t.Fatal(err)}
 wrong:=output;wrong.OriginRunID="other"
 for _,items:=range [][]agentchat.HistoryItem{{call,wrong},{call,output,output},{output,call},{call},{call, {Type:"assistant",CallID:"exact",Content:output.Content}}}{
  if _,err:=runCollectionEvidencePair(items,spec,"exact");err==nil{t.Fatal("invalid pair accepted")}
 }
}
