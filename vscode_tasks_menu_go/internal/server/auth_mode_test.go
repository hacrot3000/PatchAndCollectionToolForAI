package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func authMigrationWorkspace(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	return workspace, filepath.Join(root, "identity", "identity.db")
}

func TestSingleAuthMigratesToSharedIdentityAndScrubsBasicPassword(t *testing.T) {
	workspace, dbPath := authMigrationWorkspace(t)
	cfg := config.Default()
	cfg.Protocol = config.ProtocolHTTPS
	cfg.Port = 8443
	cfg.AuthEnabled = true
	cfg.Username = "legacy-admin"
	cfg.Password = "legacy-password-123"
	cfg.SharedServerEnabled = false
	cfg.SharedProjectID = ""
	cfg.SharedIdentityDB = ""
	if err := config.WriteAuthenticationMode(workspace, cfg); err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace, Config: cfg}
	req := httptest.NewRequest("POST", "/api/config/auth-mode", nil)
	resp, err := s.migrateSingleToShared(req, authModeMigrationRequest{
		TargetMode: authModeShared,
		ProjectID:  "project-one",
		IdentityDB: dbPath,
		Username:   "shared-admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Mode != authModeShared || resp.ProjectID != "project-one" || resp.Username != "shared-admin" {
		t.Fatalf("unexpected migration response: %#v", resp)
	}

	loaded, _, err := config.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.SharedServerEnabled || loaded.AuthEnabled || loaded.Protocol != config.ProtocolHTTPS {
		t.Fatalf("shared config not activated: %+v", loaded)
	}
	if loaded.Password != "change-me" {
		t.Fatalf("legacy Basic Auth plaintext was not scrubbed: password=%q", loaded.Password)
	}
	if loaded.SharedProjectID != "project-one" || loaded.SharedIdentityDB != dbPath {
		t.Fatalf("shared config lost project/db settings: %+v", loaded)
	}

	ctx := context.Background()
	store, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	user, err := store.UserByUsername(ctx, "shared-admin")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := identity.VerifyPassword(ctx, "legacy-password-123", user.PasswordHash)
	if err != nil || !ok {
		t.Fatalf("migrated shared password verification ok=%v err=%v", ok, err)
	}
	project, err := store.ProjectByKey(ctx, "project-one")
	if err != nil {
		t.Fatal(err)
	}
	member, err := store.ProjectMember(ctx, project.ID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !member.Enabled || member.RoleID != "system:admin" {
		t.Fatalf("migrated user is not project admin: %#v", member)
	}
}

func TestSingleToSharedDoesNotOverwriteExistingGlobalUserPassword(t *testing.T) {
	workspace, dbPath := authMigrationWorkspace(t)
	cfg := config.Default()
	cfg.Protocol = config.ProtocolHTTPS
	cfg.Port = 8443
	cfg.AuthEnabled = true
	cfg.Username = "alice"
	cfg.Password = "legacy-password-123"
	if err := config.WriteAuthenticationMode(workspace, cfg); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := identity.HashPassword(ctx, "different-password-456")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BootstrapFirstAdmin(ctx, "other-project", "alice", hash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace, Config: cfg}
	req := httptest.NewRequest("POST", "/api/config/auth-mode", nil)
	_, err = s.migrateSingleToShared(req, authModeMigrationRequest{
		ProjectID: "project-one", IdentityDB: dbPath, Username: "alice",
	})
	var conflict authModeConflictError
	if err == nil || !errorsAsAuthConflict(err, &conflict) {
		t.Fatalf("existing global identity password mismatch should conflict, err=%v", err)
	}

	loaded, _, loadErr := config.Load(workspace)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if loaded.SharedServerEnabled {
		t.Fatal("failed shared identity verification must not switch authentication mode")
	}
}

func TestSharedAuthMigratesToSingleAndRevokesProjectSessions(t *testing.T) {
	workspace, dbPath := authMigrationWorkspace(t)
	ctx := context.Background()
	store, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, err := identity.HashPassword(ctx, "shared-password-123")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	principal, err := store.BootstrapFirstAdmin(ctx, "project-one", "alice", hash, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSystemRoles(ctx); err != nil {
		t.Fatal(err)
	}
	_, session, err := identity.CreateBrowserSession(ctx, store, principal.ProjectID, principal.UserID, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Protocol = config.ProtocolHTTPS
	cfg.Port = 8443
	cfg.AuthEnabled = false
	cfg.Username = "admin"
	cfg.Password = "change-me"
	cfg.SharedServerEnabled = true
	cfg.SharedProjectID = "project-one"
	cfg.SharedIdentityDB = dbPath
	if err := config.WriteAuthenticationMode(workspace, cfg); err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace, Config: cfg, Identity: store}
	req := httptest.NewRequest("POST", "/api/config/auth-mode", nil)
	resp, err := s.migrateSharedToSingle(req, authModeMigrationRequest{
		TargetMode: authModeSingle,
		Username:   "local-admin",
		Password:   "new-local-password-789",
	}, principal)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Mode != authModeSingle || resp.Username != "local-admin" {
		t.Fatalf("unexpected single migration response: %#v", resp)
	}
	loaded, _, err := config.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SharedServerEnabled || !loaded.AuthEnabled || loaded.Username != "local-admin" || loaded.Password != "new-local-password-789" {
		t.Fatalf("single auth config not activated: %+v", loaded)
	}
	if loaded.SharedProjectID != "project-one" || loaded.SharedIdentityDB != dbPath {
		t.Fatalf("shared identity settings should remain available for switching back: %+v", loaded)
	}

	sessions, err := store.ListAuthSessions(ctx, identity.AuthSessionQuery{
		ProjectID: principal.ProjectID, IncludeRevoked: true, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range sessions {
		if item.ID == session.ID {
			found = true
			if item.RevokedAt == nil {
				t.Fatalf("shared browser session was not revoked: %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("created shared browser session %s not found after migration", session.ID)
	}
}

func TestAuthenticationModeStatusNeverReturnsPlaintextPassword(t *testing.T) {
	workspace, _ := authMigrationWorkspace(t)
	cfg := config.Default()
	cfg.AuthEnabled = true
	cfg.Username = "admin"
	cfg.Password = "top-secret-password"
	s := &Server{Workspace: workspace, Config: cfg}
	status, err := s.authenticationModeStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Username != "admin" || !status.LegacyCredentialReusable {
		t.Fatalf("unexpected auth mode status: %#v", status)
	}
}

func errorsAsAuthConflict(err error, target *authModeConflictError) bool {
	if value, ok := err.(authModeConflictError); ok {
		*target = value
		return true
	}
	return false
}
