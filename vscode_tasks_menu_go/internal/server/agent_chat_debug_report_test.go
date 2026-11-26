package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
 "encoding/json"
 "fmt"

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
 // Compare the actual persisted journal, not an assumed event position:
 // Save may append guard-transition events after the original marker.
 beforeClear,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 found:=false
 for _,ev:=range beforeClear.Events{if ev.Kind=="collect_request_rejected"&&ev.Detail==secret{found=true}}
 if !found||len(beforeClear.CollectionGuard)==0{t.Fatal("redaction fixture lost its private marker or guard before clear")}
 beforeBytes,err:=json.Marshal(beforeClear);if err!=nil{t.Fatal(err)}
 clear:=httptest.NewRecorder();s.agentChatDebugClearAPI(clear,httptest.NewRequest("POST","/api/agent/debug/clear",strings.NewReader("{}")))
 if clear.Code!=200{t.Fatal("clear failed")}
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 afterBytes,err:=json.Marshal(y);if err!=nil{t.Fatal(err)}
 if string(beforeBytes)!=string(afterBytes){t.Fatal("debug clear changed authoritative persisted journal")}
 y.AppendEvent("fresh_turn","failed","","local_request_not_prepared")
 if _,err=store.Save(y,y.Revision);err!=nil{t.Fatal(err)}
 after:=report()
 if strings.Contains(after,secret)||strings.Contains(after,"recovery_budget_exhausted")||!strings.Contains(after,`"collection_progress": null`)||!strings.Contains(after,"local_request_not_prepared"){t.Fatal("clear boundary leaked old guard metadata or lost new event")}
}


func TestAgentCLP20DebugReportAttemptAuditAfterClear(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 journal,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=journal.Get(id,"local");if err!=nil{t.Fatal(err)}
 body:=[]byte(`{"model":"fixture","input":"CLP20_PRIVATE_WIRE","max_output_tokens":8192}`)
 now:=time.Now().UTC()
 x.Status="requesting_ai";x.RetryMode=agentchat.RetryUnlimited
 x.ProviderRun=&agentchat.ProviderRun{ID:"debug-attempt-run",Kind:"collection",Phase:"sending",CallID:"read",PayloadSHA256:agentchat.ProviderPayloadDigest(body),ProviderFingerprint:"fixture",CreatedAt:now,UpdatedAt:now}
 x,err=journal.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 old,err:=journal.RecordProviderAttempt(id,"local",x.ProviderRun.ID,1,body,true,agentchat.ProviderAttemptPolicy{Mode:agentchat.RetryUnlimited});if err!=nil{t.Fatal(err)}
 report:=func()map[string]any{
  t.Helper()
  w:=httptest.NewRecorder();s.agentChatDebugReportAPI(w,httptest.NewRequest("GET","/api/agent/debug/report",nil))
  if w.Code!=200{t.Fatalf("report %d: %s",w.Code,w.Body)}
  if strings.Contains(w.Body.String(),"CLP20_PRIVATE_WIRE"){t.Fatal("wire leaked")}
  var decoded struct{Sessions []map[string]any}
  if err:=json.Unmarshal(w.Body.Bytes(),&decoded);err!=nil{t.Fatal(err)}
  for _,v:=range decoded.Sessions{if v["id"]==id{return v}}
  return nil
 }
 first:=report()
 if first==nil{t.Fatal("missing journal")}
 run:=first["provider_run"].(map[string]any)
 a:=first["provider_attempts"].([]any)
 if run["run_id"]!="debug-attempt-run"||run["phase"]!="sending"||run["attempt_total"]!=float64(1)||len(a)!=1||a[0].(map[string]any)["client_request_id"]!=old{t.Fatal("missing exact attempt correlation")}
 clear:=httptest.NewRecorder();s.agentChatDebugClearAPI(clear,httptest.NewRequest("POST","/api/agent/debug/clear",strings.NewReader("{}")))
 if clear.Code!=200{t.Fatal("clear failed")}
 fresh,err:=journal.RecordProviderAttempt(id,"local",x.ProviderRun.ID,2,body,true,agentchat.ProviderAttemptPolicy{Mode:agentchat.RetryUnlimited});if err!=nil{t.Fatal(err)}
 // Recording attempt 2 only changes the sidecar, not session revision or
 // UpdatedAt. The report must still include the new post-clear attempt.
 after:=report()
 run=after["provider_run"].(map[string]any);a=after["provider_attempts"].([]any)
 if run["attempt_scope"]!="after_clear"||run["attempt_total"]!=nil||len(a)!=1||a[0].(map[string]any)["client_request_id"]!=fresh{t.Fatal("post-clear attempt lost or historical attempt leaked")}
 audit,err:=journal.ProviderAttempts(id,"local");if err!=nil||audit==nil||audit.Total!=2||audit.Attempts[0].ClientRequestID!=old{t.Fatal("debug clear changed durable attempt audit")}
}


