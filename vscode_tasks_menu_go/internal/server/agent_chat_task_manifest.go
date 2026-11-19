package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
	"bletonfc/vscode_tasks_menu/internal/taskmanifest"
)

// agentTaskManifestValidateAPI is planning-only: it parses untrusted Markdown
// into a canonical dependency report. It makes no provider call, writes no
// project file, creates no TaskDeck task, and never invokes a scheduler.
func (s *Server) agentTaskManifestValidateAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsRead, "agent.task_manifest.validate", "agent_task_manifest") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, taskmanifest.MaxMarkdownBytes+4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req struct {
		Markdown string `json:"markdown"`
	}
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid Task Manifest request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object required", 400)
		return
	}
	if len(req.Markdown) == 0 {
		http.Error(w, "markdown is required", 400)
		return
	}
	verified, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil {
		http.Error(w, "verified project identity unavailable", 409)
		return
	}
	manifest, err := taskmanifest.ParseMarkdown(req.Markdown, verified.ID)
	if err != nil {
		status := 400
		if len(req.Markdown) > taskmanifest.MaxMarkdownBytes {
			status = 413
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"manifest":          manifest,
		"planning_only":     true,
		"execution_enabled": false,
	})
}
