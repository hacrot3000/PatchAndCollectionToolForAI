package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	projectProfilesFile        = "vscode_tasks_menu.project_profiles.json"
	projectProfilesMaxBytes    = 1 << 20
	projectProfilesMaxCount    = 40
	projectProfileMaxRefs      = 32
	projectProfileMaxTerminals = 16
	projectProfileMaxTasks     = 128
	projectProfileMaxEnv       = 128
	projectProfileMaxEnvValue  = 16 << 10
)

type projectProfileTerminal struct {
	Cwd   string `json:"cwd,omitempty"`
	Title string `json:"title,omitempty"`
}

type projectProfileTerminalSplit struct {
	First       int     `json:"first"`
	Second      int     `json:"second"`
	Ratio       float64 `json:"ratio,omitempty"`
	Orientation string  `json:"orientation,omitempty"`
}

type projectProfile struct {
	ID                 string                   `json:"id"`
	Name               string                   `json:"name"`
	EnvironmentProfile string                   `json:"environment_profile,omitempty"`
	Environment        map[string]string        `json:"environment,omitempty"`
	CommandPresetIDs   []string                 `json:"command_preset_ids,omitempty"`
	Terminals          []projectProfileTerminal `json:"terminals,omitempty"`
	TerminalSplits     []projectProfileTerminalSplit `json:"terminal_splits,omitempty"`
	DatabaseProfileIDs []string                 `json:"database_profile_ids,omitempty"`
	SSHProfileIDs      []string                 `json:"ssh_profile_ids,omitempty"`
	TransferProfileIDs []string                 `json:"transfer_profile_ids,omitempty"`
	DefaultGitRepo     string                   `json:"default_git_repository,omitempty"`
	TaskIDs            []int                    `json:"task_ids,omitempty"`
	CreatedAt          string                   `json:"created_at"`
	UpdatedAt          string                   `json:"updated_at"`
}

type projectProfileStore struct {
	Version  int              `json:"version"`
	Profiles []projectProfile `json:"profiles"`
}

type projectProfileSaveRequest struct {
	ID                 string                   `json:"id,omitempty"`
	Name               string                   `json:"name"`
	EnvironmentProfile string                   `json:"environment_profile,omitempty"`
	Environment        map[string]string        `json:"environment,omitempty"`
	CommandPresetIDs   []string                 `json:"command_preset_ids,omitempty"`
	Terminals          []projectProfileTerminal `json:"terminals,omitempty"`
	TerminalSplits     []projectProfileTerminalSplit `json:"terminal_splits,omitempty"`
	DatabaseProfileIDs []string                 `json:"database_profile_ids,omitempty"`
	SSHProfileIDs      []string                 `json:"ssh_profile_ids,omitempty"`
	TransferProfileIDs []string                 `json:"transfer_profile_ids,omitempty"`
	DefaultGitRepo     string                   `json:"default_git_repository,omitempty"`
	TaskIDs            []int                    `json:"task_ids,omitempty"`
}

func projectProfilesPath(workspace string) (string, error) {
	return projectfiles.Resolve(workspace, projectProfilesFile)
}

func readProjectProfileStore(workspace string) (projectProfileStore, error) {
	out := projectProfileStore{Version: 1, Profiles: []projectProfile{}}
	path, err := projectProfilesPath(workspace)
	if err != nil {
		return out, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read project profiles: %w", err)
	}
	if len(data) > projectProfilesMaxBytes {
		return out, fmt.Errorf("project profile file is too large")
	}
	if strings.TrimSpace(string(data)) == "" {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return projectProfileStore{Version: 1, Profiles: []projectProfile{}}, fmt.Errorf("parse project profiles: %w", err)
	}
	if out.Version != 1 {
		return projectProfileStore{Version: 1, Profiles: []projectProfile{}}, fmt.Errorf("unsupported project profile version")
	}
	if out.Profiles == nil {
		out.Profiles = []projectProfile{}
	}
	for i := range out.Profiles {
		out.Profiles[i] = normalizeProjectProfile(out.Profiles[i])
	}
	return out, nil
}

