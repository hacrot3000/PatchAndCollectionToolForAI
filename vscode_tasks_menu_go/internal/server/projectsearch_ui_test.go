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
		"/api/project/content/search?",
		"limit:'200'",
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
		"addSection(files.pop,'OPEN',[quickOpen,searchFiles,symbols,explorer])",
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

func TestProjectSearchReplaceUIFiltersPreviewAndUndo(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectsearch.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Search and replace in project",
		"Use regular expression",
		"Match case",
		"Match whole word",
		"Include glob",
		"Exclude glob",
		"Folder scope:",
		"function groupedItems()",
		"Replace file",
		"async function replaceOneMatch(item)",
		"async function previewReplaceAll()",
		"function showReplaceAllPreview(preview)",
		"Apply Replace All",
		"/api/project/content/replace",
		"action:'preview'",
		"action:'apply'",
		"action:'undo'",
		"taskdeck:project-replace-undo:",
		"Save or close unsaved editor",
		"await reloadOpenEditors",
		"TaskMenuExplorer?.refreshGitStatus?.()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("project Search/Replace UI missing %q", want)
		}
	}
	if strings.Index(js, "showReplaceAllPreview(preview)") > strings.Index(js, "async function applyReplaceAll()") {
		t.Fatal("Replace All preview must be implemented before the apply action")
	}
}

func TestExplorerCanScopeProjectSearchToFolder(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"appendSharedProjectActions(pathValue,type)",
		"TaskMenuProjectFileActions?.standardActions?.(pathValue,type)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Explorer scoped project search missing %q", want)
		}
	}
}
