package server

import (
 "context"
 "errors"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentPreparedIntentConstruction(t *testing.T) {
 p := agentProviderFixture()
 model := p.Models[0]
 payload := []byte("prepared intent payload")
 if err := setAgentPreparedProviderRun(nil, "message", "", p, model, payload); err == nil { t.Fatal("nil session accepted") }
 x := agentchat.Session{Revision: 3, EventSeq: 4}
 if err := setAgentPreparedProviderRun(&x, "message", "", p, model, payload); err != nil { t.Fatal(err) }
 if x.ProviderRun == nil || x.ProviderRun.ID != "r4-e5" || x.ProviderRun.Kind != "message" || x.ProviderRun.Phase != "prepared" || x.ProviderRun.PayloadSHA256 != agentchat.ProviderPayloadDigest(payload) || x.ProviderRun.ProviderFingerprint != agentchat.CapabilityFingerprint(p, model) { t.Fatalf("incorrect intent: %+v", x.ProviderRun) }
 if x.ProviderRun.CreatedAt.IsZero() || !x.ProviderRun.CreatedAt.Equal(x.ProviderRun.UpdatedAt) { t.Fatal("invalid intent timestamps") }
}

func TestAgentPreparedProviderRunResumesExactlyOnceAfterRestart(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 provider:=agentProviderFixture()
 model:=provider.Models[0]
 payload:=[]byte(`{"model":"gpt-4.1-mini","input":[{"role":"user","content":"Finish the original task"}]}`)
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Finish the original task"})
 x.Status="requesting_ai"
 x.AppendEvent("turn_started","requesting_ai","","")
 if err=prepareAgentProviderRun(store,&x,"message","",provider,model,payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 counts:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,key string,body []byte)(agentchat.ResponsesResult,error){
  counts++
  if string(body)!=string(payload)||key!="TEST-KEY"{t.Fatal("recovered request payload was not exact")}
  return agentchat.ResponsesResult{ResponseID:"response-from-resumed-run",Text:"Task checkpoint ready."},nil
 }
 // The startup pass leaves a PREPARED intent untouched: it was never sent.
 s.agentReconcilePendingOnStartup(context.Background())
 current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if current.Status!="requesting_ai"||current.ProviderRun.Phase!="prepared"{t.Fatal("unsent durable provider intent was discarded on startup")}
 if !agentAutoFeedbackNeedsWork(current){t.Fatal("scheduler did not discover verified PREPARED intent")}
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 finished,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if counts!=1 || finished.Status!="completed"||finished.ProviderRun.Phase!="completed" {
  t.Fatalf("safe prepared run did not complete exactly once: calls=%d status=%s phase=%+v",counts,finished.Status,finished.ProviderRun)
 }
 if !strings.Contains(finished.History[len(finished.History)-1].Content,"Task checkpoint ready"){
  t.Fatal("resumed provider response was not saved")
 }
 if _,_,err=store.PreparedProviderPayload(id,"local");err!=agentchat.ErrInvalidSession{
  t.Fatal("completed provider run remained replayable")
 }
 // Old PREPARED request is no longer schedulable; the provider must never
 // receive a second copy on subsequent background scheduler ticks.
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 if counts!=1{t.Fatal("completed provider run was replayed")}
}
func TestAgentPreparedResumeRejectsCredentialDrift(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 p:=agentProviderFixture(); model:=p.Models[0]
 payload:=[]byte(`{"model":"model","input":"queued request"}`)
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"queued request"})
 x.Status="requesting_ai"
 if err=prepareAgentProviderRun(store,&x,"message","",p,model,payload);err!=nil{t.Fatal(err)}
 if err=agentPinActiveTurnBinding(&x,agentchat.Stateless,p,model,"",[]byte("old-test-key"),payload,"req-1");err!=nil{t.Fatal(err)}
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){calls++;return agentchat.ResponsesResult{},nil}
 if !s.resumePreparedAgentProviderRun(context.Background(),store,id){t.Fatal("prepared run not inspected")}
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=0 || got.ProviderRun==nil || got.ProviderRun.Phase!="blocked" {t.Fatalf("credential drift crossed boundary: calls=%d run=%+v",calls,got.ProviderRun)}
}

