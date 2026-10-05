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
