//go:build linux || darwin

package server

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"

)

// Support host-authored options.cwd without opening arbitrary commands,
// env overrides or user-selected directories over the E2EE transport.

// Read the full authenticated host capture rather than just the first
// 128-byte E2EE Poll page; workspace temp paths may be longer than that.
func remoteWorkflowCwdFullCapture(job *remoteLongTaskJob)string{
 job.capture.mu.Lock()
 defer job.capture.mu.Unlock()
 return string(append([]byte(nil),job.capture.data...))
}

func TestRemoteWorkflowWorkspaceRelativeCwdExecutes(t *testing.T){
 for _,mode:=range []string{"relative","workspace_variable"}{
  t.Run(mode,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   folder:="dir with spaces"
   if err:=os.Mkdir(filepath.Join(s.Workspace,folder),0700);err!=nil{
    t.Fatal(err)
   }
   cwd:=folder
   if mode=="workspace_variable"{cwd="${workspaceFolder}/"+folder}
   source:=`{"tasks":[
    {"label":"Cwd Step","type":"process","command":"/bin/pwd",
     "options":{"cwd":"`+cwd+`"}},
    {"label":"Root Step","type":"process","command":"/bin/echo",
     "args":["WORKFLOW-CWD-OK"],"dependsOn":"Cwd Step"}]}`
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Root Step")
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("workspace cwd was denied: %v",err)
   }
   if got:=awaitRemoteWorkflow(t,session);got!="completed"{
    t.Fatalf("bounded cwd workflow failed: %s",got)
   }
   capture:=remoteWorkflowCwdFullCapture(session.longTask)
   if !strings.Contains(capture,filepath.Join(s.Workspace,folder))||
    !strings.Contains(capture,"WORKFLOW-CWD-OK"){
    t.Fatalf("task did not execute inside the selected workspace folder: %q",capture)
   }
   if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
  })
 }
}

func TestRemoteWorkflowCwdPolicyRejectsEscapesAndNonCwdOptions(t *testing.T){
 for _,tc:=range []struct{name,options string}{
  {"absolute_path",`{"cwd":"/tmp"}`},
  {"parent_traversal",`{"cwd":"../outside"}`},
  {"indirect_traversal",`{"cwd":"inside/../outside"}`},
  {"workspace_escape",`{"cwd":"${workspaceFolder}/../outside"}`},
  {"environment_interpolation",`{"cwd":"${env:HOME}"}`},
  {"shell_option",`{"shell":{"executable":"/bin/sh"}}`},
  {"environment_option",`{"env":{"KEY":"value"}}`},
  {"mixed_options",`{"cwd":"build","env":{"KEY":"value"}}`},
  {"wrong_cwd_type",`{"cwd":42}`},
  {"regular_file_cwd",`{"cwd":"not-a-directory"}`},
 }{
  t.Run(tc.name,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   if err:=os.WriteFile(filepath.Join(s.Workspace,"not-a-directory"),
    []byte("regular file"),0600);err!=nil{t.Fatal(err)}
   source:=`{"tasks":[
    {"label":"First","type":"process","command":"/bin/echo",
     "args":["SHOULD-NOT-EXECUTE"],"options":`+tc.options+`},
    {"label":"Root","type":"process","command":"/bin/echo",
     "args":["ROOT-SHOULD-NOT-EXECUTE"],"dependsOn":"First"}]}`
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Root")
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);
    err==nil{
    t.Fatal("unsafe options.cwd or non-cwd workflow option was admitted")
   }
   session.longTaskMu.Lock()
   job:=session.longTask
   session.longTaskMu.Unlock()
   if job!=nil{t.Fatal("denied workflow allocated host job")}
  })
 }
}

