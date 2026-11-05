//go:build linux || darwin

package server

import (
 "context"
 "errors"
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