func TestAgentCLP20DebugReportBoundedExportRetainsNewestDomains(t *testing.T){
 // Metadata-only pressure; no raw request/source bytes. The actual encoder
 // is shared with the owner-scoped HTTP endpoint.
 traces:=make([]agentDebugTrace,24)
 sessions:=make([]map[string]any,24)
 for i:=range traces{
  traces[i].ID=fmt.Sprintf("trace-%02d",i)
  traces[i].Events=make([]agentDebugEvent,agentDebugMaxEvents)
  for j:=range traces[i].Events{traces[i].Events[j]=agentDebugEvent{Seq:j+1,Event:"provider_http",SafeMessage:strings.Repeat("bounded diagnostic metadata ",30),Redacted:true}}
  rows:=make([]map[string]any,128)
  for j:=range rows{rows[j]=map[string]any{"seq":j+1,"detail_sha256":strings.Repeat("a",64),"call_id":fmt.Sprintf("call-%d",j)}}
  sessions[i]=map[string]any{"id":fmt.Sprintf("session-%02d",i),"events":rows}
 }
 report:=map[string]any{"provider_traces":traces,"provider_trace_count":len(traces),"sessions":sessions,"session_count":len(sessions)}
 out,err:=agentEncodeSafeDebugReport(report)
 if err!=nil||len(out)>agentDebugReportMaxBytes||!json.Valid(out){t.Fatalf("invalid bounded export: %d bytes %v",len(out),err)}
 var decoded struct{
  Traces []agentDebugTrace `json:"provider_traces"`
  Sessions []map[string]any `json:"sessions"`
  OmittedTraces int `json:"omitted_oldest_traces"`
  OmittedSessions int `json:"omitted_oldest_sessions"`
 }
 if err=json.Unmarshal(out,&decoded);err!=nil{t.Fatal(err)}
 if len(decoded.Traces)==0||len(decoded.Sessions)==0||decoded.Traces[len(decoded.Traces)-1].ID!="trace-23"||decoded.Sessions[0]["id"]!="session-00"{t.Fatal("latest diagnostic domain lost")}
 if decoded.OmittedTraces!=24-len(decoded.Traces)||decoded.OmittedSessions!=24-len(decoded.Sessions)||decoded.OmittedTraces==0{t.Fatal("omission not accurately disclosed")}
 if traces[0].ID!="trace-00"||len(traces[0].Events)!=agentDebugMaxEvents||sessions[23]["id"]!="session-23"{t.Fatal("report encoding mutated original records")}
}

func TestAgentCLP20DebugTraceRetentionKeepsRecentEvidence(t *testing.T){
 d:=&agentDebugStore{owners:map[string]*agentDebugOwner{}}
 o:=d.owner("local")
 now:=time.Now().UTC()
 o.Traces=append(o.Traces,&agentDebugTrace{ID:"expired",UpdatedAt:now.Add(-agentDebugTTL-time.Second)})
 for i:=0;i<agentDebugMaxTraces+3;i++{
  o.Traces=append(o.Traces,&agentDebugTrace{ID:fmt.Sprint(i),UpdatedAt:now})
 }
 d.pruneLocked(o,now)
 if len(o.Traces)!=agentDebugMaxTraces||o.Traces[0].ID!="3"{t.Fatal("trace retention did not keep newest bounded window")}
 newest:=o.Traces[len(o.Traces)-1]
 for i:=0;i<agentDebugMaxEvents+5;i++{d.addLocked(o,newest,agentDebugEvent{Event:"provider.attempt"})}
 if len(newest.Events)!=agentDebugMaxEvents||newest.DroppedEvents!=5||newest.Events[0].Seq!=6||newest.Events[len(newest.Events)-1].Seq!=agentDebugMaxEvents+5{t.Fatal("event retention lost sequence/omission evidence")}
}