func TestAgentSendingRunRestartBecomesAmbiguousWithoutProviderReplay(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 provider:=agentProviderFixture()
 payload:=[]byte(`{"model":"model","input":"the original user message"}`)
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"the original user message"})
 x.Status="requesting_ai"
 if err=prepareAgentProviderRun(store,&x,"message","",provider,provider.Models[0],payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if err=markAgentProviderSending(store,&x);err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++;return agentchat.ResponsesResult{},nil
 }
 s.agentReconcilePendingOnStartup(context.Background())
 after,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if after.Status!="failed"||after.ProviderRun.Phase!="unknown"{
  t.Fatalf("ambiguous egress was not fenced: %+v",after.ProviderRun)
 }
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 if calls!=0{t.Fatal("ambiguous provider call was automatically replayed")}
 handoff,_,err:=store.Handoff(id,"local")
 if err!=nil||!strings.Contains(handoff,"failed"){t.Fatal("handoff omitted ambiguous run")}
}

func TestAgentPreparedProviderPayloadCorruptionFailsWithoutSending(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 p:=agentProviderFixture()
 x.Status="requesting_ai"
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Read source before patch"})
 payload:=[]byte(`{"model":"model","input":"read source"}`)
 if err=prepareAgentProviderRun(store,&x,"message","",p,p.Models[0],payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 // The saved payload should be verified by hash and run ID, not derived
 // again from changing inputs. A mismatched digest blocks recovery.
 // The strengthened Store rejects this mutation at admission. Exercise
 // both that fence and the actual recovery path after disk corruption.
 originalDigest:=x.ProviderRun.PayloadSHA256
 x.ProviderRun.PayloadSHA256=strings.Repeat("0",64)
 if _,err=store.Save(x,x.Revision);err==nil{t.Fatal("Save accepted mismatched PREPARED digest")}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.ProviderRun.PayloadSHA256!=originalDigest{t.Fatal("rejected Save changed the journal")}
 privateFile,err:=aiPrivatePath(s.Workspace);if err!=nil{t.Fatal(err)}
 sidecar:=filepath.Join(filepath.Dir(privateFile),"agent-sessions-"+aiWorkspaceHash(s.Workspace),id+".provider-run.json")
 if err=os.WriteFile(sidecar,[]byte(`{"version":1,"payload":"corrupt"}`),0600);err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++;return agentchat.ResponsesResult{},nil
 }
 if !s.resumePreparedAgentProviderRun(context.Background(),store,id){t.Fatal("prepared run was not inspected")}
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if got.Status!="failed"||got.ProviderRun.Phase!="blocked"||calls!=0{
  t.Fatalf("tampered request crossed provider boundary: calls=%d phase=%+v",calls,got.ProviderRun)
 }
}

func TestAgentPreparedPatchFeedbackResumesOnlyOriginalReceipt(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 p:=agentProviderFixture()
 x.History=append(x.History,
  agentchat.HistoryItem{Type:"user",Content:"Fix file and inspect the verified result"},
  agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_submit_patch",CallID:"patch-1",Arguments:`{"name":"Patch","patch_python":"print('ok')"}`},
  agentchat.HistoryItem{Type:"function_call_output",CallID:"patch-1",Content:`{"status":"PASS","rc":0}`},
 )
 x.ToolCalls=append(x.ToolCalls,agentchat.ToolCallRecord{
  CallID:"patch-1",Name:"taskdeck_submit_patch",Status:"done",
  Result:`{"status":"PASS","rc":0}`,
  FeedbackSent:true,FeedbackStatus:"prepared",
 })
 x.Status="requesting_ai"
 payload:=[]byte(`{"model":"model","input":[{"type":"function_call_output","call_id":"patch-1","output":"PASS"}]}`)
 if err=prepareAgentProviderRun(store,&x,"feedback","patch-1",p,p.Models[0],payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 requests:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,body []byte)(agentchat.ResponsesResult,error){
  requests++
  var parsed map[string]any
  if json.Unmarshal(body,&parsed)!=nil{t.Fatal("invalid exact feedback payload")}
  return agentchat.ResponsesResult{ResponseID:"ack-patch",Text:"Patch passed. Continuing."},nil
 }
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if requests!=1||len(got.ToolCalls)!=1||got.ToolCalls[0].FeedbackStatus!="acknowledged"||
  got.Status!="completed" {t.Fatalf("once-only feedback recovery failed: %d %+v",requests,got)}
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 if requests!=1{t.Fatal("verified old patch result resent")}
}

