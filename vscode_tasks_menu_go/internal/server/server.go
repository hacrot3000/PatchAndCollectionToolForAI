package server

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
	"github.com/coder/websocket"
)

type Server struct {
	Workspace string
	Config    config.Config
	Log       *log.Logger
	Sessions  session.Service

	projectIndexMu         sync.Mutex
	projectIndex           *projectFileIndex
	projectIndexRefreshing bool

	authMu       sync.Mutex
	authFailures map[string]authFailureState

	browserLeaseMu sync.Mutex
	browserLease   *browserLease

	terminalStateMu     sync.RWMutex
	terminalStateFrozen bool
}

func (s *Server) FreezeTerminalStatePersistence() {
	s.terminalStateMu.Lock()
	s.terminalStateFrozen = true
	s.terminalStateMu.Unlock()
}

func (s *Server) terminalStatePersistenceFrozen() bool {
	s.terminalStateMu.RLock()
	frozen := s.terminalStateFrozen
	s.terminalStateMu.RUnlock()
	return frozen
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/browser/lease", s.browserLeaseAPI)
	mux.HandleFunc("/api/tasks", s.tasks)
	mux.HandleFunc("/api/state/tasks", s.taskState)
	mux.HandleFunc("/api/config/page-title", s.pageTitle)
	mux.HandleFunc("/api/config/terminal-cwds", s.terminalCWDConfig)
	mux.HandleFunc("/api/broadcast", s.broadcastStateAPI)
	mux.HandleFunc("/api/command-presets", s.commandPresets)
	mux.HandleFunc("/api/git/status", s.gitStatus)
	mux.HandleFunc("/api/files/selection", s.filesSelection)
	mux.HandleFunc("/api/files/download", s.fileDownload)
	mux.HandleFunc("/api/files/upload", s.fileUpload)
	mux.HandleFunc("/api/project/tree", s.projectTree)
	mux.HandleFunc("/api/project/file", s.projectFile)
	mux.HandleFunc("/api/project/files/search", s.projectFileSearch)
	mux.HandleFunc("/api/project/content/search", s.projectContentSearch)
	mux.HandleFunc("/api/sessions/force-kill", s.sessionForceKill)
	mux.HandleFunc("/api/sessions/clear-console", s.sessionClearConsole)
	mux.HandleFunc("/api/sessions", s.sessionsRoot)
	mux.HandleFunc("/api/sessions/", s.sessionItem)
	mux.HandleFunc("/", staticUI)

	var handler http.Handler = mux
	handler = s.requireBrowserLease(handler)
	handler = s.sameOriginMutations(handler)
	if s.Config.AuthEnabled {
		handler = s.basicAuth(handler)
	}
	return securityHeaders(handler, s.Config.TLS())
}

