package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

type sharedAdminUserView struct {
	ID                   identity.ID                          `json:"id"`
	Username             string                               `json:"username"`
	DisplayName          string                               `json:"display_name"`
	UserEnabled          bool                                 `json:"user_enabled"`
	MemberEnabled        bool                                 `json:"member_enabled"`
	RoleID               identity.ID                          `json:"role_id"`
	RoleName             string                               `json:"role_name"`
	LastLoginAt          *time.Time                           `json:"last_login_at,omitempty"`
	PasswordChangedAt    *time.Time                           `json:"password_changed_at,omitempty"`
	PermissionOverrides  map[string]identity.PermissionEffect `json:"permission_overrides"`
	EffectivePermissions []string                             `json:"effective_permissions"`
}

type sharedAdminRoleView struct {
	ID          identity.ID `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	SystemRole  bool        `json:"system_role"`
	Permissions []string    `json:"permissions"`
}

type sharedAdminSessionView struct {
	ID             identity.ID `json:"id"`
	UserID         identity.ID `json:"user_id"`
	Username       string      `json:"username"`
	CreatedAt      time.Time   `json:"created_at"`
	ExpiresAt      time.Time   `json:"expires_at"`
	LastSeenAt     time.Time   `json:"last_seen_at"`
	RevokedAt      *time.Time  `json:"revoked_at,omitempty"`
	ClientMetadata string      `json:"client_metadata,omitempty"`
}

type sharedAdminAuditView struct {
	ID           identity.ID  `json:"id"`
	Timestamp    time.Time    `json:"timestamp"`
	UserID       *identity.ID `json:"user_id,omitempty"`
	ProjectID    *identity.ID `json:"project_id,omitempty"`
	Action       string       `json:"action"`
	ResourceType string       `json:"resource_type,omitempty"`
	ResourceID   string       `json:"resource_id,omitempty"`
	Result       string       `json:"result"`
	ClientIP     string       `json:"client_ip,omitempty"`
	Details      string       `json:"details"`
}

type sharedAdminCreateUserRequest struct {
	Username    string      `json:"username"`
	DisplayName string      `json:"display_name"`
	Password    string      `json:"password"`
	RoleID      identity.ID `json:"role_id"`
}

type sharedAdminUpdateAccessRequest struct {
	UserID  identity.ID `json:"user_id"`
	RoleID  identity.ID `json:"role_id"`
	Enabled *bool       `json:"enabled"`
}

type sharedAdminPermissionRequest struct {
	UserID        identity.ID                `json:"user_id"`
	PermissionKey string                     `json:"permission_key"`
	Effect        identity.PermissionEffect `json:"effect,omitempty"`
}

type sharedAdminRevokeSessionRequest struct {
	SessionID identity.ID `json:"session_id"`
}

type sharedAdminCreateRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type sharedAdminRolePermissionsRequest struct {
	RoleID      identity.ID `json:"role_id"`
	Permissions []string    `json:"permissions"`
}

func (s *Server) sharedAdminReady(w http.ResponseWriter, r *http.Request) bool {
	if !s.Config.SharedServerEnabled {
		http.NotFound(w, r)
		return false
	}
	if s.Identity == nil {
		http.Error(w, "shared administration unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func sharedAdminContext(r *http.Request) (context.Context, context.CancelFunc, identity.Principal, bool) {
	principal, ok := PrincipalFromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	return ctx, cancel, principal, ok
}

func permissionKeys(permissions map[string]bool) []string {
	keys := make([]string, 0, len(permissions))
	for _, definition := range identity.PermissionRegistry() {
		if permissions[definition.Key] {
			keys = append(keys, definition.Key)
		}
	}
	return keys
}

func (s *Server) sharedAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.sharedAdminReady(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.sharedAdminUsersList(w, r)
	case http.MethodPost:
		s.sharedAdminUserCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) sharedAdminUsersList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	if request.UserID == principal.UserID {
		s.appendSharedAudit(r, &principal, nil, "admin.member.update", "user", string(request.UserID), "denied", map[string]any{"reason": "self_modification"})
		http.Error(w, "cannot modify your own project access", http.StatusConflict)
		return
	}
	members, err := s.Identity.ListProjectMembers(ctx, principal.ProjectID)
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	views := make([]sharedAdminUserView, 0, len(members))
	for _, member := range members {
		effective, err := s.Identity.EffectivePermissions(ctx, principal.ProjectID, member.UserID)
		if err != nil {
			sharedAuthError(w, err)
			return
		}
		views = append(views, sharedAdminUserView{
			ID: member.UserID, Username: member.Username, DisplayName: member.DisplayName,
			UserEnabled: member.UserEnabled, MemberEnabled: member.MemberEnabled,
			RoleID: member.RoleID, RoleName: member.RoleName, LastLoginAt: member.LastLoginAt,
			PasswordChangedAt: member.PasswordChangedAt, PermissionOverrides: member.PermissionOverrides,
			EffectivePermissions: permissionKeys(effective),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": views})
}

func (s *Server) sharedAdminUserCreate(w http.ResponseWriter, r *http.Request) {
	if !requireSharedJSON(w, r) {
		return
	}
	var request sharedAdminCreateUserRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	request.Username = strings.TrimSpace(request.Username)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.RoleID = identity.ID(strings.TrimSpace(string(request.RoleID)))
	if err != nil || decoder.Decode(new(any)) != io.EOF ||
		request.Username == "" || len(request.Username) > 128 ||
		!utf8.ValidString(request.Username) || strings.ContainsAny(request.Username, " \t\r\n\x00") ||
		len(request.DisplayName) > 256 || !utf8.ValidString(request.DisplayName) || strings.ContainsRune(request.DisplayName, '\x00') ||
		request.RoleID == "" ||
		!utf8.ValidString(request.Password) || utf8.RuneCountInString(request.Password) < 12 ||
		len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\r\n\x00") {
		request.Password = ""
		http.Error(w, "invalid user request", http.StatusBadRequest)
		return
	}

	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		request.Password = ""
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	roles, err := s.Identity.ListProjectRoles(ctx, principal.ProjectID)
	if err != nil {
		request.Password = ""
		sharedAuthError(w, err)
		return
	}
	roleName := ""
	for _, role := range roles {
		if role.ID == request.RoleID {
			roleName = role.Name
			break
		}
	}
	if roleName == "" {
		request.Password = ""
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	passwordHash, err := identity.HashPassword(ctx, request.Password)
	request.Password = ""
	if err != nil {
		s.appendSharedAudit(r, &principal, nil, "admin.user.create", "user", "", "error", map[string]any{"reason": "password_hash"})
		sharedAuthError(w, err)
		return
	}
	now := time.Now().UTC()
	userID, err := identity.NewID()
	if err != nil {
		s.appendSharedAudit(r, &principal, nil, "admin.user.create", "user", "", "error", map[string]any{"reason": "user_id"})
		sharedAuthError(w, err)
		return
	}
	user := identity.User{
		ID: userID, Username: request.Username, DisplayName: request.DisplayName,
		PasswordHash: passwordHash, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	member := identity.ProjectMember{
		ProjectID: principal.ProjectID, UserID: userID, RoleID: request.RoleID,
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Identity.CreateProjectUser(ctx, user, member); err != nil {
		if errors.Is(err, identity.ErrConflict) {
			s.appendSharedAudit(r, &principal, nil, "admin.user.create", "user", "", "denied", map[string]any{"reason": "username_conflict", "username": request.Username})
			http.Error(w, "username already exists", http.StatusConflict)
			return
		}
		s.appendSharedAudit(r, &principal, nil, "admin.user.create", "user", "", "error", map[string]any{"reason": "identity_store"})
		sharedAuthError(w, err)
		return
	}
	effective, err := s.Identity.EffectivePermissions(ctx, principal.ProjectID, userID)
	if err != nil {
		s.appendSharedAudit(r, &principal, nil, "admin.user.create", "user", string(userID), "error", map[string]any{"reason": "permission_read"})
		sharedAuthError(w, err)
		return
	}
	changedAt := now
	s.appendSharedAudit(r, &principal, nil, "admin.user.create", "user", string(userID), "success", map[string]any{"username": request.Username, "role_id": string(request.RoleID)})
	writeJSON(w, http.StatusCreated, map[string]any{"user": sharedAdminUserView{
		ID: userID, Username: request.Username, DisplayName: request.DisplayName,
		UserEnabled: true, MemberEnabled: true, RoleID: request.RoleID, RoleName: roleName,
		PasswordChangedAt: &changedAt, PermissionOverrides: map[string]identity.PermissionEffect{},
		EffectivePermissions: permissionKeys(effective),
	}})
}

func (s *Server) sharedAdminUserAccess(w http.ResponseWriter, r *http.Request) {
	if !s.sharedAdminReady(w, r) {
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireSharedJSON(w, r) {
		return
	}
	var request sharedAdminUpdateAccessRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	request.UserID = identity.ID(strings.TrimSpace(string(request.UserID)))
	request.RoleID = identity.ID(strings.TrimSpace(string(request.RoleID)))
	if err != nil || decoder.Decode(new(any)) != io.EOF || request.UserID == "" || request.RoleID == "" || request.Enabled == nil {
		http.Error(w, "invalid project access request", http.StatusBadRequest)
		return
	}

	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	members, err := s.Identity.ListProjectMembers(ctx, principal.ProjectID)
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	var current *identity.ProjectMemberDetails
	for i := range members {
		if members[i].UserID == request.UserID {
			current = &members[i]
			break
		}
	}
	if current == nil {
		http.Error(w, "project member not found", http.StatusNotFound)
		return
	}
	roles, err := s.Identity.ListProjectRoles(ctx, principal.ProjectID)
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	validRole := false
	for _, role := range roles {
		if role.ID == request.RoleID {
			validRole = true
			break
		}
	}
	if !validRole {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	member := identity.ProjectMember{
		ProjectID: principal.ProjectID,
		UserID: request.UserID,
		RoleID: request.RoleID,
		Enabled: *request.Enabled,
		CreatedAt: current.MemberCreatedAt,
		UpdatedAt: now,
	}
	if err := s.Identity.UpsertProjectMember(ctx, member); err != nil {
		s.appendSharedAudit(r, &principal, nil, "admin.member.update", "user", string(request.UserID), "error", map[string]any{"reason": "identity_store"})
		sharedAuthError(w, err)
		return
	}
	s.appendSharedAudit(r, &principal, nil, "admin.member.update", "user", string(request.UserID), "success", map[string]any{"role_id": string(request.RoleID), "enabled": *request.Enabled})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sharedAdminUserPermission(w http.ResponseWriter, r *http.Request) {
	if !s.sharedAdminReady(w, r) {
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireSharedJSON(w, r) {
		return
	}
	var request sharedAdminPermissionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	request.UserID = identity.ID(strings.TrimSpace(string(request.UserID)))
	request.PermissionKey = strings.TrimSpace(request.PermissionKey)
	if err != nil || decoder.Decode(new(any)) != io.EOF || request.UserID == "" || !identity.KnownPermission(request.PermissionKey) {
		http.Error(w, "invalid member permission request", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodPut && request.Effect != identity.PermissionAllow && request.Effect != identity.PermissionDeny {
		http.Error(w, "invalid member permission effect", http.StatusBadRequest)
		return
	}

	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	if request.UserID == principal.UserID {
		action := "admin.member_permission.set"
		if r.Method == http.MethodDelete {
			action = "admin.member_permission.delete"
		}
		s.appendSharedAudit(r, &principal, nil, action, "user", string(request.UserID), "denied", map[string]any{"reason": "self_modification", "permission": request.PermissionKey})
		http.Error(w, "cannot modify your own permission overrides", http.StatusConflict)
		return
	}
	if _, err := s.Identity.ProjectMember(ctx, principal.ProjectID, request.UserID); err != nil {
		if errors.Is(err, identity.ErrNotFound) {
			http.Error(w, "project member not found", http.StatusNotFound)
			return
		}
		sharedAuthError(w, err)
		return
	}
	permissionID := identity.ID(request.PermissionKey)
	action := "admin.member_permission.set"
	details := map[string]any{"permission": request.PermissionKey}
	if r.Method == http.MethodDelete {
		action = "admin.member_permission.delete"
		err = s.Identity.DeleteMemberPermission(ctx, principal.ProjectID, request.UserID, permissionID)
		if errors.Is(err, identity.ErrNotFound) {
			http.Error(w, "member permission override not found", http.StatusNotFound)
			return
		}
	} else {
		details["effect"] = string(request.Effect)
		err = s.Identity.SetMemberPermission(ctx, identity.MemberPermission{
			ProjectID: principal.ProjectID,
			UserID: request.UserID,
			PermissionID: permissionID,
			Effect: request.Effect,
		})
	}
	if err != nil {
		s.appendSharedAudit(r, &principal, nil, action, "user", string(request.UserID), "error", map[string]any{"permission": request.PermissionKey})
		sharedAuthError(w, err)
		return
	}
	s.appendSharedAudit(r, &principal, nil, action, "user", string(request.UserID), "success", details)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sharedAdminRoles(w http.ResponseWriter, r *http.Request) {
	if !s.sharedAdminReady(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.sharedAdminRolesList(w, r)
	case http.MethodPost:
		s.sharedAdminRoleCreate(w, r)
	case http.MethodPatch:
		s.sharedAdminRolePermissionsUpdate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) sharedAdminRolesList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	roles, err := s.Identity.ListProjectRoles(ctx, principal.ProjectID)
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	views := make([]sharedAdminRoleView, 0, len(roles))
	for _, role := range roles {
		views = append(views, sharedAdminRoleView{ID: role.ID, Name: role.Name, Description: role.Description, SystemRole: role.SystemRole, Permissions: role.Permissions})
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": views})
}

func (s *Server) sharedAdminRoleCreate(w http.ResponseWriter, r *http.Request) {
	if !requireSharedJSON(w, r) {
		return
	}
	var request sharedAdminCreateRoleRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if err != nil || decoder.Decode(new(any)) != io.EOF ||
		request.Name == "" || len(request.Name) > 128 || !utf8.ValidString(request.Name) || strings.ContainsAny(request.Name, "\r\n\x00") ||
		len(request.Description) > 512 || !utf8.ValidString(request.Description) || strings.ContainsRune(request.Description, '\x00') {
		http.Error(w, "invalid role request", http.StatusBadRequest)
		return
	}
	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	id, err := identity.NewID()
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	roleID := identity.ID("custom:" + string(id))
	role := identity.Role{ID: roleID, Name: request.Name, Description: request.Description}
	if err := s.Identity.CreateProjectRole(ctx, principal.ProjectID, role); err != nil {
		if errors.Is(err, identity.ErrConflict) {
			s.appendSharedAudit(r, &principal, nil, "admin.role.create", "role", "", "denied", map[string]any{"reason": "role_conflict", "name": request.Name})
			http.Error(w, "role name already exists", http.StatusConflict)
			return
		}
		s.appendSharedAudit(r, &principal, nil, "admin.role.create", "role", "", "error", map[string]any{"reason": "identity_store"})
		sharedAuthError(w, err)
		return
	}
	s.appendSharedAudit(r, &principal, nil, "admin.role.create", "role", string(roleID), "success", map[string]any{"name": request.Name})
	writeJSON(w, http.StatusCreated, map[string]any{"role": sharedAdminRoleView{
		ID: roleID, Name: request.Name, Description: request.Description, Permissions: []string{},
	}})
}

func (s *Server) sharedAdminRolePermissionsUpdate(w http.ResponseWriter, r *http.Request) {
	if !requireSharedJSON(w, r) {
		return
	}
	var request sharedAdminRolePermissionsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	request.RoleID = identity.ID(strings.TrimSpace(string(request.RoleID)))
	if err != nil || decoder.Decode(new(any)) != io.EOF || request.RoleID == "" || len(request.Permissions) > len(identity.PermissionRegistry()) {
		http.Error(w, "invalid role permissions request", http.StatusBadRequest)
		return
	}
	for i := range request.Permissions {
		request.Permissions[i] = strings.TrimSpace(request.Permissions[i])
		if !identity.KnownPermission(request.Permissions[i]) {
			http.Error(w, "invalid role permission", http.StatusBadRequest)
			return
		}
	}
	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	err = s.Identity.SetProjectRolePermissions(ctx, principal.ProjectID, request.RoleID, request.Permissions)
	switch {
	case errors.Is(err, identity.ErrNotFound):
		http.Error(w, "project role not found", http.StatusNotFound)
		return
	case errors.Is(err, identity.ErrConflict):
		s.appendSharedAudit(r, &principal, nil, "admin.role.permissions.update", "role", string(request.RoleID), "denied", map[string]any{"reason": "system_role_read_only"})
		http.Error(w, "system roles are read-only", http.StatusConflict)
		return
	case err != nil:
		s.appendSharedAudit(r, &principal, nil, "admin.role.permissions.update", "role", string(request.RoleID), "error", map[string]any{"reason": "identity_store"})
		sharedAuthError(w, err)
		return
	}
	s.appendSharedAudit(r, &principal, nil, "admin.role.permissions.update", "role", string(request.RoleID), "success", map[string]any{"permissions": request.Permissions})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sharedAdminPermissions(w http.ResponseWriter, r *http.Request) {
	if !s.sharedAdminReady(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": identity.PermissionRegistry()})
}

func (s *Server) sharedAdminSessions(w http.ResponseWriter, r *http.Request) {
	if !s.sharedAdminReady(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.sharedAdminSessionsList(w, r)
	case http.MethodDelete:
		s.sharedAdminSessionRevoke(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) sharedAdminSessionsList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	members, err := s.Identity.ListProjectMembers(ctx, principal.ProjectID)
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	usernames := make(map[identity.ID]string, len(members))
	for _, member := range members {
		usernames[member.UserID] = member.Username
	}
	sessions, err := s.Identity.ListAuthSessions(ctx, identity.AuthSessionQuery{ProjectID: principal.ProjectID, IncludeRevoked: r.URL.Query().Get("include_revoked") == "1", Limit: 200})
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	views := make([]sharedAdminSessionView, 0, len(sessions))
	for _, value := range sessions {
		views = append(views, sharedAdminSessionView{
			ID: value.ID, UserID: value.UserID, Username: usernames[value.UserID],
			CreatedAt: value.CreatedAt, ExpiresAt: value.ExpiresAt, LastSeenAt: value.LastSeenAt,
			RevokedAt: value.RevokedAt, ClientMetadata: value.ClientMetadata,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": views})
}

func (s *Server) sharedAdminSessionRevoke(w http.ResponseWriter, r *http.Request) {
	if !requireSharedJSON(w, r) {
		return
	}
	var request sharedAdminRevokeSessionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	request.SessionID = identity.ID(strings.TrimSpace(string(request.SessionID)))
	if err != nil || decoder.Decode(new(any)) != io.EOF || request.SessionID == "" {
		http.Error(w, "invalid session revoke request", http.StatusBadRequest)
		return
	}
	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	session, err := s.Identity.AuthSessionForProject(ctx, principal.ProjectID, request.SessionID)
	if errors.Is(err, identity.ErrNotFound) {
		http.Error(w, "auth session not found", http.StatusNotFound)
		return
	}
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	if session.RevokedAt == nil {
		if err := s.Identity.RevokeAuthSession(ctx, session.ID, time.Now().UTC()); err != nil {
			s.appendSharedAudit(r, &principal, nil, "admin.session.revoke", "auth_session", string(session.ID), "error", map[string]any{"target_user_id": string(session.UserID)})
			sharedAuthError(w, err)
			return
		}
	}
	s.appendSharedAudit(r, &principal, nil, "admin.session.revoke", "auth_session", string(session.ID), "success", map[string]any{"target_user_id": string(session.UserID)})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sharedAdminAudit(w http.ResponseWriter, r *http.Request) {
	if !s.sharedAdminReady(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel, principal, ok := sharedAdminContext(r)
	defer cancel()
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			http.Error(w, "invalid audit limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	query := identity.AuditQuery{ProjectID: principal.ProjectID, UserID: identity.ID(strings.TrimSpace(r.URL.Query().Get("user_id"))), Action: strings.TrimSpace(r.URL.Query().Get("action")), Limit: limit}
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		before, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			http.Error(w, "invalid audit cursor", http.StatusBadRequest)
			return
		}
		query.Before = &before
	}
	events, err := s.Identity.ListAudit(ctx, query)
	if err != nil {
		sharedAuthError(w, err)
		return
	}
	views := make([]sharedAdminAuditView, 0, len(events))
	for _, event := range events {
		views = append(views, sharedAdminAuditView{
			ID: event.ID, Timestamp: event.Timestamp, UserID: event.UserID, ProjectID: event.ProjectID,
			Action: event.Action, ResourceType: event.ResourceType, ResourceID: event.ResourceID,
			Result: event.Result, ClientIP: event.ClientIP, Details: event.Details,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": views})
}
