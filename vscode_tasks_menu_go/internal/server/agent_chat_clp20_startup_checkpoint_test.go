package server

import (
 "reflect"
 "strings"
 "testing"
 "time"
 "context"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCLP20InterruptedCollectionCheckpointProof(t *testing.T){
 for _,tc:=range []struct{name string;kind string;tail []agentchat.HistoryItem;duplicate bool;want bool}{
  {"exact","message",nil,false,true},
  {"collection","collection",nil,false,true},
  {"duplicate_marker","collection",nil,true,false},
  {"already_observed","collection",[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"read",Content:"verified receipt"}},false,false},
  {"later_patch","collection",[]agentchat.HistoryItem{{Type:"function_call",CallID:"patch",Name:"taskdeck_submit_patch",Arguments:`{}`}},false,false},
  {"duplicate_call","collection",[]agentchat.HistoryItem{{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{}`}},false,false},
 }{
  t.Run(tc.name,func(t *testing.T){
   x:=agentchat.Session{Status:"failed",ProviderRun:&agentchat.ProviderRun{ID:"r1-e1",Kind:tc.kind,Phase:"acknowledged"},
    History:append([]agentchat.HistoryItem{{Type:"user",Content:"original objective"},{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{}`}},tc.tail...)}
   x.AppendEvent("collect_response_checkpoint","requesting_ai","read","r1-e1")
   if tc.duplicate{x.AppendEvent("collect_response_checkpoint","requesting_ai","read","r1-e1")}
   before:=append([]agentchat.HistoryItem(nil),x.History...)
   if got:=agentCloseInterruptedCollection(&x);got!=tc.want{t.Fatalf("checkpoint closed=%t want=%t",got,tc.want)}
   if !tc.want{
    if !reflect.DeepEqual(before,x.History){t.Fatal("ambiguous checkpoint modified history")}
   }else{
    if len(x.History)!=len(before)+1||!strings.Contains(x.History[len(before)].Content,"COLLECTION_LOCAL_INTERRUPTED"){t.Fatal("missing explicitly unverified diagnostic")}
    if agentCloseInterruptedCollection(&x){t.Fatal("checkpoint closed twice")}
   }
   if x.ProviderRun.Phase!="acknowledged"{t.Fatal("ACK downgraded")}
  })
 }
}

func TestAgentCLP20StartupAcknowledgedCollectionNoReplay(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.Status="requesting_ai"
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"original objective"},{Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{}`},{Type:"function_call_output",CallID:"old-patch",Content:"verified old receipt"},{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{}`}}
 x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"old-patch",Name:"taskdeck_submit_patch",Status:"done",Result:"verified old receipt",FeedbackSent:true,FeedbackStatus:"acknowledged"}}
 now:=time.Now().UTC()
 x.ProviderRun=&agentchat.ProviderRun{ID:"ack-run",Kind:"collection",Phase:"acknowledged",PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("old wire")),ProviderFingerprint:"fixture",CreatedAt:now,UpdatedAt:now}
 x.PreviousResponseID="old-response"
 x.AppendEvent("collect_response_checkpoint","requesting_ai","read","ack-run")
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(context.Context,agentchat.ProviderConfig,string,[]byte)(agentchat.ResponsesResult,error){calls++;return agentchat.ResponsesResult{},nil}
 s.agentReconcilePendingOnStartup(context.Background())
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if got.Status!="failed"||got.ProviderRun.Phase!="acknowledged"||got.PreviousResponseID!=""||!reflect.DeepEqual(got.ToolCalls,x.ToolCalls)||len(got.History)!=len(x.History)+1||!reflect.DeepEqual(got.History[:len(x.History)],x.History){t.Fatal("startup did not preserve acknowledged receipt and close interrupted read")}
 if !strings.Contains(got.History[len(got.History)-1].Content,"COLLECTION_LOCAL_INTERRUPTED"){t.Fatal("local interruption not recorded")}
 s.agentReconcilePendingOnStartup(context.Background())
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 after,err:=store.Get(id,"local");if err!=nil||calls!=0||!reflect.DeepEqual(after.History,got.History)||after.ProviderRun.Phase!="acknowledged"{t.Fatal("restart replayed or changed acknowledged run")}
}


