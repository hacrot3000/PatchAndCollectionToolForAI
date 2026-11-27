package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Private in-process provenance, not a request body/header/idempotency prefix.
// A browser cannot mark arbitrary text as an internal continuation.
type collectionContinuationContextKey struct{}
func collectionContinuationContext(ctx context.Context)context.Context{
 return context.WithValue(ctx,collectionContinuationContextKey{},true)
}
func isCollectionContinuation(ctx context.Context)bool{
 v,_:=ctx.Value(collectionContinuationContextKey{}).(bool);return v
}

func collectionOwnerContinue(message string)bool{
 switch strings.ToLower(strings.TrimSpace(message)){
 case "continue","continue.","tiếp","tiếp tục","tiếp tục.","hãy tiếp tục","hãy tiếp tục.":return true
 default:return false
 }
}

// Called once at real owner turn admission. The plan is consulted only for
// continuation, not to reinterpret an explicit new owner instruction.
func bindLegacyCollectionObjective(ctx context.Context,x *agentchat.Session,message string){
 continuation:=isCollectionContinuation(ctx)||collectionOwnerContinue(message)
 if continuation{
  if step:=agentchat.TaskPlanNext(x.TaskPlan);step!=nil{
   x.BindCollectionGuard(x.TaskPlan.Goal+"\n"+step.Title+"\n"+step.Acceptance,step.ID,true)
   return
  }
  if x.BindCollectionGuard("","",true){return}
  // Old sessions cannot safely recover an owner goal from arbitrary
  // user-role history containing a historical AI Pack or tool output.
  x.BindCollectionGuard("Continue","",true)
  return
 }
 x.BindCollectionGuard(message,"",false)
}

func collectionEventIdentity(x *agentchat.Session,callID string,kind cp.EventKind,sequence uint64)string{
 runID:=""
 if x.ProviderRun!=nil{runID=x.ProviderRun.ID}
 body,_:=json.Marshal([]any{x.ID,runID,callID,kind,sequence})
 sum:=sha256.Sum256(body)
 return hex.EncodeToString(sum[:])
}

// reduceLegacyCollectionEvent runs inside the same session turn lock and
// in-memory journal update as the canonical call/receipt. Only the existing
// Session.Save CAS commits it. No metadata retry executes a native tool.
func reduceLegacyCollectionEvent(x *agentchat.Session,g agentchat.CollectionGuardState,callID string,kind cp.EventKind,novelty cp.Novelty,native,patchAvailable bool)cp.Decision{
 none:=cp.Decision{Kind:cp.Continue}
 if x.EventSeq==^uint64(0){return none}
 seq:=x.EventSeq+1
 event:=cp.CounterEvent{Sequence:seq,Identity:collectionEventIdentity(x,callID,kind,seq),Kind:kind,Novelty:novelty,NativeRead:native}
 next,d,applied,err:=cp.ApplyCounterEvent(g.Policy,g.Counters,event,patchAvailable)
 if err!=nil||!applied{return none}
 g.Counters=next
 x.AppendEvent("clp_"+string(kind),x.Status,callID,string(d.Reason))
 if !x.SetCollectionGuard(g){return none}
 return d
}

func legacyCollectionRequested(x *agentchat.Session,callID string,patchAvailable bool){
 g,ok:=x.ReadCollectionGuard();if !ok{return}
 reduceLegacyCollectionEvent(x,g,callID,cp.CollectionRequested,"",false,patchAvailable)
}

func legacyCollectionObserved(x *agentchat.Session,callID string,identity patchtool.ProjectIdentity,request []byte,raw string,native,patchAvailable bool)cp.Decision{
 g,ok:=x.ReadCollectionGuard();if !ok{return cp.Decision{Kind:cp.Continue}}
 novelty:=cp.UnknownNovelty
 if code,_,repair:=agentCollectionRepairError(raw);repair&&code!="COLLECTION_NO_PROGRESS"{
  novelty=cp.ObservationError
 }else if native{
  scope,err:=cp.BindCollectionScope(g.Counters.Objective,identity.Key,identity.ID,request,"")
  if err==nil{
   g.Evidence,g.Coverage,novelty=cp.ObserveNativeRead(g.Evidence,g.Coverage,g.Counters.Objective,scope,identity.Key,identity.ID,request,raw)
  }
 }
 return reduceLegacyCollectionEvent(x,g,callID,cp.CollectionObserved,novelty,native,patchAvailable)
}

