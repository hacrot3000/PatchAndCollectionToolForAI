package server

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"
 "testing"
 "time"

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
  if calls>1&&calls<=16{
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
 if w.Code!=200||reads!=15||calls!=16||replans!=1{t.Fatalf("no-plan continuation stopped at duplicate evidence: HTTP=%d reads=%d calls=%d replans=%d %s",w.Code,reads,calls,replans,w.Body)}
 saved,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 g,ok:=saved.ReadCollectionGuard()
 if !ok||g.Recovery.Attempts!=1||g.Counters.NewEvidence!=2||g.Counters.DuplicateEvidence!=13||g.Counters.Progress.DuplicateStreak!=0||!saved.AutoContinueText{t.Fatalf("incorrect counters, recovery reservation or consent: %+v",g)}
 count:=0
 for _,it:=range saved.History{if it.Type=="function_call_output"{count++;if receipts[it.CallID]!=it.Content{t.Fatal("canonical receipt mutated")}}}
 if count!=15||len(saved.ToolCalls)!=0{t.Fatal("lost receipt or invented Patch")}
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


func TestAgentCLP20RenamedReadIdentityRetainsVerifiedDuplicateAndTargetsMissingScope(t *testing.T) {
 s, id := newAgentConversationFixture(t, "stateless")
 reads, calls, warnings := 0, 0, 0
 receipts := map[string]string{}
 s.AgentCollectionCall = func(_ string, identity patchtool.ProjectIdentity, _ []byte) string {
  reads++
  path := "src/a.go"
  if reads == 5 { path = "src/b.go" }
  raw, err := json.Marshal(map[string]any{
   "source": "native_patch_tool_collect", "project": identity,
   "native_session_id": "synthetic-native", "run_id": fmt.Sprintf("renamed-native-%d", reads),
   "request_name": fmt.Sprintf("renamed-title-%d", reads),
   "status": "PASS", "native_status": "PASS", "rc": 0,
   "evidence_verified": true, "text_excerpt_truncated": false,
   "native_full_file_reads": []map[string]any{{"path": path, "action_index": 1, "total_lines": 10, "report_sha256": strings.Repeat("a", 64)}},
  })
  if err != nil { t.Fatal(err) }
  receipts[fmt.Sprintf("renamed-call-%d", reads)] = string(raw)
  return string(raw)
 }
 s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
  calls++
  var wire struct {
   Input []map[string]any `json:"input"`
   Instructions string `json:"instructions"`
  }
  if err := json.Unmarshal(payload, &wire); err != nil { t.Fatal(err) }
  if calls > 1 {
   callID := fmt.Sprintf("renamed-call-%d", calls-1)
   n := 0
   for _, item := range wire.Input {
    if item["type"] == "function_call_output" && item["call_id"] == callID {
     n++
     if item["output"] != receipts[callID] { t.Fatal("current native receipt changed") }
    }
   }
   if n != 1 { t.Fatalf("current receipt count %d at call %d", n, calls) }
  }
  warning := strings.Contains(wire.Instructions, "verified repeated evidence was observed")
  if warning != (calls == 5) { t.Fatalf("unexpected verified warning at provider call %d", calls) }
  if warning {
   warnings++
   if !strings.Contains(wire.Instructions, "Use the already delivered receipt") ||
    !strings.Contains(wire.Instructions, "Re-read when freshness or completeness is uncertain") ||
    !strings.Contains(wire.Instructions, "does not authorize skipping a native read") {
    t.Fatal("missing safe evidence reuse guidance")
   }
  }
  if calls == 6 { return agentchat.ResponsesResult{ResponseID: "complete", Text: "Inspected both scopes without changing source."}, nil }
  if calls > 6 { t.Fatalf("unexpected call %d", calls) }
  path := "src/a.go"
  if calls == 5 { path = "src/b.go" }
  args := fmt.Sprintf(`{"id":"read-%d","title":"Different title %d","actions":[{"type":"file","path":%q}]}`, calls, calls, path)
  return agentchat.ResponsesResult{ResponseID: fmt.Sprintf("reply-%d", calls), Calls: []agentchat.FunctionCall{{CallID: fmt.Sprintf("renamed-call-%d", calls), Name: "taskdeck_collect_workspace", Arguments: args}}}, nil
 }
 w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages", map[string]any{"message": "Audit this source read-only", "confirm_external": true, "expected_revision": 1})
 if w.Code != 200 || reads != 5 || calls != 6 || warnings != 1 {
  t.Fatalf("HTTP=%d native reads=%d calls=%d warnings=%d body=%s", w.Code, reads, calls, warnings, w.Body)
 }
 store, err := s.agentChatStore()
 if err != nil { t.Fatal(err) }
 x, err := store.Get(id, "local")
 if err != nil { t.Fatal(err) }
 guard, ok := x.ReadCollectionGuard()
 if !ok || guard.Counters.NativeReads != 5 || guard.Counters.NewEvidence != 2 ||
  guard.Counters.DuplicateEvidence != 3 || guard.Counters.UnknownEvidence != 0 ||
  guard.Counters.Progress.DuplicateStreak != 0 || len(x.ToolCalls) != 0 {
  t.Fatalf("unexpected guard state or Patch: %+v", guard)
 }
 outputs := 0
 for _, item := range x.History {
  if item.Type == "function_call_output" {
   outputs++
   if receipts[item.CallID] != item.Content { t.Fatal("canonical receipt changed") }
  }
 }
 if outputs != 5 { t.Fatalf("canonical receipt count %d", outputs) }
}

