package agentchat

import(
 "strings"
 "testing"
)
func TestProviderPayloadMeasurementCountsFinalFormatWithoutSource(t *testing.T){
 raw,err:=BuildResponsesRequest(RequestConfig{Model:"m",Instructions:"trusted instructions",Mode:Stateless,Next:[]HistoryItem{{Type:"user",Content:strings.Repeat("private source 日本語",200)}},MaxOutputTokens:8192,EnableCollectionTool:true,EnablePatchTool:true})
 if err!=nil{t.Fatal(err)}
 for _,api:=range []string{"responses","chat_completions"}{
  m,err:=MeasureProviderPayload(api,raw);if err!=nil{t.Fatal(err)}
  if m.CanonicalBytes!=len(raw)||m.InstructionsBytes+m.InputBytes+m.ToolsBytes+m.OtherBytes!=len(raw)||m.OutputBudget!=8192||m.ToolsBytes==0||m.EstimatedInputTokens!=(m.WireBytes+1)/2+4{t.Fatalf("bad measurement: %+v",m)}
  if api=="chat_completions"{wire,err:=BuildChatCompletionsRequest(raw);if err!=nil||m.WireBytes!=len(wire){t.Fatal("wrong target wire measurement")}}
 }
}
