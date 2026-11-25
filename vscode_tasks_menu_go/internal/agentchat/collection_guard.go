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
 Policy cp.Policy `json:"policy"`
 Counters cp.CounterState `json:"counters"`
 Evidence cp.EvidenceIndex `json:"evidence"`
 Recovery cp.RecoveryState `json:"recovery"`
 Legacy LegacyCollectionSummary `json:"legacy"`
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
 c:=g.Counters
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
