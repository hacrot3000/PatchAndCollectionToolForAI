package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitPushLargeRepo(t *testing.T) (string, *Server, string, string) {
	t.Helper()
	workspace, s, branch := setupGitQuickRepo(t)
	remoteRoot := t.TempDir()
	remote := filepath.Join(remoteRoot, "remote.git")
	gitQuickRun(t, remoteRoot, "init", "--bare", remote)
	gitQuickRun(t, workspace, "remote", "add", "origin", remote)
	gitQuickRun(t, workspace, "push", "--set-upstream", "origin", branch)
	upstream := "origin/" + branch
	return workspace, s, branch, upstream
}

func writeLargeTestFile(t *testing.T, root, rel string, size int) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	data := bytes.Repeat([]byte("0123456789abcdef"), size/16+1)
	data = data[:size]
	if err := os.WriteFile(path, data, 0o644); err != nil { t.Fatal(err) }
}

func installFakeGitLFS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "git-lfs")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  version) echo 'git-lfs/3.7.1 (TaskDeck fixture)' ;;\n" +
		"  install) echo 'Updated Git hooks.' ;;\n" +
		"  migrate) echo 'migrate fixture' ;;\n" +
		"  *) exit 2 ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { t.Fatal(err) }
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestGitLFSPatternValidationAndArguments(t *testing.T) {
	for _, value := range []string{"logic/capture.csv", "*.csv", "assets/**/large.bin"} {
		if got, err := validGitLFSPattern(value); err != nil || got != value {
			t.Fatalf("pattern %q got=%q err=%v", value, got, err)
		}
	}
	for _, value := range []string{"", "a,b", "bad\npattern"} {
		if _, err := validGitLFSPattern(value); err == nil {
			t.Fatalf("invalid pattern %q accepted", value)
		}
	}
	args := strings.Join(gitLFSMigrateArgs("*.csv"), " ")
	for _, want := range []string{"lfs migrate import", "--yes", "--skip-fetch", "--include=*.csv"} {
		if !strings.Contains(args, want) { t.Fatalf("args=%q missing %q", args, want) }
	}
}

