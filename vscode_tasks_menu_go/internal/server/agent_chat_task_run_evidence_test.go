package server

import (
 "net/http"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestTaskRunEvidenceRejectsNonNativeAndCrossProjectReceipts(t *testing.T){
 workspace:=t.TempDir()
 plan,err:=agentchat.NewTaskPlan("Verify requested change",[]agentchat.TaskPlanStep{
  {ID:"test",Title:"Run independent tests",Acceptance:"User checks real TaskDeck test command and exit code"},
 })
 if err!=nil{t.Fatal(err)}
 x:=agentchat.Session{Owner:"local",ProjectID:"project-123",TaskPlan:plan}
 now:=time.Now().UTC()
 code:=0
 good:=session.Metadata{ID:"run-verified",TaskID:42,Label:"Tests",CommandPreview:"go test ./...",
  CommandSHA256:strings.Repeat("a",64),Cwd:workspace,Status:"exited",ExitCode:&code,
  StartedAt:now.Format(time.RFC3339),EndedAt:now.Add(time.Second).Format(time.RFC3339)}
 if _,err=verifyNativeTaskRunMetadata(workspace,false,x,good,42,"Reviewed test scope");err!=nil{
  t.Fatalf("valid trusted native Task metadata rejected: %v",err)
 }
 cases:=map[string]func(*session.Metadata){
  "wrong_task_id":func(m *session.Metadata){m.TaskID=41},
  "terminal_instead_of_task":func(m *session.Metadata){m.Kind=tasks.SessionKindTerminal},
  "stopped_but_zero":func(m *session.Metadata){m.Status="stopped"},
  "failed_exit":func(m *session.Metadata){exit:=3;m.ExitCode=&exit},
  "missing_exit":func(m *session.Metadata){m.ExitCode=nil},
  "missing_command_digest":func(m *session.Metadata){m.CommandSHA256=""},
  "bad_command_digest":func(m *session.Metadata){m.CommandSHA256=strings.Repeat("z",64)},
  "outside_workspace":func(m *session.Metadata){m.Cwd=t.TempDir()},
  "stale_pre_plan":func(m *session.Metadata){m.StartedAt=plan.CreatedAt.Add(-time.Hour).Format(time.RFC3339)},
  "missing_end":func(m *session.Metadata){m.EndedAt=""},
  "owner_mismatch":func(m *session.Metadata){m.OwnerUserID="other-user"},
  "project_mismatch":func(m *session.Metadata){m.ProjectID="other-project"},
 }
 for name,change:=range cases{
  t.Run(name,func(t *testing.T){
   bad:=good
   change(&bad)
   if _,err:=verifyNativeTaskRunMetadata(workspace,false,x,bad,42,"");err==nil{
    t.Fatalf("%s received a verified Task receipt despite mismatch",name)
   }
  })
 }
 scoped:=good
 scoped.Kind=tasks.SessionKindTask
 scoped.OwnerUserID="alice"
 scoped.ProjectID=x.ProjectID
 x.Owner="user:alice"
 if _,err:=verifyNativeTaskRunMetadata(workspace,true,x,scoped,42,"");err!=nil{
  t.Fatalf("matching authenticated shared Task rejected: %v",err)
 }
 x.Owner="user:bob"
 if _,err:=verifyNativeTaskRunMetadata(workspace,true,x,scoped,42,"");err==nil{
  t.Fatal("another principal could accept someone else's run")
 }
}

func TestTaskPlanAPIAdoptsActualSuccessfulTaskRunWithoutProviderOrPatch(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("PTY-based Task execution fixture uses POSIX shell")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan"
 create:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"create","expected_revision":x.Revision,"goal":"Verify the independent native Task output",
  "steps":[]map[string]string{{"id":"test","title":"Run project tests","acceptance":"Inspect the exact native test Task's command and exit code"}},
 })
 if create.Code!=200{t.Fatalf("create Task plan: %d %s",create.Code,create.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 manager:=session.NewManager(1<<20)
 s.Sessions=manager
 defer manager.Shutdown(time.Second)
 launch:=func(script string) session.Metadata{
  t.Helper()
  meta,err:=manager.Start(tasks.Execution{TaskID:42,Label:"Unit tests",Command:"/bin/sh",
   Args:[]string{"-c",script},Cwd:s.Workspace,Env:os.Environ(),Preview:"/bin/sh -c "+script})
  if err!=nil{t.Fatal(err)}
  return waitTaskRunSession(t,manager,meta.ID)
 }
 failed:=launch("exit 3")
 good:=launch("exit 0")
 if good.Status!="exited"||good.ExitCode==nil||*good.ExitCode!=0||len(good.CommandSHA256)!=64{
  t.Fatalf("native Task did not produce finished hashed command evidence: %+v",good)
 }
 try:=func(ref string,taskID int,expect int){
  t.Helper()
  current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  rr:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
   "action":"set_step","expected_revision":current.Revision,
   "step_id":"test","status":"done","evidence_kind":"task_run",
   "evidence_ref":ref,"evidence_task_id":taskID,"evidence_note":"Reviewed exact test command",
  })
  if rr.Code!=expect{t.Fatalf("Task receipt ref=%s task=%d code=%d want=%d body=%s",
   ref,taskID,rr.Code,expect,rr.Body.String())}
 }
 try("invented-native-run",42,http.StatusConflict)
 try(failed.ID,42,http.StatusConflict)
 try(good.ID,41,http.StatusConflict)
 try(good.ID,42,http.StatusOK)
 persisted,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 step:=persisted.TaskPlan.Steps[0]
 if step.Status!="done"||len(step.Evidence)!=1||step.Evidence[0].Kind!="task_run"||
  step.Evidence[0].Ref!=good.ID||len(step.Evidence[0].Digest)!=64{
  t.Fatalf("native receipt was not durably recorded: %+v",step)
 }
 if persisted.TaskPlan.State=="completed"{t.Fatal("Task exit 0 silently completed whole plan")}
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 afterRestart,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(afterRestart.TaskPlan.Steps[0].Evidence)!=1||
  afterRestart.TaskPlan.Steps[0].Evidence[0].Digest!=step.Evidence[0].Digest{
  t.Fatal("Task execution proof was lost across store reopening")
 }
 // Another Task ID cannot silently replace the accepted exact session.
 noEvidence:=step.Evidence[0]
 if noEvidence.Kind!="task_run"{t.Fatal("source was mislabeled as AI/manual evidence")}
 _=filepath.Base(s.Workspace)
}
