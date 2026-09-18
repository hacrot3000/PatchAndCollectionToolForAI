package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSplitTerminalFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"let groups=[]",
		"split-vertical",
		"split-horizontal",
		"session-split-vertical",
		"session-split-horizontal",
		"session-merge-vertical",
		"session-merge-horizontal",
		"session-unsplit",
		"function groupFor(id)",
		"function createGroup(first,second,orientation",
		"removeGroupsContaining([first,second])",
		"function mergeWith(view,orientation)",
		"function syncForActive()",
		"cleanupPresentation();updateButtons()",
		"await app.startTerminal()",
		"pointermove",
		"sessionStorage.setItem",
		"MutationObserver",
		"function restoreProjectGroups",
		"getGroups",
		"taskmenu:split-changed",
		"orientation==='horizontal'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("split.js missing behavior %q", want)
		}
	}
	if strings.Contains(js, "let pair=null") {
		t.Fatal("split implementation must not regress to one global pair")
	}
	if strings.Contains(js, "else clearSplit();") || strings.Contains(js, "else clearAll();") {
		t.Fatal("activating an unrelated task/tab must suspend split presentation, not destroy saved split groups")
	}
	if strings.Contains(js, "observe(document.body") {
		t.Fatal("split feature must not observe the whole document body")
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/split.js';") {
		t.Fatal("next.js must load split.js")
	}
}


func TestSplitSuspendsForExternalEditorViewWithoutDestroyingGroups(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"window.addEventListener('taskmenu:view-activated'",
		"event.detail?.kind==='external'",
		"cleanupPresentation();updateButtons();return",
		"event.detail?.kind==='terminal'",
		"setTimeout(syncForActive,0)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("split external-view integration missing %q", want)
		}
	}
}
