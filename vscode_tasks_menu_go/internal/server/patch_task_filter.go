package server

import (
	"strings"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func legacyPatchLauncherTask(task tasks.Task) bool {
	command, ok := task.Command.(string)
	if !ok {
		return false
	}
	command = strings.ReplaceAll(strings.TrimSpace(command), "\\", "/")
	switch command {
	case "./tools/run_python_patches.sh", "tools/run_python_patches.sh", "${workspaceFolder}/tools/run_python_patches.sh":
	default:
		return false
	}

	switch args := task.Args.(type) {
	case nil:
		return true
	case []any:
		return len(args) == 0
	case []string:
		return len(args) == 0
	default:
		return false
	}
}

func visibleTasks(items []tasks.Task) []tasks.Task {
	out := make([]tasks.Task, 0, len(items))
	for _, item := range items {
		if legacyPatchLauncherTask(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}
