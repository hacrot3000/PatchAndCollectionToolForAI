package server

import (
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "errors"
 "os"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

// prepareAgentProviderRun is strictly BEFORE any provider network attempt. A
// private exact-payload sidecar is synced before the journal commits PREPARED.
// The retry/resume path cannot silently regenerate different prompt content.
func prepareAgentProviderRun(store *agentchat.Store,x *agentchat.Session,
 kind,callID string,p agentchat.ProviderConfig,model agentchat.ModelConfig,payload []byte)error{
 if err:=setAgentPreparedProviderRun(x,kind,callID,p,model,payload);err!=nil{return err}
 return store.StageProviderPayload(*x,payload)
}

// setAgentPreparedProviderRun only constructs the intent; it does not stage,
// persist, authorize or send. Atomic pending admission can use it before commit.
func setAgentPreparedProviderRun(x *agentchat.Session,kind,callID string,
 p agentchat.ProviderConfig,model agentchat.ModelConfig,payload []byte)error{
 if x==nil{return agentchat.ErrInvalidSession}
 now:=time.Now().UTC()
 x.ProviderRun=&agentchat.ProviderRun{
  RequestDisplay:agentRequestDisplay(p,payload,now),
  ID:fmt.Sprintf("r%d-e%d",x.Revision+1,x.EventSeq+1),
  Kind:kind,Phase:"prepared",CallID:callID,
  PayloadSHA256:agentchat.ProviderPayloadDigest(payload),
  ProviderFingerprint:agentchat.CapabilityFingerprint(p,model),
  EstimatedInputTokens:uint64(contextcompact.Tokens(string(payload))),
  CreatedAt:now,UpdatedAt:now,
 }
 return nil
}

// Committing SENDING BEFORE the network is the only safe retry boundary.
// A process crash after this Save may have sent bytes to the provider;
// automatic restart recovery must treat that request as ambiguous.
func markAgentProviderSending(store *agentchat.Store,x *agentchat.Session)error{
 if x==nil||x.ProviderRun==nil||x.ProviderRun.Phase!="prepared"{
  return agentchat.ErrInvalidSession
 }
 candidate:=*x
 run:=*x.ProviderRun
 candidate.ProviderRun=&run
 run.Phase="sending"
 run.UpdatedAt=time.Now().UTC()
 if run.RequestAt!=""{run.RequestAt=run.UpdatedAt.Format(time.RFC3339Nano)}
 candidate.Events=append(x.Events[:0:0],x.Events...)
 candidate.AppendEvent("provider_egress",candidate.Status,run.CallID,run.Kind)
 saved,err:=store.CommitProviderSending(candidate,x.Revision)
 if errors.Is(err,agentchat.ErrCollectionReplanRevoked){
  *x=saved
  _=store.DeleteProviderPayload(x.ID)
  return err
 }
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
 // A successful response cannot become ambiguous merely because downstream
 // processing failed. New provider egress always gets a new run.
 if phase=="unknown"&&(x.ProviderRun.Phase=="acknowledged"||x.ProviderRun.Phase=="completed"){return}
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


// Report only a bounded classification and numeric sizes, never raw I/O
// errors (which can contain private paths) or request content.
func agentPreparedFailureCode(err error)string{
 switch{
 case errors.Is(err,agentchat.ErrStaleRevision):return "STALE_REVISION"
 case errors.Is(err,agentchat.ErrInvalidSession):return "INVALID_PREPARED_STATE"
 case errors.Is(err,agentchat.ErrNotFound):return "SESSION_NOT_FOUND"
 case os.IsPermission(err):return "STORAGE_PERMISSION"
 case os.IsNotExist(err):return "STORAGE_MISSING"
 default:return "STORAGE_OR_DURABILITY_FAILURE"
 }
}
func(s *Server) agentPreparedFailure(x agentchat.Session,kind string,payload []byte,err error)string{
 code:=agentPreparedFailureCode(err)
 callID:="";if x.ProviderRun!=nil{callID=x.ProviderRun.CallID}
 s.recordAgentDebugEvent(x.Owner,x.ID,callID,"provider_prepare","provider.prepare_failed",code,fmt.Sprintf("kind=%s; canonical_bytes=%d; code=%s; new request not sent",kind,len(payload),code))
 return fmt.Sprintf("%s; payload=%d bytes; request not sent",code,len(payload))
}
