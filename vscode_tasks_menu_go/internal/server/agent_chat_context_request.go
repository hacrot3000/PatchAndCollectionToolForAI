package server

import (
 "fmt"
 "encoding/json"

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
func(s *Server) agentBuildCachedContextRequest(store *agentchat.Store,x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,c agentchat.RequestConfig)([]byte,error){
 if store==nil||c.Mode!=agentchat.Stateless{return agentBuildContextRequest(x,model,p,c)}
 old,_:=store.ReadCacheEpoch(x.ID,x.Owner,x.Revision)
 body,next,reason,err:=agentBuildContextEpoch(x,model,p,c,old,true)
 if err==nil{
  if e:=store.WriteCacheEpoch(x.ID,x.Owner,x.Revision,next);e!=nil{reason+="; checkpoint_unavailable"}
  if next!=nil{reason+="; epoch="+next.ID}
  s.recordAgentDebugEvent(x.Owner,x.ID,"","provider_cache","cache.epoch","",reason)
 }
 return body,err
}

func agentBuildContextEpoch(x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,c agentchat.RequestConfig,old *agentchat.CacheEpoch,enabled bool)([]byte,*agentchat.CacheEpoch,string,error){
 if agentchat.ProviderUsesPromptCacheKey(p){c.PromptCacheKey=agentchat.ProviderAffinityKey(x,p,c.Model)}
 if c.Mode==agentchat.Stateless {
  x=agentProjectHistoricalCollectionPack(x,c.NativeCollectionContract)
  // The same guidance is useful before and after the advisory threshold.
  // Do not mutate the system prefix merely because an estimate crosses it.
  if c.EnableCollectionTool||c.NativeCollectionContract!=""{c.Instructions+=agentCollectionBudgetAdvice}
 }
 budgetInstructions:=c.Instructions
 if c.NativeCollectionContract!=""{budgetInstructions+="\n"+c.NativeCollectionContract}
 policy:=agentContextPolicy(x,model,budgetInstructions,c.Next,false)
 scoped:=policy;scoped.Next=nil
 scopeBytes,_:=json.Marshal([]any{"cache-epoch-v1",agentchat.RunProviderConfigVersion(p,model),c.Model,c.Mode,c.ReasoningEffort,c.EnablePatchTool,c.EnableCollectionTool,c.NativeCollectionContract,c.PromptCacheKey,scoped})
 scope:=agentchat.BodyDigest(scopeBytes)
 reason:="epoch_disabled"
 if enabled&&c.Mode==agentchat.Stateless{
  var reused bool
  c.History,reason,reused=contextcompact.ReuseCacheEpoch(x.History,policy,scope,old)
  if reused{
   if body,e:=agentchat.BuildResponsesRequest(c);e==nil{
    if m,e:=agentchat.MeasureProviderPayload(p.APIType,body);e==nil&&!agentCollectionBudgetPressure(model,m){
     if next:=contextcompact.NewCacheEpoch(x.History,c.History,policy,scope,old.ID);next!=nil{return body,next,"append_only",nil}
    }
   }
   reason="wire_budget_or_format"
  }
 }
 var last error
 targets:=[]int{0}
 if enabled&&c.Mode==agentchat.Stateless{targets=[]int{60,0}}
 for _,target:=range targets{
  for _,minimal:=range []bool{false,true}{
   policy.Minimal=minimal;policy.TargetInputPercent=target
   var stats contextcompact.Stats
   c.History,stats=contextcompact.Build(x.History,policy)
   if target!=0&&stats.BudgetExceeded{continue}
   body,err:=agentchat.BuildResponsesRequest(c)
   if err==nil{
    var m agentchat.ProviderPayloadMeasurement
    m,err=agentchat.MeasureProviderPayload(p.APIType,body)
    if err==nil{
     var checkpoint *agentchat.CacheEpoch
     if enabled&&c.Mode==agentchat.Stateless&&!minimal&&!agentCollectionBudgetPressure(model,m){
      checkpoint=contextcompact.NewCacheEpoch(x.History,c.History,policy,scope,"")
     }
     return body,checkpoint,reason,nil
    }
   }
   last=err
  }
 }
 return nil,nil,reason,fmt.Errorf("mandatory context or provider format cannot fit; exact current receipt and owner instructions retained: %w",last)
}

const agentCollectionBudgetAdvice = "\nCONTEXT PRESSURE GUIDANCE (advisory, not an execution restriction): If another authorized native COLLECT is needed, request only the exact missing path, symbol, search or line range. Prefer small bounded excerpts using the installed schema's supported actions and limits. Reuse verified earlier observations and native artifact references; do not fetch an entire large source file to repeat evidence. If the task is read-only, continue useful inspection; do not invent a Patch to end the read loop. Preserve the exact current receipt and original user objective."

func agentCollectionBudgetPressure(model agentchat.ModelConfig,m agentchat.ProviderPayloadMeasurement)bool{
 limit:=model.MaxInputTokens
 if limit<=0{limit=contextcompact.DefaultInputTokens}
 // Same soft planning ceiling used by context compaction. Do not add the
 // reserved output to this input-only model limit or infer provider quota.
 return m.EstimatedInputTokens>0 && m.EstimatedInputTokens>=limit*contextcompact.SafeInputPercent/100
}
