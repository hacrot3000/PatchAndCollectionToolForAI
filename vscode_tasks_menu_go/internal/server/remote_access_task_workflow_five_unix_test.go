//go:build linux || darwin

package server

import (
 "context"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A five-node sequence may include a multi-parent join ONLY when the
// workspace author explicitly serialized it. The client sends the single
// E2EE-issued final Task.ID; the host owns every executable and edge.
func TestRemoteWorkflowFiveStepSequentialJoinE2EE(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"A","type":"process","command":"/bin/echo","args":["FIVE-A"]},
  {"label":"B","type":"process","command":"/bin/echo","args":["FIVE-B"],"dependsOn":"A"},
  {"label":"C","type":"process","command":"/bin/echo","args":["FIVE-C"],"dependsOn":"B"},
  {"label":"D","type":"process","command":"/bin/echo","args":["FIVE-D"],"dependsOn":"C"},
  {"label":"E","type":"process","command":"/bin/echo","args":["FIVE-E"],
   "dependsOn":["B","D"],"dependsOrder":"sequence"}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"E")
 info,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!info.RunAvailable||len(info.Order)!=5||len(info.Edges)!=5{
  t.Fatalf("host did not expose explicitly sequential five-step DAG: %+v %v",info,err)
 }
 started,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksWorkflowStart,root,0,"",
  "22222222222222222222222222222222")
 if err!=nil||started.Status!="started"{
  t.Fatalf("five-step E2EE start was denied: %+v %v",started,err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("five-step native execution failed: %s",got)
 }
 job:=session.longTask
 job.capture.mu.Lock()
 complete:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 prior:=-1
 for _,mark:=range []string{"FIVE-A","FIVE-B","FIVE-C","FIVE-D","FIVE-E"}{
  at:=strings.Index(complete,mark)
  if at<=prior{t.Fatalf("step was missing or reordered (%q): %q",mark,complete)}
  prior=at
 }
 final,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,root,0,"",
  "33333333333333333333333333333333")
 if err!=nil||final.Status!="completed"||final.Step!=5||
  final.StepTotal!=5||final.StepHistory!="CCCCC"||final.Parallel{
  t.Fatalf("authenticated fifth-step progress lost: %+v %v",final,err)
 }
 if len(final.Output)>32{
  t.Fatalf("fifth-step progress exceeded bounded E2EE output: %d",len(final.Output))
 }
}

// This SAME helper is used by immediate and timed-out host Poll paths;
// previously timeout returned 64 bytes for a four-worker progress record
// which is only authorized to carry 48, causing intermittent AEAD failure.
func TestRemoteWorkflowChunkLimitSameAcrossPollPaths(t *testing.T){
 cases:=[]struct{step,total int;parallel bool;want int}{
  {0,0,false,remoteLongTaskMaxChunkBytes},
  {1,2,false,64},
  {3,4,false,64},
  {3,4,true,48},
  {4,4,true,48},
  {1,5,false,32},
  {5,5,false,32},
 }
 for _,tc:=range cases{
  if got:=remoteLongTaskWorkflowChunkLimit(
   tc.step,tc.total,tc.parallel);got!=tc.want{
   t.Fatalf("wrong wire bound for %+v: %d != %d",tc,got,tc.want)
  }
 }
}
