package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestAgentDebugReportAutoCaptureAndJournal(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 store:=s.debugStore()
 store.mu.Lock()
 store.owner("local").State.Enabled=false
 store.mu.Unlock()
 recorder,traceID:=s.startAgentDebug("local","provider_request",id,
  agentchat.ProviderConfig{ID:"provider",APIType:"responses"},"model")
 recorder.RecordProviderDiagnostic(agentchat.DiagnosticEvent{
  Level:"error",Stage:"provider_http",Event:"http.error",Code:"PROVIDER_HTTP_502",
  HTTPStatus:502,RequestBody:[]byte("TOP_SECRET private project prompt"),
  SafeMessage:"Provider returned HTTP 502",
 })
 s.recordAgentDebugEvent("local",id,"","collection","collection.failed",
  "COLLECTION_CONTINUATION_FAILED","round=2 provider request trace="+traceID)
 w:=httptest.NewRecorder()
 s.agentChatDebugReportAPI(w,httptest.NewRequest("GET","/api/agent/debug/report",nil))
 if w.Code!=200{t.Fatalf("report %d: %s",w.Code,w.Body)}
 for _,v:=range []string{"taskdeck.agent.debug.report.v1",traceID,
  "PROVIDER_HTTP_502","COLLECTION_CONTINUATION_FAILED",id,"history_items"} {
  if !strings.Contains(w.Body.String(),v) {t.Fatalf("report omitted %q: %s",v,w.Body)}
 }
 if strings.Contains(w.Body.String(),"TOP_SECRET")||strings.Contains(w.Body.String(),"private project prompt"){
  t.Fatal("report exposed request body")
 }
 // The session journal still appears after losing in-memory traces.
 s.agentDebugMu.Lock();s.agentDebug=nil;s.agentDebugMu.Unlock()
 back:=httptest.NewRecorder()
 s.agentChatDebugReportAPI(back,httptest.NewRequest("GET","/api/agent/debug/report",nil))
 if back.Code!=200||!strings.Contains(back.Body.String(),id)||
  !strings.Contains(back.Body.String(),"No in-memory provider traces"){
  t.Fatalf("restarted safe report missing journal: %d %s",back.Code,back.Body)
 }
}

func TestAgentDebugReportAuthorizationAndOwnerIsolation(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 s.startAgentDebug("local","provider_request",id,agentchat.ProviderConfig{ID:"private-provider"},"m")
 if got:=sharedRoutePermissions(httptest.NewRequest("GET","/api/agent/debug/report",nil));
  len(got)!=1||got[0]!=identity.PermissionSettingsRead{
  t.Fatalf("report permission %v",got)
 }
 s.Config.SharedServerEnabled=true
 req:=httptest.NewRequest("GET","/api/agent/debug/report",nil)
 req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},
  identity.Principal{UserID:identity.ID("other-user"),
   Permissions:map[string]bool{identity.PermissionSettingsRead:true}}))
 w:=httptest.NewRecorder()
 s.agentChatDebugReportAPI(w,req)
 if w.Code!=200{t.Fatalf("other owner status %d: %s",w.Code,w.Body)}
 if strings.Contains(w.Body.String(),id)||strings.Contains(w.Body.String(),"private-provider"){
  t.Fatal("report leaked a different owner's trace or journal")
 }
 denied:=httptest.NewRecorder()
 s.agentChatDebugReportAPI(denied,httptest.NewRequest(http.MethodPost,"/api/agent/debug/report",nil))
 if denied.Code!=http.StatusMethodNotAllowed{t.Fatalf("write accepted: %d",denied.Code)}
}


func TestSafeContinuationDiagnosticOmitsModelText(t *testing.T){
 x:=agentchat.Session{Owner:"local",Status:"completed",AutoContinueText:true}
 agentchat.RecordTurnDisposition(&x,"Tiếp tục P1.1.2.3: TOP_SECRET next action")
 d:=agentSafeContinuationDiagnostic(x)
 if d["disposition"]!=agentchat.TurnNeedsContinuation||d["reason"]!="unfinished_action_announcement"||d["standalone_eligible"]!=true{
  t.Fatalf("missing continuation diagnostic: %+v",d)
 }
 if _,ok:=d["next_action"];ok{t.Fatal("model next action exposed")}
 if _,ok:=d["checkpoint_path"];ok{t.Fatal("checkpoint path exposed")}
 if strings.Contains(d["reason"].(string),"TOP_SECRET"){t.Fatal("model text exposed")}
}


func TestAgentCLP20SafeDebugReportGuardAndRedaction(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 const secret="CLP20_PRIVATE_PAYLOAD_MARKER"
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:secret},{Type:"function_call",CallID:"patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"fixture","patch_python":"`+secret+`"}`}}
 x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"patch",Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS","rc":0,"nested":{"secret":"`+secret+`"},"stdout_tail":"`+secret+`","diagnosis_kind":"`+secret+`"}`}}
 x.BindCollectionGuard("inspect "+secret,"",false)
 x.AppendEvent("collect_request_rejected","failed","read",secret)
 x.AppendEvent("clp_checkpoint","failed","read","recovery_budget_exhausted")
 x.Status="failed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 report:=func()string{
  t.Helper()
  w:=httptest.NewRecorder();s.agentChatDebugReportAPI(w,httptest.NewRequest("GET","/api/agent/debug/report",nil))
  if w.Code!=200{t.Fatalf("report %d: %s",w.Code,w.Body)}
  return w.Body.String()
 }
 first:=report()
 if strings.Contains(first,secret)||!strings.Contains(first,"detail_sha256")||!strings.Contains(first,"recovery_budget_exhausted")||!strings.Contains(first,"collection_progress")||!strings.Contains(first,`"status": "PASS"`){t.Fatal("safe report leaked source or omitted structured metadata")}
 clear:=httptest.NewRecorder();s.agentChatDebugClearAPI(clear,httptest.NewRequest("POST","/api/agent/debug/clear",strings.NewReader("{}")))
 if clear.Code!=200{t.Fatal("clear failed")}
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(y.CollectionGuard)==0||string(y.CollectionGuard)!=string(x.CollectionGuard)||y.ToolCalls[0].Result!=x.ToolCalls[0].Result||y.Events[len(y.Events)-2].Detail!=secret{t.Fatal("debug clear changed authoritative data")}
 y.AppendEvent("fresh_turn","failed","","local_request_not_prepared")
 if _,err=store.Save(y,y.Revision);err!=nil{t.Fatal(err)}
 after:=report()
 if strings.Contains(after,secret)||strings.Contains(after,"recovery_budget_exhausted")||!strings.Contains(after,`"collection_progress": null`)||!strings.Contains(after,"local_request_not_prepared"){t.Fatal("clear boundary leaked old guard metadata or lost new event")}
}
