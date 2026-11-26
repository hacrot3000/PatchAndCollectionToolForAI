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
