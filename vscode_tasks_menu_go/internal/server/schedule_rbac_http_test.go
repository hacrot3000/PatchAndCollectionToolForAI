package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestScheduleHTTPViewerCannotReadSecretsOrExecute(t *testing.T) {
 const secret="UNIQUE_PRIVATE_SCHEDULE_CANARY"
 e:=&scheduleEngine{
  folder:t.TempDir(),
  lastStatus:map[string]string{"job:trigger":secret},
  store:scheduleStore{
   Schedules:[]scheduleDefinition{{ID:"job",Name:"Public job",Enabled:true,
    Type:"cron",Cron:"* * * * *",Timezone:"Local",Target:"command",
    Command:secret,URL:"https://example.invalid/"+secret,
    NotifyWebhookURL:secret,RetainDays:7,RetainRuns:30}},
   Runs:[]scheduleRun{{ID:"run",ScheduleID:"job",Status:"success",
    StartedAt:time.Now().UTC(),Outputs:map[string]string{"secret":secret},Error:secret}},
  },
 }
 s:=&Server{scheduleState:e}
 request:=func(method,path,body string,grants ...string)*http.Request{
  req:=httptest.NewRequest(method,path,strings.NewReader(body))
  m:=map[string]bool{}
  for _,grant:=range grants{m[grant]=true}
  return req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},identity.Principal{Permissions:m}))
 }
 viewer:=request(http.MethodGet,"/api/schedules","",identity.PermissionSchedulesView)
 out:=httptest.NewRecorder()
 s.schedulesAPI(out,viewer)
 if out.Code!=http.StatusOK{t.Fatalf("view status=%d: %s",out.Code,out.Body)}
 if strings.Contains(out.Body.String(),secret)||strings.Contains(out.Body.String(),`"id":"run"`){
  t.Fatalf("view-only response leaks job definition/output: %s",out.Body)
 }
 if !strings.Contains(out.Body.String(),`"redacted":true`){
  t.Fatalf("view-only endpoint not redacted: %s",out.Body)
 }
 for _,action:=range []string{"upsert","run","stop","backfill","signal_webhook","inspect","simulate","preview_backfill","pause","resume","delete","import"}{
  for _,grants:=range [][]string{
   {identity.PermissionSchedulesView},
   {identity.PermissionSchedulesManage},
   {identity.PermissionSchedulesRun},
  }{
   payload:=`{"action":"`+action+`","id":"job"}`
   rec:=httptest.NewRecorder()
   s.schedulesAPI(rec,request(http.MethodPost,"/api/schedules",payload,grants...))
   if rec.Code!=http.StatusForbidden{
    t.Fatalf("action=%s grants=%v unexpectedly returned %d: %s",action,grants,rec.Code,rec.Body)
   }
  }
 }
 for _,path:=range []string{"/api/schedules/log?run_id=run","/api/schedules/repositories"}{
  rec:=httptest.NewRecorder()
  req:=request(http.MethodGet,path,"",identity.PermissionSchedulesView)
  if strings.Contains(path,"/log"){s.scheduleLogAPI(rec,req)}else{s.scheduleRepositoriesAPI(rec,req)}
  if rec.Code!=http.StatusForbidden{t.Fatalf("viewer %s returned %d: %s",path,rec.Code,rec.Body)}
 }
}
