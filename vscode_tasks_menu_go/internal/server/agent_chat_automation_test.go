package server

import (
 "context"
 "errors"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentAutomationOptionsPerConversation(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/automation"
 unauthorized:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"patch","enabled":true,"expected_revision":x.Revision,
 })
 if unauthorized.Code!=403 {t.Fatalf("Auto Patch opt-in needs explicit setting authorization: %d",unauthorized.Code)}
 activated:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"patch","enabled":true,"confirm_execution":true,"expected_revision":x.Revision,
 })
 if activated.Code!=200 {t.Fatalf("Auto Patch opt-in failed: %d %s",activated.Code,activated.Body)}
 x,err=store.Get(id,"local")
 if err!=nil||!x.AutoPatch {t.Fatalf("session option was not saved: %+v %v",x,err)}
 disabledCollection:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"collection","enabled":false,"expected_revision":x.Revision,
 })
 if disabledCollection.Code!=200 {t.Fatalf("Collection toggle failed: %d %s",disabledCollection.Code,disabledCollection.Body)}
 x,err=store.Get(id,"local")
 if err!=nil||x.AutoCollection==nil||*x.AutoCollection {t.Fatalf("Collection opt-out not persisted: %+v %v",x.AutoCollection,err)}
 noConsent:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"feedback","enabled":true,"expected_revision":x.Revision,
 })
 if noConsent.Code!=403 {t.Fatalf("provider egress opt-in lacked external authorization: %d",noConsent.Code)}
 enabledFeedback:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "option":"feedback","enabled":true,"confirm_external":true,"expected_revision":x.Revision,
 })
 if enabledFeedback.Code!=200 {t.Fatalf("Auto feedback opt-in failed: %d %s",enabledFeedback.Code,enabledFeedback.Body)}
 x,err=store.Get(id,"local")
 if err!=nil||!x.AutoFeedback||!x.AutoFeedbackPersistent||x.AutoFeedbackUntil!=nil {t.Fatalf("Persistent Auto feedback not saved: %+v %v",x,err)}
 otherID:=createAgentSessionWithSameFixture(t,s)
 other,err:=store.Get(otherID,"local")
 if err!=nil {t.Fatal(err)}
 if other.AutoPatch||other.AutoFeedback||other.AutoCollection!=nil {t.Fatal("automation permissions leaked across conversations")}
}

func createAgentSessionWithSameFixture(t *testing.T,s *Server) string {
 t.Helper()
 p:=agentProviderFixture()
 r:=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]any{
  "mode":"stateless","provider_id":p.ID,"model_id":p.Models[0].ID,
 })
 if r.Code!=201 {t.Fatalf("second session create failed: %d %s",r.Code,r.Body)}
 var data struct{ Session agentchat.Session `json:"session"` }
 if err:=json.Unmarshal(r.Body.Bytes(),&data);err!=nil {t.Fatal(err)}
 return data.Session.ID
}

func TestAgentAutoPatchUsesNativeQueueOnceWithoutPerCallConfirmation(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if call,ok:=agentAutoPatchCandidate(x);ok||call!="" {t.Fatal("Auto Patch must default OFF")}
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local")
 if err!=nil||x.ToolCalls[0].Status!="pending" {t.Fatal("Patch unexpectedly auto-approved before opt-in")}
 enabled:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/automation",map[string]any{
  "option":"patch","enabled":true,"confirm_execution":true,"expected_revision":x.Revision,
 })
 if enabled.Code!=200 {t.Fatalf("Auto Patch enable failed: %d %s",enabled.Code,enabled.Body)}
 s.runAgentAutoFeedback(context.Background())
 x,err=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if x.ToolCalls[0].Status!="queued" || x.ToolCalls[0].QueuePath=="" {
  t.Fatalf("Native Patch was not queued under Auto Patch: %+v",x.ToolCalls[0])
 }
 queued:=filepath.Join(s.Workspace,filepath.FromSlash(x.ToolCalls[0].QueuePath))
 if _,err:=os.Stat(queued);err!=nil {t.Fatal(err)}
 s.runAgentAutoFeedback(context.Background())
 y,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 if y.ToolCalls[0].QueuePath!=x.ToolCalls[0].QueuePath||y.Revision!=x.Revision {
  t.Fatal("Auto Patch was queued twice or changed after its idempotent handoff")
 }
 again:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/tools/"+x.ToolCalls[0].CallID+"/decision",map[string]any{
  "action":"approve","confirm_execution":true,"automatic":true,"expected_revision":y.Revision,
 })
 if again.Code!=409 {t.Fatalf("duplicate automatic Patch execution was allowed: %d",again.Code)}
}

