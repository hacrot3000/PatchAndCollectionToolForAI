package server

import (
	"encoding/json"
	"io"
	"strings"
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


// patchProjectRules provides editable, project-local AI constraints. Identity
// binding is checked by patchtool on both reads and writes.
func (s *Server) patchProjectRules(w http.ResponseWriter,r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        result,err:=patchtool.ReadAIPackProjectRules(s.Workspace)
        if err!=nil {http.Error(w,err.Error(),http.StatusConflict);return}
        writeJSON(w,http.StatusOK,result)
    case http.MethodPut:
        defer r.Body.Close()
        var payload struct { Rules string `json:"rules"` }
        dec:=json.NewDecoder(io.LimitReader(r.Body, 40<<10))
        dec.DisallowUnknownFields()
        if err:=dec.Decode(&payload);err!=nil {
            http.Error(w,"invalid AI project rules: "+err.Error(),http.StatusBadRequest);return
        }
        if strings.TrimSpace(r.Header.Get("Content-Type"))=="" {
            http.Error(w,"JSON Content-Type required",http.StatusUnsupportedMediaType);return
        }
        result,err:=patchtool.SaveAIPackProjectRules(s.Workspace,payload.Rules)
        if err!=nil {http.Error(w,err.Error(),http.StatusConflict);return}
        writeJSON(w,http.StatusOK,result)
    default:
        http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
    }
}