func TestAgentStopCancelsDurablePreparedRunBeforeEgress(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 p:=agentProviderFixture()
 x.Status="requesting_ai"
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Long-running research"})
 payload:=[]byte(`{"model":"model","input":"research"}`)
 if err=prepareAgentProviderRun(store,&x,"message","",p,p.Models[0],payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/cancel",nil)
 if w.Code!=200||!strings.Contains(w.Body.String(),`"cancelled":true`){
  t.Fatalf("Stop did not cancel PREPARED intent: %d %s",w.Code,w.Body)
 }
 fresh,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if fresh.Status!="cancelled"||fresh.ProviderRun.Phase!="blocked"{
  t.Fatalf("stopped request is still eligible for provider egress: %+v",fresh.ProviderRun)
 }
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++;return agentchat.ResponsesResult{},nil
 }
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 if calls!=0{t.Fatal("stopped prepared provider request was replayed")}
}

func TestAgentRunningProviderHeartbeatUpdatesLiveHandoff(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 p:=agentProviderFixture()
 x.Status="requesting_ai"
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Long research requiring hours"})
 payload:=[]byte(`{"model":"model","input":"Long research"}`)
 if err=prepareAgentProviderRun(store,&x,"message","",p,p.Models[0],payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if err=markAgentProviderSending(store,&x);err!=nil{t.Fatal(err)}
 revision:=x.Revision
 ctx,cancel:=context.WithCancel(context.Background())
 stop:=startAgentProviderHeartbeat(ctx,store,"local",id)
 defer cancel()
 defer stop()
 found:=false
 deadline:=time.Now().Add(900*time.Millisecond)
 for time.Now().Before(deadline){
  content,_,e:=store.Handoff(id,"local")
  if e==nil&&strings.Contains(content,"Provider worker heartbeat"){
   found=true;break
  }
  time.Sleep(15*time.Millisecond)
 }
 if !found{t.Fatal("live provider worker did not provide owner-scoped heartbeat")}
 fresh,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if fresh.Revision!=revision{t.Fatal("heartbeat invalidated running provider CAS")}
}


func TestAgentCLP20PreparedReplanRevocationWinsEgressCAS(t *testing.T){
 for _,tc:=range []struct{name,option string;restart bool}{
  {"live_continue","continue",false},{"live_collection","collection",false},{"restart_continue","continue",true},
 }{t.Run(tc.name,func(t *testing.T){
  s,id:=newAgentConversationFixture(t,"stateless")
  store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
  x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  x.AutoContinueText=true
  x.Status="requesting_ai"
  const callID="replan-read"
  const receipt=`{"status":"PASS","run_id":"verified-replan-receipt"}`
  x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Fix the original issue"},agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:callID,Arguments:"{}"},agentchat.HistoryItem{Type:"function_call_output",CallID:callID,Content:receipt})
  if !x.BindCollectionGuard("Fix the original issue","",false){t.Fatal("guard bind")}
  p:=agentProviderFixture()
  payload:=[]byte(`{"model":"gpt-4.1-mini","input":"bounded replan fixture"}`)
  if err=prepareAgentProviderRun(store,&x,"collection",callID,p,p.Models[0],payload);err!=nil{t.Fatal(err)}
  x.ProviderRun.CollectionReplan=true
  x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
  stale:=x
  // Owner update does NOT advance the turn revision. The atomic egress
  // transaction must nevertheless see the latest revocation.
  updated,err:=store.UpdateAutomation(id,"local",x.Workspace,x.Revision,tc.option,false,"");if err!=nil{t.Fatal(err)}
  if updated.Revision!=stale.Revision||updated.AutomationRevision<=stale.AutomationRevision{t.Fatal("fixture did not hit same-revision metadata race")}
  calls:=0
  s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){calls++;return agentchat.ResponsesResult{},nil}
  if tc.restart{
   s.agentReconcilePendingOnStartup(context.Background())
   if !s.resumePreparedAgentProviderRun(context.Background(),store,id){t.Fatal("restart did not inspect prepared replan")}
  }else{
   if err=markAgentProviderSending(store,&stale);!errors.Is(err,agentchat.ErrCollectionReplanRevoked){t.Fatalf("expected revoked egress: %v",err)}
   if stale.ProviderRun.Phase!="blocked"{t.Fatal("caller retained stale sending state")}
  }
  got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  if calls!=0||got.Status!="failed"||got.ProviderRun.Phase!="blocked"||got.ProviderRun.ID!=x.ProviderRun.ID||got.History[len(got.History)-1].Content!=receipt{t.Fatal("revocation lost canonical receipt or crossed egress")}
  for _,event:=range got.Events{if event.Kind=="provider_egress"{t.Fatal("revoked replan crossed sending boundary")}}
  if _,_,err=store.PreparedProviderPayload(id,"local");!errors.Is(err,agentchat.ErrInvalidSession){t.Fatal("blocked request remained replayable")}
  s.processAgentAutoFeedbackSession(context.Background(),store,id)
  if calls!=0{t.Fatal("blocked replan replayed")}
 })}
}


