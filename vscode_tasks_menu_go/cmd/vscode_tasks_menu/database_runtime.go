package main

import (
	"fmt"
	"log"
	"os"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbmysql"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
)

type databaseRuntime struct {
	Registry *dbadapter.Registry
	Sessions *dbsession.Manager
}

func newDatabaseRuntime(logger *log.Logger) (*databaseRuntime, error) {
	registry := dbadapter.NewRegistry()
	sessions, err := dbsession.NewManager(registry, 0)
	if err != nil {
		return nil, err
	}
	runtime := &databaseRuntime{Registry: registry, Sessions: sessions}

	executable, err := os.Executable()
	if err != nil {
		sessions.Close()
		return nil, fmt.Errorf("resolve TaskDeck executable for database adapters: %w", err)
	}
	client, err := dbmysql.FindClient()
	if err != nil {
		if logger != nil {
			logger.Printf("database adapter mysql unavailable: %v", err)
		}
		return runtime, nil
	}
	manifest, err := dbmysql.BuiltinManifest(executable)
	if err != nil {
		sessions.Close()
		return nil, err
	}
	if err := registry.Register(manifest); err != nil {
		sessions.Close()
		return nil, err
	}
	if logger != nil {
		logger.Printf(
			"database adapter=%s client=%s flavor=%s version=%s",
			manifest.ID,
			client.Path,
			client.Flavor,
			client.Version,
		)
	}
	return runtime, nil
}

func (r *databaseRuntime) Close() {
	if r == nil || r.Sessions == nil {
		return
	}
	r.Sessions.Close()
}
