package server

import (
 "fmt"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Validate the final serialized target format before PREPARED. On local
// build/format failure only, retry a minimal optional-history projection.
// Do not turn a token estimate into a new hard read quota.
func agentBuildContextRequest(x agentchat.Session,model agentchat.ModelConfig,p agentchat.ProviderConfig,c agentchat.RequestConfig)([]byte,error){
 budgetInstructions:=c.Instructions
 if c.NativeCollectionContract!=""{budgetInstructions+="\n"+c.NativeCollectionContract}
 var last error
 for _,minimal:=range []bool{false,true}{
  c.History=agentContextForRequestWithPolicy(x,model,budgetInstructions,c.Next,minimal)
  body,err:=agentchat.BuildResponsesRequest(c)
  if err==nil{
   _,err=agentchat.MeasureProviderPayload(p.APIType,body)
   if err==nil{return body,nil}
  }
  last=err
 }
 return nil,fmt.Errorf("mandatory context or provider format cannot fit; exact current receipt and owner instructions retained: %w",last)
}
