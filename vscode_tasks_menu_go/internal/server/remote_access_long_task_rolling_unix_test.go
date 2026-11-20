//go:build linux || darwin

package server

import (
 "context"
 "os"
 "strings"
 "testing"
 "time"

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
  !strings.HasSuffix(kept,"LATE-ERROR"){
  t.Fatalf("rolling task did not retain the bounded late output: base=%d size=%d",base,len(kept))
 }
 page,cursor,lost,more:=rolling.next(0)
 if !lost||len(page)==0||cursor<=base||!more||
  strings.Contains(page,"A"){
  t.Fatalf("rolling Poll did not skip dropped pre-tail bytes: cursor=%d lost=%t more=%t",cursor,lost,more)
 }
 // The bounded 512-byte late excerpt must include the final marker
 // within at most four signed 128-byte Polls after a long idle gap.
 text:=page
 for i:=0;i<5&&more;i++{
  chunk,next,_,nextMore:=rolling.next(cursor)
  if next<=cursor{t.Fatal("rolling task tail cursor stalled")}
  text+=chunk
  cursor,more=next,nextMore
 }
 if !strings.Contains(text,"LATE-ERROR"){
  t.Fatal("late task error could not be retrieved within GUI Poll quota")
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
   "yes X | head -c 35000; printf 'LATE-BUILD-ERROR'"]}]}`)
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
 var polled strings.Builder
 for i:=0;i<5;i++{
  snapshot,err:=session.pollRemoteLongTask(context.Background(),id)
  if err!=nil{t.Fatalf("long task Poll failed: %v",err)}
  if i==0&&!snapshot.Truncated{t.Fatal("overflow was not reported")}
  polled.WriteString(snapshot.Output)
  if !snapshot.More{break}
 }
 if !strings.Contains(polled.String(),"LATE-BUILD-ERROR"){
  t.Fatal("signed-poll-sized late build error remained unreachable")
 }
 if capture.rollingBase==0{t.Fatal("build log never exercised rolling overflow")}
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
}

 
// The workflow captures stdout from multiple independently audited native
// steps. The final step's result must survive a verbose earlier step when
// the Agent host explicitly chooses rolling output.
func TestRemoteWorkflowRollingOutputKeepsFinalStep(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_ROLLING_OUTPUT","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 if err:=os.WriteFile(s.Workspace+"/large-task-output.txt",
  []byte(strings.Repeat("B",35000)),0600);err!=nil{t.Fatal(err)}
 source:=`{"tasks":[
  {"label":"Long log","type":"process","command":"/bin/cat",
   "args":["large-task-output.txt"]},
  {"label":"Final result","type":"process","command":"/bin/echo",
   "args":["LATE-WORKFLOW-SUCCESS"],"dependsOn":"Long log"}]}`
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Final result")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err!=nil{
  t.Fatalf("authorized workflow rejected: %v",err)
 }
 if result:=awaitRemoteWorkflow(t,session);result!="completed"{
  t.Fatalf("rolling workflow did not finish: %s",result)
 }
 capture:=session.longTask.capture
 capture.mu.Lock()
 output:=string(append([]byte(nil),capture.data...))
 base:=capture.rollingBase
 capture.mu.Unlock()
 if !capture.rolling||base==0||len(output)>remoteLongTaskMaxCaptureBytes||
  !strings.Contains(output,"LATE-WORKFLOW-SUCCESS"){
  t.Fatalf("final workflow result lost after large prior step: base=%d size=%d",base,len(output))
 }
 var received strings.Builder
 for i:=0;i<9;i++{
  snapshot,err:=session.pollRemoteLongTask(context.Background(),id)
  if err!=nil{t.Fatalf("workflow Poll failed: %v",err)}
  if i==0&&!snapshot.Truncated{t.Fatal("evicted workflow prefix was not reported")}
  received.WriteString(snapshot.Output)
  if !snapshot.More{break}
 }
 if !strings.Contains(received.String(),"LATE-WORKFLOW-SUCCESS"){
  t.Fatal("workflow final output remained behind the 16-action Poll quota")
 }
 if err:=session.clearRemoteLongTask(id);err!=nil{t.Fatal(err)}
}
