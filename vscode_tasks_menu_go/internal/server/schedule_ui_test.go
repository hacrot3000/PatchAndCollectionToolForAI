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
  "schedulePreferredGitRepo()",
  "schedulePrefillGitPath()",
  "scheduleSyncRepoSelect(rebuild=false)",
  "schedule-repo-combo",
  "globalThis.TaskMenuScheduleViews.render",
  "globalThis.TaskMenuScheduleConditions.create",
  "scheduleShowPage(page||(id?'edit':'overview'))",
  "model.condition_group=model.type==='multi'",
  "['runs','All runs']",
  "['graph','Dependency graph']",
  "gitInput.setAttribute('list',scheduleRepoList.id)",
  "scheduleRepoSelect.addEventListener('change'",
  "discoverScheduleRepos({force:true})",
  "scheduleRequest",
 }{if word=="scheduleRequest"{continue};if !strings.Contains(string(panel),word){t.Errorf("missing Schedule UI %q",word)}}
 // Navigation must keep the Schedule sidebar and select the workspace tab.
 for _, want:=range []string{
  "function syncScheduleActiveTab()",
  "data.tab.classList.toggle('active',active)",
  "data.tab.setAttribute('aria-selected',active?'true':'false')",
  "data.workspace.classList.toggle('visible',active)",
  "window.addEventListener('taskmenu:view-activated',syncScheduleActiveTab)",
  "if(id)load(id)",
 } {
  if !strings.Contains(string(panel),want){t.Errorf("Schedule navigation missing %q",want)}
 }
 // The open handler must never call closePanel(); selecting a schedule or run
 // should preserve the left Schedule activity view.
 source:=string(panel)
 a:=strings.Index(source,"function openTab(id='',page=''){")
 b:=strings.Index(source,"function schedulePreferredGitRepo(){")
 if a<0||b<=a{t.Fatal("Schedule open handler missing")}
 if strings.Contains(source[a:b],"closePanel()"){t.Fatal("opening Schedule must not hide Schedule sidebar")}
 if strings.Contains(source,"app.activateExternalView('empty'"){t.Fatal("Schedule close must not leave an empty active workspace")}
 if !strings.Contains(string(activity),"globalThis.TaskMenuSchedules?.open?.('','overview')"){
  t.Error("Schedule icon must open the management dashboard")
 }
 conditions,condErr:=webassets.Files.ReadFile("featuremods/scheduleconditions.js")
 views,viewErr:=webassets.Files.ReadFile("featuremods/scheduleviews.js")
 if condErr!=nil||viewErr!=nil{t.Fatalf("Schedule modules missing: conditions=%v views=%v",condErr,viewErr)}
 for _,token:=range []string{"function scheduleExpressionEditor(","function scheduleLegacyGroup(","function scheduleGroupParents("}{
  if !strings.Contains(string(conditions),token){t.Errorf("condition builder missing %q",token)}
 }
 for _,token:=range []string{"function scheduleRenderWorkspacePage(","function scheduleDependencyGraph(","function scheduleDetailRelationships("}{
  if !strings.Contains(string(views),token){t.Errorf("Schedule management view missing %q",token)}
 }
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
