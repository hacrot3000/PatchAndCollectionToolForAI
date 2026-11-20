package server

import(
 "testing"
 "time"
)

func TestScheduleConditionTTLExpiresOldEvidence(t *testing.T){
 def:=scheduleDefinition{ConditionTTLSeconds:300}
 now:=time.Date(2026,10,18,10,0,0,0,time.UTC)
 latched:=map[string]bool{"url":true,"git":true}
 timestamps:=map[string]time.Time{"url":now.Add(-301*time.Second),"git":now.Add(-299*time.Second)}
 active:=scheduleActiveSignals(def,latched,timestamps,now)
 if active["url"]||!active["git"]{t.Fatalf("stale evidence accepted: %+v",active)}
 def.ConditionTTLSeconds=0
 if !scheduleActiveSignals(def,latched,timestamps,now)["url"]{t.Fatal("TTL=0 must preserve existing latch semantics")}
}
func TestScheduleCorrelatedParentSignalsDoNotCombineUnrelatedWorkflows(t *testing.T){
 group:=scheduleConditionGroup{Operator:"and",Conditions:[]scheduleCondition{
  {ID:"build",Type:"dependency"},{ID:"test",Type:"dependency"},
 }}
 matched:=map[string]bool{"build":true,"test":true}
 flows:=map[string]string{"build":"flow-1","test":"flow-2"}
 ok,_,_:=scheduleEvaluateWithWorkflow(group,matched,flows,nil)
 if ok{t.Fatal("different workflow results cannot satisfy same AND")}
 flows["test"]="flow-1"
 ok,flow,_:=scheduleEvaluateWithWorkflow(group,matched,flows,nil)
 if !ok||flow!="flow-1"{t.Fatalf("expected matching workflow flow-1, got %v %q",ok,flow)}
 group.Operator="or"
 flows["test"]="flow-2"
 ok,_,_ =scheduleEvaluateWithWorkflow(group,matched,flows,nil)
 if !ok{t.Fatal("OR may accept any completed branch")}
}
func TestScheduleCronPreviewAndAnalytics(t *testing.T){
 now:=time.Date(2026,10,18,10,2,35,0,time.UTC)
 dates:=scheduleNextCronTimes("*/5 * * * *","UTC",now,3)
 if len(dates)!=3||dates[0].Minute()!=5||dates[1].Minute()!=10||dates[2].Minute()!=15{
  t.Fatalf("wrong next Cron matches: %v",dates)
 }
 finish:=now.Add(7*time.Second)
 runs:=[]scheduleRun{
  {ScheduleID:"A",StartedAt:now,FinishedAt:&finish,Status:"success"},
  {ScheduleID:"B",StartedAt:now,Status:"running"},
 }
 stats:=scheduleRunStatistics(runs)
 if len(stats)!=2{t.Fatalf("expected one result per schedule, got %+v",stats)}
 for _,row:=range stats{
  if row.ScheduleID=="A"&&(row.Completed!=1||row.Success!=1||row.AverageDurationSeconds!=7){t.Fatalf("wrong metrics: %+v",row)}
  if row.ScheduleID=="B"&&row.Completed!=0{t.Fatalf("running run should not count as completed: %+v",row)}
 }
}
func TestScheduleNewRuntimeLimitsValidate(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 def:=scheduleDefinition{ID:"cron1",Name:"Cron",Type:"cron",Cron:"*/5 * * * *",Target:"command",Command:"echo good",
  RetainDays:7,RetainRuns:20,RetryMax:2,RetryDelaySeconds:30,RetryBackoff:2,
  ConditionTTLSeconds:300,ConcurrencyPolicy:"skip",ResourceGroup:"deploy-prod"}
 store:=scheduleStore{Schedules:[]scheduleDefinition{def}}
 if err:=s.validateSchedule(def,store);err!=nil{t.Fatal(err)}
 def.RetryMax=11
 if err:=s.validateSchedule(def,store);err==nil{t.Fatal("unbounded retry accepted")}
 def.RetryMax=2
 for _,policy:=range []string{"skip","parallel","queue","replace"}{
  def.ConcurrencyPolicy=policy
  if err:=s.validateSchedule(def,store);err!=nil{t.Fatalf("supported concurrency policy %q rejected: %v",policy,err)}
 }
 def.ConcurrencyPolicy="unsupported-policy"
 if err:=s.validateSchedule(def,store);err==nil{t.Fatal("unsupported concurrency behavior accepted")}
}


func TestScheduleWorkflowCorrelationChoosesCompatibleORBranch(t *testing.T){
 group:=scheduleConditionGroup{Operator:"and",Groups:[]scheduleConditionGroup{
  {Operator:"or",Conditions:[]scheduleCondition{{ID:"a"},{ID:"b"}}},
  {Operator:"or",Conditions:[]scheduleCondition{{ID:"c"},{ID:"d"}}},
 }}
 matched:=map[string]bool{"a":true,"b":true,"c":true,"d":true}
 flows:=map[string]string{"a":"flow-one","b":"flow-two","c":"flow-two","d":"flow-three"}
 ready,flow,_:=scheduleEvaluateWithWorkflow(group,matched,flows,nil)
 if !ready||flow!="flow-two"{t.Fatalf("must select compatible b + c, got ready=%v flow=%s",ready,flow)}
 flows["c"]="flow-four"
 ready,_,_=scheduleEvaluateWithWorkflow(group,matched,flows,nil)
 if ready{t.Fatal("unrelated OR branches must not be combined")}
}
