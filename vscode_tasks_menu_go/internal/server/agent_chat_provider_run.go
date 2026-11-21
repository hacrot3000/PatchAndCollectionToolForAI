package server

import (
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
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

// requestDigestShort is a non-secret value for diagnosable handoff events.
// It must never expose actual prompt/tool-result contents.
func agentProviderPayloadShortHash(payload []byte)string{
 h:=sha256.Sum256(payload);return hex.EncodeToString(h[:8])
}
