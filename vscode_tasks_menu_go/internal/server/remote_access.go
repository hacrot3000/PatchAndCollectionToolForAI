package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/remoteportal"
)

// remoteAccessConfig exposes only administrative pairing and presence config.
// It NEVER exposes TaskDeck HTTP/WS handlers or accepts remote commands.
func (s *Server) remoteAccessConfig(w http.ResponseWriter, r *http.Request) {
	// The shared authorization middleware is also enforced in this handler,
	// so direct unit calls cannot bypass administrative access restrictions.
	if !s.requireSharedActionPermission(w, r, identity.PermissionProjectAdmin,
		"remote_access.config", s.Workspace) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		status := map[string]any{
			"enabled": s.Config.RemoteAccessEnabled,
			"portal_url": s.Config.RemotePortalURL,
			"relay_url": s.Config.RemoteRelayURL,
			"portal_public_key":s.Config.RemotePortalPublicKey,
			"authentication_required": true,
			"remote_control_available": false,
		}
		if s.Config.RemoteAccessEnabled {
			path, err := remoteportal.DefaultIdentityPath(s.Workspace)
			if err != nil {
				http.Error(w, "remote identity unavailable", http.StatusServiceUnavailable)
				return
			}
			ident, err := remoteportal.LoadOrCreateIdentity(path)
			if err != nil {
				http.Error(w, "remote identity unavailable", http.StatusServiceUnavailable)
				return
			}
			status["device_id"] = ident.DeviceID()
		}
		writeJSON(w, http.StatusOK, status)
	case http.MethodPut:
		var requested config.RemoteSettings
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&requested); err != nil {
			http.Error(w, "invalid remote settings JSON", http.StatusBadRequest)
			return
		}
		if err := config.SetRemoteSettings(s.Workspace, requested); err != nil {
			http.Error(w, "invalid remote settings or configuration conflict", http.StatusConflict)
			return
		}
		s.auditSharedSuccess(r, "settings.remote_access.update", "setting", "remote_access", map[string]any{
			"enabled": requested.Enabled,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"saved": true, "enabled": requested.Enabled,
			"portal_url": requested.PortalURL,
			"relay_url": requested.RelayURL,
			"portal_public_key":requested.PortalPublicKey,
			"restart_required": true, "remote_control_available": false,
		})
		// Existing daemon restart mechanism preserves the independent broker.
		// The new config is not active until the process reloads.
		s.scheduleAuthenticationModeReload()
	case http.MethodPost:
		if !s.Config.RemoteAccessEnabled ||
			(!s.Config.SharedServerEnabled && !s.Config.AuthEnabled) {
			http.Error(w, "remote presence is disabled or authentication is not enabled", http.StatusConflict)
			return
		}
		var request struct {
			Action string `json:"action"`
			Name   string `json:"name"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || request.Action != "enroll" {
			http.Error(w, "expected action=enroll", http.StatusBadRequest)
			return
		}
		key, err := remoteportal.DefaultIdentityPath(s.Workspace)
		if err != nil {
			http.Error(w, "remote identity unavailable", http.StatusServiceUnavailable)
			return
		}
		agent, err := remoteportal.NewAgent(s.Config.RemotePortalURL, key, nil)
		if err != nil {
			http.Error(w, "remote agent unavailable", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		code, err := agent.Enroll(ctx, request.Name)
		if err != nil {
			// Do not expose transport URLs or credentials through error strings.
			http.Error(w, "portal enrollment rejected", http.StatusBadGateway)
			return
		}
		s.auditSharedSuccess(r, "settings.remote_access.enroll", "setting", "remote_access", nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id": agent.DeviceID(),
			"pairing_code": code,
			"expires_in_seconds": 300,
		})
	default:
		w.Header().Set("Allow", "GET, PUT, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
