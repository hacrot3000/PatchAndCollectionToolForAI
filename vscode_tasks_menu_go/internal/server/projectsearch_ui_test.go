package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestProjectSearchUIUsesBoundedCancelableBackendSearch(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectsearch.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/project/content/search?q=",
		"&limit=100",
		"new AbortController()",
		"controller.abort()",
		"globalProjectSearchShortcut",
		"globalThis.TaskMenuEditor",
		"editor.openFile(item.path)",
		"scrollIntoView:true",
		"preview.textContent=item.preview||''",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("project search UI missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("project search results must not render source through innerHTML")
	}
}

func TestProjectSearchFeatureIsLoadedAndExposedInFilesMenu(t *testing.T) {
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "import '/featuremods/projectsearch.js';") {
		t.Fatal("project search feature is not loaded")
	}
	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(menus)
	for _, want := range []string{
		"Search in Files…  Ctrl+Shift+F",
		"globalThis.TaskMenuProjectSearch?.open()",
		"addSection(files.pop,'OPEN',[quickOpen,searchFiles,explorer])",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Files menu missing project search contract %q", want)
		}
	}
}


func TestProjectSearchShortcutIsGlobalCaptureAndExact(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectsearch.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function globalProjectSearchShortcut(event)",
		"const primary=event.ctrlKey||event.metaKey",
		"!primary||!event.shiftKey||event.altKey||event.key.toLowerCase()!=='f'",
		"event.preventDefault()",
		"event.stopPropagation()",
		"window.addEventListener('keydown',globalProjectSearchShortcut,true)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Project Search global shortcut contract missing %q", want)
		}
	}
}
