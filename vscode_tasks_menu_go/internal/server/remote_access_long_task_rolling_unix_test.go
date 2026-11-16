//go:build linux || darwin

package server

import (
 "context"
 "os"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteTaskRollingOutputHostOptInAndBackwardCompatibility(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_ROLLING_OUTPUT","0")
 first:=newRemoteTaskOutputCapture()
 if first.rolling{t.Fatal("host opt-out unexpectedly enabled rolling task output")}
 payload:=strings.Repeat("A",remoteLongTaskMaxCaptureBytes)+
  strings.Repeat("B",4096)+"LATE-ERROR"
 if _,err:=first.Write([]byte(payload));err!=nil{t.Fatal(err)}
 first.finishTaskRollingOutput()
 first.mu.Lock()
 original:=string(first.data)
 first.mu.Unlock()
 if strings.Contains(original,"LATE-ERROR")||len(original)!=remoteLongTaskMaxCaptureBytes{
  t.Fatal("opted-out Task did not retain its original first-window semantics")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_ROLLING_OUTPUT","1")
 rolling:=newRemoteTaskOutputCapture()
 if !rolling.rolling{t.Fatal("host opt-in failed to enable bounded rolling output")}
 if _,err:=rolling.Write([]byte(payload));err!=nil{t.Fatal(err)}
 rolling.finishTaskRollingOutput()
 rolling.mu.Lock()
 kept:=string(rolling.data)
 base:=rolling.rollingBase
 rolling.mu.Unlock()
 if len(kept)>remoteLongTaskMaxCaptureBytes||base==0||
  !strings.HasSuffix(kept,"LATE-ERROR")||
  strings.HasPrefix(kept,strings.Repeat("A",128)){
  t.Fatalf("rolling task did not retain the bounded late output: base=%d size=%d",base,len(kept))
 }
 page,cursor,lost,more:=rolling.next(0)
 if !lost||len(page)==0||cursor<=0||!more{
  t.Fatalf("rolling Poll did not acknowledge dropped pre-tail bytes: cursor=%d lost=%t more=%t",cursor,lost,more)
 }
 if _,err:=rolling.Write([]byte("SHOULD-NOT-APPEND"));err!=nil{t.Fatal(err)}
 rolling.mu.Lock()
 same:=string(rolling.data)==kept
 rolling.mu.Unlock()
 if !same{t.Fatal("completed Task accepted a late rolling Write")}
}

func TestRemoteTaskRollingOutputMaintainsMonotonicPollCursor(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_ROLLING_OUTPUT","1")
 capture:=newRemoteTaskOutputCapture()
 job:=&remoteLongTaskJob{
  taskID:81,status:"running",done:make(chan struct{}),capture:capture,
 }
 session:=&remoteTaskDeckSession{longTask:job}
 cursorBefore:=0
 for i:=0;i<7;i++{
  payload:=strings.Repeat(string(rune('a'+i)),5000)
  if _,err:=capture.Write([]byte(payload));err!=nil{t.Fatal(err)}
  var restored strings.Builder
  for n:=0;n<45;n++{
   snapshot,err:=session.pollRemoteLongTask(context.Background(),81)
   if err!=nil{t.Fatal(err)}
   if snapshot.Truncated{
    t.Fatalf("rolling output lost unread bytes while polling continuously: step=%d",i)
   }
   restored.WriteString(snapshot.Output)
   if !snapshot.More{break}
   if n==44{t.Fatal("rolling output did not finish bounded pages")}
  }
  if restored.String()!=payload{
   t.Fatalf("rolling chunk %d was lost or duplicated: %d vs %d",i,restored.Len(),len(payload))
  }
  job.mu.Lock()
  current:=job.outputCursor
  job.mu.Unlock()
  if current<=cursorBefore{t.Fatalf("rolling output cursor rewound: %d -> %d",cursorBefore,current)}
  cursorBefore=current
 }
 if cursorBefore!=7*5000{t.Fatalf("unexpected final cursor %d",cursorBefore)}
}

func TestRemoteLongTaskRollingOutputDeliversFinalBuildError(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_ROLLING_OUTPUT","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 // Only a native host-owned process definition; no remote selected shell
 // commands or output payload are accepted by the E2EE client.
 id:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Build with late error","type":"process",
   "command":"/bin/sh","args":["-c",
   "head -c 35000 /dev/zero | tr '\\\\0' X; printf 'LATE-BUILD-ERROR'"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("approved long task did not start: %v",err)
 }
 select{
 case <-session.longTask.done:
 case <-time.After(4*time.Second):t.Fatal("long build did not complete")
 }
 capture:=session.longTask.capture
 capture.mu.Lock()
 got:=string(capture.data)
 bounded:=len(capture.data)<=remoteLongTaskMaxCaptureBytes
 capture.mu.Unlock()
 if !bounded||!strings.HasSuffix(got,"LATE-BUILD-ERROR"){
  t.Fatalf("final build diagnostics not retained at end of bounded job: %d bytes",len(got))
 }
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
 _=remotetransport.ErrRemoteActionDenied
}
