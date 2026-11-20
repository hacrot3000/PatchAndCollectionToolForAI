//go:build linux || darwin

package server

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func longTaskE2EEAction(
 t *testing.T,s *Server,session *remoteTaskDeckSession,
 client,agent *remotetransport.Channel,op string,taskID,choice int,value string,
 requestID string,
)(remotetransport.RemoteLongTaskProgress,error){
 t.Helper()
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:requestID,Operation:op,TaskID:taskID,Choice:choice,Text:value,
 })
 if err!=nil{t.Fatal(err)}
 frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{return remotetransport.RemoteLongTaskProgress{},err}
 return remotetransport.OpenRemoteLongTaskProgress(client,frame,requestID,taskID)
}


func TestRemoteLongTaskE2EEFailedSealPreservesOutputCursor(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 capture:=newRemoteLongTaskCapture()
 const output="AUTHENTICATED-TAIL"
 if _,err:=capture.Write([]byte(output));err!=nil{t.Fatal(err)}
 job:=&remoteLongTaskJob{
  taskID:91,status:"completed",exitCode:0,
  cancel:func(){},workspaceRelease:func(){},
  done:make(chan struct{}),capture:capture,
  workflowStep:1,workflowTotal:2,
  workflowAttempt:1,workflowAttemptTotal:1,
  workflowStepStatus:"invalid", // Fails the actual typed AEAD sealer.
  workflowStepHistory:"C-",
  workflowStepDetails:[]string{"0:0","-"},
 }
 close(job.done)
 session.longTaskMu.Lock()
 session.longTask=job
 session.longTaskMu.Unlock()
 // This is an authenticated E2EE Poll through the real dispatcher,
 // not a unit test that merely omits a commit closure.
 invalidID:="71717171717171717171717171717171"
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:invalidID,Operation:remotetransport.RemoteActionTasksPoll,TaskID:91,
 })
 if err!=nil{t.Fatal(err)}
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(sealed)!=0{
  t.Fatalf("invalid workflow metadata unexpectedly sealed: %d %v",len(sealed),err)
 }
 job.mu.Lock()
 cursor:=job.outputCursor
 job.workflowStepStatus="completed"
 job.mu.Unlock()
 if cursor!=0{
  t.Fatalf("failed E2EE Seal consumed %d output bytes",cursor)
 }
 // The channel and output must survive a rejected local response. The
 // next correctly typed Poll delivers the exact undelivered bytes.
 restored,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,91,0,"",
  "72727272727272727272727272727272")
 if err!=nil||restored.Status!="completed"||
  restored.StepStatus!="completed"||
  restored.Output!=output||restored.More{
  t.Fatalf("valid E2EE Poll failed to replay unsealed output: %+v %v",restored,err)
 }
 job.mu.Lock()
 cursor=job.outputCursor
 job.mu.Unlock()
 if cursor!=len(output){
  t.Fatalf("successful Seal failed to commit output cursor: %d",cursor)
 }
 again,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,91,0,"",
  "73737373737373737373737373737373")
 if err!=nil||again.Output!=""||again.More{
  t.Fatalf("already sealed output was delivered twice: %+v %v",again,err)
 }
}

