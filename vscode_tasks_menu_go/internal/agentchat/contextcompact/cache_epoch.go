package contextcompact

import (
 "encoding/json"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Ignore only presentation metadata; all message text, arguments, call IDs
// and provenance are part of the exact source-prefix binding.
func epochSourceDigest(items []agentchat.HistoryItem)string{
 normalized:=append([]agentchat.HistoryItem(nil),items...)
 for i:=range normalized{
  normalized[i].RequestEffort="";normalized[i].RequestContext=""
  normalized[i].RequestAt="";normalized[i].RequestInputTokens=0
 }
 b,_:=json.Marshal(normalized)
 return agentchat.BodyDigest(b)
}

// Only ordinary text and native COLLECT calls may be appended. A new Patch,
// orphan, synthetic failure projection or text fallback starts a new
// projection using the existing authoritative compactor.
// A receipt at the first position of a suffix cannot be safely classified
// by a parser requiring its preceding proposal. Treat these envelope
// prefixes conservatively even if the full canonical pair is not present.
func epochTextReceipt(text string)bool{
 return strings.HasPrefix(text,agentchat.NativeCollectionTextObservationPrefix)||
  strings.HasPrefix(text,"TaskDeck local read-only Collection tool returned this VERIFIED observation")||
  strings.HasPrefix(text,"TaskDeck native Patch Tool result for your prior text-only proposal")
}
func epochAppendable(items []agentchat.HistoryItem)bool{
 for i,it:=range items{
  switch it.Type{
  case "user","assistant":
   if agentchat.CollectionTextObservation(items,i)||epochTextReceipt(it.Content)||strings.Contains(it.Content,"taskdeck_patch_proposal")||strings.HasPrefix(it.Content,"Historical Native Patch failure evidence")||strings.HasPrefix(it.Content,"[TASKDECK VERIFIED HISTORICAL EVIDENCE CATALOG"){return false}
  case "function_call":
   if it.Name!="taskdeck_collect_workspace"||i+1>=len(items)||items[i+1].Type!="function_call_output"||items[i+1].CallID!=it.CallID{return false}
  case "function_call_output":
   if i==0||items[i-1].Type!="function_call"||items[i-1].CallID!=it.CallID||items[i-1].Name!="taskdeck_collect_workspace"{return false}
  default:return false
  }
 }
 return true
}
func NewCacheEpoch(history,visible []agentchat.HistoryItem,p Policy,scope,id string)*agentchat.CacheEpoch{
 source:=append(append([]agentchat.HistoryItem(nil),history...),p.Next...)
 full:=append(append([]agentchat.HistoryItem(nil),visible...),p.Next...)
 if len(source)==0||len(source)>256||len(full)==0||len(full)>MaxProviderHistoryItems{return nil}
 // Do not freeze a full historical Patch or a receipt for a current Patch.
 // Completed PATCH evidence is represented only by the normal catalog.
 for i,it:=range full{
  if it.Type=="function_call"&&it.Name!="taskdeck_collect_workspace"{return nil}
  if agentchat.CollectionTextObservation(full,i)||epochTextReceipt(it.Content){return nil}
 }
 digest:=epochSourceDigest(source)
 if id==""{id=agentchat.BodyDigest([]byte(scope+"\x00"+digest))}
 return &agentchat.CacheEpoch{ID:id,Scope:scope,SourceCount:len(source),SourceDigest:digest,Visible:full}
}
func ReuseCacheEpoch(history []agentchat.HistoryItem,p Policy,scope string,old *agentchat.CacheEpoch)([]agentchat.HistoryItem,string,bool){
 if old==nil{return nil,"cold_start",false}
 if old.Scope!=scope{return nil,"policy_or_archive_changed",false}
 if old.SourceCount<1||old.SourceCount>len(history)||epochSourceDigest(history[:old.SourceCount])!=old.SourceDigest{return nil,"source_prefix_changed",false}
 suffix:=history[old.SourceCount:]
 tail:=append(append([]agentchat.HistoryItem(nil),suffix...),p.Next...)
 if !epochAppendable(tail){return nil,"evidence_boundary",false}
 visible:=append(append([]agentchat.HistoryItem(nil),old.Visible...),suffix...)
 limit:=p.MaxInputTokens;if limit<=0{limit=DefaultInputTokens}
 if len(visible)+len(p.Next)>MaxProviderHistoryItems||Tokens(p.Instructions)+groupTokens(visible)+groupTokens(p.Next)>=limit*SafeInputPercent/100{return nil,"context_budget",false}
 return visible,"append_only",true
}
