package server

import (
 "fmt"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

// Validate the final serialized target format before PREPARED. On local
// build/format failure only, retry a minimal optional-history projection.
// Do not turn a token estimate into a new hard read quota.
func agentBuildContextRequest(x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,c agentchat.RequestConfig)([]byte,error){
 if agentchat.ProviderUsesPromptCacheKey(p){c.PromptCacheKey=agentchat.ProviderAffinityKey(x,p,c.Model)}
 if c.Mode==agentchat.Stateless {x=agentProjectHistoricalCollectionPack(x,c.NativeCollectionContract)}
 budgetInstructions:=c.Instructions
 if c.NativeCollectionContract!=""{budgetInstructions+="\n"+c.NativeCollectionContract}
 var last error
 for _,minimal:=range []bool{false,true}{
  c.History=agentContextForRequestWithPolicy(x,model,budgetInstructions,c.Next,minimal)
  body,err:=agentchat.BuildResponsesRequest(c)
  if err==nil{
   var measurement agentchat.ProviderPayloadMeasurement
   measurement,err=agentchat.MeasureProviderPayload(p.APIType,body)
   if err==nil{
    // The model receives this advice before it proposes another native
    // read. The estimate is advisory, not a new admission or read quota.
    if c.Mode==agentchat.Stateless && (c.EnableCollectionTool || c.NativeCollectionContract!="") && agentCollectionBudgetPressure(model,measurement){
     advised:=c
     advised.Instructions+=agentCollectionBudgetAdvice
     advised.History=agentContextForRequestWithPolicy(x,model,budgetInstructions+agentCollectionBudgetAdvice,c.Next,minimal)
     if candidate,e:=agentchat.BuildResponsesRequest(advised);e==nil{
      if _,e=agentchat.MeasureProviderPayload(p.APIType,candidate);e==nil{return candidate,nil}
     }
     // Never fail a valid mandatory request merely to add optional advice.
    }
    return body,nil
   }
  }
  last=err
 }
 return nil,fmt.Errorf("mandatory context or provider format cannot fit; exact current receipt and owner instructions retained: %w",last)
}

const agentCollectionBudgetAdvice = "\nCONTEXT PRESSURE (estimated, not an execution restriction): If another authorized native COLLECT is needed, request only the exact missing path, symbol, search or line range. Prefer small bounded excerpts using the installed schema's supported actions and limits. Reuse verified earlier observations and native artifact references; do not fetch an entire large source file to repeat evidence. If the task is read-only, continue useful inspection; do not invent a Patch to end the read loop. Preserve the exact current receipt and original user objective."

func agentCollectionBudgetPressure(model agentchat.ModelConfig,m agentchat.ProviderPayloadMeasurement)bool{
 limit:=model.MaxInputTokens
 if limit<=0{limit=contextcompact.DefaultInputTokens}
 // Same soft planning ceiling used by context compaction. Do not add the
 // reserved output to this input-only model limit or infer provider quota.
 return m.EstimatedInputTokens>0 && m.EstimatedInputTokens>=limit*contextcompact.SafeInputPercent/100
}
