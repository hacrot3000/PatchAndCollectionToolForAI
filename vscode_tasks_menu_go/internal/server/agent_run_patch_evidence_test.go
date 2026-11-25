package server

import (
 "context"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestAgentRunPatchAutoCheckRequiresCanonicalAcknowledgedReceipt(t *testing.T){
 s,id,_,spec:=providerToolRunFixture(t,"run-patch-proof")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Apply reviewed native Patch",[]agentchat.TaskPlanStep{
  {ID:"repair",Title:"Repair source",Acceptance:"Native Patch wrapper PASS"},
  {ID:"verify",Title:"Verify behavior",Acceptance:"Independent tests"},
 });if err!=nil{t.Fatal(err)}
 ref:="run:"+spec.RunID+"/patch-exact"
 x.TaskPlan.Steps[0].AutoCheck=&agentchat.TaskAutoCheck{Kind:"patch_pass",CallID:ref}
 x.TaskPlan.Steps[1].AutoCheck=&agentchat.TaskAutoCheck{Kind:"patch_pass",CallID:"run:other-run/patch-exact"}
 call:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"patch-exact",
  Arguments:`{"name":"reviewed patch"}`,OriginRunID:spec.RunID,ProviderID:spec.ProviderID,ModelID:spec.ModelID,ParentTurnID:spec.ParentTurnID}
 out:=agentchat.HistoryItem{Type:"function_call_output",CallID:call.CallID,Content:`{"status":"PASS","run_id":"native-run-1","rc":0}`,
  OriginRunID:spec.RunID,ProviderID:spec.ProviderID,ModelID:spec.ModelID,ParentTurnID:spec.ParentTurnID}
 x.ToolCalls=[]agentchat.ToolCallRecord{{AgentRunID:spec.RunID,CallID:call.CallID,Name:call.Name,
  Status:"done",ArgumentsSHA256:agentPatchPayloadSHA256(call.Arguments),Result:out.Content,
  RunID:"native-run-1",PatchSessionID:"native-session",PackageSHA256:strings.Repeat("a",64),
  FeedbackSent:true,FeedbackStatus:"acknowledged"}}
 snapshots,err:=s.agentContextSnapshotStore("local");if err!=nil{t.Fatal(err)}
 initial,err:=snapshots.Get(spec.ContextSnapshotID,spec.OwnerWorkspaceID,id,x.ProjectID);if err!=nil{t.Fatal(err)}
 history:=append(append([]agentchat.HistoryItem(nil),initial.History...),call,out)
 snapshot,err:=snapshots.CreateWithPolicy(spec.OwnerWorkspaceID,id,x.ProjectID,history,initial.SourceRefs,initial.Policy);if err!=nil{t.Fatal(err)}
 spec.ContextSnapshotID=snapshot.ID
 run:=agentrun.Record{Spec:spec,Status:agentrun.Running,History:history}
 proof,err:=verifyRunPatchProof(x,run,snapshot,call.CallID)
 if err!=nil||proof.Ref!=ref||proof.Kind!="patch"||len(proof.Digest)!=64{t.Fatalf("canonical proof failed: %+v %v",proof,err)}
 for _,tc:=range []struct{name string;mutate func(*agentchat.ToolCallRecord)}{
  {"unknown feedback",func(c *agentchat.ToolCallRecord){c.FeedbackStatus="unknown"}},
  {"prepared feedback",func(c *agentchat.ToolCallRecord){c.FeedbackStatus="prepared"}},
  {"foreign native run",func(c *agentchat.ToolCallRecord){c.RunID="foreign"}},
  {"foreign agent run",func(c *agentchat.ToolCallRecord){c.AgentRunID="foreign"}},
  {"no package",func(c *agentchat.ToolCallRecord){c.PackageSHA256=""}},
  {"wrong arguments",func(c *agentchat.ToolCallRecord){c.ArgumentsSHA256=strings.Repeat("b",64)}},
  {"wrong result",func(c *agentchat.ToolCallRecord){c.Result=`{"status":"PASS","run_id":"other","rc":0}`}},
 }{
  t.Run(tc.name,func(t *testing.T){bad:=x;bad.ToolCalls=append([]agentchat.ToolCallRecord(nil),x.ToolCalls...);tc.mutate(&bad.ToolCalls[0])
   if _,err:=verifyRunPatchProof(bad,run,snapshot,call.CallID);err==nil{t.Fatal("unverified Patch accepted")}
  })
 }
 duplicate:=x;duplicate.ToolCalls=append(append([]agentchat.ToolCallRecord(nil),x.ToolCalls...),x.ToolCalls[0])
 if _,err:=verifyRunPatchProof(duplicate,run,snapshot,call.CallID);err==nil{t.Fatal("ambiguous native receipt accepted")}
 tampered:=run;tampered.History=append([]agentchat.HistoryItem(nil),history...);tampered.History[len(history)-1].Content="forged"
 if _,err:=verifyRunPatchProof(x,tampered,snapshot,call.CallID);err==nil{t.Fatal("snapshot mismatch accepted")}
 changed,err:=applyRunCollectionAutoChecks(context.Background(),&x,run,snapshot)
 if err!=nil||!changed||x.TaskPlan.Steps[0].Status!="done"||len(x.TaskPlan.Steps[0].Evidence)!=1||
  x.TaskPlan.Steps[1].Status=="done"||x.TaskPlan.State!="active"{t.Fatalf("incorrect Patch proof projection: %v %+v",err,x.TaskPlan)}
 changed,err=applyRunCollectionAutoChecks(context.Background(),&x,run,snapshot)
 if err!=nil||changed||len(x.TaskPlan.Steps[0].Evidence)!=1{t.Fatal("repeat projection duplicated evidence")}
}
