package dbsqlite

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
	AdapterID   = "sqlite-python"
	AdapterKind = "sqlite"

	probeTimeout = 3 * time.Second
)

type Python struct {
	Path    string
	Version string
}

func FindPython() (Python, error) {
	var failures []string
	for _, name := range []string{"python3", "python"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return Python{}, fmt.Errorf("resolve %s path: %w", name, err)
		}
		absolute = filepath.Clean(absolute)
		version, err := ProbePython(absolute)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if !strings.HasPrefix(version, "Python 3.") {
			failures = append(failures, fmt.Sprintf("%s is not Python 3: %s", absolute, version))
			continue
		}
		return Python{Path: absolute, Version: version}, nil
	}
	if len(failures) != 0 {
		return Python{}, errors.New(strings.Join(failures, "; "))
	}
	return Python{}, errors.New("Python 3 not found in PATH; SQLite adapter requires Python 3 stdlib sqlite3")
}

func ProbePython(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("Python path is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", errors.New("Python version probe timed out")
	}
	text := strings.TrimSpace(string(output))
	if len(text) > 4096 {
		text = text[:4096]
	}
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("Python version probe failed: %s", text)
		}
		return "", fmt.Errorf("Python version probe failed: %w", err)
	}
	if text == "" {
		return "", errors.New("Python version probe returned no version")
	}
	return text, nil
}

func BuiltinManifest(taskdeckExecutable string) (dbadapter.Manifest, error) {
	taskdeckExecutable = strings.TrimSpace(taskdeckExecutable)
	if taskdeckExecutable == "" {
		return dbadapter.Manifest{}, errors.New("TaskDeck executable path is required for SQLite adapter")
	}
	absolute, err := filepath.Abs(taskdeckExecutable)
	if err != nil {
		return dbadapter.Manifest{}, fmt.Errorf("resolve TaskDeck executable path: %w", err)
	}
	manifest := dbadapter.Manifest{
		ID:              AdapterID,
		Name:            "SQLite (Python stdlib)",
		Kind:            AdapterKind,
		ProtocolVersion: dbadapter.ProtocolVersion,
		Command:         filepath.Clean(absolute),
		Args:            []string{"--db-adapter", "sqlite"},
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
			Cancel:         true,
			Transactions:   true,
			ImportSQL:      true,
		},
	}
	if err := manifest.Validate(); err != nil {
		return dbadapter.Manifest{}, err
	}
	return manifest, nil
}