func (s *Server) Serve(listener net.Listener) error {
	if err := s.Config.Validate(); err != nil {
		_ = listener.Close()
		return fmt.Errorf("server config validation: %w", err)
	}
	if addr := listener.Addr(); addr != nil && strings.HasPrefix(addr.Network(), "tcp") {
		if err := s.Config.ValidateListenerAddress(addr.String()); err != nil {
			_ = listener.Close()
			return fmt.Errorf("listener security validation: %w", err)
		}
	}
	if s.Config.TLS() && !s.Config.CustomTLS() {
		_ = listener.Close()
		return fmt.Errorf("HTTPS certificate/key chưa được resolve trước khi serve")
	}
	httpServer := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	if s.Config.TLS() {
		return httpServer.ServeTLS(listener, s.Config.TLSCert, s.Config.TLSKey)
	}
	return httpServer.Serve(listener)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := tasks.Load(s.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": s.Workspace, "tasks": visibleTasks(items)})
}

func (s *Server) sessionsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"sessions": s.withStoredTitles(s.Sessions.List())})
	case http.MethodPost:
		var req struct {
			Kind      string            `json:"kind"`
			TaskID    int               `json:"task_id"`
			Inputs    map[string]string `json:"inputs,omitempty"`
			Env       map[string]string `json:"env,omitempty"`
			Cwd       string            `json:"cwd,omitempty"`
			PatchMode string            `json:"patch_mode,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		switch strings.TrimSpace(req.Kind) {
		case "terminal":
			spec, err := workspaceTerminalExecutionAt(s.Workspace, req.Cwd)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := tasks.ApplyEnvironmentOverrides(&spec, req.Env); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := configureTerminalGitTextconv(s.Workspace, &spec); err != nil && s.Log != nil {
				s.Log.Printf("terminal git textconv warning: %v", err)
			}
			meta, err := s.Sessions.Start(spec)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, meta)
			return
		case "patch":
			spec, err := patchToolExecution(s.Workspace, req.PatchMode)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := tasks.ApplyEnvironmentOverrides(&spec, req.Env); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			meta, err := s.Sessions.Start(spec)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, meta)
			return
		case "", "task":
			// Existing task execution path below.
		default:
			http.Error(w, "unknown session kind", http.StatusBadRequest)
			return
		}

		items, err := tasks.Load(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var selected *tasks.Task
		for i := range items {
			if items[i].ID == req.TaskID {
				selected = &items[i]
				break
			}
		}
		if selected == nil {
			http.Error(w, "task not found; reload tasks.json", http.StatusNotFound)
			return
		}
		spec, err := tasks.ResolveExecutionWithInputs(*selected, s.Workspace, req.Inputs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := tasks.ApplyEnvironmentOverrides(&spec, req.Env); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := s.Sessions.Start(spec)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, meta)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func patchModeArguments(mode string) ([]string, string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "queue":
		return nil, "Queue", nil
	case "resume":
		return []string{"resume"}, "Resume", nil
	case "history":
		return []string{"report"}, "History", nil
	case "plan":
		return []string{"plan"}, "Plan", nil
	default:
		return nil, "", fmt.Errorf("unknown Patch Tool mode %q", mode)
	}
}

func patchToolExecution(workspace, mode string) (tasks.Execution, error) {
	exe, err := os.Executable()
	if err != nil {
		return tasks.Execution{}, fmt.Errorf("resolve TaskDeck executable: %w", err)
	}
	runtimeSpec, err := patchtool.Resolve(workspace, exe)
	if err != nil {
		return tasks.Execution{}, err
	}
	patchArgs, modeLabel, err := patchModeArguments(mode)
	if err != nil {
		return tasks.Execution{}, err
	}
	command, commandArgs, err := runtimeSpec.Command(workspace, patchArgs)
	if err != nil {
		return tasks.Execution{}, err
	}
	rawArgs := make([]any, len(commandArgs))
	for i, arg := range commandArgs {
		rawArgs[i] = arg
	}
	spec, err := tasks.ResolveExecution(tasks.Task{
		ID:        -1,
		Label:     "Patch Tool · " + modeLabel,
		MenuLabel: "Patch Tool",
		Detail:    "TaskDeck built-in Python Patch Tool add-on",
		Type:      "process",
		Command:   command,
		Args:      rawArgs,
	}, workspace)
	if err != nil {
		return tasks.Execution{}, err
	}
	spec.ProtocolEvents = true
	spec.ProtocolCommands = true
	if strings.EqualFold(strings.TrimSpace(mode), "resume") {
		if err := tasks.ApplyEnvironmentOverrides(&spec, map[string]string{"TASKDECK_PATCH_NATIVE_RESUME": "1"}); err != nil {
			return tasks.Execution{}, err
		}
	}
	if strings.EqualFold(strings.TrimSpace(mode), "history") {
		if err := tasks.ApplyEnvironmentOverrides(&spec, map[string]string{"TASKDECK_PATCH_NATIVE_HISTORY": "1"}); err != nil {
			return tasks.Execution{}, err
		}
	}
	return spec, nil
}

type patchPromptResponseRequest struct {
	PromptID string `json:"prompt_id"`
	Action   string `json:"action"`
	Indexes  []int  `json:"indexes,omitempty"`
}

func buildPatchPromptResponseCommand(state session.ProtocolState, req patchPromptResponseRequest) ([]byte, error) {
	if !state.CommandsEnabled {
		return nil, fmt.Errorf("Patch protocol command channel is not enabled")
	}
	if len(state.Prompt) == 0 {
		return nil, fmt.Errorf("Patch session has no active prompt")
	}
	var prompt struct {
		Protocol   string `json:"protocol"`
		Version    int    `json:"version"`
		Type       string `json:"type"`
		PromptID   string `json:"prompt_id"`
		PromptKind string `json:"prompt_kind"`
		Actions    []string `json:"actions"`
		Items      []struct {
			Index int `json:"index"`
		} `json:"items"`
	}
	if err := json.Unmarshal(state.Prompt, &prompt); err != nil {
		return nil, fmt.Errorf("invalid active Patch prompt: %w", err)
	}
	if prompt.Protocol != "taskdeck.patch" || prompt.Version != 1 || prompt.Type != "prompt" || prompt.PromptKind != "queue_selection" {
		return nil, fmt.Errorf("unsupported active Patch prompt")
	}
	req.PromptID = strings.TrimSpace(req.PromptID)
	if req.PromptID == "" || req.PromptID != prompt.PromptID {
		return nil, fmt.Errorf("prompt response does not match the active prompt")
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	allowed := false
	for _, value := range prompt.Actions {
		if action == value {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("unsupported prompt action %q", action)
	}
	payload := map[string]any{"prompt_id": req.PromptID, "action": action}
	switch action {
	case "cancel":
		if len(req.Indexes) != 0 {
			return nil, fmt.Errorf("cancel prompt response must not include indexes")
		}
	case "select":
		if len(req.Indexes) == 0 || len(req.Indexes) > 4096 {
			return nil, fmt.Errorf("select prompt response requires a bounded non-empty indexes array")
		}
		valid := make(map[int]bool, len(prompt.Items))
		for _, item := range prompt.Items {
			if item.Index > 0 {
				valid[item.Index] = true
			}
		}
		for _, index := range req.Indexes {
			if !valid[index] {
				return nil, fmt.Errorf("prompt response index out of range: %d", index)
			}
		}
		payload["indexes"] = req.Indexes
	default:
		return nil, fmt.Errorf("unsupported prompt action %q", action)
	}
	seq := time.Now().UnixNano()
	if seq < 1 {
		seq = 1
	}
	return json.Marshal(map[string]any{
		"protocol": "taskdeck.patch",
		"version": 1,
		"type": "command",
		"seq": seq,
		"command": "prompt_response",
		"payload": payload,
	})
}

type patchItemActionRequest struct {
	PromptID string `json:"prompt_id"`
	Action   string `json:"action"`
	Index    int    `json:"index"`
}

func newPatchItemActionID() (string, error) {
	var raw [12]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate Patch item action id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func buildPatchItemActionCommand(state session.ProtocolState, req patchItemActionRequest) ([]byte, string, error) {
	if !state.CommandsEnabled {
		return nil, "", fmt.Errorf("Patch protocol command channel is not enabled")
	}
	if len(state.Prompt) == 0 {
		return nil, "", fmt.Errorf("Patch session has no active prompt")
	}
	var prompt struct {
		Protocol    string   `json:"protocol"`
		Version     int      `json:"version"`
		Type        string   `json:"type"`
		PromptID    string   `json:"prompt_id"`
		PromptKind  string   `json:"prompt_kind"`
		ItemActions []string `json:"item_actions"`
		Items       []struct {
			Index int    `json:"index"`
			Kind  string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal(state.Prompt, &prompt); err != nil {
		return nil, "", fmt.Errorf("invalid active Patch prompt: %w", err)
	}
	if prompt.Protocol != "taskdeck.patch" || prompt.Version != 1 || prompt.Type != "prompt" || prompt.PromptKind != "queue_selection" {
		return nil, "", fmt.Errorf("unsupported active Patch prompt")
	}
	req.PromptID = strings.TrimSpace(req.PromptID)
	if req.PromptID == "" || req.PromptID != prompt.PromptID {
		return nil, "", fmt.Errorf("item action does not match the active prompt")
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	allowed := false
	for _, value := range prompt.ItemActions {
		if action == strings.ToLower(strings.TrimSpace(value)) { allowed = true; break }
	}
	if !allowed {
		return nil, "", fmt.Errorf("unsupported Patch item action %q", action)
	}
	if req.Index < 1 {
		return nil, "", fmt.Errorf("Patch item action index is invalid")
	}
	selectedKind := ""
	for _, item := range prompt.Items {
		if item.Index == req.Index { selectedKind = strings.ToUpper(strings.TrimSpace(item.Kind)); break }
	}
	if selectedKind == "" {
		return nil, "", fmt.Errorf("Patch item action index out of range: %d", req.Index)
	}
	if selectedKind != "PATCH" {
		return nil, "", fmt.Errorf("native inspect/preview/validate applies only to PATCH items")
	}
	actionID, err := newPatchItemActionID()
	if err != nil { return nil, "", err }
	seq := time.Now().UnixNano()
	if seq < 1 { seq = 1 }
	command, err := json.Marshal(map[string]any{
		"protocol": "taskdeck.patch", "version": 1, "type": "command", "seq": seq, "command": "item_action",
		"payload": map[string]any{"prompt_id": req.PromptID, "action_id": actionID, "action": action, "index": req.Index},
	})
	if err != nil { return nil, "", err }
	return command, actionID, nil
}

type patchQueueDeleteRequest struct {
	PromptID string `json:"prompt_id"`
	Index    int    `json:"index"`
}

func newPatchQueueMutationID() (string, error) {
	var raw [12]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate Patch queue mutation id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func buildPatchQueueDeleteCommand(state session.ProtocolState, req patchQueueDeleteRequest) ([]byte, string, error) {
	if !state.CommandsEnabled {
		return nil, "", fmt.Errorf("Patch protocol command channel is not enabled")
	}
	if len(state.Prompt) == 0 {
		return nil, "", fmt.Errorf("Patch session has no active prompt")
	}
	var prompt struct {
		Protocol     string   `json:"protocol"`
		Version      int      `json:"version"`
		Type         string   `json:"type"`
		PromptID     string   `json:"prompt_id"`
		PromptKind   string   `json:"prompt_kind"`
		QueueActions []string `json:"queue_actions"`
		Items        []struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
			Kind  string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal(state.Prompt, &prompt); err != nil {
		return nil, "", fmt.Errorf("invalid active Patch queue prompt: %w", err)
	}
	if prompt.Protocol != "taskdeck.patch" || prompt.Version != 1 || prompt.Type != "prompt" || prompt.PromptKind != "queue_selection" {
		return nil, "", fmt.Errorf("unsupported active Patch queue prompt")
	}
	req.PromptID = strings.TrimSpace(req.PromptID)
	if req.PromptID == "" || req.PromptID != prompt.PromptID {
		return nil, "", fmt.Errorf("Queue delete does not match the active prompt")
	}
	deleteAdvertised := false
	for _, value := range prompt.QueueActions {
		if strings.EqualFold(strings.TrimSpace(value), "delete") {
			deleteAdvertised = true
			break
		}
	}
	if !deleteAdvertised {
		return nil, "", fmt.Errorf("Patch Queue delete is not available")
	}
	if req.Index < 1 {
		return nil, "", fmt.Errorf("Patch Queue delete index is invalid")
	}
	itemAvailable := false
	for _, item := range prompt.Items {
		if item.Index == req.Index && strings.TrimSpace(item.Name) != "" && strings.TrimSpace(item.Kind) != "" {
			itemAvailable = true
			break
		}
	}
	if !itemAvailable {
		return nil, "", fmt.Errorf("Patch Queue delete index is unavailable: %d", req.Index)
	}
	mutationID, err := newPatchQueueMutationID()
	if err != nil {
		return nil, "", err
	}
	seq := time.Now().UnixNano()
	if seq < 1 {
		seq = 1
	}
	command, err := json.Marshal(map[string]any{
		"protocol": "taskdeck.patch",
		"version": 1,
		"type": "command",
		"seq": seq,
		"command": "queue_delete",
		"payload": map[string]any{
			"prompt_id": req.PromptID,
			"mutation_id": mutationID,
			"index": req.Index,
		},
	})
	if err != nil {
		return nil, "", err
	}
	return command, mutationID, nil
}

type patchResumeActionRequest struct {
	PromptID     string `json:"prompt_id"`
	Action       string `json:"action"`
	FailedIndexes []int `json:"failed_indexes,omitempty"`
}

func buildPatchResumeActionCommand(state session.ProtocolState, req patchResumeActionRequest) ([]byte, error) {
	if !state.CommandsEnabled {
		return nil, fmt.Errorf("Patch protocol command channel is not enabled")
	}
	if len(state.Prompt) == 0 {
		return nil, fmt.Errorf("Patch session has no active prompt")
	}
	var prompt struct {
		Protocol   string   `json:"protocol"`
		Version    int      `json:"version"`
		Type       string   `json:"type"`
		PromptID   string   `json:"prompt_id"`
		PromptKind string   `json:"prompt_kind"`
		Actions    []string `json:"actions"`
		FailedItems []struct {
			Index      int  `json:"index"`
			CanRetry   bool `json:"can_retry"`
			CanCollect bool `json:"can_collect"`
			CanDelete  bool `json:"can_delete"`
		} `json:"failed_items"`
		Constraints struct {
			SelectionActions []string `json:"selection_actions"`
		} `json:"constraints"`
	}
	if err := json.Unmarshal(state.Prompt, &prompt); err != nil {
		return nil, fmt.Errorf("invalid active Patch Resume prompt: %w", err)
	}
	if prompt.Protocol != "taskdeck.patch" || prompt.Version != 1 || prompt.Type != "prompt" || prompt.PromptKind != "resume_action" {
		return nil, fmt.Errorf("unsupported active Patch Resume prompt")
	}
	req.PromptID = strings.TrimSpace(req.PromptID)
	if req.PromptID == "" || req.PromptID != prompt.PromptID {
		return nil, fmt.Errorf("Resume action does not match the active prompt")
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "all", "failed", "remaining", "collect_failed", "delete_failed", "history", "normal":
	default:
		return nil, fmt.Errorf("unsupported Patch Resume action %q", action)
	}
	advertised := false
	for _, value := range prompt.Actions {
		if action == strings.ToLower(strings.TrimSpace(value)) {
			advertised = true
			break
		}
	}
	if !advertised {
		return nil, fmt.Errorf("Patch Resume action %q is not available", action)
	}
	selectionAction := false
	for _, value := range prompt.Constraints.SelectionActions {
		if action == strings.ToLower(strings.TrimSpace(value)) {
			selectionAction = true
			break
		}
	}
	payload := map[string]any{"prompt_id": req.PromptID, "action": action}
	if selectionAction {
		if len(req.FailedIndexes) == 0 || len(req.FailedIndexes) > 4096 {
			return nil, fmt.Errorf("Patch Resume action %q requires a bounded non-empty failed_indexes array", action)
		}
		valid := make(map[int]bool, len(prompt.FailedItems))
		for _, item := range prompt.FailedItems {
			allowed := false
			switch action {
			case "failed":
				allowed = item.CanRetry
			case "collect_failed":
				allowed = item.CanCollect
			case "delete_failed":
				allowed = item.CanDelete
			}
			if item.Index > 0 && allowed {
				valid[item.Index] = true
			}
		}
		seen := make(map[int]bool, len(req.FailedIndexes))
		indexes := make([]int, 0, len(req.FailedIndexes))
		for _, index := range req.FailedIndexes {
			if !valid[index] {
				return nil, fmt.Errorf("Patch Resume failed index is unavailable for %s: %d", action, index)
			}
			if !seen[index] {
				seen[index] = true
				indexes = append(indexes, index)
			}
		}
		payload["failed_indexes"] = indexes
	} else if len(req.FailedIndexes) != 0 {
		return nil, fmt.Errorf("Patch Resume action %q must not include failed_indexes", action)
	}
	seq := time.Now().UnixNano()
	if seq < 1 {
		seq = 1
	}
	return json.Marshal(map[string]any{
		"protocol": "taskdeck.patch",
		"version": 1,
		"type": "command",
		"seq": seq,
		"command": "resume_action",
		"payload": payload,
	})
}

type patchHistoryDetailRequest struct {
	PromptID string `json:"prompt_id"`
	RunID    string `json:"run_id"`
}

func buildPatchHistoryDetailCommand(state session.ProtocolState, req patchHistoryDetailRequest) ([]byte, error) {
	if !state.CommandsEnabled {
		return nil, fmt.Errorf("Patch protocol command channel is not enabled")
	}
	if len(state.Prompt) == 0 {
		return nil, fmt.Errorf("Patch session has no active prompt")
	}
	var prompt struct {
		Protocol   string   `json:"protocol"`
		Version    int      `json:"version"`
		Type       string   `json:"type"`
		PromptID   string   `json:"prompt_id"`
		PromptKind string   `json:"prompt_kind"`
		Actions    []string `json:"actions"`
		Runs       []struct {
			RunID string `json:"run_id"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(state.Prompt, &prompt); err != nil {
		return nil, fmt.Errorf("invalid active Patch History prompt: %w", err)
	}
	if prompt.Protocol != "taskdeck.patch" || prompt.Version != 1 || prompt.Type != "prompt" || prompt.PromptKind != "history_action" {
		return nil, fmt.Errorf("unsupported active Patch History prompt")
	}
	req.PromptID = strings.TrimSpace(req.PromptID)
	if req.PromptID == "" || req.PromptID != prompt.PromptID {
		return nil, fmt.Errorf("History detail does not match the active prompt")
	}
	detailAdvertised := false
	for _, value := range prompt.Actions {
		if strings.EqualFold(strings.TrimSpace(value), "detail") {
			detailAdvertised = true
			break
		}
	}
	if !detailAdvertised {
		return nil, fmt.Errorf("Patch History detail is not available")
	}
	req.RunID = strings.TrimSpace(req.RunID)
	if req.RunID == "" || len(req.RunID) > 128 {
		return nil, fmt.Errorf("Patch History run_id is invalid")
	}
	runAvailable := false
	for _, row := range prompt.Runs {
		if strings.TrimSpace(row.RunID) == req.RunID {
			runAvailable = true
			break
		}
	}
	if !runAvailable {
		return nil, fmt.Errorf("Patch History run_id is not available in the active prompt")
	}
	seq := time.Now().UnixNano()
	if seq < 1 { seq = 1 }
	return json.Marshal(map[string]any{
		"protocol": "taskdeck.patch", "version": 1, "type": "command", "seq": seq,
		"command": "history_detail",
		"payload": map[string]any{"prompt_id": req.PromptID, "run_id": req.RunID},
	})
}

