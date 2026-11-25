package server

import (
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const agentCollectionResumeInstruction="Resume the original unfinished task from the canonical conversation and current verified native receipts. This is a NEW continuation after an unconfirmed read-only Collection provider response, not a replay. Do not re-execute prior Patches or resend acknowledged results. Inspect only the specific missing scope using the current native COLLECT contract; never claim the prior provider received the result. Preserve the existing task plan and approval rules."

// The latest saved observation is mandatory context for explicit resume,
// not a replay of its old provider request or a new native execution.
// Move its exact pair into Next on a private projection. The only new
// canonical item is still the actual new owner message.
func agentCollectionResumeProjection(x agentchat.Session,history []agentchat.HistoryItem,next agentchat.HistoryItem)([]agentchat.HistoryItem,[]agentchat.HistoryItem,bool){
 digest,ok:=agentchat.CollectionResumeReceipt(x)
 if !ok||len(x.History)==0||len(history)<len(x.History){return nil,nil,false}
 for i,v:=range x.History{if v!=history[i]{return nil,nil,false}}
 last:=len(x.History)-1
 if agentchat.ProviderPayloadDigest([]byte(history[last].Content))!=digest{return nil,nil,false}
 first:=-1
 if x.ProviderRun.CallID!=""{
  for i,v:=range x.History[:last]{
   if v.CallID!=x.ProviderRun.CallID{continue}
   if v.Type!="function_call"||v.Name!="taskdeck_collect_workspace"||first>=0{return nil,nil,false}
   first=i
  }
 }else if agentchat.CollectionTextObservation(x.History,last){
  first=last
  if strings.HasPrefix(history[last].Content,agentchat.NativeCollectionTextObservationPrefix){first=last-1}
 }
 if first<0{return nil,nil,false}
 visible:=make([]agentchat.HistoryItem,0,len(history))
 for i,v:=range history{if i!=first&&i!=last{visible=append(visible,v)}}
 mandatory:=[]agentchat.HistoryItem{history[first]}
 if first!=last{mandatory=append(mandatory,history[last])}
 return visible,append(mandatory,next),true
}

// Structured context rejection may compact optional history but must not
// remove this exact receipt from a resume request. Also keep the model's
// request paired with a current modern text fallback observation.
func agentMandatoryRetrySuffix(items []agentchat.HistoryItem)int{
 cut:=len(items)-1
 if cut<0{return 0}
 if items[cut].Type=="function_call_output"{
  for cut>0&&items[cut-1].Type=="function_call_output"{cut--}
  return cut
 }
 if agentchat.CollectionTextObservation(items,cut)&&strings.HasPrefix(items[cut].Content,agentchat.NativeCollectionTextObservationPrefix){return cut-1}
 if items[cut].Type=="user"&&items[cut].Content==agentCollectionResumeInstruction&&cut>0{
  prior:=cut-1
  if items[prior].Type=="function_call_output"&&prior>0&&items[prior-1].Type=="function_call"&&items[prior-1].Name=="taskdeck_collect_workspace"&&items[prior-1].CallID==items[prior].CallID{return prior-1}
  if agentchat.CollectionTextObservation(items,prior){
   if strings.HasPrefix(items[prior].Content,agentchat.NativeCollectionTextObservationPrefix){return prior-1}
   return prior
  }
 }
 return cut
}
