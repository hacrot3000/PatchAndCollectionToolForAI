package server

import (
	"context"
	"encoding/json"
 "fmt"
 "net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
 "time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentRunCollectionPendingRequiresSingleUnresolvedExactCall(t *testing.T) {
	collect := agentchat.HistoryItem{Type: "function_call", CallID: "collect-1", Name: "taskdeck_collect_workspace"}
	patch := agentchat.HistoryItem{Type: "function_call", CallID: "patch-1", Name: "taskdeck_submit_patch"}
	output := agentchat.HistoryItem{Type: "function_call_output", CallID: collect.CallID}
	for _, tc := range []struct{
		name string
		history []agentchat.HistoryItem
		want bool
	}{
		{"single", []agentchat.HistoryItem{collect}, true},
		{"already reconciled", []agentchat.HistoryItem{collect,output}, false},
		{"mixed patch", []agentchat.HistoryItem{collect,patch}, false},
		{"patch only", []agentchat.HistoryItem{patch}, false},
	}{
		t.Run(tc.name,func(t *testing.T){
			got,ok:=pendingRunCollection(tc.history)
			if ok!=tc.want || ok && got.CallID!=collect.CallID {t.Fatalf("unexpected pending call: %+v %v",got,ok)}
		})
	}
}

func enableRunAutomationFixture(t *testing.T, s *Server, spec agentchat.RunSpec, collection, continuation bool) (agentchat.RunSpec, []agentchat.HistoryItem) {
 t.Helper()
 store, err := s.agentChatStore()
 if err != nil { t.Fatal(err) }
 x, err := store.Get(spec.ConversationID,"local")
 if err != nil { t.Fatal(err) }
 x.AutoCollection = &collection
 x.AutoContinueText = continuation
 if _, err = store.Save(x,x.Revision); err != nil { t.Fatal(err) }
 snapshots,err := s.agentContextSnapshotStore("local")
 if err != nil { t.Fatal(err) }
 old,err := snapshots.Get(spec.ContextSnapshotID,spec.OwnerWorkspaceID,spec.ConversationID,x.ProjectID)
 if err != nil { t.Fatal(err) }
 policy:=*old.Policy
 policy.Automation.AutoCollection=&collection
 policy.Automation.AutoContinueText=continuation
 snap,err:=snapshots.CreateWithPolicy(spec.OwnerWorkspaceID,spec.ConversationID,x.ProjectID,old.History,old.SourceRefs,&policy)
 if err!=nil {t.Fatal(err)}
 spec.ContextSnapshotID=snap.ID
 return spec,snap.History
}

