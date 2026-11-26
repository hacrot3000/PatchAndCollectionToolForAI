package server

import (
 "encoding/json"
 "reflect"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCLP20CollectionBudgetPressureKeepsCurrentReceipt(t *testing.T){
 goal:=agentchat.HistoryItem{Type:"user",Content:"Read-only audit: inspect the 150MB source only where necessary"}
 call:=agentchat.HistoryItem{Type:"function_call",CallID:"large-read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"range","path":"large.dat","start_line":1,"end_line":20}]}`}
 x:=agentchat.Session{History:[]agentchat.HistoryItem{goal,call}}
 original:=append([]agentchat.HistoryItem(nil),x.History...)
 for _,api:=range []string{"responses","chat_completions"}{
  for _,high:=range []bool{false,true}{
   text:="verified native excerpt"
   if high{text=strings.Repeat(text,6000)}
   receipt:=agentchat.HistoryItem{Type:"function_call_output",CallID:call.CallID,Content:text}
   cfg:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"Current trusted policy",EnableCollectionTool:true,Next:[]agentchat.HistoryItem{receipt},MaxOutputTokens:8192}
   body,err:=agentBuildContextRequest(x,agentchat.ModelConfig{MaxInputTokens:65536},agentchat.ProviderConfig{APIType:api},cfg)
   if err!=nil{t.Fatal(err)}
   var wire struct{Instructions string;Input []map[string]string}
   if err=json.Unmarshal(body,&wire);err!=nil{t.Fatal(err)}
   if strings.Contains(wire.Instructions,"CONTEXT PRESSURE")!=high{t.Fatalf("unexpected advisory for %s high=%v",api,high)}
   if high&&(!strings.Contains(wire.Instructions,"do not invent a Patch")||!strings.Contains(wire.Instructions,"exact missing path")){t.Fatal("missing bounded read-only advice")}
   count:=0
   for _,item:=range wire.Input{
    if item["type"]=="function_call_output"&&item["call_id"]==call.CallID{
     count++
     if item["output"]!=receipt.Content{t.Fatal("mandatory receipt mutated")}
    }
   }
   if count!=1||wire.Input[0]["content"]!=goal.Content||!reflect.DeepEqual(x.History,original){t.Fatal("canonical goal, history or receipt not preserved")}
  }
 }
}
