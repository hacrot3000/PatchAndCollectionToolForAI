package server

import (
 "context"
 "errors"
 "encoding/json"
 "net/http"
 "testing"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentRetryModePersistsAndValidatesConsent(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 for _,mode:=range []string{"limited","unlimited","off"}{
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/retry",
   map[string]any{"mode":mode,"expected_revision":x.Revision,"confirm_external":mode!="off"})
  if w.Code!=200{t.Fatalf("retry mode %s rejected: %d %s",mode,w.Code,w.Body)}
  x,err=store.Get(id,"local")
  if err!=nil{t.Fatal(err)}
  if string(x.RetryMode)!=mode{t.Fatalf("mode not saved: %s",x.RetryMode)}
 }
 denied:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/retry",
  map[string]any{"mode":"unlimited","expected_revision":x.Revision,"confirm_external":false})
 if denied.Code!=403{t.Fatalf("retry without consent: %d",denied.Code)}
 invalid:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/retry",
  map[string]any{"mode":"forever_and_execute_patch","expected_revision":x.Revision,"confirm_external":true})
 if invalid.Code!=400{t.Fatalf("invalid mode accepted: %d",invalid.Code)}
}

func TestAgentLimitedProviderRetryFiveTimesAtTenSecondPolicy(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 x.RetryMode=agentchat.RetryLimited
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 oldDelay:=agentRetryDelay
 agentRetryDelay=2*time.Millisecond
 defer func(){agentRetryDelay=oldDelay}()
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"HTTP_503",HTTPStatus:503,Retryable:true}
 }
 _,err,_=s.agentChatCallForOwner(context.Background(),"local",id,agentchat.ProviderConfig{ID:"fake",APIType:"responses"},"test-key",[]byte(`{"model":"m"}`))
 if err==nil{t.Fatal("failed calls must return error after budget")}
 if calls!=6{t.Fatalf("limited mode needs one initial plus exactly five retries, got %d",calls)}
 if agentRetrySnapshot(s.Workspace,"local",id)!=nil{t.Fatal("retry progress must clear on completion")}
}

func TestAgentUnlimitedRetryStopsOnCancellationAndNeverRetriesPermanentError(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 x.RetryMode=agentchat.RetryUnlimited
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 oldDelay:=agentRetryDelay
 agentRetryDelay=time.Millisecond
 defer func(){agentRetryDelay=oldDelay}()
 ctx,cancel:=context.WithCancel(context.Background())
 defer cancel()
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==4{cancel()}
  return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"HTTP_503",HTTPStatus:503,Retryable:true}
 }
 _,err,_=s.agentChatCallForOwner(ctx,"local",id,agentchat.ProviderConfig{ID:"fake",APIType:"responses"},"test-key",[]byte(`{"model":"m"}`))
 if !errors.Is(err,context.Canceled){t.Fatalf("Stop did not cancel unlimited retry: %v",err)}
 if calls!=4{t.Fatalf("unexpected count after Stop: %d",calls)}
 if agentRetrySnapshot(s.Workspace,"local",id)!=nil{t.Fatal("retry state leaked after Stop")}
 calls=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++
  return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"AUTH_FAILURE",HTTPStatus:401}
 }
 _,err,_=s.agentChatCallForOwner(context.Background(),"local",id,agentchat.ProviderConfig{ID:"fake",APIType:"responses"},"test-key",[]byte(`{"model":"m"}`))
 if err==nil||calls!=1{t.Fatalf("401 must not retry forever; calls=%d error=%v",calls,err)}
}