func TestAgentRunCollectionContinuesExactProviderWithNativeObservation(t *testing.T) {
 s,_,_,spec:=providerToolRunFixture(t,"run-native-collect")
 spec,history:=enableRunAutomationFixture(t,s,spec,true,false)
 var requests,reads atomic.Int32
 payloads:=make(chan string,2)
 s.AgentCollectionCall=func(workspace string, _ patchtool.ProjectIdentity, _ []byte) string {
  reads.Add(1)
  if workspace!=s.Workspace { return `{"error":"unexpected execution workspace"}` }
  return `{"source":"native_patch_tool_collect","status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"native_session_id":"fixture-native","run_id":"fixture-read","text_excerpt":"fixture-collection-marker"}`
 }
 s.AgentChatCall=func(_ context.Context,p agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  if p.ID!=spec.ProviderID { t.Errorf("provider drift: %s",p.ID) }
  count:=requests.Add(1)
  if count<=2 { payloads<-string(payload) }
  if count==1 {return agentchat.ResponsesResult{ResponseID:"response-1",Calls:[]agentchat.FunctionCall{{CallID:"collect-exact-1",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"main.go"}]}`}}},nil}
  return agentchat.ResponsesResult{ResponseID:"response-2",Text:"Task complete: all work done."},nil
 }
 manager,err:=s.agentRunManager("local")
 if err!=nil {t.Fatal(err)}
 defer manager.Close()
 if _,err=manager.Submit(spec,history);err!=nil{t.Fatal(err)}
 waitForAgentRunStatus(t,manager,spec.RunID,agentrun.Completed)
 record,_:=manager.Get(spec.RunID)
 if reads.Load()!=1||requests.Load()!=2||record.Attempt!=2 {t.Fatalf("reads=%d requests=%d attempt=%d status=%s",reads.Load(),requests.Load(),record.Attempt,record.Status)}
 first,second:=<-payloads,<-payloads
 if !strings.Contains(first,"taskdeck_collect_workspace")||!strings.Contains(second,"collect-exact-1")||!strings.Contains(second,"fixture-collection-marker"){t.Fatal("native Collection tool/result missing from actual provider payload")}
 last:=record.History[len(record.History)-1]
 if last.CallID!="collect-exact-1"||last.OriginRunID!=spec.RunID||last.ProviderID!=spec.ProviderID||last.ModelID!=spec.ModelID {t.Fatalf("wrong native result lineage: %+v",last)}
}

func TestAgentRunContinuationKeepsPinnedProviderAndStopsOnClaim(t *testing.T) {
 s,_,_,spec:=providerToolRunFixture(t,"run-auto-continue")
 spec,history:=enableRunAutomationFixture(t,s,spec,false,true)
 var requests atomic.Int32
 payloads:=make(chan string,2)
 s.AgentChatCall=func(_ context.Context,p agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  if p.ID!=spec.ProviderID {t.Errorf("provider drift: %s",p.ID)}
  var request struct {Model string `json:"model"`}
  if json.Unmarshal(payload,&request)!=nil||request.Model!=spec.ModelID {t.Errorf("model drift: %s",request.Model)}
  count:=requests.Add(1)
  if count<=2 {payloads<-string(payload)}
  if count==1 {return agentchat.ResponsesResult{ResponseID:"response-1",Text:"Audit chưa hoàn tất. Bước tiếp theo: kiểm tra nguồn."},nil}
  return agentchat.ResponsesResult{ResponseID:"response-2",Text:"Task complete: all work done."},nil
 }
 manager,err:=s.agentRunManager("local")
 if err!=nil {t.Fatal(err)}
 defer manager.Close()
 if _,err=manager.Submit(spec,history);err!=nil {t.Fatal(err)}
 waitForAgentRunStatus(t,manager,spec.RunID,agentrun.Completed)
 record,_:=manager.Get(spec.RunID)
 if requests.Load()!=2||record.Attempt!=2||record.Spec.ProviderID!=spec.ProviderID||record.Continuation==nil||record.Continuation.Disposition!=agentchat.TurnClaimedComplete {t.Fatalf("unexpected auto continuation: requests=%d record=%+v",requests.Load(),record)}
 <-payloads
 if second:=<-payloads;!strings.Contains(second,"ORIGINAL unfinished conversation"){t.Fatal("continuation did not retain original task intent")}
}

func TestAgentRunContinuationConsentAndUnknownEgressFence(t *testing.T){
 x:=agentchat.Session{Owner:"local",Status:"completed",AutoContinueText:true}
 policy:=agentrun.AutomationSnapshot{AutoContinueText:true}
 if runAutoContinueMode(x,policy)!="conversation"{t.Fatal("valid pinned and live consent rejected")}
 x.AutoContinueText=false
 if runAutoContinueMode(x,policy)!="" {t.Fatal("owner revocation ignored")}
 x.AutoContinueText=true
 x.ProviderRun=&agentchat.ProviderRun{Phase:"unknown"}
 if runAutoContinueMode(x,policy)!="" {t.Fatal("ambiguous legacy provider outcome bypassed")}
}

func TestAgentRunContinuationNoWorkBrake(t *testing.T) {
 s,_,_,spec:=providerToolRunFixture(t,"run-no-work-brake")
 spec,history:=enableRunAutomationFixture(t,s,spec,false,true)
 var requests atomic.Int32
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  requests.Add(1)
  return agentchat.ResponsesResult{ResponseID:"repeated",Text:"Audit chưa hoàn tất. Bước tiếp theo: kiểm tra nguồn."},nil
 }
 manager,err:=s.agentRunManager("local")
 if err!=nil {t.Fatal(err)}
 defer manager.Close()
 if _,err=manager.Submit(spec,history);err!=nil{t.Fatal(err)}
 waitForAgentRunStatus(t,manager,spec.RunID,agentrun.Completed)
 record,_:=manager.Get(spec.RunID)
 if requests.Load()!=int32(agentchat.MaxContinuationNoWorkTurns+1)||record.Continuation==nil||record.Continuation.Reason!="stalled_without_verified_work"{t.Fatalf("no-work brake not applied: %d %+v",requests.Load(),record.Continuation)}
}

func TestAgentRunUnsupportedToolDenialContinuesPinnedProvider(t *testing.T){
 s,_,_,spec:=providerToolRunFixture(t,"run-denied-shell")
 spec,history:=enableRunAutomationFixture(t,s,spec,false,false)
 var requests,reads atomic.Int32
 payloads:=make(chan string,2)
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{
  reads.Add(1);return `{"error":"unexpected read"}`
 }
 s.AgentChatCall=func(_ context.Context,p agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  if p.ID!=spec.ProviderID{t.Errorf("wrong provider %s",p.ID)}
  count:=requests.Add(1)
  if count<=2{payloads<-string(payload)}
  if count==1{return agentchat.ResponsesResult{ResponseID:"denied-response",Calls:[]agentchat.FunctionCall{
   {CallID:"denied-shell",Name:"shell",Arguments:`{"command":"whoami"}`},
  }},nil}
  return agentchat.ResponsesResult{ResponseID:"corrected-response",Text:"No command executed; continuing safely."},nil
 }
 manager,err:=s.agentRunManager("local");if err!=nil{t.Fatal(err)}
 defer manager.Close()
 if _,err:=manager.Submit(spec,history);err!=nil{t.Fatal(err)}
 waitForAgentRunStatus(t,manager,spec.RunID,agentrun.Completed)
 record,_:=manager.Get(spec.RunID)
 if requests.Load()!=2||reads.Load()!=0||record.Attempt!=2{t.Fatalf("requests=%d reads=%d attempt=%d",requests.Load(),reads.Load(),record.Attempt)}
 <-payloads
 second:=<-payloads
 if !strings.Contains(second,"UNSUPPORTED_TOOL")||!strings.Contains(second,`"call_id":"denied-shell"`){t.Fatal("denial absent from actual continuation payload")}
 last:=record.History[len(record.History)-1]
 if last.Type!="function_call_output"||last.CallID!="denied-shell"||last.OriginRunID!=spec.RunID||!agentRepairableDenialOutput(last.Content){
  t.Fatalf("wrong run denial evidence: %+v",last)
 }
}


func TestAgentRunCollectionRepeatedShapeReadsChangedSource(t *testing.T){
 s,_,_,spec:=providerToolRunFixture(t,"run-native-repeated")
 spec,history:=enableRunAutomationFixture(t,s,spec,true,false)
 // Pin a deliberately different advisory threshold to the actual Run
 // snapshot. The existing default requires 3 duplicates, this requires 1.
 journal,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 session,err:=journal.Get(spec.ConversationID,"local");if err!=nil{t.Fatal(err)}
 snapshots,err:=s.agentContextSnapshotStore("local");if err!=nil{t.Fatal(err)}
 initial,err:=snapshots.Get(spec.ContextSnapshotID,spec.OwnerWorkspaceID,spec.ConversationID,session.ProjectID);if err!=nil{t.Fatal(err)}
 policy:=*initial.Policy
 policy.CollectionGuardPolicy=agentrun.NewCollectionGuardPolicySnapshot()
 policy.CollectionGuardPolicy.Thresholds.DuplicateStreak=1
 pinned,err:=snapshots.CreateWithPolicy(spec.OwnerWorkspaceID,spec.ConversationID,session.ProjectID,history,initial.SourceRefs,&policy);if err!=nil{t.Fatal(err)}
 spec.ContextSnapshotID=pinned.ID
 policy.CollectionGuardPolicy.Thresholds.DuplicateStreak=9 // caller mutation must not alter active run
 var requests,reads atomic.Int32
 s.AgentCollectionCall=func(_ string,identity patchtool.ProjectIdentity,_ []byte)string{
  n:=reads.Add(1);digest:=strings.Repeat("a",64);if n>=3{digest=strings.Repeat("b",64)}
  raw,_:=json.Marshal(map[string]any{"source":"native_patch_tool_collect","project":identity,"status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"native_session_id":"native","run_id":"read-"+string(rune('0'+n)),"text_excerpt":"read-marker-"+string(rune('0'+n)),"text_excerpt_truncated":false,"native_full_file_reads":[]any{map[string]any{"path":"main.go","action_index":1,"total_lines":1,"report_sha256":digest}}})
  return string(raw)
 }
 s.AgentChatCall=func(_ context.Context,p agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  n:=requests.Add(1)
  if p.ID!=spec.ProviderID{t.Errorf("provider drift")}
  if n>1&&!strings.Contains(string(payload),"read-marker-"+string(rune('0'+n-1))){t.Errorf("current native receipt missing at request %d",n)}
  if n==3&&!strings.Contains(string(payload),"repeated_verified_evidence"){t.Errorf("pinned duplicate threshold was not used")}
  if n!=3&&strings.Contains(string(payload),"Collection advisory metadata (not new evidence or authorization):"){t.Errorf("unexpected duplicate advisory at request %d",n)}
  if n<=4{return agentchat.ResponsesResult{ResponseID:"read-response",Calls:[]agentchat.FunctionCall{{CallID:"collect-"+string(rune('0'+n)),Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"main.go"}]}`}}},nil}
  return agentchat.ResponsesResult{ResponseID:"complete",Text:"Read completed."},nil
 }
 manager,err:=s.agentRunManager("local");if err!=nil{t.Fatal(err)}
 defer manager.Close()
 if _,err=manager.Submit(spec,history);err!=nil{t.Fatal(err)}
 waitForAgentRunStatus(t,manager,spec.RunID,agentrun.Completed)
 if reads.Load()!=4||requests.Load()!=5{t.Fatalf("reads=%d requests=%d",reads.Load(),requests.Load())}
 rec,_:=manager.Get(spec.RunID)
 n:=0
 for _,it:=range rec.History{
  if it.Type=="function_call_output"&&strings.HasPrefix(it.CallID,"collect-"){
   n++;if !strings.Contains(it.Content,"read-marker-"+string(rune('0'+n))){t.Fatal("receipt replaced")}
  }
 }
 if n!=4{t.Fatalf("receipt count %d",n)}
 inputs,err:=s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(context.Background(),"local"),rec.Spec);if err!=nil{t.Fatal(err)}
 if threshold,ok:=inputs.Snapshot.Policy.CollectionPolicy();!ok||threshold.DuplicateStreak!=1{t.Fatal("derived Run snapshot lost immutable threshold")}
 g,ok:=readRunCollectionProgress("local",rec.Spec,inputs)
 if !ok||g.Counters.Requested!=4||g.Counters.NativeReads!=4||g.Counters.NewEvidence!=2||g.Counters.DuplicateEvidence!=2{t.Fatalf("guard not retained: %+v",g)}
 if _,ok:=readRunCollectionProgress("foreign",rec.Spec,inputs);ok{t.Fatal("cross-owner progress accepted")}
 foreign:=rec.Spec;foreign.ExecutionWorkspaceID="foreign-worktree"
 if _,ok:=readRunCollectionProgress("local",foreign,inputs);ok{t.Fatal("cross-worktree progress accepted")}
 // Read-only diagnostic report sees the actual committed snapshot, not the
 // in-flight tool result or private request/receipt bytes.
 w:=httptest.NewRecorder()
 s.agentChatDebugReportAPI(w,httptest.NewRequest("GET","/api/agent/debug/report",nil))
 if w.Code!=200||!strings.Contains(w.Body.String(),`"verified_duplicate": 2`)||!strings.Contains(w.Body.String(),`"verified_new": 2`)||!strings.Contains(w.Body.String(),spec.RunID)||strings.Contains(w.Body.String(),"read-marker-"){t.Fatalf("Unified debug projection not safe or incomplete: %d %s",w.Code,w.Body)}

 // A fresh Server reads only the persisted Unified record and snapshot.
 // It must not instantiate workers or re-send a provider request.
 reopened:=&Server{Workspace:s.Workspace,Config:s.Config,ConnectionSecrets:s.ConnectionSecrets}
 report:=func(server *Server)string{
  t.Helper()
  w:=httptest.NewRecorder()
  server.agentChatDebugReportAPI(w,httptest.NewRequest("GET","/api/agent/debug/report",nil))
  if w.Code!=200{t.Fatalf("reopened report HTTP %d: %s",w.Code,w.Body)}
  return w.Body.String()
 }
 body:=report(reopened)
 if !strings.Contains(body,spec.RunID)||!strings.Contains(body,`"verified_duplicate": 2`)||strings.Contains(body,"read-marker-"){t.Fatal("restart lost or disclosed native evidence")}
 journal,err=reopened.agentChatStore();if err!=nil{t.Fatal(err)}
 sessions,err:=journal.List("local");if err!=nil{t.Fatal(err)}
 foreignRows,err:=reopened.agentDebugUnifiedRuns("foreign",sessions,time.Time{})
 if err!=nil||len(foreignRows)!=0{t.Fatal("foreign owner can read Unified run metadata")}
 clear:=httptest.NewRecorder()
 reopened.agentChatDebugClearAPI(clear,httptest.NewRequest("POST","/api/agent/debug/clear",strings.NewReader("{}")))
 if clear.Code!=200{t.Fatalf("clear HTTP %d: %s",clear.Code,clear.Body)}
 afterClear:=&Server{Workspace:s.Workspace,Config:s.Config,ConnectionSecrets:s.ConnectionSecrets}
 if body=report(afterClear);strings.Contains(body,spec.RunID)||strings.Contains(body,"read-marker-"){t.Fatal("old Unified run resurfaced after clear/restart")}
 records,err:=afterClear.agentRunRecords("local");if err!=nil{t.Fatal(err)}
 retained:=false
 for _,r:=range records{if r.Spec.RunID==spec.RunID{
  retained=r.Status==agentrun.Completed&&len(r.History)==len(rec.History)
  for i,item:=range r.History{if i>=len(rec.History)||item!=rec.History[i]{t.Fatal("clear changed durable Unified receipt")}}
 }}
 if !retained||reads.Load()!=4||requests.Load()!=5||len(reopened.agentRunManagers)!=0||len(afterClear.agentRunManagers)!=0{t.Fatal("diagnostic read/clear scheduled work or lost journal")}
}




 // Invalid advisory configuration is a compatibility condition, not a new
 // authorization or execution gate. Keep the actual native receipt pipeline.
