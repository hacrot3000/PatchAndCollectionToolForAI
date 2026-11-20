package server

import (
 "context"
 "errors"
 "net/http"
 "testing"
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
