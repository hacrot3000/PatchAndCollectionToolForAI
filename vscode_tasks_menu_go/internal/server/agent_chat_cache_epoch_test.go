package server

import (
 "encoding/json"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCacheEpochAppendAndPolicyReset(t *testing.T){
 for _,api:=range []string{"responses","chat_completions"}{
  x:=agentchat.Session{History:[]agentchat.HistoryItem{{Type:"user",Content:"Original task"}}}
  p:=agentchat.ProviderConfig{APIType:api}
  model:=agentchat.ModelConfig{MaxInputTokens:65536}
  c:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"Trusted policy",EnableCollectionTool:true,Next:[]agentchat.HistoryItem{{Type:"user",Content:"Continue"}}}
  first,e,_,err:=agentBuildContextEpoch(x,model,p,c,nil,true)
  if err!=nil||e==nil{t.Fatalf("first epoch: %v",err)}
  x.History=append(x.History,c.Next...)
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read",Arguments:"{}"})
  c.Next=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"read",Content:"native receipt"}}
  second,next,reason,err:=agentBuildContextEpoch(x,model,p,c,e,true)
  if err!=nil||next==nil||reason!="append_only"||e.ID!=next.ID{t.Fatalf("reuse %s: %v",reason,err)}
  var a,b struct{Instructions string;Input []json.RawMessage}
  if json.Unmarshal(first,&a)!=nil||json.Unmarshal(second,&b)!=nil{t.Fatal("invalid payload")}
  if a.Instructions!=b.Instructions||len(a.Input)>=len(b.Input){t.Fatal("prefix did not remain stable")}
  for i:=range a.Input{if string(a.Input[i])!=string(b.Input[i]){t.Fatal("old wire input changed")}}
  if strings.Count(string(second),"native receipt")!=1{t.Fatal("receipt duplicated")}
  c.Instructions="Updated trusted policy: collection disabled";c.EnableCollectionTool=false
  third,_,why,err:=agentBuildContextEpoch(x,model,p,c,next,true)
  if err!=nil||why=="append_only"||!strings.Contains(string(third),"Updated trusted policy"){t.Fatalf("policy reset: %s %v",why,err)}
 }
}


func TestAgentCacheEpochCurrentPlanAndRevocationRemainTrusted(t *testing.T){
 x:=agentchat.Session{ProjectID:"project-id",TaskPlan:&agentchat.TaskPlan{Goal:"audit source",State:"active",Steps:[]agentchat.TaskPlanStep{{ID:"one",Title:"inspect",Status:"pending",Acceptance:"read exact file"}}}}
 stable,current:=agentChatInstructionsWithAutomationParts(x,true,"project")
 if !strings.Contains(current,"audit source")||strings.Contains(stable,"audit source"){t.Fatal("owner plan not separated")}
 x.TaskPlan.Goal="updated objective";x.TaskPlan.Steps[0].Acceptance="verify receipt"
 stable2,current2:=agentChatInstructionsWithAutomationParts(x,true,"project")
 if stable2!=stable||current2==current||!strings.Contains(current2,"updated objective"){t.Fatal("mutable plan affects static policy or failed to refresh")}
 cfg:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:stable2,CurrentInstructions:current2,EnableCollectionTool:true,Next:[]agentchat.HistoryItem{{Type:"user",Content:"Continue"}}}
 body,err:=agentchat.BuildResponsesRequest(cfg);if err!=nil{t.Fatal(err)}
 var wire struct{Instructions string;Input []json.RawMessage}
 if json.Unmarshal(body,&wire)!=nil||!strings.HasSuffix(wire.Instructions,current2)||len(wire.Input)!=1||strings.Contains(string(wire.Input[0]),"updated objective"){t.Fatal("trusted plan demoted to history")}
 disabled:=false;x.AutoCollection=&disabled
 revoked,_:=agentChatInstructionsWithAutomationParts(x,true,"project")
 if revoked==stable||!strings.Contains(revoked,"Collection has been disabled"){t.Fatal("revocation was deferred to retain cache")}
}

func TestAgentCacheEpochOptionalAdviceCannotBlockMandatoryRequest(t *testing.T){
 // Find the exact native contract overhead with the real request builder.
 c:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"x",EnableCollectionTool:true,Next:[]agentchat.HistoryItem{{Type:"user",Content:"mandatory owner instruction"}}}
 body,err:=agentchat.BuildResponsesRequest(c);if err!=nil{t.Fatal(err)}
 var wire struct{Instructions string}
 if json.Unmarshal(body,&wire)!=nil{t.Fatal("invalid request")}
 c.Instructions=strings.Repeat("x",(1<<20)-len(wire.Instructions)+1)
 got,_,reason,err:=agentBuildContextEpoch(agentchat.Session{},agentchat.ModelConfig{MaxInputTokens:4000000},agentchat.ProviderConfig{APIType:"responses"},c,nil,false)
 if err!=nil||!strings.Contains(reason,"optional_advice_omitted")||!strings.Contains(string(got),"mandatory owner instruction"){t.Fatalf("optional guidance prevented valid request: %s %v",reason,err)}
}
