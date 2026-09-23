package patchtool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Kind string

const (
	KindBundledEntry Kind = "bundled_entry"
	KindProjectEntry Kind = "project_entry"
	KindLegacyLauncher Kind = "legacy_launcher"
)

type Runtime struct {
	Kind Kind
	Path string
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func entryCandidate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if info, err := os.Stat(raw); err == nil && info.IsDir() {
		return filepath.Join(raw, "python_patch_entry.py")
	}
	return raw
}

func executableCandidates(executable string) []string {
	out := []string{executable}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil && filepath.Clean(resolved) != filepath.Clean(executable) {
		out = append(out, resolved)
	}
	return out
}

func Resolve(workspace, executable string) (Runtime, error) {
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return Runtime{}, fmt.Errorf("resolve workspace: %w", err)
	}

	if explicit := entryCandidate(os.Getenv("TASKDECK_PATCH_RUNTIME")); explicit != "" {
		if !regularFile(explicit) {
			return Runtime{}, fmt.Errorf("TASKDECK_PATCH_RUNTIME does not contain python_patch_entry.py: %s", explicit)
		}
		return Runtime{Kind: KindBundledEntry, Path: explicit}, nil
	}

	for _, exe := range executableCandidates(executable) {
		candidate := filepath.Join(filepath.Dir(exe), "patchtool", "python_patch_entry.py")
		if regularFile(candidate) {
			return Runtime{Kind: KindBundledEntry, Path: candidate}, nil
		}
	}

	for _, candidate := range []string{
		filepath.Join(workspace, "tools", "python_patch_entry.py"),
		filepath.Join(workspace, "python_patch_entry.py"),
	} {
		if regularFile(candidate) {
			return Runtime{Kind: KindProjectEntry, Path: candidate}, nil
		}
	}

	for _, candidate := range []string{
		filepath.Join(workspace, "tools", "run_python_patches.sh"),
		filepath.Join(workspace, "run_python_patches.sh"),
	} {
		if regularFile(candidate) {
			return Runtime{Kind: KindLegacyLauncher, Path: candidate}, nil
		}
	}

	return Runtime{}, fmt.Errorf(
		"Patch Tool runtime was not found; expected bundled patchtool/python_patch_entry.py or a legacy project tools/run_python_patches.sh",
	)
}

func pythonCommand() (string, []string, error) {
	if explicit := strings.TrimSpace(os.Getenv("TASKDECK_PATCH_PYTHON")); explicit != "" {
		if !regularFile(explicit) {
			return "", nil, fmt.Errorf("TASKDECK_PATCH_PYTHON is not an executable file: %s", explicit)
		}
		return explicit, nil, nil
	}
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil, nil
		}
	}
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath("py"); err == nil {
			return path, []string{"-3"}, nil
		}
	}
	return "", nil, fmt.Errorf("Python 3.10+ was not found in PATH")
}

func entryArgs(entry, workspace string, args []string) []string {
	out := []string{entry, "--project-root", workspace, "--"}
	return append(out, args...)
}

func (r Runtime) Command(workspace string, args []string) (string, []string, error) {
	if r.Kind == KindLegacyLauncher {
		return r.Path, append([]string(nil), args...), nil
	}
	python, prefix, err := pythonCommand()
	if err != nil {
		return "", nil, err
	}
	commandArgs := append([]string(nil), prefix...)
	commandArgs = append(commandArgs, entryArgs(r.Path, workspace, args)...)
	return python, commandArgs, nil
}