// An automatic PATCH feedback resume must re-check consent at the durable
// SENDING boundary. A revocation committed after PREPARED but before egress
// (same turn revision, via UpdateAutomation) must win and never cross SENDING.
func TestAgentCLP20PatchFeedbackResumeRevocationWinsEgressCAS(t *testing.T){
 for _,tc:=range []struct{name,option string}{
  {"continue_revoked","continue"},{"feedback_revoked","feedback"},
 }{t.Run(tc.name,func(t *testing.T){
  s,id:=newAgentConversationFixture(t,"stateless")
  store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
  x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  x.AutoContinueText=true;x.AutoFeedback=true;x.AutoFeedbackPersistent=true;x.RetryMode=agentchat.RetryUnlimited
  x.Status="requesting_ai"
  p:=agentProviderFixture()
  payload:=[]byte(`{"model":"gpt-4.1-mini","input":"feedback resume fixture"}`)
  if err=prepareAgentProviderRun(store,&x,"message","",p,p.Models[0],payload);err!=nil{t.Fatal(err)}
  x.ProviderRun.FeedbackAutoResume=true
  x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
  stale:=x
  updated,err:=store.UpdateAutomation(id,"local",x.Workspace,x.Revision,tc.option,false,"");if err!=nil{t.Fatal(err)}
  if updated.Revision!=stale.Revision||updated.AutomationRevision<=stale.AutomationRevision{t.Fatal("fixture did not hit same-revision metadata race")}
  calls:=0
  s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){calls++;return agentchat.ResponsesResult{},nil}
  if err=markAgentProviderSending(store,&stale);!errors.Is(err,agentchat.ErrCollectionReplanRevoked){t.Fatalf("expected revoked egress: %v",err)}
  if stale.ProviderRun.Phase!="blocked"{t.Fatal("caller retained stale sending state")}
  got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  if calls!=0||got.ProviderRun.Phase!="blocked"{t.Fatalf("revocation crossed egress: calls=%d phase=%s",calls,got.ProviderRun.Phase)}
  for _,event:=range got.Events{if event.Kind=="provider_egress"{t.Fatal("revoked feedback resume crossed sending boundary")}}
  if _,_,err=store.PreparedProviderPayload(id,"local");!errors.Is(err,agentchat.ErrInvalidSession){t.Fatal("blocked request remained replayable")}
 })}
}
