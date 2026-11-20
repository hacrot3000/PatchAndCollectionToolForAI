package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadDetectionSkipsInteractiveGitOutputButKeepsNextArtifact(t *testing.T) {
	workspace := t.TempDir()
	write := func(rel string) string {
		t.Helper()
		path := filepath.Join(workspace, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tracked := write("src/tracked.go")
	untracked := write("tmp/untracked.txt")
	artifact := write("artifacts/ptv_to_ai/CR_keep.zip")

	console := strings.Join([]string{
		"duong@host:" + workspace + "$ git status",
		"On branch main",
		"Changes not staged for commit:",
		"  modified:   " + tracked,
		"Untracked files:",
		"  " + untracked,
		"duong@host:" + workspace + "$ ./run_python_patches.sh",
		"ZIP (preferred) — copy path below:",
		artifact,
	}, "\n")

	files := (&Server{Workspace: workspace}).downloadableFilesFromText(console)
	if len(files) != 1 || files[0].Path != artifact {
		t.Fatalf("files=%#v want only artifact %q", files, artifact)
	}
}

func TestGitOutputFilterLeavesCompoundCommandUntouched(t *testing.T) {
	input := "user@host:/tmp$ git status && ./collector\nresult: artifacts/result.zip"
	got := stripGitShellOutput(input)
	if !strings.Contains(got, "git status && ./collector") || !strings.Contains(got, "artifacts/result.zip") {
		t.Fatalf("compound command must not be suppressed: %q", got)
	}
}

func TestStandaloneGitShellCommandRecognizesCommonForms(t *testing.T) {
	for _, command := range []string{
		"git status",
		"git merge develop",
		"git switch main",
		"git checkout testing",
		"git diff --stat",
		"command git log -5",
		"sudo git status",
		"env GIT_PAGER=cat git log",
		"/usr/bin/git status",
	} {
		if !standaloneGitShellCommand(command) {
			t.Fatalf("expected Git command: %q", command)
		}
	}
	for _, command := range []string{
		"echo git status",
		"git status && ./collector",
		"git status; ./collector",
	} {
		if standaloneGitShellCommand(command) {
			t.Fatalf("must not suppress compound/non-Git command: %q", command)
		}
	}
}
