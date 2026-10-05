package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitFileHistoryFollowsTrackedFile(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	if err := os.Rename(filepath.Join(workspace, "tracked.txt"), filepath.Join(workspace, "renamed.txt")); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "-A")
	gitQuickRun(t, workspace, "commit", "-m", "rename tracked file")
	if err := os.WriteFile(filepath.Join(workspace, "renamed.txt"), []byte("two\n"), 0o644); err != nil { t.Fatal(err) }
	gitQuickRun(t, workspace, "add", "renamed.txt")
	gitQuickRun(t, workspace, "commit", "-m", "update renamed file")

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=file-history&path=renamed.txt&limit=20", "")
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
	var response struct {
		Path string `json:"path"`
		Commits []gitCommitRow `json:"commits"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if response.Path != "renamed.txt" || len(response.Commits) < 3 {
		t.Fatalf("response=%+v", response)
	}
	subjects := []string{}
	for _, commit := range response.Commits { subjects = append(subjects, commit.Subject) }
	joined := strings.Join(subjects, "\n")
	for _, want := range []string{"update renamed file", "rename tracked file", "initial"} {
		if !strings.Contains(joined, want) { t.Fatalf("subjects=%q missing %q", joined, want) }
	}
}

func TestGitBlameIncludesCommittedAndWorkingTreeLines(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("one\ntwo\n"), 0o644); err != nil { t.Fatal(err) }

	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=blame&path=tracked.txt", "")
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
	var response struct {
		Path string `json:"path"`
		Lines []gitBlameRow `json:"lines"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil { t.Fatal(err) }
	if response.Path != "tracked.txt" || len(response.Lines) != 2 {
		t.Fatalf("response=%+v", response)
	}
	if response.Lines[0].Text != "one" || !response.Lines[0].Committed {
		t.Fatalf("first line=%+v", response.Lines[0])
	}
	if response.Lines[1].Text != "two" || response.Lines[1].Committed {
		t.Fatalf("working-tree line=%+v", response.Lines[1])
	}
}

func TestParseGitBlamePorcelain(t *testing.T) {
	raw := "0123456789abcdef0123456789abcdef01234567 1 3 1\n" +
		"author Alice\nauthor-time 1700000000\nsummary Example change\nfilename tracked.txt\n\tline text\n"
	rows, err := parseGitBlamePorcelain(raw)
	if err != nil { t.Fatal(err) }
	if len(rows) != 1 || rows[0].Line != 3 || rows[0].Author != "Alice" || rows[0].Summary != "Example change" || rows[0].Text != "line text" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestGitFileContentReturnsExactCommitVersionForCompare(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	commit := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if len(commit) != 40 && len(commit) != 64 {
		t.Fatalf("unexpected commit=%q", commit)
	}
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("working tree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := "/api/git/status?view=file-content&repo=.&path=tracked.txt&ref=" + commit
	rr := callGitStatusHandler(t, s, http.MethodGet, target, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("file-content status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Path    string `json:"path"`
		Ref     string `json:"ref"`
		Commit  string `json:"commit"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != "tracked.txt" || got.Ref != commit || got.Commit != commit || got.Content != "one\n" {
		t.Fatalf("unexpected Git file content: %+v", got)
	}
}

func TestGitFileContentRejectsUnsafeRefAndPath(t *testing.T) {
	_, s, _ := setupGitQuickRepo(t)
	for _, target := range []string{
		"/api/git/status?view=file-content&repo=.&path=tracked.txt&ref=HEAD",
		"/api/git/status?view=file-content&repo=.&path=../outside&ref=0123456789012345678901234567890123456789",
	} {
		rr := callGitStatusHandler(t, s, http.MethodGet, target, "")
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", target, rr.Code, rr.Body.String())
		}
	}
}

func TestGitFileContentReturnsHeadAndIndexStatesForCompare(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "tracked.txt")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("working\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		state string
		want  string
	}{
		{state: "head", want: "one\n"},
		{state: "index", want: "staged\n"},
	} {
		target := "/api/git/status?view=file-content&repo=.&path=tracked.txt&state=" + test.state
		rr := callGitStatusHandler(t, s, http.MethodGet, target, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("state=%s status=%d body=%s", test.state, rr.Code, rr.Body.String())
		}
		var got struct {
			State   string `json:"state"`
			Content string `json:"content"`
			Commit  string `json:"commit"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.State != test.state || got.Content != test.want {
			t.Fatalf("state=%s response=%+v", test.state, got)
		}
		if test.state == "head" && got.Commit == "" {
			t.Fatal("HEAD state must return resolved commit")
		}
		if test.state == "index" && got.Commit != "" {
			t.Fatalf("index state unexpectedly returned commit %q", got.Commit)
		}
	}
}

func TestGitFileContentAllowMissingForCommitCompare(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	parent := gitQuickRun(t, workspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(workspace, "added later.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, workspace, "add", "added later.txt")
	gitQuickRun(t, workspace, "commit", "-m", "add later file")
	commit := gitQuickRun(t, workspace, "rev-parse", "HEAD")

	missing := callGitStatusHandler(t, s, http.MethodGet,
		"/api/git/status?view=file-content&path=added%20later.txt&ref="+parent+"&allow_missing=1", "")
	if missing.Code != http.StatusOK {
		t.Fatalf("missing side status=%d body=%s", missing.Code, missing.Body.String())
	}
	var left struct {
		Exists  bool   `json:"exists"`
		Content string `json:"content"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(missing.Body.Bytes(), &left); err != nil {
		t.Fatal(err)
	}
	if left.Exists || left.Content != "" || left.Commit != parent {
		t.Fatalf("missing side=%+v", left)
	}

	present := callGitStatusHandler(t, s, http.MethodGet,
		"/api/git/status?view=file-content&path=added%20later.txt&ref="+commit+"&allow_missing=1", "")
	if present.Code != http.StatusOK {
		t.Fatalf("present side status=%d body=%s", present.Code, present.Body.String())
	}
	var right struct {
		Exists  bool   `json:"exists"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(present.Body.Bytes(), &right); err != nil {
		t.Fatal(err)
	}
	if !right.Exists || right.Content != "later\n" {
		t.Fatalf("present side=%+v", right)
	}
}
