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

func TestRemoteGitMutationSlotExcludesTaskAndPTY(t *testing.T){
 _,session,_,_:=remoteDispatchFixture(t)
 release,err:=session.reserveRemoteGitMutation(context.Background())
 if err!=nil||release==nil{t.Fatalf("idle session denied: %v",err)}
 start:=time.Now()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||
  time.Since(start)>time.Second{
  t.Fatalf("overlapping Git operation did not fail fast: %v",err)
 }
 release()
 session.longTaskMu.Lock()
 session.longTask=&remoteLongTaskJob{taskID:123}
 session.longTaskMu.Unlock()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("live long job admitted Git: %v",err)
 }
 session.longTaskMu.Lock()
 session.longTask=nil
 session.longTaskMu.Unlock()
 session.terminalMu.Lock()
 session.terminal=&remoteTerminalJob{}
 session.terminalMu.Unlock()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("host PTY admitted Git: %v",err)
 }
 session.terminalMu.Lock()
 session.terminal=nil
 session.terminalMu.Unlock()
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 _,err=session.reserveRemoteGitMutation(ctx)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("cancelled operation admitted Git: %v",err)
 }
 release,err=session.reserveRemoteGitMutation(context.Background())
 if err!=nil||release==nil{t.Fatalf("Git slot not released: %v",err)}
 release()
 session.Close()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("revoked session admitted Git: %v",err)
 }
}

// Two independently authenticated sessions targeting the SAME canonical
// host workspace must not concurrently mutate its Git index or HEAD.
func TestRemoteGitWorkspaceMutationExcludesOtherSessions(t *testing.T){
 root:=t.TempDir()
 left:=&Server{Workspace:root}
 right:=&Server{Workspace:root}
 first:=&remoteTaskDeckSession{}
 second:=&remoteTaskDeckSession{}
 release,err:=left.reserveRemoteGitMutation(context.Background(),first)
 if err!=nil{t.Fatalf("first workspace reservation failed: %v",err)}
 began:=time.Now()
 _,err=right.reserveRemoteGitMutation(context.Background(),second)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||
  time.Since(began)>time.Second{
  t.Fatalf("second authenticated session overlapped Git mutation: %v",err)
 }
 // Locking one repo must not accidentally lock a different workspace.
 unrelated:=&Server{Workspace:t.TempDir()}
 free,err:=unrelated.reserveRemoteGitMutation(context.Background(),second)
 if err!=nil{t.Fatalf("unrelated repository was unnecessarily blocked: %v",err)}
 free()
 release()
 release() // safe even if two teardown paths both attempt cleanup
 acquired,err:=right.reserveRemoteGitMutation(context.Background(),second)
 if err!=nil{t.Fatalf("released Git workspace remained busy: %v",err)}
 acquired()
}

func TestRemoteGitWorkspaceMutationCanonicalSymlinkAlias(t *testing.T){
 root:=t.TempDir()
 alias:=filepath.Join(t.TempDir(),"workspace-link")
 if err:=os.Symlink(root,alias);err!=nil{
  t.Skipf("filesystem does not allow a symlink alias: %v",err)
 }
 direct:=&Server{Workspace:root}
 linked:=&Server{Workspace:alias}
 owner:=&remoteTaskDeckSession{}
 other:=&remoteTaskDeckSession{}
 release,err:=direct.reserveRemoteGitMutation(context.Background(),owner)
 if err!=nil{t.Fatal(err)}
 _,err=linked.reserveRemoteGitMutation(context.Background(),other)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatal("same workspace symlink escaped cross-session Git exclusion")
 }
 release()
 next,err:=linked.reserveRemoteGitMutation(context.Background(),other)
 if err!=nil{t.Fatal(err)}
 next()
}

