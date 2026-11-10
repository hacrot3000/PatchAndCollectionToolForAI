//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteLongTaskStagedSessionBoundedChunksAndIndependentCancellation(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Streaming","type":"process","command":"/bin/sh",
    "args":["-c","printf one; sleep 0.6; printf two; sleep 10; touch should-not-exist"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("authenticated host long task never started: %v",err)
 }
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err==nil{
  t.Fatal("same E2EE session started a second concurrent command")
 }
 // A short run must not execute while this session still owns the long
 // job. Checking only terminalMu allows a second process to be launched
 // after TasksStart has released its initial E2EE action.
 began:=time.Now()
 if _,err:=s.runRemoteTaskBoundedWithInput(context.Background(),session,id,0,"");
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("short task overlapped an existing long job: %v",err)
 }
 if time.Since(began)>time.Second{
  t.Fatal("short task attempted to execute instead of rejecting the active job")
 }
 first,err:=session.pollRemoteLongTask(context.Background(),id)
 if err!=nil||first.TaskID!=id||first.Status!="running"||first.Output!="one"{
  t.Fatalf("first incremental output missing: %+v %v",first,err)
 }
 second,err:=session.pollRemoteLongTask(context.Background(),id)
 if err!=nil||second.Output!="two"||second.Status!="running"{
  t.Fatalf("second incremental output missing: %+v %v",second,err)
 }
 start:=time.Now()
 if err:=session.cancelRemoteLongTask(id);err!=nil{t.Fatal(err)}
 if time.Since(start)>time.Second{t.Fatal("independent task cancel blocked")}
 select{
 case <-session.longTask.done:
 case <-time.After(3*time.Second):
  t.Fatal("long task did not stop after independent cancel")
 }
 final,err:=session.pollRemoteLongTask(context.Background(),id)
 if err!=nil||final.Status!="cancelled"{
  t.Fatalf("canceled job was not observable while session stayed open: %+v %v",final,err)
 }
 if session.closed.Load()||!session.channel.Established(){
  t.Fatal("task cancellation wrongly revoked entire E2EE session")
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"should-not-exist"));!os.IsNotExist(err){
  t.Fatal("cancelled job left side effects")
 }
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
 if session.longTask!=nil{t.Fatal("finished job retained after explicit release")}
}

func TestRemoteLongTaskSessionCloseKillsChildrenWithoutPolling(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Background","type":"process","command":"/bin/sh",
    "args":["-c","echo started > task-started; (sleep 2; touch escaped) >/dev/null 2>&1 & sleep 20"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{t.Fatal(err)}
 if !remoteTestAwaitFile(filepath.Join(s.Workspace,"task-started"),3*time.Second){
  session.Close()
  t.Fatal("host command did not begin")
 }
 began:=time.Now()
 session.Close()
 if time.Since(began)>time.Second{t.Fatal("logout blocked on long-running task")}
 select{
 case <-session.longTask.done:
 case <-time.After(3*time.Second):
  t.Fatal("session logout left long job running")
 }
 if err:=session.cancelRemoteLongTask(id);!errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatal("revoked session admitted task cancellation")
 }
 time.Sleep(2300*time.Millisecond)
 if _,err:=os.Stat(filepath.Join(s.Workspace,"escaped"));!os.IsNotExist(err){
  t.Fatal("session logout did not kill job's child process group")
 }
}

func TestRemoteLongTaskStartDeniedWithoutOptInOrStaleSource(t *testing.T){
 for _,enable:=range []string{"0","1"}{
  t.Run(enable,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
   t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
   t.Setenv("TASKDECK_REMOTE_TASKS_LONG",enable)
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   doc:=`{"tasks":[{"label":"No side effects","type":"process",
      "command":"/bin/sh","args":["-c","touch created"]}]}`
   id:=previewOneRemoteRunTask(t,s,session,client,agent,doc)
   if enable=="1" {
    file:=filepath.Join(s.Workspace,".vscode","tasks.json")
    if err:=os.WriteFile(file,[]byte(doc+" "),0600);err!=nil{t.Fatal(err)}
   }
   if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err==nil{
    t.Fatal("opt-out or changed snapshot launched process")
   }
   if _,err:=os.Stat(filepath.Join(s.Workspace,"created"));!os.IsNotExist(err){
    t.Fatal("denied long task modified host workspace")
   }
  })
 }
}

