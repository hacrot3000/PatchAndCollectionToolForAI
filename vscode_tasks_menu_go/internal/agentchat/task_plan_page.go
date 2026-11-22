package agentchat

import (
 "errors"
 "strings"
)

// ErrTaskPlanPageOutOfRange is a client pagination error rather than a
// damaged plan. It must not be used for a missing or corrupt private segment.
var ErrTaskPlanPageOutOfRange = errors.New("Task plan page offset exceeds plan length")

// TaskPlanPageResult contains only the requested milestone slice while
// progress, completion and revision still refer to the canonical whole plan.
type TaskPlanPageResult struct {
 SessionID string `json:"session_id"`
 Revision uint64 `json:"revision"`
 Plan *TaskPlan `json:"plan"`
 Done int `json:"done"`
 Total int `json:"total"`
 AllDone bool `json:"all_done"`
 Completed bool `json:"completed"`
 Page struct {
  Offset int `json:"offset"`
  Limit int `json:"limit"`
  TotalSteps int `json:"total_steps"`
  HasNext bool `json:"has_next"`
 } `json:"page"`
}

// TaskPlanPage is read-only, owner/project-scoped and serialized with journal
// writes by the existing directory lock. For segmented plans it validates
// every immutable page's digest and progress but retains only the requested
// <=32 steps. It never trusts page-local completion as whole-plan acceptance,
// and any missing/corrupted off-page segment fails the entire request.
// Legacy inline plans remain readable without a migration or extra writes.
func (s *Store) TaskPlanPage(id,owner string,offset,limit int)(TaskPlanPageResult,error){
 var result TaskPlanPageResult
 if !validID(id){return result,ErrNotFound}
 if offset<0||limit<1||limit>32{return result,ErrTaskPlanPageOutOfRange}
 err:=s.withLock(func()error{
  x,e:=s.readSessionManifestLocked(id)
  if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}

  result.SessionID=id
  result.Revision=x.Revision
  result.Page.Offset=offset
  result.Page.Limit=limit
  if len(x.TaskPlanSegments)==0 {
   // Full legacy validation remains the authority for inline journals.
   if e=validateSession(x);e!=nil{return e}
   if x.TaskPlan!=nil{
    result.Done,result.Total=TaskPlanProgress(x.TaskPlan)
    result.AllDone=TaskPlanAllDone(x.TaskPlan)
    result.Completed=x.TaskPlan.State=="completed"
    if offset>result.Total{return ErrTaskPlanPageOutOfRange}
    end:=offset+limit
    if end>result.Total{end=result.Total}
    plan:=*x.TaskPlan
    plan.Steps=append([]TaskPlanStep{},x.TaskPlan.Steps[offset:end]...)
    result.Plan=&plan
    result.Page.HasNext=end<result.Total
   }else if offset!=0{return ErrTaskPlanPageOutOfRange}
   result.Page.TotalSteps=result.Total
   return nil
  }

  // Validate the other canonical fields without a bogus one-step plan.
  // The plan manifest itself is checked below against all private pages.
  background:=x
  background.TaskPlan=nil
  background.TaskPlanSegments=nil
  if e=validateSession(background);e!=nil{return e}
  if x.TaskPlan==nil||len(x.TaskPlan.Steps)!=0||
   len(x.TaskPlanSegments)>(MaxTaskPlanSteps+taskPlanSegmentSize-1)/taskPlanSegmentSize||
   len(x.TaskPlan.Goal)==0||len(x.TaskPlan.Goal)>MaxTaskPlanGoalBytes||
   strings.TrimSpace(x.TaskPlan.Goal)==""||
   x.TaskPlan.CreatedAt.IsZero()||x.TaskPlan.UpdatedAt.IsZero(){
   return ErrInvalidSession
  }
  switch x.TaskPlan.State{
  case "active","paused","completed":
  default:return ErrInvalidSession
  }
  // Keep this bounded without allocating/hydrating the 1,024-step plan.
  total:=0
  for i,ref:=range x.TaskPlanSegments{
   if ref.Index!=i+1||!validTaskPlanSegmentRef(ref,x.Revision)||
    (i<len(x.TaskPlanSegments)-1&&ref.Count!=taskPlanSegmentSize){
    return ErrInvalidSession
   }
   total+=ref.Count
  }
  if total<=MaxInlineTaskPlanSteps||total>MaxTaskPlanSteps{return ErrInvalidSession}
  if offset>total{return ErrTaskPlanPageOutOfRange}
  end:=offset+limit
  if end>total{end=total}
  pageSteps:=make([]TaskPlanStep,0,end-offset)
  done:=0
  first:=0
  for _,ref:=range x.TaskPlanSegments{
   // Check ALL page digests, including off-screen pages, before reporting
   // whole-plan progress or completion; don't trade integrity for latency.
   part,e:=s.readVerifiedTaskPlanSegment(id,ref)
   if e!=nil{return e}
   for j,step:=range part{
    switch step.Status{
    case "pending","active","blocked":
    case "done":
     if len(step.Evidence)==0{return ErrInvalidSession}
     done++
    default:return ErrInvalidSession
    }
    n:=first+j
    if n>=offset&&n<end{pageSteps=append(pageSteps,step)}
   }
   first+=ref.Count
  }
  if len(pageSteps)!=end-offset{return ErrInvalidSession}
  allDone:=done==total
  if x.TaskPlan.State=="completed"&&(!allDone||x.TaskPlan.AutoContinue){
   return ErrInvalidSession
  }
  plan:=*x.TaskPlan
  plan.Steps=pageSteps
  result.Plan=&plan
  result.Done=done
  result.Total=total
  result.AllDone=allDone
  result.Completed=plan.State=="completed"
  result.Page.TotalSteps=total
  result.Page.HasNext=end<total
  return nil
 })
 if err!=nil{return TaskPlanPageResult{},err}
 return result,nil
}
