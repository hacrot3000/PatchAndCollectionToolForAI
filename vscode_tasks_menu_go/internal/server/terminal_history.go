package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	projectTerminalHistoryFile       = "vscode_tasks_menu.terminal_history.json"
	projectTerminalHistoryMaxFile    = 8 << 20
	projectTerminalHistoryMaxSessions = 64
	projectTerminalHistoryMaxCommands = 200
	projectTerminalHistoryMaxCommand = 16 << 10
	projectTerminalHistoryMaxOutput  = 128 << 10
	projectTerminalHistoryMaxNote    = 16 << 10
	projectTerminalHistoryMaxCwd     = 4096
)

type terminalHistoryCommand struct {
	Command      string `json:"command,omitempty"`
	Output       string `json:"output,omitempty"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	Cwd          string `json:"cwd,omitempty"`
	Remote       bool   `json:"remote,omitempty"`
	StartedAt    int64  `json:"started_at,omitempty"`
	FinishedAt   int64  `json:"finished_at,omitempty"`
	BookmarkLine int    `json:"bookmark_line,omitempty"`
	BookmarkText string `json:"bookmark_text,omitempty"`
}

type terminalHistorySession struct {
	Commands  []terminalHistoryCommand `json:"commands"`
	Note      string                   `json:"note,omitempty"`
	UpdatedAt int64                    `json:"updated_at"`
}

type terminalHistoryStore struct {
	Version  int                               `json:"version"`
	Sessions map[string]terminalHistorySession `json:"sessions"`
}

type terminalHistoryPutRequest struct {
	Commands []terminalHistoryCommand `json:"commands"`
	Note     string                   `json:"note,omitempty"`
}

var terminalHistoryMu sync.Mutex

func defaultTerminalHistoryStore() terminalHistoryStore {
	return terminalHistoryStore{Version: 1, Sessions: map[string]terminalHistorySession{}}
}

func terminalHistoryPath(workspace string) (string, error) {
	return projectfiles.Resolve(workspace, projectTerminalHistoryFile)
}

func truncateTerminalHistoryText(value string, max int) string {
	value = strings.ReplaceAll(value, "\x00", "")
	if len(value) > max {
		value = value[:max]
	}
	return value
}

func normalizeTerminalHistoryCommand(value terminalHistoryCommand) terminalHistoryCommand {
	value.Command = truncateTerminalHistoryText(value.Command, projectTerminalHistoryMaxCommand)
	value.Output = truncateTerminalHistoryText(value.Output, projectTerminalHistoryMaxOutput)
	value.Cwd = truncateTerminalHistoryText(value.Cwd, projectTerminalHistoryMaxCwd)
	if value.StartedAt < 0 {
		value.StartedAt = 0
	}
	if value.FinishedAt < value.StartedAt {
		value.FinishedAt = value.StartedAt
	}
	lines := 0
	if value.Output != "" {
		lines = 1 + strings.Count(value.Output, "\n")
	}
	if value.BookmarkLine < 1 || value.BookmarkLine > lines {
		value.BookmarkLine = 0
		value.BookmarkText = ""
	} else {
		value.BookmarkText = truncateTerminalHistoryText(value.BookmarkText, 4096)
	}
	return value
}

func normalizeTerminalHistorySession(value terminalHistorySession) terminalHistorySession {
	if len(value.Commands) > projectTerminalHistoryMaxCommands {
		value.Commands = value.Commands[len(value.Commands)-projectTerminalHistoryMaxCommands:]
	}
	out := make([]terminalHistoryCommand, 0, len(value.Commands))
	for _, command := range value.Commands {
		command = normalizeTerminalHistoryCommand(command)
		if command.Command == "" && command.Output == "" {
			continue
		}
		out = append(out, command)
	}
	value.Commands = out
	value.Note = truncateTerminalHistoryText(value.Note, projectTerminalHistoryMaxNote)
	if value.UpdatedAt <= 0 {
		value.UpdatedAt = time.Now().UnixMilli()
	}
	return value
}

func normalizeTerminalHistoryStore(value terminalHistoryStore) terminalHistoryStore {
	out := defaultTerminalHistoryStore()
	type pair struct {
		id      string
		session terminalHistorySession
	}
	items := make([]pair, 0, len(value.Sessions))
	for rawID, session := range value.Sessions {
		id := normalizeTerminalSessionID(rawID)
		if id == "" {
			continue
		}
		session = normalizeTerminalHistorySession(session)
		if len(session.Commands) == 0 && session.Note == "" {
			continue
		}
		items = append(items, pair{id: id, session: session})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].session.UpdatedAt == items[j].session.UpdatedAt {
			return items[i].id < items[j].id
		}
		return items[i].session.UpdatedAt > items[j].session.UpdatedAt
	})
	if len(items) > projectTerminalHistoryMaxSessions {
		items = items[:projectTerminalHistoryMaxSessions]
	}
	for _, item := range items {
		out.Sessions[item.id] = item.session
	}
	return out
}

func readTerminalHistoryStore(workspace string) (terminalHistoryStore, error) {
	path, err := terminalHistoryPath(workspace)
	if err != nil {
		return terminalHistoryStore{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaultTerminalHistoryStore(), nil
	}
	if err != nil {
		return terminalHistoryStore{}, fmt.Errorf("read terminal history: %w", err)
	}
	if len(data) > projectTerminalHistoryMaxFile {
		return terminalHistoryStore{}, fmt.Errorf("terminal history file is too large")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return defaultTerminalHistoryStore(), nil
	}
	var value terminalHistoryStore
	if err := json.Unmarshal(data, &value); err != nil {
		return terminalHistoryStore{}, fmt.Errorf("parse terminal history: %w", err)
	}
	if value.Version != 1 {
		return terminalHistoryStore{}, fmt.Errorf("unsupported terminal history version %d", value.Version)
	}
	return normalizeTerminalHistoryStore(value), nil
}

func writeTerminalHistoryStore(workspace string, value terminalHistoryStore) error {
	value = normalizeTerminalHistoryStore(value)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > projectTerminalHistoryMaxFile {
		return fmt.Errorf("terminal history exceeds size limit")
	}
	target, err := terminalHistoryPath(workspace)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".vscode_tasks_menu.terminal_history.*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, target); err != nil {
		return err
	}
	return os.Chmod(target, 0o600)
}

func mutateTerminalHistoryStore(workspace string, fn func(*terminalHistoryStore) error) (terminalHistoryStore, error) {
	terminalHistoryMu.Lock()
	defer terminalHistoryMu.Unlock()
	value, err := readTerminalHistoryStore(workspace)
	if err != nil {
		return terminalHistoryStore{}, err
	}
	if err := fn(&value); err != nil {
		return terminalHistoryStore{}, err
	}
	if err := writeTerminalHistoryStore(workspace, value); err != nil {
		return terminalHistoryStore{}, err
	}
	return normalizeTerminalHistoryStore(value), nil
}

func loadTerminalHistorySession(workspace, sessionID string) (terminalHistorySession, error) {
	terminalHistoryMu.Lock()
	defer terminalHistoryMu.Unlock()
	store, err := readTerminalHistoryStore(workspace)
	if err != nil {
		return terminalHistorySession{}, err
	}
	return store.Sessions[normalizeTerminalSessionID(sessionID)], nil
}

func (s *Server) terminalHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := normalizeTerminalSessionID(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	if _, ok := s.Sessions.Metadata(sessionID); !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.authorizeSharedSessionItem(w, r, sessionID, "history") {
			return
		}
		value, err := loadTerminalHistorySession(s.Workspace, sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if value.Commands == nil {
			value.Commands = []terminalHistoryCommand{}
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		if !s.authorizeSharedSessionItem(w, r, sessionID, "history-write") {
			return
		}
		var req terminalHistoryPutRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectTerminalHistoryMaxFile))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		value := normalizeTerminalHistorySession(terminalHistorySession{
			Commands: req.Commands,
			Note: req.Note,
			UpdatedAt: time.Now().UnixMilli(),
		})
		store, err := mutateTerminalHistoryStore(s.Workspace, func(store *terminalHistoryStore) error {
			if len(value.Commands) == 0 && value.Note == "" {
				delete(store.Sessions, sessionID)
			} else {
				store.Sessions[sessionID] = value
			}
			return nil
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		saved := store.Sessions[sessionID]
		if saved.Commands == nil {
			saved.Commands = []terminalHistoryCommand{}
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func removeTerminalHistorySession(workspace, sessionID string) error {
	_, err := mutateTerminalHistoryStore(workspace, func(store *terminalHistoryStore) error {
		delete(store.Sessions, normalizeTerminalSessionID(sessionID))
		return nil
	})
	return err
}

func remapTerminalHistorySessions(workspace string, remap map[string]string) error {
	if len(remap) == 0 {
		return nil
	}
	_, err := mutateTerminalHistoryStore(workspace, func(store *terminalHistoryStore) error {
		for rawOld, rawNew := range remap {
			oldID := normalizeTerminalSessionID(rawOld)
			newID := normalizeTerminalSessionID(rawNew)
			if oldID == "" || newID == "" || oldID == newID {
				continue
			}
			value, ok := store.Sessions[oldID]
			if !ok {
				continue
			}
			delete(store.Sessions, oldID)
			if current, exists := store.Sessions[newID]; exists && current.UpdatedAt > value.UpdatedAt {
				continue
			}
			value.UpdatedAt = time.Now().UnixMilli()
			store.Sessions[newID] = value
		}
		return nil
	})
	return err
}
