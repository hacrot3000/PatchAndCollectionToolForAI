package patchtool

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LegacyMigrationPlan struct {
	LegacyRoot  string
	BundledRoot string
	Present     bool
	Safe        bool
	Verified    []string
	Modified    []string
	Unknown     []string
}

var legacyTopLevelRuntimeFiles = []string{
	"python_patch_entry.py",
	"run_python_patches.sh",
	"run_python_patches.ps1",
	"run_python_patches.bat",
}

func BundledRoot(executable string) (string, bool) {
	for _, exe := range executableCandidates(executable) {
		root := filepath.Join(filepath.Dir(exe), "patchtool")
		if regularFile(filepath.Join(root, "python_patch_entry.py")) && regularDirectory(filepath.Join(root, "_patch_lib")) {
			return root, true
		}
	}
	return "", false
}

func regularDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func migrationCachePath(rel string) bool {
	rel = filepath.ToSlash(rel)
	return strings.Contains("/"+rel+"/", "/__pycache__/") || strings.HasSuffix(rel, ".pyc")
}

func fileDigest(path string) ([32]byte, error) {
	var zero [32]byte
	f, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return zero, err
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

func runtimeFiles(root string) (map[string]string, error) {
	out := map[string]string{}
	for _, name := range legacyTopLevelRuntimeFiles {
		path := filepath.Join(root, name)
		if regularFile(path) {
			out[filepath.ToSlash(name)] = path
		}
	}
	libRoot := filepath.Join(root, "_patch_lib")
	if !regularDirectory(libRoot) {
		return out, nil
	}
	err := filepath.WalkDir(libRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			out[filepath.ToSlash(rel)] = path
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if migrationCachePath(rel) {
			return nil
		}
		out[filepath.ToSlash(rel)] = path
		return nil
	})
	return out, err
}

func PlanLegacyMigration(workspace, executable string) (LegacyMigrationPlan, error) {
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return LegacyMigrationPlan{}, fmt.Errorf("resolve workspace: %w", err)
	}
	bundled, ok := BundledRoot(executable)
	if !ok {
		return LegacyMigrationPlan{}, fmt.Errorf("bundled Patch Tool runtime not found beside TaskDeck executable")
	}
	legacy := filepath.Join(workspace, "tools")
	plan := LegacyMigrationPlan{LegacyRoot: legacy, BundledRoot: bundled}

	legacyFiles, err := runtimeFiles(legacy)
	if err != nil {
		return plan, fmt.Errorf("scan legacy Patch runtime: %w", err)
	}
	if len(legacyFiles) == 0 {
		return plan, nil
	}
	plan.Present = true

	bundledFiles, err := runtimeFiles(bundled)
	if err != nil {
		return plan, fmt.Errorf("scan bundled Patch runtime: %w", err)
	}

	for rel, legacyPath := range legacyFiles {
		bundledPath, managed := bundledFiles[rel]
		if !managed {
			plan.Unknown = append(plan.Unknown, rel)
			continue
		}
		legacyInfo, statErr := os.Lstat(legacyPath)
		if statErr != nil {
			return plan, statErr
		}
		if legacyInfo.Mode()&os.ModeSymlink != 0 {
			plan.Unknown = append(plan.Unknown, rel)
			continue
		}
		want, err := fileDigest(bundledPath)
		if err != nil {
			return plan, fmt.Errorf("digest bundled %s: %w", rel, err)
		}
		got, err := fileDigest(legacyPath)
		if err != nil {
			return plan, fmt.Errorf("digest legacy %s: %w", rel, err)
		}
		if got != want {
			plan.Modified = append(plan.Modified, rel)
			continue
		}
		plan.Verified = append(plan.Verified, rel)
	}
	sort.Strings(plan.Verified)
	sort.Strings(plan.Modified)
	sort.Strings(plan.Unknown)
	plan.Safe = plan.Present && len(plan.Verified) > 0 && len(plan.Modified) == 0 && len(plan.Unknown) == 0
	return plan, nil
}


