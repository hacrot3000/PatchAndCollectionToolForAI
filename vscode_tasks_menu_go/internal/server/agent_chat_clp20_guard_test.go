package server

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentCLP20VerifiedStallPreservesReceiptAndBoundsReplan(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.AutoContinueText=true
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 reads,calls,replans:=0,0,0
 receipts:=map[string]string{}
 s.AgentCollectionCall=func(_ string,identity patchtool.ProjectIdentity,_ []byte)string{
  reads++
  digest:=strings.Repeat("a",64)
  if reads>14{digest=strings.Repeat("b",64)}
  raw,_:=json.Marshal(map[string]any{"source":"native_patch_tool_collect","project":identity,
   "native_session_id":"synthetic-native","run_id":fmt.Sprintf("native-%d",reads),
   "status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"text_excerpt_truncated":false,
   "native_full_file_reads":[]map[string]any{{"path":"src/a.go","action_index":1,"total_lines":10,"report_sha256":digest}},
  })
  value:=string(raw)
  receipts[fmt.Sprintf("stall-%d",reads)]=value
  return value
 }
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if strings.Contains(string(payload),strings.TrimSpace(collectionReplanAdvice)){replans++}
  if calls>1&&calls<=14{
   var wire struct{Input []map[string]any `json:"input"`}
   if err:=json.Unmarshal(payload,&wire);err!=nil{t.Fatal(err)}
   callID:=fmt.Sprintf("stall-%d",calls-1);count:=0
   for _,it:=range wire.Input{
    if it["type"]=="function_call_output"&&it["call_id"]==callID{
     count++;if it["output"]!=receipts[callID]{t.Fatal("replan changed current receipt")}
    }
   }
   if count!=1{t.Fatalf("current receipt count %d at call %d",count,calls)}
  }
  if calls<=15{return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("reply-%d",calls),Calls:[]agentchat.FunctionCall{{CallID:fmt.Sprintf("stall-%d",calls),Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"src/a.go"}]}`}}},nil}
  if calls==16{return agentchat.ResponsesResult{ResponseID:"novel",Text:"New source observed."},nil}
  t.Fatalf("unexpected provider call %d",calls);return agentchat.ResponsesResult{},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",map[string]any{"message":"Fix the bug","confirm_external":true,"expected_revision":x.Revision})
 if w.Code<400||reads!=14||calls!=14||replans!=1{t.Fatalf("stall not bounded: HTTP %d reads=%d calls=%d replans=%d body=%s",w.Code,reads,calls,replans,w.Body)}
 saved,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 g,ok:=saved.ReadCollectionGuard()
 if !ok||g.Recovery.Attempts!=1||g.Counters.NewEvidence!=1||g.Counters.DuplicateEvidence!=13||saved.ProviderRun.Phase!="acknowledged"{t.Fatalf("wrong checkpoint: %+v %+v",g,saved.ProviderRun)}
 count:=0
 for _,it:=range saved.History{if it.Type=="function_call_output"{count++;if receipts[it.CallID]!=it.Content{t.Fatal("canonical receipt mutated")}}}
 if count!=14||len(saved.ToolCalls)!=0{t.Fatal("lost checkpoint or invented Patch")}
 w=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",map[string]any{"message":"Continue","confirm_external":true,"expected_revision":saved.Revision})
 if w.Code!=200||reads!=15||calls!=16||replans!=1{t.Fatalf("novel evidence did not resume: HTTP=%d reads=%d calls=%d replans=%d %s",w.Code,reads,calls,replans,w.Body)}
 saved,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 after,ok:=saved.ReadCollectionGuard()
 if !ok||after.Generation!=g.Generation||after.Recovery.Attempts!=1||after.Counters.NewEvidence!=2||after.Counters.Progress.DuplicateStreak!=0{t.Fatalf("resume reset objective or recovery budget: %+v",after)}
}


func TestAgentCLP20StallRequiresExactDuplicateAndLiveConsent(t *testing.T){
 x,err:=agentchat.NewSession("local","/workspace","project","provider","model",agentchat.Stateless);if err!=nil{t.Fatal(err)}
 if !x.BindCollectionGuard("Fix issue","",false){t.Fatal("bind")}
 x.AutoContinueText=true
 g,_:=x.ReadCollectionGuard()
 for i:=0;i<8;i++{
  reduceLegacyCollectionEvent(&x,g,"read",cp.CollectionObserved,cp.VerifiedDuplicate,true,true)
  g,_=x.ReadCollectionGuard()
 }
 if !legacyCollectionStallCandidate(x,"read"){t.Fatal("verified stall missed")}
 if !legacyCollectionReplanAllowed(x){t.Fatal("grant ignored")}
 x.AutoContinueText=false
 if legacyCollectionReplanAllowed(x){t.Fatal("revoked continuation allowed")}
 x.AutoContinueText=true
 disabled:=false;x.AutoCollection=&disabled
 if legacyCollectionReplanAllowed(x){t.Fatal("revoked collection allowed")}
 // Even if stale/corrupt progress metadata claims a duplicate streak, the
 // actual last counter event must have the exact duplicate digest.
 g.Counters.LastEventDigest=strings.Repeat("a",64)
 if !x.SetCollectionGuard(g){t.Fatal("fixture")}
 if legacyCollectionStallCandidate(x,"read"){t.Fatal("stale duplicate caused checkpoint")}
}