func TestAgentUnlimitedRetriesHTTP499AfterTimeoutUntilSuccessful(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 x.RetryMode=agentchat.RetryUnlimited
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 oldDelay:=agentRetryDelay
 agentRetryDelay=time.Millisecond
 defer func(){agentRetryDelay=oldDelay}()
 original:=[]byte(`{"model":"model","input":"collection continuation"}`)
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if string(payload)!=string(original){t.Fatalf("request mutated at attempt %d",calls)}
  switch calls{
  case 1:return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"CONNECT_TIMEOUT",Stage:"provider_http",Retryable:true,Ambiguous:true}
  case 2:return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"PROVIDER_HTTP_499",Stage:"provider_http",Message:"context canceled",HTTPStatus:499}
  case 3:return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"PROVIDER_HTTP_499",Stage:"provider_http",HTTPStatus:499}
  case 4:return agentchat.ResponsesResult{Text:"complete",ResponseID:"ack"},nil
  default:t.Fatalf("unexpected retry %d",calls);return agentchat.ResponsesResult{},nil
  }
 }
 result,err,_:=s.agentChatCallForOwner(context.Background(),"local",id,
  agentchat.ProviderConfig{ID:"fake",APIType:"responses"},"key",original)
 if err!=nil||result.Text!="complete"||calls!=4{t.Fatalf("did not continue after 499: count=%d result=%+v err=%v",calls,result,err)}
 if agentRetrySnapshot(s.Workspace,"local",id)!=nil{t.Fatal("retry state leaked")}
}

func TestAgentUnlimitedRetriesIncompleteByExpandingOutputBudget(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 x.RetryMode=agentchat.RetryUnlimited
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 oldDelay:=agentRetryDelay
 agentRetryDelay=time.Millisecond
 defer func(){agentRetryDelay=oldDelay}()
 p:=agentchat.ProviderConfig{ID:"fake",APIType:"responses",Models:[]agentchat.ModelConfig{{ID:"model",MaxOutputTokens:8192}}}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  var request struct{Model string `json:"model"`;Limit int `json:"max_output_tokens"`}
  if err:=json.Unmarshal(payload,&request);err!=nil{t.Fatal(err)}
  expected:=4096
  if calls==2{expected=8192}
  if request.Model!="model"||request.Limit!=expected{t.Fatalf("unexpected retry payload: %+v",request)}
  if calls==2&&!strings.Contains(string(payload),"OUTPUT-BOUND RECOVERY"){t.Fatalf("guided retry instructions missing from payload")}
  if calls==1{return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"RESPONSE_INCOMPLETE",ProviderErrorCode:"max_output_tokens",Retryable:true}}
  return agentchat.ResponsesResult{ResponseID:"done",Text:"Recovered"},nil
 }
 result,err,_:=s.agentChatCallForOwner(context.Background(),"local",id,p,"key",[]byte(`{"model":"model","max_output_tokens":4096}`))
 if err!=nil||calls!=2||result.Text!="Recovered"{t.Fatalf("retry failed: calls=%d result=%+v err=%v",calls,result,err)}
 if agentRetrySnapshot(s.Workspace,"local",id)!=nil{t.Fatal("progress state leaked")}
}

func TestAgentUnlimitedStopsAtConfiguredOutputCap(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local")
 if err!=nil{t.Fatal(err)}
 x.RetryMode=agentchat.RetryUnlimited
 if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==2&&!strings.Contains(string(payload),"OUTPUT-BOUND RECOVERY"){t.Fatal("recovery request must have explicit bounded-output guidance")}
  if calls>2{t.Fatalf("identical max_output_tokens failure retried indefinitely: %d",calls)}
  return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"RESPONSE_INCOMPLETE",ProviderErrorCode:"max_output_tokens",Retryable:true}
 }
 _,err,_=s.agentChatCallForOwner(context.Background(),"local",id,
  agentchat.ProviderConfig{ID:"fake",Models:[]agentchat.ModelConfig{{ID:"model",MaxOutputTokens:4096}}},
  "key",[]byte(`{"model":"model","max_output_tokens":4096}`))
 if err==nil||calls!=2{t.Fatalf("hard output cap must stop after one guided recovery: calls=%d err=%v",calls,err)}
}

