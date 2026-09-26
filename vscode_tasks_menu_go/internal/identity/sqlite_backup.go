package identity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BackupSQLiteFile creates a private, consistent SQLite snapshot without
// copying the live WAL/database files directly.
func BackupSQLiteFile(ctx context.Context, sourcePath, destinationPath string) error {
	sourcePath, destinationPath, err := validateSQLiteCopyPaths(sourcePath, destinationPath)
	if err != nil {
		return err
	}
	if err := requireRegularSQLiteFile(sourcePath, false); err != nil {
		return fmt.Errorf("backup source: %w", err)
	}
	if _, err := os.Lstat(destinationPath); err == nil {
		return fmt.Errorf("backup destination already exists: %s", destinationPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect backup destination: %w", err)
	}
	if err := ensurePrivateParent(filepath.Dir(destinationPath)); err != nil {
		return err
	}
	file, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create backup destination: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return fmt.Errorf("close backup destination: %w", err)
	}
	if err := runSQLiteBackup(ctx, sourcePath, destinationPath); err != nil {
		_ = os.Remove(destinationPath)
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destinationPath, 0o600); err != nil {
			_ = os.Remove(destinationPath)
			return fmt.Errorf("protect backup destination: %w", err)
		}
	}
	return nil
}

// RestoreSQLiteFile restores a validated SQLite snapshot onto the live identity
// DB using SQLite's backup API. A pre-restore snapshot is always created first
// and its path is returned to the caller.
func RestoreSQLiteFile(ctx context.Context, targetPath, backupPath string, now time.Time) (string, error) {
	targetPath, backupPath, err := validateSQLiteCopyPaths(targetPath, backupPath)
	if err != nil {
		return "", err
	}
	if now.IsZero() {
		return "", fmt.Errorf("restore timestamp is required")
	}
	if err := requireRegularSQLiteFile(targetPath, true); err != nil {
		return "", fmt.Errorf("restore target: %w", err)
	}
	if err := requireRegularSQLiteFile(backupPath, false); err != nil {
		return "", fmt.Errorf("restore source: %w", err)
	}
	safetyPath, err := nextPreRestoreBackupPath(targetPath, now.UTC())
	if err != nil {
		return "", err
	}
	if err := BackupSQLiteFile(ctx, targetPath, safetyPath); err != nil {
		return "", fmt.Errorf("create pre-restore identity backup: %w", err)
	}
	if err := runSQLiteBackup(ctx, backupPath, targetPath); err != nil {
		if rollbackErr := runSQLiteBackup(context.Background(), safetyPath, targetPath); rollbackErr != nil {
			return safetyPath, fmt.Errorf("restore identity DB: %v; rollback from %s failed: %w", err, safetyPath, rollbackErr)
		}
		return safetyPath, fmt.Errorf("restore identity DB: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(targetPath, 0o600); err != nil {
			return safetyPath, fmt.Errorf("protect restored identity DB: %w", err)
		}
	}
	return safetyPath, nil
}

func validateSQLiteCopyPaths(first, second string) (string, string, error) {
	first = filepath.Clean(strings.TrimSpace(first))
	second = filepath.Clean(strings.TrimSpace(second))
	if first == "." || second == "." || !filepath.IsAbs(first) || !filepath.IsAbs(second) {
		return "", "", fmt.Errorf("SQLite backup paths must be absolute")
	}
	firstAbs, err := filepath.Abs(first)
	if err != nil {
		return "", "", err
	}
	secondAbs, err := filepath.Abs(second)
	if err != nil {
		return "", "", err
	}
	if filepath.Clean(firstAbs) == filepath.Clean(secondAbs) {
		return "", "", fmt.Errorf("SQLite source and destination must differ")
	}
	return filepath.Clean(firstAbs), filepath.Clean(secondAbs), nil
}

func requireRegularSQLiteFile(path string, enforcePrivate bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("path is not a regular file: %s", path)
	}
	if enforcePrivate && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("file permissions are too broad: %s mode=%#o", path, info.Mode().Perm())
	}
	return nil
}

func ensurePrivateParent(parent string) error {
	info, err := os.Lstat(parent)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("backup parent is not a real directory: %s", parent)
		}
		return nil
	case os.IsNotExist(err):
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
		return nil
	default:
		return err
	}
}

func nextPreRestoreBackupPath(targetPath string, now time.Time) (string, error) {
	stamp := now.UTC().Format("20060102-150405")
	base := targetPath + ".pre-restore-" + stamp
	for i := 0; i < 100; i++ {
		candidate := base + ".db"
		if i > 0 {
			candidate = fmt.Sprintf("%s-%02d.db", base, i)
		}
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("cannot allocate pre-restore backup path")
}

func runSQLiteBackup(ctx context.Context, sourcePath, destinationPath string) error {
	executable, prefix, err := resolvePythonSQLiteCommand()
	if err != nil {
		return err
	}
	args := append(append([]string(nil), prefix...), "-u", "-c", pythonSQLiteBackupHelper, sourcePath, destinationPath)
	cmd := exec.CommandContext(ctx, executable, args...)
	stderr := &boundedTextBuffer{limit: 16 * 1024}
	cmd.Stderr = stderr
	if output, err := cmd.Output(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(string(output))
		}
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("SQLite online backup failed: %s", detail)
	}
	return nil
}

const pythonSQLiteBackupHelper = `
import sqlite3
import sys

if sys.version_info < (3, 10):
    raise RuntimeError("TaskDeck identity backup requires Python 3.10+")

source_path = sys.argv[1]
destination_path = sys.argv[2]
source = sqlite3.connect(source_path, timeout=5.0)
destination = sqlite3.connect(destination_path, timeout=5.0)

try:
    source_check = source.execute("PRAGMA quick_check").fetchone()
    if not source_check or source_check[0] != "ok":
        raise RuntimeError("source SQLite quick_check failed")
    source.backup(destination, pages=256, sleep=0.05)
    destination.commit()
    destination_check = destination.execute("PRAGMA quick_check").fetchone()
    if not destination_check or destination_check[0] != "ok":
        raise RuntimeError("destination SQLite quick_check failed")
finally:
    destination.close()
    source.close()
`
