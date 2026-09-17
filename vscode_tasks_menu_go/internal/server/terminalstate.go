package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

const (
	projectTerminalStateFile = "vscode_tasks_menu.terminals.json"
	projectTerminalStateMax  = 32 << 10
	projectTerminalMaxTabs   = 32
)

type terminalStateItem struct {
	SessionID string `json:"session_id,omitempty"`
	Cwd       string `json:"cwd"`
}

type terminalSplitState struct {
	Left        int     `json:"left"`
	Right       int     `json:"right"`
	Ratio       float64 `json:"ratio"`
	Orientation string  `json:"orientation,omitempty"`
}

type projectTerminalState struct {
	Version     int                  `json:"version"`
	Terminals   []terminalStateItem  `json:"terminals"`
	ActiveIndex int                  `json:"active_index"`
	Splits      []terminalSplitState `json:"splits,omitempty"`
	// Split is kept only so version-1 state files and cached older browsers can
	// still be read. normalizeProjectTerminalState migrates it into Splits.
	Split *terminalSplitState `json:"split,omitempty"`
}

type terminalSnapshotSplitRequest struct {
	LeftSessionID  string  `json:"left_session_id"`
	RightSessionID string  `json:"right_session_id"`
	Ratio          float64 `json:"ratio"`
	Orientation    string  `json:"orientation,omitempty"`
}

type terminalSnapshotRequest struct {
	SessionIDs      []string                       `json:"session_ids"`
	ActiveSessionID string                         `json:"active_session_id"`
	Splits          []terminalSnapshotSplitRequest `json:"splits,omitempty"`
	// Split is the version-1 request field accepted for browser-cache
	// compatibility. New clients send Splits.
	Split *terminalSnapshotSplitRequest `json:"split,omitempty"`
}

type terminalRestoreResponse struct {
	Sessions    []session.Metadata   `json:"sessions"`
	ActiveIndex int                  `json:"active_index"`
	Splits      []terminalSplitState `json:"splits,omitempty"`
	// Split mirrors the first group for cached version-1 frontends.
	Split    *terminalSplitState `json:"split,omitempty"`
	Warnings []string            `json:"warnings,omitempty"`
}

func defaultProjectTerminalState() projectTerminalState {
	return projectTerminalState{Version: 3, Terminals: []terminalStateItem{}, ActiveIndex: -1, Splits: []terminalSplitState{}}
}

func projectTerminalStatePath(workspace string) string {
	return filepath.Join(workspace, projectTerminalStateFile)
}

func normalizeSplitOrientation(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "horizontal") {
		return "horizontal"
	}
	return "vertical"
}

func normalizeSplitRatio(value float64) float64 {
	if value == 0 {
		value = 0.5
	}
	if value < 0.2 {
		value = 0.2
	}
	if value > 0.8 {
		value = 0.8
	}
	return value
}

func normalizeTerminalSessionID(value string) string {
	value = strings.TrimSpace(value)
	if strings.ContainsRune(value, '\x00') {
		return ""
	}
	if len(value) > 160 {
		value = value[:160]
	}
	return value
}

