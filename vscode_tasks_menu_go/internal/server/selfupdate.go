package server

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
)

var selfUpdateHandoffs sync.Map // map[*Server]func(string) error

func RegisterSelfUpdateHandoff(s *Server, fn func(string) error) {
	if s == nil {
		return
	}
	if fn == nil {
		selfUpdateHandoffs.Delete(s)
		return
	}
	selfUpdateHandoffs.Store(s, fn)
}

func (s *Server) selfUpdateState(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		req, err := updater.Load(s.Workspace)
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "idle"})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct{ ID string `json:"id"` }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	body.ID = strings.TrimSpace(body.ID)
	if body.ID == "" {
		http.Error(w, "missing update id", http.StatusBadRequest)
		return
	}
	req, err := updater.Load(s.Workspace)
	if err != nil || req.ID != body.ID {
		http.Error(w, "self-update request not found", http.StatusNotFound)
		return
	}

	switch r.URL.Query().Get("action") {
	case "confirm":
		if req.Status != "awaiting_confirmation" {
			http.Error(w, "update is not awaiting confirmation", http.StatusConflict)
			return
		}
		req, err = updater.Update(s.Workspace, req.ID, "confirmed", "Đã xác nhận; chuẩn bị cập nhật…", "", "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, req)
	case "cancel":
		if req.Status != "awaiting_confirmation" {
			http.Error(w, "update can no longer be cancelled", http.StatusConflict)
			return
		}
		req, err = updater.Update(s.Workspace, req.ID, "cancelled", "Người dùng đã hủy cập nhật.", "", "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, req)
	case "ack":
		if req.Status != "completed" {
			http.Error(w, "only completed updates can be acknowledged", http.StatusConflict)
			return
		}
		if err := os.Remove(updater.RequestPath(s.Workspace)); err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": req.ID})
	case "handoff":
		if !loopbackRemote(r.RemoteAddr) {
			http.Error(w, "handoff is restricted to loopback", http.StatusForbidden)
			return
		}
		if req.Status != "ready_restart" {
			http.Error(w, "update is not ready to restart", http.StatusConflict)
			return
		}
		value, ok := selfUpdateHandoffs.Load(s)
		if !ok {
			http.Error(w, "daemon handoff unavailable", http.StatusServiceUnavailable)
			return
		}
		fn, ok := value.(func(string) error)
		if !ok || fn == nil {
			http.Error(w, "daemon handoff unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, err := updater.Update(s.Workspace, req.ID, "restarting", "Đang chuyển daemon sang binary mới…", "", ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "id": req.ID})
		go func() {
			// Let net/http flush the Accepted response before the callback closes
			// the listener. Otherwise the CLI can misread a successful handoff as
			// a connection reset and start a duplicate fallback restart.
			time.Sleep(120 * time.Millisecond)
			if err := fn(req.ID); err != nil {
				_, _ = updater.Update(s.Workspace, req.ID, "failed", "Daemon handoff thất bại.", "", err.Error())
			}
		}()
	default:
		http.Error(w, "unknown self-update action", http.StatusBadRequest)
	}
}

func loopbackRemote(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
