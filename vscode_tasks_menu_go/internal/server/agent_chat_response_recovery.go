package server

import (
 "context"
 "encoding/json"
 "errors"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const agentResponseSaveAttempts = 3

// This path retries ONLY local journal storage. It has no provider client,
// execution callback or native queue access. Serialize once before Save so
// failed archive compaction cannot mutate the next retry's candidate.
func (s *Server) saveAgentReceivedResponse(store *agentchat.Store, candidate agentchat.Session)(agentchat.Session,error){
 raw,err:=json.Marshal(candidate)
 if err!=nil{return agentchat.Session{},err}
 if candidate.ProviderRun==nil{return agentchat.Session{},agentchat.ErrInvalidSession}
 runID:=candidate.ProviderRun.ID
 stageErr:=store.StageProviderResponse(candidate)
 if stageErr!=nil{
  s.recordAgentResponseSaveDiagnostic(candidate,"response.checkpoint_failed",stageErr)
  // A binding or conflicting checkpoint error is not a storage retry.
  if errors.Is(stageErr,agentchat.ErrInvalidSession)||errors.Is(stageErr,agentchat.ErrStaleRevision){return agentchat.Session{},stageErr}
 }
 var last error
 for attempt:=0;attempt<agentResponseSaveAttempts;attempt++{
  // A previous rename may have committed even when directory sync failed.
  // Accept only the canonical same-run completed projection.
  current,e:=store.Get(candidate.ID,candidate.Owner)
  if e==nil&&agentResponseAlreadyCommitted(current,candidate){
   _=store.ClearCommittedProviderResponse(candidate.ID,candidate.Owner,runID)
   return current,nil
  }
  if stageErr==nil{
   var found bool
   current,found,last=store.RecoverProviderResponse(candidate.ID,candidate.Owner)
   if last==nil&&found{return current,nil}
   if last==nil{last=agentchat.ErrStaleRevision}
  }else{
   // Disk may be too full even for a sidecar; a regular journal write can
   // still succeed after compaction. Never abandon that chance.
   var fresh agentchat.Session
   if e=json.Unmarshal(raw,&fresh);e!=nil{return agentchat.Session{},e}
   current,last=store.Save(fresh,fresh.Revision)
   if last==nil{
    _=store.ClearCommittedProviderResponse(candidate.ID,candidate.Owner,runID)
    return current,nil
   }
  }
  s.recordAgentResponseSaveDiagnostic(candidate,"response.save_failed",last)
  if errors.Is(last,agentchat.ErrStaleRevision){break}
  if attempt+1<agentResponseSaveAttempts{time.Sleep(time.Duration(attempt+1)*50*time.Millisecond)}
 }
 // Preserve the exact candidate if the initial checkpoint write failed.
 // This never changes the primary journal or retries the network.
 if stageErr!=nil{
  if e:=store.StageProviderResponse(candidate);e!=nil{s.recordAgentResponseSaveDiagnostic(candidate,"response.checkpoint_failed",e)}
 }
 return agentchat.Session{},last
}

func agentResponseAlreadyCommitted(current,candidate agentchat.Session)bool{
 return current.ID==candidate.ID&&current.Owner==candidate.Owner&&current.ProjectID==candidate.ProjectID&&
  current.Workspace==candidate.Workspace&&current.Revision==candidate.Revision+1&&
  current.ProviderRun!=nil&&candidate.ProviderRun!=nil&&current.ProviderRun.ID==candidate.ProviderRun.ID&&
  current.ProviderRun.PayloadSHA256==candidate.ProviderRun.PayloadSHA256&&current.ProviderRun.Phase=="completed"&&
  current.Status==candidate.Status&&current.Status!="requesting_ai"&&current.EventSeq>=candidate.EventSeq
}

func (s *Server) recordAgentResponseSaveDiagnostic(x agentchat.Session,event string,err error){
 code:=agentchat.ProviderResponseSaveErrorCode(err)
 s.recordAgentDebugEvent(x.Owner,x.ID,"","response_storage",event,code,"Local response journal: "+code)
}

// Recover only an abandoned Legacy turn under its cross-process lease.
// The attempt ledger is telemetry, not a response body or replay authority.
// With no checkpoint it can explain the loss, but cannot reconstruct content.
func (s *Server) recoverAgentReceivedResponseLocked(store *agentchat.Store,x agentchat.Session)(agentchat.Session,bool,error){
 if x.Status!="requesting_ai"||x.ProviderRun==nil||
  (x.ProviderRun.Phase!="sending"&&x.ProviderRun.Phase!="acknowledged"){return x,false,nil}
 if s.agentConversationRequiresRunContinuation(x.Owner,x.ID){return x,false,nil}
 saved,found,err:=store.RecoverProviderResponse(x.ID,x.Owner)
 if found&&err==nil{
  s.recordAgentDebugEvent(x.Owner,x.ID,"","response_storage","response.recovered","completed","Recovered exact local response checkpoint; no provider request replayed")
  return saved,true,nil
 }
 // Re-read after any ambiguous journal rename, and do not overwrite a newer
 // revision. If the candidate was committed, the primary journal wins.
 fresh,e:=store.Get(x.ID,x.Owner)
 if e!=nil{return x,true,e}
 if fresh.Revision!=x.Revision||fresh.Status!="requesting_ai"{return fresh,true,nil}
 if found&&err!=nil{s.recordAgentResponseSaveDiagnostic(fresh,"response.recovery_failed",err)}
 code:="orphaned_provider_run"
 if found{
  code="provider_response_save_failed"
 }else if audit,auditErr:=store.ProviderAttempts(x.ID,x.Owner);auditErr==nil&&audit!=nil&&audit.RunID==x.ProviderRun.ID&&len(audit.Attempts)>0{
  last:=audit.Attempts[len(audit.Attempts)-1]
  if last.Outcome==agentchat.AttemptResponse&&last.PayloadSHA256==x.ProviderRun.PayloadSHA256{
   code="provider_response_not_saved"
  }
 }
 // Leave unknown egress fenced. A local save failure is not grounds to
 // automatically resend the old user message or tool feedback.
 fresh.Status="failed"
 agentCloseInterruptedCollection(&fresh)
 fresh.PreviousResponseID=""
 fresh.ConversationID=""
 markAgentProviderOutcome(&fresh,"unknown")
 agentClearActiveTurnBinding(&fresh)
 for i:=range fresh.ToolCalls{
  if fresh.ToolCalls[i].FeedbackStatus=="sending"{fresh.ToolCalls[i].FeedbackStatus="unknown"}
 }
 fresh.AppendEvent("turn_interrupted","failed","",code)
 saved,e=store.Save(fresh,fresh.Revision)
 if e!=nil{s.recordAgentResponseSaveDiagnostic(fresh,"response.recovery_failed",e);return fresh,true,e}
 s.recordAgentDebugEvent(x.Owner,x.ID,"","response_storage","response.recovery_required",code,"Abandoned turn closed without provider replay; inspect conversation before continuing")
 return saved,true,nil
}

// The scheduler invokes this only for a stale active marker. Failure to
// acquire the same lease is proof we must leave the other worker alone.
func (s *Server) recoverAgentReceivedResponseOne(ctx context.Context,store *agentchat.Store,id string)bool{
 if ctx.Err()!=nil{return false}
 x,err:=store.Get(id,"local")
 if err!=nil||x.Workspace!=s.Workspace||!agentResponseRecoveryCandidate(x){return false}
 if agentRequestActive(s.Workspace,"local",id){return true}
 release,ok:=lockAgentTurn(s.agentOwnerWorkspace(),"local",id)
 if !ok{return true}
 defer release()
 x,err=store.Get(id,"local")
 if err!=nil||x.Workspace!=s.Workspace||!agentResponseRecoveryCandidate(x){return false}
 _,handled,_:=s.recoverAgentReceivedResponseLocked(store,x)
 return handled
}

func agentResponseRecoveryCandidate(x agentchat.Session)bool{
 return !x.Archived&&x.Status=="requesting_ai"&&x.ProviderRun!=nil&&
  (x.ProviderRun.Phase=="sending"||x.ProviderRun.Phase=="acknowledged")&&
  time.Since(x.ProviderRun.UpdatedAt)>=30*time.Second
}
