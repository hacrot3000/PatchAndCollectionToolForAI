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

func TestRemoteTaskStartsCannotOverlapLiveSessionPTY(t *testing.T){
 if os.Geteuid()==0{t.Skip("PTY host must never run TaskDeck as root")}
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTerminalAdminFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Controlled","type":"process",
    "command":"/bin/sh","args":["-c","touch task-ran"]}]}`)
 if err:=s.startRemoteTerminal(context.Background(),session);err!=nil{
  t.Fatalf("authenticated PTY setup failed: %v",err)
 }
 if _,err:=s.runRemoteTaskBoundedWithInput(context.Background(),session,id,0,"");
   !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("short task overlapped a live PTY: %v",err)
 }
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");
   !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("long task overlapped a live PTY: %v",err)
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"task-ran"));!os.IsNotExist(err){
  t.Fatalf("denied task caused host side effects: %v",err)
 }
 if _,err:=s.closeRemoteTerminal(context.Background(),session);err!=nil{
  t.Fatalf("operator cannot close the PTY: %v",err)
 }
 // Closing the PTY must not permanently block the independent task
 // permission. Re-verify the exact host-issued source and run as before.
 result,err:=s.runRemoteTaskBoundedWithInput(context.Background(),session,id,0,"")
 if err!=nil||result.TaskID!=id||result.ExitCode!=0{
  t.Fatalf("task did not resume after terminal close: %+v %v",result,err)
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"task-ran"));err!=nil{
  t.Fatalf("authorized task did not execute: %v",err)
 }
}

func TestRemoteTerminalCannotStartDuringLongTask(t *testing.T){
 if os.Geteuid()==0{t.Skip("PTY host must never run TaskDeck as root")}
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTerminalAdminFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Bounded","type":"process",
    "command":"/bin/sh","args":["-c","sleep 12"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{t.Fatal(err)}
 if err:=s.startRemoteTerminal(context.Background(),session);
   !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("PTY started while a session-owned long task existed: %v",err)
 }
 if err:=session.cancelRemoteLongTask(id);err!=nil{t.Fatal(err)}
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):t.Fatal("cancel failed to stop host process")
 }
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
 if err:=s.startRemoteTerminal(context.Background(),session);err!=nil{
  t.Fatalf("cleared job continued blocking PTY: %v",err)
 }
 if _,err:=s.closeRemoteTerminal(context.Background(),session);err!=nil{
  t.Fatalf("cannot close PTY after task: %v",err)
 }
}

// An actual host child process reserves the canonical workspace across
// distinct E2EE principals, even when the second session has no Task/PTY
// assigned. Releasing a completed job makes that workspace available.
func TestRemoteLongTaskBlocksOtherSessionGitWorkspace(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Guarded task","type":"process",
    "command":"/bin/sh","args":["-c","sleep 12"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("cannot start long host task: %v",err)
 }
 other:=&remoteTaskDeckSession{}
 _,err:=s.reserveRemoteGitMutation(context.Background(),other)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatal("second E2EE session mutated Git while long task was active")
 }
 if err:=session.cancelRemoteLongTask(id);err!=nil{t.Fatal(err)}
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):t.Fatal("cancelled host task did not exit")
 }
 // The workspace lock is released by the process waiter, not by an
 // untrusted client's Cancel response or by Clear discarding output.
 release,err:=s.reserveRemoteGitMutation(context.Background(),other)
 if err!=nil{t.Fatalf("exited task leaked workspace lock: %v",err)}
 release()
}

func TestRemoteTerminalBlocksOtherSessionGitWorkspace(t *testing.T){
 if os.Geteuid()==0{t.Skip("PTY host must never run as root")}
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 if err:=s.startRemoteTerminal(context.Background(),session);err!=nil{
  t.Fatal(err)
 }
 other:=&remoteTaskDeckSession{}
 _,err:=s.reserveRemoteGitMutation(context.Background(),other)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatal("second E2EE session mutated Git while host PTY was open")
 }
 if _,err:=s.closeRemoteTerminal(context.Background(),session);err!=nil{
  t.Fatalf("explicit terminal close failed to retire workspace lease: %v",err)
 }
 release,err:=s.reserveRemoteGitMutation(context.Background(),other)
 if err!=nil{t.Fatalf("terminal close left a stale workspace lease: %v",err)}
 release()
}
