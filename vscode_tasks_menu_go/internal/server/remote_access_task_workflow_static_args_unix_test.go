//go:build linux || darwin

package server

import(
 "context"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// The client still sends only the host-issued Task.ID, but a VS Code
// process task may refer to stable workspace values inside its argv.
func TestRemoteWorkflowStaticWorkspaceArgsSequential(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Workspace argument","type":"process","command":"/usr/bin/printf",
   "args":["WS:%s;NAME:%s","\u0024{workspaceFolder}","\u0024{workspaceFolderBasename}"]},
  {"label":"Finish","type":"process","command":"/bin/echo",
   "args":["STATIC-ARGV-FINISHED"],"dependsOn":"Workspace argument"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Finish")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("safe workspace argv was rejected: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("sequential workflow did not finish: %s",got)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 expected:="WS:"+s.Workspace+";NAME:"+filepath.Base(s.Workspace)
 if !strings.Contains(output,expected)||
  !strings.Contains(output,"STATIC-ARGV-FINISHED"){
  t.Fatalf("literal workspace args were not expanded by host: %q",output)
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteParallelWorkflowStaticWorkspaceArgs(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"A","type":"process","command":"/usr/bin/printf",
   "args":["A:%s","\u0024{workspaceFolder}"]},
  {"label":"B","type":"process","command":"/usr/bin/printf",
   "args":["B:%s","prefix-\u0024{workspaceFolderBasename}-suffix"]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["STATIC-JOIN-OK"],"dependsOn":["A","B"],"dependsOrder":"parallel"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("parallel stable argv were denied: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("parallel workspace argv did not finish: %s",got)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 for _,wanted:=range []string{"A:"+s.Workspace,
  "B:prefix-"+filepath.Base(s.Workspace)+"-suffix","STATIC-JOIN-OK"}{
  if !strings.Contains(output,wanted){
   t.Fatalf("parallel task lost host-expanded argv %q: %q",wanted,output)
  }
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteWorkflowStaticWorkspaceArgsPolicyRejectsDynamicVariables(t *testing.T){
 for _,raw:=range []string{
  "${env:HOME}","${input:choice}","${file}",
  "${workspaceFolder:other}","${cwd}","${command:foo}",
  "${workspaceFolderBasenameX}",
 }{
  if remoteWorkflowSafeStaticArg(raw){
   t.Fatalf("dynamic/non-workspace argument allowed: %q",raw)
  }
 }
 for _,raw:=range []string{
  "--project=${workspaceFolder}",
  "${workspaceFolderBasename}/some dir",
  "name-${workspaceFolderBasename}-${workspaceFolder}",
  "literal-no-variable",
 }{
  if !remoteWorkflowSafeStaticArg(raw){
   t.Fatalf("stable workspace argument denied: %q",raw)
  }
 }
 if remoteWorkflowSafeStaticArg(strings.Repeat("A",257))||
  remoteWorkflowSafeStaticArg("bad\ninput"){
  t.Fatal("unbounded/control argument admitted")
 }
}

// Ensure the policy rejects non-workspace variable substitutions at the
// graph admission boundary, not only after a child was already started.
func TestRemoteWorkflowDynamicArgNeverCreatesHostJob(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Unsupported","type":"process","command":"/bin/echo",
   "args":["\u0024{env:HOME}"]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["MUST-NOT-EXECUTE"],"dependsOn":"Unsupported"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("dynamic variable reached executable workflow")
 }
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 if job!=nil{t.Fatal("dynamic variable allocated a remote task job")}
}

func TestRemoteWorkflowStableArgValidationAppliedToTaskObject(t *testing.T){
 task:=tasks.Task{Type:"process",Command:"/usr/bin/printf",
  Args:[]any{"%s","${workspaceFolder}"}}
 if !remoteWorkflowSafeProcess(task){
  t.Fatal("workspace constant arg was denied by process policy")
 }
 task.Args=[]any{"%s","${env:HOME}"}
 if remoteWorkflowSafeProcess(task){
  t.Fatal("task process policy admitted env interpolation")
 }
}
