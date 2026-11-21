package server

import (
 "context"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Check only explicitly owner-registered exact native receipts/Collection
// observations. This neither contacts an AI provider nor executes a Patch.
// Model text, event summaries, task titles and acceptance prose cannot
// authorize an automatic status transition.
func (s *Server) autoCheckTaskPlanOne(ctx context.Context,store *agentchat.Store,id string){
 if s==nil||store==nil||ctx.Err()!=nil{return}
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{return}
 defer release()
 x,err:=store.Get(id,"local")
 if err!=nil||x.Workspace!=s.Workspace||x.TaskPlan==nil||
  x.TaskPlan.State!="active"{return}
 changed:=false
 for i:=range x.TaskPlan.Steps{
  step:=&x.TaskPlan.Steps[i]
  if step.AutoCheck==nil||step.Status=="done"{continue}
  evidence,e:=agentchat.VerifyArchivedTaskAutoCheck(store,x,step.AutoCheck)
  if e!=nil{continue}
  if e:=agentchat.UpdateTaskPlanStep(&x,step.ID,"done",&evidence);e!=nil{continue}
  x.AppendEvent("plan_proof_verified",x.Status,step.AutoCheck.CallID,step.ID)
  changed=true
 }
 if !changed{return}
 x.TaskPlan.UpdatedAt=time.Now().UTC()
 // Leave the overall plan ACTIVE. A verified Patch wrapper/Collection read is
 // not semantic end-to-end acceptance; /plan complete has its own user action.
 _,_=store.Save(x,x.Revision)
}
