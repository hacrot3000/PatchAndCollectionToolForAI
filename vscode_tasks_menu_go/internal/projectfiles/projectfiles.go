package projectfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DirName      = ".vscode"
	managedPrefix = "vscode_tasks_menu."
)

func Dir(workspace string) string {
	return filepath.Join(workspace, DirName)
}

func Path(workspace, name string) string {
	return filepath.Join(Dir(workspace), name)
}

func validateName(name string) error {
	if name == "" || filepath.Base(name) != name || !strings.HasPrefix(name, managedPrefix) {
		return fmt.Errorf("invalid TaskDeck project file name %q", name)
	}
	return nil
}

func EnsureDir(workspace string) error {
	dir := Dir(workspace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return nil
}

// MigrateLegacyFiles moves legacy TaskDeck project files from the workspace
// root into .vscode. Existing files in .vscode always win and are never
// overwritten.
func MigrateLegacyFiles(workspace string) error {
	if err := EnsureDir(workspace); err != nil {
		return err
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return fmt.Errorf("read workspace for TaskDeck migration: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, managedPrefix) || entry.IsDir() {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		target := Path(workspace, name)
		if _, err := os.Lstat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect TaskDeck target %s: %w", target, err)
		}
		legacy := filepath.Join(workspace, name)
		info, err := os.Lstat(legacy)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect legacy TaskDeck file %s: %w", legacy, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := os.Rename(legacy, target); err != nil {
			return fmt.Errorf("move legacy TaskDeck file %s to %s: %w", legacy, target, err)
		}
	}
	return nil
}

// Resolve returns the canonical .vscode path after migrating any legacy
// workspace-root TaskDeck files.
func Resolve(workspace, name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	if err := MigrateLegacyFiles(workspace); err != nil {
		return "", err
	}
	return Path(workspace, name), nil
}