func TestAgentDeleteConversationDoesNotRemoveRunningNativeWork(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 endpoint:="/api/agent/sessions/"+id
 noDelete:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodDelete,endpoint,map[string]any{"expected_revision":x.Revision})
 if noDelete.Code!=409 {t.Fatalf("pending Patch conversation was deleted: %d",noDelete.Code)}
 if _,err:=store.Get(id,"local");err!=nil {t.Fatal("failed delete damaged the session")}
 decision:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,endpoint+"/tools/"+x.ToolCalls[0].CallID+"/decision",map[string]any{
  "action":"reject","expected_revision":x.Revision,
 })
 if decision.Code!=200 {t.Fatalf("reject failed: %d %s",decision.Code,decision.Body)}
 x,err=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 stale:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodDelete,endpoint,map[string]any{"expected_revision":x.Revision-1})
 if stale.Code!=409 {t.Fatalf("stale delete accepted: %d",stale.Code)}
 deleted:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodDelete,endpoint,map[string]any{"expected_revision":x.Revision})
 if deleted.Code!=200||!strings.Contains(deleted.Body.String(),`"deleted":true`) {
  t.Fatalf("conversation delete failed: %d %s",deleted.Code,deleted.Body)
 }
 if _,err:=store.Get(id,"local");!errors.Is(err,agentchat.ErrNotFound) {
  t.Fatalf("deleted conversation remains available: %v",err)
 }
}

func TestAgentAutoCollectionOffBlocksReadWithoutCallingProviderAgain(t *testing.T) {
	s,id:=newAgentConversationFixture(t,"stateless")
	store,err:=s.agentChatStore()
	if err!=nil {t.Fatal(err)}
	x,err:=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	disabled:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
		"/api/agent/sessions/"+id+"/automation",
		map[string]any{"option":"collection","enabled":false,"expected_revision":x.Revision})
	if disabled.Code!=200 {t.Fatalf("disable Collection failed: %d %s",disabled.Code,disabled.Body)}
	x,err=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	count:=0
	s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
		count++
		if count>1 {t.Fatal("Collection continued despite automation OFF")}
		return agentchat.ResponsesResult{ResponseID:"read-disabled",Calls:[]agentchat.FunctionCall{{
			CallID:"collect-off",Name:"taskdeck_collect_workspace",
			Arguments:`{"actions":[{"type":"ls","path":"."}]}`,
		}}},nil
	}
	resp:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
		"/api/agent/sessions/"+id+"/messages",
		map[string]any{"message":"inspect the project files","confirm_external":true,"expected_revision":x.Revision})
	if resp.Code!=200 || count!=1 || !strings.Contains(resp.Body.String(),"Automatic Collection is disabled") {
		t.Fatalf("read-only opt-out not enforced: %d %s calls=%d",resp.Code,resp.Body,count)
	}
	y,err:=store.Get(id,"local")
	if err!=nil||y.Status!="completed"||len(y.ToolCalls)!=0 {
		t.Fatalf("disabled Collection caused tool execution: %+v %v",y,err)
	}
}

