package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "strings"

 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

const maxCollectionGuardBytes=96<<10

// CollectionGuardState is a versioned, non-authorizing projection in the
// existing session journal. The raw field preserves unknown future versions;
// malformed metadata disables guard decisions without discarding history.
type CollectionGuardState struct {
 Version int `json:"version"`
 ScopeDigest string `json:"scope_digest"`
 GoalDigest string `json:"goal_digest"`
 Generation uint64 `json:"generation"`
 AcceptedStep bool `json:"accepted_step,omitempty"`
 Policy cp.Policy `json:"policy"`
 Counters cp.CounterState `json:"counters"`
 Evidence cp.EvidenceIndex `json:"evidence"`
 Coverage *cp.CoverageIndex `json:"coverage,omitempty"`
 Recovery cp.RecoveryState `json:"recovery"`
 Legacy LegacyCollectionSummary `json:"legacy"`
 AttemptRunID string `json:"attempt_run_id,omitempty"`
 AttemptCursor uint64 `json:"attempt_cursor,omitempty"`
 AttemptAuditIncomplete bool `json:"attempt_audit_incomplete,omitempty"`
}

// LegacyCollectionSummary describes only the bounded retained canonical
// history at migration. It is not attributed to the new objective and never
// establishes novelty, source coverage or accepted progress.
type LegacyCollectionSummary struct {
 RetainedCalls uint64 `json:"retained_calls"`
 RetainedOutputs uint64 `json:"retained_outputs"`
 AcknowledgedPatches uint64 `json:"acknowledged_patches"`
 Incomplete bool `json:"incomplete"`
}

func guardDigest(v any)string{
 b,_:=json.Marshal(v)
 hash:=sha256.Sum256(b)
 return hex.EncodeToString(hash[:])
}

func validGuardDigest(s string)bool{
 return len(s)==64&&strings.Trim(s,"0123456789abcdef")==""
}

func (x Session) collectionScopeDigest()string{
 return guardDigest([]string{x.Owner,x.ProjectID,x.Workspace,x.ID})
}

func (x Session) validCollectionGuard(g CollectionGuardState)bool{
 if g.Version!=cp.Version||g.ScopeDigest!=x.collectionScopeDigest()||!validGuardDigest(g.GoalDigest)||
  g.Generation==0||!g.Policy.Valid()||g.Counters.Version!=cp.Version||!g.Counters.Objective.Valid()||
  g.Evidence.ObjectiveKey!=g.Counters.Objective.Key||len(g.Evidence.Entries)>cp.MaxEvidenceEntries||
  g.Counters.LastSequence>x.EventSeq||g.Recovery.Attempts>cp.MaxRecoveryAttempts||
  g.Recovery.LastSequence>g.Counters.LastSequence||g.Recovery.LastAttemptReads>g.Counters.NativeReads{
  return false
 }
 if (g.Counters.LastSequence==0&&g.Counters.LastEventDigest!="")||(g.Counters.LastSequence!=0&&!validGuardDigest(g.Counters.LastEventDigest)){return false}
 if len(g.AttemptRunID)>256||(g.AttemptRunID=="")!=(g.AttemptCursor==0)||g.AttemptCursor>1000000000{return false}
 c:=g.Counters
 if g.Coverage!=nil&&!cp.ValidateCoverage(*g.Coverage,c.Objective){return false}
 if c.NativeReads>c.Observed||c.NewEvidence>c.Observed||c.DuplicateEvidence>c.Observed||
  c.UnknownEvidence>c.Observed||c.Errors>c.Observed||c.Progress.ReadsSinceProgress>c.NativeReads||
  c.Progress.DuplicateStreak>c.Progress.ReadsSinceProgress||c.ReadsSincePatchProposal>c.NativeReads||
  c.ReadsSincePatchExecution>c.NativeReads{return false}
 seen:=map[cp.ScopeDescriptor]bool{}
 for _,e:=range g.Evidence.Entries{
  if e.Scope.ObjectiveKey!=c.Objective.Key||!validGuardDigest(e.Scope.RequestDigest)||
   (e.Scope.SourceRevisionDigest!=""&&!validGuardDigest(e.Scope.SourceRevisionDigest))||
   !validGuardDigest(e.ProofDigest)||seen[e.Scope]{return false}
  seen[e.Scope]=true
 }
 return true
}

