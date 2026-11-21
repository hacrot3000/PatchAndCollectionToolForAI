package server

import (
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestAgentGoSuiteEndToEndRequiresPredeclaredTestChecklist(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("native Go PTY fixture requires POSIX")}
 if _,err:=exec.LookPath("go");err!=nil{t.Skip("Go is unavailable")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan"
 initial:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"create","expected_revision":x.Revision,
  "goal":"Verify all user-required native Go tests",
  "steps":[]map[string]string{{"id":"tests","title":"Required test suite","acceptance":"TestFirst and TestSecond passed in one run"}},
 })
 if initial.Code!=http.StatusOK{t.Fatalf("create plan: %d %s",initial.Code,initial.Body)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"go.mod"),[]byte("module example.com/taskproofsuite\n\ngo 1.21\n"),0600);err!=nil{t.Fatal(err)}
 content:="package taskproofsuite\nimport \"testing\"\nfunc TestFirst(t *testing.T){t.Log(\"first ran\")}\nfunc TestSecond(t *testing.T){t.Log(\"second ran\")}\n"
 if err:=os.WriteFile(filepath.Join(s.Workspace,"suite_test.go"),[]byte(content),0600);err!=nil{t.Fatal(err)}
 manager:=session.NewManager(2<<20);s.Sessions=manager
 defer manager.Shutdown(time.Second)
 launch:=func()session.Metadata{
  t.Helper()
  started,err:=manager.Start(tasks.Execution{TaskID:55,Label:"Go suite",
   Command:"go",Args:[]string{"test","-json","-count=1","./..."},
   Cwd:s.Workspace,Env:os.Environ(),Preview:"go test -json -count=1 ./..."})
  if err!=nil{t.Fatal(err)}
  until:=time.Now().Add(60*time.Second)
  for time.Now().Before(until){
   m,ok:=manager.Metadata(started.ID)
   if ok&&m.Status!="running"{
    if m.Status!="exited"||m.ExitCode==nil||*m.ExitCode!=0||m.TestRunner!="go_test_json"{
     t.Fatalf("real go test suite failed: %+v",m)
    }
    return m
   }
   time.Sleep(25*time.Millisecond)
  }
  t.Fatal("real native go test -json Task exceeded fixture deadline")
  return session.Metadata{}
 }
 apply:=func(action map[string]any)int{
  t.Helper()
  current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
  action["expected_revision"]=current.Revision
  out:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,action)
  return out.Code
 }
 requirements:=func(tests ...string)map[string]any{
  return map[string]any{
   "action":"set_go_test_requirements","step_id":"tests",
   "enabled":true,"confirm_go_tests":true,"evidence_go_package":"example.com/taskproofsuite",
   "required_go_tests":tests,
  }
 }
 proof:=func(ref string,kind string,goTests []string)int{
  item:=map[string]any{
   "action":"set_step","step_id":"tests","status":"done",
   "evidence_kind":kind,"evidence_task_id":55,"evidence_ref":ref,
  }
  if goTests!=nil{item["required_go_tests"]=goTests}
  return apply(item)
 }
 if got:=apply(requirements("TestFirst","TestSecond"));got!=http.StatusConflict{
  t.Fatalf("unbound Go test requirements accepted without native command lock: %d",got)
 }
 baseline:=launch()
 if got:=apply(map[string]any{
  "action":"set_task_command_guard","step_id":"tests","enabled":true,
  "confirm_command_guard":true,"evidence_task_id":55,
  "expected_command_sha256":baseline.CommandSHA256,
 });got!=http.StatusOK{t.Fatalf("owner command lock failed: %d",got)}
 if got:=apply(map[string]any{
  "action":"set_go_test_requirements","step_id":"tests","enabled":true,
  "evidence_go_package":"example.com/taskproofsuite","required_go_tests":[]string{"TestFirst","TestSecond"},
 });got!=http.StatusForbidden{t.Fatalf("Go requirements without confirmation accepted: %d",got)}
 if got:=apply(requirements("TestFirst","TestFirst"));got!=http.StatusBadRequest{
  t.Fatalf("duplicated required tests accepted: %d",got)
 }
 if got:=apply(requirements("TestFirst","TestMissing"));got!=http.StatusOK{
  t.Fatalf("could not lock test requirements: %d",got)
 }
 if got:=apply(map[string]any{
  "action":"set_task_command_guard","step_id":"tests","enabled":false,"confirm_command_guard":true,
 });got!=http.StatusConflict{t.Fatalf("command guard was changed under locked requirements: %d",got)}
 if got:=proof(baseline.ID,"task_run",nil);got!=http.StatusConflict{
  t.Fatalf("weak exit-zero evidence bypassed predeclared checklist: %d",got)
 }
 if got:=proof(baseline.ID,"go_test_suite",nil);got!=http.StatusConflict{
  t.Fatalf("prebinding native Task bypassed test requirements: %d",got)
 }
 time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second))+150*time.Millisecond)
 runMissing:=launch()
 if got:=proof(runMissing.ID,"go_test_suite",nil);got!=http.StatusConflict{
  t.Fatalf("missing predeclared TestMissing accepted: %d",got)
 }
 if got:=apply(requirements("TestFirst","TestSecond"));got!=http.StatusOK{
  t.Fatalf("corrected owner-required test list was not persisted: %d",got)
 }
 if got:=proof(runMissing.ID,"go_test_suite",nil);got!=http.StatusConflict{
  t.Fatalf("previous Task run accepted after checklist was updated: %d",got)
 }
 if got:=proof(runMissing.ID,"go_test_case",nil);got!=http.StatusConflict{
  t.Fatalf("opportunistic single test proof bypassed required checklist: %d",got)
 }
 time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second))+150*time.Millisecond)
 verified:=launch()
 if got:=proof(verified.ID,"go_test_suite",[]string{"TestFirst"});got!=http.StatusBadRequest{
  t.Fatalf("user-supplied substitute test list accepted at evidence time: %d",got)
 }
 if got:=proof(verified.ID,"go_test_suite",nil);got!=http.StatusOK{
  t.Fatalf("complete native required Go test suite was rejected: %d",got)
 }
 current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 step:=current.TaskPlan.Steps[0]
 if current.TaskPlan.State=="completed"||step.Status!="done"||len(step.Evidence)!=1||
  step.Evidence[0].Kind!="go_test_suite"||step.Evidence[0].Ref!=verified.ID||len(step.Evidence[0].Digest)!=64{
  t.Fatalf("accepted suite was not persisted without whole-plan completion: %+v",current.TaskPlan)
 }
 if got:=apply(map[string]any{
  "action":"set_go_test_requirements","step_id":"tests","enabled":false,"confirm_go_tests":true,
 });got!=http.StatusConflict{t.Fatalf("accepted checklist was changed without reopening: %d",got)}
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 persisted,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if persisted.TaskPlan.Steps[0].GoTestRequirements==nil||
  len(persisted.TaskPlan.Steps[0].GoTestRequirements.Tests)!=2||
  persisted.TaskPlan.Steps[0].Evidence[0].Digest!=step.Evidence[0].Digest{
  t.Fatal("bound required tests or accepted native proof were lost after restart")
 }
 handoff,_,err:=reopened.Handoff(id,"local")
 if err!=nil||!strings.Contains(handoff,"Owner-bound required Go tests")||
  !strings.Contains(handoff,"TestFirst, TestSecond"){
  t.Fatalf("durable per-revision handoff omitted required Go tests: %v %s",err,handoff)
 }
}
