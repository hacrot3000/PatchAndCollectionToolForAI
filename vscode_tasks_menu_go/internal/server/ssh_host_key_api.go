package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

type sshHostKeyRecoveryRequest struct {
	ProfileID string `json:"profile_id"`
	Action    string `json:"action"`
	Confirmed bool   `json:"confirmed,omitempty"`
}

func (s *Server) sshHostKeyRecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req sshHostKeyRecoveryRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.Action = strings.TrimSpace(req.Action)
	if req.ProfileID == "" {
		http.Error(w, "ssh profile id is required", http.StatusBadRequest)
		return
	}
	store, err := s.sshProfileStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	profile, err := store.Get(req.ProfileID)
	if errors.Is(err, sshprofile.ErrProfileNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	profile, err = sshprofile.Normalize(profile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	switch req.Action {
	case "inspect":
		info := inspectSSHHostKey(r.Context(), profile, "")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "host_key": info})
	case "remove":
		if !req.Confirmed {
			http.Error(w, "explicit confirmation is required before removing an SSH known_hosts entry", http.StatusBadRequest)
			return
		}
		info, output, err := removeSSHHostKey(r.Context(), profile)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "output": output, "host_key": info})
			return
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "ssh_host_key", Action: "remove", ProfileID: req.ProfileID, Success: true})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "output": output, "host_key": info,
			"message": "Removed the old known_hosts entry. Reconnect explicitly and verify the new host key before continuing.",
		})
	default:
		http.Error(w, "unsupported SSH host-key recovery action", http.StatusBadRequest)
	}
}
