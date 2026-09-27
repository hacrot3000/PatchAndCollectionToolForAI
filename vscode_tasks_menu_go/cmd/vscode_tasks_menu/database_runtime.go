package main

import (
	"fmt"
	"log"
	"os"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbmysql"
	"bletonfc/vscode_tasks_menu/internal/dbredis"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshtunnel"
	"bletonfc/vscode_tasks_menu/internal/state"
)

type databaseRuntime struct {
	Registry *dbadapter.Registry
	Sessions *dbsession.Manager
	Tunnels  *sshtunnel.Manager
	Secrets  secretstore.Store
}

func newDatabaseRuntime(workspace string, logger *log.Logger) (*databaseRuntime, error) {
	registry := dbadapter.NewRegistry()
	sessions, err := dbsession.NewManager(registry, 0)
	if err != nil {
		return nil, err
	}

	secrets, err := secretstore.NewDefaultFileStore()
	if err != nil {
		sessions.Close()
		return nil, fmt.Errorf("initialize connection secret store: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		sessions.Close()
		return nil, fmt.Errorf("resolve TaskDeck executable for database adapters: %w", err)
	}
	tunnels, err := sshtunnel.NewManager(secrets, sshtunnel.Options{
		RuntimeDir:    state.Dir(workspace),
		AskpassHelper: executable,
	})
	if err != nil {
		sessions.Close()
		return nil, fmt.Errorf("initialize SSH tunnel manager: %w", err)
	}

	runtime := &databaseRuntime{
		Registry: registry,
		Sessions: sessions,
		Tunnels:  tunnels,
		Secrets:  secrets,
	}

	redisManifest, err := dbredis.BuiltinManifest(executable)
	if err != nil {
		runtime.Close()
		return nil, err
	}
	if err := registry.Register(redisManifest); err != nil {
		runtime.Close()
		return nil, err
	}
	if logger != nil {
		logger.Printf("database adapter=%s implementation=built-in-go", redisManifest.ID)
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
		runtime.Close()
		return nil, err
	}
	if err := registry.Register(manifest); err != nil {
		runtime.Close()
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
	if r == nil {
		return
	}
	if r.Sessions != nil {
		r.Sessions.Close()
	}
	if r.Tunnels != nil {
		r.Tunnels.Close()
	}
}
