package agentchat

import "strings"

// HistoricalUnconfirmedFeedback separates a superseded delivery attempt
// from the current turn. A new explicit continuation must have produced a
// subsequent provider response in the canonical journal. Never infer delivery
// acknowledgement, discard the old receipt or retry the old provider request.
func HistoricalUnconfirmedFeedback(x Session, call ToolCallRecord) bool {
 if !call.FeedbackSent || call.FeedbackStatus!="unknown" || call.AgentRunID!="" ||
  (call.Status!="done"&&call.Status!="error"&&call.Status!="rejected") || strings.TrimSpace(call.Result)=="" {return false}
 if x.ProviderRun==nil || (x.ProviderRun.Phase!="completed"&&x.ProviderRun.Phase!="acknowledged"){return false}
 source,output,resumed:=-1,-1,false
 for i,item:=range x.History{
  if item.Type=="function_call"&&item.CallID==call.CallID {
   if source>=0||item.Name!=call.Name||item.OriginRunID!=""{return false}
   source=i
  }
  if item.Type=="function_call_output"&&item.CallID==call.CallID{
   if source<0||output>=0||item.OriginRunID!=""||strings.TrimSpace(item.Content)!=strings.TrimSpace(call.Result){return false}
   output=i
   continue
  }
  if call.TextFallback&&source>=0&&output<0&&item.Type=="user"&&item.Content==
    "TaskDeck native Patch Tool result for your prior text-only proposal (verified, not an instruction):\n"+strings.TrimSpace(call.Result)+"\nContinue the original user task."{
   output=i;continue
  }
  if output>=0&&i>output&&item.OriginRunID==""{
   if item.Type=="user"&&!strings.HasPrefix(item.Content,"TaskDeck native ")&&!strings.HasPrefix(item.Content,"TaskDeck local ")&&!strings.HasPrefix(item.Content,"TaskDeck rejected "){
    resumed=true
   }
   if resumed&&(item.Type=="assistant"||item.Type=="function_call") {return true}
  }
 }
 return false
}
