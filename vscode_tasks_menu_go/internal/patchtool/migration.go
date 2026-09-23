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