// Trusted, fixed-size instructions, separate from the byte-exact native
// receipt. No prompt/source/arguments or model-generated advice is copied.
func collectionProgressAdvice(d cp.Decision)string{
 if d.Kind!=cp.Advise{return ""}
 switch d.Reason{
 case cp.AssessReadiness:
  return " Collection progress: assess whether you already have enough evidence for the next small implementation step. If not, request only the specific missing scope. Do not invent a Patch or skip necessary reads."
 case cp.BoundNextScope:
  return " Collection progress: identify the next concrete objective and exact missing evidence. Prefer one bounded native read or a justified small Patch proposal. Continue necessary reads; never bypass approval or claim completion without evidence."
 case cp.RepeatedEvidence:
  return " Collection progress: verified repeated evidence was observed. Use the already delivered receipt to answer what it actually proves; choose a distinct missing scope or change strategy. Re-read when freshness or completeness is uncertain. This advisory does not authorize skipping a native read, a Patch, or task completion."
 default:return ""
 }
}


// Request-shape advice is distinct from verified evidence novelty. It never
// skips native reads or modifies receipts. Warn on third matching request,
// including alternating scopes, once per bounded retained fingerprint.
type collectionRequestAdvisories struct {
 counts map[string]int
 order []string
}
func newCollectionRequestAdvisories()*collectionRequestAdvisories{
 return &collectionRequestAdvisories{counts:map[string]int{}}
}
const repeatedCollectionRequestAdvice=" Collection request pattern repeated: this is a request-shape hint, not proof that source content is unchanged. If the receipt answers the question, narrow the next missing scope or change strategy; re-read if needed."
func(a *collectionRequestAdvisories)Observe(request []byte)string{
 digest,err:=cp.CanonicalRequestFingerprint(request)
 if err!=nil{return ""}
 n:=a.counts[digest]
 if n==0{
  if len(a.order)>=64{delete(a.counts,a.order[0]);a.order=a.order[1:]}
  a.order=append(a.order,digest)
 }
 if n<3{a.counts[digest]=n+1}
 if n==2{return repeatedCollectionRequestAdvice}
 return ""
}


func legacyCollectionStallCandidate(x agentchat.Session,callID string)bool{
 g,ok:=x.ReadCollectionGuard()
 if !ok||!g.Counters.Progress.DuplicateAdvised||g.Counters.Progress.DuplicateStreak<g.Policy.DuplicateStreak+g.Policy.DuplicateCooldown{return false}
 if len(x.Events)==0{return false}
 last:=x.Events[len(x.Events)-1]
 return last.Kind=="clp_collection_observed"&&last.CallID==callID&&last.Seq==g.Counters.LastSequence&&legacyVerifiedDuplicateEvent(x,callID,g)
}

func legacyVerifiedDuplicateEvent(x agentchat.Session,callID string,g agentchat.CollectionGuardState)bool{
 seq:=g.Counters.LastSequence
 e:=cp.CounterEvent{Sequence:seq,Identity:collectionEventIdentity(&x,callID,cp.CollectionObserved,seq),Kind:cp.CollectionObserved,Novelty:cp.VerifiedDuplicate,NativeRead:true}
 raw,err:=json.Marshal(e)
 return err==nil&&agentchat.ProviderPayloadDigest(raw)==g.Counters.LastEventDigest
}

func legacyCollectionReplanAllowed(x agentchat.Session)bool{
 allowed:=x.Owner=="local"&&!x.Archived&&(x.AutoCollection==nil||*x.AutoCollection)
 if x.TaskPlan!=nil{return allowed&&x.TaskPlan.AutoContinue&&agentchat.TaskPlanNext(x.TaskPlan)!=nil}
 return allowed&&x.AutoContinueText
}

