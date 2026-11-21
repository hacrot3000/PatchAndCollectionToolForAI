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

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestGoTestJSONEvidenceRejectsMissingOrMismatchedTestEvents(t *testing.T){
 pkg:="example.com/agentproof"
 test:="TestVerifiedPass"
 lines:=func(contents ...string)[]byte{return []byte(strings.Join(contents,"\n")+"\n")}
 run:=`{"Action":"run","Package":"example.com/agentproof","Test":"TestVerifiedPass"}`
 pass:=`{"Action":"pass","Package":"example.com/agentproof","Test":"TestVerifiedPass"}`
 packagePass:=`{"Action":"pass","Package":"example.com/agentproof"}`
 if err:=verifyNamedGoTestJSON(lines("Task header",run,pass,packagePass,"Task ended"),pkg,test);err!=nil{
  t.Fatalf("valid exact named test events rejected: %v",err)
 }
 invalid:=map[string][]byte{
  "no_run":lines(pass,packagePass),
  "no_test_pass":lines(run,packagePass),
  "no_package_pass":lines(run,pass),
  "skipped":lines(run,`{"Action":"skip","Package":"example.com/agentproof","Test":"TestVerifiedPass"}`,packagePass),
  "failed_package":lines(run,pass,`{"Action":"fail","Package":"example.com/agentproof"}`),
  "wrong_test":lines(`{"Action":"run","Package":"example.com/agentproof","Test":"TestOther"}`,
   `{"Action":"pass","Package":"example.com/agentproof","Test":"TestOther"}`,packagePass),
  "wrong_package":lines(`{"Action":"run","Package":"other/pkg","Test":"TestVerifiedPass"}`,
   `{"Action":"pass","Package":"other/pkg","Test":"TestVerifiedPass"}`,packagePass),
  "malformed_json":lines(run,pass,packagePass,"{not-json"),
  "duplicate_run":lines(run,run,pass,packagePass),
  "oversize":[]byte(strings.Repeat("x",maxNativeGoTestOutput+1)),
 }
 for name,input:=range invalid{
  t.Run(name,func(t *testing.T){
   if err:=verifyNamedGoTestJSON(input,pkg,test);err==nil{
    t.Fatal("unverified named Go test was accepted")
   }
  })
 }
}

func TestNativeGoTestJSONEvidenceRequiresBoundNewCompleteNativeRun(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("POSIX PTY-based Go test fixture")}
 if _,err:=exec.LookPath("go");err!=nil{t.Skip("Go executable unavailable")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 path:="/api/agent/sessions/"+id+"/plan"
 resp:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"create","expected_revision":x.Revision,
  "goal":"Confirm exact Go unit test execution from native output",
  "steps":[]map[string]string{{"id":"test","title":"Verify Go named test","acceptance":"TestVerifiedPass in example.com/agentproof"}},
 })
 if resp.Code!=http.StatusOK{t.Fatalf("create plan: %d %s",resp.Code,resp.Body)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"go.mod"),[]byte("module example.com/agentproof\n\ngo 1.21\n"),0600);err!=nil{t.Fatal(err)}
 code:=`package agentproof
import "testing"
func TestVerifiedPass(t *testing.T){t.Log("native test observed")}
func TestOther(t *testing.T){t.Log("another test")}
`
 if err:=os.WriteFile(filepath.Join(s.Workspace,"proof_test.go"),[]byte(code),0600);err!=nil{t.Fatal(err)}
 manager:=session.NewManager(2<<20)
 s.Sessions=manager
 defer manager.Shutdown(time.Second)
 launch:=func(m *session.Manager,cmd string,args ...string)session.Metadata{
  t.Helper()
  start,err:=m.Start(tasks.Execution{TaskID:23,Label:"Go test JSON",Command:cmd,
   Args:args,Cwd:s.Workspace,Env:os.Environ(),Preview:cmd+" "+strings.Join(args," ")})
  if err!=nil{t.Fatal(err)}
  deadline:=time.Now().Add(60*time.Second)
  for time.Now().Before(deadline){
   meta,ok:=m.Metadata(start.ID)
   if ok&&meta.Status!="running"{return meta}
   time.Sleep(25*time.Millisecond)
  }
  t.Fatal("native Go test Task did not terminate")
  return session.Metadata{}
 }
 args:=[]string{"test","-json","-count=1","./..."}
 baseline:=launch(manager,"go",args...)
 if baseline.TestRunner!="go_test_json"||baseline.ExitCode==nil||*baseline.ExitCode!=0{
  t.Fatalf("baseline real Go test did not succeed: %+v",baseline)
 }
 x,err=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 lock:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
  "action":"set_task_command_guard","expected_revision":x.Revision,"step_id":"test",
  "enabled":true,"confirm_command_guard":true,"evidence_task_id":23,
  "expected_command_sha256":baseline.CommandSHA256,
 })
 if lock.Code!=http.StatusOK{t.Fatalf("bind expected native command: %d %s",lock.Code,lock.Body)}
 try:=func(id string,expected int,pkg,name string)int{
  t.Helper()
  cur,err:=store.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
  result:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,map[string]any{
   "action":"set_step","expected_revision":cur.Revision,"step_id":"test","status":"done",
   "evidence_kind":"go_test_case","evidence_ref":id,"evidence_task_id":expected,
   "evidence_go_package":pkg,"evidence_go_test":name,
  })
  return result.Code
 }
 if code:=try(baseline.ID,23,"example.com/agentproof","TestVerifiedPass");code!=http.StatusConflict{
  t.Fatalf("pre-bind real test run passed named test proof: %d",code)
 }
 // A shell that prints forged Go JSON never qualifies as a native Go runner.
 shell:=launch(manager,"/bin/sh","-c",`printf '%s\\n' '{"Action":"run","Package":"example.com/agentproof","Test":"TestVerifiedPass"}' '{"Action":"pass","Package":"example.com/agentproof","Test":"TestVerifiedPass"}' '{"Action":"pass","Package":"example.com/agentproof"}'`)
 if code:=try(shell.ID,23,"example.com/agentproof","TestVerifiedPass");code!=http.StatusConflict{
  t.Fatalf("forged shell JSON passed named test proof: %d",code)
 }
 // Go test metadata has RFC3339 second resolution. New Task launch must
 // begin in a later second than the owner's confirmed guard binding.
 time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second))+150*time.Millisecond)
 verified:=launch(manager,"go",args...)
 if verified.OutputBytes<=0||verified.OutputBytes!=verified.RetainedBytes||
  verified.TestRunner!="go_test_json"{t.Fatalf("full native JSON output unavailable: %+v",verified)}
 if code:=try(verified.ID,23,"example.com/agentproof","TestMissing");code!=http.StatusConflict{
  t.Fatalf("missing test case passed: %d",code)
 }
 if code:=try(verified.ID,23,"other/pkg","TestVerifiedPass");code!=http.StatusConflict{
  t.Fatalf("wrong package passed: %d",code)
 }
 if code:=try(verified.ID,23,"example.com/agentproof","TestVerifiedPass");code!=http.StatusOK{
  t.Fatalf("real native named Go test proof refused: %d",code)
 }
 current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 evidence:=current.TaskPlan.Steps[0].Evidence
 if current.TaskPlan.State=="completed"||len(evidence)!=1||evidence[0].Kind!="go_test_case"||
  evidence[0].Ref!=verified.ID||len(evidence[0].Digest)!=64{
  t.Fatalf("named Go test proof not adopted durably: %+v",current.TaskPlan)
 }
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 old,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if len(old.TaskPlan.Steps[0].Evidence)!=1||old.TaskPlan.Steps[0].Evidence[0].Digest!=evidence[0].Digest{
  t.Fatal("structured native test result was lost after store reopen")
 }
 // Even an otherwise valid native test cannot be verified as a structured
 // result once its source output is evicted, unlike metadata-only exit proof.
 if err:=manager.Remove(verified.ID);err!=nil{t.Fatal(err)}
 if code:=try(verified.ID,23,"example.com/agentproof","TestVerifiedPass");code!=http.StatusConflict{
  t.Fatalf("evicted native output was accepted for structured test: %d",code)
 }
}

