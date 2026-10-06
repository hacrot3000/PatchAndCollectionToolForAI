package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestFileWatcherUsesBoundedMetadataPolling(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filewatcher.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const FILE_POLL_MS=1500",
		"const DIRECTORY_POLL_MS=2500",
		"const MAX_WATCHED_DIRECTORIES=30",
		"'/api/project/file?meta=1&path='",
		"if(!enabled||document.hidden)return",
		"editor.editors?.values?.()",
		"explorer.expandedPaths",
		"['',...(explorer.expandedPaths||[])].slice(0,MAX_WATCHED_DIRECTORIES)",
		"TaskMenuFileWatcher",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filewatcher.js missing bounded watcher contract %q", want)
		}
	}
	if strings.Contains(js, "/api/project/files/search") {
		t.Fatal("File watcher must not crawl/search the whole project")
	}
}

func TestFileWatcherExternalChangeAndGeneratedFileContract(t *testing.T) {
	watcherData, err := webassets.Files.ReadFile("featuremods/filewatcher.js")
	if err != nil {
		t.Fatal(err)
	}
	watcher := string(watcherData)
	for _, want := range []string{
		"await editor.handleExternalFileChange?.(view,latest)",
		"taskmenu:project-directory-changed",
		"generated:added",
		"await explorer.refreshWatchedDirectory?.(path)",
		"taskmenu:editor-external-missing",
	} {
		if !strings.Contains(watcher, want) {
			t.Fatalf("file watcher change contract missing %q", want)
		}
	}

	editorData, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	editor := string(editorData)
	for _, want := range []string{
		"async function handleExternalFileChange(view,latest=null)",
		"if(!view.dirty)",
		"setEditorDocument(view,latest)",
		"const choice=await conflictChoice(view)",
		"showConflictCompare(view,latest)",
		"putEditorFile(view,latest.sha256)",
		"handleExternalFileChange,",
	} {
		if !strings.Contains(editor, want) {
			t.Fatalf("editor external-change contract missing %q", want)
		}
	}

	explorerData, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	explorer := string(explorerData)
	for _, want := range []string{
		"async function refreshWatchedDirectory(pathValue)",
		"await loadDirectory(pathValue,true)",
		"get expandedPaths(){return [...expanded];}",
	} {
		if !strings.Contains(explorer, want) {
			t.Fatalf("Explorer watcher bridge missing %q", want)
		}
	}
}

func TestFileWatcherLoadsAfterEditorAndExplorer(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	explorer := strings.Index(js, "explorer.js")
	editor := strings.Index(js, "editor.js")
	watcher := strings.Index(js, "filewatcher.js")
	if explorer < 0 || editor < 0 || watcher < 0 || watcher < explorer || watcher < editor {
		t.Fatalf("filewatcher.js must load after Explorer and Editor: %q", js)
	}
}
