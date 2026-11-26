package server

import (
 "bytes"
 "encoding/json"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentRequestDisplayUsesSerializedRequestAndPreservesWire(t *testing.T){
 at:=time.Date(2026,11,26,2,37,0,0,time.UTC)
 p:=agentchat.ProviderConfig{ID:"custom-r2d",APIType:"responses"}
 for _,tc:=range []struct{mode agentchat.ContextMode;prev,conv,effort,want string}{
  {agentchat.Stateless,"","","xhigh","stateless"},
  {agentchat.PreviousResponse,"resp-1","","high","previous_response"},
  {agentchat.Conversation,"","conv-1","medium","conversation"},
 }{
  item:=agentchat.HistoryItem{Type:"user",Content:"Run test"}
  cfg:=agentchat.RequestConfig{Model:"gpt-6.1-sol",Instructions:"stable",Mode:tc.mode,PreviousResponseID:tc.prev,ConversationID:tc.conv,ReasoningEffort:tc.effort,Next:[]agentchat.HistoryItem{item}}
  raw,err:=agentchat.BuildResponsesRequest(cfg);if err!=nil{t.Fatal(err)}
  d:=agentRequestDisplay(p,raw,at)
  if d.ProviderID!=p.ID||d.ModelID!=cfg.Model||d.RequestContext!=tc.want||d.RequestEffort!=tc.effort||d.RequestAt!="2026-11-26T02:37:00Z"||d.RequestInputTokens<=0{t.Fatalf("wrong metadata: %+v",d)}
  stamped:=d.WithHistoryItem(item)
  encoded,err:=json.Marshal(stamped);if err!=nil{t.Fatal(err)}
  var restored agentchat.HistoryItem
  if err=json.Unmarshal(encoded,&restored);err!=nil||restored!=stamped{t.Fatal("metadata failed persistence roundtrip")}
  cfg.Next=[]agentchat.HistoryItem{restored}
  wire,err:=agentchat.BuildResponsesRequest(cfg);if err!=nil||!bytes.Equal(wire,raw){t.Fatalf("display metadata modified provider payload: %v",err)}
  x:=agentchat.Session{}
  appendAgentProviderResult(&x,agentchat.ResponsesResult{Text:"done",RequestDisplay:d},false)
  if len(x.History)!=1||x.History[0].RequestAt!=d.RequestAt||x.History[0].RequestEffort!=d.RequestEffort{t.Fatal("assistant lost pinned request display")}
 }
}
