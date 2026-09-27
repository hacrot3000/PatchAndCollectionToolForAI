package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

var errCommandPresetNotFound = errors.New("command preset not found")

type commandPresetRequest struct {
	Action   string   `json:"action"`
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name,omitempty"`
	Commands []string `json:"commands,omitempty"`
}

func (s *Server) commandPresets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state, err := loadProjectCommandPresetState(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, state)
	case http.MethodPost:
		var req commandPresetRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectCommandPresetMaxFile))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		switch strings.TrimSpace(req.Action) {
		case "create":
			s.createCommandPreset(w, r, req)
		case "update":
			s.updateCommandPreset(w, r, req)
		case "delete":
			s.deleteCommandPreset(w, r, req)
		default:
			http.Error(w, "unknown preset action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func validateCommandPresetInput(name string, commands []string) (string, []string, error) {
	name = normalizePresetName(name)
	if name == "" {
		return "", nil, errors.New("preset name is required")
	}
	normalizedCommands, err := normalizePresetCommands(commands)
	if err != nil {
		return "", nil, err
	}
	return name, normalizedCommands, nil
}

func (s *Server) createCommandPreset(w http.ResponseWriter, r *http.Request, req commandPresetRequest) {
	name, commands, err := validateCommandPresetInput(req.Name, req.Commands)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := newCommandPresetID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := mutateProjectCommandPresetState(s.Workspace, func(state *projectCommandPresetState) error {
		if len(state.Presets) >= projectCommandPresetMaxItems {
			return errors.New("preset limit reached")
		}
		state.Presets = append(state.Presets, commandPreset{ID: id, Name: name, Commands: commands})
		return nil
	})
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "preset limit reached" {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	s.auditSharedSuccess(r, "settings.command_preset.create", "command_preset", id, nil)
	writeJSON(w, http.StatusCreated, state)
}

func (s *Server) updateCommandPreset(w http.ResponseWriter, r *http.Request, req commandPresetRequest) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		http.Error(w, "preset id is required", http.StatusBadRequest)
		return
	}
	name, commands, err := validateCommandPresetInput(req.Name, req.Commands)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state, err := mutateProjectCommandPresetState(s.Workspace, func(state *projectCommandPresetState) error {
		preset, ok := findCommandPreset(state, id)
		if !ok {
			return errCommandPresetNotFound
		}
		preset.Name = name
		preset.Commands = commands
		return nil
	})
	if err != nil {
		if errors.Is(err, errCommandPresetNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	s.auditSharedSuccess(r, "settings.command_preset.update", "command_preset", id, nil)
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) deleteCommandPreset(w http.ResponseWriter, r *http.Request, req commandPresetRequest) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		http.Error(w, "preset id is required", http.StatusBadRequest)
		return
	}
	state, err := mutateProjectCommandPresetState(s.Workspace, func(state *projectCommandPresetState) error {
		found := false
		presets := state.Presets[:0]
		for _, preset := range state.Presets {
			if preset.ID == id {
				found = true
				continue
			}
			presets = append(presets, preset)
		}
		if !found {
			return errCommandPresetNotFound
		}
		state.Presets = presets
		return nil
	})
	if err != nil {
		if errors.Is(err, errCommandPresetNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	s.auditSharedSuccess(r, "settings.command_preset.delete", "command_preset", id, nil)
	writeJSON(w, http.StatusOK, state)
}
