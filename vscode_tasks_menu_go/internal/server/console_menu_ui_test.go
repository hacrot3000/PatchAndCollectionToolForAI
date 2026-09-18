package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func mustEmbeddedAsset(t *testing.T, path string) string {
	t.Helper()
	data, err := webassets.Files.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestConsoleMenuOrderIconsColorsAndClearConfirmation(t *testing.T) {
	menus := mustEmbeddedAsset(t, "featuremods/menus.js")
	for _, want := range []string{
		"['CONSOLE',['.copy-console','.console-search-btn','.console-save-btn','.session-clear-console']]",
		".copy-console{background:",
		".console-search-btn{background:",
		".console-save-btn{background:",
		".session-clear-console{background:",
	} {
		if !strings.Contains(menus, want) {
			t.Fatalf("menus.js missing console menu behavior %q", want)
		}
	}

	all := mustEmbeddedAsset(t, "featuremods/all.js")
	for _, want := range []string{"🔎 Find in console", "💾 Save console log"} {
		if !strings.Contains(all, want) {
			t.Fatalf("all.js missing console action %q", want)
		}
	}

	clear := mustEmbeddedAsset(t, "featuremods/restartclear.js")
	for _, want := range []string{
		"🧹 Clear console",
		"window.confirm('Clear this console and server-side scrollback? This cannot be undone. The running process will not be stopped.')",
	} {
		if !strings.Contains(clear, want) {
			t.Fatalf("restartclear.js missing destructive clear guard %q", want)
		}
	}

	if !strings.Contains(appJS, "📋 Copy console") {
		t.Fatal("core UI must give Copy console a visible icon")
	}
}


func TestConsoleFindShortcutDoesNotStealProjectSearchShortcut(t *testing.T) {
	all := mustEmbeddedAsset(t, "featuremods/all.js")
	for _, want := range []string{
		"e.shiftKey||e.altKey||e.key.toLowerCase()!=='f'",
		"showConsoleFind(view)",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("all.js missing console/project-search shortcut separation %q", want)
		}
	}
}
