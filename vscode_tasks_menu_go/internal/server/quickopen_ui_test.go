package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestQuickOpenUsesBackendBoundedSearchAndKeyboardNavigation(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/quickopen.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/project/files/search?q=",
		"&limit=50",
		"AbortController",
		"setTimeout(()=>search(query,currentSeq),80)",
		"ArrowDown",
		"ArrowUp",
		"event.key==='Enter'",
		"globalQuickOpenShortcut",
		"taskmenu:project-file-open-request",
		"data.results.slice(0,50)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("quickopen.js missing %q", want)
		}
	}
	if strings.Contains(js, "/api/project/tree") {
		t.Fatal("Quick Open must not fetch the project tree")
	}
}

func TestQuickOpenIsLoadedBeforeGroupedMenus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	quick := strings.Index(js, "quickopen.js")
	menus := strings.Index(js, "menus.js")
	if quick < 0 || menus < 0 || quick > menus {
		t.Fatalf("next.js must load quickopen before menus: %q", js)
	}
}


func TestQuickOpenShortcutIsGlobalCaptureAndExact(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/quickopen.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function globalQuickOpenShortcut(event)",
		"const primary=event.ctrlKey||event.metaKey",
		"!primary||event.shiftKey||event.altKey||event.key.toLowerCase()!=='p'",
		"event.preventDefault()",
		"event.stopPropagation()",
		"window.addEventListener('keydown',globalQuickOpenShortcut,true)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Quick Open global shortcut contract missing %q", want)
		}
	}
}
