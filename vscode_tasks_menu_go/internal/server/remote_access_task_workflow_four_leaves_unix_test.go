//go:build linux || darwin

package server

import (
 "context"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A four-worker fork is still host-authored: only a previously published
// E2EE root Task.ID reaches the native executor. No nested/retry/conditional
// process is admitted, and the existing 90-second overall lease is retained.
func TestRemoteWorkflowFourParallelLeavesSingleJoin(t *testing.T){
 remoteWorkflowHostFlags(t)
 const source=`{"tasks":[
  {"label":"Leaf A","type":"process","command":"/bin/sleep","args":["2"]},
  {"label":"Leaf B","type":"process","command":"/bin/sleep","args":["2"]},
  {"label":"Leaf C","type":"process","command":"/bin/sleep","args":["2"]},
  {"label":"Leaf D","type":"process","command":"/bin/sleep","args":["2"]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["FOUR-LEAVES-JOINED"],
   "dependsOn":["Leaf A","Leaf B","Leaf C","Leaf D"],
   "dependsOrder":"parallel"}]}`
 t.Run("separate host opt-in required",func(t *testing.T){
  t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","0")
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
  deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!deps.Available||deps.RunAvailable{
   t.Fatalf("unapproved four-worker workflow advertised as executable: %+v %v",deps,err)
  }
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
   t.Fatal("unapproved four-worker workflow started")
  }
 })
 t.Run("four workers overlap and root waits for all",func(t *testing.T){
  t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
  deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!deps.RunAvailable||len(deps.Order)!=5||len(deps.Edges)!=4{
   t.Fatalf("host did not approve the exact four-leaf fork: %+v %v",deps,err)
  }
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
   t.Fatalf("host-authorized four-worker workflow failed: %v",err)
  }
  job:=session.longTask
  overlapped:=false
  deadline:=time.Now().Add(1500*time.Millisecond)
  for time.Now().Before(deadline){
   job.mu.Lock()
   history,parallel:=job.workflowStepHistory,job.workflowParallel
   job.mu.Unlock()
   if parallel&&history=="RRRR-"{overlapped=true;break}
   time.Sleep(10*time.Millisecond)
  }
  if !overlapped{t.Fatal("four workers did not overlap before the root")}
  if got:=awaitRemoteWorkflow(t,session);got!="completed"{
   t.Fatalf("four-way workflow did not complete: %q",got)
  }
  job.mu.Lock()
  history,code:=job.workflowStepHistory,job.exitCode
  job.mu.Unlock()
  if history!="CCCCC"||code!=0{
   t.Fatalf("five-step parallel workflow lost outcomes: %q exit=%d",history,code)
  }
  job.capture.mu.Lock()
  transcript:=string(append([]byte(nil),job.capture.data...))
  job.capture.mu.Unlock()
  if !strings.Contains(transcript,"FOUR-LEAVES-JOINED")||
   strings.Count(transcript,"parallel attempt 1/1")!=5{
   t.Fatalf("root or exactly-once parallel children missing: %q",transcript)
  }
  progress,err:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,root,0,"",
   "34343434343434343434343434343434")
  if err!=nil||!progress.Parallel||progress.StepTotal!=5||
   progress.Step!=5||progress.StepHistory!="CCCCC"||len(progress.Output)>16{
   t.Fatalf("five-step parallel E2EE result exceeded bounds: %+v %v",progress,err)
  }
 })
}
