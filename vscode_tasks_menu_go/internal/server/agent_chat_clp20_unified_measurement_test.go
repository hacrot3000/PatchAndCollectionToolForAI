package server

import (
 "encoding/json"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCLP20UnifiedAttemptDebugTraceIsolation(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 r:=ownerAgentRunProviderResolver{server:s,owner:"local"}
 body,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"private policy text",Next:[]agentchat.HistoryItem{{Type:"user",Content:"private objective text"}},MaxOutputTokens:8192})
 if err!=nil{t.Fatal(err)}
 m,err:=agentchat.MeasureProviderPayload("responses",body);if err!=nil{t.Fatal(err)}
 digest:=agentchat.BodyDigest(body)
 for _,run:=range []string{"run-one","run-two"}{
  r.ObserveProviderAttempt(agentchat.RunSpec{RunID:run,ConversationID:id,ProviderID:"p",ModelID:"m"},1,m,digest,agentchat.AttemptResponse)
 }
 d:=s.debugStore()
 d.mu.Lock();defer d.mu.Unlock()
 o:=d.owner("local")
 traces:=[]*agentDebugTrace{}
 for _,trace:=range o.Traces{if trace.Action=="unified_provider_attempt"{traces=append(traces,trace)}}
 if len(traces)!=2||traces[0].ID==traces[1].ID{t.Fatal("concurrent run trace identities merged")}
 for i,trace:=range traces{
  if trace.SessionID!=id||len(trace.Events)!=2{t.Fatalf("unexpected trace %+v",trace)}
  ev:=trace.Events[1]
  run:=[]string{"run-one","run-two"}[i]
  if ev.Event!="payload.budget"||ev.RequestSHA256!=digest||ev.RequestBytes!=len(body)||!strings.Contains(ev.SafeMessage,"run="+run)||!strings.Contains(ev.SafeMessage,"output_budget=8192")||!ev.Redacted{t.Fatalf("wrong attempt diagnostic %+v",ev)}
  raw,e:=json.Marshal(trace);if e!=nil{t.Fatal(e)}
  if strings.Contains(string(raw),"private policy text")||strings.Contains(string(raw),"private objective text")||len(trace.payload.Request)!=0||len(trace.payload.Response)!=0{t.Fatal("trace leaked raw content")}
 }
 if len(d.owner("another-owner").Traces)!=0{t.Fatal("owner trace leaked")}
}
