package dbredis

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	AdapterID   = "redis-go"
	AdapterKind = "redis"
)

func BuiltinManifest(taskdeckExecutable string) (dbadapter.Manifest, error) {
	taskdeckExecutable = strings.TrimSpace(taskdeckExecutable)
	if taskdeckExecutable == "" {
		return dbadapter.Manifest{}, errors.New("TaskDeck executable path is required for Redis adapter")
	}
	absolute, err := filepath.Abs(taskdeckExecutable)
	if err != nil {
		return dbadapter.Manifest{}, fmt.Errorf("resolve TaskDeck executable path: %w", err)
	}
	manifest := dbadapter.Manifest{
		ID:              AdapterID,
		Name:            "Redis (built-in Go)",
		Kind:            AdapterKind,
		ProtocolVersion: dbadapter.ProtocolVersion,
		Command:         filepath.Clean(absolute),
		Args:            []string{"--db-adapter", "redis"},
		Capabilities: dbadapter.CapabilitySet{
			Connect:        true,
			Ping:           true,
			ListCatalogs:   true,
			ListObjects:    true,
			DescribeObject: true,
			BrowseRows:     true,
			MutateRows:     true,
			ObjectActions:  true,
			Execute:        true,
		},
	}
	if err := manifest.Validate(); err != nil {
		return dbadapter.Manifest{}, err
	}
	return manifest, nil
}
