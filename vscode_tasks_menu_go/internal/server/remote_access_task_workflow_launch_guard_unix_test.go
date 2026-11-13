//go:build linux || darwin

package server

import (
 "context"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func remoteLaunchPreflightFixture(t *testing.T,leafCommand string,
 setup func(*Server))(*Server,*remoteTaskDeckSession,remoteWorkflowStep,
 remoteParallelPrepared,[32]byte){
 t.Helper()
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 if setup!=nil{setup(s)}
 source:=`{"tasks":[
  {"label":"Leaf A","type":"process","command":"/bin/echo",
   "args":["SAFE-A"]},
  {"label":"Leaf B","type":"process","command":"/bin/echo",
   "args":["SAFE-B"]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["SAFE-JOIN"],"dependsOn":["Leaf A","Leaf B"],
   "dependsOrder":"parallel"}]}`
 if leafCommand!=""{
  source=strings.Replace(source,`"command":"/bin/echo"`,
   fmt.Sprintf(`"command":%q`,leafCommand),1)
 }
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 plan,hash,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,id)
 if err!=nil||len(plan)!=3||!plan[2].parallelJoin{
  t.Fatalf("host did not admit safe flat fork: %+v %v",plan,err)
 }
 approved,err:=s.prepareRemoteParallelStep(context.Background(),session,plan[0],hash)
 if err!=nil{t.Fatalf("initial preflight denied: %v",err)}
 verified,err:=s.revalidateRemoteParallelAtLaunch(context.Background(),
  session,plan[0],approved,hash,0)
 if err!=nil||!remoteParallelSameExecution(approved,verified){
  t.Fatalf("stable safe task should be permitted: %+v %v",verified,err)
 }
 return s,session,plan[0],approved,hash
}

// The previous execution records canonical cwd. A malicious or stale
// prepared snapshot must not change this field while keeping the same
// Task ID, argv and lexical path.
func TestRemoteParallelLaunchRejectsResolvedCwdDrift(t *testing.T){
 s,session,step,original,hash:=remoteLaunchPreflightFixture(t,"",nil)
 poisoned:=original
 poisoned.resolvedCwd=filepath.Join(s.Workspace,"different")
 if remoteParallelSameExecution(poisoned,original){
  t.Fatal("canonical cwd changed without invalidating process identity")
 }
 if _,err:=s.revalidateRemoteParallelAtLaunch(context.Background(),
  session,step,poisoned,hash,0);err==nil{
  t.Fatal("stale resolved cwd bypassed last-moment launch guard")
 }
}

// A host-authored executable symlink outside the workspace is still
// mutable; re-check its canonical target at the actual launch boundary.
func TestRemoteParallelLaunchRejectsExecutableRetarget(t *testing.T){
 dir:=t.TempDir()
 link:=filepath.Join(dir,"native-binary")
 if err:=os.Symlink("/bin/echo",link);err!=nil{t.Fatal(err)}
 s,session,step,approved,hash:=remoteLaunchPreflightFixture(t,link,nil)
 if err:=os.Remove(link);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("/bin/cat",link);err!=nil{t.Fatal(err)}
 if _,err:=s.revalidateRemoteParallelAtLaunch(context.Background(),
  session,step,approved,hash,0);err==nil{
  t.Fatal("changed executable symlink bypassed worker pre-exec check")
 }
}

// The host can change inherited env without changing tasks.json or a
// stable 31-bit ID. This must be caught even after all attempted audits.
func TestRemoteParallelLaunchRejectsEnvironmentDrift(t *testing.T){
 t.Setenv("TASKDECK_LAUNCH_GUARD_SAMPLE","original")
 s,session,step,original,hash:=remoteLaunchPreflightFixture(t,"",nil)
 t.Setenv("TASKDECK_LAUNCH_GUARD_SAMPLE","modified")
 if _,err:=s.revalidateRemoteParallelAtLaunch(context.Background(),
  session,step,original,hash,0);err==nil{
  t.Fatal("changed inherited env bypassed final worker preflight")
 }
}

// The authority to start a fork is separate from the right to inspect
// its dependency graph. Once host opts out, even a prepared worker must
// never reach exec. A changed source hash must also be denied.
func TestRemoteParallelLaunchRejectsOptOutAndChangedSource(t *testing.T){
 s,session,step,original,hash:=remoteLaunchPreflightFixture(t,"",nil)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","0")
 if _,err:=s.revalidateRemoteParallelAtLaunch(context.Background(),
  session,step,original,hash,0);err==nil{
  t.Fatal("parallel opt-out still launched a prepared worker")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 f:=filepath.Join(s.Workspace,".vscode","tasks.json")
 data,err:=os.ReadFile(f)
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(f,append(data,' '),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.revalidateRemoteParallelAtLaunch(context.Background(),
  session,step,original,hash,0);err==nil{
  t.Fatal("tasks.json changed after attempt bypassed final worker preflight")
 }
}

