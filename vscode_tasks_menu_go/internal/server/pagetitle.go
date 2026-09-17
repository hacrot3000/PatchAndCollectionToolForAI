package server

import (
	"encoding/json"
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func (s *Server) pageTitle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		title, err := config.ReadPageTitle(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"title": title})
	case http.MethodPut:
		var req struct {
			Title string `json:"title"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := config.SetPageTitle(s.Workspace, req.Title); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		title, err := config.ReadPageTitle(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"title": title})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