func workspaceTerminalExecution(workspace string) (tasks.Execution, error) {
	candidates := []string{strings.TrimSpace(os.Getenv("SHELL")), "/bin/bash", "/bin/sh"}
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		shell, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		return tasks.ResolveExecution(tasks.Task{
			Label:   "Terminal",
			Detail:  "Shell tương tác tại thư mục gốc project",
			Type:    "process",
			Command: shell,
		}, workspace)
	}
	return tasks.Execution{}, fmt.Errorf("không tìm thấy shell tương tác ($SHELL, /bin/bash hoặc /bin/sh)")
}

func (s *Server) sessionItem(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(strings.Trim(tail, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			meta, ok := s.Sessions.Metadata(id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, http.StatusOK, s.withStoredTitle(meta))
		case http.MethodDelete:
			if err := s.Sessions.Remove(id); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if err := removeStoredSessionTitle(s.Workspace, id); err != nil && s.Log != nil {
				s.Log.Printf("session title cleanup warning: %v", err)
			}
			if err := removeBroadcastSession(s.Workspace, id); err != nil && s.Log != nil {
				s.Log.Printf("broadcast assignment cleanup warning: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch parts[1] {
	case "stop":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.Sessions.Stop(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		meta, _ := s.Sessions.Metadata(id)
		writeJSON(w, http.StatusOK, s.withStoredTitle(meta))
	case "title":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Title string `json:"title"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		meta, ok := s.Sessions.Metadata(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		title := session.NormalizeTitle(req.Title)
		if err := setStoredSessionTitle(s.Workspace, id, title); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		syncToSession := true
		if capability, ok := s.Sessions.(interface{ SupportsSessionTitle() bool }); ok && !capability.SupportsSessionTitle() {
			syncToSession = false
		}
		if syncToSession {
			if updated, err := s.Sessions.SetTitle(id, title); err != nil {
				if s.Log != nil {
					s.Log.Printf("session title broker sync warning id=%s: %v", id, err)
				}
			} else {
				meta = updated
			}
		}
		meta.Title = title
		writeJSON(w, http.StatusOK, meta)
	case "prompt-response":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provider, ok := s.Sessions.(session.ProtocolStateProvider)
		if !ok {
			http.Error(w, "Patch protocol state is unavailable", http.StatusConflict)
			return
		}
		writer, ok := s.Sessions.(session.ProtocolCommandWriter)
		if !ok {
			http.Error(w, "Patch protocol commands are unavailable", http.StatusConflict)
			return
		}
		state, err := provider.ProtocolState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if !state.CommandsEnabled {
			http.Error(w, "Patch protocol command channel is not enabled", http.StatusConflict)
			return
		}
		var req patchPromptResponseRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid prompt response JSON", http.StatusBadRequest)
			return
		}
		command, err := buildPatchPromptResponseCommand(state, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := writer.ProtocolCommand(id, command); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "item-action":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provider, ok := s.Sessions.(session.ProtocolStateProvider)
		if !ok {
			http.Error(w, "Patch protocol state is unavailable", http.StatusConflict)
			return
		}
		writer, ok := s.Sessions.(session.ProtocolCommandWriter)
		if !ok {
			http.Error(w, "Patch protocol commands are unavailable", http.StatusConflict)
			return
		}
		state, err := provider.ProtocolState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var req patchItemActionRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid Patch item action JSON", http.StatusBadRequest)
			return
		}
		command, actionID, err := buildPatchItemActionCommand(state, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := writer.ProtocolCommand(id, command); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "action_id": actionID})
	case "queue-delete":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provider, ok := s.Sessions.(session.ProtocolStateProvider)
		if !ok {
			http.Error(w, "Patch protocol state is unavailable", http.StatusConflict)
			return
		}
		writer, ok := s.Sessions.(session.ProtocolCommandWriter)
		if !ok {
			http.Error(w, "Patch protocol commands are unavailable", http.StatusConflict)
			return
		}
		state, err := provider.ProtocolState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var req patchQueueDeleteRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid Patch Queue delete JSON", http.StatusBadRequest)
			return
		}
		command, mutationID, err := buildPatchQueueDeleteCommand(state, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := writer.ProtocolCommand(id, command); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "mutation_id": mutationID})
	case "resume-action":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provider, ok := s.Sessions.(session.ProtocolStateProvider)
		if !ok {
			http.Error(w, "Patch protocol state is unavailable", http.StatusConflict)
			return
		}
		writer, ok := s.Sessions.(session.ProtocolCommandWriter)
		if !ok {
			http.Error(w, "Patch protocol commands are unavailable", http.StatusConflict)
			return
		}
		state, err := provider.ProtocolState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var req patchResumeActionRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid Patch Resume action JSON", http.StatusBadRequest)
			return
		}
		command, err := buildPatchResumeActionCommand(state, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := writer.ProtocolCommand(id, command); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "history-detail":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provider, ok := s.Sessions.(session.ProtocolStateProvider)
		if !ok {
			http.Error(w, "Patch protocol state is unavailable", http.StatusConflict)
			return
		}
		writer, ok := s.Sessions.(session.ProtocolCommandWriter)
		if !ok {
			http.Error(w, "Patch protocol commands are unavailable", http.StatusConflict)
			return
		}
		state, err := provider.ProtocolState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var req patchHistoryDetailRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid Patch History detail JSON", http.StatusBadRequest)
			return
		}
		command, err := buildPatchHistoryDetailCommand(state, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := writer.ProtocolCommand(id, command); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "protocol":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provider, ok := s.Sessions.(session.ProtocolStateProvider)
		if !ok {
			writeJSON(w, http.StatusOK, session.ProtocolState{Available: false})
			return
		}
		state, err := provider.ProtocolState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, state)
	case "resize":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Rows uint16 `json:"rows"`
			Cols uint16 `json:"cols"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := s.Sessions.Resize(id, req.Rows, req.Cols); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "ws":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.sessionWebSocket(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) sessionWebSocket(w http.ResponseWriter, r *http.Request, id string) {
	leaseToken := strings.TrimSpace(r.URL.Query().Get("lease"))
	revoked, ok := s.browserLeaseState().watch(leaseToken)
	if !ok {
		writeLeaseRevoked(w)
		return
	}
	backlog, stream, unsubscribe, err := s.Sessions.Subscribe(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer unsubscribe()
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(32 << 10)
	defer conn.Close(websocket.StatusNormalClosure, "session closed")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if len(backlog) > 0 {
		if err := conn.Write(ctx, websocket.MessageBinary, backlog); err != nil {
			return
		}
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if !s.browserLeaseState().valid(leaseToken) {
				return
			}
			_ = s.Sessions.Input(id, data)
		}
	}()
	for {
		select {
		case <-revoked:
			_ = conn.Close(websocket.StatusPolicyViolation, "browser control lease revoked")
			return
		case <-readDone:
			return
		case data, ok := <-stream:
			if !ok {
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
				return
			}
		}
	}
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" && loopbackRemote(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		if blocked, retry := s.authBlocked(r.RemoteAddr, time.Now()); blocked {
			writeAuthRateLimit(w, retry)
			return
		}
		user, pass, ok := r.BasicAuth()
		userMatch := constantTimeCredentialEqual(user, s.Config.Username)
		passMatch := constantTimeCredentialEqual(pass, s.Config.Password)
		if !ok || !userMatch || !passMatch {
			s.authRecordFailure(r.RemoteAddr, time.Now())
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("WWW-Authenticate", `Basic realm="VSCode Tasks Menu", charset="UTF-8"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		s.authRecordSuccess(r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOriginMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
					http.Error(w, "cross-origin mutation rejected", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler, tlsEnabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; font-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if tlsEnabled {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func RemoteWarning(cfg config.Config) string {
	if strings.HasPrefix(cfg.Bind, "127.") || cfg.Bind == "localhost" || cfg.Bind == "::1" {
		return ""
	}
	if !cfg.TLS() {
		return "SECURITY WARNING: remote access đang dùng protocol=http; Basic Auth chỉ base64 nên username/password và dữ liệu phiên có thể bị chặn đọc trên mạng. Khuyến nghị đổi server.protocol=https; nếu không cấu hình tls_cert/tls_key thì tool sẽ tự tạo self-signed certificate. Nếu dùng HTTPS reverse proxy, hãy bind tool vào loopback và vẫn giữ auth.enabled=true, hoặc cấu hình proxy tự enforce authentication."
	}
	return ""
}
