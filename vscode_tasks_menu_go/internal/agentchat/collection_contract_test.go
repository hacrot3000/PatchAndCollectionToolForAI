package agentchat

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestCollectionContractTracksInstalledSchema(t *testing.T){
 raw:=`{"tool_version":"fixture1","actions":{"pack":{"required":["type","paths"],"allowed_fields":["type","paths"]},"git":{"required":["type"],"allowed_fields":["type","operations"],"operations":{"allowed":["status"]}}},"limits":{"allowed_fields":["max_files"]}}`
 for _,mode:=range []ContextMode{Stateless,PreviousResponse,Conversation}{
  c:=RequestConfig{Model:"fixture",Instructions:"policy",Mode:mode,Next:[]HistoryItem{{Type:"user",Content:"inspect"}},EnableCollectionTool:true,NativeCollectionContract:raw}
  b,err:=BuildResponsesRequest(c);if err!=nil{t.Fatal(err)}
  var req struct{Instructions string `json:"instructions"`;Tools []struct{Parameters map[string]any `json:"parameters"`} `json:"tools"`}
  if err=json.Unmarshal(b,&req);err!=nil{t.Fatal(err)}
  if !strings.Contains(req.Instructions,"pack: required=type,paths")||!strings.Contains(req.Instructions,"fixture1"){t.Fatal("missing current action contract")}
  p,_:=json.Marshal(req.Tools[0].Parameters)
  if !strings.Contains(string(p),`"paths":{"items":{"type":"string"},"minItems":1,"type":"array"}`)||!strings.Contains(string(p),`"enum":["status"]`){t.Fatalf("missing nested argument shapes: %s",p)}
  c.NativeCollectionContract=strings.ReplaceAll(raw,"fixture1","fixture2")
  b,err=BuildResponsesRequest(c);if err!=nil||!strings.Contains(string(b),"fixture2"){t.Fatal("stale contract")}
 }
}
