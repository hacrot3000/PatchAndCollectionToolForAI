package server

import (
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

// prepareAgentProviderRun is strictly BEFORE any provider network attempt. A
// private exact-payload sidecar is synced before the journal commits PREPARED.
// The retry/resume path cannot silently regenerate different prompt content.
func prepareAgentProviderRun(store *agentchat.Store,x *agentchat.Session,
 kind,callID string,p agentchat.ProviderConfig,model agentchat.ModelConfig,payload []byte)error{
 if x==nil{return agentchat.ErrInvalidSession}
 now:=time.Now().UTC()
 x.ProviderRun=&agentchat.ProviderRun{
  ID:fmt.Sprintf("r%d-e%d",x.Revision+1,x.EventSeq+1),
  Kind:kind,Phase:"prepared",CallID:callID,
  PayloadSHA256:agentchat.ProviderPayloadDigest(payload),
  ProviderFingerprint:agentchat.CapabilityFingerprint(p,model),
  EstimatedInputTokens:uint64(contextcompact.Tokens(string(payload))),
  CreatedAt:now,UpdatedAt:now,
 }
 return store.StageProviderPayload(*x,payload)
}

// Committing SENDING BEFORE the network is the only safe retry boundary.
// A process crash after this Save may have sent bytes to the provider;
// automatic restart recovery must treat that request as ambiguous.
func markAgentProviderSending(store *agentchat.Store,x *agentchat.Session)error{
 if x==nil||x.ProviderRun==nil||x.ProviderRun.Phase!="prepared"{
  return agentchat.ErrInvalidSession
 }
 x.ProviderRun.Phase="sending"
 x.ProviderRun.UpdatedAt=time.Now().UTC()
 x.AppendEvent("provider_egress",x.Status,x.ProviderRun.CallID,x.ProviderRun.Kind)
 saved,err:=store.Save(*x,x.Revision)
 if err!=nil{return err}
 *x=saved
 // The exact payload is needed only while PREPARED. Once the durable
 // SENDING record exists, retaining it cannot help restart recovery.
 _=store.DeleteProviderPayload(x.ID)
 return nil
}
func markAgentProviderOutcome(x *agentchat.Session,phase string){
 if x==nil||x.ProviderRun==nil{return}
 switch phase{case "acknowledged","unknown","completed","blocked":default:return}
 x.ProviderRun.Phase=phase
 x.ProviderRun.UpdatedAt=time.Now().UTC()
}

// recordAgentTokenUsage runs ONLY after a provider returned a successful
// response. It never counts speculative SENDING/UNKNOWN network attempts and
// never guesses usage for pre-upgrade historical turns. Some providers do not
// report token usage; those responses are clearly marked estimated.
func recordAgentTokenUsage(x *agentchat.Session,result agentchat.ResponsesResult){
 if x==nil||x.ProviderRun==nil||x.ProviderRun.UsageRecorded{return}
 estimate:=x.ProviderRun.EstimatedInputTokens
 in:=estimate
 out:=uint64(contextcompact.Tokens(result.Text))
 for _,call:=range result.Calls{
  out+=uint64(contextcompact.Tokens(call.Arguments))
 }
 approximate:=false
 if result.InputTokens>0{in=uint64(result.InputTokens)}else{approximate=true}
 if result.OutputTokens>0{out=uint64(result.OutputTokens)}else{approximate=true}
 const maxTokens=uint64(100000000000000)
 if x.TokenUsage.InputTokens>maxTokens-in||
    x.TokenUsage.OutputTokens>maxTokens-out{return}
 x.TokenUsage.InputTokens+=in
 x.TokenUsage.OutputTokens+=out
 x.TokenUsage.Requests++
 if approximate{x.TokenUsage.EstimatedRequests++}
 x.ProviderRun.UsageRecorded=true
}

// requestDigestShort is a non-secret value for diagnosable handoff events.
// It must never expose actual prompt/tool-result contents.
func agentProviderPayloadShortHash(payload []byte)string{
 h:=sha256.Sum256(payload);return hex.EncodeToString(h[:8])
}