func normalizeProjectTerminalState(value projectTerminalState) projectTerminalState {
	out := defaultProjectTerminalState()
	for _, item := range value.Terminals {
		cwd := strings.TrimSpace(item.Cwd)
		if cwd == "" || strings.ContainsRune(cwd, '\x00') {
			continue
		}
		if len(cwd) > 4096 {
			cwd = cwd[:4096]
		}
		out.Terminals = append(out.Terminals, terminalStateItem{SessionID: normalizeTerminalSessionID(item.SessionID), Cwd: cwd})
		if len(out.Terminals) == projectTerminalMaxTabs {
			break
		}
	}
	if value.ActiveIndex >= 0 && value.ActiveIndex < len(out.Terminals) {
		out.ActiveIndex = value.ActiveIndex
	} else if len(out.Terminals) > 0 {
		out.ActiveIndex = 0
	}

	candidates := append([]terminalSplitState(nil), value.Splits...)
	if len(candidates) == 0 && value.Split != nil {
		candidates = append(candidates, *value.Split)
	}
	used := make(map[int]bool)
	for _, group := range candidates {
		if group.Left < 0 || group.Right < 0 || group.Left == group.Right || group.Left >= len(out.Terminals) || group.Right >= len(out.Terminals) {
			continue
		}
		if used[group.Left] || used[group.Right] {
			continue
		}
		used[group.Left] = true
		used[group.Right] = true
		out.Splits = append(out.Splits, terminalSplitState{
			Left:        group.Left,
			Right:       group.Right,
			Ratio:       normalizeSplitRatio(group.Ratio),
			Orientation: normalizeSplitOrientation(group.Orientation),
		})
		if len(out.Splits) >= projectTerminalMaxTabs/2 {
			break
		}
	}
	return out
}

func readProjectTerminalState(workspace string) (projectTerminalState, error) {
	value := defaultProjectTerminalState()
	data, err := os.ReadFile(projectTerminalStatePath(workspace))
	if os.IsNotExist(err) {
		return value, nil
	}
	if err != nil {
		return value, fmt.Errorf("read terminal state: %w", err)
	}
	if len(data) > projectTerminalStateMax {
		return value, fmt.Errorf("terminal state file is too large")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return value, nil
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return defaultProjectTerminalState(), fmt.Errorf("parse terminal state: %w", err)
	}
	return normalizeProjectTerminalState(value), nil
}

func writeProjectTerminalState(workspace string, value projectTerminalState) error {
	value = normalizeProjectTerminalState(value)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode terminal state: %w", err)
	}
	data = append(data, '\n')
	if len(data) > projectTerminalStateMax {
		return fmt.Errorf("terminal state exceeds size limit")
	}
	tmp, err := os.CreateTemp(workspace, ".vscode_tasks_menu.terminals.*.tmp")
	if err != nil {
		return fmt.Errorf("create terminal state temp file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write terminal state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync terminal state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close terminal state: %w", err)
	}
	if err := os.Rename(name, projectTerminalStatePath(workspace)); err != nil {
		return fmt.Errorf("replace terminal state: %w", err)
	}
	return os.Chmod(projectTerminalStatePath(workspace), 0o600)
}

func (s *Server) terminalState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		value, err := readProjectTerminalState(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		var req terminalSnapshotRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectTerminalStateMax))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		value, err := s.captureTerminalState(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		previous, _ := readProjectTerminalState(s.Workspace)
		if !reflect.DeepEqual(previous, value) {
			if err := writeProjectTerminalState(s.Workspace, value); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPost:
		resp, err := s.restoreTerminalState()
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) captureTerminalState(req terminalSnapshotRequest) (projectTerminalState, error) {
	value := defaultProjectTerminalState()
	indexByID := make(map[string]int)
	seen := make(map[string]bool)
	for _, id := range req.SessionIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		meta, ok := s.Sessions.Metadata(id)
		if !ok || meta.TaskID != 0 || meta.Status != "running" {
			continue
		}
		cwd, err := s.Sessions.CurrentCwd(id)
		if err != nil {
			cwd = meta.Cwd
		}
		cwd = strings.TrimSpace(cwd)
		if cwd == "" {
			continue
		}
		indexByID[id] = len(value.Terminals)
		value.Terminals = append(value.Terminals, terminalStateItem{SessionID: id, Cwd: cwd})
		if len(value.Terminals) == projectTerminalMaxTabs {
			break
		}
	}
	if idx, ok := indexByID[strings.TrimSpace(req.ActiveSessionID)]; ok {
		value.ActiveIndex = idx
	} else if len(value.Terminals) > 0 {
		value.ActiveIndex = 0
	}

	groups := append([]terminalSnapshotSplitRequest(nil), req.Splits...)
	if len(groups) == 0 && req.Split != nil {
		groups = append(groups, *req.Split)
	}
	used := make(map[int]bool)
	for _, group := range groups {
		left, lok := indexByID[strings.TrimSpace(group.LeftSessionID)]
		right, rok := indexByID[strings.TrimSpace(group.RightSessionID)]
		if !lok || !rok || left == right || used[left] || used[right] {
			continue
		}
		used[left] = true
		used[right] = true
		value.Splits = append(value.Splits, terminalSplitState{
			Left:        left,
			Right:       right,
			Ratio:       group.Ratio,
			Orientation: group.Orientation,
		})
	}
	return normalizeProjectTerminalState(value), nil
}