func TestGitLFSMigrationPlanUsesVerifiedOutgoingRange(t *testing.T) {
	workspace, s, branch, upstream := setupGitPushLargeRepo(t)
	installFakeGitLFS(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("two\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "outgoing")

	plan := s.gitLFSMigrationPlan(context.Background())
	if !plan.Available || !strings.Contains(plan.Version, "git-lfs/3.7.1") {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Branch != branch || plan.Upstream != upstream || plan.Range != upstream+"..HEAD" || plan.OutgoingCommits != 1 || plan.Head == "" {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestParseGitLargeBlobBatch(t *testing.T) {
	raw := strings.Join([]string{
		"aaaaaaaa blob 512 small.txt",
		"bbbbbbbb tree 9000 dir",
		"cccccccc blob 2048 logic/capture.csv",
		"dddddddd blob 4096 logic/capture.csv",
		"eeeeeeee blob 3072 reports/big file.bin",
	}, "\n")
	rows := parseGitLargeBlobBatch(raw, 1024)
	if len(rows) != 2 {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].Path != "logic/capture.csv" || rows[0].SizeBytes != 4096 {
		t.Fatalf("first=%+v", rows[0])
	}
	if rows[1].Path != "reports/big file.bin" || rows[1].SizeBytes != 3072 {
		t.Fatalf("second=%+v", rows[1])
	}
}

func TestGitOutgoingLargeBlobsScansOnlyOutgoingObjects(t *testing.T) {
	workspace, s, _, upstream := setupGitPushLargeRepo(t)
	writeLargeTestFile(t, workspace, "logic/capture.csv", 4096)
	gitQuickRun(t, workspace, "add", "logic/capture.csv")
	gitQuickRun(t, workspace, "commit", "-m", "add capture")

	rows, err := s.gitOutgoingLargeBlobs(context.Background(), upstream, 1024)
	if err != nil { t.Fatal(err) }
	if len(rows) != 1 || rows[0].Path != "logic/capture.csv" || rows[0].SizeBytes != 4096 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestGitPushPreflightStopsGitHubLargeBlobBeforeNetwork(t *testing.T) {
	workspace, s, branch := setupGitQuickRepo(t)
	initial := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "remote", "add", "origin", "git@github.com:example/example.git")
	gitQuickRun(t, workspace, "config", "branch."+branch+".remote", "origin")
	gitQuickRun(t, workspace, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	gitQuickRun(t, workspace, "update-ref", "refs/remotes/origin/"+branch, initial)

	writeLargeTestFile(t, workspace, "logic/capture.csv", 4096)
	gitQuickRun(t, workspace, "add", "logic/capture.csv")
	gitQuickRun(t, workspace, "commit", "-m", "add capture")

	installFakeGitLFS(t)

	oldLimit := gitHubPushBlobLimit
	gitHubPushBlobLimit = 1024
	defer func(){ gitHubPushBlobLimit = oldLimit }()

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"push"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		OK          bool                `json:"ok"`
		FailureCode string              `json:"failure_code"`
		Provider    string              `json:"provider"`
		LargeFiles  []gitLargeBlobInfo  `json:"large_files"`
		LFSPlan     gitLFSMigrationPlan `json:"lfs_plan"`
		Output      string              `json:"output"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if response.OK || response.FailureCode != "file_too_large" || response.Provider != "github" {
		t.Fatalf("response=%+v body=%s", response, rr.Body.String())
	}
	if len(response.LargeFiles) != 1 || response.LargeFiles[0].Path != "logic/capture.csv" {
		t.Fatalf("large_files=%+v", response.LargeFiles)
	}
	if !response.LFSPlan.Available || response.LFSPlan.OutgoingCommits != 1 || response.LFSPlan.Range == "" {
		t.Fatalf("lfs_plan=%+v", response.LFSPlan)
	}
	if !strings.Contains(response.Output, "stopped before uploading") {
		t.Fatalf("output=%q", response.Output)
	}
}

func TestGitLargeFileRemoveFromLatestKeepsFileAndAmendsCommit(t *testing.T) {
	workspace, s, _, upstream := setupGitPushLargeRepo(t)
	writeLargeTestFile(t, workspace, "logic/capture.csv", 4096)
	gitQuickRun(t, workspace, "add", "logic/capture.csv")
	gitQuickRun(t, workspace, "commit", "-m", "add capture")
	oldHead := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	output, _, err := s.gitLargeFileRemoveFromLatest(context.Background(), "logic/capture.csv", 1024)
	if err != nil { t.Fatalf("cleanup failed: %v\n%s", err, output) }
	newHead := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if newHead == oldHead {
		t.Fatal("latest commit was not amended")
	}
	if _, err := os.Stat(filepath.Join(workspace, "logic", "capture.csv")); err != nil {
		t.Fatalf("working-tree file should be kept: %v", err)
	}
	if got := gitQuickRun(t, workspace, "ls-files", "--", "logic/capture.csv"); got != "" {
		t.Fatalf("file still tracked: %q", got)
	}
	if got := gitQuickRun(t, workspace, "check-ignore", "logic/capture.csv"); got != "logic/capture.csv" {
		t.Fatalf("file not ignored: %q", got)
	}
	rows, err := s.gitOutgoingLargeBlobs(context.Background(), upstream, 1024)
	if err != nil { t.Fatal(err) }
	if len(rows) != 0 {
		t.Fatalf("large outgoing blobs remain: %+v", rows)
	}
}

func TestGitLargeFileLatestCleanupRejectsOlderOutgoingCommit(t *testing.T) {
	workspace, s, _, _ := setupGitPushLargeRepo(t)
	writeLargeTestFile(t, workspace, "logic/capture.csv", 4096)
	gitQuickRun(t, workspace, "add", "logic/capture.csv")
	gitQuickRun(t, workspace, "commit", "-m", "add capture")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("two\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "later change")

	_, _, err := s.gitLargeFileRemoveFromLatest(context.Background(), "logic/capture.csv", 1024)
	if err == nil || !strings.Contains(err.Error(), "multi-commit cleanup") {
		t.Fatalf("expected multi-commit guidance, got %v", err)
	}
}

func TestGitLargeFilePrepareRecommitKeepsCombinedChangesStaged(t *testing.T) {
	workspace, s, _, upstream := setupGitPushLargeRepo(t)
	writeLargeTestFile(t, workspace, "logic/capture.csv", 4096)
	gitQuickRun(t, workspace, "add", "logic/capture.csv")
	gitQuickRun(t, workspace, "commit", "-m", "add capture")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("two\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "later change")
	originalHead := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	output, _, err := s.gitLargeFilePrepareRecommit(context.Background(), "logic/capture.csv", 1024)
	if err != nil { t.Fatalf("prepare failed: %v\n%s", err, output) }
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != gitQuickRun(t, workspace, "rev-parse", upstream) {
		t.Fatalf("HEAD=%q want upstream", got)
	}
	if got := gitQuickRun(t, workspace, "diff", "--cached", "--name-only"); !strings.Contains(got, "tracked.txt") || !strings.Contains(got, ".gitignore") {
		t.Fatalf("staged files=%q", got)
	}
	if got := gitQuickRun(t, workspace, "ls-files", "--", "logic/capture.csv"); got != "" {
		t.Fatalf("large file still staged/tracked: %q", got)
	}
	if got := gitQuickRun(t, workspace, "check-ignore", "logic/capture.csv"); got != "logic/capture.csv" {
		t.Fatalf("large file not ignored: %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "logic", "capture.csv")); err != nil {
		t.Fatalf("working-tree large file should remain: %v", err)
	}
	if !strings.Contains(output, originalHead) || !strings.Contains(output, "Review the staged changes") {
		t.Fatalf("output=%q", output)
	}
}

func TestGitLargeFileClassificationWinsOverPreReceiveHookText(t *testing.T) {
	text := "remote: error: File logic/capture.csv is 637.58 MB; this exceeds GitHub's file size limit of 100.00 MB\n" +
		"remote: error: GH001: Large files detected.\n" +
		"! [remote rejected] main -> main (pre-receive hook declined)"
	if got := classifyGitFailure("push", text, ""); got != "file_too_large" {
		t.Fatalf("classify=%q want file_too_large", got)
	}
}

func TestGitHubRemoteDetection(t *testing.T) {
	for _, value := range []string{
		"git@github.com:owner/repo.git",
		"https://github.com/owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
	} {
		if !isGitHubRemoteURL(value) { t.Fatalf("not detected: %q", value) }
	}
	for _, value := range []string{"https://gitlab.com/owner/repo.git", "https://evilgithub.com/x.git"} {
		if isGitHubRemoteURL(value) { t.Fatalf("false positive: %q", value) }
	}
}
