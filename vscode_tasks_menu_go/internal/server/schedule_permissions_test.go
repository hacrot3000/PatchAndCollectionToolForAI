package server

import (
 "context"
 "net/http"
 "net/http/httptest"
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
  {"runner",[]string{identity.PermissionSchedulesRun},true,true,false},
  {"manager",[]string{identity.PermissionSchedulesManage},true,false,true},
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
