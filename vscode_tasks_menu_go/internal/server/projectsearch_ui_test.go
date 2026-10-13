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
		"editor.focusProjectSearchResult?.(view,",
		"renderPreviewMatches(preview,item)",
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
 
func TestProjectSearchDocksToRightAndHighlightsExactEditorMatches(t *testing.T) {
	searchData, err := webassets.Files.ReadFile("featuremods/projectsearch.js")
	if err != nil { t.Fatal(err) }
	js := string(searchData)
	for _, want := range []string{
		"project-search-panel{display:none;position:fixed;right:0",
		"body.project-search-docked>main>section{width:calc(100% -",
		".project-search-panel.visible{display:flex}",
		".project-search-hit{background:",
		".project-search-result.selected .project-search-hit{background:",
		"function renderPreviewMatches(preview,item)",
		"mark.textContent=match[0]",
		"panel.setAttribute('role','complementary')",
		"panel.append(panelTitle,top,statusRow,results,replacePreview)",
		"document.body.classList.add('project-search-docked')",
		"document.body.classList.remove('project-search-docked')",
		"function refreshDockLayout()",
		"editor.openFile(item.path)",
		"editor.focusProjectSearchResult?.(view,",
		"matchLength:item.match_length",
		"matches:items.filter(hit=>hit.path===item.path)",
		"function globalProjectSearchShortcut(event)",
		"prevButton.onclick=",
		"nextButton.onclick=",
		"panelClose.onclick=close",
		"globalThis.TaskMenuEditor?.clearProjectSearchHighlights?.()",
	} {
		if !strings.Contains(js, want) { t.Fatalf("project search right dock contract missing %q", want) }
	}
	if strings.Contains(js, "project-search-backdrop") || strings.Contains(js, "project-search-dialog") {
		t.Fatal("project search must not open a blocking backdrop or dialog")
	}

	editorData, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	editorJS := string(editorData)
	for _, want := range []string{
		"function projectSearchResultRange(doc,item)",
		"doc.sliceString(from,to)!==text",
		"function focusProjectSearchResult(view,",
		"function clearProjectSearchHighlights()",
		"findState.projectSearchActive",
		"findState.projectRanges",
		"state.activeFrom=range.from;state.activeTo=range.to",
		"view.cm.dispatch({selection:{anchor:range.from,head:range.to},scrollIntoView:true})",
		"view.cm.focus()",
		"if(update.docChanged&&findState.projectSearchActive)",
		"focusProjectSearchResult,",
		"clearProjectSearchHighlights,",
	} {
		if !strings.Contains(editorJS, want) { t.Fatalf("project search editor integration missing %q", want) }
	}
}
