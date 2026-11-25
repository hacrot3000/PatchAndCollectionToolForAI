package server

import (
 "strconv"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Recover only an old journal that proves this exact provider run returned
// before LOCAL identity validation failed. UsageRecorded is set only after
// successful provider results. The event sequence binds it to the current run.
// No uploaded report, pre-egress attempt ID or timestamp grants recovery.
func agentRecoverAcknowledgedCollection(x *agentchat.Session) bool {
 if x==nil||x.Owner!="local"||x.Archived||x.Status!="failed"||x.ProviderRun==nil {return false}
 run:=x.ProviderRun
 if run.Kind!="collection"||run.Phase!="unknown"||!run.UsageRecorded||len(run.PayloadSHA256)!=64 {return false}
 prefix,seqText,ok:=strings.Cut(run.ID,"-e")
 if !ok||!strings.HasPrefix(prefix,"r"){return false}
 if _,err:=strconv.ParseUint(strings.TrimPrefix(prefix,"r"),10,64);err!=nil{return false}
 seq,err:=strconv.ParseUint(seqText,10,64)
 if err!=nil||seq==0||seq>^uint64(0)-2{return false}
 if x.EventSeq!=seq+2||len(x.Events)<3{return false}
 n:=len(x.Events)
 egress,rejected,failed:=x.Events[n-3],x.Events[n-2],x.Events[n-1]
 if egress.Seq!=seq||egress.Kind!="provider_egress"||egress.CallID!=run.CallID||egress.Detail!="collection"||
  rejected.Seq!=seq+1||rejected.Kind!="collect_request_rejected"||rejected.Status!="failed"||
  !(strings.HasPrefix(rejected.Detail,"project identity does not match this TaskDeck workspace")||strings.HasPrefix(rejected.Detail,"invalid project identity:")||strings.HasPrefix(rejected.Detail,"missing project.key/project.id")||strings.HasPrefix(rejected.Detail,"text COLLECT must include exact project.key and project.id"))||
  failed.Seq!=seq+2||failed.Kind!="collect_failed"||failed.Status!="failed"||failed.Detail!="collection_continuation" {return false}
 // Preserve task status and history. This is not recovery of the missing
 // response body and does not authorize replay of the old provider request.
 markAgentProviderOutcome(x,"acknowledged")
 x.PreviousResponseID="";x.ConversationID=""
 x.AppendEvent("provider_ack_recovered","failed",run.CallID,run.ID+": verified local response accounting and exact Collection rejection; do not replay original request")
 return true
}


// A response checkpoint is an exact current-run marker in canonical local
// history. If interrupted after acknowledgement, never re-run native work:
// close the pending call with an explicitly unverified observation instead.
func agentCloseInterruptedCollection(x *agentchat.Session)bool{
 if x==nil||x.ProviderRun==nil||x.ProviderRun.Phase!="acknowledged"{return false}
 var event *agentchat.SessionEvent
 for i:=len(x.Events)-1;i>=0;i--{
  e:=&x.Events[i]
  if e.Kind=="collect_response_checkpoint"&&e.Detail==x.ProviderRun.ID{event=e;break}
 }
 if event==nil{return false}
 const diagnostic=`{"status":"FAILED","error":{"code":"COLLECTION_LOCAL_INTERRUPTED","message":"The acknowledged native Collection response was interrupted during local processing. No verified evidence is available for this call. Inspect native history before any potentially mutating operation; do not replay the old request.","retry_allowed":false}}`
 if event.CallID!=""{
  found:=false
  for _,it:=range x.History{
   if it.CallID!=event.CallID{continue}
   if it.Type=="function_call_output"{x.PreviousResponseID="";x.ConversationID="";return false}
   if it.Type=="function_call"&&it.Name=="taskdeck_collect_workspace"{found=true}
  }
  if !found{return false}
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call_output",CallID:event.CallID,Content:diagnostic})
 }else{
  if len(x.History)==0||x.History[len(x.History)-1].Type!="assistant"{x.PreviousResponseID="";x.ConversationID="";return false}
  x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"TaskDeck local Collection checkpoint: "+diagnostic})
 }
 x.PreviousResponseID="";x.ConversationID=""
 x.AppendEvent("collect_checkpoint_closed",x.Status,event.CallID,x.ProviderRun.ID)
 return true
}


