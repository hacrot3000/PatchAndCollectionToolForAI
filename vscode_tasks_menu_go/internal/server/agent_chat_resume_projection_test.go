package server

import(
 "encoding/json"
 "strings"
 "testing"
 "bletonfc/vscode_tasks_menu/internal/agentchat"
)
func TestAgentCLP20MandatoryReceiptAcrossContextRecovery(t *testing.T){
 for _,fallback:=range []bool{false,true}{
  goal:=agentchat.HistoryItem{Type:"user",Content:"Original owner goal"}
  call:=agentchat.HistoryItem{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"project":{"key":"p","id":"id"},"actions":[{"type":"file","path":"a"}]}`}
  receipt:=agentchat.HistoryItem{Type:"function_call_output",CallID:"read",Content:`{"status":"PASS","text":"`+strings.Repeat("exact-receipt-",14000)+`"}`}
  if fallback{call=agentchat.HistoryItem{Type:"assistant",Content:call.Arguments};receipt=agentchat.HistoryItem{Type:"user",Content:agentchat.NativeCollectionTextObservationPrefix+receipt.Content}}
  history:=[]agentchat.HistoryItem{goal}
  for i:=0;i<5;i++{history=append(history,agentchat.HistoryItem{Type:"assistant",Content:strings.Repeat("optional-old-context-",12000)})}
  next:=[]agentchat.HistoryItem{call,receipt,{Type:"user",Content:agentCollectionResumeInstruction}}
  raw,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Instructions:"Trusted policy",Mode:agentchat.Stateless,History:history,Next:next,MaxOutputTokens:65536});if err!=nil{t.Fatal(err)}
  for i:=0;i<3;i++{
   reduced,ok:=agentRetryContextPayload(raw);if !ok{t.Fatalf("recovery %d failed",i)}
   var wire struct{Input []map[string]string `json:"input"`}
   if json.Unmarshal(reduced,&wire)!=nil{t.Fatal("invalid wire")}
   count:=0
   for _,it:=range wire.Input{if it["output"]==receipt.Content||it["content"]==receipt.Content{count++}}
   if count!=1||!strings.Contains(string(reduced),"Original owner goal"){t.Fatalf("fallback=%v round=%d lost mandatory context",fallback,i)}
   raw=reduced
  }
 }
}
