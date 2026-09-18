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
