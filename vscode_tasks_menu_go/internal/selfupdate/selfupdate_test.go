package selfupdate

import (
	"os"
	"path/filepath"
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
