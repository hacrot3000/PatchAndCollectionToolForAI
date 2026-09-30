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
		"const groups=[];const introduced=new Set()",
		"const pushGroup=(first,second,ratio,orientation)=>",
		"if(!first||!second||first===second||introduced.has(second))return",
		"introduced.add(first);introduced.add(second)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalrestore.js missing safe CWD remap behavior %q", want)
		}
	}
}


func TestCoreUIRestoresSSHMinusOneTerminalButKeepsPatchHeadless(t *testing.T) {
	for _, want := range []string{
		"function autoAttachSession(meta)",
		"function isHeadlessPatchSession(meta)",
		"if(Number(meta?.task_id)!==-1)return false;",
		"if(meta?.kind==='patch')return true;",
		"return String(meta?.label||'').startsWith('Patch Tool · ');",
		"return !isHeadlessPatchSession(meta);",
		"else if(autoAttachSession(meta))attach(meta,false);",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing SSH reload restore behavior %q", want)
		}
	}
	if strings.Contains(appJS, "return Number(meta?.task_id)!==-1;") {
		t.Fatal("task_id=-1 alone must not hide SSH terminal sessions from reload restore")
	}
}