func writeProjectProfileStore(workspace string, store projectProfileStore) error {
	store.Version = 1
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project profiles: %w", err)
	}
	data = append(data, '\n')
	if len(data) > projectProfilesMaxBytes {
		return fmt.Errorf("project profiles exceed size limit")
	}
	target, err := projectProfilesPath(workspace)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".vscode_tasks_menu.project_profiles.*.tmp")
	if err != nil {
		return fmt.Errorf("create project profile temp file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write project profile temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync project profile temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close project profile temp file: %w", err)
	}
	if err := os.Rename(name, target); err != nil {
		return fmt.Errorf("replace project profiles: %w", err)
	}
	return os.Chmod(target, 0o600)
}

func newProjectProfileID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

var projectProfileEnvNamePattern = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")

func validateProjectProfileEnvironment(values map[string]string) error {
	if len(values) > projectProfileMaxEnv {
		return fmt.Errorf("project profile environment exceeds %d variables", projectProfileMaxEnv)
	}
	for key, value := range values {
		if !projectProfileEnvNamePattern.MatchString(key) {
			return fmt.Errorf("invalid project profile environment variable name %q", key)
		}
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("project profile environment variable %q contains NUL", key)
		}
		if len(value) > projectProfileMaxEnvValue {
			return fmt.Errorf("project profile environment variable %q exceeds %d bytes", key, projectProfileMaxEnvValue)
		}
	}
	return nil
}

func normalizeProjectProfileEnvironment(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if projectProfileEnvNamePattern.MatchString(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > projectProfileMaxEnv {
		keys = keys[:projectProfileMaxEnv]
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		value := values[key]
		if strings.ContainsRune(value, '\x00') {
			continue
		}
		if len(value) > projectProfileMaxEnvValue {
			value = value[:projectProfileMaxEnvValue]
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
func normalizeProjectProfileText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		value = value[:max]
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

func normalizeProjectProfileRepository(value string) string {
	value = strings.TrimSpace(value)
	if value == "." {
		return "."
	}
	return normalizeSnapshotPath(value, true)
}

func normalizeProjectProfileRefs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = normalizeProjectProfileText(value, 200)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) >= projectProfileMaxRefs {
			break
		}
	}
	return out
}

func normalizeProjectProfileTasks(values []int) []int {
	out := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) >= projectProfileMaxTasks {
			break
		}
	}
	sort.Ints(out)
	return out
}