func TestRemoteWorkflowCwdSymlinkRetargetDuringStepAuditDoesNotExecute(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 for _,dir:=range []string{"original","retargeted"}{
  if err:=os.Mkdir(filepath.Join(s.Workspace,dir),0700);err!=nil{
   t.Fatal(err)
  }
 }
 link:=filepath.Join(s.Workspace,"working")
 if err:=os.Symlink("original",link);err!=nil{t.Fatal(err)}
 source:=`{"tasks":[
  {"label":"Create","type":"process","command":"/bin/touch",
   "args":["DO-NOT-CREATE"],"options":{"cwd":"working"}},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["DO-NOT-START-ROOT"],"dependsOn":"Create"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 hook:=&remoteLinearAuditRetarget{
  Store:s.Identity,link:link,target:"retargeted",
 }
 s.Identity=hook
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="failed"{
  t.Fatalf("retargeted cwd workflow was not rejected: %s",got)
 }
 if !hook.mutated{t.Fatal("cwd retarget did not happen during attempted audit")}
 for _,dir:=range []string{"original","retargeted"}{
  if _,err:=os.Stat(filepath.Join(s.Workspace,dir,"DO-NOT-CREATE"));
   !os.IsNotExist(err){t.Fatalf("stale cwd task had side effect: %v",err)}
 }
 capture:=remoteWorkflowCwdFullCapture(session.longTask)
 if strings.Contains(capture,"DO-NOT-START-ROOT"){
  t.Fatal("workflow root started after cwd mutation")
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteParallelWorkflowWorkspaceRelativeCwdExecutes(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 for _,dir:=range []string{"leaf-left","leaf-right"}{
  if err:=os.Mkdir(filepath.Join(s.Workspace,dir),0700);err!=nil{
   t.Fatal(err)
  }
 }
 source:=`{"tasks":[
  {"label":"Leaf Left","type":"process","command":"/bin/pwd",
   "options":{"cwd":"leaf-left"}},
  {"label":"Leaf Right","type":"process","command":"/bin/pwd",
   "options":{"cwd":"leaf-right"}},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["CWD-JOIN-COMPLETE"],"dependsOn":["Leaf Left","Leaf Right"],
   "dependsOrder":"parallel"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("parallel workspace-relative cwd denied: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("parallel cwd failed: %s",got)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 for _,dir:=range []string{"leaf-left","leaf-right"}{
  if !strings.Contains(output,filepath.Join(s.Workspace,dir)){
   t.Fatalf("parallel worker cwd %s was lost: %q",dir,output)
  }
 }
 if !strings.Contains(output,"CWD-JOIN-COMPLETE"){
  t.Fatalf("parallel root did not run after both cwd leaves: %q",output)
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

 
// In a parallel join both attempted rows must be durable before ANY
// sibling launches. A host-local cwd symlink change during the first row
// must abort the entire fork, not just the affected leaf.
func TestRemoteParallelWorkflowCwdSymlinkAuditDriftStopsBothWorkers(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 for _,dir:=range []string{"original","retargeted"}{
  if err:=os.Mkdir(filepath.Join(s.Workspace,dir),0700);err!=nil{t.Fatal(err)}
 }
 link:=filepath.Join(s.Workspace,"working")
 if err:=os.Symlink("original",link);err!=nil{t.Fatal(err)}
 source:=`{"tasks":[
  {"label":"Leaf A","type":"process","command":"/bin/touch",
   "args":["UNSAFE-A"],"options":{"cwd":"working"}},
  {"label":"Leaf B","type":"process","command":"/bin/touch",
   "args":["UNSAFE-B"],"options":{"cwd":"original"}},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["UNSAFE-ROOT"],"dependsOn":["Leaf A","Leaf B"],
   "dependsOrder":"parallel"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 hook:=&remoteLinearAuditRetarget{
  Store:s.Identity,link:link,target:"retargeted",
 }
 s.Identity=hook
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="failed"{
  t.Fatalf("parallel fork accepted audit-time cwd retarget: %q",got)
 }
 if !hook.mutated{t.Fatal("audit hook did not change the cwd symlink")}
 for _,dir:=range []string{"original","retargeted"}{
  for _,name:=range []string{"UNSAFE-A","UNSAFE-B"}{
   if _,err:=os.Stat(filepath.Join(s.Workspace,dir,name));!os.IsNotExist(err){
    t.Fatalf("stale fork started an unapproved process: %s/%s %v",dir,name,err)
   }
  }
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 if strings.Contains(output,"UNSAFE-ROOT")||
  !strings.Contains(output,"Parallel prelaunch revalidation denied"){
  t.Fatalf("fork did not fail before worker launch: %q",output)
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}
