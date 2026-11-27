package server

import (
 "context"
 "fmt"
 "net/http"
 "strings"
 "testing"
 "time"

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

func TestAgentCollectionFingerprintIgnoresDisplayMetadata(t *testing.T){
 a:=[]byte(`{"id":"1","actions":[{"type":"pack","title":"a","paths":["x"]}],"limits":{"max_files":2}}`)
 b:=[]byte(`{"limits":{"max_files":2},"actions":[{"paths":["x"],"type":"pack","id":"2"}],"title":"b"}`)
 if agentCollectionRequestFingerprint(a)!=agentCollectionRequestFingerprint(b){t.Fatal("metadata bypassed fingerprint")}
 if agentCollectionRequestFingerprint(a)==agentCollectionRequestFingerprint([]byte(strings.ReplaceAll(string(a),"\"x\"","\"y\""))){t.Fatal("different paths collapsed")}
}
func TestAgentCollectionIdentityClassification(t *testing.T){
 identity:=patchtool.ProjectIdentity{Key:"key",ID:"id"}
 for _,tc:=range []struct{raw string;required bool;kind string}{
  {"",false,""},{"",true,"missing"},{`"id"`,false,"malformed"},{`{"key":"foreign"}`,false,"foreign"},{`{"key":"key","id":"id","extra":1}`,false,"malformed"},{`{"key":"key","id":"id"}`,true,""},
 }{
  err:=agentCollectionIdentity([]byte(tc.raw),identity,tc.required)
  if agentCollectionIdentityKind(err)!=tc.kind||(tc.kind==""&&err!=nil){t.Fatalf("identity classification %q: %v",tc.raw,err)}
 }
}
func TestAgentInterruptedCollectionCheckpointClosesExactCallOnce(t *testing.T){
 x:=agentchat.Session{Status:"failed",ProviderRun:&agentchat.ProviderRun{ID:"r5-e2",Phase:"acknowledged"},History:[]agentchat.HistoryItem{{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read-1",Arguments:`{"actions":[{"type":"overview"}]}`}}}
 x.AppendEvent("collect_response_checkpoint","requesting_ai","read-1","r5-e2")
 if !agentCloseInterruptedCollection(&x)||len(x.History)!=2||x.History[1].CallID!="read-1"||!strings.Contains(x.History[1].Content,"COLLECTION_LOCAL_INTERRUPTED"){t.Fatal("checkpoint not closed")}
 if agentCloseInterruptedCollection(&x){t.Fatal("checkpoint duplicated")}
}

func TestAgentRecoveredCollectionResumeRequiresGrantAndRunsOnce(t *testing.T){
 x:=agentchat.Session{Owner:"local",Status:"failed",ProviderRun:&agentchat.ProviderRun{ID:"r50-e61",Kind:"collection",Phase:"acknowledged",CallID:"read"}}
 x.AppendEvent("provider_ack_recovered","failed","read","r50-e61: verified local response")
 if agentRecoveredCollectionResumeCandidate(x){t.Fatal("automatic egress without grant")}
 x.AutoContinueText=true
 if !agentRecoveredCollectionResumeCandidate(x){t.Fatal("authorized recovery refused")}
 x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"patch",Status:"unknown"}}
 if agentRecoveredCollectionResumeCandidate(x){t.Fatal("uncertain native Patch bypassed")}
 x.ToolCalls=nil
 x.AppendEvent("collection_resume_attempted","failed","read","r50-e61")
 if agentRecoveredCollectionResumeCandidate(x){t.Fatal("same recovery repeated")}
}
func TestAgentFeedbackSelectsCurrentRejectionNotHistorical(t *testing.T){
 old:=agentchat.ToolCallRecord{CallID:"old",Status:"rejected",Name:"shell",Result:agentRejectedUnsupportedToolFeedback("shell","UNSUPPORTED_TOOL")}
 current:=agentchat.ToolCallRecord{CallID:"current",Status:"rejected",Name:"taskdeck_submit_patch",Result:agentRejectedPatchFeedback("PATCH_APPROVAL_STALE","stale",true)}
 x:=agentchat.Session{Owner:"local",Status:"tool_rejected",AutoContinueText:true,ToolCalls:[]agentchat.ToolCallRecord{old,current},History:[]agentchat.HistoryItem{{Type:"function_call",CallID:"old",Name:"shell"},{Type:"user",Content:"continue"},{Type:"function_call",CallID:"current",Name:"taskdeck_submit_patch"}}}
 if idx,ok:=agentAutoFeedbackCandidate(x);!ok||idx!=1{t.Fatalf("wrong feedback target %d/%v",idx,ok)}
 if agentFeedbackCallIsCurrent(x,"old"){t.Fatal("old denial mistaken for current")}
}
func TestAgentCollectionDiagnosticsContainOnlyFingerprints(t *testing.T){
 const digest="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 raw:=[]byte(`{"instructions":"Installed native COLLECT contract version test, SHA-256 `+digest+`","tools":[{"name":"taskdeck_collect_workspace","parameters":{"type":"object"}}],"input":[{"type":"function_call_output","output":"{\"repair_attempt\":1,\"secret\":\"do-not-export\"}"}]}`)
 tool,contract,repair:=agentCollectionRequestDiagnostics(raw)
 if len(tool)!=64||contract!=digest||!repair{t.Fatal("missing safe Collection diagnostics")}
}

func TestAgentUnknownCollectionRecoveryPolicy(t *testing.T){
 now:=time.Now().UTC()
 fixture:=func()agentchat.Session{
  return agentchat.Session{Owner:"local",Status:"failed",AutoContinueText:true,RetryMode:agentchat.RetryUnlimited,
   ProviderRun:&agentchat.ProviderRun{ID:"r50-e61",Kind:"collection",Phase:"unknown",CallID:"read"},
   History:[]agentchat.HistoryItem{{Type:"user",Content:"original task"},{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read",Arguments:`{}`},{Type:"function_call_output",CallID:"read",Content:`{"status":"PASS"}`}}}
 }
 x:=fixture()
 if !agentAutoFeedbackNeedsWork(x)||agentUnknownCollectionRecoveryState(x,now)!="ready"{t.Fatal("interrupted Collection not scheduled")}
 for _,mutate:=range []func(*agentchat.Session){
  func(x *agentchat.Session){x.AutoContinueText=false},
  func(x *agentchat.Session){off:=false;x.AutoCollection=&off},
  func(x *agentchat.Session){x.Status="cancelled"},
  func(x *agentchat.Session){x.Owner="other"},
  func(x *agentchat.Session){x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"patch",Status:"unknown"}}},
  func(x *agentchat.Session){x.History=x.History[:2]},
 }{
  blocked:=fixture();mutate(&blocked)
  if agentUnknownCollectionResumeCandidate(blocked){t.Fatal("unsafe automatic recovery")}
 }
 x.Continuation=&agentchat.TurnContinuation{NoProgressTurns:100}
 if !agentUnknownCollectionResumeCandidate(x){t.Fatal("exact receipt recovery was blocked by diagnostic counter")}
 x.AppendEvent("collection_auto_resume_deferred","failed","read","r50-e61")
 if agentUnknownCollectionRecoveryState(x,now)!="retry_deferred"{t.Fatal("missing recovery backoff")}
 if agentUnknownCollectionRecoveryState(x,now.Add(time.Minute))!="ready"{t.Fatal("unlimited retry did not become eligible")}
 x.RetryMode=agentchat.RetryOff
 if agentUnknownCollectionRecoveryState(x,now.Add(time.Minute))!="retry_exhausted"{t.Fatal("retry off loop")}
 x.RetryMode=agentchat.RetryLimited
 for i:=0;i<5;i++{x.AppendEvent("collection_auto_resume_deferred","failed","read","r50-e61")}
 if agentUnknownCollectionRecoveryState(x,now.Add(time.Minute))!="retry_exhausted"{t.Fatal("limited retry loop")}
}

func TestAgentUnknownCollectionAutoResumeAfterRestart(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 p:=agentProviderFixture()
 receipt:=`{"status":"PASS","read_only":true}`
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"original unfinished task"},{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read",Arguments:`{}`},{Type:"function_call_output",CallID:"read",Content:receipt}}
 x.Status="requesting_ai"
 x.AutoContinueText=true
 x.RetryMode=agentchat.RetryUnlimited
 x.ProviderRun=&agentchat.ProviderRun{ID:"r50-e61",Kind:"collection",Phase:"sending",CallID:"read",PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("old payload")),ProviderFingerprint:agentchat.CapabilityFingerprint(p,p.Models[0]),CreatedAt:time.Now().UTC(),UpdatedAt:time.Now().UTC()}
 x.AppendEvent("provider_egress","requesting_ai","read","collection")
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 s.agentReconcileOneStartupSession(context.Background(),store,id)
 old,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if old.Status!="failed"||old.ProviderRun.Phase!="unknown"||!agentUnknownCollectionResumeCandidate(old){t.Fatalf("startup failed to schedule recovery: %+v",old.ProviderRun)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,provider agentchat.ProviderConfig,key string,body []byte)(agentchat.ResponsesResult,error){
  calls++
  if provider.ID!=p.ID||modelFromAgentPayload(body)!=p.Models[0].ID||key!="TEST-KEY"{t.Fatal("wrong recovery provider binding")}
  if agentchat.ProviderPayloadDigest(body)==old.ProviderRun.PayloadSHA256||!strings.Contains(string(body),"NEW continuation"){t.Fatal("old payload replayed or recovery instruction missing")}
  return agentchat.ResponsesResult{ResponseID:"fresh-response",Text:"recovered response"},nil
 }
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=1||y.Status!="completed"||y.ProviderRun.ID==old.ProviderRun.ID||len(y.CollectionResumeHistory)!=1||
  y.CollectionResumeHistory[0].Previous.Phase!="unknown"||y.CollectionResumeHistory[0].ReceiptSHA256!=agentchat.ProviderPayloadDigest([]byte(receipt)){
  t.Fatalf("recovery did not preserve old unknown checkpoint: calls=%d status=%s checkpoints=%+v",calls,y.Status,y.CollectionResumeHistory)
 }
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 if calls!=1{t.Fatal("recovered checkpoint dispatched twice")}
}


func TestCancelOrphanedAcknowledgedCollectionPreservesPatchAndAllowsNewMessage(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 x.Status="requesting_ai"
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"original task"},{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"old-patch",Arguments:"{}"},{Type:"function_call_output",CallID:"old-patch",Content:"FAIL"},{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read-1",Arguments:"{}"}}
 x.ToolCalls=[]agentchat.ToolCallRecord{{Name:"taskdeck_submit_patch",CallID:"old-patch",Status:"error",Result:"FAIL",FeedbackSent:true,FeedbackStatus:"acknowledged"}}
 x.ProviderRun=&agentchat.ProviderRun{ID:"r5-e2",Kind:"feedback",Phase:"acknowledged",CallID:"old-patch",PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("old payload")),CreatedAt:now,UpdatedAt:now}
 x.AppendEvent("collect_response_checkpoint","requesting_ai","read-1",x.ProviderRun.ID)
 x.AppendEvent("collect_started","requesting_ai","read-1","native_patch_tool_collect")
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/cancel",map[string]any{})
 if w.Code!=200||!strings.Contains(w.Body.String(),`"recovered_local_collection":true`){t.Fatalf("Cancel did not reconcile orphan: %d %s",w.Code,w.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.Status!="cancelled"||x.ProviderRun.Phase!="acknowledged"||x.ToolCalls[0].Result!="FAIL"||!x.ToolCalls[0].FeedbackSent{t.Fatalf("wrong cancellation state: %+v",x)}
 last:=x.History[len(x.History)-1]
 if last.Type!="function_call_output"||last.CallID!="read-1"||!strings.Contains(last.Content,"COLLECTION_LOCAL_INTERRUPTED"){t.Fatal("missing exact interrupted receipt")}
 rev:=x.Revision
 w=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/cancel",map[string]any{})
 if w.Code!=200{t.Fatal(w.Body.String())}
 x,err=store.Get(id,"local");if err!=nil||x.Revision!=rev{t.Fatal("second Cancel rewrote closed checkpoint")}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){calls++;return agentchat.ResponsesResult{ResponseID:"new",Text:"Resumed original task."},nil}
 w=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",map[string]any{"message":"continue","confirm_external":true,"expected_revision":x.Revision,"request_id":"resume-after-orphan"})
 if w.Code!=200||calls!=1{t.Fatalf("new message failed: %d %s calls=%d",w.Code,w.Body,calls)}
}

func TestOrphanedCollectionCancelRejectsUncertainWork(t *testing.T){
 base:=agentchat.Session{Owner:"local",Status:"requesting_ai",ProviderRun:&agentchat.ProviderRun{ID:"r5-e2",Phase:"acknowledged"},History:[]agentchat.HistoryItem{{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read",Arguments:"{}"}}}
 base.AppendEvent("collect_response_checkpoint","requesting_ai","read","r5-e2")
 for _,mutate:=range []func(*agentchat.Session){
  func(x *agentchat.Session){x.ProviderRun=&agentchat.ProviderRun{ID:"r5-e2",Phase:"unknown"}},
  func(x *agentchat.Session){x.ToolCalls=[]agentchat.ToolCallRecord{{Name:"taskdeck_submit_patch",Status:"running"}}},
  func(x *agentchat.Session){x.ToolCalls=[]agentchat.ToolCallRecord{{Name:"taskdeck_submit_patch",Status:"error",FeedbackStatus:"unknown"}}},
  func(x *agentchat.Session){x.Owner="other"},
  func(x *agentchat.Session){x.Events=nil},
 }{
  x:=base;mutate(&x)
  if agentCancelOrphanedCollection(&x)||x.Status!="requesting_ai"{t.Fatal("unsafe orphan cancellation")}
 }
}


// A long valid full-history request must not be rejected by the older
// two-MiB PREPARED limit. The old UNKNOWN read stays evidence, not replay.
func TestAgentUnknownCollectionAutoResumeLargeFullHistory(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 p:=agentProviderFixture()
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"original unfinished task"}}
 for i:=0;i<24;i++{
  call:=fmt.Sprintf("historical-read-%d",i)
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:call,Name:"taskdeck_collect_workspace",Arguments:"{}"},agentchat.HistoryItem{Type:"function_call_output",CallID:call,Content:strings.Repeat("source evidence ",6000)})
 }
 receipt:=`{"status":"PASS","read_only":true}`
 x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:"{}"},agentchat.HistoryItem{Type:"function_call_output",CallID:"read",Content:receipt})
 x.Status="requesting_ai";x.AutoContinueText=true;x.RetryMode=agentchat.RetryUnlimited
 x.ProviderRun=&agentchat.ProviderRun{ID:"r50-e61",Kind:"collection",Phase:"sending",CallID:"read",PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("old payload")),ProviderFingerprint:agentchat.CapabilityFingerprint(p,p.Models[0]),CreatedAt:time.Now().UTC(),UpdatedAt:time.Now().UTC()}
 x.AppendEvent("provider_egress","requesting_ai","read","collection")
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 s.agentReconcileOneStartupSession(context.Background(),store,id)
 old,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if !agentUnknownCollectionResumeCandidate(old){t.Fatal("not eligible for fresh continuation")}
 calls:=0
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{t.Fatal("old native read replayed");return ""}
 s.AgentChatCall=func(_ context.Context,provider agentchat.ProviderConfig,key string,body []byte)(agentchat.ResponsesResult,error){
  calls++
  if len(body)<=2<<20||len(body)>4<<20{t.Fatalf("expected full payload in previously rejected range: %d",len(body))}
  if provider.ID!=p.ID||key!="TEST-KEY"||agentchat.ProviderPayloadDigest(body)==old.ProviderRun.PayloadSHA256||!strings.Contains(string(body),"NEW continuation"){t.Fatal("wrong recovery binding")}
  return agentchat.ResponsesResult{ResponseID:"fresh-response",Text:"recovered response"},nil
 }
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=1||y.Status!="completed"||len(y.CollectionResumeHistory)!=1||y.CollectionResumeHistory[0].ReceiptSHA256!=agentchat.ProviderPayloadDigest([]byte(receipt)){t.Fatalf("resume did not complete: calls=%d status=%s",calls,y.Status)}
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 if calls!=1{t.Fatal("recovery repeated")}
}

func TestAgentPreparedFailureClassificationDoesNotExposeRawError(t *testing.T){
 if got:=agentPreparedFailureCode(agentchat.ErrStaleRevision);got!="STALE_REVISION"{t.Fatal(got)}
 if got:=agentPreparedFailureCode(agentchat.ErrInvalidSession);got!="INVALID_PREPARED_STATE"{t.Fatal(got)}
 if got:=agentPreparedFailureCode(fmt.Errorf("private source path and credential"));got!="STORAGE_OR_DURABILITY_FAILURE"{t.Fatal(got)}
}
