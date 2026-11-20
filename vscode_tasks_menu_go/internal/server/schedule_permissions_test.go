package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

func schedulePermissionRequest(grants ...string)*http.Request{
 perms:=map[string]bool{}
 for _,grant:=range grants{perms[grant]=true}
 req:=httptest.NewRequest(http.MethodGet,"/api/schedules",nil)
 return req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},
  identity.Principal{Permissions:perms}))
}

func TestSchedulePermissionActionMapping(t *testing.T){
 for _,tc:=range []struct{action,want string}{
  {"inspect",identity.PermissionSchedulesView},
  {"simulate",identity.PermissionSchedulesView},
  {"preview_backfill",identity.PermissionSchedulesView},
  {"run",identity.PermissionSchedulesRun},
  {"stop",identity.PermissionSchedulesRun},
  {"backfill",identity.PermissionSchedulesRun},
  {"signal_webhook",identity.PermissionSchedulesRun},
  {"upsert",identity.PermissionSchedulesManage},
  {"delete",identity.PermissionSchedulesManage},
  {"import",identity.PermissionSchedulesManage},
  {"pause",identity.PermissionSchedulesManage},
  {"resume",identity.PermissionSchedulesManage},
  {"unknown_action",identity.PermissionSchedulesManage},
 }{
  if got:=schedulePermissionForAction(tc.action);got!=tc.want{
   t.Errorf("%s requires %s, expected %s",tc.action,got,tc.want)
  }
 }
}
func TestSchedulePermissionLeastPrivilege(t *testing.T){
 tests:=[]struct{
  name string
  grants []string
  view,run,manage bool
 }{
  {"viewer",[]string{identity.PermissionSchedulesView},true,false,false},
  {"runner cannot trigger arbitrary jobs",[]string{identity.PermissionSchedulesRun},true,false,false},
  {"manager cannot create executable jobs alone",[]string{identity.PermissionSchedulesManage},true,false,false},
  {"paired elevated grants",[]string{identity.PermissionSchedulesManage,identity.PermissionSchedulesRun},true,true,true},
  {"admin",[]string{identity.PermissionProjectAdmin},true,true,true},
  {"settings editor cannot run",[]string{identity.PermissionSettingsWrite},false,false,false},
  {"tasks runner cannot run schedules",[]string{identity.PermissionTasksRun},false,false,false},
  {"missing grant",nil,false,false,false},
 }
 for _,tc:=range tests{
  t.Run(tc.name,func(t *testing.T){
   req:=schedulePermissionRequest(tc.grants...)
   for _,op:=range []struct{grant string;allow bool}{
    {identity.PermissionSchedulesView,tc.view},
    {identity.PermissionSchedulesRun,tc.run},
    {identity.PermissionSchedulesManage,tc.manage},
   }{
    if got:=schedulePermissionAllowed(req,op.grant);got!=op.allow{
     t.Errorf("permission %s granted=%v, expected=%v",op.grant,got,op.allow)
    }
    recorder:=httptest.NewRecorder()
    ok:=scheduleRequirePermission(recorder,req,op.grant)
    if ok!=op.allow{t.Errorf("handler %s allowed=%v",op.grant,ok)}
    if !op.allow&&recorder.Code!=http.StatusForbidden{t.Errorf("denied %s status=%d",op.grant,recorder.Code)}
   }
  })
 }
}

 
// View-only access is not a data-export permission. Never serialize command,
// webhook, matching secrets, process output, or diagnostic errors to viewers.
func TestSchedulePublicSummaryNeverLeaksSensitiveFields(t *testing.T) {
 canary:="TOP_SECRET_CANARY"
 store:=scheduleStore{
  Schedules:[]scheduleDefinition{{
   ID:"schedule-1",Name:"Safe label",Type:"multi",Enabled:true,
   Command:canary,URL:"https://example.invalid/"+canary,
   MatchText:canary,NotifyWebhookURL:canary,
  }},
  Runs:[]scheduleRun{{
   ID:"run-1",ScheduleID:"schedule-1",Name:"Safe label",
   Outputs:map[string]string{"TOKEN":canary},Error:canary,
  }},
 }
 result:=schedulePublicSummary(store)
 data,err:=json.Marshal(result)
 if err!=nil{t.Fatal(err)}
 if strings.Contains(string(data),canary)||strings.Contains(string(data),"run-1")||
  strings.Contains(string(data),"command")||strings.Contains(string(data),"notify_webhook_url"){
  t.Fatalf("schedule viewer response leaked protected content: %s",data)
 }
 if !strings.Contains(string(data),`"redacted":true`)||
  !strings.Contains(string(data),`"id":"schedule-1"`){
  t.Fatalf("viewer response lost safe metadata: %s",data)
 }
}

func TestScheduleSensitiveEndpointsRequirePairedGrants(t *testing.T) {
 for _,tc:=range []struct {
  name string
  grants []string
  elevated bool
 }{
  {"no grants",nil,false},
  {"view only",[]string{identity.PermissionSchedulesView},false},
  {"run only",[]string{identity.PermissionSchedulesRun},false},
  {"manage only",[]string{identity.PermissionSchedulesManage},false},
  {"paired run/manage",[]string{identity.PermissionSchedulesRun,identity.PermissionSchedulesManage},true},
  {"administrator",[]string{identity.PermissionProjectAdmin},true},
 }{
  t.Run(tc.name,func(t *testing.T){
   req:=schedulePermissionRequest(tc.grants...)
   if got:=scheduleElevatedAllowed(req);got!=tc.elevated{
    t.Fatalf("elevated=%v want %v",got,tc.elevated)
   }
   for _,permission:=range []string{identity.PermissionSchedulesRun,identity.PermissionSchedulesManage}{
    w:=httptest.NewRecorder()
    if granted:=scheduleRequirePermission(w,req,permission);granted!=tc.elevated{
     t.Fatalf("permission %s: allowed=%v",permission,granted)
    }
    if !tc.elevated&&w.Code!=http.StatusForbidden{t.Fatalf("%s status=%d",permission,w.Code)}
   }
  })
 }
}
