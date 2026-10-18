package server

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestScheduleCronFiveFieldAndWeekday(t *testing.T){
 cases:=[]struct{spec string; at time.Time; ok bool}{
  {"*/5 * * * *",time.Date(2026,10,17,10,10,0,0,time.UTC),true},
  {"*/5 * * * *",time.Date(2026,10,17,10,11,0,0,time.UTC),false},
  {"10 9 * * 1-5",time.Date(2026,10,19,9,10,0,0,time.UTC),true},
  {"10 9 * * 1-5",time.Date(2026,10,18,9,10,0,0,time.UTC),false},
  {"0 0 1 * 0",time.Date(2026,11,1,0,0,0,0,time.UTC),true},
 }
 for _,tc:=range cases{
  matched,err:=cronMatches(tc.spec,tc.at)
  if err!=nil{t.Errorf("%s: %v",tc.spec,err);continue}
  if matched!=tc.ok{t.Errorf("%s at %s: got %v want %v",tc.spec,tc.at,matched,tc.ok)}
 }
 for _,bad:=range []string{"* * * *","60 * * * *","*/0 * * * *","* 24 * * *","bad * * * *"}{
  if _,err:=cronMatches(bad,time.Now());err==nil{t.Errorf("accepted invalid cron %q",bad)}
 }
}
func TestScheduleRetentionLaterConditionWins(t *testing.T){
 folder:=t.TempDir()
 if err:=os.MkdirAll(filepath.Join(folder,"logs"),0700);err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 e:=&scheduleEngine{folder:folder,store:scheduleStore{Schedules:[]scheduleDefinition{{ID:"S",RetainDays:2,RetainRuns:10}}}}
 for i:=0;i<60;i++{
  e.store.Runs=append(e.store.Runs,scheduleRun{ID:newScheduleID(),ScheduleID:"S",Status:"success",StartedAt:now.Add(-time.Duration(i)*time.Hour)})
 }
 e.pruneLocked(now)
 // One per hour: 2 days retain 49 samples (including the present hour).
 if got:=len(e.store.Runs);got!=49{t.Fatalf("got %d, want 49 because age beats count",got)}
 e.store.Schedules[0].RetainDays=1
 e.store.Schedules[0].RetainRuns=55
 e.pruneLocked(now)
 if got:=len(e.store.Runs);got!=49{t.Fatalf("retention cannot resurrect deleted runs: %d",got)}
}
func TestScheduleDependencyCycle(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 store:=scheduleStore{Schedules:[]scheduleDefinition{
  {ID:"A",Name:"A",Type:"dependency",ParentID:"C"},
  {ID:"B",Name:"B",Type:"dependency",ParentID:"A"},
  {ID:"C",Name:"C",Type:"dependency",ParentID:"B"},
 }}
 config:=scheduleDefinition{ID:"A",Name:"A",Type:"dependency",ParentID:"C",ParentState:"success",Target:"command",Command:"echo hello",RetainDays:7,RetainRuns:10}
 if err:=s.validateSchedule(config,store);err==nil||!strings.Contains(err.Error(),"cycle"){t.Fatalf("expected cycle rejection: %v",err)}
}
func TestScheduleURLValidationBlocksUnsafeSchemes(t *testing.T){
 for _,value:=range []string{"file:///etc/passwd","http://localhost:8080/","http://user:password@example.com","https://example.com/#fragment","http:///bad"}{
  if err:=validateScheduleURL(value);err==nil{t.Errorf("unsafe schedule URL accepted %q",value)}
 }
 for _,value:=range []string{"https://example.com/health","http://example.com/status"}{
  if err:=validateScheduleURL(value);err!=nil{t.Errorf("safe syntax rejected %q: %v",value,err)}
 }
}
func TestScheduleJSONAndXMLConditions(t *testing.T){
 body:=`{"status":{"state":"READY"},"values":[{"code":3}]}`
 if !scheduleJSONMatch(body,"status.state","READY"){t.Fatal("nested JSON match")}
 if !scheduleJSONMatch(body,"values.0.code","3"){t.Fatal("array JSON match")}
 if scheduleJSONMatch(body,"status.state","WAITING"){t.Fatal("mismatched value")}
 xml:=`<root><status>READY</status><error>NO</error></root>`
 if !scheduleXMLMatch(xml,"root/status","READY"){t.Fatal("XML match")}
 if scheduleXMLMatch(xml,"root/error","READY"){t.Fatal("wrong element match")}
}
func TestScheduleStoreReopensRunningAsInterrupted(t *testing.T){
 // Keep the state folder in the user cache for real deployments; this test
 // covers persistence semantics without depending on the user's cache path.
 e:=&scheduleEngine{folder:t.TempDir(),store:scheduleStore{Schedules:[]scheduleDefinition{{ID:"a"}},Runs:[]scheduleRun{{ID:"b",ScheduleID:"a",Status:"running",StartedAt:time.Now().UTC()}}}}
 if err:=e.persistLocked();err!=nil{t.Fatal(err)}
 contents,err:=os.ReadFile(filepath.Join(e.folder,"state.json"));if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(contents),`"status": "running"`){t.Fatal("state not persisted")}
}
