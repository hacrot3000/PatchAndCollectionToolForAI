package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"bletonfc/vscode_tasks_menu/internal/session"
	runtimestate "bletonfc/vscode_tasks_menu/internal/state"
)

const sessionTitlesVersion = 1

type sessionTitleState struct {
	Version int               `json:"version"`
	Titles  map[string]string `json:"titles"`
}

var sessionTitlesMu sync.Mutex

func sessionTitlesPath(workspace string) string {
	return filepath.Join(runtimestate.Dir(workspace), "session-titles.json")
}

func readSessionTitleState(workspace string) (sessionTitleState, error) {
	data, err := os.ReadFile(sessionTitlesPath(workspace))
	if os.IsNotExist(err) {
		return sessionTitleState{Version: sessionTitlesVersion, Titles: map[string]string{}}, nil
	}
	if err != nil {
		return sessionTitleState{}, err
	}
	var state sessionTitleState
	if err := json.Unmarshal(data, &state); err != nil {
		return sessionTitleState{}, fmt.Errorf("parse session title state: %w", err)
	}
	if state.Version != sessionTitlesVersion {
		return sessionTitleState{}, fmt.Errorf("unsupported session title state version %d", state.Version)
	}
	if state.Titles == nil {
		state.Titles = map[string]string{}
	}
	return state, nil
}

func writeSessionTitleState(workspace string, state sessionTitleState) error {
	if err := runtimestate.EnsureDir(workspace); err != nil {
		return err
	}
	if state.Titles == nil {
		state.Titles = map[string]string{}
	}
	state.Version = sessionTitlesVersion
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := runtimestate.Dir(workspace)
	tmp, err := os.CreateTemp(dir, ".session-titles.*.tmp")
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
	if err := os.Rename(name, sessionTitlesPath(workspace)); err != nil {
		return err
	}
	return os.Chmod(sessionTitlesPath(workspace), 0o600)
}

func setStoredSessionTitle(workspace, id, title string) error {
	sessionTitlesMu.Lock()
	defer sessionTitlesMu.Unlock()
	state, err := readSessionTitleState(workspace)
	if err != nil {
		return err
	}
	state.Titles[id] = session.NormalizeTitle(title)
	return writeSessionTitleState(workspace, state)
}

func removeStoredSessionTitle(workspace, id string) error {
	sessionTitlesMu.Lock()
	defer sessionTitlesMu.Unlock()
	state, err := readSessionTitleState(workspace)
	if err != nil {
		return err
	}
	delete(state.Titles, id)
	return writeSessionTitleState(workspace, state)
}

func storedSessionTitles(workspace string) (map[string]string, error) {
	sessionTitlesMu.Lock()
	defer sessionTitlesMu.Unlock()
	state, err := readSessionTitleState(workspace)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(state.Titles))
	for id, title := range state.Titles {
		out[id] = title
	}
	return out, nil
}

func overlayStoredSessionTitle(meta session.Metadata, titles map[string]string) session.Metadata {
	if title, ok := titles[meta.ID]; ok {
		meta.Title = title
	}
	return meta
}

func overlayStoredSessionTitles(items []session.Metadata, titles map[string]string) []session.Metadata {
	out := make([]session.Metadata, len(items))
	for i, meta := range items {
		out[i] = overlayStoredSessionTitle(meta, titles)
	}
	return out
}

func (s *Server) withStoredTitle(meta session.Metadata) session.Metadata {
	titles, err := storedSessionTitles(s.Workspace)
	if err != nil {
		if s.Log != nil {
			s.Log.Printf("session title state read warning: %v", err)
		}
		return meta
	}
	return overlayStoredSessionTitle(meta, titles)
}

func (s *Server) withStoredTitles(items []session.Metadata) []session.Metadata {
	titles, err := storedSessionTitles(s.Workspace)
	if err != nil {
		if s.Log != nil {
			s.Log.Printf("session title state read warning: %v", err)
		}
		return items
	}
	return overlayStoredSessionTitles(items, titles)
}
