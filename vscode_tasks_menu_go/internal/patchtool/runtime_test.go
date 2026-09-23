package patchtool

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePrefersBundledRuntime(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	release := filepath.Join(root, "release")
	exe := filepath.Join(release, "taskdeck")
	bundled := filepath.Join(release, "patchtool", "python_patch_entry.py")
	legacy := filepath.Join(workspace, "tools", "run_python_patches.sh")
	writeFile(t, exe, 0o755)
	writeFile(t, bundled, 0o755)
	writeFile(t, legacy, 0o755)

	got, err := Resolve(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindBundledEntry || got.Path != bundled {
		t.Fatalf("runtime=%+v want bundled %s", got, bundled)
	}
}

func TestResolveUsesProjectEntryThenLegacyLauncher(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	exe := filepath.Join(root, "bin", "taskdeck")
	writeFile(t, exe, 0o755)
	projectEntry := filepath.Join(workspace, "tools", "python_patch_entry.py")
	writeFile(t, projectEntry, 0o755)

	got, err := Resolve(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindProjectEntry || got.Path != projectEntry {
		t.Fatalf("runtime=%+v want project entry", got)
	}

	if err := os.Remove(projectEntry); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(workspace, "tools", "run_python_patches.sh")
	writeFile(t, legacy, 0o755)
	got, err = Resolve(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindLegacyLauncher || got.Path != legacy {
		t.Fatalf("runtime=%+v want legacy launcher", got)
	}
}

func TestEntryCommandPassesWorkspaceAndPatchArgs(t *testing.T) {
	root := t.TempDir()
	python := filepath.Join(root, "python3")
	entry := filepath.Join(root, "python_patch_entry.py")
	writeFile(t, python, 0o755)
	writeFile(t, entry, 0o755)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)

	spec := Runtime{Kind: KindBundledEntry, Path: entry}
	command, args, err := spec.Command("/work/project", []string{"resume", "--resume-mode", "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if command != python {
		t.Fatalf("command=%q want %q", command, python)
	}
	want := []string{entry, "--project-root", "/work/project", "--", "resume", "--resume-mode", "failed"}
	if len(args) != len(want) {
		t.Fatalf("args=%#v want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d]=%q want %q", i, args[i], want[i])
		}
	}
}

func TestResolveBundledRuntimeThroughExecutableSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows CI")
	}
	root := t.TempDir()
	release := filepath.Join(root, "releases", "abc")
	exe := filepath.Join(release, "taskdeck")
	entry := filepath.Join(release, "patchtool", "python_patch_entry.py")
	writeFile(t, exe, 0o755)
	writeFile(t, entry, 0o755)
	link := filepath.Join(root, "bin", "taskdeck")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(root, link)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != entry {
		t.Fatalf("runtime=%+v want %s", got, entry)
	}
}