func TestAgentRunInvalidCollectionPolicyDoesNotBlockNativeRead(t *testing.T){
 for _,version:=range []int{1,999}{
  t.Run(map[int]string{1:"invalid_thresholds",999:"future_version"}[version],func(t *testing.T){
   s,_,_,spec:=providerToolRunFixture(t,"run-invalid-clp-policy")
   spec,history:=enableRunAutomationFixture(t,s,spec,true,false)
   journal,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   x,err:=journal.Get(spec.ConversationID,"local");if err!=nil{t.Fatal(err)}
   snapshots,err:=s.agentContextSnapshotStore("local");if err!=nil{t.Fatal(err)}
   initial,err:=snapshots.Get(spec.ContextSnapshotID,spec.OwnerWorkspaceID,spec.ConversationID,x.ProjectID);if err!=nil{t.Fatal(err)}
   policy:=*initial.Policy
   policy.CollectionGuardPolicy=agentrun.NewCollectionGuardPolicySnapshot()
   policy.CollectionGuardPolicy.Version=version
   if version==1{policy.CollectionGuardPolicy.Thresholds.DuplicateStreak=0}
   pinned,err:=snapshots.CreateWithPolicy(spec.OwnerWorkspaceID,spec.ConversationID,x.ProjectID,history,initial.SourceRefs,&policy);if err!=nil{t.Fatal(err)}
   spec.ContextSnapshotID=pinned.ID
   var reads,calls atomic.Int32
   receipts:=make(chan string,2)
   s.AgentCollectionCall=func(_ string,identity patchtool.ProjectIdentity,_ []byte)string{
    n:=reads.Add(1)
    raw,_:=json.Marshal(map[string]any{"source":"native_patch_tool_collect","project":identity,"native_session_id":"fixture","run_id":"invalid-policy-"+string(rune('0'+n)),"status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"text_excerpt_truncated":false,"native_full_file_reads":[]map[string]any{{"path":"main.go","action_index":1,"total_lines":1,"report_sha256":strings.Repeat("a",64)}}})
    receipts<-string(raw);return string(raw)
   }
   s.AgentChatCall=func(_ context.Context,p agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
    n:=calls.Add(1)
    if p.ID!=spec.ProviderID{t.Errorf("provider drift")}
    if strings.Contains(string(payload),"Collection advisory metadata (not new evidence or authorization):"){t.Errorf("invalid policy emitted advisory")}
    if n>1{
     want:=<-receipts
     var wire struct{Input []map[string]any}
     if json.Unmarshal(payload,&wire)!=nil{t.Errorf("bad payload")}
     count:=0;for _,item:=range wire.Input{if item["output"]==want{count++}}
     if count!=1{t.Errorf("receipt missing or duplicated: %d",count)}
    }
    if n<=2{return agentchat.ResponsesResult{ResponseID:"read",Calls:[]agentchat.FunctionCall{{CallID:"invalid-policy-read-"+string(rune('0'+n)),Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"main.go"}]}`}}},nil}
    return agentchat.ResponsesResult{ResponseID:"done",Text:"Inspected source."},nil
   }
   manager,err:=s.agentRunManager("local");if err!=nil{t.Fatal(err)}
   defer manager.Close()
   if _,err=manager.Submit(spec,history);err!=nil{t.Fatal(err)}
   waitForAgentRunStatus(t,manager,spec.RunID,agentrun.Completed)
   rec,_:=manager.Get(spec.RunID)
   if reads.Load()!=2||calls.Load()!=3{t.Fatalf("invalid policy blocked read or caused replay: %d/%d",reads.Load(),calls.Load())}
   inputs,err:=s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(context.Background(),"local"),rec.Spec);if err!=nil{t.Fatal(err)}
   if _,ok:=readRunCollectionProgress("local",rec.Spec,inputs);ok||len(inputs.Snapshot.Policy.CollectionProgress)!=0{t.Fatal("invalid policy enabled guard")}
   if inputs.Snapshot.Policy.CollectionGuardPolicy==nil||*inputs.Snapshot.Policy.CollectionGuardPolicy!=*pinned.Policy.CollectionGuardPolicy{t.Fatal("invalid/future policy was rewritten")}
  })
 }
}

func TestAgentRunCollectionProjectionRequiresExactPendingCallAcrossSnapshots(t *testing.T){
 s,_,_,spec:=providerToolRunFixture(t,"run-clp-exact-projection")
 spec,history:=enableRunAutomationFixture(t,s,spec,true,false)
 ctx:=agentrun.WithPrincipalOwner(context.Background(),"local")
 inputs,err:=s.ResolveAgentRunInputs(ctx,spec);if err!=nil{t.Fatal(err)}
 identity,err:=patchtool.ReadProjectIdentity(inputs.ExecutionWorkspacePath);if err!=nil{t.Fatal(err)}
 call:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"logical-read",Arguments:`{"actions":[{"type":"file","path":"main.go"}]}`,OriginRunID:spec.RunID,ProviderID:spec.ProviderID,ModelID:spec.ModelID,ParentTurnID:spec.ParentTurnID}
 request,err:=agentNativeCollectionRequest(call.Arguments,identity,false);if err!=nil{t.Fatal(err)}
 raw,err:=json.Marshal(map[string]any{"source":"native_patch_tool_collect","project":identity,"status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"native_session_id":"native","run_id":"logical-receipt","text_excerpt_truncated":false,"native_full_file_reads":[]any{map[string]any{"path":"main.go","action_index":1,"total_lines":1,"report_sha256":strings.Repeat("a",64)}}});if err!=nil{t.Fatal(err)}
 run:=agentrun.Record{Spec:spec,History:append(append([]agentchat.HistoryItem(nil),history...),call)}
 // Different call/owner/worktree/provenance cannot mint a progress event.
 wrong:=call;wrong.CallID="another-call"
 if got:=runCollectionProgressUpdate("local",run,inputs,identity,wrong,request,string(raw),true);len(got)!=0{t.Fatal("foreign call counted")}
 wrong=call;wrong.ProviderID="other-provider"
 foreign:=run;foreign.History=append(append([]agentchat.HistoryItem(nil),history...),wrong)
 if got:=runCollectionProgressUpdate("local",foreign,inputs,identity,wrong,request,string(raw),true);len(got)!=0{t.Fatal("foreign provider counted")}
 // First projection is deterministic even if an uncommitted snapshot is
 // staged twice. It is not a second native execution or observation.
 projected:=runCollectionProgressUpdate("local",run,inputs,identity,call,request,string(raw),true)
 again:=runCollectionProgressUpdate("local",run,inputs,identity,call,request,string(raw),true)
 if len(projected)==0||string(again)!=string(projected){t.Fatal("projection not deterministic")}
 var g runCollectionProgress
 if json.Unmarshal(projected,&g)!=nil||g.Counters.Requested!=1||g.Counters.Observed!=1||g.Counters.NativeReads!=1||g.Counters.NewEvidence!=1||g.Decision.Kind!=cp.Continue{t.Fatalf("unexpected counters: %+v",g)}
 result:=agentchat.HistoryItem{Type:"function_call_output",CallID:call.CallID,Content:string(raw),OriginRunID:spec.RunID,ProviderID:spec.ProviderID,ModelID:spec.ModelID,ParentTurnID:spec.ParentTurnID}
 committed:=append(append([]agentchat.HistoryItem(nil),run.History...),result)
 snapshots,err:=s.agentContextSnapshotStore("local");if err!=nil{t.Fatal(err)}
 policy:=*inputs.Snapshot.Policy;policy.CollectionProgress=projected
 snap,err:=snapshots.CreateWithPolicy(spec.OwnerWorkspaceID,spec.ConversationID,inputs.ProjectID,committed,inputs.Snapshot.SourceRefs,&policy);if err!=nil{t.Fatal(err)}
 reopened,err:=snapshots.Get(snap.ID,spec.OwnerWorkspaceID,spec.ConversationID,inputs.ProjectID);if err!=nil{t.Fatal(err)}
 derived:=inputs;derived.Snapshot=reopened
 run.Spec.ContextSnapshotID=snap.ID
 run.History=committed
 if got:=runCollectionProgressUpdate("local",run,derived,identity,call,request,string(raw),true);string(got)!=string(projected){t.Fatal("resolved observation changed counters")}
 // Old snapshot prefix cannot be used to attribute a second count to a
 // completed call even if history has subsequently grown.
 run.History=append(run.History,agentchat.HistoryItem{Type:"assistant",Content:"Read complete."})
 if got:=runCollectionProgressUpdate("local",run,derived,identity,call,request,string(raw),true);string(got)!=string(projected){t.Fatal("stale observation counted again")}
 got,ok:=readRunCollectionProgress("local",run.Spec,derived)
 if !ok||got.Counters!=g.Counters{t.Fatal("committed counters changed on reopen")}
 // Advance two actual immutable projections for the same request, first
 // an exact duplicate with new receipt ID, then a changed source revision.
 session:=agentchat.Session{}
 firstSignature:=runContinuationWorkSignature("local",run,derived,session)
 run.History=committed
 for i:=0;i<2;i++{
  nextCall:=call
  nextCall.CallID=fmt.Sprintf("followup-%d",i)
  nextRaw:=strings.ReplaceAll(string(raw),"logical-receipt",fmt.Sprintf("native-followup-%d",i))
  if i==1{nextRaw=strings.ReplaceAll(nextRaw,strings.Repeat("a",64),strings.Repeat("b",64))}
  run.History=append(append([]agentchat.HistoryItem(nil),run.History...),nextCall)
  nextProgress:=runCollectionProgressUpdate("local",run,derived,identity,nextCall,request,nextRaw,true)
  var state runCollectionProgress
  if json.Unmarshal(nextProgress,&state)!=nil||state.Counters.NativeReads!=uint64(i+2)||state.Counters.DuplicateEvidence!=1||state.Counters.NewEvidence!=uint64(i+1){t.Fatalf("bad continuation novelty: %+v",state.Counters)}
  nextResult:=result;nextResult.CallID=nextCall.CallID;nextResult.Content=nextRaw
  run.History=append(run.History,nextResult)
  nextPolicy:=*derived.Snapshot.Policy;nextPolicy.CollectionProgress=nextProgress
  nextSnap,err:=snapshots.CreateWithPolicy(spec.OwnerWorkspaceID,spec.ConversationID,inputs.ProjectID,run.History,inputs.Snapshot.SourceRefs,&nextPolicy);if err!=nil{t.Fatal(err)}
  derived.Snapshot=nextSnap;run.Spec.ContextSnapshotID=nextSnap.ID
  signature:=runContinuationWorkSignature("local",run,derived,session)
  if (signature==firstSignature)!=(i==0){t.Fatalf("outer progress signature changed incorrectly for observation %d",i)}
 }
}
