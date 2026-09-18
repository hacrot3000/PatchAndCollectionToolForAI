package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestEditorUsesVendoredCodeMirrorAndStaysOutsideTerminalSessions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"globalThis.cm6?.load",
		"cmFactory.newEditor",
		"dataset.viewKind='editor'",
		"app.activateExternalView(id)",
		"/api/project/file?path=",
		"taskmenu:project-file-open-request",
		"contenteditable",
		"editor-tab",
		"editor-pane",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor.js missing %q", want)
		}
	}
	if strings.Contains(js, "app.views.set(") || strings.Contains(js, "/api/sessions") {
		t.Fatal("Editor must not register with terminal session broker")
	}
}

func TestEditorInitialLanguageCoverageMapping(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"options.cpp=true",
		"options.go=true",
		"options.python=true",
		"options.javascript=true",
		"options.json=true",
		"options.yaml=true",
		"options.markdown=true",
		"options.html=true",
		"options.css=true",
		"options.xml=true",
		"options.java=true",
		"CMake (plain)",
		"Shell (plain)",
		"Lua (plain)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor language mapping missing %q", want)
		}
	}
}


func TestEditorDirtySaveShortcutsAndCloseFlow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function setDirty(view,dirty)",
		"view.tab.classList.toggle('dirty',view.dirty)",
		"method:'PUT'",
		"expected_sha256:view.file.sha256",
		"content:view.cm.state.doc.toString()",
		"Save changes?",
		"Discard",
		"Cancel",
		"function closeEditor(id)",
		"function goToLine(view)",
		"event.key==='Tab'",
		"replaceSelection('\\t')",
		"key==='s'",
		"key==='g'",
		"Discard unsaved changes and reload",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor dirty/save flow missing %q", want)
		}
	}
	if strings.Contains(js, "localStorage.setItem") && strings.Contains(js, "view.cm.state.doc.toString()") {
		t.Fatal("dirty editor contents must not be persisted to localStorage")
	}
}


func TestEditorExternalConflictResolutionFlow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"response.status===409",
		"File changed outside the editor.",
		"compare.textContent='Compare'",
		"reload.textContent='Reload'",
		"overwrite.textContent='Overwrite'",
		"cancel.textContent='Cancel'",
		"function showConflictCompare(view,latest)",
		"pre.textContent=text",
		"putEditorFile(view,latest.sha256)",
		"if(result.conflict)continue",
		"setEditorDocument(view,latest)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor conflict flow missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("editor conflict/source UI must not render source through innerHTML")
	}
}
