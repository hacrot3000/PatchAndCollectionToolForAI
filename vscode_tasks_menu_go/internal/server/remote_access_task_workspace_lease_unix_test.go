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
)

// An actual short host task must own the canonical workspace for its full
// process-group lifetime. Other authenticated E2EE sessions may not run
// remote Git mutations until the cancelled command and children exit.
func TestRemoteShortTaskCrossSessionWorkspaceLease(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Running with shared-workspace exclusion",
  "type":"process","command":"/bin/sh",
  "args":["-c","touch task-lease-started; sleep 20"]}]}`)
 taskCtx,cancel:=context.WithCancel(context.Background())
 defer cancel()
 finished:=make(chan error,1)
 go func(){
  _,err:=s.runRemoteTaskBoundedWithInput(taskCtx,session,id,0,"")
  finished<-err
 }()
 marker:=filepath.Join(s.Workspace,"task-lease-started")
 if !remoteTestAwaitFile(marker,3*time.Second){
  cancel()
  select{case <-finished:case <-time.After(3*time.Second):}
  t.Fatal("bounded task never started a host process")
 }
 other:=&remoteTaskDeckSession{}
 began:=time.Now()
 _,err:=s.reserveRemoteGitMutation(context.Background(),other)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||
  time.Since(began)>time.Second{
  cancel()
  t.Fatalf("second session overlapped running short task: %v",err)
 }
 cancel()
 select{
 case err:=<-finished:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("cancelled task unexpectedly succeeded: %v",err)
  }
 case <-time.After(4*time.Second):
  t.Fatal("short task did not finish promptly on cancellation")
 }
 release,err:=s.reserveRemoteGitMutation(context.Background(),other)
 if err!=nil{
  t.Fatalf("exited short task leaked canonical workspace lease: %v",err)
 }
 release()
 if _,err:=os.Stat(marker);err!=nil{
  t.Fatalf("test task side-effect marker disappeared: %v",err)
 }
}

// Denied task preflight must not strand the workspace lease, including
// in a session with no launched process or host output to poll.
func TestRemoteShortTaskDeniedPreflightReleasesWorkspace(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Stale task",
  "type":"process","command":"/bin/sh","args":["-c","touch must-not-run"]}]}`)
 doc:=filepath.Join(s.Workspace,".vscode","tasks.json")
 file,err:=os.OpenFile(doc,os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{t.Fatal(err)}
 if _,err:=file.WriteString(" ");err!=nil{file.Close();t.Fatal(err)}
 if err:=file.Close();err!=nil{t.Fatal(err)}
 if _,err:=s.runRemoteTaskBoundedWithInput(context.Background(),session,id,0,"");
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("stale task unexpectedly started: %v",err)
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"must-not-run"));
  !os.IsNotExist(err){
  t.Fatalf("denied task produced side effects: %v",err)
 }
 other:=&remoteTaskDeckSession{}
 release,err:=s.reserveRemoteGitMutation(context.Background(),other)
 if err!=nil{t.Fatalf("rejected task leaked workspace lease: %v",err)}
 release()
}
