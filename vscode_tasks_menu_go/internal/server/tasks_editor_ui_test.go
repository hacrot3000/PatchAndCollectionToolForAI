package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTasksJSONVisualEditorFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const TASKS_PATH='.vscode/tasks.json'",
		"button.id='tasks-json-editor'",
		"button.textContent='Edit tasks.json…'",
		"Visual",
		"Raw JSON",
		"function parseTasksDocument(text)",
		"function applyTemplate(task,id)",
		"Bash script",
		"Python script",
		"Node.js script",
		"Go · test all",
		"Rust · cargo run",
		"CMake · build",
		"Gradle · build",
		"Maven · test",
		"Docker Compose · up",
		"function pickWorkspacePath(title)",
		"browser?.pickFile",
		"Choose command / executable",
		"+ File argument…",
		"MULTI-FILE / SCRIPT EXECUTION",
		"+ Add execution file",
		"executionFiles",
		"commands.join(meta.executionMode==='continue'?'; ':' && ')",
		"/api/project/file?path=",
		"expected_sha256:fileMeta.sha256",
		"method:'PUT'",
		"await app.loadTasks()",
		"Ctrl",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("taskseditor.js missing visual tasks editor behavior %q", want)
		}
	}
}

func TestTasksEditorLoadsBeforeGroupedSettingsMenu(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	editor := strings.Index(js, "import '/featuremods/taskseditor.js';")
	menus := strings.Index(js, "import '/featuremods/menus.js';")
	if editor < 0 || menus < 0 || editor > menus {
		t.Fatal("taskseditor.js must load before menus.js so Settings can adopt the editor button")
	}
}

func TestTasksEditorUsesExistingProjectFileMutationAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	if strings.Contains(js, "/api/tasks/config") || strings.Contains(js, "/api/tasks/editor") {
		t.Fatal("tasks editor should reuse the hardened project-file read/write API instead of adding a second tasks mutation path")
	}
	for _, want := range []string{
		"app.hasPermission?.('files.read')",
		"app.hasPermission?.('files.write')",
		"expected_sha256:fileMeta.sha256",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tasks editor missing project-file safety behavior %q", want)
		}
	}
}