// All remote action families use a single in-process workspace
// reservation. A Git action may not overlap a different session's
// Task/PTY lease, but another project directory is unaffected.
func TestRemoteWorkspaceExecutionBlocksCrossSessionGit(t *testing.T){
 root:=t.TempDir()
 host:=&Server{Workspace:root}
 first:=&remoteTaskDeckSession{}
 second:=&remoteTaskDeckSession{}
 releaseTask,err:=reserveRemoteWorkspaceExecution(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 _,err=host.reserveRemoteGitMutation(context.Background(),first)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("remote Git started during another session's task: %v",err)
 }
 releaseTask()
 releaseGit,err:=host.reserveRemoteGitMutation(context.Background(),first)
 if err!=nil{t.Fatal(err)}
 _,err=reserveRemoteWorkspaceExecution(context.Background(),root)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("task or PTY started during another session's Git: %v",err)
 }
 unrelated,err:=reserveRemoteWorkspaceExecution(context.Background(),t.TempDir())
 if err!=nil{t.Fatalf("independent workspace blocked: %v",err)}
 unrelated()
 releaseGit()
 releaseTask,err=reserveRemoteWorkspaceExecution(context.Background(),root)
 if err!=nil{t.Fatalf("workspace lease leaked: %v",err)}
 releaseTask()
 releaseOther,err:=host.reserveRemoteGitMutation(context.Background(),second)
 if err!=nil{t.Fatal(err)}
 releaseOther()
}

 
func TestRemoteGitAllowsCompletedRetainedTaskOnlyAfterWorkspaceRelease(t *testing.T){
 s,session,_,_:=remoteDispatchFixture(t)
 ctx:=context.Background()
 capture:=newRemoteLongTaskCapture()
 if _,err:=capture.Write([]byte("retained-output"));err!=nil{t.Fatal(err)}
 done:=make(chan struct{})
 job:=&remoteLongTaskJob{
  taskID:127,done:done,status:"running",exitCode:-1,
  capture:capture,finalizing:false,
 }
 session.longTaskMu.Lock()
 session.longTask=job
 session.longTaskMu.Unlock()
 if _,err:=s.reserveRemoteGitMutation(ctx,session);
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("running long task admitted Git mutation: %v",err)
 }
 // Host has classified exit but has NOT released process-group lease.
 job.mu.Lock()
 job.finalizing=true
 job.status="completed"
 job.exitCode=0
 job.mu.Unlock()
 if _,err:=s.reserveRemoteGitMutation(ctx,session);
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("finalizing task admitted Git before done: %v",err)
 }
 // Even if an incorrect caller closed done prematurely, the independent
 // canonical workspace lease still rejects mutation across E2EE sessions.
 releaseWorkspace,err:=reserveRemoteWorkspaceExecution(ctx,s.Workspace)
 if err!=nil{t.Fatal(err)}
 close(done)
 if _,err:=s.reserveRemoteGitMutation(ctx,session);
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  releaseWorkspace()
  t.Fatalf("workspace lease did not guard Git after task completion: %v",err)
 }
 releaseWorkspace()
 releaseGit,err:=s.reserveRemoteGitMutation(ctx,session)
 if err!=nil{t.Fatalf("finished retained task unexpectedly blocked Git: %v",err)}
 releaseGit()
 if session.longTask!=job{
  t.Fatal("Git mutation discarded the completed task's output history")
 }
 remaining,err:=session.pollRemoteLongTaskWithWait(ctx,127,10*time.Millisecond)
 if err!=nil||remaining.Status!="completed"||remaining.Output!="retained-output"{
  t.Fatalf("completed task output lost after Git: %+v %v",remaining,err)
 }
 if err:=session.clearRemoteLongTask(127);err!=nil{
  t.Fatalf("completed task can no longer be cleared: %v",err)
 }
}

func TestRemoteGitRetainedJobWithInvalidDoneOrStatusDenied(t *testing.T){
 session:=&remoteTaskDeckSession{}
 for _,tt:=range []struct{
  name string
  status string
  finalize bool
  closeDone bool
 }{
  {"nil_done","completed",true,false},
  {"done_open","completed",true,false},
  {"not_finalizing","completed",false,true},
  {"still_running","running",true,true},
 }{
  t.Run(tt.name,func(t *testing.T){
   job:=&remoteLongTaskJob{status:tt.status,finalizing:tt.finalize}
   if tt.name!="nil_done"{job.done=make(chan struct{})}
   if tt.closeDone{close(job.done)}
   session.longTask=job
   release,err:=session.reserveRemoteGitMutation(context.Background())
   if release!=nil||!errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("inconsistent host task state admitted Git: %v",err)
   }
  })
 }
}
