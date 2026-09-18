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

	runtimestate "bletonfc/vscode_tasks_menu/internal/state"
)

const broadcastStateVersion = 1

const (
	broadcastModeNone  = "none"
	broadcastModeAll   = "all"
	broadcastModeGroup = "group"
)

var broadcastPresetIDs = map[string]struct{}{
	"slate": {}, "ocean": {}, "forest": {}, "amber": {},
	"violet": {}, "rose": {}, "cyan": {}, "lime": {},
}

type broadcastGroup struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Preset string `json:"preset"`
}

type broadcastState struct {
	Version     int               `json:"version"`
	Mode        string            `json:"mode"`
	Groups      []broadcastGroup  `json:"groups"`
	Assignments map[string]string `json:"assignments"`
}

var broadcastStateMu sync.Mutex

func broadcastStatePath(workspace string) string {
	return filepath.Join(runtimestate.Dir(workspace), "session-broadcast.json")
}

func defaultBroadcastState() broadcastState {
	return broadcastState{
		Version: broadcastStateVersion,
		Mode: broadcastModeNone,
		Groups: []broadcastGroup{},
		Assignments: map[string]string{},
	}
}

func validBroadcastMode(mode string) bool {
	switch mode {
	case broadcastModeNone, broadcastModeAll, broadcastModeGroup:
		return true
	default:
		return false
	}
}

func normalizeBroadcastGroupName(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}

func validBroadcastPreset(preset string) bool {
	_, ok := broadcastPresetIDs[preset]
	return ok
}

func newBroadcastGroupID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func readBroadcastState(workspace string) (broadcastState, error) {
	data, err := os.ReadFile(broadcastStatePath(workspace))
	if os.IsNotExist(err) {
		return defaultBroadcastState(), nil
	}
	if err != nil {
		return broadcastState{}, err
	}
	var state broadcastState
	if err := json.Unmarshal(data, &state); err != nil {
		return broadcastState{}, fmt.Errorf("parse broadcast state: %w", err)
	}
	if state.Version != broadcastStateVersion {
		return broadcastState{}, fmt.Errorf("unsupported broadcast state version %d", state.Version)
	}
	if !validBroadcastMode(state.Mode) {
		state.Mode = broadcastModeNone
	}
	if state.Groups == nil {
		state.Groups = []broadcastGroup{}
	}
	if state.Assignments == nil {
		state.Assignments = map[string]string{}
	}
	return state, nil
}

func writeBroadcastState(workspace string, state broadcastState) error {
	if err := runtimestate.EnsureDir(workspace); err != nil {
		return err
	}
	state.Version = broadcastStateVersion
	if state.Groups == nil {
		state.Groups = []broadcastGroup{}
	}
	if state.Assignments == nil {
		state.Assignments = map[string]string{}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := runtimestate.Dir(workspace)
	tmp, err := os.CreateTemp(dir, ".session-broadcast.*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
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
	if err := os.Rename(name, broadcastStatePath(workspace)); err != nil {
		return err
	}
	return os.Chmod(broadcastStatePath(workspace), 0o600)
}

func loadBroadcastState(workspace string) (broadcastState, error) {
	broadcastStateMu.Lock()
	defer broadcastStateMu.Unlock()
	return readBroadcastState(workspace)
}

func mutateBroadcastState(workspace string, fn func(*broadcastState) error) (broadcastState, error) {
	broadcastStateMu.Lock()
	defer broadcastStateMu.Unlock()
	state, err := readBroadcastState(workspace)
	if err != nil {
		return broadcastState{}, err
	}
	if err := fn(&state); err != nil {
		return broadcastState{}, err
	}
	if err := writeBroadcastState(workspace, state); err != nil {
		return broadcastState{}, err
	}
	return state, nil
}

func findBroadcastGroup(state *broadcastState, id string) (*broadcastGroup, bool) {
	for i := range state.Groups {
		if state.Groups[i].ID == id {
			return &state.Groups[i], true
		}
	}
	return nil, false
}

func removeBroadcastSession(workspace, sessionID string) error {
	_, err := mutateBroadcastState(workspace, func(state *broadcastState) error {
		delete(state.Assignments, sessionID)
		return nil
	})
	return err
}
