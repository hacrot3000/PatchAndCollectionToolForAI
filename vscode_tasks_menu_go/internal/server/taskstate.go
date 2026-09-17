package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	projectTaskStateFile = "vscode_tasks_menu.state.json"
	projectTaskStateMax  = 64 << 10
)

type taskHistoryItem struct {
	SessionID string `json:"session_id"`
	TaskID    int    `json:"task_id"`
	Label     string `json:"label"`
	Status    string `json:"status"`
	ExitCode  *int   `json:"exit_code"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
	Duration  *int64 `json:"duration"`
}

type projectTaskState struct {
	Version   int               `json:"version"`
	Favorites []int             `json:"favorites"`
	Recent    []int             `json:"recent"`
	History   []taskHistoryItem `json:"history"`
}

func defaultProjectTaskState() projectTaskState {
	return projectTaskState{Version: 1, Favorites: []int{}, Recent: []int{}, History: []taskHistoryItem{}}
}

func projectTaskStatePath(workspace string) string {
	return filepath.Join(workspace, projectTaskStateFile)
}

func readProjectTaskState(workspace string) (projectTaskState, error) {
	state := defaultProjectTaskState()
	data, err := os.ReadFile(projectTaskStatePath(workspace))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read task-menu state: %w", err)
	}
	if len(data) > projectTaskStateMax {
		return state, fmt.Errorf("task-menu state file is too large")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return defaultProjectTaskState(), fmt.Errorf("parse task-menu state: %w", err)
	}
	return normalizeProjectTaskState(state), nil
}

func normalizeProjectTaskState(state projectTaskState) projectTaskState {
	state.Version = 1
	state.Favorites = normalizeTaskIDs(state.Favorites, 20)
	state.Recent = normalizeTaskIDs(state.Recent, 10)
	if len(state.History) > 10 {
		state.History = state.History[:10]
	}
	out := make([]taskHistoryItem, 0, len(state.History))
	seenSessions := make(map[string]bool, len(state.History))
	for _, item := range state.History {
		item.SessionID = trimStateString(item.SessionID, 160)
		item.Label = trimStateString(item.Label, 512)
		item.Status = trimStateString(item.Status, 32)
		item.StartedAt = trimStateString(item.StartedAt, 80)
		item.EndedAt = trimStateString(item.EndedAt, 80)
		if item.TaskID <= 0 || item.SessionID == "" || seenSessions[item.SessionID] {
			continue
		}
		seenSessions[item.SessionID] = true
		out = append(out, item)
		if len(out) == 10 {
			break
		}
	}
	state.History = out
	return state
}

func normalizeTaskIDs(ids []int, limit int) []int {
	out := make([]int, 0, len(ids))
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == limit {
			break
		}
	}
	return out
}

func trimStateString(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		value = value[:max]
	}
	return value
}

func writeProjectTaskState(workspace string, state projectTaskState) error {
	state = normalizeProjectTaskState(state)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode task-menu state: %w", err)
	}
	data = append(data, '\n')
	if len(data) > projectTaskStateMax {
		return fmt.Errorf("task-menu state exceeds size limit")
	}

	tmp, err := os.CreateTemp(workspace, ".vscode_tasks_menu.state.*.tmp")
	if err != nil {
		return fmt.Errorf("create task-menu state temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod task-menu state temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write task-menu state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync task-menu state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close task-menu state: %w", err)
	}
	if err := os.Rename(tmpName, projectTaskStatePath(workspace)); err != nil {
		return fmt.Errorf("replace task-menu state: %w", err)
	}
	return os.Chmod(projectTaskStatePath(workspace), 0o600)
}

func (s *Server) taskState(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("scope") == "terminals" {
		s.terminalState(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		state, err := readProjectTaskState(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, state)
	case http.MethodPut:
		var state projectTaskState
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectTaskStateMax))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&state); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := writeProjectTaskState(s.Workspace, state); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		state, err := readProjectTaskState(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, state)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