func (x Session) ReadCollectionGuard()(CollectionGuardState,bool){
 var g CollectionGuardState
 if len(x.CollectionGuard)==0||len(x.CollectionGuard)>maxCollectionGuardBytes||
  json.Unmarshal(x.CollectionGuard,&g)!=nil||!x.validCollectionGuard(g){return CollectionGuardState{},false}
 return g,true
}

// SetCollectionGuard never overwrites an opaque invalid/future record. No
// guard failure is a reason to remove or rewrite canonical source evidence.
func (x *Session) SetCollectionGuard(g CollectionGuardState)bool{
 if x==nil||!x.validCollectionGuard(g){return false}
 if len(x.CollectionGuard)!=0{if _,ok:=x.ReadCollectionGuard();!ok{return false}}
 body,err:=json.Marshal(g)
 if err!=nil||len(body)>maxCollectionGuardBytes{return false}
 x.CollectionGuard=body
 return true
}

func (x Session) retainedCollectionSummary()LegacyCollectionSummary{
 out:=LegacyCollectionSummary{Incomplete:x.ArchivedHistoryItems!=0||x.ArchivedPatchCalls!=0}
 calls:=map[string]bool{}
 for _,item:=range x.History{
  if item.Type=="function_call"&&item.Name=="taskdeck_collect_workspace"{
   calls[item.CallID]=true
   out.RetainedCalls++
  }else if item.Type=="function_call_output"&&calls[item.CallID]{
   delete(calls,item.CallID)
   out.RetainedOutputs++
  }
 }
 for _,call:=range x.ToolCalls{
  if call.Name=="taskdeck_submit_patch"&&call.FeedbackStatus=="acknowledged"{out.AcknowledgedPatches++}
 }
 if len(calls)!=0{out.Incomplete=true}
 return out
}

// BindCollectionGuard receives ONLY a real owner goal/plan step, or an
// explicitly server-originated continuation. Do not infer intent from
// provider-generated user-role history or historical AI Pack content.
func (x *Session) BindCollectionGuard(goal,step string,continuation bool)bool{
 if x==nil{return false}
 old,ok:=x.ReadCollectionGuard()
 if len(x.CollectionGuard)!=0&&!ok{return false}
 if continuation&&goal==""{return ok}
 if strings.TrimSpace(goal)==""{return false}
 digest:=guardDigest([]string{goal,step})
 if ok&&old.GoalDigest==digest{return true}
 generation:=uint64(1)
 legacy:=x.retainedCollectionSummary()
 if ok{
  if old.Generation==^uint64(0){return false}
  generation=old.Generation+1
  legacy=old.Legacy
 }
 binding,err:=cp.BindObjective(cp.ObjectiveScope{
  Owner:x.Owner,ProjectID:x.ProjectID,OwnerWorkspace:x.Workspace,ExecutionWorkspace:x.Workspace,
  ConversationID:x.ID,Goal:goal,StepID:step,Generation:generation,
 })
 if err!=nil{return false}
 counters,err:=cp.NewCounterState(binding)
 if err!=nil{return false}
 return x.SetCollectionGuard(CollectionGuardState{
  Version:cp.Version,ScopeDigest:x.collectionScopeDigest(),GoalDigest:digest,Generation:generation,
  Policy:cp.Defaults(),Counters:counters,Evidence:cp.NewEvidenceIndex(binding),Legacy:legacy,
 })
}


