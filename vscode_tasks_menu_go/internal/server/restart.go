package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/session"
)

func (s *Server) sessionTerminate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := sessionActionID(w, r)
	if !ok {
		return
	}
	if !s.authorizeSharedSessionItem(w, r, id, "terminate") {
		return
	}
	capability, ok := s.Sessions.(session.TerminateCapability)
	if !ok {
		http.Error(w, "session terminate capability unavailable", http.StatusNotImplemented)
		return
	}
	if err := capability.Terminate(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	meta, ok := s.Sessions.Metadata(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.releaseSharedMutationForSession(id)
	s.auditSharedSuccess(r, "session.terminate", "session", id, map[string]any{"kind": meta.Kind})
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) sessionProcessTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	if !s.authorizeSharedSessionItem(w, r, id, "process-tree") {
		return
	}
	capability, ok := s.Sessions.(session.ProcessTreeCapability)
	if !ok {
		http.Error(w, "session process-tree capability unavailable", http.StatusNotImplemented)
		return
	}
	processes, err := capability.ProcessTree(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"processes": processes})
}

func (s *Server) sessionForceKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := sessionActionID(w, r)
	if !ok {
		return
	}
	if !s.authorizeSharedSessionItem(w, r, id, "kill") {
		return
	}
	if err := s.Sessions.Kill(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	meta, ok := s.Sessions.Metadata(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.releaseSharedMutationForSession(id)
	s.auditSharedSuccess(r, "session.kill", "session", id, map[string]any{"kind": meta.Kind})
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) sessionClearConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := sessionActionID(w, r)
	if !ok {
		return
	}
	if !s.authorizeSharedSessionItem(w, r, id, "clear") {
		return
	}
	if err := s.Sessions.Clear(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.auditSharedSuccess(r, "session.clear", "session", id, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func sessionActionID(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		ID string `json:"id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return "", false
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return "", false
	}
	return id, true
}
