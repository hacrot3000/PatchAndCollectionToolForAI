package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
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
