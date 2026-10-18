package server

import (
 "strings"
 "testing"
)
func TestScheduleDependencyMultiParentEnvironment(t *testing.T){
 runs:=[]scheduleRun{
  {ID:"runA",ScheduleID:"build",Outputs:map[string]string{"SHA":"abc","SHARED":"fromA"}},
  {ID:"runB",ScheduleID:"test",Outputs:map[string]string{"SHARED":"fromB","RESULT":"PASS"}},
 }
 vars:=scheduleDependencyEnvAll("runB",[]string{"runA","runB"},runs)
 joined:="\n"+strings.Join(vars,"\n")+"\n"
 for _,want:=range []string{
  "TASKDECK_DEP_RESULT=PASS",
  "TASKDECK_DEP_SHARED=fromB",
  "TASKDECK_DEP_PARENT_1_RUN_ID=runA",
  "TASKDECK_DEP_PARENT_1_SCHEDULE_ID=build",
  "TASKDECK_DEP_PARENT_1_SHARED=fromA",
  "TASKDECK_DEP_PARENT_2_RUN_ID=runB",
  "TASKDECK_DEP_PARENT_2_SHARED=fromB",
  "TASKDECK_DEP_PARENT_2_RESULT=PASS",
 }{
  if !strings.Contains(joined,"\n"+want+"\n"){t.Errorf("missing %q in %q",want,joined)}
 }
}
func TestScheduleDependencyMultiParentDedupAndLimits(t *testing.T){
 runs:=[]scheduleRun{{ID:"A",ScheduleID:"one",Outputs:map[string]string{"OK":"1"}}}
 got:=scheduleDependencyEnvAll("",[]string{"A","A","missing"},runs)
 joined:=strings.Join(got,"\n")
 if strings.Count(joined,"TASKDECK_DEP_PARENT_1_OK=")!=1{t.Fatalf("duplicated first parent: %v",got)}
 if strings.Contains(joined,"TASKDECK_DEP_PARENT_2_OK="){t.Fatalf("duplicate or absent parent exposed: %v",got)}
}
