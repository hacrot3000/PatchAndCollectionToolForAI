package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func workspaceTerminalExecutionAt(workspace, requested string) (tasks.Execution, error) {
	spec, err := workspaceTerminalExecution(workspace)
	if err != nil {
		return tasks.Execution{}, err
	}
	spec.TargetType = "local"
	requested = strings.TrimSpace(requested)
	if requested == "" || requested == "." {
		return spec, nil
	}
	if filepath.IsAbs(requested) || strings.ContainsRune(requested, '\x00') {
		return tasks.Execution{}, fmt.Errorf("terminal cwd phải là thư mục tương đối bên trong workspace")
	}
	clean := filepath.Clean(requested)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return tasks.Execution{}, fmt.Errorf("terminal cwd phải nằm trong workspace")
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return tasks.Execution{}, fmt.Errorf("workspace không hợp lệ")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return tasks.Execution{}, fmt.Errorf("workspace không hợp lệ")
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil || !pathWithin(root, candidate) {
		return tasks.Execution{}, fmt.Errorf("terminal cwd không tồn tại hoặc nằm ngoài workspace")
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return tasks.Execution{}, fmt.Errorf("terminal cwd không phải thư mục")
	}
	spec.Cwd = candidate
	spec.Detail = "Shell tương tác tại " + clean
	return spec, nil
}
