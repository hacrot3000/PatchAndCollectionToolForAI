package agentchat

import (
 "encoding/json"
 "errors"
)

// Numeric diagnostics only. WireBytes is the serialized non-streaming
// target-format template, excluding headers/SSE flags/server-side context.
// EstimatedInputTokens is a heuristic, not a new admission quota.
type ProviderPayloadMeasurement struct{
 CanonicalBytes int
 WireBytes int
 EstimatedInputTokens int
 OutputBudget int
 InstructionsBytes int
 InputBytes int
 ToolsBytes int
 OtherBytes int
 Stateful bool
}
func MeasureProviderPayload(api string,canonical []byte)(ProviderPayloadMeasurement,error){
 var m ProviderPayloadMeasurement
 if len(canonical)==0||len(canonical)>maxRequestBytes{return m,errors.New("canonical payload exceeds technical size limit")}
 var fields map[string]json.RawMessage
 if json.Unmarshal(canonical,&fields)!=nil||fields==nil||len(fields["input"])==0{return m,errors.New("invalid canonical provider payload")}
 m.CanonicalBytes=len(canonical)
 m.InstructionsBytes=len(fields["instructions"]);m.InputBytes=len(fields["input"]);m.ToolsBytes=len(fields["tools"])
 m.OtherBytes=m.CanonicalBytes-m.InstructionsBytes-m.InputBytes-m.ToolsBytes
 if m.OtherBytes<0{return ProviderPayloadMeasurement{},errors.New("invalid canonical payload measurement")}
 if raw:=fields["max_output_tokens"];len(raw)>0{if json.Unmarshal(raw,&m.OutputBudget)!=nil||m.OutputBudget<0{return ProviderPayloadMeasurement{},errors.New("invalid output reservation")}}
 m.Stateful=len(fields["previous_response_id"])!=0||len(fields["conversation"])!=0
 switch api{
 case "responses":m.WireBytes=len(canonical)
 case "chat_completions":
  wire,err:=BuildChatCompletionsRequest(canonical);if err!=nil{return ProviderPayloadMeasurement{},err};m.WireBytes=len(wire)
 default:return ProviderPayloadMeasurement{},errors.New("unsupported provider API type")
 }
 m.EstimatedInputTokens=(m.WireBytes+1)/2+4
 return m,nil
}
