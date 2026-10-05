package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
	runtimestate "bletonfc/vscode_tasks_menu/internal/state"
)

const (
	authModeSingle = "single"
	authModeShared = "shared"
)

type authModeStatus struct {
	Mode                     string `json:"mode"`
	Protocol                 string `json:"protocol"`
	Port                     int    `json:"port"`
	AuthEnabled              bool   `json:"auth_enabled"`
	Username                 string `json:"username,omitempty"`
	SharedProjectID          string `json:"shared_project_id,omitempty"`
	SharedIdentityDB         string `json:"shared_identity_db,omitempty"`
	ResolvedIdentityDB       string `json:"resolved_identity_db,omitempty"`
	SuggestedProjectID       string `json:"suggested_project_id"`
	LegacyCredentialReusable bool   `json:"legacy_credential_reusable"`
	CanMigrate               bool   `json:"can_migrate"`
	AutomaticRestart         bool   `json:"automatic_restart"`
	RestartPreservesSessions bool   `json:"restart_preserves_sessions"`
}

type authModeMigrationRequest struct {
	TargetMode string `json:"target_mode"`
	ProjectID  string `json:"project_id,omitempty"`
	IdentityDB string `json:"identity_db,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
}

type authModeMigrationResponse struct {
	Mode                     string `json:"mode"`
	Protocol                 string `json:"protocol"`
	ProjectID                string `json:"project_id,omitempty"`
	IdentityDB               string `json:"identity_db,omitempty"`
	Username                 string `json:"username"`
	RestartScheduled         bool   `json:"restart_scheduled"`
	RestartRequired          bool   `json:"restart_required"`
	RestartPreservesSessions bool   `json:"restart_preserves_sessions"`
	LoginPath                string `json:"login_path"`
	Message                  string `json:"message"`
}

func (s *Server) authModeConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status, err := s.authenticationModeStatus()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if s.Config.SharedServerEnabled {
			principal, ok := PrincipalFromContext(r.Context())
			status.CanMigrate = ok && principal.Allowed(identity.PermissionProjectAdmin)
		}
		writeJSON(w, http.StatusOK, status)
	case http.MethodPost:
		var request authModeMigrationRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid authentication migration JSON", http.StatusBadRequest)
			return
		}
		request.TargetMode = strings.ToLower(strings.TrimSpace(request.TargetMode))
		switch request.TargetMode {
		case authModeShared:
			if s.Config.SharedServerEnabled {
				http.Error(w, "TaskDeck is already using shared-server authentication", http.StatusConflict)
				return
			}
			response, err := s.migrateSingleToShared(r, request)
			request.Password = ""
			if err != nil {
				writeAuthModeMigrationError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response)
			if response.RestartScheduled {
				s.scheduleAuthenticationModeReload()
			}
		case authModeSingle:
			if !s.Config.SharedServerEnabled {
				http.Error(w, "TaskDeck is already using single authentication", http.StatusConflict)
				return
			}
			principal, ok := PrincipalFromContext(r.Context())
			if !ok || !principal.Allowed(identity.PermissionProjectAdmin) {
				writePermissionDenied(w)
				return
			}
			response, err := s.migrateSharedToSingle(r, request, principal)
			request.Password = ""
			if err != nil {
				writeAuthModeMigrationError(w, err)
				return
			}
			s.auditSharedSuccess(r, "settings.auth_mode.migrate_to_single", "setting", "auth_mode", map[string]any{
				"username": response.Username,
			})
			writeJSON(w, http.StatusOK, response)
			if response.RestartScheduled {
				s.scheduleAuthenticationModeReload()
			}
		default:
			request.Password = ""
			http.Error(w, "target_mode must be single or shared", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) authenticationModeStatus() (authModeStatus, error) {
	mode := authModeSingle
	if s.Config.SharedServerEnabled {
		mode = authModeShared
	}
	configuredDB := strings.TrimSpace(s.Config.SharedIdentityDB)
	resolvedDB, err := identity.ResolveDBPathForWorkspace(configuredDB, s.Workspace)
	if err != nil {
		return authModeStatus{}, err
	}
	reusable := s.Config.AuthEnabled && strings.TrimSpace(s.Config.Username) != "" && identity.ValidateNewPassword(s.Config.Password) == nil
	return authModeStatus{
		Mode:                     mode,
		Protocol:                 strings.ToLower(strings.TrimSpace(s.Config.Protocol)),
		Port:                     s.Config.Port,
		AuthEnabled:              s.Config.AuthEnabled,
		Username:                 strings.TrimSpace(s.Config.Username),
		SharedProjectID:          strings.TrimSpace(s.Config.SharedProjectID),
		SharedIdentityDB:         configuredDB,
		ResolvedIdentityDB:       resolvedDB,
		SuggestedProjectID:       suggestedSharedProjectID(s.Workspace),
		LegacyCredentialReusable: reusable,
		CanMigrate:               !s.Config.SharedServerEnabled,
		AutomaticRestart:         runtime.GOOS != "windows",
		RestartPreservesSessions: runtime.GOOS != "windows",
	}, nil
}

func (s *Server) migrateSingleToShared(r *http.Request, request authModeMigrationRequest) (authModeMigrationResponse, error) {
	projectKey := strings.TrimSpace(request.ProjectID)
	if projectKey == "" {
		projectKey = strings.TrimSpace(s.Config.SharedProjectID)
	}
	if projectKey == "" {
		projectKey = suggestedSharedProjectID(s.Workspace)
	}
	username := strings.TrimSpace(request.Username)
	if username == "" {
		username = strings.TrimSpace(s.Config.Username)
	}
	if err := validateMigrationUsername(username); err != nil {
		return authModeMigrationResponse{}, authModeClientError{err}
	}
	password := request.Password
	if password == "" && s.Config.AuthEnabled && username == strings.TrimSpace(s.Config.Username) && identity.ValidateNewPassword(s.Config.Password) == nil {
		password = s.Config.Password
	}
	if err := identity.ValidateNewPassword(password); err != nil {
		return authModeMigrationResponse{}, authModeClientError{fmt.Errorf("shared admin password: %w", err)}
	}

	configuredDB := strings.TrimSpace(request.IdentityDB)
	if configuredDB == "" {
		configuredDB = strings.TrimSpace(s.Config.SharedIdentityDB)
	}
	dbPath, err := identity.ResolveDBPathForWorkspace(configuredDB, s.Workspace)
	if err != nil {
		return authModeMigrationResponse{}, authModeClientError{err}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	store, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		return authModeMigrationResponse{}, fmt.Errorf("open shared identity DB: %w", err)
	}
	defer store.Close()
	if err := store.SeedSystemRoles(ctx); err != nil {
		return authModeMigrationResponse{}, fmt.Errorf("seed shared identity roles: %w", err)
	}

	now := time.Now().UTC()
	project, err := store.ProjectByKey(ctx, projectKey)
	switch {
	case errors.Is(err, identity.ErrNotFound):
		projectID, idErr := identity.NewID()
		if idErr != nil {
			return authModeMigrationResponse{}, idErr
		}
		project, err = store.EnsureProject(ctx, identity.Project{
			ID: projectID, Key: projectKey, DisplayName: projectKey,
			Enabled: true, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return authModeMigrationResponse{}, fmt.Errorf("create shared project: %w", err)
		}
	case err != nil:
		return authModeMigrationResponse{}, fmt.Errorf("lookup shared project: %w", err)
	case !project.Enabled:
		return authModeMigrationResponse{}, authModeConflictError{fmt.Errorf("shared project %q already exists but is disabled", projectKey)}
	}

	user, err := store.UserByUsername(ctx, username)
	switch {
	case errors.Is(err, identity.ErrNotFound):
		hash, hashErr := identity.HashPassword(ctx, password)
		password = ""
		if hashErr != nil {
			return authModeMigrationResponse{}, fmt.Errorf("hash shared admin password: %w", hashErr)
		}
		userID, idErr := identity.NewID()
		if idErr != nil {
			return authModeMigrationResponse{}, idErr
		}
		user = identity.User{
			ID: userID, Username: username, PasswordHash: hash,
			Enabled: true, CreatedAt: now, UpdatedAt: now,
		}
		changed := now
		user.PasswordChangedAt = &changed
		if err := store.CreateProjectUser(ctx, user, identity.ProjectMember{
			ProjectID: project.ID, UserID: user.ID, RoleID: "system:admin",
			Enabled: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return authModeMigrationResponse{}, fmt.Errorf("create shared admin: %w", err)
		}
	case err != nil:
		password = ""
		return authModeMigrationResponse{}, fmt.Errorf("lookup shared admin: %w", err)
	default:
		verified, verifyErr := identity.VerifyPassword(ctx, password, user.PasswordHash)
		password = ""
		if verifyErr != nil {
			return authModeMigrationResponse{}, fmt.Errorf("verify existing shared admin: %w", verifyErr)
		}
		if !verified {
			return authModeMigrationResponse{}, authModeConflictError{fmt.Errorf("username %q already exists in the shared identity DB with a different password; use that account password or choose another username", username)}
		}
		if !user.Enabled {
			return authModeMigrationResponse{}, authModeConflictError{fmt.Errorf("shared identity %q is disabled", username)}
		}
		createdAt := now
		if member, memberErr := store.ProjectMember(ctx, project.ID, user.ID); memberErr == nil {
			createdAt = member.CreatedAt
		} else if !errors.Is(memberErr, identity.ErrNotFound) {
			return authModeMigrationResponse{}, fmt.Errorf("lookup shared project membership: %w", memberErr)
		}
		if err := store.UpsertProjectMember(ctx, identity.ProjectMember{
			ProjectID: project.ID, UserID: user.ID, RoleID: "system:admin",
			Enabled: true, CreatedAt: createdAt, UpdatedAt: now,
		}); err != nil {
			return authModeMigrationResponse{}, fmt.Errorf("grant shared project admin membership: %w", err)
		}
	}

	next := s.Config
	next.Protocol = config.ProtocolHTTPS
	next.Port = s.authMigrationStablePort()
	next.AuthEnabled = false
	next.Username = username
	next.Password = "change-me"
	next.SharedServerEnabled = true
	next.SharedProjectID = projectKey
	next.SharedIdentityDB = configuredDB
	if err := config.WriteAuthenticationMode(s.Workspace, next); err != nil {
		return authModeMigrationResponse{}, fmt.Errorf("write shared-server authentication config: %w", err)
	}
	scheduled := runtime.GOOS != "windows"
	return authModeMigrationResponse{
		Mode: authModeShared, Protocol: config.ProtocolHTTPS,
		ProjectID: projectKey, IdentityDB: dbPath, Username: username,
		RestartScheduled: scheduled, RestartRequired: !scheduled,
		RestartPreservesSessions: scheduled,
		LoginPath: "/login",
		Message: "Shared-server authentication is configured. The web daemon will restart while the session broker remains running.",
	}, nil
}

func (s *Server) migrateSharedToSingle(r *http.Request, request authModeMigrationRequest, principal identity.Principal) (authModeMigrationResponse, error) {
	username := strings.TrimSpace(request.Username)
	if username == "" {
		username = strings.TrimSpace(principal.Username)
	}
	if err := validateMigrationUsername(username); err != nil {
		return authModeMigrationResponse{}, authModeClientError{err}
	}
	if err := identity.ValidateNewPassword(request.Password); err != nil {
		return authModeMigrationResponse{}, authModeClientError{fmt.Errorf("single-auth password: %w", err)}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	revoker, ok := s.Identity.(identity.ProjectSessionRevoker)
	if !ok {
		return authModeMigrationResponse{}, fmt.Errorf("shared identity store cannot revoke project sessions")
	}
	if err := revoker.RevokeProjectSessions(ctx, principal.ProjectID, time.Now().UTC()); err != nil {
		return authModeMigrationResponse{}, fmt.Errorf("revoke shared browser sessions: %w", err)
	}

	next := s.Config
	next.Port = s.authMigrationStablePort()
	next.AuthEnabled = true
	next.Username = username
	next.Password = request.Password
	next.SharedServerEnabled = false
	if err := config.WriteAuthenticationMode(s.Workspace, next); err != nil {
		return authModeMigrationResponse{}, fmt.Errorf("write single-auth config: %w", err)
	}
	scheduled := runtime.GOOS != "windows"
	return authModeMigrationResponse{
		Mode: authModeSingle, Protocol: strings.ToLower(strings.TrimSpace(next.Protocol)),
		ProjectID: strings.TrimSpace(next.SharedProjectID),
		IdentityDB: strings.TrimSpace(next.SharedIdentityDB),
		Username: username,
		RestartScheduled: scheduled, RestartRequired: !scheduled,
		RestartPreservesSessions: scheduled,
		LoginPath: "/",
		Message: "Single authentication is configured. Shared identities remain stored for a later switch back.",
	}, nil
}

func (s *Server) authMigrationStablePort() int {
	if s.Config.Port > 0 {
		return s.Config.Port
	}
	if current, err := runtimestate.Load(s.Workspace); err == nil {
		if _, portText, splitErr := net.SplitHostPort(strings.TrimSpace(current.Address)); splitErr == nil {
			if port, atoiErr := strconv.Atoi(portText); atoiErr == nil && port > 0 && port <= 65535 {
				return port
			}
		}
	}
	return s.Config.Port
}

func (s *Server) scheduleAuthenticationModeReload() {
	if runtime.GOOS == "windows" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		if s.Log != nil {
			s.Log.Printf("auth-mode reload executable warning: %v", err)
		}
		return
	}
	workspace := s.Workspace
	logger := s.Log
	go func() {
		time.Sleep(450 * time.Millisecond)
		cmd := exec.Command(exe, "--workspace", workspace, "--reload-config")
		cmd.Stdin = nil
		if err := cmd.Start(); err != nil {
			if logger != nil {
				logger.Printf("auth-mode reload start failed: %v", err)
			}
			return
		}
		_ = cmd.Process.Release()
	}()
}

func validateMigrationUsername(value string) error {
	if value == "" {
		return fmt.Errorf("username is required")
	}
	if len(value) > 128 || strings.ContainsAny(value, "\r\n\x00") || value != strings.TrimSpace(value) {
		return fmt.Errorf("username must be 1-128 characters without surrounding whitespace, newline or NUL")
	}
	return nil
}

func suggestedSharedProjectID(workspace string) string {
	base := strings.TrimSpace(filepath.Base(filepath.Clean(workspace)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "project"
	}
	var builder strings.Builder
	for _, r := range base {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('-')
		}
		if builder.Len() >= 120 {
			break
		}
	}
	value := strings.Trim(builder.String(), "-")
	if value == "" {
		value = "project"
	}
	first := value[0]
	if !((first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')) {
		value = "project-" + value
	}
	if len(value) > 128 {
		value = value[:128]
	}
	return value
}

type authModeClientError struct{ error }
type authModeConflictError struct{ error }

func writeAuthModeMigrationError(w http.ResponseWriter, err error) {
	var client authModeClientError
	if errors.As(err, &client) {
		http.Error(w, client.Error(), http.StatusBadRequest)
		return
	}
	var conflict authModeConflictError
	if errors.As(err, &conflict) {
		http.Error(w, conflict.Error(), http.StatusConflict)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
