package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestLifecycle(t *testing.T) {
	workspace := t.TempDir()
	req, err := CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", false)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "awaiting_confirmation" || req.ID == "" {
		t.Fatalf("unexpected request: %#v", req)
	}
	loaded, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != req.ID || loaded.Revision != req.Revision {
		t.Fatalf("loaded request mismatch: %#v", loaded)
	}
	confirmed, err := Update(workspace, req.ID, "confirmed", "ok", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "confirmed" || confirmed.ConfirmedAt == "" {
		t.Fatalf("confirmation not recorded: %#v", confirmed)
	}
	completed, err := Update(workspace, req.ID, "completed", "done", "http://127.0.0.1:1234", "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.CompletedAt == "" || completed.TargetURL == "" {
		t.Fatalf("completion not recorded: %#v", completed)
	}
}

func TestInstallReplacesBinaryAndWritesRevision(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "vscode_tasks_menu")
	staged := filepath.Join(dir, "staged")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(staged, target, "abcdef0123456789"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("target=%q want new", got)
	}
	if revision := InstalledRevision(target); revision != "abcdef0123456789" {
		t.Fatalf("revision=%q", revision)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged binary still exists: %v", err)
	}
}


func TestLauncherPathForBinaryRecognizesStandardInstallLayout(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "vscode_tasks_menu_go", ".build", "vscode_tasks_menu")
	got, ok := launcherPathForBinary(binary)
	if !ok {
		t.Fatal("standard install layout was not recognized")
	}
	want := filepath.Join(root, "vscode_tasks_menu")
	if got != want {
		t.Fatalf("launcher path=%q want %q", got, want)
	}

	for _, binary := range []string{
		filepath.Join(root, "vscode_tasks_menu"),
		filepath.Join(root, ".build", "vscode_tasks_menu"),
		filepath.Join(root, "other", ".build", "vscode_tasks_menu"),
		filepath.Join(root, "vscode_tasks_menu_go", ".build", "renamed"),
	} {
		if got, ok := launcherPathForBinary(binary); ok {
			t.Fatalf("unexpected launcher path for %q: %q", binary, got)
		}
	}
}

func TestInstallReplacesRootLauncherAndBinaryTogether(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, "vscode_tasks_menu_go", ".build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetBinary := filepath.Join(buildDir, "vscode_tasks_menu")
	targetLauncher := filepath.Join(root, "vscode_tasks_menu")
	stagedBinary := filepath.Join(buildDir, ".vscode_tasks_menu.new.test")
	stagedLauncher := stagedLauncherPath(stagedBinary)

	if err := os.WriteFile(targetBinary, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetLauncher, []byte("old-launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedBinary, []byte("new-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedLauncher, []byte("new-launcher"), 0o755); err != nil {
		t.Fatal(err)
	}

	const revision = "abcdef0123456789"
	if err := Install(stagedBinary, targetBinary, revision); err != nil {
		t.Fatal(err)
	}
	gotBinary, err := os.ReadFile(targetBinary)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("binary=%q", gotBinary)
	}
	gotLauncher, err := os.ReadFile(targetLauncher)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotLauncher) != "new-launcher" {
		t.Fatalf("launcher=%q", gotLauncher)
	}
	if revisionGot := InstalledRevision(targetBinary); revisionGot != revision {
		t.Fatalf("revision=%q want %q", revisionGot, revision)
	}
	if _, err := os.Stat(stagedLauncher); !os.IsNotExist(err) {
		t.Fatalf("staged launcher still exists: %v", err)
	}
}

