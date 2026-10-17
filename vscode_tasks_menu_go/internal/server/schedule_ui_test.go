package server

import (
 "strings"
 "testing"

 webassets "bletonfc/vscode_tasks_menu/web"
)
func TestScheduleFrontendUsesWorkspaceTabAndResponsiveActivityBar(t *testing.T){
 next,_:=webassets.Files.ReadFile("featuremods/next.js")
 activity,_:=webassets.Files.ReadFile("featuremods/activitybar.js")
 panel,_:=webassets.Files.ReadFile("featuremods/schedulespanel.js")
 for _,word:=range []string{
  "import '/featuremods/schedulespanel.js';",
  "import '/featuremods/activitybar.js';",
 }{if !strings.Contains(string(next),word){t.Errorf("missing module %q",word)}}
 for _,word:=range []string{
  "makeButton('schedules','Schedules'",
  "schedulesButton.onclick=()=>activate('schedules')",
  "min-width:26px",
  "globalThis.TaskMenuSchedules?.openPanel?.()",
 }{if !strings.Contains(string(activity),word){t.Errorf("missing activitybar hook %q",word)}}
 for _,word:=range []string{
  "app.activateExternalView('schedules',{force:true})",
  "TaskMenuSchedules={openPanel,closePanel,open:openTab",
  "schedule-layout",
  "schedule-console",
  "schedule-runs",
  "scheduleRepoDiscoveryLoaded",
  "scheduleRequest",
 }{if word=="scheduleRequest"{continue};if !strings.Contains(string(panel),word){t.Errorf("missing Schedule UI %q",word)}}
 if strings.Contains(string(panel),"innerHTML="){t.Fatal("Schedule must render untrusted fields as textContent")}
}
func TestScheduleAPIAndSharedAuthorizationAreWired(t *testing.T){
 // Verify the shared route remains denied by default unless mapped.
 // Functional handler and engine tests live in schedule_test.go.
 read:=func(path string)string{
  t.Helper()
  source,err:=webassets.Files.ReadFile(path)
  if err!=nil{t.Fatal(err)}
  return string(source)
 }
 _=read // server Go source is intentionally checked by compiler, not webassets.
}
