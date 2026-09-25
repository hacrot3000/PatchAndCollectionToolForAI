package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const identityDBName = "identity.db"

// ResolveDBPath returns the durable identity DB path. A configured path must be
// absolute. With no configured path, TaskDeck uses a platform user data
// directory outside any project workspace.
func ResolveDBPath(configured string) (string, error) {
	if value := strings.TrimSpace(configured); value != "" {
		if !filepath.IsAbs(value) {
			return "", fmt.Errorf("identity DB path must be absolute")
		}
		return filepath.Clean(value), nil
	}
	return defaultDBPath()
}

func defaultDBPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve Windows user data directory: %w", err)
		}
		return filepath.Join(base, "TaskDeck", identityDBName), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve macOS user home: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", "TaskDeck", identityDBName), nil
	default:
		base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve user home: %w", err)
			}
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "taskdeck", identityDBName), nil
	}
}

// EnsureDBParent creates the dedicated TaskDeck identity directory when it does
// not exist. Existing configured directories are not chmod'd implicitly because
// they may be managed by a system service/deployment policy.
func EnsureDBParent(dbPath string) error {
	dbPath = filepath.Clean(strings.TrimSpace(dbPath))
	if dbPath == "" || !filepath.IsAbs(dbPath) {
		return fmt.Errorf("identity DB path must be absolute")
	}
	parent := filepath.Dir(dbPath)
	info, err := os.Lstat(parent)
	switch {
	case err == nil:
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("identity DB parent is not a real directory: %s", parent)
		}
		return nil
	case os.IsNotExist(err):
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("create identity DB directory: %w", err)
		}
		return nil
	default:
		return err
	}
}
