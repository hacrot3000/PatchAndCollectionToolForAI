package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestLegacyPatchLauncherTaskIsConservative(t *testing.T) {
	for _, command := range []string{
		"./tools/run_python_patches.sh",
		"tools/run_python_patches.sh",
		"${workspaceFolder}/tools/run_python_patches.sh",
		".\\tools\\run_python_patches.sh",
	} {
		task := tasks.Task{Command: command}
		if !legacyPatchLauncherTask(task) {
			t.Fatalf("expected legacy Patch launcher task for %q", command)
		}
	}

	for _, task := range []tasks.Task{
		{Command: "./tools/run_python_patches.sh", Args: []any{"collect"}},
		{Command: "./tools/run_python_patches.sh", Args: []string{"report"}},
		{Command: "bash ./tools/run_python_patches.sh"},
		{Command: "./tools/custom_run_python_patches.sh"},
		{Command: "./run_python_patches.sh"},
		{Command: 123},
	} {
		if legacyPatchLauncherTask(task) {
			t.Fatalf("must not hide non-duplicate task: %#v", task)
		}
	}
}

func TestVisibleTasksHidesOnlyZeroArgLegacyPatchLauncher(t *testing.T) {
	items := []tasks.Task{
		{ID: 1, Label: "Patchs: Run Python Patch", Command: "./tools/run_python_patches.sh"},
		{ID: 2, Label: "Patch collect special", Command: "./tools/run_python_patches.sh", Args: []any{"collect", "search"}},
		{ID: 3, Label: "Build", Command: "./build.sh"},
	}
	got := visibleTasks(items)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("visible tasks=%#v", got)
	}
}