func TestNativeGoTestRejectsTruncatedNativeScrollback(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("POSIX PTY fixture")}
 if _,err:=exec.LookPath("go");err!=nil{t.Skip("Go executable unavailable")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=agentchat.NewTaskPlan("Bound native JSON test",[]agentchat.TaskPlanStep{{ID:"check",Title:"Go test",Acceptance:"Full native Go test output"}})
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"go.mod"),[]byte("module example.com/agentproof\n\ngo 1.21\n"),0600);err!=nil{t.Fatal(err)}
 source:=`package agentproof
import ("testing";"strings")
func TestVerifiedPass(t *testing.T){t.Log(strings.Repeat("x",8000))}
`
 if err:=os.WriteFile(filepath.Join(s.Workspace,"proof_test.go"),[]byte(source),0600);err!=nil{t.Fatal(err)}
 manager:=session.NewManager(512)
 s.Sessions=manager
 defer manager.Shutdown(time.Second)
 spec:=tasks.Execution{TaskID:23,Label:"Go test",Command:"go",
  Args:[]string{"test","-json","-count=1","./..."},Cwd:s.Workspace,Env:os.Environ(),Preview:"go test -json ./..."}
 started,err:=manager.Start(spec);if err!=nil{t.Fatal(err)}
 deadline:=time.Now().Add(60*time.Second)
 var meta session.Metadata
 for time.Now().Before(deadline){
  meta,_=manager.Metadata(started.ID)
  if meta.Status!="running"{break}
  time.Sleep(20*time.Millisecond)
 }
 if meta.Status!="exited"||meta.ExitCode==nil||*meta.ExitCode!=0{t.Fatalf("native truncated test did not exit 0: %+v",meta)}
 if meta.OutputBytes<=meta.RetainedBytes{t.Fatalf("fixture did not truncate output: %+v",meta)}
 x.TaskPlan.Steps[0].TaskCommandGuard=&agentchat.TaskCommandGuard{
  TaskID:23,CommandSHA256:meta.CommandSHA256,BoundAt:x.TaskPlan.CreatedAt.Add(-2*time.Second)}
 result,err:=s.verifyNativeGoTestCaseEvidence(x,meta.ID,23,"check",
  "example.com/agentproof","TestVerifiedPass","")
 if err==nil{t.Fatalf("truncated scrollback accepted: %+v",result)}
}
