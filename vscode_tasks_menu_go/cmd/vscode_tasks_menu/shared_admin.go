package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func readBootstrapPassword(input io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(input, 4099))
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if err := validateBootstrapPassword(password); err != nil {
		return "", err
	}
	return password, nil
}

func validateBootstrapPassword(password string) error {
	return identity.ValidateNewPassword(password)
}

func runSharedAdminBootstrap(workspace string, cfg config.Config, username string, fromStdin bool) error {
	if !cfg.SharedServerEnabled {
		return fmt.Errorf("first enable [shared_server] and configure project_id in the workspace INI")
	}
	path, err := identity.ResolveDBPathForWorkspace(cfg.SharedIdentityDB, workspace)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var password string
	if fromStdin {
		password, err = readBootstrapPassword(os.Stdin)
	} else {
		password, err = identity.PromptNewPassword(ctx, os.Stdin, os.Stderr)
	}
	if err != nil {
		return err
	}
	if err := validateBootstrapPassword(password); err != nil {
		return err
	}
	workCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	hash, err := identity.HashPassword(workCtx, password)
	password = ""
	if err != nil {
		return err
	}
	store, err := identity.OpenSQLiteStore(workCtx, path)
	if err != nil {
		return err
	}
	defer store.Close()
	principal, err := store.BootstrapFirstAdmin(workCtx, cfg.SharedProjectID, username, hash, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Printf("Shared admin %s created for project %s.\n", principal.Username, principal.ProjectKey)
	return nil
}