func TestAgentPersistentAutoSendDeliversOnceWithoutExpiration(t *testing.T) {
	s,id,_:=agentPendingCallFixture(t,true)
	store,err:=s.agentChatStore()
	if err!=nil {t.Fatal(err)}
	x,err:=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	enabled:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,
		"/api/agent/sessions/"+id+"/automation",
		map[string]any{"option":"feedback","enabled":true,
			"confirm_external":true,"expected_revision":x.Revision})
	if enabled.Code!=200 {t.Fatalf("enable persistent feedback failed: %d %s",enabled.Code,enabled.Body)}
	x,err=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	x.ToolCalls[0].Status="done"
	x.ToolCalls[0].Result=`{"status":"PASS","rc":0}`
	x,err=store.Save(x,x.Revision)
	if err!=nil {t.Fatal(err)}
	count:=0
	s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error) {
		count++
		return agentchat.ResponsesResult{ResponseID:"feedback",Text:"Acknowledged native Patch PASS."},nil
	}
	s.runAgentAutoFeedback(context.Background())
	y,err:=store.Get(id,"local")
	if err!=nil||count!=1||!y.ToolCalls[0].FeedbackSent {
		t.Fatalf("persistent Auto-send failed: %+v %d %v",y.ToolCalls,count,err)
	}
	s.runAgentAutoFeedback(context.Background())
	if count!=1 {t.Fatal("persistent Auto-send retried acknowledged result")}
}

func TestAgentDeleteAfterFinalPatchWithoutSendingAIResult(t *testing.T){
	s,id,_:=agentPendingCallFixture(t,true)
	store,err:=s.agentChatStore()
	if err!=nil {t.Fatal(err)}
	x,err:=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	x.ToolCalls[0].Status="done"
	x.ToolCalls[0].Result=`{"status":"PASS","rc":0}`
	if x.Status!="waiting_tool" {t.Fatalf("fixture status unexpected: %s",x.Status)}
	x,err=store.Save(x,x.Revision)
	if err!=nil {t.Fatal(err)}
	if agentchat.HasUnresolvedWork(x){t.Fatal("verified final native work is not unresolved")}
	list:=invokeAI(t,s.agentChatSessionsAPI,http.MethodGet,"/api/agent/sessions",nil)
	if list.Code!=200||!strings.Contains(list.Body.String(),`"unresolved":false`){
		t.Fatalf("conversation list did not expose final-state deletability: %d %s",list.Code,list.Body)
	}
	r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodDelete,"/api/agent/sessions/"+id,
		map[string]any{"expected_revision":x.Revision})
	if r.Code!=200 {t.Fatalf("finished Patch conversation should be removable: %d %s",r.Code,r.Body)}
}


func TestAgentAutomationAndRetryCanChangeWhileTurnLocked(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 stale,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 stale.Status="requesting_ai"
 stale,err=store.Save(stale,stale.Revision);if err!=nil{t.Fatal(err)}
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{t.Fatal("failed to hold active turn")}
 defer release()
 for _,option:=range []string{"patch","collection","feedback","continue"}{
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/automation",map[string]any{
   "option":option,"enabled":option!="collection","expected_revision":stale.Revision,"confirm_external":true,"confirm_execution":true})
  if w.Code!=200{t.Fatalf("%s rejected during active turn: %d %s",option,w.Code,w.Body)}
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/retry",map[string]any{"mode":"unlimited","expected_revision":stale.Revision,"confirm_external":true})
 if w.Code!=200{t.Fatalf("retry rejected during active turn: %d %s",w.Code,w.Body)}
 current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if current.Revision!=stale.Revision||current.AutomationRevision!=stale.AutomationRevision+5{t.Fatal("metadata invalidated turn CAS")}
 stale.Status="completed"
 stale.History=append(stale.History,agentchat.HistoryItem{Type:"assistant",Content:"completed response"})
 saved,err:=store.Save(stale,stale.Revision);if err!=nil{t.Fatal(err)}
 if !saved.AutoPatch||saved.AutoCollection==nil||*saved.AutoCollection||!saved.AutoFeedback||!saved.AutoFeedbackPersistent||!saved.AutoContinueText||saved.RetryMode!=agentchat.RetryUnlimited||saved.AutomationRevision!=current.AutomationRevision{
  t.Fatal("active turn overwrote newer Automation settings")
 }
 if saved.Status!="completed"||saved.History[len(saved.History)-1].Content!="completed response"{t.Fatal("metadata update lost response")}
}


