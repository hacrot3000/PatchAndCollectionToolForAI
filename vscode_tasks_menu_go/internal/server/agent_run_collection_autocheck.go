package server

import (
 "context"
 "errors"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
)

// applyRunCollectionAutoChecks is called only after the active provider
// attempt completed successfully, before the manager selects its next prompt.
// It does not execute tools or manufacture receipt records. All evidence is
// derived from exact pairs in the pinned snapshot and the current run journal.
func applyRunCollectionAutoChecks(ctx context.Context,x *agentchat.Session,run agentrun.Record,snapshot agentrun.ContextSnapshot)(bool,error){
 if err:=ctx.Err();err!=nil{return false,err}
 if x==nil||x.Owner!="local"||x.Archived||x.TaskPlan==nil||x.TaskPlan.State!="active"{return false,nil}
 if run.Status!=agentrun.Running||run.Spec.ConversationID!=x.ID||snapshot.ProjectID!=x.ProjectID{
  return false,errors.New("run Auto-check scope mismatch")
 }
 changed:=false
 for i:=range x.TaskPlan.Steps{
  if err:=ctx.Err();err!=nil{return false,err}
  step:=&x.TaskPlan.Steps[i]
  rule:=step.AutoCheck
  if rule==nil||rule.Kind!="full_file_read"||step.Status=="done"||
   step.GoTestRequirements!=nil||!agentchat.TaskPlanStepCriteriaReviewed(*step){continue}
  runID,callID,ok:=parseRunCollectionEvidenceRef(rule.CallID)
  if !ok||runID!=run.Spec.RunID{continue}
  proof,err:=verifyRunCollectionProof(run,snapshot,x.ProjectID,callID,"",rule.Path)
  if err!=nil{continue}
  proof.Ref=rule.CallID
  // Work on a private plan copy: UpdateTaskPlanStep may fail validation after
  // mutating its argument. Never persist a partial mutation on that path.
  candidate:=*x
  plan:=*x.TaskPlan
  plan.Steps=append([]agentchat.TaskPlanStep(nil),plan.Steps...)
  plan.Steps[i].Evidence=append([]agentchat.TaskPlanEvidence(nil),step.Evidence...)
  candidate.TaskPlan=&plan
  if err:=agentchat.UpdateTaskPlanStep(&candidate,step.ID,"done",&proof);err!=nil{return false,err}
  x.TaskPlan=candidate.TaskPlan
  x.AppendEvent("plan_proof_verified",x.Status,rule.CallID,step.ID)
  changed=true
 }
 return changed,nil
}
