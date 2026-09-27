package dbmongo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	AdapterID   = "mongo-mongosh"
	AdapterKind = "mongo"

	probeTimeout = 3 * time.Second
)

type Client struct {
	Path    string
	Version string
}

func FindClient() (Client, error) {
	path, err := exec.LookPath("mongosh")
	if err != nil {
		return Client{}, errors.New("MongoDB Shell not found in PATH; install mongosh")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Client{}, fmt.Errorf("resolve mongosh path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	version, err := ProbeClient(absolute)
	if err != nil {
		return Client{}, err
	}
	return Client{Path: absolute, Version: version}, nil
}

func ProbeClient(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("mongosh path is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", errors.New("mongosh version probe timed out")
	}
	text := strings.TrimSpace(string(output))
	if len(text) > 4096 {
		text = text[:4096]
	}
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("mongosh version probe failed: %s", text)
		}
		return "", fmt.Errorf("mongosh version probe failed: %w", err)
	}
	if text == "" {
		return "", errors.New("mongosh version probe returned no version")
	}
	return text, nil
}

func BuiltinManifest(taskdeckExecutable string) (dbadapter.Manifest, error) {
	taskdeckExecutable = strings.TrimSpace(taskdeckExecutable)
	if taskdeckExecutable == "" {
		return dbadapter.Manifest{}, errors.New("TaskDeck executable path is required for MongoDB adapter")
	}
	absolute, err := filepath.Abs(taskdeckExecutable)
	if err != nil {
		return dbadapter.Manifest{}, fmt.Errorf("resolve TaskDeck executable path: %w", err)
	}
	manifest := dbadapter.Manifest{
		ID:              AdapterID,
		Name:            "MongoDB (mongosh)",
		Kind:            AdapterKind,
		ProtocolVersion: dbadapter.ProtocolVersion,
		Command:         filepath.Clean(absolute),
		Args:            []string{"--db-adapter", "mongo"},
		Capabilities: dbadapter.CapabilitySet{
			Connect:        true,
			Ping:           true,
			ListCatalogs:   true,
			ListObjects:    true,
			DescribeObject: true,
			Execute:        true,
		},
	}
	if err := manifest.Validate(); err != nil {
		return dbadapter.Manifest{}, err
	}
	return manifest, nil
}
