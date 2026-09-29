package server

import (
	"encoding/json"
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func (s *Server) runningIndicatorConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := config.ReadRunningIndicatorSettings(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req config.RunningIndicatorSettings
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		settings, err := config.NormalizeRunningIndicatorSettings(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := config.SetRunningIndicatorSettings(s.Workspace, settings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := config.ReadRunningIndicatorSettings(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "settings.running_indicator.update", "setting", "running_indicator", map[string]any{
			"mode": saved.Mode,
			"rpm":  saved.RPM,
		})
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
