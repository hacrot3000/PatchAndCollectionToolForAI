package server

import (
 "context"
 "encoding/json"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

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
 x.ProviderRun.PayloadSHA256=strings.Repeat("0",64)
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
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
