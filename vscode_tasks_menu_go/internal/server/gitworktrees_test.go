package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
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

func TestGitWorktreeCreateExistingAndNewBranch(t *testing.T) {
	workspace, s, mainBranch := setupGitQuickRepo(t)
	head := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "branch", "feature/existing")

	existingBody := `{"action":"worktree_add_branch","branch":"feature/existing","directory_name":"wt-existing"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", existingBody)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("add existing status=%d body=%s", rr.Code, rr.Body.String())
	}
	existingPath := filepath.Join(filepath.Dir(workspace), "wt-existing")
	if got := gitQuickRun(t, existingPath, "branch", "--show-current"); got != "feature/existing" {
		t.Fatalf("existing branch=%q", got)
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != mainBranch {
		t.Fatalf("active branch changed=%q want=%q", got, mainBranch)
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", existingBody)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "already checked out") {
		t.Fatalf("duplicate branch status=%d body=%s", rr.Code, rr.Body.String())
	}

	newBody := `{"action":"worktree_add_new_branch","branch":"feature/new-worktree","directory_name":"wt-new","ref":"HEAD","expected_sha":"` + head + `"}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", newBody)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("add new status=%d body=%s", rr.Code, rr.Body.String())
	}
	newPath := filepath.Join(filepath.Dir(workspace), "wt-new")
	if got := gitQuickRun(t, newPath, "branch", "--show-current"); got != "feature/new-worktree" {
		t.Fatalf("new branch=%q", got)
	}
	if got := gitQuickRun(t, newPath, "rev-parse", "HEAD"); got != head {
		t.Fatalf("new worktree HEAD=%q want=%q", got, head)
	}
}

func TestGitWorktreeCreateRejectsUnsafeTargetAndStaleSource(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	head := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "branch", "feature/existing")

	for _, name := range []string{"../escape", "nested/path", "nested\\path", "..", ""} {
		body := `{"action":"worktree_add_branch","branch":"feature/existing","directory_name":"` + strings.ReplaceAll(name, "\\", "\\\\") + `"}`
		rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("unsafe %q status=%d body=%s", name, rr.Code, rr.Body.String())
		}
	}
	stale := strings.Repeat("f", len(head))
	body := `{"action":"worktree_add_new_branch","branch":"feature/stale","directory_name":"wt-stale","ref":"HEAD","expected_sha":"` + stale + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "source changed") {
		t.Fatalf("stale source status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitWorktreeRemoveUsesOpaqueListedIdentityAndRequiresConfirmation(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	linked := filepath.Join(filepath.Dir(workspace), "wt-remove")
	gitQuickRun(t, workspace, "worktree", "add", "-b", "feature/remove", linked)
	rows, err := s.gitWorktreeRows(withGitRepository(context.Background(), gitRepository{ID: ".", Name: filepath.Base(workspace), Path: ".", Root: workspace}))
	if err != nil {
		t.Fatal(err)
	}
	var target gitWorktreeRow
	for _, row := range rows {
		if row.Branch == "feature/remove" {
			target = row
		}
	}
	if target.ID == "" {
		t.Fatalf("linked worktree not found: %+v", rows)
	}

	body := `{"action":"worktree_remove","worktree_id":"` + target.ID + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed remove status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatalf("unconfirmed remove changed worktree: %v", err)
	}

	body = `{"action":"worktree_remove","worktree_id":"` + target.ID + `","confirmed":true}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("remove status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(linked); !os.IsNotExist(err) {
		t.Fatalf("linked worktree still exists err=%v", err)
	}

	stale := `{"action":"worktree_remove","worktree_id":"../../tmp","confirmed":true}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", stale)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("arbitrary ID status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitWorktreePruneRequiresConfirmation(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	linked := filepath.Join(filepath.Dir(workspace), "wt-prune")
	gitQuickRun(t, workspace, "worktree", "add", "-b", "feature/prune", linked)
	if err := os.RemoveAll(linked); err != nil {
		t.Fatal(err)
	}

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"worktree_prune"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed prune status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"worktree_prune","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("prune status=%d body=%s", rr.Code, rr.Body.String())
	}
	out := gitQuickRun(t, workspace, "worktree", "list", "--porcelain")
	if strings.Contains(out, "feature/prune") || strings.Contains(out, linked) {
		t.Fatalf("stale worktree not pruned: %s", out)
	}
}

func TestGitWorktreeOpenResolvesOpaqueIDServerSide(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	linked := filepath.Join(filepath.Dir(workspace), "wt-open")
	gitQuickRun(t, workspace, "worktree", "add", "-b", "feature/open", linked)

	ctx := withGitRepository(context.Background(), gitRepository{ID: ".", Name: filepath.Base(workspace), Path: ".", Root: workspace})
	rows, err := s.gitWorktreeRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target gitWorktreeRow
	for _, row := range rows {
		if row.Branch == "feature/open" {
			target = row
		}
	}
	if target.ID == "" {
		t.Fatalf("target not found: %+v", rows)
	}

	opened := ""
	s.OpenWorkspace = func(path string) error {
		opened = path
		return nil
	}
	body := `{"action":"worktree_open","worktree_id":"` + target.ID + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("open status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sameGitWorktreePath(opened, linked) {
		t.Fatalf("opened=%q want=%q", opened, linked)
	}
	if strings.Contains(rr.Body.String(), filepath.Clean(linked)) {
		t.Fatalf("open response leaked absolute path: %s", rr.Body.String())
	}

	opened = ""
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"worktree_open","worktree_id":"../../wt-open"}`)
	if rr.Code != http.StatusNotFound || opened != "" {
		t.Fatalf("arbitrary ID status=%d opened=%q body=%s", rr.Code, opened, rr.Body.String())
	}
}
