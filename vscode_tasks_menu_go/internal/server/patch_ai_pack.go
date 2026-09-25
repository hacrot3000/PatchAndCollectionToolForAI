package server

import (
	"net/http"
	"os"

	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func (s *Server) patchAIPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		http.Error(w, "resolve TaskDeck executable: "+err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := patchtool.BuildAIPack(s.Workspace, executable)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
