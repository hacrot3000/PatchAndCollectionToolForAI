package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func (s *Server) terminalCWDConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := config.ReadTerminalCWDSettings(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req config.TerminalCWDSettings
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		settings, err := config.NormalizeTerminalCWDSettings(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, dir := range settings.CustomDirs {
			if _, err := workspaceTerminalExecutionAt(s.Workspace, dir); err != nil {
				http.Error(w, fmt.Sprintf("terminal directory %q: %v", dir, err), http.StatusBadRequest)
				return
			}
		}
		if err := config.SetTerminalCWDSettings(s.Workspace, settings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := config.ReadTerminalCWDSettings(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