// Exercise the actual startup lease, durable journal and prepared-only
// recovery using distinct persisted old-format states. The scheduler is not
// allowed to send an old UNKNOWN request or execute a historical Patch.
func TestAgentCLP20MixedLegacyRestartMatrix(t *testing.T){
 for _,tc:=range []struct{name,phase,status,feedback string;wantPhase,wantStatus,wantFeedback string;send bool}{
  {"prepared","prepared","requesting_ai","acknowledged","completed","completed","acknowledged",true},
  {"sending","sending","requesting_ai","sending","unknown","failed","unknown",false},
  {"acknowledged","acknowledged","requesting_ai","acknowledged","acknowledged","failed","acknowledged",false},
  {"unknown","unknown","failed","unknown","unknown","failed","unknown",false},
  {"cancelled","unknown","cancelled","acknowledged","unknown","cancelled","acknowledged",false},
  {"pending_patch","completed","waiting_tool","","completed","waiting_tool","",false},
 }{
 t.Run(tc.name,func(t *testing.T){
  s,id:=newAgentConversationFixture(t,"stateless")
  store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
  x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  x.History=[]agentchat.HistoryItem{{Type:"user",Content:"original task across restart"},{Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"prior","patch_python":"print(1)"}`}}
  call:=agentchat.ToolCallRecord{CallID:"old-patch",Name:"taskdeck_submit_patch",Status:"done",Result:`{"status":"PASS","run_id":"native-old"}`,FeedbackSent:true,FeedbackStatus:tc.feedback}
  if tc.name=="pending_patch"{call.Status="pending";call.Result="";call.FeedbackSent=false}else{x.History=append(x.History,agentchat.HistoryItem{Type:"function_call_output",CallID:"old-patch",Content:call.Result})}
  x.ToolCalls=[]agentchat.ToolCallRecord{call}
  x.AutoPatch=false;x.AutoFeedback=false;x.AutoContinueText=false
  x.RetryMode=agentchat.RetryOff
  x.Status=tc.status
  p:=agentProviderFixture()
  payload:=[]byte(`{"model":"gpt-4.1-mini","input":[{"role":"user","content":"original task across restart"}]}`)
  if tc.phase=="prepared"||tc.phase=="sending"{
   kind,callID:="message",""
   if tc.phase=="sending"{kind,callID="feedback","old-patch"}
   if err=prepareAgentProviderRun(store,&x,kind,callID,p,p.Models[0],payload);err!=nil{t.Fatal(err)}
   if tc.phase=="sending"{x.ToolCalls[0].FeedbackStatus="prepared"}
   x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   if tc.phase=="sending"{x.ToolCalls[0].FeedbackStatus="sending";if err=markAgentProviderSending(store,&x);err!=nil{t.Fatal(err)}}
  }else{
   now:=time.Now().UTC()
   x.ProviderRun=&agentchat.ProviderRun{ID:"legacy-run",Kind:"message",Phase:tc.phase,PayloadSHA256:agentchat.ProviderPayloadDigest(payload),ProviderFingerprint:"fixture",CreatedAt:now,UpdatedAt:now}
   x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
  }
  calls:=0
  restarted:=&Server{Workspace:s.Workspace,Config:s.Config,ConnectionSecrets:s.ConnectionSecrets,AgentChatPack:s.AgentChatPack}
  restarted.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,key string,body []byte)(agentchat.ResponsesResult,error){
   calls++
   if !tc.send||key!="TEST-KEY"||string(body)!=string(payload){t.Fatal("unexpected old provider replay")}
   return agentchat.ResponsesResult{Text:"verified new response",ResponseID:"new-response"},nil
  }
  reopened,err:=restarted.agentChatStore();if err!=nil{t.Fatal(err)}
  restarted.agentReconcilePendingOnStartup(context.Background())
  restarted.processAgentAutoFeedbackSession(context.Background(),reopened,id)
  got,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
  wantCalls:=0;if tc.send{wantCalls=1}
  if calls!=wantCalls||got.ProviderRun.Phase!=tc.wantPhase||got.Status!=tc.wantStatus||got.ToolCalls[0].FeedbackStatus!=tc.wantFeedback{t.Fatalf("wrong recovered state calls=%d phase=%s status=%s feedback=%s",calls,got.ProviderRun.Phase,got.Status,got.ToolCalls[0].FeedbackStatus)}
  if len(got.History)<len(x.History)||!reflect.DeepEqual(got.History[:len(x.History)],x.History)||got.ToolCalls[0].Result!=x.ToolCalls[0].Result||got.ToolCalls[0].Status!=x.ToolCalls[0].Status||got.RetryMode!=x.RetryMode||got.AutoPatch!=x.AutoPatch||got.AutoFeedback!=x.AutoFeedback{t.Fatal("old canonical data or settings changed")}
  restarted.agentReconcilePendingOnStartup(context.Background())
  restarted.processAgentAutoFeedbackSession(context.Background(),reopened,id)
  if calls!=wantCalls{t.Fatal("second startup replayed request")}
 })
 }
}

func TestAgentCLP20StopDuringPreparedResumePreservesUnknown(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.Status="requesting_ai";x.History=[]agentchat.HistoryItem{{Type:"user",Content:"original task"}}
 p:=agentProviderFixture();payload:=[]byte(`{"model":"gpt-4.1-mini","input":[{"role":"user","content":"original task"}]}`)
 if err=prepareAgentProviderRun(store,&x,"message","",p,p.Models[0],payload);err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 calls:=0
 s.AgentChatCall=func(context.Context,agentchat.ProviderConfig,string,[]byte)(agentchat.ResponsesResult,error){
  calls++;cancel();return agentchat.ResponsesResult{Text:"late success"},nil
 }
 if !s.resumePreparedAgentProviderRun(ctx,store,id){t.Fatal("resume not inspected")}
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=1||got.Status!="cancelled"||got.ProviderRun.Phase!="unknown"||!reflect.DeepEqual(got.History,x.History){t.Fatal("Stop lost to late prepared response")}
 s.agentReconcilePendingOnStartup(context.Background())
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 if calls!=1{t.Fatal("cancelled prepared request replayed")}
}
