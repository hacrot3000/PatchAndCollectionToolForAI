package server

import (
	"encoding/json"
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/config"
	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
)

func (s *Server) selfUpdateConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("action") == "branches" {
			branches, err := updater.RemoteBranches(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"branches": branches})
			return
		}
		if action := r.URL.Query().Get("action"); action != "" {
			http.Error(w, "unknown self-update config action", http.StatusBadRequest)
			return
		}
		settings, err := config.ReadSelfUpdateSettings(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req config.SelfUpdateSettings
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		branch, err := config.NormalizeSelfUpdateBranch(req.Branch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Branch = branch
		if err := config.SetSelfUpdateSettings(s.Workspace, req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := config.ReadSelfUpdateSettings(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "settings.self_update.update", "setting", "self_update", map[string]any{
			"branch": saved.Branch,
		})
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
