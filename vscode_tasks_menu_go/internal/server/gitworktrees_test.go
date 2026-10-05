package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitWorktreesListsLinkedBranchWithoutLeakingAbsolutePaths(t *testing.T) {
	workspace, s, mainBranch := setupGitQuickRepo(t)
	linked := filepath.Join(filepath.Dir(workspace), "linked worktree")
	gitQuickRun(t, workspace, "worktree", "add", "-b", "feature/worktree", linked)

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=worktrees", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("worktrees status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Worktrees []gitWorktreeRow `json:"worktrees"`
		Count     int              `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || len(got.Worktrees) != 2 {
		t.Fatalf("worktrees=%+v count=%d", got.Worktrees, got.Count)
	}
	var current, linkedRow *gitWorktreeRow
	for i := range got.Worktrees {
		row := &got.Worktrees[i]
		if row.Current {
			current = row
		}
		if row.Branch == "feature/worktree" {
			linkedRow = row
		}
	}
	if current == nil || current.Branch != mainBranch || current.DisplayPath != "." || !current.Primary {
		t.Fatalf("current=%+v main=%q", current, mainBranch)
	}
	if linkedRow == nil || linkedRow.DisplayPath != "linked worktree" || linkedRow.ID == "" || linkedRow.Current {
		t.Fatalf("linked=%+v", linkedRow)
	}
	body := rr.Body.String()
	if strings.Contains(body, filepath.Clean(workspace)) || strings.Contains(body, filepath.Clean(linked)) {
		t.Fatalf("worktree API leaked absolute host path: %s", body)
	}
}

func TestParseGitWorktreePorcelainHandlesDetachedLockedAndPrunable(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "srv", "repo")
	linked := filepath.Join(string(filepath.Separator), "srv", "repo-fix")
	raw := "worktree " + root + "\x00" +
		"HEAD " + strings.Repeat("a", 40) + "\x00" +
		"branch refs/heads/main\x00\x00" +
		"worktree " + linked + "\x00" +
		"HEAD " + strings.Repeat("b", 40) + "\x00" +
		"detached\x00" +
		"locked build in progress\x00" +
		"prunable gitdir file points to non-existent location\x00\x00"
	rows := parseGitWorktreePorcelain(raw, root)
	if len(rows) != 2 {
		t.Fatalf("rows=%+v", rows)
	}
	if !rows[0].Current || !rows[0].Primary || rows[0].Branch != "main" {
		t.Fatalf("primary=%+v", rows[0])
	}
	second := rows[1]
	if second.Current || second.Primary || !second.Detached || !second.Locked || !second.Prunable {
		t.Fatalf("linked=%+v", second)
	}
	if second.LockReason != "build in progress" || !strings.Contains(second.PruneReason, "non-existent") {
		t.Fatalf("linked reasons=%+v", second)
	}
	if second.DisplayPath != "repo-fix" {
		t.Fatalf("display_path=%q", second.DisplayPath)
	}
	if second.ID == "" || second.ID == rows[0].ID {
		t.Fatalf("ids=%q %q", rows[0].ID, second.ID)
	}
}

func TestGitWorktreeByIDMatchesOnlyListedIdentity(t *testing.T) {
	rows := []gitWorktreeRow{
		{ID: "one", path: "/tmp/a"},
		{ID: "two", path: "/tmp/b"},
	}
	if got, ok := gitWorktreeByID(rows, "two"); !ok || got.path != "/tmp/b" {
		t.Fatalf("match=%+v ok=%v", got, ok)
	}
	if _, ok := gitWorktreeByID(rows, "../tmp/b"); ok {
		t.Fatal("arbitrary path must not match worktree identity")
	}
}
