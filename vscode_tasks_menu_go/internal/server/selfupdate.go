package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
)

const InternalControlHeader = "X-TaskDeck-Internal-Control"

type SelfUpdateCheckResult struct {
	Available        bool   `json:"available"`
	InstalledRevision string `json:"installed_revision,omitempty"`
	RemoteRevision    string `json:"remote_revision,omitempty"`
}

var selfUpdateHandoffs sync.Map // map[*Server]func(string) error
var selfUpdateDetaches sync.Map // map[*Server]func(string) error
var selfUpdateChecks sync.Map   // map[*Server]func(context.Context) (SelfUpdateCheckResult, error)
var selfUpdateStarts sync.Map   // map[*Server]func() error

func RegisterSelfUpdateCheck(s *Server, fn func(context.Context) (SelfUpdateCheckResult, error)) {
	if s == nil {
		return
	}
	if fn == nil {
		selfUpdateChecks.Delete(s)
		return
	}
	selfUpdateChecks.Store(s, fn)
}

func RegisterSelfUpdateStart(s *Server, fn func() error) {
	if s == nil {
		return
	}
	if fn == nil {
		selfUpdateStarts.Delete(s)
		return
	}
	selfUpdateStarts.Store(s, fn)
}

func RegisterSelfUpdateDetach(s *Server, fn func(string) error) {
	if s == nil {
		return
	}
	if fn == nil {
		selfUpdateDetaches.Delete(s)
		return
	}
	selfUpdateDetaches.Store(s, fn)
}

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
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	if r.Method == http.MethodGet {
		if action == "check" {
			value, ok := selfUpdateChecks.Load(s)
			if !ok {
				http.Error(w, "self-update check unavailable", http.StatusServiceUnavailable)
				return
			}
			fn, ok := value.(func(context.Context) (SelfUpdateCheckResult, error))
			if !ok || fn == nil {
				http.Error(w, "self-update check unavailable", http.StatusServiceUnavailable)
				return
			}
			result, err := fn(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		if action != "" {
			http.Error(w, "unknown self-update action", http.StatusBadRequest)
			return
		}
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

	if action == "start" {
		if req, err := updater.Load(s.Workspace); err == nil {
			switch req.Status {
			case "awaiting_confirmation", "confirmed", "downloading", "testing", "building", "installing", "ready_restart", "restarting":
				http.Error(w, "self-update is already in progress", http.StatusConflict)
				return
			}
		} else if !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		value, ok := selfUpdateStarts.Load(s)
		if !ok {
			http.Error(w, "self-update start unavailable", http.StatusServiceUnavailable)
			return
		}
		fn, ok := value.(func() error)
		if !ok || fn == nil {
			http.Error(w, "self-update start unavailable", http.StatusServiceUnavailable)
			return
		}
		lease, ok := s.acquireSharedMutation(w, r, "selfupdate.run", "")
		if !ok {
			return
		}
		if err := fn(); err != nil {
			s.releaseSharedMutation(lease)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "selfupdate.start", "selfupdate", "", nil)
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "status": "starting"})
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

	switch action {
	case "confirm":
		if req.Status != "awaiting_confirmation" {
			http.Error(w, "update is not awaiting confirmation", http.StatusConflict)
			return
		}
		if !s.ensureSharedSelfUpdateMutation(w, r, req.ID) {
			return
		}
		req, err = updater.Update(s.Workspace, req.ID, "confirmed", "Đã xác nhận; chuẩn bị cập nhật…", "", "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "selfupdate.confirm", "selfupdate", req.ID, nil)
		writeJSON(w, http.StatusOK, req)
	case "cancel":
		if req.Status != "awaiting_confirmation" {
			http.Error(w, "update can no longer be cancelled", http.StatusConflict)
			return
		}
		if !s.ensureSharedSelfUpdateMutation(w, r, req.ID) {
			return
		}
		req, err = updater.Update(s.Workspace, req.ID, "cancelled", "Người dùng đã hủy cập nhật.", "", "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "selfupdate.cancel", "selfupdate", req.ID, nil)
		s.sharedMutation.releaseOperation("selfupdate.run")
		writeJSON(w, http.StatusOK, req)
	case "ack":
		switch req.Status {
		case "completed", "failed", "cancelled":
		default:
			http.Error(w, "only terminal self-update states can be acknowledged", http.StatusConflict)
			return
		}
		if !s.ensureSharedSelfUpdateMutation(w, r, req.ID) {
			return
		}
		if err := os.Remove(updater.RequestPath(s.Workspace)); err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "selfupdate.ack", "selfupdate", req.ID, nil)
		s.sharedMutation.releaseOperation("selfupdate.run")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": req.ID})
	case "detach":
		if !s.internalControlRequest(r) {
			http.Error(w, "detach requires authenticated local control", http.StatusForbidden)
			return
		}
		switch req.Status {
		case "ready_restart", "restarting", "failed":
		default:
			http.Error(w, "update is not ready to detach daemon", http.StatusConflict)
			return
		}
		value, ok := selfUpdateDetaches.Load(s)
		if !ok {
			http.Error(w, "daemon detach unavailable", http.StatusServiceUnavailable)
			return
		}
		fn, ok := value.(func(string) error)
		if !ok || fn == nil {
			http.Error(w, "daemon detach unavailable", http.StatusServiceUnavailable)
			return
		}
		if !s.ensureSharedSelfUpdateMutation(w, r, req.ID) {
			return
		}
		if _, err := updater.Update(s.Workspace, req.ID, "restarting", "Detaching old daemon while preserving session broker…", "", ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "selfupdate.detach", "selfupdate", req.ID, nil)
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "id": req.ID})
		go func() {
			time.Sleep(120 * time.Millisecond)
			if err := fn(req.ID); err != nil {
				_, _ = updater.Update(s.Workspace, req.ID, "failed", "Daemon detach failed.", "", err.Error())
			}
		}()
	case "handoff":
		if !s.internalControlRequest(r) {
			http.Error(w, "handoff requires authenticated local control", http.StatusForbidden)
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
		if !s.ensureSharedSelfUpdateMutation(w, r, req.ID) {
			return
		}
		if _, err := updater.Update(s.Workspace, req.ID, "restarting", "Đang chuyển daemon sang binary mới…", "", ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "selfupdate.handoff", "selfupdate", req.ID, nil)
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

func (s *Server) internalControlRequest(r *http.Request) bool {
	if !loopbackRemote(r.RemoteAddr) {
		return false
	}
	expected := strings.TrimSpace(s.InternalControlToken)
	provided := strings.TrimSpace(r.Header.Get(InternalControlHeader))
	if expected == "" || len(expected) != len(provided) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

func (s *Server) ensureSharedSelfUpdateMutation(w http.ResponseWriter, r *http.Request, resourceID string) bool {
	if !s.Config.SharedServerEnabled {
		return true
	}
	s.refreshSharedMutationLock()
	if holder, ok := s.sharedMutation.snapshot(); ok && holder.Operation == "selfupdate.run" {
		if holder.ResourceID == "" && resourceID != "" {
			s.sharedMutation.bindOperationResource("selfupdate.run", resourceID)
		}
		return true
	}
	lease, ok := s.acquireSharedMutation(w, r, "selfupdate.run", resourceID)
	if !ok {
		return false
	}
	if resourceID != "" && !s.sharedMutation.bindResource(lease.token, resourceID) {
		s.releaseSharedMutation(lease)
		http.Error(w, "workspace mutation lock lost during self-update", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *Server) refreshSharedSelfUpdateMutationLock(holder sharedMutationOwner) {
	if holder.Operation != "selfupdate.run" {
		return
	}
	req, err := updater.Load(s.Workspace)
	if os.IsNotExist(err) {
		if time.Since(holder.AcquiredAt) > 30*time.Second {
			s.sharedMutation.releaseOperation("selfupdate.run")
		}
		return
	}
	if err != nil {
		return
	}
	if holder.ResourceID == "" && req.ID != "" {
		s.sharedMutation.bindOperationResource("selfupdate.run", req.ID)
	}
	switch req.Status {
	case "completed", "failed", "cancelled":
		s.sharedMutation.releaseOperation("selfupdate.run")
	}
}
