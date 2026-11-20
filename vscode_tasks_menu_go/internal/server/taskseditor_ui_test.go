package server

import (
	"regexp"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTasksEditorHasUniqueTopLevelFunctionDeclarations(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	re := regexp.MustCompile(`(?m)^function[[:space:]]+([A-Za-z_$][A-Za-z0-9_$]*)[[:space:]]*\(`)
	seen := map[string]bool{}
	for _, match := range re.FindAllStringSubmatch(js, -1) {
		name := match[1]
		if seen[name] {
			t.Fatalf("taskseditor.js redeclares top-level function %q", name)
		}
		seen[name] = true
	}
	if !seen["updateList"] {
		t.Fatal("taskseditor.js must define updateList")
	}
}
