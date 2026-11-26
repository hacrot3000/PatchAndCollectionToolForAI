package server

import (
 "context"
 "encoding/json"
 "errors"
 "reflect"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCLP20LegacySameDigestTimeoutsAndStop(t *testing.T){
 oldDelay:=agentRetryDelay
 agentRetryDelay=time.Millisecond
 defer func(){agentRetryDelay=oldDelay}()
 for _,tc:=range []struct{name string;mode agentchat.RetryMode;stop bool;want int}{
  {"off",agentchat.RetryOff,false,1},
  {"limited",agentchat.RetryLimited,false,4},
  {"unlimited",agentchat.RetryUnlimited,false,4},
  {"stop",agentchat.RetryUnlimited,true,3},
 }{
  t.Run(tc.name,func(t *testing.T){
   s,id:=newAgentConversationFixture(t,"stateless")
   store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
   x.History=[]agentchat.HistoryItem{
    {Type:"user",Content:"private original objective"},
    {Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"old","patch_python":"print(1)"}`},
    {Type:"function_call_output",CallID:"old-patch",Content:`{"status":"PASS","run_id":"old-native"}`},
    {Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"a"}]}`},
    {Type:"function_call_output",CallID:"read",Content:"private exact native observation"},
   }
   x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"old-patch",Name:"taskdeck_submit_patch",Status:"done",Result:x.History[2].Content,FeedbackSent:true,FeedbackStatus:"acknowledged"}}
   body,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"trusted policy",History:x.History[:4],Next:x.History[4:],MaxOutputTokens:8192})
   if err!=nil{t.Fatal(err)}
   digest:=agentchat.ProviderPayloadDigest(body)
   now:=time.Now().UTC()
   x.Status="requesting_ai";x.RetryMode=tc.mode
   x.ProviderRun=&agentchat.ProviderRun{ID:"collection-retry",Kind:"collection",Phase:"sending",CallID:"read",PayloadSHA256:digest,ProviderFingerprint:"fixture",CreatedAt:now,UpdatedAt:now}
   x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   ctx,cancel:=context.WithCancel(context.Background());defer cancel()
   calls:=0
   s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
    calls++
    if key!="private credential"||!reflect.DeepEqual(body,payload){t.Fatal("retry changed provider context")}
    audit,e:=store.ProviderAttempts(id,"local")
    if e!=nil||audit==nil||audit.Total!=uint64(calls)||audit.Attempts[calls-1].Outcome!=""{t.Fatalf("missing pre-egress audit: %+v %v",audit,e)}
    progress:=agentRetrySnapshot(s.Workspace,"local",id)
    if progress==nil||progress.SameDigestFailures!=calls-1{t.Fatalf("wrong live stagnation count: %+v",progress)}
    if tc.stop&&calls==3{cancel();return agentchat.ResponsesResult{Text:"late result"},nil}
    if calls<=3{return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"RESPONSE_HEADER_TIMEOUT",Retryable:true,Ambiguous:true}}
    return agentchat.ResponsesResult{Text:"complete"},nil
   }
   result,err,_:=s.agentChatCallForOwner(ctx,"local",id,agentchat.ProviderConfig{ID:"fake",APIType:"responses"},"private credential",body)
   if calls!=tc.want{t.Fatalf("attempts %d want %d",calls,tc.want)}
   if tc.stop{if !errors.Is(err,context.Canceled)||result.Text!=""{t.Fatal("Stop did not win")}}else if tc.mode==agentchat.RetryOff{if err==nil{t.Fatal("Off retried")}}else if err!=nil||result.Text!="complete"{t.Fatalf("unexpected result %v",err)}
   reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   audit,err:=reopened.ProviderAttempts(id,"local");if err!=nil||audit==nil||audit.Total!=uint64(tc.want){t.Fatalf("audit missing: %+v %v",audit,err)}
   for i,attempt:=range audit.Attempts{
    want:=agentchat.AttemptHeaderTimeout
    if tc.stop&&i==2{want=agentchat.AttemptCancelled}else if i==3{want=agentchat.AttemptResponse}
    if attempt.Outcome!=want||attempt.PayloadSHA256!=digest||attempt.RetryMode!=tc.mode||attempt.OutputBudget!=8192||attempt.RecoveryGeneration!=0{t.Fatalf("attempt mismatch %+v",attempt)}
   }
   raw,err:=json.Marshal(audit);if err!=nil{t.Fatal(err)}
   if strings.Contains(string(raw),"private "){t.Fatal("private prompt or credential leaked")}
   saved,err:=reopened.Get(id,"local")
   if err!=nil||!reflect.DeepEqual(saved.History,x.History)||!reflect.DeepEqual(saved.ToolCalls,x.ToolCalls)||saved.ProviderRun.Phase!="sending"||saved.Revision!=x.Revision{t.Fatal("retry telemetry changed canonical history or ACK")}
   if agentRetrySnapshot(s.Workspace,"local",id)!=nil{t.Fatal("live retry state leaked")}
  })
 }
}
