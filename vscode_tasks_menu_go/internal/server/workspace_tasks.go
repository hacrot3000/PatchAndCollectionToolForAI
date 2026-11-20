package server

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func attachedWorkspaceTaskID(rootID string, originalID int) int {
	sum := sha256.Sum256([]byte(rootID + "\x00" + strconv.Itoa(originalID)))
	id := int(binary.BigEndian.Uint32(sum[:4]) & 0x7fffffff)
	if id == 0 {
		id = 1
	}
	return id
}

func (s *Server) loadWorkspaceTasks() ([]tasks.Task, error) {
	roots, err := s.workspaceRootViews(!s.Config.SharedServerEnabled)
	if err != nil {
		return nil, err
	}
	out := []tasks.Task{}
	seen := map[int]string{}
	for _, root := range roots {
		if !root.Available {
			continue
		}
		tasksPath := filepath.Join(root.Path, ".vscode", "tasks.json")
		if _, statErr := os.Stat(tasksPath); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, fmt.Errorf("inspect tasks for workspace root %q: %w", root.Name, statErr)
		}
		items, loadErr := tasks.Load(root.Path)
		if loadErr != nil {
			return nil, fmt.Errorf("workspace root %q: %w", root.Name, loadErr)
		}
		for _, item := range items {
			if len(tasks.WorkflowNodeForTask(item).DependsOn) > 0 {
				workflowInputs, inputErr := tasks.WorkflowInputs(items, item.Label)
				if inputErr != nil {
					return nil, fmt.Errorf("workspace root %q task %q workflow: %w", root.Name, item.Label, inputErr)
				}
				item.Inputs = workflowInputs
			}
			originalID := item.ID
			if !root.Primary {
				item.ID = attachedWorkspaceTaskID(root.ID, originalID)
			}
			identity := root.ID + ":" + strconv.Itoa(originalID)
			if previous, exists := seen[item.ID]; exists && previous != identity {
				return nil, fmt.Errorf("workspace task id collision between %q and %q", previous, identity)
			}
			seen[item.ID] = identity
			item.WorkspaceRootID = root.ID
			item.WorkspaceRootName = root.Name
			if !root.Primary {
				group := make([]string, 0, len(item.Group)+1)
				group = append(group, root.Name)
				group = append(group, item.Group...)
				item.Group = group
				if item.Detail == "" {
					item.Detail = "Workspace root: " + root.Name
				} else {
					item.Detail += " · Workspace root: " + root.Name
				}
			}
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Server) workspaceTaskByID(id int) (tasks.Task, workspaceRootView, error) {
	if id <= 0 {
		return tasks.Task{}, workspaceRootView{}, fmt.Errorf("task id is invalid")
	}
	items, err := s.loadWorkspaceTasks()
	if err != nil {
		return tasks.Task{}, workspaceRootView{}, err
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		root, resolveErr := s.resolveWorkspaceRoot(item.WorkspaceRootID)
		if resolveErr != nil {
			return tasks.Task{}, workspaceRootView{}, resolveErr
		}
		return item, root, nil
	}
	return tasks.Task{}, workspaceRootView{}, fmt.Errorf("task not found")
}


func (s *Server) workspaceWorkflowByTaskID(id int) (tasks.Task, []tasks.Task, workspaceRootView, error) {
	selected, root, err := s.workspaceTaskByID(id)
	if err != nil {
		return tasks.Task{}, nil, workspaceRootView{}, err
	}
	items, err := tasks.Load(root.Path)
	if err != nil {
		return tasks.Task{}, nil, workspaceRootView{}, err
	}
	var rootTask tasks.Task
	for _, item := range items {
		if item.Label == selected.Label {
			rootTask = item
			break
		}
	}
	if rootTask.Label == "" {
		return tasks.Task{}, nil, workspaceRootView{}, fmt.Errorf("workflow root task disappeared")
	}
	return rootTask, items, root, nil
}
