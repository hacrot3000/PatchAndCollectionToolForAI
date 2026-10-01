package server

import (
	"encoding/json"
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func (s *Server) selfUpdateConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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
			"run_full_validation_tests": saved.RunFullValidationTests,
		})
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
