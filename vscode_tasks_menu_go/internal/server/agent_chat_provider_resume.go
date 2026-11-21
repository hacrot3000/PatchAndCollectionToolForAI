package server

import (
 "context"
 "encoding/json"
 "errors"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// resumePreparedAgentProviderRun is called only by the local single-daemon
// background coordinator. It can continue ONLY an exact, checksummed request
// whose canonical journal phase is PREPARED. A SENDING/UNKNOWN request is never
// replayed after a restart; such provider egress is ambiguous.
func (s *Server) resumePreparedAgentProviderRun(ctx context.Context,store *agentchat.Store,id string) bool{
 if ctx.Err()!=nil||s.Config.SharedServerEnabled{return false}
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{return false}
 defer release()
 x,err:=store.Get(id,"local")
 if err!=nil||x.Workspace!=s.Workspace||x.Status!="requesting_ai"||
  x.ProviderRun==nil||x.ProviderRun.Phase!="prepared"{return false}
 run:=*x.ProviderRun
 fail:=func(code string){
  x.Status="failed"
  markAgentProviderOutcome(&x,"blocked")
  x.AppendEvent("provider_resume_blocked","failed",run.CallID,code)
  if run.Kind=="feedback"{
   for i:=range x.ToolCalls{
    if x.ToolCalls[i].CallID==run.CallID &&
     x.ToolCalls[i].FeedbackStatus=="prepared" {
     // No network attempt occurred. The feedback remains recorded and
     // cannot be duplicated by a new manual or automatic submission.
     x.ToolCalls[i].FeedbackStatus="failed"
    }
   }
  }
  _,_=store.Save(x,x.Revision)
 }
 identity,err:=patchtool.EnsureProjectIdentity(s.Workspace)
 if err!=nil||identity.ID!=x.ProjectID{fail("project_drift");return true}
 registry,err:=s.agentChatProviders()
 if err!=nil{fail("provider_store_unavailable");return true}
 p,err:=registry.Get("local",x.ProviderID)
 if err!=nil{fail("provider_unavailable");return true}
 model,ok:=p.FindModel(x.ModelID)
 if !ok||agentchat.CapabilityFingerprint(p,model)!=run.ProviderFingerprint{
  fail("provider_config_changed");return true
 }
 if s.ConnectionSecrets==nil{fail("credentials_unavailable");return true}
 secret,err:=s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace,"local",p.ID))
 if err!=nil||len(secret)==0{fail("credentials_unavailable");return true}
 current,payload,err:=store.PreparedProviderPayload(id,"local")
 if err!=nil{fail("prepared_payload_invalid");return true}
 if current.Revision!=x.Revision||current.ProviderRun==nil||
  current.ProviderRun.ID!=run.ID{fail("run_revision_changed");return true}
 if run.Kind=="feedback"{
  matched:=false
  for _,call:=range x.ToolCalls{
   if call.CallID==run.CallID&&call.FeedbackSent&&
    call.FeedbackStatus=="prepared"&&call.Result!=""&&
    (call.Status=="done"||call.Status=="error"||call.Status=="rejected"){
    matched=true;break
   }
  }
  if !matched{fail("feedback_receipt_not_prepared");return true}
 }
 releaseSlot,admitted:=acquireAgentSlot(ctx,aiWorkspaceHash(s.Workspace)+":local")
 if !admitted{return false}
 defer releaseSlot()
 releaseQuota,allowed:=s.reserveAIRequest(aiWorkspaceHash(s.Workspace)+":local",20)
 if !allowed{return false}
 defer releaseQuota()

 // After this durable Save, a restart cannot distinguish delivered from
 // undelivered provider bytes. Never retry this run without reconciliation.
 if run.Kind=="feedback"{
  for i:=range x.ToolCalls{
   if x.ToolCalls[i].CallID==run.CallID{
    x.ToolCalls[i].FeedbackStatus="sending"
    break
   }
  }
 }
 if err=markAgentProviderSending(store,&x);err!=nil{return true}
 turnCtx,cancel:=agentRetryTurnContext(ctx,x.RetryMode)
 defer cancel()
 unregister:=registerAgentRequest(agentRequestKey(s.Workspace,"local",id),cancel)
 defer unregister()
 result,callErr,_:=s.agentChatCallForOwner(turnCtx,"local",id,p,string(secret),payload)
 if callErr!=nil{
  x.Status="failed"
  markAgentProviderOutcome(&x,"unknown")
  x.AppendEvent("provider_resume_unknown","failed",run.CallID,"provider_acknowledgement_unconfirmed")
  if run.Kind=="feedback"{
   for i:=range x.ToolCalls{
    if x.ToolCalls[i].CallID==run.CallID{x.ToolCalls[i].FeedbackStatus="unknown"}
   }
  }
  _,_=store.Save(x,x.Revision)
  _=store.DeleteProviderPayload(id)
  return true
 }
 markAgentProviderOutcome(&x,"acknowledged")
 if run.Kind=="feedback"{
  for i:=range x.ToolCalls{
   if x.ToolCalls[i].CallID==run.CallID{x.ToolCalls[i].FeedbackStatus="acknowledged"}
  }
  x.AppendEvent("feedback_acknowledged",x.Status,run.CallID,"recovered_pre_egress")
 }
 patchToolAvailable:=model.ToolCalling&&s.agentToolCapabilityVerified("local",p,model)
 result,statelessCollection,err:=s.agentChatRunCollection(turnCtx,store,&x,
  result,p,model,string(secret),identity,patchToolAvailable)
 if err!=nil{
  x.Status="failed"
  markAgentProviderOutcome(&x,"unknown")
  x.AppendEvent("provider_resume_collect_failed","failed",run.CallID,"continuation_unconfirmed")
  _,_=store.Save(x,x.Revision)
  _=store.DeleteProviderPayload(id)
  return true
 }
 appendAgentProviderResult(&x,result,patchToolAvailable)
 var wire map[string]json.RawMessage
 _=json.Unmarshal(payload,&wire)
 _,prev:=wire["previous_response_id"]
 _,conv:=wire["conversation"]
 if (!prev&&!conv)||statelessCollection{
  x.PreviousResponseID=""
  x.ConversationID=""
 }
 appendAgentTextPatchProposal(&x,result,identity)
 if len(x.ToolCalls)>0{
  if snap,e:=agentWorkspaceSnapshotOf(turnCtx,s.Workspace);e==nil{
   for i:=range x.ToolCalls{
    call:=&x.ToolCalls[i]
    if call.Status=="pending" && call.ExpectedHead=="" && !call.GitIsRepo{
     call.ExpectedHead=snap.Head;call.ExpectedBranch=snap.Branch
     call.WorkspaceStatusSHA256=snap.StatusSHA256
     call.GitIsRepo=snap.IsRepo
    }
   }
  }
 }
 markAgentProviderOutcome(&x,"completed")
 x.AppendEvent("provider_resume_finished",x.Status,run.CallID,"prepared_only")
 if _,err=store.Save(x,x.Revision);err==nil{
  _=store.DeleteProviderPayload(id)
 }
 return true
}

// In-process workers use existing session locks. This timestamp is a short
// refresh hint for monitoring; it is never evidence of provider acknowledgement.
var _=time.Now
var _=errors.Is
