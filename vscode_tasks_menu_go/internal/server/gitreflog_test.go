package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitReflogRecoveryRepo(t *testing.T) (string, *Server, string, string) {
	t.Helper()
	workspace, s, _ := setupGitQuickRepo(t)
	initial := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("recover me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "commit", "-am", "recoverable commit")
	lost := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	gitQuickRun(t, workspace, "reset", "--hard", initial)
	return workspace, s, initial, lost
}

func TestGitReflogFindsCommitLostAfterReset(t *testing.T) {
	_, s, _, lost := setupGitReflogRecoveryRepo(t)
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=reflog&limit=100", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("reflog status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Entries []gitReflogEntry `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	foundLost, foundReset := false, false
	for _, entry := range got.Entries {
		if entry.SHA == lost && strings.Contains(entry.Subject, "recoverable commit") {
			foundLost = true
		}
		if entry.Kind == "reset" {
			foundReset = true
		}
		if entry.Selector == "" || entry.Ref == "" {
			t.Fatalf("reflog entry missing selector/ref: %+v", entry)
		}
	}
	if !foundLost || !foundReset {
		t.Fatalf("entries=%+v foundLost=%v foundReset=%v", got.Entries, foundLost, foundReset)
	}

	search := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=reflog&search="+lost[:10], "")
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), lost) {
		t.Fatalf("search status=%d body=%s", search.Code, search.Body.String())
	}
	resetOnly := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=reflog&kind=reset", "")
	if resetOnly.Code != http.StatusOK || !strings.Contains(resetOnly.Body.String(), `"kind":"reset"`) {
		t.Fatalf("reset filter status=%d body=%s", resetOnly.Code, resetOnly.Body.String())
	}
}

func TestGitReflogRecoveryBranchDoesNotTouchDirtyWorktreeOrHEAD(t *testing.T) {
	workspace, s, initial, lost := setupGitReflogRecoveryRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("dirty after reset\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"action":"create_branch_ref","branch":"recovery/lost","ref":"` + lost + `","expected_sha":"` + lost + `"}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("recovery branch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "recovery/lost"); got != lost {
		t.Fatalf("recovery branch=%q want %q", got, lost)
	}
	if got := gitQuickRun(t, workspace, "rev-parse", "HEAD"); got != initial {
		t.Fatalf("HEAD moved to %q want %q", got, initial)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "dirty after reset\n" {
		t.Fatalf("dirty worktree changed: %q", data)
	}
}
