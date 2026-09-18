package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
	"github.com/coder/websocket"
)

type Server struct {
	Workspace string
	Config    config.Config
	Log       *log.Logger
	Sessions  session.Service
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/tasks", s.tasks)
	mux.HandleFunc("/api/state/tasks", s.taskState)
	mux.HandleFunc("/api/config/page-title", s.pageTitle)
	mux.HandleFunc("/api/git/status", s.gitStatus)
	mux.HandleFunc("/api/files/selection", s.filesSelection)
	mux.HandleFunc("/api/files/download", s.fileDownload)
	mux.HandleFunc("/api/files/upload", s.fileUpload)
	mux.HandleFunc("/api/sessions/force-kill", s.sessionForceKill)
	mux.HandleFunc("/api/sessions/clear-console", s.sessionClearConsole)
	mux.HandleFunc("/api/sessions", s.sessionsRoot)
	mux.HandleFunc("/api/sessions/", s.sessionItem)
	mux.HandleFunc("/", staticUI)

	var handler http.Handler = mux
	handler = s.sameOriginMutations(handler)
	if s.Config.AuthEnabled {
		handler = s.basicAuth(handler)
	}
	return securityHeaders(handler)
}

func (s *Server) Serve(listener net.Listener) error {
	httpServer := &http.Server{Handler: s.Handler()}
	if s.Config.TLS() {
		return httpServer.ServeTLS(listener, s.Config.TLSCert, s.Config.TLSKey)
	}
	return httpServer.Serve(listener)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := tasks.Load(s.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": s.Workspace, "tasks": items})
}

func (s *Server) sessionsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"sessions": s.withStoredTitles(s.Sessions.List())})
	case http.MethodPost:
		var req struct {
			Kind   string            `json:"kind"`
			TaskID int               `json:"task_id"`
			Inputs map[string]string `json:"inputs,omitempty"`
			Env    map[string]string `json:"env,omitempty"`
			Cwd    string            `json:"cwd,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		switch strings.TrimSpace(req.Kind) {
		case "terminal":
			spec, err := workspaceTerminalExecutionAt(s.Workspace, req.Cwd)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := tasks.ApplyEnvironmentOverrides(&spec, req.Env); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			meta, err := s.Sessions.Start(spec)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, meta)
			return
		case "", "task":
			// Existing task execution path below.
		default:
			http.Error(w, "unknown session kind", http.StatusBadRequest)
			return
		}

		items, err := tasks.Load(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var selected *tasks.Task
		for i := range items {
			if items[i].ID == req.TaskID {
				selected = &items[i]
				break
			}
		}
		if selected == nil {
			http.Error(w, "task not found; reload tasks.json", http.StatusNotFound)
			return
		}
		spec, err := tasks.ResolveExecutionWithInputs(*selected, s.Workspace, req.Inputs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := tasks.ApplyEnvironmentOverrides(&spec, req.Env); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := s.Sessions.Start(spec)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, meta)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func workspaceTerminalExecution(workspace string) (tasks.Execution, error) {
	candidates := []string{strings.TrimSpace(os.Getenv("SHELL")), "/bin/bash", "/bin/sh"}
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		shell, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		return tasks.ResolveExecution(tasks.Task{
			Label:   "Terminal",
			Detail:  "Shell tương tác tại thư mục gốc project",
			Type:    "process",
			Command: shell,
		}, workspace)
	}
	return tasks.Execution{}, fmt.Errorf("không tìm thấy shell tương tác ($SHELL, /bin/bash hoặc /bin/sh)")
}

func (s *Server) sessionItem(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(strings.Trim(tail, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			meta, ok := s.Sessions.Metadata(id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, http.StatusOK, s.withStoredTitle(meta))
		case http.MethodDelete:
			if err := s.Sessions.Remove(id); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if err := removeStoredSessionTitle(s.Workspace, id); err != nil && s.Log != nil {
				s.Log.Printf("session title cleanup warning: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch parts[1] {
	case "stop":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.Sessions.Stop(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		meta, _ := s.Sessions.Metadata(id)
		writeJSON(w, http.StatusOK, s.withStoredTitle(meta))
	case "title":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Title string `json:"title"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		meta, ok := s.Sessions.Metadata(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		title := session.NormalizeTitle(req.Title)
		if err := setStoredSessionTitle(s.Workspace, id, title); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		syncToSession := true
		if capability, ok := s.Sessions.(interface{ SupportsSessionTitle() bool }); ok && !capability.SupportsSessionTitle() {
			syncToSession = false
		}
		if syncToSession {
			if updated, err := s.Sessions.SetTitle(id, title); err != nil {
				if s.Log != nil {
					s.Log.Printf("session title broker sync warning id=%s: %v", id, err)
				}
			} else {
				meta = updated
			}
		}
		meta.Title = title
		writeJSON(w, http.StatusOK, meta)
	case "resize":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Rows uint16 `json:"rows"`
			Cols uint16 `json:"cols"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := s.Sessions.Resize(id, req.Rows, req.Cols); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "ws":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.sessionWebSocket(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) sessionWebSocket(w http.ResponseWriter, r *http.Request, id string) {
	backlog, stream, unsubscribe, err := s.Sessions.Subscribe(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer unsubscribe()
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "session closed")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if len(backlog) > 0 {
		if err := conn.Write(ctx, websocket.MessageBinary, backlog); err != nil {
			return
		}
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			_ = s.Sessions.Input(id, data)
		}
	}()
	for {
		select {
		case <-readDone:
			return
		case data, ok := <-stream:
			if !ok {
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
				return
			}
		}
	}
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(s.Config.Username)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(s.Config.Password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="VSCode Tasks Menu", charset="UTF-8"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOriginMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
					http.Error(w, "cross-origin mutation rejected", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; font-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func RemoteWarning(cfg config.Config) string {
	if strings.HasPrefix(cfg.Bind, "127.") || cfg.Bind == "localhost" || cfg.Bind == "::1" {
		return ""
	}
	if !cfg.TLS() {
		return "WARNING: remote access đang dùng HTTP; Basic Auth sẽ không được mã hóa trên đường truyền. Nên cấu hình tls_cert/tls_key."
	}
	return ""
}
