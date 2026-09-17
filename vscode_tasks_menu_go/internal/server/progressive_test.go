package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func progressiveJS(t *testing.T) string {
	t.Helper()
	data, err := webassets.Files.ReadFile("featuremods/all.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestProgressiveFeaturesFavoritesAndRecent(t *testing.T) {
	js := progressiveJS(t)
	for _, want := range []string{
		"★ FAVORITES",
		"↻ RECENT",
		"task-favorite-toggle",
		"/api/state/tasks",
		"queueProjectStateSave()",
		"taskmenu:session",
		"recordRecent(meta.task_id)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("featuremods/all.js missing favorites/recent behavior %q", want)
		}
	}
	if strings.Contains(js, "localStorage.setItem(storageKey(") {
		t.Fatalf("favorites/recent must not persist through legacy localStorage")
	}
	if !strings.Contains(indexHTML, `/featuremods/all.js`) {
		t.Fatalf("index HTML must load progressive feature module")
	}
}

func TestProgressiveFeaturesRerunHistoryAndStatus(t *testing.T) {
	js := progressiveJS(t)
	for _, want := range []string{
		"button.textContent=view.meta.status==='running'?'Restart':'Run again'",
		"function restartOrRun(view)",
		"HISTORY",
		"recordHistory(meta)",
		"projectState.history=normalizeHistory(items).slice(0,10)",
		"state-success",
		"state-fail",
		"formatDuration",
		"setInterval(()=>{for(const view of app.views.values())",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("featuremods/all.js missing rerun/history/status behavior %q", want)
		}
	}
}

func TestProgressiveFeaturesConsoleFindAndSaveLog(t *testing.T) {
	js := progressiveJS(t)
	for _, want := range []string{
		"Find in console…",
		"function findInConsole",
		"term.scrollToLine",
		"term.select",
		"💾 Save console log",
		"new Blob([text",
		"safeLogName(view)",
		"e.key.toLowerCase()!=='f'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("featuremods/all.js missing console find/save behavior %q", want)
		}
	}
}
