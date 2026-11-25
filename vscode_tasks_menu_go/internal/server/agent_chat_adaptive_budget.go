package server

import (
 "encoding/json"
 "sync"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

// Bounded process-local hints, not authoritative provider token counts.
// Scope by workspace, owner, provider/model configuration and credential
// fingerprint. Never retain credentials, payloads or canonical history.
type agentAdaptiveBudgetKey struct {
 workspace, owner, version, credential string
}
type agentAdaptiveBudget struct {
 input, output int
 expires time.Time
}
const agentAdaptiveBudgetTTL = 24*time.Hour
const agentAdaptiveBudgetCapacity = 256
var agentAdaptiveBudgets = struct {
 sync.Mutex
 entries map[agentAdaptiveBudgetKey]agentAdaptiveBudget
}{entries:make(map[agentAdaptiveBudgetKey]agentAdaptiveBudget)}

func agentAdaptiveKey(workspace,owner string,p agentchat.ProviderConfig,m agentchat.ModelConfig,key string) agentAdaptiveBudgetKey {
 return agentAdaptiveBudgetKey{workspace,owner,agentchat.RunProviderConfigVersion(p,m),agentchat.RunCredentialFingerprint([]byte(key))}
}
func agentAdaptiveLookup(k agentAdaptiveBudgetKey) (agentAdaptiveBudget,bool) {
 agentAdaptiveBudgets.Lock()
 defer agentAdaptiveBudgets.Unlock()
 b,ok:=agentAdaptiveBudgets.entries[k]
 if ok && !time.Now().Before(b.expires) {
  delete(agentAdaptiveBudgets.entries,k)
  return agentAdaptiveBudget{},false
 }
 return b,ok
}
func agentAdaptiveRemember(k agentAdaptiveBudgetKey,b agentAdaptiveBudget) {
 if b.input<=0||b.output<=0{return}
 agentAdaptiveBudgets.Lock()
 defer agentAdaptiveBudgets.Unlock()
 now:=time.Now()
 for key,old:=range agentAdaptiveBudgets.entries{
  if !now.Before(old.expires){delete(agentAdaptiveBudgets.entries,key)}
 }
 if old,ok:=agentAdaptiveBudgets.entries[k];ok {
  if old.input<b.input{b.input=old.input}
  if old.output<b.output{b.output=old.output}
 }else if len(agentAdaptiveBudgets.entries)>=agentAdaptiveBudgetCapacity{
  var oldest agentAdaptiveBudgetKey
  var expiry time.Time
  for key,old:=range agentAdaptiveBudgets.entries{
   if expiry.IsZero()||old.expires.Before(expiry){oldest,expiry=key,old.expires}
  }
  delete(agentAdaptiveBudgets.entries,oldest)
 }
 b.expires=now.Add(agentAdaptiveBudgetTTL)
 agentAdaptiveBudgets.entries[k]=b
}

// Learn only after an explicitly rejected context was reduced and accepted.
// Retain both the retry compactor's budget and the accepted output reservation.
func agentLearnContextRecovery(workspace,owner string,p agentchat.ProviderConfig,key string,rejected,accepted []byte) bool {
 var old,wire struct{
  Model string `json:"model"`
  Output int `json:"max_output_tokens"`
 }
 if json.Unmarshal(rejected,&old)!=nil||json.Unmarshal(accepted,&wire)!=nil||
  wire.Model==""||wire.Model!=old.Model||wire.Output<=0{return false}
 model,ok:=p.FindModel(wire.Model)
 if !ok{return false}
 budget:=contextcompact.Tokens(string(rejected))/2
 if budget<4096{budget=4096}
 agentAdaptiveRemember(agentAdaptiveKey(workspace,owner,p,model,key),agentAdaptiveBudget{input:budget,output:wire.Output})
 return true
}

// Apply before durable PREPARED, not at egress. The original configured
// model remains authoritative for credentials, version and receipts.
func agentAdaptiveRequestBudget(x agentchat.Session,p agentchat.ProviderConfig,model agentchat.ModelConfig,key string,limit int)(agentchat.ModelConfig,int){
 b,ok:=agentAdaptiveLookup(agentAdaptiveKey(x.Workspace,x.Owner,p,model,key))
 if !ok{return model,limit}
 configured:=model.MaxInputTokens
 if configured<=0{configured=contextcompact.DefaultInputTokens}
 if b.input<configured{model.MaxInputTokens=b.input}
 if b.output<limit{limit=b.output}
 return model,limit
}
