//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func remoteTestAwaitFile(path string,duration time.Duration)bool{
 deadline:=time.Now().Add(duration)
 for time.Now().Before(deadline){
  if _,err:=os.Stat(path);err==nil{return true}
  time.Sleep(15*time.Millisecond)
 }
 return false
}

// A local /api/close or a revoked Portal session cancels the parent task
// and the entire process group, not only /bin/sh. No Windows-specific path
// is involved in this test or implementation.
func TestRemoteTaskRunCancelledContextKillsChildProcessGroup(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Long-running shell","type":"process",
  "command":"/bin/sh","args":["-c",
   "echo ready > started; (sleep 2; touch escaped-marker) >/dev/null 2>&1 & sleep 10"]}]}`)
 ctx,cancel:=context.WithCancel(context.Background())
 defer cancel()
 finished:=make(chan error,1)
 go func(){
  _,err:=s.runRemoteTaskBounded(ctx,session,taskID,0)
  finished<-err
 }()
 if !remoteTestAwaitFile(filepath.Join(s.Workspace,"started"),3*time.Second){
  cancel()
  t.Fatal("task process never started; cancellation test is invalid")
 }
 cancel()
 select{
 case err:=<-finished:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("cancelled task unexpectedly reported success: %v",err)
  }
 case <-time.After(3*time.Second):
  t.Fatal("cancellation failed to terminate running process group")
 }
 time.Sleep(2300*time.Millisecond)
 if _,err:=os.Stat(filepath.Join(s.Workspace,"escaped-marker"));!os.IsNotExist(err){
  t.Fatalf("cancel left an orphaned subprocess able to write: %v",err)
 }
}

func TestRemoteTaskRunNormalExitKillsBackgroundChildren(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Short with stray background task","type":"process",
  "command":"/bin/sh","args":["-c",
   "(sleep 1; touch stray-marker) >/dev/null 2>&1 & echo complete"]}]}`)
 result,err:=s.runRemoteTaskBounded(context.Background(),session,taskID,0)
 if err!=nil||result.ExitCode!=0||result.Output!="complete\n"{
  t.Fatalf("short task did not finish cleanly: %+v %v",result,err)
 }
 time.Sleep(1350*time.Millisecond)
 if _,err:=os.Stat(filepath.Join(s.Workspace,"stray-marker"));!os.IsNotExist(err){
  t.Fatalf("completed task left a detached subprocess: %v",err)
 }
}

func TestRemoteTaskRunSessionCloseCancelsRunningChildProcessGroup(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Cancels on native logout","type":"process",
   "command":"/bin/sh","args":["-c",
   "echo ready > logged-in-task; (sleep 2; touch escaped-after-logout) >/dev/null 2>&1 & sleep 10"]}]}`)
 action,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:"44444444444444444444444444444444",
  Operation:remotetransport.RemoteActionTasksRun,TaskID:taskID,
 })
 if err!=nil{t.Fatal(err)}
 done:=make(chan error,1)
 go func(){
  _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,action)
  done<-err
 }()
 if !remoteTestAwaitFile(filepath.Join(s.Workspace,"logged-in-task"),3*time.Second){
  session.Close()
  t.Fatal("authenticated task never reached the process start")
 }
 began:=time.Now()
 session.Close() // native /api/close revokes the same authenticated session
 if time.Since(began)>time.Second{
  t.Fatal("TaskDeck session Close blocked while remote task was running")
 }
 select{
 case err:=<-done:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("revoked session delivered a task result: %v",err)
  }
 case <-time.After(3*time.Second):
  t.Fatal("revoked E2EE session left a running task")
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 finish,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.tasks_run_finished",Limit:5,
 })
 if err!=nil||len(finish)!=1||finish[0].Result!="cancelled"{
  t.Fatalf("revoked task was not recorded as cancelled: %+v %v",finish,err)
 }
 time.Sleep(2300*time.Millisecond)
 if _,err:=os.Stat(filepath.Join(s.Workspace,"escaped-after-logout"));!os.IsNotExist(err){
  t.Fatalf("session logout left a child process able to write: %v",err)
 }
}
