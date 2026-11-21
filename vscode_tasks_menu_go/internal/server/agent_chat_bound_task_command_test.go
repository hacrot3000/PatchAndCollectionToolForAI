package server

import (
 "net/http"
 "os"
 "runtime"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestBoundTaskCommandRejectsWrongTaskCommandAndPriorRun(t *testing.T){
 workspace:=t.TempDir()
 p,err:=agentchat.NewTaskPlan("Verify a real test command",
  []agentchat.TaskPlanStep{{ID:"suite",Title:"Run test suite",Acceptance:"Native command identity and exit code 0"}})
 if err!=nil{t.Fatal(err)}
 x:=agentchat.Session{Owner:"local",ProjectID:"project",TaskPlan:p}
 now:=time.Now().UTC()
 zero:=0
 digest:=strings.Repeat("b",64)
 good:=session.Metadata{
  ID:"real-session",TaskID:42,Kind:tasks.SessionKindTask,CommandPreview:"go test ./...",
  CommandSHA256:digest,Cwd:workspace,Status:"exited",ExitCode:&zero,
  StartedAt:now.Format(time.RFC3339),EndedAt:now.Add(time.Second).Format(time.RFC3339),
 }
 x.TaskPlan.Steps[0].TaskCommandGuard=&agentchat.TaskCommandGuard{
  TaskID:42,CommandSHA256:digest,BoundAt:now.Add(-10*time.Second),
 }
 if err:=agentchat.ValidateTaskPlan(p);err!=nil{t.Fatal(err)}
 if _,err:=verifyNativeTaskRunMetadata(workspace,false,x,good,42,"","suite");err!=nil{
  t.Fatalf("matching signed or live Task metadata was refused: %v",err)
 }
 cases:=map[string]func(*session.Metadata){
  "changed_command_argv":func(m *session.Metadata){m.CommandSHA256=strings.Repeat("c",64)},
  "wrong_task_id":func(m *session.Metadata){m.TaskID=43},
  "pre_binding":func(m *session.Metadata){m.StartedAt=now.Add(-1*time.Hour).Format(time.RFC3339)},
  "same_second":func(m *session.Metadata){m.StartedAt=now.Add(-10*time.Second).Format(time.RFC3339)},
  "exit_failed":func(m *session.Metadata){code:=1;m.ExitCode=&code},
 }
 for name,change:=range cases{
  t.Run(name,func(t *testing.T){
   bad:=good
   change(&bad)
   if _,err:=verifyNativeTaskRunMetadata(workspace,false,x,bad,42,"","suite");err==nil{
    t.Fatalf("%s was treated as matching new successful test command",name)
   }
  })
 }
 x.TaskPlan.Steps[0].TaskCommandGuard.CommandSHA256="bogus"
 if err:=agentchat.ValidateTaskPlan(x.TaskPlan);err==nil{
  t.Fatal("invalid test command digest was accepted in durable plan")
 }
 x.TaskPlan.Steps[0].TaskCommandGuard.CommandSHA256=strings.ToUpper(digest)
 if err:=agentchat.ValidateTaskPlan(x.TaskPlan);err==nil{
  t.Fatal("noncanonical uppercase test command fingerprint was accepted")
 }
}

func TestBoundTestCommandPlanAPIRequiresConsentAndFreshMatchingNativeRun(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("native PTY Task fixture uses POSIX shell")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan"
 created:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"create","expected_revision":x.Revision,"goal":"Test an implementation with the bound command",
  "steps":[]map[string]string{{"id":"test","title":"Run target tests","acceptance":"Exact test command and Task identity"}},
 })
 if created.Code!=200{t.Fatalf("plan create: %d %s",created.Code,created.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 manager:=session.NewManager(1<<20)
 s.Sessions=manager
 defer manager.Shutdown(time.Second)
 launch:=func(taskID int,script string)session.Metadata{
  t.Helper()
  started,err:=manager.Start(tasks.Execution{TaskID:taskID,Label:"Tests",Command:"/bin/sh",
   Args:[]string{"-c",script},Cwd:s.Workspace,Env:os.Environ(),Preview:"/bin/sh -c "+script})
  if err!=nil{t.Fatal(err)}
  meta:=waitTaskRunSession(t,manager,started.ID)
  if meta.Status!="exited"||meta.ExitCode==nil||*meta.ExitCode!=0||len(meta.CommandSHA256)!=64{
   t.Fatalf("native test fixture failed: %+v",meta)
  }
  return meta
 }
 baseline:=launch(31,"exit 0")
 badConsent:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"test","enabled":true,"evidence_task_id":31,
  "expected_command_sha256":baseline.CommandSHA256,
 })
 if badConsent.Code!=http.StatusForbidden{t.Fatalf("missing user approval accepted: %d %s",badConsent.Code,badConsent.Body)}
 badDigest:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"test","enabled":true,"confirm_command_guard":true,"evidence_task_id":31,
  "expected_command_sha256":"invalid",
 })
 if badDigest.Code!=http.StatusBadRequest{t.Fatalf("malformed expected hash accepted: %d %s",badDigest.Code,badDigest.Body)}
 accepted:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"test","enabled":true,"confirm_command_guard":true,"evidence_task_id":31,
  "expected_command_sha256":baseline.CommandSHA256,
 })
 if accepted.Code!=http.StatusOK{t.Fatalf("valid user pinning failed: %d %s",accepted.Code,accepted.Body)}
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 guard:=x.TaskPlan.Steps[0].TaskCommandGuard
 if guard==nil||guard.CommandSHA256!=baseline.CommandSHA256||guard.TaskID!=31{
  t.Fatalf("user-bound test identity lost: %+v",guard)
 }
 if again:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision-1,
  "step_id":"test","enabled":false,"confirm_command_guard":true,
 });again.Code!=http.StatusConflict{t.Fatalf("stale binding change accepted: %d",again.Code)}
 try:=func(meta session.Metadata,taskID int,want int){
  t.Helper()
  current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  result:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
   "action":"set_step","expected_revision":current.Revision,
   "step_id":"test","status":"done","evidence_kind":"task_run",
   "evidence_ref":meta.ID,"evidence_task_id":taskID,
  })
  if result.Code!=want{t.Fatalf("native run %s task=%d code=%d want=%d: %s",
   meta.ID,taskID,result.Code,want,result.Body.String())}
 }
 try(baseline,31,http.StatusConflict)
 changed:=launch(31,"exit 0; true")
 try(changed,31,http.StatusConflict)
 // The Task service records RFC3339 seconds. Wait for the next wall-clock
 // second so the new run unambiguously starts after the bound identity.
 time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second))+100*time.Millisecond)
 matching:=launch(31,"exit 0")
 try(matching,32,http.StatusConflict)
 try(matching,31,http.StatusOK)
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if x.TaskPlan.Steps[0].Status!="done"||len(x.TaskPlan.Steps[0].Evidence)!=1||
  x.TaskPlan.State=="completed"{t.Fatal("new run was not accepted exactly once without finishing entire plan")}
 denied:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,
  "step_id":"test","enabled":false,"confirm_command_guard":true,
 })
 if denied.Code!=http.StatusConflict{t.Fatalf("accepted proof was repinned: %d",denied.Code)}
 again,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 persisted,err:=again.Get(id,"local");if err!=nil{t.Fatal(err)}
 if persisted.TaskPlan.Steps[0].TaskCommandGuard==nil||
  persisted.TaskPlan.Steps[0].TaskCommandGuard.CommandSHA256!=matching.CommandSHA256{
  t.Fatal("guard not retained after durable store reopen")
 }
 handoff,_,err:=again.Handoff(id,"local")
 if err!=nil||!strings.Contains(handoff,"Owner-bound expected test command")||
  !strings.Contains(handoff,matching.CommandSHA256){t.Fatalf("handoff omitted command pinning: %v %s",err,handoff)}
}