func agentContextRecoveryFixture(t *testing.T) []byte {
 t.Helper()
 history:=[]agentchat.HistoryItem{
  {Type:"user",Content:"Original task: implement and verify the requested change"},
  {Type:"function_call",CallID:"old-collect",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[]}`},
  {Type:"function_call_output",CallID:"old-collect",Content:strings.Repeat("historical source code\n",12000)},
  {Type:"assistant",Content:"Continue with the verified patch."},
  {Type:"function_call",CallID:"fresh-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"patch","patch_python":"print(1)"}`},
 }
 next:=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"fresh-patch",Content:`{"status":"PASS","run_id":"native-1"}`}}
 payload,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{
  Model:"model",Mode:agentchat.Stateless,Instructions:"Mandatory TaskDeck rules",
  History:history,Next:next,MaxOutputTokens:32768,EnablePatchTool:true,
 })
 if err!=nil{t.Fatal(err)}
 return payload
}
func TestAgentContextRecoveryPreservesFreshReceiptAndShrinksHistory(t *testing.T){
 original:=agentContextRecoveryFixture(t)
 reduced,ok:=agentRetryContextPayload(original)
 if !ok||len(reduced)>=len(original){t.Fatal("expected smaller stateless request")}
 var before,after struct{
  Instructions string `json:"instructions"`
  Tools json.RawMessage `json:"tools"`
  Input []map[string]string `json:"input"`
  Limit int `json:"max_output_tokens"`
 }
 if json.Unmarshal(original,&before)!=nil||json.Unmarshal(reduced,&after)!=nil{t.Fatal("invalid request")}
 if before.Instructions!=after.Instructions||string(before.Tools)!=string(after.Tools)||after.Limit!=16384{
  t.Fatal("provider options or mandatory instructions changed unexpectedly")
 }
 if after.Input[0]["content"]!=before.Input[0]["content"]{t.Fatal("original task lost")}
 for _,want:=range before.Input[len(before.Input)-2:]{
  found:=false
  for _,got:=range after.Input{
   if got["call_id"]==want["call_id"]&&got["type"]==want["type"]&&got["arguments"]==want["arguments"]&&got["output"]==want["output"]{found=true}
  }
  if !found{t.Fatalf("current source call/output lost: %v",want)}
 }
 for _,got:=range after.Input{if got["call_id"]=="old-collect"{t.Fatal("large historical collection should be summarized")}}
}
func TestAgentContextRecoveryIsBoundedAndOptIn(t *testing.T){
 for _,tc:=range []struct{name string;mode agentchat.RetryMode;code string;rejects int;alwaysFail bool;want int}{
  {"success",agentchat.RetryUnlimited,"context_length_exceeded",1,false,2},
  {"progressive",agentchat.RetryUnlimited,"context_length_exceeded",2,false,3},
  {"bounded",agentchat.RetryUnlimited,"context_window_exceeded",4,true,4},
  {"off",agentchat.RetryOff,"context_length_exceeded",1,true,1},
  {"generic_400",agentchat.RetryUnlimited,"invalid_request_error",1,true,1},
 }{
  t.Run(tc.name,func(t *testing.T){
   s,id:=newAgentConversationFixture(t,"stateless")
   store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
   x.RetryMode=tc.mode
   if _,err=store.Save(x,x.Revision);err!=nil{t.Fatal(err)}
   oldDelay:=agentRetryDelay;agentRetryDelay=time.Millisecond
   defer func(){agentRetryDelay=oldDelay}()
   original:=agentContextRecoveryFixture(t)
   var req map[string]json.RawMessage
   if err=json.Unmarshal(original,&req);err!=nil{t.Fatal(err)}
   req["max_output_tokens"]=json.RawMessage("65536")
   original,err=json.Marshal(req);if err!=nil{t.Fatal(err)}
   calls:=0
   previousInput,previousOutput:=0,0
   s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
    calls++
    if calls>tc.want{t.Fatalf("unexpected attempt %d",calls)}
    var wire struct{Input json.RawMessage `json:"input"`;Output int `json:"max_output_tokens"`}
    if err:=json.Unmarshal(payload,&wire);err!=nil{t.Fatal(err)}
    if calls>1&&(len(wire.Input)>previousInput||wire.Output>previousOutput||(len(wire.Input)==previousInput&&wire.Output==previousOutput)){t.Fatal("recovery did not strictly reduce input or output")}
    previousInput,previousOutput=len(wire.Input),wire.Output
    if !strings.Contains(string(wire.Input),"fresh-patch")||!strings.Contains(string(wire.Input),"native-1"){t.Fatal("current receipt was lost")}
    if calls<=tc.rejects||tc.alwaysFail{return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"PROVIDER_HTTP_400",HTTPStatus:400,ProviderErrorCode:tc.code}}
    return agentchat.ResponsesResult{Text:"Recovered",ResponseID:"ack"},nil
   }
   res,err,_:=s.agentChatCallForOwner(context.Background(),"local",id,agentchat.ProviderConfig{ID:"fake",APIType:"responses"},"test-key",original)
   if calls!=tc.want||(err==nil)==tc.alwaysFail{t.Fatalf("calls=%d result=%+v err=%v",calls,res,err)}
  })
 }
}
func TestAgentContextRecoveryRefusesStatefulReplay(t *testing.T){
 for _,payload:=range []string{
  `{"previous_response_id":"resp_1","model":"m","instructions":"rules","input":[{"role":"user","content":"task"}]}`,
  `{"conversation":"conv_1","model":"m","instructions":"rules","input":[{"role":"user","content":"task"}]}`,
  `{"store":true,"model":"m","instructions":"rules","input":[{"role":"user","content":"task"}]}`,
 }{
  if _,ok:=agentRetryContextPayload([]byte(payload));ok{t.Fatal("stateful replay must not be rebuilt")}
 }
}