func TestRemoteLongTaskCaptureBoundedAndUtf8Safe(t *testing.T){
 c:=newRemoteLongTaskCapture()
 content:=append([]byte("good \x1b[31m...\n"),[]byte(strings.Repeat("X",20*1024))...)
 n,err:=c.Write(content)
 if err!=nil||n!=len(content){t.Fatal("long task output was not fully drained")}
 var all strings.Builder
 cursor:=0
 sawEnd:=false
 for steps:=0;steps<130;steps++{
  chunk,next,truncated,more:=c.next(cursor)
  if !truncated||len(chunk)>remoteLongTaskMaxChunkBytes||next<cursor||
     (chunk==""&&more){
   t.Fatalf("unbounded or invalid chunk: %d %+v",next,chunk)
  }
  if strings.ContainsRune(chunk,'\x1b'){t.Fatal("terminal control was returned")}
  if !more{sawEnd=true}
  if next==cursor{break}
  all.WriteString(chunk)
  cursor=next
 }
 if !sawEnd{t.Fatal("bounded capture never signaled complete output")}
 if len(all.String())>remoteLongTaskMaxCaptureBytes{
  t.Fatal("output buffer leaked an unbounded response")
 }
}


// Completed jobs can be polled concurrently by browser actions. Each
// successful reply must consume a distinct output window, never replay it.
func TestRemoteLongTaskConcurrentPollsHaveDistinctCursors(t *testing.T){
 capture:=newRemoteLongTaskCapture()
 var output strings.Builder
 for i:=0;i<128;i++{
  output.WriteString(fmt.Sprintf("%04d%s",i,strings.Repeat("x",124)))
 }
 raw:=[]byte(output.String())
 if n,err:=capture.Write(raw);err!=nil||n!=len(raw){t.Fatal("capture setup failed")}
 job:=&remoteLongTaskJob{
  taskID:77,status:"completed",done:make(chan struct{}),
  capture:capture,exitCode:0,
 }
 close(job.done)
 session:=&remoteTaskDeckSession{longTask:job}
 start:=make(chan struct{})
 type pollResult struct{chunk string;err error}
 results:=make(chan pollResult,64)
 for i:=0;i<64;i++{
  go func(){
   <-start
   got,err:=session.pollRemoteLongTask(context.Background(),77)
   results<-pollResult{chunk:got.Output,err:err}
  }()
 }
 close(start)
 seen:=make(map[string]bool,64)
 for i:=0;i<64;i++{
  result:=<-results
  if result.err!=nil||len(result.chunk)!=remoteLongTaskMaxChunkBytes{
   t.Fatalf("concurrent poll %d returned an invalid window: %q %v",i,result.chunk,result.err)
  }
  if seen[result.chunk]{t.Fatalf("concurrent polls replayed the same output chunk: %q",result.chunk[:4])}
  seen[result.chunk]=true
 }
 job.mu.Lock()
 cursor:=job.outputCursor
 job.mu.Unlock()
 if cursor!=64*remoteLongTaskMaxChunkBytes{
  t.Fatalf("concurrent polls lost progress: cursor=%d",cursor)
 }
}