// Record only new transitions in the server-owned native Patch ledger.
// Called after successful session CAS validation, before archive/atomic write.
// A failed Save cannot publish either the transition or its guard metadata.
// The model's proposed Python, wrapper PASS and provider completion do not
// establish objective progress or acceptance.
func (x *Session) recordCollectionGuardPatchTransitions(previous Session){
 g,ok:=x.ReadCollectionGuard();if !ok{return}
 oldGuard,oldOK:=previous.ReadCollectionGuard()
 if oldOK&&oldGuard.Counters.Objective.Key!=g.Counters.Objective.Key{
  // Different owner objective: do not attribute previous work to it.
  return
 }
 old:=map[string]ToolCallRecord{}
 for _,call:=range previous.ToolCalls{old[call.CallID]=call}
 emit:=func(call ToolCallRecord,kind cp.EventKind){
  if x.EventSeq==^uint64(0){return}
  seq:=x.EventSeq+1
  event:=cp.CounterEvent{Sequence:seq,Identity:guardDigest([]any{x.ID,call.CallID,call.PatchSessionID,call.RunID,kind,seq}),Kind:kind}
  updated,_,applied,err:=cp.ApplyCounterEvent(g.Policy,g.Counters,event,true)
  if err!=nil||!applied{return}
  g.Counters=updated
  x.AppendEvent("clp_"+string(kind),x.Status,call.CallID,"")
 }
 dirty:=false
 for _,call:=range x.ToolCalls{
  if call.Name!="taskdeck_submit_patch"{continue}
  prior,found:=old[call.CallID]
  if found&&prior.Status==call.Status&&prior.RunID==call.RunID&&prior.PatchSessionID==call.PatchSessionID{continue}
  // This is the Legacy objective; a Unified record may be from a
  // different worktree. Conservatively invalidate potential writes but
  // do not attribute its execution count to this objective.
  attributable:=call.AgentRunID==""
  switch call.Status{
  case "pending":
   if !found&&attributable{emit(call,cp.PatchProposed)}
  case "rejected":
   if attributable{emit(call,cp.PatchRejected)}
  case "queuing","queued","dispatching","running":
   dirty=true
  case "unknown":
   dirty=true
   if attributable{emit(call,cp.PatchUncertain)}
  case "done","error":
   var receipt struct{
    RunID string `json:"run_id"`
    Status string `json:"status"`
    ProjectUnchanged bool `json:"project_unchanged"`
   }
   exact:=call.RunID!=""&&call.PatchSessionID!=""&&json.Unmarshal([]byte(call.Result),&receipt)==nil&&receipt.RunID==call.RunID
   if exact&&receipt.Status=="PREFLIGHT_FAIL"&&receipt.ProjectUnchanged&&call.Status=="error"{
    if attributable{emit(call,cp.PatchRejected)}
    continue
   }
   dirty=true
   if attributable{
    if exact&&((receipt.Status=="PASS"&&call.Status=="done")||((receipt.Status=="FAIL"||receipt.Status=="INCOMPLETE")&&call.Status=="error")){
     emit(call,cp.PatchExecuted)
    }else{emit(call,cp.PatchUncertain)}
   }
  }
 }
 if dirty{
  g.Evidence=cp.InvalidateEvidence(g.Evidence,false)
  if g.Coverage!=nil{v:=cp.InvalidateCoverage(*g.Coverage,nil,false);g.Coverage=&v}
  // Earlier duplicate streak is not proof about source after a potential
  // mutation. Keep reads-since-meaningful-progress and recovery budget.
  g.Counters.Progress.DuplicateStreak=0
 }
 x.SetCollectionGuard(g)
}


// Record a newly accepted step only when this objective is bound to the
// exact plan goal/step/acceptance. UpdateTaskPlanStep's existing authenticated
// callers verify native evidence or explicit owner attestation before calling
// it. This is a non-authorizing projection, not a new acceptance mechanism.
// Never reset on an unrelated milestone, wrapper PASS, model completion or
// replay of a previously accepted step in the same objective generation.
func(x *Session)recordCollectionGuardAcceptedStep(step TaskPlanStep,evidence TaskPlanEvidence){
 g,ok:=x.ReadCollectionGuard()
 if !ok||g.AcceptedStep||x.TaskPlan==nil||x.EventSeq==^uint64(0)||
  g.GoalDigest!=guardDigest([]string{x.TaskPlan.Goal+"\n"+step.Title+"\n"+step.Acceptance,step.ID}){return}
 kind:=cp.VerifiedProgress
 switch evidence.Kind{
 case "manual":
  if strings.TrimSpace(evidence.Note)==""{return}
  kind=cp.OwnerAttestedProgress
 case "patch","collection","task_run","go_test_case","go_test_suite":
  if evidence.Ref==""||!validGuardDigest(evidence.Digest){return}
  // Unified adoption must not reset the Legacy objective.
  if strings.HasPrefix(evidence.Ref,"run:"){return}
 default:return
 }
 seq:=x.EventSeq+1
 event:=cp.CounterEvent{Sequence:seq,Identity:guardDigest([]any{x.ID,g.Generation,step.ID,kind,seq}),Kind:kind}
 c,_,applied,err:=cp.ApplyCounterEvent(g.Policy,g.Counters,event,true)
 if err!=nil||!applied{return}
 g.Counters=c;g.AcceptedStep=true
 x.AppendEvent("clp_"+string(kind),x.Status,"","")
 x.SetCollectionGuard(g)
}
