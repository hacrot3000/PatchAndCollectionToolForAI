package server

import (
 "encoding/json"
 "fmt"
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

// Proves a real host-native process and complete structured Go test JSON can
// complete individual milestones through the paged one-segment endpoint.
// No in-test fabricated proof is passed as the native success authority.
func TestAgentPagedNativeTaskAndGoSuiteProofUsesOneVerifiedSegment(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("native PTY requires POSIX")}
 if _,err:=exec.LookPath("go");err!=nil{t.Skip("Go is unavailable")}
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 steps:=make([]agentchat.TaskPlanStep,160)
 for i:=range steps{
  steps[i]=agentchat.TaskPlanStep{
   ID:fmt.Sprintf("step-%04d",i),
   Title:"Host native Task evidence",
   Acceptance:"Owner-bound native process and complete JSON test proof",
  }
 }
 x.TaskPlan,err=agentchat.NewTaskPlan("Verify native Task and two Go tests in a large plan",steps)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if len(x.TaskPlanSegments)!=5{t.Fatal("160-step test plan was not persisted as immutable pages")}
 originalRefs:=append([]agentchat.TaskPlanSegmentRef(nil),x.TaskPlanSegments...)

 if err=os.WriteFile(filepath.Join(s.Workspace,"go.mod"),
  []byte("module example.com/pagednativeproof\n\ngo 1.21\n"),0600);err!=nil{t.Fatal(err)}
 source:="package pagednativeproof\nimport \"testing\"\nfunc TestFirst(t *testing.T){t.Log(\"first passed\")}\nfunc TestSecond(t *testing.T){t.Log(\"second passed\")}\n"
 if err=os.WriteFile(filepath.Join(s.Workspace,"native_proof_test.go"),
  []byte(source),0600);err!=nil{t.Fatal(err)}
 manager:=session.NewManager(2<<20)
 s.Sessions=manager
 defer manager.Shutdown(time.Second)
 launch:=func()session.Metadata{
  t.Helper()
  started,e:=manager.Start(tasks.Execution{
   TaskID:55,Label:"Native verified Go suite",
   Command:"go",Args:[]string{"test","-json","-count=1","./..."},
   Cwd:s.Workspace,Env:os.Environ(),Preview:"go test -json -count=1 ./...",
  })
  if e!=nil{t.Fatal(e)}
  until:=time.Now().Add(60*time.Second)
  for time.Now().Before(until){
   meta,ok:=manager.Metadata(started.ID)
   if ok&&meta.Status!="running"{
    if meta.Status!="exited"||meta.ExitCode==nil||*meta.ExitCode!=0||
     meta.TestRunner!="go_test_json"{
     t.Fatalf("real native Go suite failed: %+v",meta)
    }
    return meta
   }
   time.Sleep(25*time.Millisecond)
  }
  t.Fatal("real go test -json Task exceeded fixture deadline")
  return session.Metadata{}
 }
 baseline:=launch()
 planPath:="/api/agent/sessions/"+id+"/plan?offset=128&limit=24"
 post:=func(payload map[string]any)int{
  t.Helper()
  state,e:=store.Get(id,"local");if e!=nil{t.Fatal(e)}
  payload["expected_revision"]=state.Revision
  r:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,planPath,payload)
  if r.Code==http.StatusOK{
   var page agentchat.TaskPlanPageResult
   if e=json.Unmarshal(r.Body.Bytes(),&page);e!=nil{t.Fatal(e)}
   if page.Revision!=state.Revision+1||page.Total!=160||
    page.Page.Offset!=128||page.Page.Limit!=24||
    len(page.Plan.Steps)!=24{
    t.Fatalf("non-atomic or unbounded native proof response: %+v",page)
   }
  }
  return r.Code
 }
 for _,step:=range []string{"step-0135","step-0136"}{
  if code:=post(map[string]any{
   "action":"set_task_command_guard","step_id":step,
   "enabled":true,"confirm_command_guard":true,
   "evidence_task_id":55,"expected_command_sha256":baseline.CommandSHA256,
  });code!=http.StatusOK{t.Fatalf("bind native command for %s failed: %d",step,code)}
 }
 if code:=post(map[string]any{
  "action":"set_go_test_requirements","step_id":"step-0135",
  "enabled":true,"confirm_go_tests":true,
  "evidence_go_package":"example.com/pagednativeproof",
  "required_go_tests":[]string{"TestFirst","TestSecond"},
 });code!=http.StatusOK{t.Fatalf("owner-required Go tests rejected: %d",code)}
 if code:=post(map[string]any{
  "action":"set_acceptance_criteria","step_id":"step-0135",
  "confirm_coverage":true,
  "acceptance_criteria":[]map[string]any{{
   "id":"workflow","description":"Owner verified relevant behavior",
   "required_tests":[]string{"TestFirst","TestSecond"},
  }},
 });code!=http.StatusOK{t.Fatalf("owner acceptance matrix rejected: %d",code)}
 if code:=post(map[string]any{
  "action":"set_step","step_id":"step-0135",
  "status":"done","evidence_kind":"go_test_suite",
  "evidence_ref":baseline.ID,"evidence_task_id":55,
 });code!=http.StatusConflict{
  t.Fatalf("prebinding or unreviewed suite was admitted: %d",code)
 }
 if code:=post(map[string]any{
  "action":"review_acceptance_criterion","step_id":"step-0135",
  "criterion_id":"workflow","coverage_review_note":"I reviewed expected behavior",
  "confirm_coverage":true,
 });code!=http.StatusOK{t.Fatalf("owner criterion review rejected: %d",code)}
 if code:=post(map[string]any{
  "action":"set_step","step_id":"step-0135",
  "status":"done","evidence_kind":"task_run",
  "evidence_ref":baseline.ID,"evidence_task_id":55,
 });code!=http.StatusConflict{
  t.Fatalf("weaker Task exit-zero bypassed required test suite: %d",code)
 }
 // Native Task timestamps have only whole-second resolution, so begin
 // AFTER the full owner checklist and review have been committed.
 time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second))+150*time.Millisecond)
 observed:=launch()
 if code:=post(map[string]any{
  "action":"set_step","step_id":"step-0135",
  "status":"done","evidence_kind":"go_test_suite",
  "evidence_ref":observed.ID,"evidence_task_id":55,
  "required_go_tests":[]string{"TestFirst"},
 });code!=http.StatusBadRequest{
  t.Fatalf("caller-supplied substitute Go tests accepted: %d",code)
 }
 if code:=post(map[string]any{
  "action":"set_step","step_id":"step-0135",
  "status":"done","evidence_kind":"go_test_suite",
  "evidence_ref":"invented-native-session","evidence_task_id":55,
 });code!=http.StatusConflict{
  t.Fatalf("unverified native suite ID admitted: %d",code)
 }
 for _,tc:=range []struct{
  step,kind string
  extra map[string]any
 }{
  {"step-0135","go_test_suite",nil},
  {"step-0136","go_test_case",map[string]any{
   "evidence_go_package":"example.com/pagednativeproof",
   "evidence_go_test":"TestFirst",
  }},
  {"step-0137","task_run",nil},
 }{
  payload:=map[string]any{
   "action":"set_step","step_id":tc.step,"status":"done",
   "evidence_kind":tc.kind,"evidence_ref":observed.ID,
   "evidence_task_id":55,"evidence_note":"Owner inspected real native receipt",
  }
  for k,v:=range tc.extra{payload[k]=v}
  if code:=post(payload);code!=http.StatusOK{
   t.Fatalf("real native %s evidence refused for %s: %d",tc.kind,tc.step,code)
  }
 }
 persisted,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if persisted.TaskPlan.State=="completed"||persisted.Revision<=x.Revision{
  t.Fatal("native evidence incorrectly auto-completed plan or did not persist")
 }
 for i,kind:=range []string{"go_test_suite","go_test_case","task_run"}{
  step:=persisted.TaskPlan.Steps[135+i]
  if step.Status!="done"||len(step.Evidence)!=1||
   step.Evidence[0].Kind!=kind||step.Evidence[0].Ref!=observed.ID||
   len(step.Evidence[0].Digest)!=64{
   t.Fatalf("native %s evidence not bound to canonical milestone: %+v",kind,step)
  }
 }
 for i,part:=range persisted.TaskPlanSegments{
  if i==135/32{
   if part==originalRefs[i]||part.Revision!=persisted.Revision{
    t.Fatal("native evidence did not commit changed 32-step page")
   }
  }else if part!=originalRefs[i]{
   t.Fatalf("native evidence rewrote unrelated page %d",i)
  }
 }
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 retained,err:=reopened.Get(id,"local")
 if err!=nil||retained.TaskPlan.Steps[135].GoTestRequirements==nil||
  retained.TaskPlan.Steps[135].GoTestRequirements.Tests[1]!="TestSecond"||
  retained.TaskPlan.Steps[136].Evidence[0].Digest!=persisted.TaskPlan.Steps[136].Evidence[0].Digest{
  t.Fatalf("restart lost native proof or bound checklist: %v",err)
 }
 handoff,_,err:=reopened.Handoff(id,"local")
 if err!=nil||!strings.Contains(handoff,"3/160"){
  t.Fatalf("native verified progress missing from derived handoff: %v",err)
 }
}