func (s *Server) restoreTerminalState() (terminalRestoreResponse, error) {
	for _, meta := range s.Sessions.List() {
		if meta.TaskID == 0 {
			return terminalRestoreResponse{}, fmt.Errorf("terminal sessions already exist")
		}
	}
	value, err := readProjectTerminalState(s.Workspace)
	if err != nil {
		return terminalRestoreResponse{}, err
	}
	resp := terminalRestoreResponse{Sessions: []session.Metadata{}, ActiveIndex: value.ActiveIndex, Splits: append([]terminalSplitState(nil), value.Splits...)}
	if len(resp.Splits) > 0 {
		first := resp.Splits[0]
		resp.Split = &first
	}
	if len(value.Terminals) == 0 {
		return resp, nil
	}
	specs := make([]restoreTerminalSpec, 0, len(value.Terminals))
	for _, item := range value.Terminals {
		spec, warning, err := terminalRestoreExecution(s.Workspace, item.Cwd)
		if err != nil {
			return terminalRestoreResponse{}, err
		}
		specs = append(specs, restoreTerminalSpec{spec: spec, warning: warning})
	}
	started := make([]string, 0, len(specs))
	for i, item := range specs {
		meta, err := s.Sessions.Start(item.spec)
		if err != nil {
			for _, id := range started {
				_ = s.Sessions.Stop(id)
			}
			return terminalRestoreResponse{}, fmt.Errorf("restore terminal: %w", err)
		}
		started = append(started, meta.ID)
		resp.Sessions = append(resp.Sessions, meta)
		value.Terminals[i].SessionID = meta.ID
		if item.warning != "" {
			resp.Warnings = append(resp.Warnings, item.warning)
		}
	}
	if err := writeProjectTerminalState(s.Workspace, value); err != nil {
		for _, id := range started {
			_ = s.Sessions.Stop(id)
		}
		return terminalRestoreResponse{}, fmt.Errorf("persist restored terminal ids: %w", err)
	}
	return resp, nil
}

// RestoreProjectTerminalsForStartup recreates the saved terminal PTYs before a
// replacement daemon advertises self-update completion. It is safe only while
// the session manager does not yet contain terminal sessions.
func (s *Server) RestoreProjectTerminalsForStartup() (int, []string, error) {
	resp, err := s.restoreTerminalState()
	if err != nil {
		return 0, nil, err
	}
	return len(resp.Sessions), append([]string(nil), resp.Warnings...), nil
}

type restoreTerminalSpec struct {
	spec    tasks.Execution
	warning string
}

func terminalRestoreExecution(workspace, savedCwd string) (tasks.Execution, string, error) {
	spec, err := workspaceTerminalExecution(workspace)
	if err != nil {
		return tasks.Execution{}, "", err
	}
	cwd := strings.TrimSpace(savedCwd)
	if cwd == "" || strings.ContainsRune(cwd, '\x00') {
		return spec, "terminal cwd cũ không hợp lệ; dùng project root", nil
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(workspace, cwd)
	}
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return spec, "terminal cwd cũ không còn tồn tại; dùng project root", nil
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return spec, "terminal cwd cũ không phải thư mục; dùng project root", nil
	}
	spec.Cwd = resolved
	spec.Detail = "Shell khôi phục tại " + resolved
	return spec, "", nil
}