// The caller has already saved the exact current observation in the same
// CAS as the counters. Replan can only replace the NEXT continuation's
// instructions, never replay a previous provider or native operation.
func assessLegacyCollectionStall(ctx context.Context,x *agentchat.Session,next agentchat.HistoryItem)(cp.RecoveryDecision,error){
 g,ok:=x.ReadCollectionGuard()
 if !ok||!legacyVerifiedDuplicateEvent(*x,next.CallID,g){return cp.RecoveryDecision{Kind:cp.Continue},nil}
 committed:=len(x.History)>0&&x.History[len(x.History)-1]==next
 sequence:=g.Counters.LastSequence
 e:=cp.CounterEvent{Sequence:sequence,Identity:collectionEventIdentity(x,next.CallID,cp.CollectionObserved,sequence),Kind:cp.CollectionObserved,Novelty:cp.VerifiedDuplicate,NativeRead:true}
 safe:=x.ProviderRun!=nil&&x.ProviderRun.Phase=="acknowledged"&&!x.Archived&&x.PendingUserTurn==nil
 for _,call:=range x.ToolCalls{
  switch call.Status{
  case "pending","queuing","queued","dispatching","running","unknown":safe=false
  }
  if call.Name=="taskdeck_submit_patch"&&(!call.FeedbackSent||call.FeedbackStatus!="acknowledged"){safe=false}
 }
 allowed:=legacyCollectionReplanAllowed(*x)
 state,decision:=cp.AssessRecovery(g.Policy,g.Counters,e,g.Recovery,cp.RecoveryGate{
  Stopped:ctx.Err()!=nil,SafeEgress:safe,CanonicalOutputCommitted:committed,OwnerAllowsContinuation:allowed,
 })
 if decision.Kind==cp.Replan{
  g.Recovery=state
  if !x.SetCollectionGuard(g){return cp.RecoveryDecision{},errors.New("cannot retain collection recovery reservation")}
  x.AppendEvent("clp_replan",x.Status,next.CallID,string(decision.Reason))
 }else if decision.Kind==cp.Checkpoint{
  x.AppendEvent("clp_checkpoint",x.Status,next.CallID,string(decision.Reason))
 }
 return decision,nil
}

const collectionReplanAdvice=" Collection progress recovery: verified duplicate evidence persisted after earlier advice. Rebase on the current original goal and exact native receipt. Change the inspection strategy, identify a concrete missing scope or justified next action, and do not repeat the same evidence or claim completion. This is the one bounded replan for this objective generation."


// Model-visible advisory metadata is an explicitly bounded server-owned
// projection, not a receipt, progress attestation, quota or permission.
// No scope strings, proof hashes, user/model text or paths cross this boundary.
// The exact missing scope must be derived from the current native receipt.
func collectionProgressAdvisory(d cp.Decision,c cp.CounterState,patchAvailable bool)string{
 guidance:=collectionProgressAdvice(d)
 if guidance==""||c.Version!=cp.Version||!c.Objective.Valid(){return ""}
 intent:=c.Objective.AdvisoryIntent(patchAvailable)
 // A stale advisory after revocation cannot pressure an unavailable Patch.
 if (d.Reason==cp.AssessReadiness||d.Reason==cp.BoundNextScope)&&intent!=cp.Implementation&&intent!=cp.Mixed{return ""}
 strategy:="choose_exact_missing_scope_from_current_receipt"
 if d.Reason==cp.RepeatedEvidence{strategy="change_inspection_strategy_or_narrow_missing_scope"}
 if intent==cp.ReadOnly{strategy="continue_read_only_with_distinct_missing_scope"}
 body,err:=json.Marshal(struct{
  Version int `json:"version"`
  Reason cp.Reason `json:"reason"`
  Intent cp.Intent `json:"intent"`
  Reads uint64 `json:"native_reads"`
  VerifiedNew uint64 `json:"verified_new"`
  VerifiedDuplicate uint64 `json:"verified_duplicate"`
  Unknown uint64 `json:"unknown"`
  Errors uint64 `json:"errors"`
  ReadsSinceProgress uint64 `json:"reads_since_progress"`
  VerifiedProgress uint64 `json:"verified_progress"`
  Strategy string `json:"next_strategy"`
 }{cp.Version,d.Reason,intent,c.NativeReads,c.NewEvidence,c.DuplicateEvidence,c.UnknownEvidence,c.Errors,c.Progress.ReadsSinceProgress,c.VerifiedProgressEvents,strategy})
 if err!=nil||len(body)>768{return guidance}
 return guidance+"\nCollection advisory metadata (not new evidence or authorization): "+string(body)+"\n"
}

func legacyCollectionProgressAdvisory(x agentchat.Session,d cp.Decision,patchAvailable bool)string{
 g,ok:=x.ReadCollectionGuard()
 if !ok{return ""}
 return collectionProgressAdvisory(d,g.Counters,patchAvailable)
}
