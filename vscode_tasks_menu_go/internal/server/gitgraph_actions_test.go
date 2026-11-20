package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitGraphActionRepo(t *testing.T) (string, *Server, string, string) {
	t.Helper()
	workspace, s, _ := setupGitQuickRepo(t)
	initial := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "commit", "-am", "second")
	second := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	return workspace, s, initial, second
}

func TestGitGraphCheckoutCommitRequiresCleanTreeAndDetaches(t *testing.T) {
	workspace, s, initial, _ := setupGitGraphActionRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"action":"checkout_commit","ref":"` + initial + `","expected_sha":"` + initial + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "dirty_worktree") {
		t.Fatalf("dirty checkout status=%d body=%s", rr.Code, rr.Body.String())
	}
	gitQuickRun(t, workspace, "restore", "tracked.txt")

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("checkout status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != initial {
		t.Fatalf("HEAD=%q want %q", got, initial)
	}
	cmdBranch := strings.TrimSpace(gitQuickRun(t, workspace, "branch", "--show-current"))
	if cmdBranch != "" {
		t.Fatalf("expected detached HEAD, branch=%q", cmdBranch)
	}
}

func TestGitGraphCreateBranchAtCommit(t *testing.T) {
	workspace, s, initial, _ := setupGitGraphActionRepo(t)
	body := `{"action":"create_branch_at","branch":"graph/from-old","ref":"` + initial + `","expected_sha":"` + initial + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("create branch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "branch", "--show-current"); got != "graph/from-old" {
		t.Fatalf("branch=%q", got)
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != initial {
		t.Fatalf("HEAD=%q want %q", got, initial)
	}
}

func TestGitGraphResetRequiresConfirmationAndHardResets(t *testing.T) {
	workspace, s, initial, second := setupGitGraphActionRepo(t)
	unconfirmed := `{"action":"reset_commit","mode":"hard","ref":"` + initial + `","expected_sha":"` + initial + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", unconfirmed)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed reset status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != second {
		t.Fatalf("unconfirmed reset moved HEAD to %q", got)
	}

	invalid := `{"action":"reset_commit","mode":"destroy","confirmed":true,"ref":"` + initial + `","expected_sha":"` + initial + `"}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", invalid)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode status=%d body=%s", rr.Code, rr.Body.String())
	}

	hard := `{"action":"reset_commit","mode":"hard","confirmed":true,"ref":"` + initial + `","expected_sha":"` + initial + `"}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", hard)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("hard reset status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != initial {
		t.Fatalf("HEAD=%q want %q", got, initial)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\n" {
		t.Fatalf("hard reset content=%q", data)
	}
}

func TestGitGraphCommitActionsRejectStaleExpectedSHA(t *testing.T) {
	_, s, initial, second := setupGitGraphActionRepo(t)
	body, _ := json.Marshal(map[string]any{
		"action": "checkout_commit",
		"ref": second,
		"expected_sha": initial,
	})
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", string(body))
	if rr.Code != http.StatusConflict {
		t.Fatalf("stale expected SHA status=%d body=%s", rr.Code, rr.Body.String())
	}
}
