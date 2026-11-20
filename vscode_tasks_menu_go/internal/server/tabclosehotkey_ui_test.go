package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestActiveTabCloseShortcutUsesExistingCloseControl(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabclosehotkey.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function isCloseTabShortcut(event)",
		"const primary=event.ctrlKey||event.metaKey",
		"primary&&!event.shiftKey&&String(event.key||'').toLowerCase()==='w'",
		"function activeWorkspaceTab()",
		"app.active",
		"tab.classList.contains('active')",
		"function activeTabCloseControl()",
		"child.classList.contains('close')",
		"function closeActiveToolTab()",
		"close.click()",
		"function closeToolTabShortcut(event)",
		"event.repeat||event.defaultPrevented",
		"if(!activeTabCloseControl())return",
		"event.preventDefault()",
		"event.stopPropagation()",
		"window.addEventListener('keydown',closeToolTabShortcut,true)",
		"shortcut:'Ctrl+Alt+W'",
		"browserShortcut:'Ctrl+W (only when the browser forwards it)'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("close-tab shortcut contract missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"window.close(", "window.open(", "location.reload(",
		"tab.remove()", "view.pane.remove()", "views.delete(",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("close-tab shortcut must delegate to existing close control, found %q", forbidden)
		}
	}
}

func TestActiveTabCloseShortcutLoadedOnStartup(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	if strings.Count(js, "import '/featuremods/tabclosehotkey.js';") != 1 {
		t.Fatal("the close-tab shortcut module must load exactly once")
	}
	if strings.Index(js, "import '/featuremods/terminalhotkey.js';") > strings.Index(js, "import '/featuremods/tabclosehotkey.js';") {
		t.Fatal("close-tab shortcut should load next to existing terminal shortcuts")
	}
}