func normalizeProjectProfileTerminalSplits(values []projectProfileTerminalSplit, terminalCount int) []projectProfileTerminalSplit {
	if terminalCount < 2 || len(values) == 0 {
		return nil
	}
	limit := terminalCount - 1
	if limit > projectProfileMaxTerminals-1 {
		limit = projectProfileMaxTerminals - 1
	}
	out := make([]projectProfileTerminalSplit, 0, limit)
	seenSecond := map[int]bool{}
	for _, value := range values {
		if value.First < 0 || value.Second < 0 || value.First >= terminalCount || value.Second >= terminalCount || value.First == value.Second || seenSecond[value.Second] {
			continue
		}
		orientation := "vertical"
		if strings.EqualFold(strings.TrimSpace(value.Orientation), "horizontal") {
			orientation = "horizontal"
		}
		ratio := value.Ratio
		if ratio <= 0 {
			ratio = 0.5
		}
		if ratio < 0.2 {
			ratio = 0.2
		}
		if ratio > 0.8 {
			ratio = 0.8
		}
		seenSecond[value.Second] = true
		out = append(out, projectProfileTerminalSplit{
			First: value.First, Second: value.Second, Ratio: ratio, Orientation: orientation,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeProjectProfile(value projectProfile) projectProfile {
	out := projectProfile{
		ID: normalizeProjectProfileText(value.ID, 80),
		Name: normalizeProjectProfileText(value.Name, 120),
		EnvironmentProfile: normalizeProjectProfileText(value.EnvironmentProfile, 120),
		Environment: normalizeProjectProfileEnvironment(value.Environment),
		CommandPresetIDs: normalizeProjectProfileRefs(value.CommandPresetIDs),
		DatabaseProfileIDs: normalizeProjectProfileRefs(value.DatabaseProfileIDs),
		SSHProfileIDs: normalizeProjectProfileRefs(value.SSHProfileIDs),
		TransferProfileIDs: normalizeProjectProfileRefs(value.TransferProfileIDs),
		DefaultGitRepo: normalizeProjectProfileRepository(value.DefaultGitRepo),
		TaskIDs: normalizeProjectProfileTasks(value.TaskIDs),
		CreatedAt: strings.TrimSpace(value.CreatedAt),
		UpdatedAt: strings.TrimSpace(value.UpdatedAt),
	}
	for _, terminal := range value.Terminals {
		cwd := normalizeSnapshotPath(terminal.Cwd, true)
		if cwd == "" {
			cwd = "."
		}
		title := normalizeProjectProfileText(terminal.Title, 120)
		out.Terminals = append(out.Terminals, projectProfileTerminal{Cwd: cwd, Title: title})
		if len(out.Terminals) >= projectProfileMaxTerminals {
			break
		}
	}
	out.TerminalSplits = normalizeProjectProfileTerminalSplits(value.TerminalSplits, len(out.Terminals))
	return out
}

func projectProfileFromRequest(req projectProfileSaveRequest) (projectProfile, error) {
	if err := validateProjectProfileEnvironment(req.Environment); err != nil {
		return projectProfile{}, err
	}
	return normalizeProjectProfile(projectProfile{
		ID: req.ID,
		Name: req.Name,
		EnvironmentProfile: req.EnvironmentProfile,
		Environment: req.Environment,
		CommandPresetIDs: req.CommandPresetIDs,
		Terminals: req.Terminals,
		TerminalSplits: req.TerminalSplits,
		DatabaseProfileIDs: req.DatabaseProfileIDs,
		SSHProfileIDs: req.SSHProfileIDs,
		TransferProfileIDs: req.TransferProfileIDs,
		DefaultGitRepo: req.DefaultGitRepo,
		TaskIDs: req.TaskIDs,
	}), nil
}

func (s *Server) projectProfiles(w http.ResponseWriter, r *http.Request) {
	s.projectProfilesMu.Lock()
	defer s.projectProfilesMu.Unlock()

	store, err := readProjectProfileStore(s.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows := append([]projectProfile(nil), store.Profiles...)
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		})
		writeJSON(w, http.StatusOK, map[string]any{"profiles": rows})
	case http.MethodPost, http.MethodPut:
		var req projectProfileSaveRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectProfilesMaxBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		item, err := projectProfileFromRequest(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if item.Name == "" {
			http.Error(w, "project profile name is required", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		index := -1
		if item.ID != "" {
			for i := range store.Profiles {
				if store.Profiles[i].ID == item.ID {
					index = i
					break
				}
			}
			if index < 0 {
				http.Error(w, "project profile not found", http.StatusNotFound)
				return
			}
			item.CreatedAt = store.Profiles[index].CreatedAt
			if item.CreatedAt == "" {
				item.CreatedAt = now
			}
		} else {
			if len(store.Profiles) >= projectProfilesMaxCount {
				http.Error(w, "project profile limit reached", http.StatusConflict)
				return
			}
			item.ID, err = newProjectProfileID()
			if err != nil {
				http.Error(w, "cannot allocate project profile id", http.StatusInternalServerError)
				return
			}
			item.CreatedAt = now
		}
		item.UpdatedAt = now
		if index >= 0 {
			store.Profiles[index] = item
		} else {
			store.Profiles = append(store.Profiles, item)
		}
		if err := writeProjectProfileStore(s.Workspace, store); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.auditSharedSuccess(r, "project_profile.save", "project_profile", item.ID, nil)
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		id := normalizeProjectProfileText(r.URL.Query().Get("id"), 80)
		if id == "" {
			http.Error(w, "project profile id is required", http.StatusBadRequest)
			return
		}
		found := false
		next := store.Profiles[:0]
		for _, item := range store.Profiles {
			if item.ID == id {
				found = true
				continue
			}
			next = append(next, item)
		}
		if !found {
			http.Error(w, "project profile not found", http.StatusNotFound)
			return
		}
		store.Profiles = next
		if err := writeProjectProfileStore(s.Workspace, store); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "project_profile.delete", "project_profile", id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