// A provider callback represents a Thinking request whose turn lease is held.
// Turning Collection OFF must prevent a newly proposed native read without
// cancelling the already completed provider response or corrupting its CAS.
func TestAgentAutomationCollectionRevokedDuringProviderRequest(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 calls,reads:=0,0
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{
  reads++
  return `{"results":[{"type":"ls","entries":[]}]}`
 }
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls!=1{t.Fatalf("unexpected provider call %d",calls)}
  current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/automation",map[string]any{
   "option":"collection","enabled":false,"expected_revision":current.Revision})
  if w.Code!=200{t.Fatalf("toggle while thinking failed: %d %s",w.Code,w.Body)}
  return agentchat.ResponsesResult{ResponseID:"read-after-toggle",Calls:[]agentchat.FunctionCall{{
   CallID:"disabled-read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"ls","path":"."}]}`,
  }}},nil
 }
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",map[string]any{
  "message":"Inspect project files","confirm_external":true,"expected_revision":x.Revision})
 if w.Code!=200||calls!=1||reads!=0{t.Fatalf("revoked read executed or provider result lost: HTTP=%d calls=%d reads=%d body=%s",w.Code,calls,reads,w.Body)}
 done,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if done.AutoCollection==nil||*done.AutoCollection||done.Status!="completed"{t.Fatal("live preference or final response lost")}
}

func TestAgentHistoricalRunDoesNotOwnNewLegacyPatch(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 call:=x.ToolCalls[0]
 // Earlier failed Run Manager response (e.g. after switching models).
 old:=agentrun.Record{Status:agentrun.Failed}
 if !agentLegacyPatchWithRunHistory(x,[]agentrun.Record{old},call.CallID) {
  t.Fatal("historical terminal run incorrectly owns the new legacy proposal")
 }
 old.Status=agentrun.Running
 if agentLegacyPatchWithRunHistory(x,[]agentrun.Record{old},call.CallID) {
  t.Fatal("active provider-bound run must prevent legacy dispatch")
 }
 old.Status=agentrun.Failed
 old.History=[]agentchat.HistoryItem{{Type:"function_call",CallID:"old-unresolved",Name:"taskdeck_submit_patch"}}
 if agentLegacyPatchWithRunHistory(x,[]agentrun.Record{old},call.CallID) {
  t.Fatal("unresolved old run must prevent legacy dispatch")
 }
 old.History=nil
 x.ToolCalls[0].AgentRunID="run-bound"
 if agentLegacyPatchWithRunHistory(x,[]agentrun.Record{old},call.CallID) {
  t.Fatal("run-owned call cannot use the legacy scheduler")
 }
 x.ToolCalls[0].AgentRunID=""
 x.ToolCalls[0].ArgumentsSHA256="wrong"
 if agentLegacyPatchWithRunHistory(x,[]agentrun.Record{old},call.CallID) {
  t.Fatal("mismatched arguments must not be dispatched")
 }
}

func TestAgentAutoPatchAdmissionBlocksActiveProviderOrNativeWork(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 store,err:=s.agentChatStore()
 if err!=nil {t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil {t.Fatal(err)}
 x.AutoPatch=true
 if _,ok:=agentAutoPatchCandidate(x);!ok {t.Fatal("fresh pending Patch must be eligible")}
 x.ProviderRun=&agentchat.ProviderRun{Phase:"unknown"}
 if _,ok:=agentAutoPatchCandidate(x);ok {t.Fatal("ambiguous provider work cannot be bypassed")}
 x.ProviderRun=nil
 x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{CallID:"other",Status:"running"})
 if _,ok:=agentAutoPatchCandidate(x);ok {t.Fatal("later in-flight native record must prevent dispatch")}
}