func TestInstallDoesNotGuessLauncherOutsideStandardLayout(t *testing.T) {
	dir := t.TempDir()
	targetBinary := filepath.Join(dir, "custom-tool-name")
	stagedBinary := filepath.Join(dir, "staged")
	if err := os.WriteFile(targetBinary, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedBinary, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedLauncherPath(stagedBinary), []byte("must-not-install"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(stagedBinary, targetBinary, "1234567890abcdef"); err != nil {
		t.Fatal(err)
	}
	if _, ok := launcherPathForBinary(targetBinary); ok {
		t.Fatal("custom binary path unexpectedly recognized as standard launcher layout")
	}
}


func TestGlobalTaskdeckBinaryPathAndPreference(t *testing.T) {
	installDir := t.TempDir()
	t.Setenv("TASKDECK_INSTALL_DIR", installDir)
	global, err := GlobalBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(installDir, "taskdeck")
	if global != want {
		t.Fatalf("global path=%q want %q", global, want)
	}
	current := filepath.Join(t.TempDir(), "vscode_tasks_menu")
	if got := PreferredBinary(current); got != current {
		t.Fatalf("preferred missing-global=%q want current %q", got, current)
	}
	if err := os.WriteFile(global, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := PreferredBinary(current); got != global {
		t.Fatalf("preferred=%q want global %q", got, global)
	}
	if !SameExecutablePath(global, filepath.Join(installDir, ".", "taskdeck")) {
		t.Fatal("equivalent global paths were not recognized")
	}
}


func TestCopyRootSupportFileStagesInstallerAndLauncher(t *testing.T) {
	source := t.TempDir()
	dest := t.TempDir()
	for _, item := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "vscode_tasks_menu", mode: 0o755},
		{name: "install.sh", mode: 0o755},
		{name: "python_patch_entry.py", mode: 0o755},
		{name: "test_python_patch_entry.py", mode: 0o644},
		{name: "run_python_patches.sh", mode: 0o755},
	} {
		want := "#!/bin/sh\necho " + item.name + "\n"
		if err := os.WriteFile(filepath.Join(source, item.name), []byte(want), item.mode); err != nil {
			t.Fatal(err)
		}
		if err := copyRootSupportFile(source, dest, item.name, item.mode); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dest, item.name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("%s content=%q want %q", item.name, got, want)
		}
		info, err := os.Stat(filepath.Join(dest, item.name))
		if err != nil {
			t.Fatal(err)
		}
		if item.mode&0o111 != 0 && info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s is not executable: %v", item.name, info.Mode())
		}
		if item.mode&0o111 == 0 && info.Mode().Perm()&0o111 != 0 {
			t.Fatalf("%s unexpectedly executable: %v", item.name, info.Mode())
		}
	}
	if err := copyRootSupportFile(source, dest, "../escape", 0o755); err == nil {
		t.Fatal("expected unsafe support-file name rejection")
	}
}


func TestSameExecutablePathRecognizesSymlinkToGlobalBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "taskdeck")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "taskdeck-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if !SameExecutablePath(link, target) {
		t.Fatalf("symlink %q and target %q must be treated as the same executable", link, target)
	}
}


func makeFakeRelease(t *testing.T, path string) {
	t.Helper()
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{
		{filepath.Join(path, "taskdeck"), 0o755},
		{filepath.Join(path, "patchtool", "python_patch_entry.py"), 0o755},
		{filepath.Join(path, "patchtool", "_patch_lib", "python_patch_queue_dispatcher.py"), 0o644},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(item.path, []byte("test\n"), item.mode); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGlobalReleasePathsAndAtomicInstallPreserveOldRelease(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	appDir := filepath.Join(root, "lib", "taskdeck")
	t.Setenv("TASKDECK_INSTALL_DIR", binDir)
	t.Setenv("TASKDECK_APP_DIR", appDir)

	rev1 := "1111111111111111111111111111111111111111"
	rev2 := "2222222222222222222222222222222222222222"

	release1, err := GlobalReleaseDir(rev1)
	if err != nil {
		t.Fatal(err)
	}
	staged1 := filepath.Join(filepath.Dir(release1), ".stage-one")
	makeFakeRelease(t, staged1)
	if err := InstallGlobalRelease(staged1, rev1); err != nil {
		t.Fatal(err)
	}

	global, err := GlobalBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(global)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(release1, "taskdeck") {
		t.Fatalf("global resolves to %q want %q", resolved, filepath.Join(release1, "taskdeck"))
	}
	if InstalledRevision(global) != rev1 {
		t.Fatalf("marker=%q want %q", InstalledRevision(global), rev1)
	}
	if !GlobalReleaseReady(rev1) {
		t.Fatal("first release should be globally ready")
	}

	release2, err := GlobalReleaseDir(rev2)
	if err != nil {
		t.Fatal(err)
	}
	staged2 := filepath.Join(filepath.Dir(release2), ".stage-two")
	makeFakeRelease(t, staged2)
	if err := InstallGlobalRelease(staged2, rev2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(release1, "taskdeck")); err != nil {
		t.Fatalf("old release was removed: %v", err)
	}
	resolved, err = filepath.EvalSymlinks(global)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(release2, "taskdeck") {
		t.Fatalf("global resolves to %q want %q", resolved, filepath.Join(release2, "taskdeck"))
	}
	if GlobalReleaseReady(rev1) {
		t.Fatal("old release must not remain current")
	}
	if !GlobalReleaseReady(rev2) {
		t.Fatal("second release should be globally ready")
	}
}

func TestGlobalReleaseDirRejectsUnsafeRevision(t *testing.T) {
	t.Setenv("TASKDECK_APP_DIR", t.TempDir())
	for _, revision := range []string{"", "../escape", "not-a-sha", "1234/567890abcdef"} {
		if _, err := GlobalReleaseDir(revision); err == nil {
			t.Fatalf("unsafe revision accepted: %q", revision)
		}
	}
}


func TestGoTestsDoNotDependOnRepositoryOnlyPatchDocs(t *testing.T) {
	moduleRoot := filepath.Clean(filepath.Join("..", ".."))
	forbidden := strings.Join([]string{"TASKDECK", "_PATCH", "_HANDOFF", ".md"}, "")
	err := filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("Go test %s depends on repository-only document %s; self-update stages only the Go module plus bounded runtime support files", path, forbidden)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
