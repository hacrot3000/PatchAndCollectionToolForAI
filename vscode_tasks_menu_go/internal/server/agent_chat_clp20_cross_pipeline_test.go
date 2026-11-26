package server

import (
 "context"
 "encoding/json"
 "errors"
 "reflect"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

type clp20MatrixBuilder struct{ body []byte }
func(b clp20MatrixBuilder)Build(agentchat.RunSpec,agentchat.ProviderConfig,agentchat.ModelConfig,[]agentchat.HistoryItem)([]byte,error){return b.body,nil}
type clp20MatrixResolver struct{ provider agentchat.ProviderConfig }
func(r clp20MatrixResolver)Resolve(context.Context,agentchat.RunSpec)(agentchat.ResolvedProvider,error){
 return agentchat.ResolvedProvider{Config:r.provider,ConfigVersion:"fixture-version",Credential:"fixture-key"},nil
}
type clp20MatrixInvoker struct{ call func(context.Context,agentchat.ProviderConfig,string,[]byte)(agentchat.ResponsesResult,error) }
func(i clp20MatrixInvoker)Invoke(ctx context.Context,p agentchat.ProviderConfig,k string,b []byte)(agentchat.ResponsesResult,error){return i.call(ctx,p,k,b)}
type clp20MatrixSink struct{events []agentchat.RunEvent}
func(s *clp20MatrixSink)Publish(_ context.Context,e agentchat.RunEvent)error{s.events=append(s.events,e);return nil}

func TestAgentCLP20CrossPipelineRetryMatrix(t *testing.T){
 timeout:=&agentchat.ProviderError{Code:"RESPONSE_HEADER_TIMEOUT",Retryable:true,Ambiguous:true}
 gateway:=&agentchat.ProviderError{Code:"PROVIDER_HTTP_499",HTTPStatus:499}
 incomplete:=&agentchat.ProviderError{Code:"RESPONSE_INCOMPLETE",ProviderErrorCode:"max_output_tokens",Retryable:true}
 overflow:=&agentchat.ProviderError{Code:"PROVIDER_HTTP_400",HTTPStatus:400,ProviderErrorCode:"context_length_exceeded"}
 permanent:=&agentchat.ProviderError{Code:"PROVIDER_HTTP_401",HTTPStatus:401}
 for _,api:=range []string{"responses","chat_completions"}{
 for _,pipeline:=range []string{"legacy","unified"}{
 for _,mode:=range []agentchat.RetryMode{agentchat.RetryOff,agentchat.RetryLimited,agentchat.RetryUnlimited}{
 for _,tc:=range []struct{name string;seq []error;stop bool;want int;success bool}{
  {"transport",[]error{timeout,gateway,context.DeadlineExceeded},false,4,true},
  {"incomplete",[]error{incomplete,timeout},false,3,true},
  {"overflow",[]error{overflow,gateway},false,3,true},
  {"permanent",[]error{permanent},false,1,false},
  {"stop",[]error{timeout},true,1,false},
  {"limited_exhaustion",[]error{gateway,gateway,gateway,gateway,gateway,gateway,gateway},false,8,true},
 }{
 t.Run(api+"/"+pipeline+"/"+string(mode)+"/"+tc.name,func(t *testing.T){
  want,success:=tc.want,tc.success
  if mode==agentchat.RetryOff{want,success=1,false}
  if mode==agentchat.RetryLimited&&want>6{want,success=6,false}
  history:=[]agentchat.HistoryItem{
   {Type:"user",Content:"original objective"},
   {Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"old","patch_python":"print(1)"}`},
   {Type:"function_call_output",CallID:"old-patch",Content:`{"status":"PASS","run_id":"old-native"}`},
   {Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"a"}]}`},
   {Type:"function_call_output",CallID:"read",Content:`{"status":"PASS","run_id":"current-native","text":"exact observation"}`},
  }
  body,e:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"trusted policy",History:history[:4],Next:history[4:],MaxOutputTokens:32768,EnableCollectionTool:true})
  if e!=nil{t.Fatal(e)}
  provider:=agentchat.ProviderConfig{ID:"p",APIType:api,Models:[]agentchat.ModelConfig{{ID:"m",MaxOutputTokens:65536}}}
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  attempts,waits:=0,0
  var previous []byte
  call:=func(_ context.Context,p agentchat.ProviderConfig,k string,payload []byte)(agentchat.ResponsesResult,error){
   attempts++
   if attempts>want||p.ID!="p"||k!="fixture-key"{t.Fatal("unexpected egress or provider changed")}
   if _,e:=agentchat.MeasureProviderPayload(api,payload);e!=nil{t.Fatal(e)}
   var wire struct{Input []map[string]string `json:"input"`}
   if json.Unmarshal(payload,&wire)!=nil{t.Fatal("invalid payload")}
   count:=0
   for _,it:=range wire.Input{if it["call_id"]=="read"&&it["type"]=="function_call_output"{count++;if it["output"]!=history[4].Content{t.Fatal("receipt changed")}}}
   if count!=1{t.Fatal("missing or duplicated receipt")}
   if attempts>1{
    prior:=tc.seq[attempts-2]
    adaptive:=agentchat.ContextLimitRejected(prior)||prior==incomplete
    if !adaptive&&!reflect.DeepEqual(previous,payload){t.Fatal("transport retry changed bytes")}
    if adaptive&&reflect.DeepEqual(previous,payload){t.Fatal("structured recovery did not adapt")}
   }
   previous=append([]byte(nil),payload...)
   if attempts<=len(tc.seq){return agentchat.ResponsesResult{},tc.seq[attempts-1]}
   return agentchat.ResponsesResult{Text:"complete"},nil
  }
  wait:=func(_ context.Context,d time.Duration)error{
   waits++;if d!=10*time.Second{t.Fatalf("unexpected retry delay %s",d)}
   if tc.stop{cancel()}
   return nil
  }
  var runErr error
  if pipeline=="legacy"{
   s,id:=newAgentConversationFixture(t,"stateless")
   store,e:=s.agentChatStore();if e!=nil{t.Fatal(e)}
   saved,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
   saved.History=history;saved.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"old-patch",Name:"taskdeck_submit_patch",Status:"done",Result:history[2].Content,FeedbackSent:true,FeedbackStatus:"acknowledged"}}
   saved.RetryMode=mode;saved.Status="requesting_ai"
   now:=time.Now().UTC()
   saved.ProviderRun=&agentchat.ProviderRun{ID:"matrix-run",Kind:"collection",Phase:"sending",CallID:"read",PayloadSHA256:agentchat.ProviderPayloadDigest(body),ProviderFingerprint:"fixture",CreatedAt:now,UpdatedAt:now}
   saved,e=store.Save(saved,saved.Revision);if e!=nil{t.Fatal(e)}
   s.AgentChatCall=call
   _,runErr,_=s.agentChatProviderWithRetryUsingWait(ctx,"local",id,provider,"fixture-key",body,wait)
   after,e:=store.Get(id,"local");if e!=nil||!reflect.DeepEqual(after.History,saved.History)||!reflect.DeepEqual(after.ToolCalls,saved.ToolCalls)||after.ProviderRun.Phase!="sending"{t.Fatal("retry altered canonical receipt or ACK")}
   audit,e:=store.ProviderAttempts(id,"local");if e!=nil||audit==nil||audit.Total!=uint64(attempts){t.Fatal("durable attempt audit mismatch")}
  }else{
   sink:=&clp20MatrixSink{}
   adapter:=agentchat.ConfiguredProviderAdapter{ProviderID:"p",Resolver:clp20MatrixResolver{provider},Builder:clp20MatrixBuilder{body},Invoker:clp20MatrixInvoker{call},Wait:wait}
   spec:=agentchat.RunSpec{ProviderID:"p",ModelID:"m",ProviderConfigVersion:"fixture-version",RetryMode:mode}
   runErr=adapter.Execute(ctx,spec,history,sink)
   for _,event:=range sink.events{if event.Type==agentchat.RunEventToolCall{t.Fatal("invented native call")}}
  }
  if attempts!=want||(runErr==nil)!=success{t.Fatalf("attempts=%d want=%d err=%v",attempts,want,runErr)}
  wantWaits:=want-1;if tc.stop&&mode!=agentchat.RetryOff{wantWaits++;if !errors.Is(runErr,context.Canceled){t.Fatal("Stop did not cancel")}}
  if waits!=wantWaits{t.Fatalf("waits=%d want=%d",waits,wantWaits)}
 })
 }
 }
 }
 }
}
