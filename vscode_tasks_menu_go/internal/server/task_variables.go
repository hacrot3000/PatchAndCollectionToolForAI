package server

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

// taskEditorVariableRequest is an untrusted browser snapshot. The host turns
// its virtual project path into a real, validated path before variable expansion.
type taskEditorVariableRequest struct {
	Path         string `json:"path"`
	LineNumber   int    `json:"line_number"`
	ColumnNumber int    `json:"column_number"`
	SelectedText string `json:"selected_text"`
}

func taskWorkflowNeedsVSCodeSettings(workflowTasks []tasks.Task) bool {
	for _, item := range workflowTasks {
		for _, value := range []any{item.Command, item.Args, item.Options} {
			data, err := json.Marshal(value)
			if err == nil && strings.Contains(string(data), "${config:") { return true }
		}
	}
	return false
}

func (s *Server) resolveTaskEditorVariables(taskRoot workspaceRootView, editor *taskEditorVariableRequest, needsConfig bool) (tasks.VariableContext, error) {
	var result tasks.VariableContext
	if needsConfig {
		config, err := tasks.LoadVSCodeConfiguration(taskRoot.Path)
		if err != nil { return result, fmt.Errorf("workspace settings: %w", err) }
		result.Config = config
	}
	roots, err := s.workspaceRootViews(!s.Config.SharedServerEnabled)
	if err != nil {
		return result, err
	}
	result.WorkspaceFolders = map[string]string{}
	duplicates := map[string]bool{}
	for _, root := range roots {
		if !root.Available || duplicates[root.Name] { continue }
		if previous, ok := result.WorkspaceFolders[root.Name]; ok && previous != root.Path {
			delete(result.WorkspaceFolders, root.Name)
			duplicates[root.Name] = true
		} else {
			result.WorkspaceFolders[root.Name] = root.Path
		}
	}
	if editor == nil {
		return result, nil
	}
	if len(editor.Path) > 4096 || editor.Path == "" || !utf8.ValidString(editor.Path) {
		return result, fmt.Errorf("invalid active editor file path")
	}
	if editor.LineNumber < 0 || editor.LineNumber > 100000000 ||
		editor.ColumnNumber < 0 || editor.ColumnNumber > 100000000 {
		return result, fmt.Errorf("invalid active editor cursor")
	}
	if len(editor.SelectedText) > 8192 || !utf8.ValidString(editor.SelectedText) ||
		strings.ContainsRune(editor.SelectedText, 0) {
		return result, fmt.Errorf("selected editor text exceeds the safe limit")
	}
	virtual, err := cleanProjectRelativePath(editor.Path, false)
	if err != nil {
		return result, fmt.Errorf("invalid editor path: %w", err)
	}
	root, _, err := s.projectRootForVirtualPath(virtual)
	if err != nil || root.ID != taskRoot.ID {
		return result, fmt.Errorf("active editor file must belong to the task workspace")
	}
	absolute, err := s.resolveProjectPath(virtual, false, false)
	if err != nil {
		return result, fmt.Errorf("active editor file is unavailable: %w", err)
	}
	if !filepath.IsAbs(absolute) {
		return result, fmt.Errorf("active editor file is not absolute")
	}
	result.File = absolute
	result.LineNumber = editor.LineNumber
	result.ColumnNumber = editor.ColumnNumber
	result.SelectedText = editor.SelectedText
	return result, nil
}