// Automatic recovery is limited to a proven legacy local validation failure,
// one new continuation per recovered run and only an existing owner grant.
// An exhausted repair budget, interrupted native execution, outstanding Patch
// or uncertain feedback must remain reviewable rather than loop on restart.
func agentRecoveredCollectionResumeCandidate(x agentchat.Session)bool{
 if x.Owner!="local"||x.Archived||x.Status!="failed"||x.ProviderRun==nil||
  x.ProviderRun.Phase!="acknowledged"||x.ProviderRun.Kind!="collection"||
  (x.AutoCollection!=nil&&!*x.AutoCollection)||agentchat.HasUnresolvedWork(x){return false}
 if x.TaskPlan!=nil{
  if !x.TaskPlan.AutoContinue||agentchat.TaskPlanNext(x.TaskPlan)==nil||x.TaskPlan.NoProgressTurns>=agentchat.MaxContinuationNoWorkTurns{return false}
 }else if !x.AutoContinueText||(x.Continuation!=nil&&x.Continuation.NoProgressTurns>=agentchat.MaxContinuationNoWorkTurns){return false}
 for _,call:=range x.ToolCalls{
  if !call.FeedbackSent||call.FeedbackStatus!="acknowledged"{return false}
 }
 for i:=len(x.Events)-1;i>=0;i--{
  e:=x.Events[i]
  if e.Kind=="collection_resume_attempted"&&e.Detail==x.ProviderRun.ID{return false}
  if e.Kind=="provider_ack_recovered"&&e.CallID==x.ProviderRun.CallID&&strings.HasPrefix(e.Detail,x.ProviderRun.ID+": "){return true}
 }
 return false
}

// Unknown Collection delivery is not an acknowledgement. It is eligible for
// a NEW user-authorized stateless continuation only, with exact read evidence
// and no unresolved mutation. Respect the live retry policy and bound retries.
func agentUnknownCollectionRecoveryState(x agentchat.Session, now time.Time) string {
 if !agentchat.HasUnknownCollectionDelivery(x)||x.Status!="failed"||x.Owner!="local"||x.Archived{return ""}
 if !agentchat.CollectionAutoResumeAllowed(x){return "owner_grant_or_receipt_unavailable"}
 run:=x.ProviderRun
 attempts:=0
 var deferredAt time.Time
 for i:=len(x.Events)-1;i>=0;i--{
  e:=x.Events[i]
  if e.Kind=="turn_finished"||e.Kind=="feedback_turn_finished"||e.Kind=="provider_resume_finished"{break}
  if e.Kind=="collection_auto_resume_new"{attempts++}
  if e.Kind=="collection_auto_resume_deferred"&&e.Detail==run.ID&&e.CallID==run.CallID{
   attempts++
   if deferredAt.IsZero(){deferredAt=e.At}
  }
 }
 limit:=1
 if x.RetryMode==agentchat.RetryLimited{limit=6}
 if x.RetryMode!=agentchat.RetryUnlimited&&attempts>=limit{return "retry_exhausted"}
 if !deferredAt.IsZero()&&now.Before(deferredAt.Add(30*time.Second)){return "retry_deferred"}
 return "ready"
}

func agentUnknownCollectionResumeCandidate(x agentchat.Session)bool{
 return agentUnknownCollectionRecoveryState(x,time.Now().UTC())=="ready"
}


// Called only with the exact conversation turn lease held and a freshly read
// canonical revision. Close an acknowledged local read checkpoint without
// replaying provider egress or inventing a successful native receipt.
func agentCancelOrphanedCollection(x *agentchat.Session) bool {
 if x==nil||x.Owner!="local"||x.Archived||x.Status!="requesting_ai"||
  x.ProviderRun==nil||x.ProviderRun.Phase!="acknowledged"{return false}
 candidate:=*x
 candidate.Status="cancelled"
 if agentchat.HasUnresolvedWork(candidate){return false}
 for _,call:=range candidate.ToolCalls{
  if call.Name=="taskdeck_submit_patch"&&(!call.FeedbackSent||call.FeedbackStatus!="acknowledged"){return false}
 }
 // Avoid mutating shared backing arrays if the candidate fails validation.
 candidate.History=append([]agentchat.HistoryItem(nil),x.History...)
 candidate.Events=append([]agentchat.SessionEvent(nil),x.Events...)
 if !agentCloseInterruptedCollection(&candidate){return false}
 candidate.AppendEvent("turn_cancelled","cancelled","","user_stop_orphaned_collect")
 *x=candidate
 return true
}
