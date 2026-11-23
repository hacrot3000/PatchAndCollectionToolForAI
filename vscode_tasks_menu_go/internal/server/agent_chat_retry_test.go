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
