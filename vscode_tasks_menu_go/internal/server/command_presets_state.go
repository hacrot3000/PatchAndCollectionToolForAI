package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	projectCommandPresetFile     = "vscode_tasks_menu.presets.json"
	projectCommandPresetVersion  = 1
	projectCommandPresetMaxFile  = 256 << 10
	projectCommandPresetMaxItems = 64
	projectCommandPresetMaxCmds  = 64
	projectCommandPresetMaxName  = 100
	projectCommandPresetMaxCmd   = 8192
)

type commandPreset struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Commands []string `json:"commands"`
}

var commandPresetStateMu sync.Mutex

type projectCommandPresetState struct {
	Version int             `json:"version"`
	Presets []commandPreset `json:"presets"`
}

func projectCommandPresetPath(workspace string) string {
	return projectfiles.Path(workspace, projectCommandPresetFile)
}

func defaultProjectCommandPresetState() projectCommandPresetState {
	return projectCommandPresetState{Version: projectCommandPresetVersion, Presets: []commandPreset{}}
}

func normalizePresetName(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	runes := []rune(value)
	if len(runes) > projectCommandPresetMaxName {
		value = string(runes[:projectCommandPresetMaxName])
	}
	return value
}

func normalizePresetCommands(values []string) ([]string, error) {
	if len(values) > projectCommandPresetMaxCmds {
		return nil, fmt.Errorf("preset exceeds %d commands", projectCommandPresetMaxCmds)
	}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("preset command contains NUL")
		}
		if len(value) > projectCommandPresetMaxCmd {
			return nil, fmt.Errorf("preset command exceeds %d bytes", projectCommandPresetMaxCmd)
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("preset requires at least one command")
	}
	return out, nil
}

func normalizeProjectCommandPresetState(value projectCommandPresetState) (projectCommandPresetState, error) {
	out := defaultProjectCommandPresetState()
	if len(value.Presets) > projectCommandPresetMaxItems {
		return out, fmt.Errorf("preset state exceeds %d presets", projectCommandPresetMaxItems)
	}
	seen := map[string]bool{}
	for _, preset := range value.Presets {
		id := strings.TrimSpace(preset.ID)
		name := normalizePresetName(preset.Name)
		if id == "" || name == "" || seen[id] || strings.ContainsRune(id, '\x00') {
			return out, fmt.Errorf("invalid preset identity")
		}
		if len(id) > 128 {
			return out, fmt.Errorf("preset id is too long")
		}
		commands, err := normalizePresetCommands(preset.Commands)
		if err != nil {
			return out, fmt.Errorf("preset %q: %w", name, err)
		}
		seen[id] = true
		out.Presets = append(out.Presets, commandPreset{ID: id, Name: name, Commands: commands})
	}
	return out, nil
}

func readProjectCommandPresetState(workspace string) (projectCommandPresetState, error) {
	path, err := projectfiles.Resolve(workspace, projectCommandPresetFile)
	if err != nil {
		return projectCommandPresetState{}, fmt.Errorf("resolve preset state: %w", err)
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaultProjectCommandPresetState(), nil
	}
	if err != nil {
		return projectCommandPresetState{}, fmt.Errorf("read preset state: %w", err)
	}
	if len(data) > projectCommandPresetMaxFile {
		return projectCommandPresetState{}, fmt.Errorf("preset state file is too large")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return defaultProjectCommandPresetState(), nil
	}
	var state projectCommandPresetState
	if err := json.Unmarshal(data, &state); err != nil {
		return projectCommandPresetState{}, fmt.Errorf("parse preset state: %w", err)
	}
	if state.Version != projectCommandPresetVersion {
		return projectCommandPresetState{}, fmt.Errorf("unsupported preset state version %d", state.Version)
	}
	return normalizeProjectCommandPresetState(state)
}

func writeProjectCommandPresetState(workspace string, state projectCommandPresetState) error {
	normalized, err := normalizeProjectCommandPresetState(state)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preset state: %w", err)
	}
	data = append(data, '\n')
	if len(data) > projectCommandPresetMaxFile {
		return fmt.Errorf("preset state exceeds size limit")
	}
	target, err := projectfiles.Resolve(workspace, projectCommandPresetFile)
	if err != nil {
		return fmt.Errorf("resolve preset state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".vscode_tasks_menu.presets.*.tmp")
	if err != nil {
		return fmt.Errorf("create preset state temp file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write preset state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync preset state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close preset state: %w", err)
	}
	if err := os.Rename(name, target); err != nil {
		return fmt.Errorf("replace preset state: %w", err)
	}
	return os.Chmod(target, 0o600)
}

func newCommandPresetID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func findCommandPreset(state *projectCommandPresetState, id string) (*commandPreset, bool) {
	for i := range state.Presets {
		if state.Presets[i].ID == id {
			return &state.Presets[i], true
		}
	}
	return nil, false
}

func loadProjectCommandPresetState(workspace string) (projectCommandPresetState, error) {
	commandPresetStateMu.Lock()
	defer commandPresetStateMu.Unlock()
	return readProjectCommandPresetState(workspace)
}

func mutateProjectCommandPresetState(workspace string, fn func(*projectCommandPresetState) error) (projectCommandPresetState, error) {
	commandPresetStateMu.Lock()
	defer commandPresetStateMu.Unlock()
	state, err := readProjectCommandPresetState(workspace)
	if err != nil {
		return projectCommandPresetState{}, err
	}
	if err := fn(&state); err != nil {
		return projectCommandPresetState{}, err
	}
	if err := writeProjectCommandPresetState(workspace, state); err != nil {
		return projectCommandPresetState{}, err
	}
	return readProjectCommandPresetState(workspace)
}