func TestRemoteLongTaskAuthenticatedStartPollCancelClear(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"E2EE long job","type":"process","command":"/bin/sh",
    "args":["-c","printf beginning; sleep 0.4; printf middle; sleep 20; touch forbidden"]}]}`)
 started,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksStart,id,0,"",
  "22222222222222222222222222222222")
 if err!=nil||started.Status!="started"{t.Fatalf("cannot start long task: %+v %v",started,err)}
 first,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,id,0,"",
  "33333333333333333333333333333333")
 if err!=nil||first.Status!="running"||!strings.HasPrefix(first.Output,"beginning"){
  t.Fatalf("first E2EE progress missing: %+v %v",first,err)
 }
 accumulated:=first.Output
 if !strings.Contains(accumulated,"middle"){
  second,secondErr:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,id,0,"",
   "44444444444444444444444444444444")
  if secondErr!=nil{t.Fatal(secondErr)}
  accumulated+=second.Output
 }
 if !strings.Contains(accumulated,"middle"){
  t.Fatalf("incremental output failed: %q",accumulated)
 }
 stopped,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksCancel,id,0,"",
  "55555555555555555555555555555555")
 if err!=nil||stopped.Status!="cancel_requested"{
  t.Fatalf("cancel did not keep the channel open: %+v %v",stopped,err)
 }
 if session.closed.Load(){t.Fatal("independent cancel killed E2EE channel")}
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):t.Fatal("long task process group not terminated")
 }
 final,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,id,0,"",
  "66666666666666666666666666666666")
 if err!=nil||final.Status!="cancelled"{
  t.Fatalf("cancelled state not visible through E2EE: %+v %v",final,err)
 }
 cleared,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksClear,id,0,"",
  "77777777777777777777777777777777")
 if err!=nil||cleared.Status!="cleared"{t.Fatalf("completed job not cleared: %+v %v",cleared,err)}
 if _,err:=os.Stat(filepath.Join(s.Workspace,"forbidden"));!os.IsNotExist(err){
  t.Fatal("cancelled job continued side effects")
 }
}

func TestRemoteLongTaskE2EEViewerDeniedAndOptOut(t *testing.T){
 for _,enabled:=range []bool{false,true}{
  t.Run(map[bool]string{false:"host off",true:"viewer"}[enabled],func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
   t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
   if enabled{t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")}else{
    t.Setenv("TASKDECK_REMOTE_TASKS_LONG","0")
   }
   var s *Server
   var sess *remoteTaskDeckSession
   var client,agent *remotetransport.Channel
   if enabled{s,sess,client,agent=remoteDispatchFixture(t)}else{
    s,sess,client,agent=remoteTaskOperatorFixture(t)
   }
   id:=previewOneRemoteRunTask(t,s,sess,client,agent,
    `{"tasks":[{"label":"Denied long","type":"process",
      "command":"/bin/sh","args":["-c","touch created"]}]}`)
   if _,err:=longTaskE2EEAction(t,s,sess,client,agent,
    remotetransport.RemoteActionTasksStart,id,0,"",
    "22222222222222222222222222222222");err==nil{
    t.Fatal("unprivileged long task started")
   }
   if _,err:=os.Stat(filepath.Join(s.Workspace,"created"));!os.IsNotExist(err){
    t.Fatal("unauthorized task executed")
   }
  })
 }
}

func TestRemoteLongTaskTerminalBacklogIsExposedWithoutDroppingTail(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 content:=strings.Repeat("Q",420)
 document:=`{"tasks":[{"label":"Buffered output","type":"process",
  "command":"/usr/bin/printf","args":["%s","`+content+`"]}]}`
 id:=previewOneRemoteRunTask(t,s,session,client,agent,document)
 start,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksStart,id,0,"",
  "22222222222222222222222222222222")
 if err!=nil||start.Status!="started"{t.Fatalf("failed to start buffered task: %+v %v",start,err)}
 select{
 case <-session.longTask.done:
 case <-time.After(3*time.Second):t.Fatal("bounded buffered task never completed")
 }
 var got strings.Builder
 for n:=0;n<4;n++{
  requestID:=strings.Repeat(string(rune('3'+n)),32)
  next,readErr:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,id,0,"",requestID)
  if readErr!=nil||next.Status!="completed"{
   t.Fatalf("terminal state dropped backlog at chunk %d: %+v %v",n,next,readErr)
  }
  got.WriteString(next.Output)
  if n<3&&!next.More{
   t.Fatalf("terminal task lost buffered data at chunk %d",n)
  }
  if n==3&&next.More{
   t.Fatal("terminal task falsely reported remaining output after last chunk")
  }
  if !next.More{break}
 }
 if got.String()!=content{
  t.Fatalf("expected all 420 bytes, got %d",got.Len())
 }
}


func TestRemoteLongTaskE2EEDuplicateCancelFailsClosed(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"One-shot E2EE cancel","type":"process",
    "command":"/bin/sh","args":["-c","sleep 20"]}]}`)
 started,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksStart,id,0,"",
  "22222222222222222222222222222222")
 if err!=nil||started.Status!="started"{t.Fatalf("start failed: %+v %v",started,err)}
 stopped,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksCancel,id,0,"",
  "33333333333333333333333333333333")
 if err!=nil||stopped.Status!="cancel_requested"{
  t.Fatalf("first cancellation not acknowledged: %+v %v",stopped,err)
 }
 duplicate,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksCancel,id,0,"",
  "44444444444444444444444444444444")
 if err==nil||duplicate.Status=="cancel_requested"{
  t.Fatalf("host accepted second E2EE cancel: %+v %v",duplicate,err)
 }
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):t.Fatal("cancelled host task remained alive")
 }
 // Denial of a redundant action must not kill the authenticated E2EE
 // channel or erase the existing job's final output/status.
 if session.closed.Load()||!session.channel.Established(){
  t.Fatal("duplicate cancel unexpectedly revoked authenticated channel")
 }
 final,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,id,0,"",
  "55555555555555555555555555555555")
 if err!=nil||final.Status!="cancelled"{
  t.Fatalf("final task state lost after denied duplicate: %+v %v",final,err)
 }
}

func TestRemoteLongTaskE2EECompletedCancelDeniedWithoutResultLoss(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Completed E2EE task","type":"process",
    "command":"/bin/sh","args":["-c","printf completed"]}]}`)
 start,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksStart,id,0,"",
  "22222222222222222222222222222222")
 if err!=nil||start.Status!="started"{t.Fatalf("start: %+v %v",start,err)}
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):t.Fatal("task completion did not arrive")
 }
 if _,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksCancel,id,0,"",
  "33333333333333333333333333333333");err==nil{
  t.Fatal("E2EE cancel falsely acknowledged an already completed job")
 }
 final,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,id,0,"",
  "44444444444444444444444444444444")
 if err!=nil||final.Status!="completed"||final.Output!="completed"{
  t.Fatalf("completed output lost after invalid cancel: %+v %v",final,err)
 }
}
