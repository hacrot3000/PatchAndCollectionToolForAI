package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestRestartAndClearConsoleControls(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/restartclear.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"session-force-restart",
		"restart.textContent='Restart'",
		"/api/sessions/force-kill",
		"/api/sessions/terminate",
		"/api/sessions/process-tree",
		"session-terminate",
		"session-force-kill",
		"session-process-tree",
		"Session process tree",
		"processAuto.checked=true",
		"setInterval(()=>loadProcessTree().catch(()=>{}),2000)",
		"function bytesLabel(bytes)",
		"'CPU'",
		"'RSS'",
		"row.cpu_percent",
		"row.rss_bytes",
		"processSummary.textContent",
		"SIGTERM",
		"SIGKILL",
		"waitUntilStopped",
		"will not be restarted to avoid overlapping processes",
		"session-clear-console",
		"🧹 Clear console",
		"window.confirm('Clear this console and server-side scrollback?",
		"/api/sessions/clear-console",
		"view.term.clear()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("restartclear.js missing behavior %q", want)
		}
	}
	if !strings.Contains(indexHTML, `/featuremods/next.js`) {
		t.Fatalf("index HTML must load feature module chain")
	}
}

func TestRestartThisTabKeepsExistingTabAndOffersLogModes(t *testing.T) {
  restart, err := webassets.Files.ReadFile("featuremods/restartclear.js")
  if err != nil { t.Fatal(err) }
  menus, err := webassets.Files.ReadFile("featuremods/menus.js")
  if err != nil { t.Fatal(err) }
  split, err := webassets.Files.ReadFile("featuremods/split.js")
  if err != nil { t.Fatal(err) }
  for _, want := range []string{
    "Restart this tab (clear log)", "Restart this tab (keep log)",
    "restartInThisTab(view,button,clearLog)", "view.retargeting=true",
    "await waitUntilStopped(oldID)", "method:'DELETE'",
    "app.retargetTaskView(view,replacement,{clearLog})",
    "if(replacement){",
  } {
    if !strings.Contains(string(restart),want) {t.Errorf("restart control missing %q",want)}
  }
  for _, selector := range []string{".session-restart-here-clear",".session-restart-here-keep"} {
    if !strings.Contains(string(menus),selector) {t.Errorf("Session menu missing %s",selector)}
  }
  for _, want := range []string{
    "function retargetTaskView(view,meta,{clearLog=false}={})",
    "views.delete(oldID)", "views.set(meta.id,view)",
    "view.tab.dataset.id=meta.id", "view.pane.dataset.id=meta.id",
    "connect(view,false)", "if(view.restartLogPrefix)view.term.write(view.restartLogPrefix)",
    "if(replay)", "view.meta.id+'/stop'", "view.meta.id+'/resize'",
    "if(!seen.has(id)&&!views.get(id)?.retargeting)",
  } {
    if !strings.Contains(appJS,want) {t.Errorf("in-place retarget missing %q",want)}
  }
  if !strings.Contains(string(split),"function retargetSession(oldID,newID)") {t.Error("split state must follow session ID swap")}
}