func TestRemoteLongTaskClearRestoresBoundedExecution(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Sequential","type":"process","command":"/bin/sh",
    "args":["-c","if test -f long-started; then printf after-clear; else touch long-started; sleep 10; fi"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("cannot start authenticated long job: %v",err)
 }
 if !remoteTestAwaitFile(filepath.Join(s.Workspace,"long-started"),3*time.Second){
  t.Fatal("long process did not begin")
 }
 began:=time.Now()
 if _,err:=s.runRemoteTaskBoundedWithInput(context.Background(),session,id,0,"");
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("short run was allowed before Clear: %v",err)
 }
 if time.Since(began)>time.Second{
  t.Fatal("bounded task blocked waiting on an active long job")
 }
 if err:=session.cancelRemoteLongTask(id);err!=nil{t.Fatal(err)}
 select{
 case <-session.longTask.done:
 case <-time.After(3*time.Second):t.Fatal("long job did not cancel promptly")
 }
 // A finished job must still reserve the session slot until Clear.
 if _,err:=s.runRemoteTaskBoundedWithInput(context.Background(),session,id,0,"");
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("finished but uncleared job allowed a short run: %v",err)
 }
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
 result,err:=s.runRemoteTaskBoundedWithInput(context.Background(),session,id,0,"")
 if err!=nil||result.TaskID!=id||result.ExitCode!=0||
  result.Output!="after-clear"{
  t.Fatalf("cleared long job did not release the bounded task slot: %+v %v",result,err)
 }
}


func TestRemoteLongTaskCancelIsOneShotAndFinalJobCannotBeCancelled(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Stop once","type":"process","command":"/bin/sh",
    "args":["-c","sleep 20"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("cannot start host long task: %v",err)
 }
 if err:=session.cancelRemoteLongTask(id);err!=nil{
  t.Fatalf("first cancel denied: %v",err)
 }
 if err:=session.cancelRemoteLongTask(id);
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("duplicate task cancel was accepted: %v",err)
 }
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):
  t.Fatal("cancelled host process failed to exit")
 }
 if err:=session.cancelRemoteLongTask(id);
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("finished job was cancelled again: %v",err)
 }
 progress,err:=session.pollRemoteLongTask(context.Background(),id)
 if err!=nil||progress.Status!="cancelled"{
  t.Fatalf("duplicate cancel changed final outcome: %+v %v",progress,err)
 }
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
 if err:=session.cancelRemoteLongTask(id);
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("cleared job accepted cancellation: %v",err)
 }
}

func TestRemoteLongTaskCancelRejectsAlreadyCompletedJob(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Complete fast","type":"process","command":"/bin/sh",
    "args":["-c","printf finished"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatal(err)
 }
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):t.Fatal("short host job did not complete")
 }
 if err:=session.cancelRemoteLongTask(id);
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("completed task was accepted as cancellable: %v",err)
 }
 progress,err:=session.pollRemoteLongTask(context.Background(),id)
 if err!=nil||progress.Status!="completed"||progress.Output!="finished"{
  t.Fatalf("completed task lost its result: %+v %v",progress,err)
 }
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
}


