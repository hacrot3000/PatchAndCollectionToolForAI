package server

import(
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestScheduleFileTriggerStaysInsideWorkspace(t *testing.T){
 root:=t.TempDir()
 s:=&Server{Workspace:root}
 if err:=s.validateScheduleFile("../outside","change");err==nil{t.Fatal("directory traversal accepted")}
 if err:=s.validateScheduleFile("/etc/passwd","exists");err==nil{t.Fatal("absolute file path accepted")}
 if err:=s.validateScheduleFile("missing.log","change");err==nil{t.Fatal("missing change target accepted")}
 file:=filepath.Join(root,"output.log")
 if err:=os.WriteFile(file,[]byte("first"),0600);err!=nil{t.Fatal(err)}
 if err:=s.validateScheduleFile("output.log","change");err!=nil{t.Fatal(err)}
 cfg:=scheduleDefinition{FilePath:"output.log",FileCondition:"change"}
 first,matched,err:=s.scheduleFileObservation(context.Background(),cfg)
 if err!=nil||!matched{t.Fatalf("file observation failed: %v",err)}
 if err:=os.WriteFile(file,[]byte("second-longer"),0600);err!=nil{t.Fatal(err)}
 second,matched,err:=s.scheduleFileObservation(context.Background(),cfg)
 if err!=nil||!matched||first==second{t.Fatal("file modification was not observed")}
 if err:=os.Symlink(file,filepath.Join(root,"link.log"));err==nil{
  if err:=s.validateScheduleFile("link.log","exists");err==nil{t.Fatal("symlink should not be watched")}
 }
 cfg.FilePath="missing.log";cfg.FileCondition="missing"
 _,matched,err=s.scheduleFileObservation(context.Background(),cfg)
 if err!=nil||!matched{t.Fatal("missing path trigger must match absent path")}
}
func TestScheduleCalendarGate(t *testing.T){
 now:=time.Date(2026,10,19,9,30,0,0,time.UTC) // Monday
 def:=scheduleDefinition{Timezone:"UTC",AllowedWeekdays:[]int{1,2,3,4,5},WindowStart:"09:00",WindowEnd:"17:00"}
 if !scheduleCalendarAllows(def,now){t.Fatal("Monday 09:30 should be allowed")}
 if scheduleCalendarAllows(def,now.Add(10*time.Hour)){t.Fatal("19:30 should not be allowed")}
 if scheduleCalendarAllows(def,now.Add(5*24*time.Hour)){t.Fatal("Saturday should be excluded")}
 def.ExcludeDates=[]string{"2026-10-19"}
 if scheduleCalendarAllows(def,now){t.Fatal("excluded date matched")}
 def.ExcludeDates=nil;def.WindowStart="22:00";def.WindowEnd="06:00"
 if !scheduleCalendarAllows(def,time.Date(2026,10,19,23,0,0,0,time.UTC)){t.Fatal("overnight gate must allow 23:00")}
 def.WindowStart="bad"
 if err:=validateScheduleCalendar(def);err==nil{t.Fatal("invalid time window accepted")}
}
func TestScheduleOutputsBoundedAndParentEnv(t *testing.T){
 logs:="TASKDECK_OUTPUT VERSION=1.2.3\nTASKDECK_OUTPUT BUILD_SHA=abc123\nTASKDECK_OUTPUT PATH=/tmp/no\nTASKDECK_OUTPUT BAD-NAME=oops\n"
 variables:=scheduleParseOutputs(logs)
 if variables["VERSION"]!="1.2.3"||variables["BAD-NAME"]!=""{t.Fatalf("variables parsed incorrectly: %v",variables)}
 run:=scheduleRun{ID:"parent",Outputs:variables}
 env:=scheduleDependencyEnv("parent",[]scheduleRun{run})
 if len(env)!=3{t.Fatalf("expected three exported variables, got %v",env)}
 if len(scheduleDependencyEnv("missing",[]scheduleRun{run}))!=0{t.Fatal("unknown parent exported variables")}
}
func TestScheduleImportRemapsNestedDependencies(t *testing.T){
 a:=scheduleDefinition{ID:"oldA",Name:"A",Type:"cron",Cron:"* * * * *",Enabled:true}
 b:=scheduleDefinition{ID:"oldB",Name:"B",Type:"multi",Enabled:true,ConditionGroup:&scheduleConditionGroup{
  Operator:"and",Conditions:[]scheduleCondition{{ID:"fromA",Type:"dependency",ParentID:"oldA"}},
 }}
 imported:=schedulePrepareImport([]scheduleDefinition{a,b})
 if imported[0].Enabled||imported[1].Enabled{t.Fatal("imports must be disabled")}
 if imported[0].ID=="oldA"||imported[1].ID=="oldB"{t.Fatal("IDs must be fresh")}
 if imported[1].ConditionGroup.Conditions[0].ParentID!=imported[0].ID{t.Fatal("nested parent not remapped")}
}
func TestScheduleBackfillGuardRails(t *testing.T){
 cfg:=scheduleDefinition{Type:"cron",Cron:"*/5 * * * *",Timezone:"UTC"}
 from:=time.Date(2026,10,18,10,0,0,0,time.UTC)
 times,err:=scheduleBackfillTimes(cfg,from,from.Add(11*time.Minute))
 if err!=nil||len(times)!=3{t.Fatalf("expected minutes 0, 5, 10: %v %v",times,err)}
 if _,err:=scheduleBackfillTimes(cfg,from,from.Add(8*24*time.Hour));err==nil{t.Fatal("backfill over 7 days accepted")}
 cfg.ConditionGroup=&scheduleConditionGroup{Operator:"and",Conditions:[]scheduleCondition{{ID:"cron",Type:"cron"},{ID:"git",Type:"git"}}}
 if _,err:=scheduleBackfillTimes(cfg,from,from.Add(time.Hour));err==nil{t.Fatal("composite trigger must not be backfilled unsafely")}
}
func TestScheduleQueueLimitsAndRestartState(t *testing.T){
 e:=&scheduleEngine{folder:t.TempDir(),store:scheduleStore{},active:map[string]func(){}}
 def:=scheduleDefinition{ID:"scheduled"}
 for i:=0;i<10;i++{
  if err:=e.scheduleQueueLocked(def,scheduleRun{ID:newScheduleID(),ScheduleID:"scheduled",Status:"queued"});err!=nil{t.Fatal(err)}
 }
 if err:=e.scheduleQueueLocked(def,scheduleRun{ID:newScheduleID(),ScheduleID:"scheduled"});err==nil||!strings.Contains(err.Error(),"limit"){
  t.Fatalf("expected per-schedule queue cap, got %v",err)
 }
}
