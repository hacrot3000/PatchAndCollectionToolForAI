package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
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
   evidence:=cp.InspectNativeEvidence(scope,identity.Key,identity.ID,request,raw)
   g.Evidence,novelty=cp.ObserveEvidence(g.Evidence,scope,evidence)
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
  return " Collection progress: verified repeated evidence was observed. Change the inspection strategy or narrow the missing scope. This advisory does not authorize cache reuse, a Patch, or task completion."
 default:return ""
 }
}
