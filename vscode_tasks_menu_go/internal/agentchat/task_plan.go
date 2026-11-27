package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "regexp"
 "strings"
 "time"
)

const (
 MaxTaskPlanSteps=1024
 MaxInlineTaskPlanSteps=128
 MaxTaskPlanGoalBytes=4096
 MaxTaskPlanEvidence=12
 MaxTaskPlanSuggestions=8
 MaxTaskGoRequiredTests=16
 MaxTaskAcceptanceCriteria=8
)
var taskPlanStepIDPattern=regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type TaskPlanEvidence struct {
 Kind string `json:"kind"` // patch, collection, task_run or manual
 Ref string `json:"ref,omitempty"`
 Digest string `json:"digest,omitempty"`
 Note string `json:"note,omitempty"`
}
// TaskCommandGuard is a user-owned expected native test command identity.
// It asserts the exact launched command+argv+cwd, not semantic coverage.
type TaskCommandGuard struct {
 TaskID int `json:"task_id"`
 CommandSHA256 string `json:"command_sha256"`
 BoundAt time.Time `json:"bound_at"`
}
// TaskGoTestRequirements is an owner-selected, pre-run checklist of exact
// named Go tests in a single Go package. This does not claim semantic coverage.
type TaskGoTestRequirements struct {
 Package string `json:"package"`
 Tests []string `json:"tests"`
 BoundAt time.Time `json:"bound_at"`
}

var goRequirementPackagePattern=regexp.MustCompile("^[A-Za-z0-9_][A-Za-z0-9_./-]{0,199}$")
var goRequirementTestPattern=regexp.MustCompile("^Test[A-Za-z0-9_]+(/[A-Za-z0-9_=.,-]+)*$")

func ValidateTaskGoTestRequirements(v *TaskGoTestRequirements)error{
 if v==nil{return nil}
 if v.BoundAt.IsZero()||!goRequirementPackagePattern.MatchString(v.Package)||
  len(v.Tests)==0||len(v.Tests)>MaxTaskGoRequiredTests{return ErrInvalidSession}
 seen:=make(map[string]bool,len(v.Tests))
 for _,name:=range v.Tests{
  if !goRequirementTestPattern.MatchString(name)||seen[name]{return ErrInvalidSession}
  seen[name]=true
 }
 return nil
}

// TaskAcceptanceCriterion binds an OWNER-declared acceptance requirement to
// zero or more PREDECLARED Go tests. A reviewed note is an explicit human
// attestation of semantic applicability, NOT proof the tests ran/PASS.
// Successful native suite verification remains a separate completion gate.
type TaskAcceptanceCriterion struct {
 ID string `json:"id"`
 Description string `json:"description"`
 RequiredTests []string `json:"required_tests,omitempty"`
 ReviewNote string `json:"review_note,omitempty"`
 ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
}

// Every declared criterion needs an explicit owner review before its
// milestone can be marked done. Legacy plans without criteria are unchanged.
func TaskPlanStepCriteriaReviewed(step TaskPlanStep) bool {
 for _,criterion:=range step.AcceptanceCriteria {
  if criterion.ReviewedAt==nil||strings.TrimSpace(criterion.ReviewNote)==""{return false}
 }
 return true
}
func ValidateTaskAcceptanceCriteria(step TaskPlanStep)error{
 if len(step.AcceptanceCriteria)>MaxTaskAcceptanceCriteria{return ErrInvalidSession}
 mapped:=make(map[string]bool)
 if step.GoTestRequirements!=nil {
  for _,name:=range step.GoTestRequirements.Tests{mapped[name]=true}
 }
 ids:=make(map[string]bool,len(step.AcceptanceCriteria))
 for _,criterion:=range step.AcceptanceCriteria{
  if !taskPlanStepIDPattern.MatchString(criterion.ID)||ids[criterion.ID]||
   strings.TrimSpace(criterion.Description)==""||
   len(criterion.Description)>300||len(criterion.RequiredTests)>MaxTaskGoRequiredTests||
   len(criterion.ReviewNote)>512{
   return ErrInvalidSession
  }
  ids[criterion.ID]=true
  if (criterion.ReviewedAt==nil)!=(criterion.ReviewNote==""){return ErrInvalidSession}
  if criterion.ReviewedAt!=nil{
   if criterion.ReviewedAt.IsZero()||strings.TrimSpace(criterion.ReviewNote)==""{return ErrInvalidSession}
  }
  seen:=make(map[string]bool,len(criterion.RequiredTests))
  for _,name:=range criterion.RequiredTests{
   // An unmapped name, duplicate, or changed Go checklist cannot become
   // a retroactive semantic-coverage claim from an old PASS receipt.
   if !mapped[name]||seen[name]{return ErrInvalidSession}
   seen[name]=true
  }
 }
 return nil
}

