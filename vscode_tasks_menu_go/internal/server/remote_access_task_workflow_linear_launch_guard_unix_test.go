//go:build linux || darwin

package server

import(
 "context"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

// A workflow step audit is a host-local storage boundary. Inject a late
// executable symlink retarget exactly while the attempted row is written.
// The mutation applies only to the first process, never a client command.
type remoteLinearAuditRetarget struct{
 identity.Store
 link,target string
 mutated bool
}
func(h *remoteLinearAuditRetarget)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=h.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.tasks_workflow_step"||
  event.Result!="attempted"||h.mutated{return nil}
 h.mutated=true
 if err:=os.Remove(h.link);err!=nil{return err}
 return os.Symlink(h.target,h.link)
}

func TestRemoteLinearWorkflowAuditCannotRetargetExecutable(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 link:=filepath.Join(t.TempDir(),"native-step")
 if err:=os.Symlink("/bin/echo",link);err!=nil{t.Fatal(err)}
 source:=`{"tasks":[
  {"label":"First","type":"process","command":`+
  strconv.Quote(link)+`,"args":["FORBIDDEN-LINEAR"]},
  {"label":"Second","type":"process","command":"/bin/echo",
   "args":["ROOT-ALSO-FORBIDDEN"],"dependsOn":"First"}]}`
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Second")
 h:=&remoteLinearAuditRetarget{
  Store:s.Identity,link:link,target:"/bin/touch",
 }
 s.Identity=h
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err!=nil{
  t.Fatal(err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="failed"{
  t.Fatalf("audit-time executable mutation still ran: %s",got)
 }
 if !h.mutated{t.Fatal("fixture never retargeted during attempted audit")}
 if _,err:=os.Stat(filepath.Join(s.Workspace,"FORBIDDEN-LINEAR"));!os.IsNotExist(err){
  t.Fatalf("post-audit replacement executable created a file: %v",err)
 }
 capture,_,_,_:=session.longTask.capture.next(0)
 if strings.Contains(capture,"ROOT-ALSO-FORBIDDEN"){
  t.Fatal("workflow root ran after first step's prelaunch denial")
 }
}

func TestRemoteLinearWorkflowFinalGuardRejectsEnvironmentAndCwdDrift(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 for _,dir:=range []string{"original","changed"}{
  if err:=os.Mkdir(filepath.Join(s.Workspace,dir),0700);err!=nil{t.Fatal(err)}
 }
 link:=filepath.Join(s.Workspace,"working")
 if err:=os.Symlink("original",link);err!=nil{t.Fatal(err)}
 source:=`{"tasks":[
  {"label":"A","type":"process","command":"/bin/echo",
   "args":["SAFE"],"options":{"cwd":"working"}},
  {"label":"B","type":"process","command":"/bin/echo",
   "args":["SAFE"],"dependsOn":"A"}]}`
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"B")
 plan,hash,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,id)
 if err!=nil||len(plan)!=2{t.Fatalf("invalid workflow: %+v %v",plan,err)}
 step:=plan[0]
 spec,_,err:=prepareRemoteTaskExecutionInputMode(
  context.Background(),s.Workspace,hash,step.taskID,0,false,"",true)
 if err!=nil{t.Fatal(err)}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{t.Fatal(err)}
 root,err=filepath.Abs(root)
 if err!=nil{t.Fatal(err)}
 cwd,err:=filepath.EvalSymlinks(spec.Cwd)
 if err!=nil{t.Fatal(err)}
 cwd,err=filepath.Abs(cwd)
 if err!=nil{t.Fatal(err)}
 exe,ok:=remoteWorkflowResolvedExecutable(spec.Command)
 if !ok{t.Fatal("safe native executable not recognized")}
 approved:=remoteParallelPrepared{
  execution:spec,executable:exe,resolvedCwd:cwd,
 }
 if _,err:=s.revalidateRemoteLinearStepAtLaunch(
  context.Background(),session,step,hash,1,approved,root);err!=nil{
  t.Fatalf("unchanged step was denied: %v",err)
 }
 if err:=os.Remove(link);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("changed",link);err!=nil{t.Fatal(err)}
 if _,err:=s.revalidateRemoteLinearStepAtLaunch(
  context.Background(),session,step,hash,1,approved,root);err==nil{
  t.Fatal("post-audit cwd alias changed without denial")
 }
 if err:=os.Remove(link);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("original",link);err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_LINEAR_GUARD_ENV","changed")
 if _,err:=s.revalidateRemoteLinearStepAtLaunch(
  context.Background(),session,step,hash,1,approved,root);err==nil{
  t.Fatal("post-audit inherited environment drift was accepted")
 }
}
