package server

import (
 "context"
 "encoding/json"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentAdaptiveBudgetLearnsAcrossRequests(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.RetryMode=agentchat.RetryUnlimited
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 oldDelay:=agentRetryDelay;agentRetryDelay=time.Millisecond
 defer func(){agentRetryDelay=oldDelay}()
 original:=agentContextRecoveryFixture(t)
 var wire struct{Model string `json:"model"`}
 if err=json.Unmarshal(original,&wire);err!=nil{t.Fatal(err)}
 model:=agentchat.ModelConfig{ID:wire.Model,MaxInputTokens:1280000,MaxOutputTokens:98304}
 p:=agentchat.ProviderConfig{ID:"adaptive-test",APIType:"responses",Models:[]agentchat.ModelConfig{model}}
 key:="fixture-key"
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==1{return agentchat.ResponsesResult{},&agentchat.ProviderError{Code:"PROVIDER_HTTP_400",HTTPStatus:400,ProviderErrorCode:"context_length_exceeded"}}
  if len(payload)>=len(original){t.Fatalf("request %d did not shrink",calls)}
  return agentchat.ResponsesResult{Text:"Recovered",ResponseID:"ack"},nil
 }
 if _,err,_=s.agentChatCallForOwner(context.Background(),"local",id,p,key,original);err!=nil{t.Fatal(err)}
 effective,limit:=agentAdaptiveRequestBudget(x,p,model,key,98304)
 if calls!=2||effective.MaxInputTokens>=model.MaxInputTokens||limit!=16384{t.Fatalf("learned limits not applied: %d %+v %d",calls,effective,limit)}
 if model.MaxInputTokens!=1280000||p.Models[0].MaxOutputTokens!=98304{t.Fatal("configured model mutated")}
 old:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"old-read",Arguments:`{"actions":[{"type":"file","path":"old.go"}]}`}
 current:=agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"new-read",Arguments:`{"actions":[{"type":"file","path":"new.go"}]}`}
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"Finish UAG-01"},old,{Type:"function_call_output",CallID:old.CallID,Content:strings.Repeat("old evidence ",50000)},current}
 next:=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:current.CallID,Content:`{"status":"PASS","run_id":"new-native-receipt"}`}}
 visible:=agentContextForRequest(x,effective,"rules",next)
 payload,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:model.ID,Mode:agentchat.Stateless,Instructions:"rules",History:visible,Next:next,MaxOutputTokens:limit})
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(payload),"new-native-receipt")||strings.Contains(string(payload),strings.Repeat("old evidence ",100)){t.Fatal("fresh receipt missing or old body leaked")}
 if _,err,_=s.agentChatCallForOwner(context.Background(),"local",id,p,key,payload);err!=nil||calls!=3{t.Fatalf("next request should succeed first attempt: %d %v",calls,err)}
 for _,tc:=range []struct{x agentchat.Session;p agentchat.ProviderConfig;key string}{
  {agentchat.Session{Workspace:x.Workspace,Owner:"other"},p,key},
  {x,p,"other-credential"},
  {x,agentchat.ProviderConfig{ID:p.ID,APIType:p.APIType,BaseURL:"https://other.example",Models:p.Models},key},
 }{
  got,out:=agentAdaptiveRequestBudget(tc.x,tc.p,model,tc.key,98304)
  if got.MaxInputTokens!=model.MaxInputTokens||out!=98304{t.Fatal("budget crossed identity boundary")}
 }
}
