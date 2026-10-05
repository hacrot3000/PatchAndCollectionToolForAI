package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitHunkRepo(t *testing.T) (string, *Server) {
	t.Helper()
	workspace, s, _ := setupGitQuickRepo(t)
	var base strings.Builder
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&base, "line-%02d\n", i)
	}
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte(base.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "tracked.txt")
	gitQuickRun(t, workspace, "commit", "-m", "hunk baseline")

	lines := strings.Split(strings.TrimSuffix(base.String(), "\n"), "\n")
	lines[1] = "line-02 changed"
	lines[19] = "line-20 changed"
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace, s
}

func TestGitDiffHunkSetReturnsStableStructuredHunks(t *testing.T) {
	_, s := setupGitHunkRepo(t)
	set, truncated, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "worktree")
	if err != nil { t.Fatal(err) }
	if truncated { t.Fatal("unexpected truncation") }
	if set.DiffSHA256 == "" || len(set.Hunks) != 2 {
		t.Fatalf("set=%+v", set)
	}
	if !strings.HasPrefix(set.Hunks[0].Header, "@@ ") || !strings.Contains(set.Hunks[0].Text, "line-02 changed") {
		t.Fatalf("first hunk=%+v", set.Hunks[0])
	}
	if !strings.Contains(set.Hunks[1].Text, "line-20 changed") {
		t.Fatalf("second hunk=%+v", set.Hunks[1])
	}
	if !strings.Contains(set.FileHeader, "diff --git") || !strings.Contains(set.FileHeader, "--- a/tracked.txt") {
		t.Fatalf("file header=%q", set.FileHeader)
	}
}

func TestGitDiffModesSeparateCommittedStagedAndWorkingVersions(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	file := filepath.Join(workspace, "tracked.txt")

	if err := os.WriteFile(file, []byte("staged version\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "tracked.txt")
	if err := os.WriteFile(file, []byte("working version\n"), 0o644); err != nil { t.Fatal(err) }

	staged, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "staged")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(staged.Diff, "staged version") || strings.Contains(staged.Diff, "working version") {
		t.Fatalf("HEAD -> staged diff mixed working-tree content:\n%s", staged.Diff)
	}

	worktree, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "worktree")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(worktree.Diff, "-staged version") || !strings.Contains(worktree.Diff, "+working version") {
		t.Fatalf("staged -> working diff does not isolate unstaged edit:\n%s", worktree.Diff)
	}

	combined, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "head-worktree")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(combined.Diff, "-one") || !strings.Contains(combined.Diff, "+working version") {
		t.Fatalf("HEAD -> working diff does not show total tracked change:\n%s", combined.Diff)
	}
	if strings.Contains(combined.Diff, "staged version") {
		t.Fatalf("HEAD -> working diff should compare endpoints, not expose intermediate index content:\n%s", combined.Diff)
	}
}

func TestGitStageAndUnstageSingleHunk(t *testing.T) {
	workspace, s := setupGitHunkRepo(t)
	set, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "worktree")
	if err != nil { t.Fatal(err) }
	if len(set.Hunks) != 2 { t.Fatalf("hunks=%d", len(set.Hunks)) }

	out, _, err := s.gitApplyHunk(context.Background(), "tracked.txt", "worktree", 0, set.DiffSHA256, "stage")
	if err != nil { t.Fatalf("stage hunk: %v\n%s", err, out) }

	staged := gitQuickRun(t, workspace, "diff", "--cached")
	if !strings.Contains(staged, "line-02 changed") || strings.Contains(staged, "line-20 changed") {
		t.Fatalf("unexpected staged diff:\n%s", staged)
	}
	remaining := gitQuickRun(t, workspace, "diff")
	if strings.Contains(remaining, "line-02 changed") || !strings.Contains(remaining, "line-20 changed") {
		t.Fatalf("unexpected worktree diff:\n%s", remaining)
	}

	stagedSet, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "staged")
	if err != nil { t.Fatal(err) }
	if len(stagedSet.Hunks) != 1 { t.Fatalf("staged hunks=%d", len(stagedSet.Hunks)) }
	out, _, err = s.gitApplyHunk(context.Background(), "tracked.txt", "staged", 0, stagedSet.DiffSHA256, "unstage")
	if err != nil { t.Fatalf("unstage hunk: %v\n%s", err, out) }
	if got := gitQuickRun(t, workspace, "diff", "--cached"); got != "" {
		t.Fatalf("staged diff remains:\n%s", got)
	}
}

func TestGitDiscardSingleHunkLeavesOtherWorktreeChanges(t *testing.T) {
	workspace, s := setupGitHunkRepo(t)
	set, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "worktree")
	if err != nil { t.Fatal(err) }
	out, _, err := s.gitApplyHunk(context.Background(), "tracked.txt", "worktree", 0, set.DiffSHA256, "discard")
	if err != nil { t.Fatalf("discard hunk: %v\n%s", err, out) }

	data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil { t.Fatal(err) }
	text := string(data)
	if strings.Contains(text, "line-02 changed") || !strings.Contains(text, "line-20 changed") {
		t.Fatalf("unexpected file after discard:\n%s", text)
	}
}

func TestGitHunkApplyRejectsDiffDrift(t *testing.T) {
	workspace, s := setupGitHunkRepo(t)
	set, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "worktree")
	if err != nil { t.Fatal(err) }
	file := filepath.Join(workspace, "tracked.txt")
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil { t.Fatal(err) }
	if _, err := f.WriteString("new drift\n"); err != nil { t.Fatal(err) }
	_ = f.Close()

	_, _, err = s.gitApplyHunk(context.Background(), "tracked.txt", "worktree", 0, set.DiffSHA256, "stage")
	if err == nil || !strings.Contains(err.Error(), "diff changed") {
		t.Fatalf("expected drift rejection, got %v", err)
	}
}

func TestGitDiffAPIIncludesHunksAndSHA(t *testing.T) {
	_, s := setupGitHunkRepo(t)
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=diff&mode=worktree&path=tracked.txt", "")
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
	var response struct {
		DiffSHA string        `json:"diff_sha256"`
		Hunks   []gitDiffHunk `json:"hunks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if response.DiffSHA == "" || len(response.Hunks) != 2 {
		t.Fatalf("response=%+v body=%s", response, rr.Body.String())
	}
}

func TestDiscardHunkAPIRequiresConfirmation(t *testing.T) {
	_, s := setupGitHunkRepo(t)
	set, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "worktree")
	if err != nil { t.Fatal(err) }
	body := fmt.Sprintf(`{"action":"discard_hunk","path":"tracked.txt","hunk_index":0,"expected_diff_sha":%q}`, set.DiffSHA256)
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestStageHunkAPIUsesServerGeneratedPatch(t *testing.T) {
	workspace, s := setupGitHunkRepo(t)
	set, _, err := s.gitDiffHunkSet(context.Background(), "tracked.txt", "worktree")
	if err != nil { t.Fatal(err) }
	body := fmt.Sprintf(`{"action":"stage_hunk","path":"tracked.txt","hunk_index":1,"expected_diff_sha":%q}`, set.DiffSHA256)
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	staged := gitQuickRun(t, workspace, "diff", "--cached")
	if !strings.Contains(staged, "line-20 changed") || strings.Contains(staged, "line-02 changed") {
		t.Fatalf("unexpected staged diff:\n%s", staged)
	}
}
