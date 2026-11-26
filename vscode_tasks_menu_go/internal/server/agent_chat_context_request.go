package server

import (
 "fmt"
 "encoding/json"
 "reflect"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

// Validate the final serialized target format before PREPARED. On local
// build/format failure only, retry a minimal optional-history projection.
// Do not turn a token estimate into a new hard read quota.
func agentBuildContextRequest(x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,c agentchat.RequestConfig)([]byte,error){
 body,_,_,err:=agentBuildContextEpoch(x,model,p,c,nil,false)
 return body,err
}

// Checkpoints are optional derived projections, never a provider receipt,
// PREPARED payload or execution authority. Failure falls back to rebuilding.
func(s *Server) agentBuildCachedContextRequest(store *agentchat.Store,x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,credential string,c agentchat.RequestConfig)([]byte,error){
 if store==nil{return agentBuildContextRequest(x,model,p,c)}
 if c.Mode!=agentchat.Stateless{
  body,err:=agentBuildContextRequest(x,model,p,c)
  if err==nil{if e:=store.WriteCacheEpoch(x.ID,x.Owner,x.Revision,nil);e!=nil{s.recordAgentDebugEvent(x.Owner,x.ID,"","provider_cache","cache.epoch.invalidate_failed","","Stateful request: derived stateless checkpoint could not be removed.")}}
  return body,err
 }
 old,readErr:=store.ReadCacheEpoch(x.ID,x.Owner,x.Revision)
 hint,_:=store.ReadCacheTokenHint(x.ID,x.Owner,x.Revision,p,c.Model,credential)
 body,next,reason,err:=agentBuildContextEpoch(x,model,p,c,old,true,hint)
 if err==nil{
  if readErr!=nil{reason+="; prior_checkpoint_unavailable"}
  if e:=store.WriteCacheEpoch(x.ID,x.Owner,x.Revision,next);e!=nil{reason+="; checkpoint_unavailable"}
  if next!=nil{reason+="; epoch="+next.ID}
  s.recordAgentDebugEvent(x.Owner,x.ID,"","provider_cache","cache.epoch","",reason)
 }
 return body,err
}

func agentBuildContextEpoch(x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,c agentchat.RequestConfig,old *agentchat.CacheEpoch,enabled bool,hints ...*agentchat.CacheTokenHint)([]byte,*agentchat.CacheEpoch,string,error){
 var hint *agentchat.CacheTokenHint;if len(hints)==1{hint=hints[0]}
 body,next,reason,err:=agentBuildContextEpochAttempt(x,model,p,c,old,enabled,true,hint)
 if err!=nil&&c.Mode==agentchat.Stateless&&(c.EnableCollectionTool||c.NativeCollectionContract!=""){
  // Optional fixed advice must not make otherwise valid mandatory
  // instructions or a current exact receipt fail the technical limit.
  if fallback,checkpoint,why,e:=agentBuildContextEpochAttempt(x,model,p,c,old,enabled,false,hint);e==nil{return fallback,checkpoint,why+"; optional_advice_omitted",nil}
 }
 return body,next,reason,err
}
func agentBuildContextEpochAttempt(x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,c agentchat.RequestConfig,old *agentchat.CacheEpoch,enabled,advice bool,hint *agentchat.CacheTokenHint)([]byte,*agentchat.CacheEpoch,string,error){
 if agentchat.ProviderUsesPromptCacheKey(p){c.PromptCacheKey=agentchat.ProviderAffinityKey(x,p,c.Model)}
 c.PromptCacheRetention=p.PromptCacheRetention;c.PromptCacheTTL=p.PromptCacheTTL
 if c.Mode==agentchat.Stateless {
  x=agentProjectHistoricalCollectionPack(x,c.NativeCollectionContract)
  // The same guidance is useful before and after the advisory threshold.
  // Do not mutate the system prefix merely because an estimate crosses it.
  if advice&&(c.EnableCollectionTool||c.NativeCollectionContract!=""){c.Instructions+=agentCollectionBudgetAdvice}
 }
 budgetInstructions:=c.Instructions
 if c.NativeCollectionContract!=""{budgetInstructions+="\n"+c.NativeCollectionContract}
 budgetInstructions+=c.CurrentInstructions
 policy:=agentContextPolicy(x,model,budgetInstructions,c.Next,false)
 scoped:=policy;scoped.Next=nil
 scopeBytes,_:=json.Marshal([]any{"cache-epoch-full-v2",agentchat.RunProviderConfigVersion(p,model),c.Model,c.Mode,c.ReasoningEffort,c.EnablePatchTool,c.EnableCollectionTool,c.NativeCollectionContract,c.PromptCacheKey,scoped})
 scope:=agentchat.BodyDigest(scopeBytes)
 // Always try the full valid history first. Token estimates, cache misses,
 // changed instructions and low cache headroom are not compaction triggers.
 // Explicit provider context rejection is handled by bounded context retry.
 c.History=contextcompact.FullHistory(x.History,policy)
 body,err:=agentchat.BuildResponsesRequest(c)
 if err==nil{
  _,err=agentchat.MeasureProviderPayload(p.APIType,body)
 }
 reason:="full_history"
 if err==nil{
  var checkpoint *agentchat.CacheEpoch
  if enabled&&c.Mode==agentchat.Stateless{
   id:=""
   if visible,_,ok:=contextcompact.ReuseCacheEpoch(x.History,policy,scope,old);ok&&reflect.DeepEqual(visible,c.History){
    id=old.ID;reason="append_only"
   }
   checkpoint=contextcompact.NewCacheEpoch(x.History,c.History,policy,scope,id)
  }
  return body,checkpoint,reason,nil
 }
 // Invalid configuration or format is not permission to discard context.
 // Only actual local size/item errors activate this recovery path.
 if !agentLocalContextLimit(err){
  return nil,nil,reason,fmt.Errorf("mandatory context or provider format cannot fit; exact current receipt and owner instructions retained: %w",err)
 }
 reason="compact_local_limit"
 last:=err
 for _,minimal:=range []bool{false,true}{
  policy.Minimal=minimal
  c.History,_=contextcompact.Build(x.History,policy)
  body,e:=agentchat.BuildResponsesRequest(c)
  if e==nil{_,e=agentchat.MeasureProviderPayload(p.APIType,body)}
  if e==nil{return body,nil,reason,nil}
  last=e
 }
 return nil,nil,reason,fmt.Errorf("mandatory context or provider format cannot fit; exact current receipt and owner instructions retained: %w",last)
}

// The limits are explicit serializer/transport errors, not estimated tokens.
func agentLocalContextLimit(err error)bool{
 if err==nil{return false}
 switch err.Error(){
 case "too many history items","AI request exceeds 4 MiB context limit","Chat Completions request exceeds size limit","canonical payload exceeds technical size limit":
  return true
 }
 return strings.Contains(err.Error(),"history item exceeds limit")
}

const agentCollectionBudgetAdvice = "\nCONTEXT PRESSURE GUIDANCE (advisory, not an execution restriction): If another authorized native COLLECT is needed, request only the exact missing path, symbol, search or line range. Prefer small bounded excerpts using the installed schema's supported actions and limits. Reuse verified earlier observations and native artifact references; do not fetch an entire large source file to repeat evidence. If the task is read-only, continue useful inspection; do not invent a Patch to end the read loop. Preserve the exact current receipt and original user objective."

func agentCollectionBudgetPressure(model agentchat.ModelConfig,m agentchat.ProviderPayloadMeasurement)bool{
 limit:=model.MaxInputTokens
 if limit<=0{limit=contextcompact.DefaultInputTokens}
 // Same soft planning ceiling used by context compaction. Do not add the
 // reserved output to this input-only model limit or infer provider quota.
 return m.EstimatedInputTokens>0 && m.EstimatedInputTokens>=limit*contextcompact.SafeInputPercent/100
}


// The learned estimate is only a second opinion for the optional epoch
// wire gate. History compaction, adaptive provider limits and hard wire
// validation are unchanged. Require extra headroom (60% instead of 70%).
func agentCacheEpochWireFits(model agentchat.ModelConfig,m agentchat.ProviderPayloadMeasurement,hint *agentchat.CacheTokenHint,api string,body []byte)(bool,bool){
 if !agentCollectionBudgetPressure(model,m){return true,false}
 if hint==nil||m.Stateful{return false,false}
 estimate,ok:=hint.Estimate(api,body)
 limit:=model.MaxInputTokens;if limit<=0{limit=contextcompact.DefaultInputTokens}
 return ok&&estimate<limit*60/100,ok&&estimate<limit*60/100
}