type TaskPlanStep struct {
 ID string `json:"id"`
 Title string `json:"title"`
 Acceptance string `json:"acceptance"`
 Status string `json:"status"` // pending, active, blocked, done
 Evidence []TaskPlanEvidence `json:"evidence,omitempty"`
 AutoCheck *TaskAutoCheck `json:"auto_check,omitempty"`
 TaskCommandGuard *TaskCommandGuard `json:"task_command_guard,omitempty"`
 GoTestRequirements *TaskGoTestRequirements `json:"go_test_requirements,omitempty"`
 AcceptanceCriteria []TaskAcceptanceCriterion `json:"acceptance_criteria,omitempty"`
 UpdatedAt time.Time `json:"updated_at,omitempty"`
}
type TaskPlan struct {
 Goal string `json:"goal"`
 // Provider-proposed exact evidence pointers are UNTRUSTED review hints.
 // They never create user-owned AutoCheck rules or complete plan steps.
 Suggestions []TaskEvidenceSuggestion `json:"suggestions,omitempty"`
 State string `json:"state"` // active, paused, completed
 AutoContinue bool `json:"auto_continue,omitempty"`
 Steps []TaskPlanStep `json:"steps"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
 // Next scheduled continuation uses a stable request id. A provider request
 // is never replayed after the SENDING/UNKNOWN egress boundary.
 LastContinuationKey string `json:"last_continuation_key,omitempty"`
 LastWorkSignature string `json:"last_work_signature,omitempty"`
 LastWorkSignatureVersion int `json:"last_work_signature_version,omitempty"`
 NoProgressTurns int `json:"no_progress_turns,omitempty"`
 RetryStreak int `json:"retry_streak,omitempty"`
 NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}
func TaskPlanAllDone(p *TaskPlan)bool{
 if p==nil||len(p.Steps)==0{return false}
 for _,step:=range p.Steps{if step.Status!="done"||len(step.Evidence)==0||!TaskPlanStepCriteriaReviewed(step){return false}}
 return true
}
func ValidateTaskPlan(p *TaskPlan)error{
 if p==nil{return nil}
 if len(p.Goal)==0||len(p.Goal)>MaxTaskPlanGoalBytes||strings.TrimSpace(p.Goal)==""||
  len(p.Steps)==0||len(p.Steps)>MaxTaskPlanSteps||p.CreatedAt.IsZero()||p.UpdatedAt.IsZero()||
  len(p.LastContinuationKey)>128||len(p.LastWorkSignature)>128||p.LastWorkSignatureVersion<0||p.LastWorkSignatureVersion>1||p.NoProgressTurns<0||p.NoProgressTurns>100||
  p.RetryStreak<0||p.RetryStreak>100||len(p.Suggestions)>MaxTaskPlanSuggestions {
  return ErrInvalidSession
 }
 switch p.State{case "active","paused","completed":default:return ErrInvalidSession}
 if p.State=="completed"&&(!TaskPlanAllDone(p)||p.AutoContinue){return ErrInvalidSession}
 for _,suggestion:=range p.Suggestions {
  if !taskPlanStepIDPattern.MatchString(suggestion.StepID)||len(suggestion.TurnID)==0||len(suggestion.TurnID)>256||
   len(suggestion.SourceDigest)!=64||suggestion.At.IsZero()||
   ValidateTaskAutoCheck(&TaskAutoCheck{Kind:suggestion.Kind,CallID:suggestion.CallID,Path:suggestion.Path})!=nil {
   return ErrInvalidSession
  }
 }
 ids:=make(map[string]bool,len(p.Steps))
 for _,step:=range p.Steps{
  if !taskPlanStepIDPattern.MatchString(step.ID)||ids[step.ID]||
   len(step.Title)==0||len(step.Title)>240||strings.TrimSpace(step.Title)==""||
   len(step.Acceptance)==0||len(step.Acceptance)>600||strings.TrimSpace(step.Acceptance)==""||
   len(step.Evidence)>MaxTaskPlanEvidence{return ErrInvalidSession}
  ids[step.ID]=true
  if guard:=step.TaskCommandGuard;guard!=nil{
   if guard.TaskID<1||len(guard.CommandSHA256)!=64||guard.BoundAt.IsZero()||
    strings.ToLower(guard.CommandSHA256)!=guard.CommandSHA256{return ErrInvalidSession}
   if _,err:=hex.DecodeString(guard.CommandSHA256);err!=nil{return ErrInvalidSession}
  }
  if err:=ValidateTaskGoTestRequirements(step.GoTestRequirements);err!=nil{return err}
  if err:=ValidateTaskAcceptanceCriteria(step);err!=nil{return err}
  if step.GoTestRequirements!=nil{
   if step.TaskCommandGuard==nil||step.GoTestRequirements.BoundAt.Before(step.TaskCommandGuard.BoundAt){return ErrInvalidSession}
  }
  if err:=ValidateTaskAutoCheck(step.AutoCheck);err!=nil{return err}
  switch step.Status{case "pending","active","blocked","done":default:return ErrInvalidSession}
  if step.Status=="done"&&(len(step.Evidence)==0||!TaskPlanStepCriteriaReviewed(step)){return ErrInvalidSession}
  for _,e:=range step.Evidence{
   switch e.Kind{case "patch","collection","task_run","go_test_case","go_test_suite","manual":default:return ErrInvalidSession}
   if len(e.Ref)>256||len(e.Note)>512||len(e.Digest)>64{return ErrInvalidSession}
   if e.Kind=="manual" {
    if strings.TrimSpace(e.Note)==""||e.Ref!=""||e.Digest!=""{return ErrInvalidSession}
   } else if e.Ref==""||len(e.Digest)!=64{return ErrInvalidSession}
  }
 }
 for _,suggestion:=range p.Suggestions{if !ids[suggestion.StepID]{return ErrInvalidSession}}
 return nil
}
func NewTaskPlan(goal string,steps []TaskPlanStep)(*TaskPlan,error){
 now:=time.Now().UTC()
 p:=&TaskPlan{Goal:strings.TrimSpace(goal),State:"active",Steps:steps,CreatedAt:now,UpdatedAt:now}
 for i:=range p.Steps{p.Steps[i].Status="pending";p.Steps[i].Evidence=nil;p.Steps[i].TaskCommandGuard=nil;p.Steps[i].GoTestRequirements=nil;p.Steps[i].AcceptanceCriteria=nil;p.Steps[i].UpdatedAt=now}
 if err:=ValidateTaskPlan(p);err!=nil{return nil,err}
 return p,nil
}
func taskPlanDigest(content string)string{
 h:=sha256.Sum256([]byte(content));return hex.EncodeToString(h[:])
}
// VerifyTaskPlanEvidence resolves source identities against the VERIFIED
// native receipt or canonical read-only Collection output, never a model
// assertion. Manual evidence is explicitly user-attested, not tool-verified.
func VerifyTaskPlanEvidence(x Session,kind,ref,note string)(TaskPlanEvidence,error){
 kind=strings.TrimSpace(kind)
 if len(note)>512||len(ref)>256{return TaskPlanEvidence{},ErrInvalidSession}
 switch kind {
 case "manual":
  if strings.TrimSpace(note)==""||ref!=""{return TaskPlanEvidence{},errors.New("manual completion needs an explicit user evidence note")}
  return TaskPlanEvidence{Kind:"manual",Note:strings.TrimSpace(note)},nil
 case "patch":
  for _,call:=range x.ToolCalls {
   if call.CallID==ref && validateTaskNativePatchSuccess(call)==nil {
    return TaskPlanEvidence{Kind:"patch",Ref:ref,Digest:taskPlanDigest(call.Result),Note:strings.TrimSpace(note)},nil
   }
  }
  return TaskPlanEvidence{},fmt.Errorf("no acknowledged final Native Patch receipt for %q",ref)
 case "collection":
  return verifyTaskCollectionPair(x.History,x.ProjectID,ref,note)
 default: return TaskPlanEvidence{},ErrInvalidSession
 }
}
 // Resolve a single ordered, unambiguous canonical call/output pair. Native
 // receipt text is never accepted from a user or assistant history message.
 func verifyTaskCollectionPair(history []HistoryItem,projectID,ref,note string)(TaskPlanEvidence,error){
  var call,output *HistoryItem
  for i:=range history{
   item:=&history[i]
   if item.CallID!=ref{continue}
   switch item.Type{
   case "function_call":
    if call!=nil||output!=nil||item.Name!="taskdeck_collect_workspace"{return TaskPlanEvidence{},ErrInvalidSession}
    call=item
   case "function_call_output":
    if call==nil||output!=nil{return TaskPlanEvidence{},ErrInvalidSession}
    output=item
   }
  }
  if call==nil||output==nil||call.OriginRunID!=output.OriginRunID||
   call.ProviderID!=output.ProviderID||call.ModelID!=output.ModelID||call.ParentTurnID!=output.ParentTurnID||
   validateTaskCollectionObservation(output.Content,projectID)!=nil{
   return TaskPlanEvidence{},fmt.Errorf("no paired verified Collection observation for %q",ref)
  }
  return TaskPlanEvidence{Kind:"collection",Ref:ref,Digest:taskPlanDigest(output.Content),Note:strings.TrimSpace(note)},nil
 }

func UpdateTaskPlanStep(x *Session,id,status string,evidence *TaskPlanEvidence)error{
 if x==nil||x.TaskPlan==nil{return ErrInvalidSession}
 switch status{case "pending","active","blocked","done":default:return ErrInvalidSession}
 for i:=range x.TaskPlan.Steps{
  step:=&x.TaskPlan.Steps[i]
  if step.ID!=id{continue}
  newlyAccepted:=status=="done"&&step.Status!="done"
  if status=="done"{
   if !TaskPlanStepCriteriaReviewed(*step){return errors.New("all owner-defined acceptance criteria need explicit reviews")}
   if evidence==nil {return errors.New("completion requires verified or user-attested evidence")}
   if len(step.Evidence)>=MaxTaskPlanEvidence{return ErrInvalidSession}
   step.Evidence=append(step.Evidence,*evidence)
  }
  step.Status=status
  step.UpdatedAt=time.Now().UTC()
  x.TaskPlan.UpdatedAt=step.UpdatedAt
  if x.TaskPlan.State=="completed"&&status!="done"{x.TaskPlan.State="paused"}
  // Never auto-complete purely because the last model message ended.
  // A verified endpoint action can close the plan only after ALL gates pass.
  if err:=ValidateTaskPlan(x.TaskPlan);err!=nil{return err}
  if newlyAccepted&&evidence!=nil{x.recordCollectionGuardAcceptedStep(*step,*evidence)}
  return nil
 }
 return errors.New("unknown task plan step")
}
func TaskPlanProgress(p *TaskPlan)(done,total int){
 if p==nil{return 0,0}
 for _,item:=range p.Steps{if item.Status=="done"{done++}}
 return done,len(p.Steps)
}
// The coordinator uses this small, stable status; it never conflates
// a completed model turn with a completed user task.
func TaskPlanNext(p *TaskPlan)*TaskPlanStep{
 if p==nil||p.State!="active"{return nil}
 for i:=range p.Steps{
  if p.Steps[i].Status=="active"||p.Steps[i].Status=="pending"{return &p.Steps[i]}
 }
 return nil
}

func TaskPlanAutoContinueCandidate(x Session)bool{
 p:=x.TaskPlan
 if p==nil||x.Owner!="local"||x.Archived||p.State!="active"||
  !p.AutoContinue||x.Status!="completed"||TaskPlanNext(p)==nil||
  HasUnresolvedWork(x){return false}
 if p.NextRetryAt!=nil&&time.Now().UTC().Before(*p.NextRetryAt){return false}
 if x.ProviderRun!=nil && (x.ProviderRun.Phase=="sending"||
  x.ProviderRun.Phase=="unknown"||x.ProviderRun.Phase=="prepared"){return false}
 for _,call:=range x.ToolCalls{
  // A final native run that has not yet been sent back to AI still owns the
  // current step. Do not skip it by starting an unrelated model request.
  if (call.Status=="done"||call.Status=="error"||call.Status=="rejected")&&
   (!call.FeedbackSent||call.FeedbackStatus!="acknowledged"){return false}
 }
 return true
}

// VerifiedWorkSignature changes only when new read-only tool observations,
// native final receipts, or human-attested plan milestones exist. Model-only
// narration / repeated generic answers cannot indefinitely reset the
// no-progress guard and spend provider tokens without observable work.
func VerifiedWorkSignature(x Session)string{
 var b strings.Builder
 done,total:=TaskPlanProgress(x.TaskPlan)
 // Once CLP has a scoped reducer, the outer scheduler must not count
 // receipt IDs, archive movement, or native PASS a second time as work.
 // New verified evidence is a read-progress signal, not task acceptance.
 if g,ok:=x.ReadCollectionGuard();ok{
  return scopedWorkSignature(g.Counters.Objective.Key,g.Counters.NewEvidence,g.Counters.VerifiedProgressEvents,g.Counters.AttestedProgressEvents,done,total)
 }
 // An opaque guard cannot establish progress. Preserve old unguarded
 // compatibility only when no projection has been installed at all.
 if len(x.CollectionGuard)!=0{return taskPlanDigest(fmt.Sprintf("clp-unknown:%d/%d",done,total))}
 fmt.Fprintf(&b,"done:%d/%d archive:%d/%d\n",done,total,x.ArchivedHistoryItems,x.ArchivedPatchCalls)
 for _,it:=range x.History{
  if it.Type!="function_call_output"{continue}
  fmt.Fprintf(&b,"output:%s:%s\n",it.CallID,taskPlanDigest(it.Content))
 }
 for _,call:=range x.ToolCalls {
  if call.FeedbackStatus=="acknowledged"{
   fmt.Fprintf(&b,"patch:%s:%s:%s\n",call.CallID,call.Status,taskPlanDigest(call.Result))
  }
 }
 return taskPlanDigest(b.String())
}

// Keep SHA-256 format compatible with older journal readers. The scheduler
// stores a separate optional version marker, not a prefix in the digest.
func WorkSignatureVersion(x Session)int{if len(x.CollectionGuard)!=0{return 1};return 0}

func scopedWorkSignature(key string,novel,verified,attested uint64,done,total int)string{
 return taskPlanDigest(fmt.Sprintf("clp-v1:%s:%d:%d:%d:%d/%d",key,novel,verified,attested,done,total))
}

// ContinuationWorkReference rebases only the signature format/objective,
// not the no-work counter. First guard binding or a step change is not
// evidence of new work. Compare actual new events and accepted plan changes
// against a zero-event baseline for the newly bound objective.
func ContinuationWorkReference(before,after Session,prior string,priorVersion int)string{
 old,oldOK:=before.ReadCollectionGuard()
 next,nextOK:=after.ReadCollectionGuard()
 if nextOK{
  if !oldOK||old.Counters.Objective.Key!=next.Counters.Objective.Key{
   done,total:=TaskPlanProgress(before.TaskPlan)
   return scopedWorkSignature(next.Counters.Objective.Key,0,0,0,done,total)
  }
  if priorVersion!=1{return VerifiedWorkSignature(before)}
 }else if len(after.CollectionGuard)!=0{
  if priorVersion!=1||len(before.CollectionGuard)==0||oldOK{
   done,total:=TaskPlanProgress(before.TaskPlan)
   return taskPlanDigest(fmt.Sprintf("clp-unknown:%d/%d",done,total))
  }
 }
 if prior==""{return VerifiedWorkSignature(before)}
 return prior
}

// VerifyArchivedTaskPlanEvidence allows a milestone to reference an older
// verified Collection or Native Patch receipt even after its full source was
// moved to an integrity-checked private archive. All segments are owner scoped.
// This is audit lookup only: no provider request or Native Patch is executed.
// VerifyArchivedTaskPlanEvidence uses the authenticated public history APIs.
// The same proof-checking predicate is shared with the journal-locked,
 // single-segment CAS path below; only the archive reader differs.
func VerifyArchivedTaskPlanEvidence(store *Store,x Session,kind,ref,note string)(TaskPlanEvidence,error){
 if store==nil{return VerifyTaskPlanEvidence(x,kind,ref,note)}
 return verifyArchivedTaskPlanEvidenceWithReaders(x,kind,ref,note,
  func(part uint64)([]PatchArchiveEntry,error){return store.ArchivedPatchHistory(x.ID,x.Owner,part)},
  func(part uint64)([]HistoryItem,error){return store.ArchivedCollectionHistory(x.ID,x.Owner,part)})
}

// verifyArchivedTaskPlanEvidenceLocked MUST run under Store.withLock after
// matching the owner and project. Calling the public Archived... APIs here
// would deadlock on the journal lock; the locked readers enforce the same
// private format and checksum validation without re-entering it.
func (s *Store) verifyArchivedTaskPlanEvidenceLocked(x Session,kind,ref,note string)(TaskPlanEvidence,error){
 return verifyArchivedTaskPlanEvidenceWithReaders(x,kind,ref,note,
  func(part uint64)([]PatchArchiveEntry,error){return s.readArchivedPatchHistoryLocked(x,part)},
  func(part uint64)([]HistoryItem,error){return s.readArchivedCollectionHistoryLocked(x,part)})
}

// No model assertion, archive summary, or unsigned record can become proof.
// Each archive must pass its private format/session/sequence/sha validation;
// the exact Native Patch success or paired Collection result must still pass
// the original canonical verifier. An error in ANY scanned archive fails
// closed instead of skipping a damaged batch to find a convenient receipt.
func verifyArchivedTaskPlanEvidenceWithReaders(
 x Session,kind,ref,note string,
 readPatch func(uint64)([]PatchArchiveEntry,error),
 readCollection func(uint64)([]HistoryItem,error),
)(TaskPlanEvidence,error){
 current,err:=VerifyTaskPlanEvidence(x,kind,ref,note)
 if err==nil{return current,nil}
 if x.ID==""||x.Owner==""||ref==""||len(ref)>256||len(note)>512{return TaskPlanEvidence{},err}
 switch kind{
 case "patch":
  for part:=x.PatchArchiveParts;part>0;part--{
   entries,e:=readPatch(part)
   if e!=nil{return TaskPlanEvidence{},fmt.Errorf("cannot verify Patch archive %d: %w",part,e)}
   for _,entry:=range entries{
    call:=entry.Call
    if call.CallID==ref&&validateTaskNativePatchSuccess(call)==nil{
     return TaskPlanEvidence{Kind:"patch",Ref:ref,Digest:taskPlanDigest(call.Result),Note:strings.TrimSpace(note)},nil
    }
   }
  }
 case "collection":
  for part:=x.HistoryArchiveParts;part>0;part--{
   items,e:=readCollection(part)
   if e!=nil{return TaskPlanEvidence{},fmt.Errorf("cannot verify Collection archive %d: %w",part,e)}
   if evidence,e:=verifyTaskCollectionPair(items,x.ProjectID,ref,note);e==nil{return evidence,nil}
  }
 }
 return TaskPlanEvidence{},err
}