func TestAgentCLP20SmallContextRecoveryPreservesFullInput(t *testing.T){
 history:=[]agentchat.HistoryItem{
  {Type:"user",Content:"original objective"},
  {Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"old","patch_python":"print(1)"}`},
  {Type:"function_call_output",CallID:"old-patch",Content:`{"status":"PASS","run_id":"old-native"}`},
  {Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"a"}]}`},
  {Type:"function_call_output",CallID:"read",Content:`{"status":"PASS","run_id":"current-native","text":"exact observation"}`},
 }
 body,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"trusted policy",History:history[:4],Next:history[4:],MaxOutputTokens:32768,EnableCollectionTool:true})
 if err!=nil{t.Fatal(err)}
 reduced,ok:=agentRetryContextPayload(body)
 if !ok{t.Fatal("output-only recovery refused")}
 var old,now map[string]json.RawMessage
 if json.Unmarshal(body,&old)!=nil||json.Unmarshal(reduced,&now)!=nil{t.Fatal("invalid request")}
 if string(old["input"])!=string(now["input"])||string(now["max_output_tokens"])!="16384"{t.Fatal("small input should stay exact, output reservation should halve")}
 for _,api:=range []string{"responses","chat_completions"}{
  if _,err:=agentchat.MeasureProviderPayload(api,reduced);err!=nil{t.Fatal(err)}
 }
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 model:=agentchat.ModelConfig{ID:"m",MaxInputTokens:128000,MaxOutputTokens:32768}
 p:=agentchat.ProviderConfig{ID:"p",APIType:"responses",Models:[]agentchat.ModelConfig{model}}
 if !agentLearnContextRecovery(store,x,p,"key",body,reduced){t.Fatal("output hint not learned")}
 effective,limit:=agentAdaptiveRequestBudget(store,x,p,model,"key",32768)
 if effective.MaxInputTokens!=model.MaxInputTokens||limit!=16384{t.Fatal("output-only recovery falsely reduced input cap")}
}