func TestAgentCLP20ObjectiveSurvivesProviderSwitchAndStop(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 second:=agentProviderFixture()
 second.ID="custom-objective-switch"
 second.Name="Second test provider"
 second.Models[0].ID="second-model"
 registry,err:=s.agentChatProviders();if err!=nil{t.Fatal(err)}
 if err=registry.Put("local",second);err!=nil{t.Fatal(err)}
 if err=s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace,"local",second.ID),[]byte("SECOND-TEST-KEY"));err!=nil{t.Fatal(err)}
 caps,err:=s.agentChatCapabilities();if err!=nil{t.Fatal(err)}
 if err=caps.Put("local",agentchat.CapabilityRecord{ProviderID:second.ID,ModelID:second.Models[0].ID,
  Fingerprint:agentchat.CapabilityFingerprint(second,second.Models[0]),Version:agentchat.CapabilityProbeVersion,
  Capabilities:map[string]string{agentchat.ProbeToolCalling:agentchat.CapabilityVerified},ProbedAt:time.Now().UTC()});err!=nil{t.Fatal(err)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 calls,reads:=0,0
 s.AgentCollectionCall=func(_ string,identity patchtool.ProjectIdentity,_ []byte)string{
  reads++
  raw,_:=json.Marshal(map[string]any{"source":"native_patch_tool_collect","project":identity,
   "native_session_id":"synthetic","run_id":fmt.Sprintf("objective-read-%d",reads),
   "status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"text_excerpt_truncated":false,
   "native_full_file_reads":[]map[string]any{{"path":"src/a.go","action_index":1,"total_lines":10,"report_sha256":strings.Repeat("a",64)}},
  })
  return string(raw)
 }
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls<=2 {
   if p.ID==second.ID||key!="TEST-KEY"{t.Fatal("first provider changed")}
  }else if p.ID!=second.ID||key!="SECOND-TEST-KEY"{t.Fatal("new provider not pinned")}
  if calls==5{
   if !cancelAgentRequest(s.Workspace,"local",id){t.Fatal("Stop failed to cancel registered request")}
   <-ctx.Done()
   return agentchat.ResponsesResult{},ctx.Err()
  }
  if calls==1||calls==3||calls==6{
   return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("reply-%d",calls),Calls:[]agentchat.FunctionCall{{CallID:fmt.Sprintf("objective-call-%d",calls),Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"src/a.go"}]}`}}},nil
  }
  return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("done-%d",calls),Text:"Inspected the source."},nil
 }
 endpoint:="/api/agent/sessions/"+id+"/messages"
 send:=func(message string,selectSecond bool,want int)agentchat.Session{
  t.Helper()
  x,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
  body:=map[string]any{"message":message,"confirm_external":true,"expected_revision":x.Revision}
  if selectSecond{body["provider_id"]=second.ID;body["model_id"]=second.Models[0].ID}
  w:=invokeAI(t,s.agentChatSessionItemAPI,"POST",endpoint,body)
  if w.Code!=want{t.Fatalf("message %q HTTP=%d want %d %s",message,w.Code,want,w.Body)}
  x,e=store.Get(id,"local");if e!=nil{t.Fatal(e)}
  return x
 }
 first:=send("Fix the source",false,200)
 g,ok:=first.ReadCollectionGuard()
 if !ok||g.Counters.NativeReads!=1||g.Counters.NewEvidence!=1||g.Counters.Objective.Intent!=cp.Implementation{t.Fatal("first objective not bound")}
 switched:=send("tiếp tục",true,200)
 after,ok:=switched.ReadCollectionGuard()
 if !ok||after.Generation!=g.Generation||after.Counters.Objective!=g.Counters.Objective||after.Counters.NativeReads!=2||after.Counters.DuplicateEvidence!=1||switched.ProviderID!=second.ID||switched.AutoPatch{t.Fatalf("provider switch reset guard or granted Patch: %+v",after)}
 stopped:=send("Continue",false,409)
 stopGuard,ok:=stopped.ReadCollectionGuard()
 if !ok||stopped.Status!="cancelled"||stopGuard.Generation!=g.Generation||stopGuard.Counters.NativeReads!=2||stopGuard.Counters.Objective!=g.Counters.Objective||reads!=2{t.Fatalf("Stop reset objective: %+v",stopGuard)}
 next:=send("Audit the source read-only",false,200)
 ng,ok:=next.ReadCollectionGuard()
 if !ok||ng.Generation!=g.Generation+1||ng.Counters.Objective.Key==g.Counters.Objective.Key||ng.Counters.Objective.Intent!=cp.ReadOnly||ng.Counters.NativeReads!=1||ng.Counters.NewEvidence!=1||next.AutoPatch||len(next.ToolCalls)!=0||calls!=7||reads!=3{
  t.Fatalf("new objective reused old coverage or permission: %+v calls=%d reads=%d",ng,calls,reads)
 }
 n:=0
 for _,item:=range next.History{if item.Type=="function_call_output"{n++}}
 if n!=3{t.Fatal("objective switch rewrote canonical receipts")}
}
