package dbsqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

const (
	defaultBusyTimeoutMS = 5000
	maxBusyTimeoutMS     = 300000
)

type Config struct {
	File          string
	ReadOnly      bool
	BusyTimeoutMS int
}

func ConfigFromConnectPayload(payload dbadapter.ConnectPayload) (Config, error) {
	if strings.TrimSpace(payload.Host) != "" || payload.Port != 0 ||
		strings.TrimSpace(payload.Username) != "" || payload.Secret != "" ||
		strings.TrimSpace(payload.Database) != "" {
		return Config{}, errors.New("SQLite adapter accepts only a local file path, read-only mode and SQLite options")
	}
	rawFile := strings.TrimSpace(payload.File)
	if rawFile == "" {
		return Config{}, errors.New("SQLite database file is required")
	}
	if strings.ContainsRune(rawFile, '\x00') {
		return Config{}, errors.New("SQLite database file contains NUL")
	}
	absolute, err := filepath.Abs(rawFile)
	if err != nil {
		return Config{}, fmt.Errorf("resolve SQLite database file: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return Config{}, fmt.Errorf("resolve SQLite database file symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return Config{}, fmt.Errorf("stat SQLite database file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Config{}, errors.New("SQLite database path must be a regular file")
	}

	config := Config{
		File:          filepath.Clean(resolved),
		ReadOnly:      payload.ReadOnly,
		BusyTimeoutMS: defaultBusyTimeoutMS,
	}
	for rawKey, rawValue := range payload.Options {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		value := strings.TrimSpace(rawValue)
		switch key {
		case "busy_timeout_ms":
			timeout, err := strconv.Atoi(value)
			if err != nil || timeout < 0 || timeout > maxBusyTimeoutMS {
				return Config{}, fmt.Errorf("SQLite busy_timeout_ms must be between 0 and %d", maxBusyTimeoutMS)
			}
			config.BusyTimeoutMS = timeout
		default:
			return Config{}, fmt.Errorf("unsupported SQLite option %q", rawKey)
		}
	}
	return config, nil
}