// A job's final status is a promise that its workspace is safe for the
// next authenticated operation. Exercise the exact admission ordering by
// deliberately blocking release of the OS lease; a completed process may
// already have exited but Poll must not report completed/failed/cancelled
// before the release function returns.
func TestRemoteLongTaskFinalStateWaitsForWorkspaceRelease(t *testing.T){
 for _,finalState:=range []struct{status string;code int}{
  {"completed",0},{"failed",1},{"cancelled",-1},{"timed_out",-1},
 }{
  t.Run(finalState.status,func(t *testing.T){
   releaseStarted:=make(chan struct{})
   releaseAllowed:=make(chan struct{})
   job:=&remoteLongTaskJob{
    taskID:41,status:"running",exitCode:-1,
    done:make(chan struct{}),
    capture:newRemoteLongTaskCapture(),
    workspaceRelease:func(){
     close(releaseStarted)
     <-releaseAllowed
    },
   }
   finished:=make(chan struct{})
   go func(){
    job.publishCompletion(finalState.status,finalState.code)
    close(finished)
   }()
   select{
   case <-releaseStarted:
   case <-time.After(2*time.Second):
    close(releaseAllowed)
    t.Fatal("completion never began workspace release")
   }
   job.mu.Lock()
   premature,exitCode,finalizing:=job.status,job.exitCode,job.finalizing
   job.mu.Unlock()
   if !finalizing{
    close(releaseAllowed)
    t.Fatal("job waiter reached workspace release without marking finalizing")
   }
   // A Cancel after process Wait but before lease release MUST fail;
   // otherwise a client receives a fake cancel_requested ACK even
   // though it can no longer stop this already finished process.
   session:=&remoteTaskDeckSession{longTask:job}
   if err:=session.cancelRemoteLongTask(41);
    !errors.Is(err,remotetransport.ErrRemoteActionDenied){
    close(releaseAllowed)
    t.Fatalf("post-exit Cancel was accepted during final cleanup: %v",err)
   }
   if premature!="running"||exitCode!=-1{
    close(releaseAllowed)
    t.Fatalf("terminal status visible before workspace release: %q / %d",premature,exitCode)
   }
   select{
   case <-job.done:
    close(releaseAllowed)
    t.Fatal("job.done closed while the workspace was still leased")
   default:
   }
   close(releaseAllowed)
   select{
   case <-finished:
   case <-time.After(2*time.Second):t.Fatal("completion stuck after workspace release")
   }
   job.mu.Lock()
   observed,code:=job.status,job.exitCode
   job.mu.Unlock()
   if observed!=finalState.status||code!=finalState.code{
    t.Fatalf("unexpected completed job: %q exit=%d",observed,code)
   }
   select{
   case <-job.done:
   default:t.Fatal("completion did not close job.done after workspace release")
   }
  })
 }
}


// A capture notification can be consumed by a different concurrent poll;
// the remaining waiter must still see newly buffered output at its timeout.
// The test injects an unnotified capture write to deterministically model
// this coalescing, without waiting the production seven-second deadline.
func TestRemoteLongTaskPollTimeoutRechecksCaptureAndAdvancesCursor(t *testing.T){
 job:=&remoteLongTaskJob{
  taskID:71,status:"running",exitCode:-1,
  done:make(chan struct{}),
  capture:newRemoteLongTaskCapture(),
 }
 session:=&remoteTaskDeckSession{longTask:job}
 type observed struct{
  result remoteLongTaskSnapshot
  err error
 }
 reply:=make(chan observed,1)
 go func(){
  progress,err:=session.pollRemoteLongTaskWithWait(
   context.Background(),71,100*time.Millisecond)
  reply<-observed{progress,err}
 }()
 // Wait until the first empty snapshot has entered its wait. A direct
 // capture write intentionally carries no notify, as when another
 // concurrent Poll consumed the single coalesced signal.
 time.Sleep(15*time.Millisecond)
 job.capture.mu.Lock()
 job.capture.data=append(job.capture.data,[]byte("unnotified-output")...)
 job.capture.mu.Unlock()
 select{
 case got:=<-reply:
  if got.err!=nil||got.result.Status!="running"||
   got.result.Output!="unnotified-output"||got.result.More{
   t.Fatalf("timeout ignored output that arrived during wait: %+v %v",got.result,got.err)
  }
 case <-time.After(time.Second):
  t.Fatal("bounded long-task Poll never returned")
 }
 job.mu.Lock()
 consumed:=job.outputCursor
 job.mu.Unlock()
 if consumed!=len("unnotified-output"){
  t.Fatalf("timeout delivered data without advancing cursor: %d",consumed)
 }
 // Subsequent reads cannot replay the already delivered chunk.
 doneCtx,cancel:=context.WithTimeout(context.Background(),300*time.Millisecond)
 defer cancel()
 second,err:=session.pollRemoteLongTaskWithWait(doneCtx,71,5*time.Millisecond)
 if err!=nil||second.Output!=""||second.More{
  t.Fatalf("long-task output was replayed after timed poll: %+v %v",second,err)
 }
}
