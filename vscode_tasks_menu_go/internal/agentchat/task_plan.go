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
 MaxTaskPlanSteps=128
 MaxTaskPlanGoalBytes=4096
 MaxTaskPlanEvidence=12
)
var taskPlanStepIDPattern=regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type TaskPlanEvidence struct {
 Kind string `json:"kind"` // patch, collection, task_run or manual
 Ref string `json:"ref,omitempty"`
 Digest string `json:"digest,omitempty"`
 Note string `json:"note,omitempty"`
}
type TaskPlanStep struct {
 ID string `json:"id"`
 Title string `json:"title"`
 Acceptance string `json:"acceptance"`
 Status string `json:"status"` // pending, active, blocked, done
 Evidence []TaskPlanEvidence `json:"evidence,omitempty"`
 AutoCheck *TaskAutoCheck `json:"auto_check,omitempty"`
 UpdatedAt time.Time `json:"updated_at,omitempty"`
}
type TaskPlan struct {
 Goal string `json:"goal"`
 State string `json:"state"` // active, paused, completed
 AutoContinue bool `json:"auto_continue,omitempty"`
 Steps []TaskPlanStep `json:"steps"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
 // Next scheduled continuation uses a stable request id. A provider request
 // is never replayed after the SENDING/UNKNOWN egress boundary.
 LastContinuationKey string `json:"last_continuation_key,omitempty"`
 LastWorkSignature string `json:"last_work_signature,omitempty"`
 NoProgressTurns int `json:"no_progress_turns,omitempty"`
 RetryStreak int `json:"retry_streak,omitempty"`
 NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}
func TaskPlanAllDone(p *TaskPlan)bool{
 if p==nil||len(p.Steps)==0{return false}
 for _,step:=range p.Steps{if step.Status!="done"||len(step.Evidence)==0{return false}}
 return true
}
func ValidateTaskPlan(p *TaskPlan)error{
 if p==nil{return nil}
 if len(p.Goal)==0||len(p.Goal)>MaxTaskPlanGoalBytes||strings.TrimSpace(p.Goal)==""||
  len(p.Steps)==0||len(p.Steps)>MaxTaskPlanSteps||p.CreatedAt.IsZero()||p.UpdatedAt.IsZero()||
  len(p.LastContinuationKey)>128||len(p.LastWorkSignature)>128||p.NoProgressTurns<0||p.NoProgressTurns>100||
  p.RetryStreak<0||p.RetryStreak>100 {
  return ErrInvalidSession
 }
 switch p.State{case "active","paused","completed":default:return ErrInvalidSession}
 if p.State=="completed"&&(!TaskPlanAllDone(p)||p.AutoContinue){return ErrInvalidSession}
 ids:=make(map[string]bool,len(p.Steps))
 for _,step:=range p.Steps{
  if !taskPlanStepIDPattern.MatchString(step.ID)||ids[step.ID]||
   len(step.Title)==0||len(step.Title)>240||strings.TrimSpace(step.Title)==""||
   len(step.Acceptance)==0||len(step.Acceptance)>600||strings.TrimSpace(step.Acceptance)==""||
   len(step.Evidence)>MaxTaskPlanEvidence{return ErrInvalidSession}
  ids[step.ID]=true
  if err:=ValidateTaskAutoCheck(step.AutoCheck);err!=nil{return err}
  switch step.Status{case "pending","active","blocked","done":default:return ErrInvalidSession}
  if step.Status=="done"&&len(step.Evidence)==0{return ErrInvalidSession}
  for _,e:=range step.Evidence{
   switch e.Kind{case "patch","collection","task_run","manual":default:return ErrInvalidSession}
   if len(e.Ref)>256||len(e.Note)>512||len(e.Digest)>64{return ErrInvalidSession}
   if e.Kind=="manual" {
    if strings.TrimSpace(e.Note)==""||e.Ref!=""||e.Digest!=""{return ErrInvalidSession}
   } else if e.Ref==""||len(e.Digest)!=64{return ErrInvalidSession}
  }
 }
 return nil
}
func NewTaskPlan(goal string,steps []TaskPlanStep)(*TaskPlan,error){
 now:=time.Now().UTC()
 p:=&TaskPlan{Goal:strings.TrimSpace(goal),State:"active",Steps:steps,CreatedAt:now,UpdatedAt:now}
 for i:=range p.Steps{p.Steps[i].Status="pending";p.Steps[i].Evidence=nil;p.Steps[i].UpdatedAt=now}
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
  var call,output bool
  var content string
  for _,item:=range x.History{
   if item.CallID!=ref{continue}
   if item.Type=="function_call"&&item.Name=="taskdeck_collect_workspace"{call=true}
   if item.Type=="function_call_output"{output=true;content=item.Content}
  }
  if call&&output&&validateTaskCollectionSuccess(content)==nil{
   return TaskPlanEvidence{Kind:"collection",Ref:ref,Digest:taskPlanDigest(content),Note:strings.TrimSpace(note)},nil
  }
  return TaskPlanEvidence{},fmt.Errorf("no paired verified Collection observation for %q",ref)
 default: return TaskPlanEvidence{},ErrInvalidSession
 }
}
func UpdateTaskPlanStep(x *Session,id,status string,evidence *TaskPlanEvidence)error{
 if x==nil||x.TaskPlan==nil{return ErrInvalidSession}
 switch status{case "pending","active","blocked","done":default:return ErrInvalidSession}
 for i:=range x.TaskPlan.Steps{
  step:=&x.TaskPlan.Steps[i]
  if step.ID!=id{continue}
  if status=="done"{
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

// VerifyArchivedTaskPlanEvidence allows a milestone to reference an older
// verified Collection or Native Patch receipt even after its full source was
// moved to an integrity-checked private archive. All segments are owner scoped.
// This is audit lookup only: no provider request or Native Patch is executed.
func VerifyArchivedTaskPlanEvidence(store *Store,x Session,kind,ref,note string)(TaskPlanEvidence,error){
 current,err:=VerifyTaskPlanEvidence(x,kind,ref,note)
 if err==nil{return current,nil}
 if store==nil||x.ID==""||x.Owner==""||ref==""||len(ref)>256{return TaskPlanEvidence{},err}
 switch kind{
 case "patch":
  for part:=x.PatchArchiveParts;part>0;part--{
   entries,e:=store.ArchivedPatchHistory(x.ID,x.Owner,part)
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
   items,e:=store.ArchivedCollectionHistory(x.ID,x.Owner,part)
   if e!=nil{return TaskPlanEvidence{},fmt.Errorf("cannot verify Collection archive %d: %w",part,e)}
   for i:=0;i+1<len(items);i++{
    call,output:=items[i],items[i+1]
    if call.CallID==ref&&call.Type=="function_call"&&call.Name=="taskdeck_collect_workspace"&&
     output.Type=="function_call_output"&&output.CallID==ref&&validateTaskCollectionSuccess(output.Content)==nil{
     return TaskPlanEvidence{Kind:"collection",Ref:ref,Digest:taskPlanDigest(output.Content),Note:strings.TrimSpace(note)},nil
    }
   }
  }
 }
 return TaskPlanEvidence{},err
}