var compatibilityLaunchers = map[string]struct {
	mode os.FileMode
	body string
}{
	"run_python_patches.sh": {
		mode: 0o755,
		body: "#!/usr/bin/env bash\nset -euo pipefail\nif command -v taskdeck >/dev/null 2>&1; then\n  exec taskdeck patch \"$@\"\nfi\nif [[ -n \"${HOME:-}\" && -x \"${HOME}/.local/bin/taskdeck\" ]]; then\n  exec \"${HOME}/.local/bin/taskdeck\" patch \"$@\"\nfi\necho \"ERROR: TaskDeck is not installed or not available in PATH.\" >&2\nexit 127\n",
	},
	"run_python_patches.ps1": {
		mode: 0o644,
		body: "$ErrorActionPreference = 'Stop'\n$cmd = Get-Command taskdeck -ErrorAction SilentlyContinue | Select-Object -First 1\nif ($null -ne $cmd) { & $cmd.Source patch @args; exit [int]$LASTEXITCODE }\n$default = Join-Path $HOME '.local/bin/taskdeck'\nif (Test-Path -LiteralPath $default -PathType Leaf) { & $default patch @args; exit [int]$LASTEXITCODE }\n[Console]::Error.WriteLine('ERROR: TaskDeck is not installed or not available in PATH.')\nexit 127\n",
	},
	"run_python_patches.bat": {
		mode: 0o644,
		body: "@echo off\r\nwhere taskdeck >nul 2>nul\r\nif %ERRORLEVEL% EQU 0 (\r\n  taskdeck patch %*\r\n  exit /b %ERRORLEVEL%\r\n)\r\necho ERROR: TaskDeck is not installed or not available in PATH. 1>&2\r\nexit /b 127\r\n",
	},
}

func writeCompatibilityLauncher(path string, spec struct {
	mode os.FileMode
	body string
}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".migration.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(spec.mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(spec.body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func verifiedSet(plan LegacyMigrationPlan) map[string]bool {
	out := make(map[string]bool, len(plan.Verified))
	for _, rel := range plan.Verified {
		out[filepath.ToSlash(rel)] = true
	}
	return out
}

func recheckVerifiedFile(plan LegacyMigrationPlan, rel string) error {
	legacyPath := filepath.Join(plan.LegacyRoot, filepath.FromSlash(rel))
	bundledPath := filepath.Join(plan.BundledRoot, filepath.FromSlash(rel))
	info, err := os.Lstat(legacyPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("legacy runtime file changed type during migration: %s", rel)
	}
	got, err := fileDigest(legacyPath)
	if err != nil {
		return err
	}
	want, err := fileDigest(bundledPath)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("legacy runtime file changed after verification: %s", rel)
	}
	return nil
}

func pruneEmptyRuntimeDirs(legacyRoot string) {
	libRoot := filepath.Join(legacyRoot, "_patch_lib")
	var dirs []string
	_ = filepath.WalkDir(libRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
}

func ApplyLegacyMigration(workspace, executable string) (LegacyMigrationPlan, error) {
	plan, err := PlanLegacyMigration(workspace, executable)
	if err != nil || !plan.Present {
		return plan, err
	}
	if !plan.Safe {
		return plan, fmt.Errorf("legacy Patch runtime is not safe to migrate: modified=%v unknown=%v", plan.Modified, plan.Unknown)
	}

	verified := verifiedSet(plan)
	for rel := range verified {
		if err := recheckVerifiedFile(plan, rel); err != nil {
			return plan, err
		}
	}

	// Install compatibility shims first. If a later cleanup step fails, old
	// launch paths already route to the globally bundled Patch Tool.
	for name, spec := range compatibilityLaunchers {
		if !verified[name] {
			continue
		}
		if err := writeCompatibilityLauncher(filepath.Join(plan.LegacyRoot, name), spec); err != nil {
			return plan, fmt.Errorf("write compatibility launcher %s: %w", name, err)
		}
	}

	for _, rel := range plan.Verified {
		if _, launcher := compatibilityLaunchers[rel]; launcher {
			continue
		}
		path := filepath.Join(plan.LegacyRoot, filepath.FromSlash(rel))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return plan, fmt.Errorf("remove verified legacy runtime file %s: %w", rel, err)
		}
	}
	pruneEmptyRuntimeDirs(plan.LegacyRoot)
	return plan, nil
}
