//go:build linux || darwin

package server

import(
 "context"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// VS Code process argument objects must remain a single literal argv item;
// the native runner must never re-interpret the quoting mode as a shell.
func TestRemoteWorkflowProcessArgObjects(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Quoted argv","type":"process","command":"/usr/bin/printf",
   "args":["<%s>|<%s>|<%s>",
    {"value":"left right","quoting":"strong"},
    {"value":"say \"hello\"","quoting":"weak"},
    {"value":"prefix-\u0024{workspaceFolderBasename}-suffix","quoting":"escape"}]},
  {"label":"Finish","type":"process","command":"/bin/echo",
   "args":["ARG-OBJECTS-OK"],"dependsOn":"Quoted argv"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Finish")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("VS Code argv object workflow denied: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("quoted argv workflow failed: %s",got)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 wanted:=`<left right>|<say "hello">|<prefix-`+filepath.Base(s.Workspace)+`-suffix>`
 if !strings.Contains(output,wanted)||!strings.Contains(output,"ARG-OBJECTS-OK"){
  t.Fatalf("object quoting changed native argv: expected %q in %q",wanted,output)
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteParallelWorkflowProcessArgObjects(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Left","type":"process","command":"/usr/bin/printf",
   "args":["L:%s",{"value":"left with spaces","quoting":"strong"}]},
  {"label":"Right","type":"process","command":"/usr/bin/printf",
   "args":["R:%s",{"value":"\u0024{workspaceFolder}","quoting":"escape"}]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["PARALLEL-ARGS-OK"],"dependsOn":["Left","Right"],
   "dependsOrder":"parallel"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("parallel object-argv workflow denied: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("parallel argv workflow failed: %s",got)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 for _,wanted:=range []string{
  "L:left with spaces","R:"+filepath.Clean(s.Workspace),"PARALLEL-ARGS-OK",
 }{
  if !strings.Contains(output,wanted){
   t.Fatalf("parallel object argv lost %q in %q",wanted,output)
  }
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteWorkflowProcessArgObjectPolicyRejectsUnsafeShapes(t *testing.T){
 good:=[]any{
  "ordinary text",
  map[string]any{"value":"one two"},
  map[string]any{"value":"prefix-\u0024{workspaceFolderBasename}","quoting":"strong"},
  map[string]any{"value":"quote ' here","quoting":"weak"},
  map[string]any{"value":"backslash \\ here","quoting":"escape"},
 }
 for _,arg:=range good{
  if !remoteWorkflowSafeProcessArg(arg){
   t.Fatalf("valid native argv object rejected: %#v",arg)
  }
 }
 bad:=[]any{
  map[string]any{"value":""},
  map[string]any{"value":123},
  map[string]any{"value":"ok","quoting":"shell"},
  map[string]any{"value":"ok","quoting":true},
  map[string]any{"value":"ok","quoting":"strong","env":"evil"},
  map[string]any{"value":"ok","unexpected":"ignored"},
  map[string]any{"value":"ok","shell":false},
  map[string]any{"quoting":"strong"},
  map[string]any{"value":"\u0024{env:HOME}","quoting":"strong"},
  map[string]any{"value":"\u0024{input:token}","quoting":"strong"},
  map[string]any{"value":"\u0024{command:thing}"},
  map[string]any{"value":strings.Repeat("Z",257)},
  map[string]any{"value":"a\nb"},
  []any{"nested"},
  12,
 }
 for _,arg:=range bad{
  if remoteWorkflowSafeProcessArg(arg){
   t.Fatalf("unsafe native argv object allowed: %#v",arg)
  }
 }
 task:=tasks.Task{Type:"process",Command:"/usr/bin/printf",
  Args:[]any{"%s",map[string]any{"value":"x y","quoting":"strong"}}}
 if !remoteWorkflowSafeProcess(task){
  t.Fatal("quoted argument object denied by workflow process policy")
 }
 for _,arg:=range bad{
  task.Args=[]any{"%s",arg}
  if remoteWorkflowSafeProcess(task){
   t.Fatalf("unsafe argv reached workflow policy: %#v",arg)
  }
 }
}

func TestRemoteWorkflowProcessArgObjectGraphDenial(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Bad","type":"process","command":"/bin/echo",
   "args":[{"value":"hello","quoting":"unsupported"}]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["SHOULD-NOT-RUN"],"dependsOn":"Bad"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("unsupported quoting mode permitted workflow launch")
 }
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 if job!=nil{t.Fatal("invalid object allocated background workflow job")}
}
