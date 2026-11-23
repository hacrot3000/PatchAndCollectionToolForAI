package server

import (
 "context"
 "net/http"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentTokenUsageAccumulatesOnlyAcknowledgedRequests(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 calls:=0
 s.AgentChatCall=func(ctx context.Context,p agentchat.ProviderConfig,key string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==1{
   return agentchat.ResponsesResult{ResponseID:"r1",Text:"answer",InputTokens:125,OutputTokens:30},nil
  }
  return agentchat.ResponsesResult{ResponseID:"r2",Text:"short response without usage"},nil
 }
 send:=func(message string,revision uint64){
  w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
    map[string]any{"message":message,"confirm_external":true,"expected_revision":revision})
  if w.Code!=http.StatusOK{t.Fatalf("send failed: %d %s",w.Code,w.Body)}
 }
 send("First question",1)
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=1||x.TokenUsage.Requests!=1||x.TokenUsage.InputTokens!=125||
    x.TokenUsage.OutputTokens!=30||x.TokenUsage.EstimatedRequests!=0{
  t.Fatalf("provider-reported totals not saved accurately: %+v calls=%d",x.TokenUsage,calls)
 }
 if x.ProviderRun==nil||x.ProviderRun.EstimatedInputTokens==0||!x.ProviderRun.UsageRecorded{
  t.Fatalf("current request estimate/acknowledgement not persisted: %+v",x.ProviderRun)
 }
 send("Second question",x.Revision)
 y,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if calls!=2||y.TokenUsage.Requests!=2||
   y.TokenUsage.InputTokens<=x.TokenUsage.InputTokens||
   y.TokenUsage.OutputTokens<=x.TokenUsage.OutputTokens||
   y.TokenUsage.EstimatedRequests!=1{
  t.Fatalf("provider missing usage must be marked as estimated: %+v",y.TokenUsage)
 }
 // Session reload from a fresh owner-scoped Store retains counters.
 other,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 z,err:=other.Get(id,"local");if err!=nil{t.Fatal(err)}
 if z.TokenUsage!=y.TokenUsage{t.Fatalf("usage not durable across reload: %+v vs %+v",z.TokenUsage,y.TokenUsage)}
}
func TestAgentTokenUsageIsIdempotentAndUnknownNotCounted(t *testing.T){
 x:=agentchat.Session{ProviderRun:&agentchat.ProviderRun{EstimatedInputTokens:200}}
 if x.TokenUsage.Requests!=0{t.Fatal("old sessions must not invent usage")}
 recordAgentTokenUsage(&x,agentchat.ResponsesResult{Text:"ok",InputTokens:42,OutputTokens:13})
 recordAgentTokenUsage(&x,agentchat.ResponsesResult{Text:"replay",InputTokens:42,OutputTokens:13})
 if x.TokenUsage.Requests!=1||x.TokenUsage.InputTokens!=42||x.TokenUsage.OutputTokens!=13{
  t.Fatalf("same provider receipt counted twice: %+v",x.TokenUsage)
 }
 x.ProviderRun=&agentchat.ProviderRun{EstimatedInputTokens:220}
 // No response has arrived for this sending/unknown request: leave
 // cumulative usage unchanged.
 if x.TokenUsage.Requests!=1{t.Fatal("unknown egress must not increment count")}
 recordAgentTokenUsage(&x,agentchat.ResponsesResult{Text:"new"})
 if x.TokenUsage.Requests!=2||x.TokenUsage.EstimatedRequests!=1{
  t.Fatalf("second acknowledged but unmetered request not tracked: %+v",x.TokenUsage)
 }
}
