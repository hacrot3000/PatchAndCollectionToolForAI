package agentchat

import (
 "errors"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestOwnerAcceptanceMatrixCannotBeReplacedByGoPASSAlone(t *testing.T){
 plan,err:=NewTaskPlan("Do not infer semantic coverage from tests",[]TaskPlanStep{{
  ID:"feature",Title:"Implement the feature",Acceptance:"All owner criteria reviewed",
 }})
 if err!=nil{t.Fatal(err)}
 plan.Steps[0].AcceptanceCriteria=[]TaskAcceptanceCriterion{
  {ID:"workflow",Description:"The workflow handles the requested input"},
  {ID:"regression",Description:"Existing users remain compatible"},
 }
 if err:=ValidateTaskPlan(plan);err!=nil{t.Fatal(err)}
 x:=Session{TaskPlan:plan}
 manual:=&TaskPlanEvidence{Kind:"manual",Note:"This only describes the test result"}
 if e:=UpdateTaskPlanStep(&x,"feature","done",manual);e==nil{
  t.Fatal("a generic successful test/note accepted unreviewed owner criteria")
 }
 if len(x.TaskPlan.Steps[0].Evidence)>0||x.TaskPlan.Steps[0].Status!="pending"{
  t.Fatal("rejected completion mutated milestone evidence or status")
 }
 now:=time.Now().UTC()
 plan.Steps[0].AcceptanceCriteria[0].ReviewNote="I verified the expected workflow covers case A"
 plan.Steps[0].AcceptanceCriteria[0].ReviewedAt=&now
 if e:=UpdateTaskPlanStep(&x,"feature","done",manual);e==nil{
  t.Fatal("one of two owner reviews was enough to complete the step")
 }
 plan.Steps[0].AcceptanceCriteria[1].ReviewNote="I reviewed legacy compatibility separately"
 plan.Steps[0].AcceptanceCriteria[1].ReviewedAt=&now
 if err:=ValidateTaskPlan(plan);err!=nil{t.Fatal(err)}
 if err:=UpdateTaskPlanStep(&x,"feature","done",manual);err!=nil{t.Fatal(err)}
 if !TaskPlanStepCriteriaReviewed(plan.Steps[0])||!TaskPlanAllDone(plan){
  t.Fatal("fully reviewed and explicitly evidenced owner step was not completed")
 }
 plan.Steps[0].AcceptanceCriteria[0].ReviewNote=""
 if err:=ValidateTaskPlan(plan);!errors.Is(err,ErrInvalidSession){
  t.Fatal("completed step with missing review note was trusted by validation")
 }
}

func TestOwnerAcceptanceMatrixRejectsUnknownTestsOrWeakenedBindings(t *testing.T){
 now:=time.Now().UTC()
 command:=&TaskCommandGuard{TaskID:25,CommandSHA256:strings.Repeat("a",64),BoundAt:now.Add(-time.Minute)}
 req:=&TaskGoTestRequirements{Package:"example.com/test",Tests:[]string{"TestOne","TestTwo"},BoundAt:now}
 base:=TaskPlanStep{
  ID:"coverage",Title:"Build behavior",Acceptance:"All criteria reviewed",
  TaskCommandGuard:command,GoTestRequirements:req,
 }
 valid:=base
 valid.AcceptanceCriteria=[]TaskAcceptanceCriterion{{
  ID:"case_A",Description:"Input A works",RequiredTests:[]string{"TestOne"},
 }}
 if err:=ValidateTaskAcceptanceCriteria(valid);err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{
  name string
  mutate func(*TaskPlanStep)
 }{
  {"unlocked-test",func(s *TaskPlanStep){s.GoTestRequirements=nil}},
  {"test-not-predeclared",func(s *TaskPlanStep){s.AcceptanceCriteria[0].RequiredTests=[]string{"TestUnknown"}}},
  {"duplicate-mapped-test",func(s *TaskPlanStep){s.AcceptanceCriteria[0].RequiredTests=[]string{"TestOne","TestOne"}}},
  {"duplicate-criterion",func(s *TaskPlanStep){s.AcceptanceCriteria=append(s.AcceptanceCriteria,s.AcceptanceCriteria[0])}},
  {"empty-criterion",func(s *TaskPlanStep){s.AcceptanceCriteria[0].Description="  "}},
  {"fake-reviewed-date",func(s *TaskPlanStep){s.AcceptanceCriteria[0].ReviewNote="Approved"}},
 }{
  t.Run(tc.name,func(t *testing.T){
   target:=valid
   target.AcceptanceCriteria=append([]TaskAcceptanceCriterion(nil),valid.AcceptanceCriteria...)
   tc.mutate(&target)
   if err:=ValidateTaskAcceptanceCriteria(target);!errors.Is(err,ErrInvalidSession){
    t.Fatalf("invalid owner coverage matrix accepted: %v",err)
   }
  })
 }
}

func TestOwnerAcceptanceMatrixSurvivesSegmentedPersistenceAndHandoff(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.TaskPlan,err=NewTaskPlan("Large plan with explicit human coverage reviews",planStepFixtures(160))
 if err!=nil{t.Fatal(err)}
 // Compact handoff deliberately selects only the earliest unfinished
 // milestones. Make index 135 the FIRST unfinished step, while retaining a
 // real 160-step segmented plan and owner-attested prior completions.
 for i:=0;i<135;i++{
  x.TaskPlan.Steps[i].Status="done"
  x.TaskPlan.Steps[i].Evidence=[]TaskPlanEvidence{{
   Kind:"manual",Note:"Owner attested earlier completion",
  }}
 }
 target:=&x.TaskPlan.Steps[135]
 target.AcceptanceCriteria=[]TaskAcceptanceCriterion{{
  ID:"feature_behavior",Description:"Behavior matches the exact acceptance statement",
 }}
 at:=time.Now().UTC()
 target.AcceptanceCriteria[0].ReviewNote="The owner reviewed output and relevant edge cases"
 target.AcceptanceCriteria[0].ReviewedAt=&at
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 reopened,err:=NewStore(filepath.Join(store.dir));if err!=nil{t.Fatal(err)}
 got,err:=reopened.Get(x.ID,"local");if err!=nil{t.Fatal(err)}
 if len(got.TaskPlan.Steps)!=160||len(got.TaskPlan.Steps[135].AcceptanceCriteria)!=1||
  got.TaskPlan.Steps[135].AcceptanceCriteria[0].ReviewNote!=target.AcceptanceCriteria[0].ReviewNote{
  t.Fatal("segmented journal lost owner acceptance criteria")
 }
 page,err:=reopened.TaskPlanPage(x.ID,"local",128,24)
 if err!=nil{t.Fatal(err)}
 if len(page.Plan.Steps)!=24||
  page.Plan.Steps[7].AcceptanceCriteria[0].ID!="feature_behavior"{
  t.Fatal("paged GET omitted owner acceptance coverage")
 }
 handoff,_,err:=reopened.Handoff(x.ID,"local")
 if err!=nil||!strings.Contains(handoff,"OWNER acceptance coverage: 1/1"){
  t.Fatalf("compact handoff omitted owner-reviewed criterion count: %v",err)
 }
 revised,err:=reopened.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
  "step-0135","blocked",128,24)
 if err!=nil{t.Fatal(err)}
 if revised.Plan.Steps[7].AcceptanceCriteria[0].ReviewNote!=target.AcceptanceCriteria[0].ReviewNote{
  t.Fatal("single-segment status edit discarded criterion review")
 }
}
