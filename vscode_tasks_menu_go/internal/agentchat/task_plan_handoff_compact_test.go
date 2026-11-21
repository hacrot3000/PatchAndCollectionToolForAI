package agentchat

import (
 "fmt"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestLargeTaskPlanHandoffStaysBoundedAndKeepsCanonicalEvidence(t *testing.T){
 root:=filepath.Join(t.TempDir(),"private","agent")
 store,err:=NewStore(root);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project-id","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 steps:=make([]TaskPlanStep,128)
 for i:=range steps{
  steps[i]=TaskPlanStep{
   ID:fmt.Sprintf("task-%03d",i),Title:fmt.Sprintf("Task %03d - %s",i,strings.Repeat("T",180)),
   Acceptance:fmt.Sprintf("Verify task %03d: %s",i,strings.Repeat("A",500)),
  }
 }
 plan,err:=NewTaskPlan("Implement bounded milestone handoff",steps)
 if err!=nil{t.Fatal(err)}
 for i:=0;i<90;i++{
  plan.Steps[i].Status="done"
  for k:=0;k<MaxTaskPlanEvidence;k++{
   plan.Steps[i].Evidence=append(plan.Steps[i].Evidence,TaskPlanEvidence{
    Kind:"manual",Note:fmt.Sprintf("Accepted step %03d proof %02d - %s",i,k,strings.Repeat("E",300)),
   })
  }
 }
 plan.Steps[90].Status="active"
 plan.Steps[93].Status="blocked"
 now:=time.Now().UTC()
 plan.Steps[91].TaskCommandGuard=&TaskCommandGuard{
  TaskID:91,CommandSHA256:strings.Repeat("a",64),BoundAt:now.Add(-time.Minute),
 }
 plan.Steps[91].GoTestRequirements=&TaskGoTestRequirements{
  Package:"example.com/project",Tests:[]string{"TestOne","TestTwo"},BoundAt:now,
 }
 if err:=ValidateTaskPlan(plan);err!=nil{t.Fatalf("valid 128-step plan rejected: %v",err)}
 x.TaskPlan=plan
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Implement all 128 milestones"})
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 out,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil{t.Fatal(err)}
 if len(out)>28000{t.Fatalf("large-plan handoff is still unbounded: %d bytes",len(out))}
 for _,want:=range []string{
  "Durable task plan (90/128)","pending=36, active=1, blocked=1, done=90",
  "Canonical step-inventory SHA-256","116 of 128 milestones omitted",
  "Next actionable milestone: task-090",
  "task-091","Expected native Task 91","TestOne, TestTwo",
  "/plan?offset=0&limit=24","task-089",
 }{
  if !strings.Contains(out,want){t.Fatalf("bounded handoff omitted %q",want)}
 }
 if strings.Contains(out,"task-127 —")||strings.Contains(out,"Accepted step 000 proof 11"){
  t.Fatal("unselected full acceptance/evidence leaked into bounded handoff")
 }
 if strings.Count(out,"- [")!=12{
  t.Fatalf("expected at most 12 detailed step rows, got %d",strings.Count(out,"- ["))
 }
 if rev!=x.Revision{t.Fatal("handoff returned wrong journal revision")}
 other,err:=NewStore(root);if err!=nil{t.Fatal(err)}
 restored,err:=other.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if len(restored.TaskPlan.Steps)!=128||
  len(restored.TaskPlan.Steps[0].Evidence)!=MaxTaskPlanEvidence||
  restored.TaskPlan.Steps[91].GoTestRequirements==nil{
  t.Fatal("compact handoff truncated canonical task evidence or bound Go requirements")
 }
 again,againRev,err:=other.Handoff(x.ID,"local")
 if err!=nil||againRev!=rev||again!=out{
  t.Fatalf("restarted handoff drifted from canonical snapshot: %v",err)
 }
}

func TestSmallTaskPlanHandoffRemainsFullyDetailed(t *testing.T){
 steps:=make([]TaskPlanStep,compactHandoffPlanThreshold)
 for i:=range steps{
  steps[i]=TaskPlanStep{
   ID:fmt.Sprintf("small-%02d",i),Title:fmt.Sprintf("Small %02d",i),
   Acceptance:fmt.Sprintf("Acceptance %02d",i),
  }
 }
 plan,err:=NewTaskPlan("Keep small plan handoff compatibility",steps)
 if err!=nil{t.Fatal(err)}
 plan.Steps[0].Status="done"
 for i:=0;i<4;i++{
  plan.Steps[0].Evidence=append(plan.Steps[0].Evidence,
   TaskPlanEvidence{Kind:"manual",Note:fmt.Sprintf("Distinct proof %d",i)})
 }
 out:=HandoffMarkdown(Session{
  ID:"0123456789abcdef0123456789abcdef",ProjectID:"project",Status:"completed",TaskPlan:plan,
 })
 if strings.Contains(out,"Large plan summary:")||strings.Contains(out,"omitted from this handoff"){
  t.Fatal("legacy small plan was unexpectedly compacted")
 }
 for i:=0;i<4;i++{
  if !strings.Contains(out,fmt.Sprintf("Distinct proof %d",i)){
   t.Fatal("small plan should still contain every evidence note")
  }
 }
 for i:=0;i<compactHandoffPlanThreshold;i++{
  if !strings.Contains(out,fmt.Sprintf("small-%02d",i)){
   t.Fatal("small plan should retain every milestone")
  }
 }
}
