package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func runSharedPasswordReset(ctx context.Context, workspace string, cfg config.Config, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("shared password reset requires a username")
	}
	if err := identity.ValidateNewPassword(password); err != nil {
		return err
	}
	dbPath, err := sharedIdentityDBPath(workspace, cfg)
	if err != nil {
		return err
	}
	store, err := identity.OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	user, err := store.UserByUsername(ctx, username)
	if err != nil {
		return err
	}
	nextHash, err := identity.HashPassword(ctx, password)
	password = ""
	if err != nil {
		return err
	}
	changedAt := time.Now().UTC()
	if err := store.ChangeUserPasswordHash(ctx, user.ID, user.PasswordHash, nextHash, changedAt); err != nil {
		return err
	}
	eventID, err := identity.NewID()
	if err == nil {
		targetID := user.ID
		_ = store.AppendAudit(ctx, identity.AuditEvent{
			ID: eventID, Timestamp: changedAt, UserID: &targetID,
			Action: "identity.password_reset", ResourceType: "user", ResourceID: string(user.ID),
			Result: "success", Details: `{"source":"local_cli"}`,
		})
	}
	return nil
}
