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


// patchProjectIdentity is deliberately POST-only: the first request may safely
// migrate a legacy workspace, but no GET request mutates its configuration.
func (s *Server) patchProjectIdentity(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
    if err != nil {
        http.Error(w, "project identity unavailable: "+err.Error(), http.StatusConflict)
        return
    }
    writeJSON(w, http.StatusOK, identity)
}
