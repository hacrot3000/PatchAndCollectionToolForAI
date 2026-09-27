package dbmysql

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
	AdapterID   = "mysql-cli"
	AdapterKind = "mysql"

	clientProbeTimeout = 3 * time.Second
)

type Client struct {
	Path    string
	Flavor  string
	Version string
}

func FindClient() (Client, error) {
	for _, candidate := range []struct {
		name   string
		flavor string
	}{
		{name: "mysql", flavor: "mysql"},
		{name: "mariadb", flavor: "mariadb"},
	} {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return Client{}, fmt.Errorf("resolve %s client path: %w", candidate.name, err)
		}
		absolute = filepath.Clean(absolute)
		version, err := ProbeClient(absolute)
		if err != nil {
			return Client{}, err
		}
		return Client{Path: absolute, Flavor: candidate.flavor, Version: version}, nil
	}
	return Client{}, errors.New("MySQL client not found in PATH; install mysql or mariadb command-line client")
}

func ProbeClient(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("MySQL client path is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", errors.New("MySQL client version probe timed out")
	}
	text := strings.TrimSpace(string(output))
	if len(text) > 4096 {
		text = text[:4096]
	}
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("MySQL client version probe failed: %s", text)
		}
		return "", fmt.Errorf("MySQL client version probe failed: %w", err)
	}
	if text == "" {
		return "", errors.New("MySQL client version probe returned no version")
	}
	return text, nil
}

func BuiltinManifest(taskdeckExecutable string) (dbadapter.Manifest, error) {
	taskdeckExecutable = strings.TrimSpace(taskdeckExecutable)
	if taskdeckExecutable == "" {
		return dbadapter.Manifest{}, errors.New("TaskDeck executable path is required for MySQL adapter")
	}
	absolute, err := filepath.Abs(taskdeckExecutable)
	if err != nil {
		return dbadapter.Manifest{}, fmt.Errorf("resolve TaskDeck executable path: %w", err)
	}
	manifest := dbadapter.Manifest{
		ID:              AdapterID,
		Name:            "MySQL / MariaDB CLI",
		Kind:            AdapterKind,
		ProtocolVersion: dbadapter.ProtocolVersion,
		Command:         filepath.Clean(absolute),
		Args:            []string{"--db-adapter", "mysql"},
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
