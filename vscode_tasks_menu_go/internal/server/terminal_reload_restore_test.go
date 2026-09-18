package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalReloadRestoresSavedOrderAndSplits(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"layoutIDsFromSaved(saved,existing)",
		"normalizedCwd",
		"normalizedCwd(meta.cwd)===wantedCwd",
		"if(item)return ''",
		"session_id",
		"applySavedLayout(saved,ids",
		"restoreProjectGroups",
		"await app.syncSessions?.()",
		"app.attachSession?.(meta,false)",
		"applySavedLayout(restored,ids",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalrestore.js missing reload/self-update restore behavior %q", want)
		}
	}
	if strings.Contains(js, "if(existing.length){\n      const ids=existing.map") {
		t.Fatal("live terminal startup must not return before applying saved layout")
	}
}

func TestCoreUIExposesImmediateSessionSyncForRestore(t *testing.T) {
	for _, want := range []string{"syncSessions,attachSession:attach", "async function syncSessions()", "function attach(meta,activate)"} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing terminal restore hook %q", want)
		}
	}
}


func TestTerminalReloadCanRemapChangedSessionIDsByCwdWithoutWrongSplitFallback(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const wantedCwd=normalizedCwd(item?.cwd)",
		"const liveCwd=normalizedCwd(app.views.get(id)?.meta?.cwd)",
		"normalizedCwd(meta.cwd)===wantedCwd",
		"if(item)return ''",
		"const reserved=new Set(used);if(first)reserved.add(first)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalrestore.js missing safe CWD remap behavior %q", want)
		}
	}
}
