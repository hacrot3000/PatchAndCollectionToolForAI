package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestResizableSidebarWaitsForTasksAndAppliesGridBeforeInsert(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/sidebar.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function initializeSidebar()",
		"app.taskData?.workspace",
		"taskmenu:tasks",
		"{once:true}",
		"let width=load();",
		"aside.after(resizer);",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("sidebar.js missing startup-race guard %q", want)
		}
	}
	if strings.Index(js, "let width=load();") > strings.Index(js, "aside.after(resizer);") {
		t.Fatal("sidebar grid must be applied before inserting the third grid child")
	}
}
