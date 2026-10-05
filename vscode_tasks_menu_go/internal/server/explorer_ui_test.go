package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestExplorerLoadsDirectoriesLazily(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/project/tree?path=",
		"async function toggleDirectory(pathValue)",
		"if(!loaded.has(pathValue))",
		"taskmenu:project-file-open-request",
		"localStorage.setItem(storageKey()",
		"loaded=new Map()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("explorer.js missing %q", want)
		}
	}
	if strings.Contains(js, "recursive") {
		t.Fatal("Explorer must not request recursive project trees")
	}
}

func TestExplorerModuleLoadsBeforeMenus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	explorer := strings.Index(js, "explorer.js")
	menus := strings.Index(js, "menus.js")
	if explorer < 0 || menus < 0 || explorer > menus {
		t.Fatalf("Explorer must load before menus")
	}
}


func TestExplorerRestoresNestedExpandedDirectories(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function restoreExpandedDirectories()",
		"a.split('/').length-b.split('/').length",
		"await loadDirectory(pathValue)",
		"expanded.delete(pathValue)",
		"await restoreExpandedDirectories()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("explorer nested restore missing %q", want)
		}
	}
	if strings.Contains(js, "if(pathValue.includes('/'))continue") {
		t.Fatal("Explorer restore must not skip nested expanded directories")
	}
}


func TestExplorerTracksSharedHeaderHeight(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	if !strings.Contains(js, ".project-explorer{display:none;position:fixed;top:var(--taskmenu-header-height,30px)") {
		t.Fatal("Explorer must track the shared compact TaskDeck header height")
	}
}

func TestExplorerSupportsSelectionFavoritesAndRecentPaths(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const selected=new Set()",
		"const favorites=new Set()",
		"let recent=[]",
		"function selectPath(event,pathValue)",
		"event.shiftKey",
		"event.ctrlKey||event.metaKey",
		"function rememberRecent(pathValue)",
		"function renderSaved()",
		"Open containing folder",
		"Pin selected",
		"Unpin selected",
		"reveal:revealPath",
		"get selectedPaths()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Explorer selection/favorites/recent support missing %q", want)
		}
	}
}

func TestExplorerRevealsFilesOpenedByEditor(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskmenu:project-file-opened",
		"rememberRecent(pathValue)",
		"panel.classList.contains('visible')",
		"revealPath(pathValue).catch(app.showError)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("explorer editor integration missing %q", want)
		}
	}
}

func TestExplorerCreatesRenamesAndMovesProjectItems(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/project/mutate",
		"async function createProjectItem(type)",
		"async function renameProjectItem(pathValue)",
		"async function moveSelectedProjectItems()",
		"function topLevelSelectedPaths(paths)",
		"body:JSON.stringify({action,path:pathValue,new_path:newPath,token})",
		"create_file",
		"Rename…",
		"Move selected…",
		"New file here…",
		"taskmenu:project-path-renamed",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Explorer project mutation UI missing %q", want)
		}
	}
}

func TestExplorerClipboardAndDuplicateActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"let fileClipboard={mode:'',paths:[]}",
		"function setProjectClipboard(mode,paths)",
		"async function duplicateSelectedProjectItems()",
		"async function pasteProjectClipboard(destinationDir)",
		"projectMutation('copy',source,target)",
		"projectMutation('rename',source,target)",
		"Copy selected",
		"Cut selected",
		"Duplicate selected",
		"Paste ",
		"get clipboard()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Explorer clipboard/duplicate support missing %q", want)
		}
	}
}

func TestExplorerTrashAndUndoActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"let lastUndo=null",
		"function recordUndo(label,steps)",
		"async function trashPaths(paths",
		"async function undoLastOperation()",
		"projectMutation('trash',source)",
		"projectMutation('restore',step.path,'',step.token||'')",
		"Save or close unsaved editor",
		"taskmenu:project-path-trashed",
		"Move selected to Trash",
		"undoButton.onclick",
		"undo:undoLastOperation",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Explorer trash/undo support missing %q", want)
		}
	}
}
