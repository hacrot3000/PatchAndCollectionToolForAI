package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func resolveSharedMaintenancePath(workspace, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("maintenance file path is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	// Reuse the identity path boundary: backups contain password/session hashes
	// and must never be placed inside the served workspace.
	return identity.ResolveDBPathForWorkspace(absolute, workspace)
}

func sharedIdentityDBPath(workspace string, cfg config.Config) (string, error) {
	if !cfg.SharedServerEnabled {
		return "", fmt.Errorf("shared-server mode is not enabled for this workspace")
	}
	return identity.ResolveDBPathForWorkspace(cfg.SharedIdentityDB, workspace)
}

func runSharedIdentityBackup(ctx context.Context, workspace string, cfg config.Config, destination string) (string, error) {
	dbPath, err := sharedIdentityDBPath(workspace, cfg)
	if err != nil {
		return "", err
	}
	destination, err = resolveSharedMaintenancePath(workspace, destination)
	if err != nil {
		return "", err
	}
	if err := identity.BackupSQLiteFile(ctx, dbPath, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func runSharedIdentityRestore(ctx context.Context, workspace string, cfg config.Config, source string) (string, string, error) {
	dbPath, err := sharedIdentityDBPath(workspace, cfg)
	if err != nil {
		return "", "", err
	}
	source, err = resolveSharedMaintenancePath(workspace, source)
	if err != nil {
		return "", "", err
	}
	safetyPath, err := identity.RestoreSQLiteFile(ctx, dbPath, source, time.Now().UTC())
	if err != nil {
		return source, safetyPath, err
	}
	return source, safetyPath, nil
}
