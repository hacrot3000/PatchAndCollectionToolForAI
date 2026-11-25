package server

import (
 "encoding/json"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

// Learn only from an explicitly rejected context followed by a safely
// smaller successful request. No timeout or generic 400 changes the hint.
func agentLearnContextRecovery(store *agentchat.Store,x agentchat.Session,p agentchat.ProviderConfig,key string,rejected,accepted []byte)bool{
 if store==nil{return false}
 var old,wire struct{
  Model string `json:"model"`
  Output int `json:"max_output_tokens"`
  Input json.RawMessage `json:"input"`
 }
 if json.Unmarshal(rejected,&old)!=nil||json.Unmarshal(accepted,&wire)!=nil||
  wire.Model==""||wire.Model!=old.Model||wire.Output<=0||len(wire.Input)>len(old.Input)||wire.Output>old.Output||
  (len(wire.Input)==len(old.Input)&&wire.Output==old.Output){return false}
 model,ok:=p.FindModel(wire.Model);if !ok{return false}
 budget:=contextcompact.Tokens(string(rejected))/2
 if budget<4096{budget=4096}
 return store.RememberAdaptiveBudgetHint(x,agentchat.RunProviderConfigVersion(p,model),agentchat.RunCredentialFingerprint([]byte(key)),budget,wire.Output)==nil
}

// Apply before PREPARED, never change configured model or mandatory input.
// Missing/corrupt/expired hints fall back to configured limits.
func agentAdaptiveRequestBudget(store *agentchat.Store,x agentchat.Session,p agentchat.ProviderConfig,model agentchat.ModelConfig,key string,limit int)(agentchat.ModelConfig,int){
 if store==nil{return model,limit}
 b,ok,err:=store.AdaptiveBudgetHint(x,agentchat.RunProviderConfigVersion(p,model),agentchat.RunCredentialFingerprint([]byte(key)))
 if err!=nil||!ok{return model,limit}
 configured:=model.MaxInputTokens
 if configured<=0{configured=contextcompact.DefaultInputTokens}
 if b.Input<configured{model.MaxInputTokens=b.Input}
 if b.Output<limit{limit=b.Output}
 return model,limit
}
