package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitSubmoduleRepo(t *testing.T) (string, *Server, string, string, string) {
	t.Helper()
	child := t.TempDir()
	gitQuickRun(t, child, "init")
	gitQuickRun(t, child, "config", "user.name", "Submodule Test")
	gitQuickRun(t, child, "config", "user.email", "submodule@example.invalid")
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, child, "add", "child.txt")
	gitQuickRun(t, child, "commit", "-m", "child one")
	first := gitQuickRun(t, child, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitQuickRun(t, child, "commit", "-am", "child two")
	second := gitQuickRun(t, child, "rev-parse", "HEAD")

	parent, s, _ := setupGitQuickRepo(t)
	gitQuickRun(t, parent, "config", "protocol.file.allow", "always")
	gitQuickRun(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "modules/child")
	gitQuickRun(t, filepath.Join(parent, "modules", "child"), "checkout", "--detach", first)
	gitQuickRun(t, parent, "add", ".gitmodules", "modules/child")
	gitQuickRun(t, parent, "commit", "-m", "add child submodule")
	expected := gitQuickRun(t, parent, "rev-parse", "HEAD:modules/child")
	if expected != first {
		t.Fatalf("expected gitlink=%q want=%q", expected, first)
	}
	return parent, s, first, second, child
}

func getSingleSubmodule(t *testing.T, s *Server) gitSubmoduleRow {
	t.Helper()
	rr := callGitStatusHandler(t, s, http.MethodGet, "/api/git/status?view=submodules", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("submodules status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Submodules []gitSubmoduleRow `json:"submodules"`
		Count      int               `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || len(got.Submodules) != 1 {
		t.Fatalf("submodules=%+v count=%d", got.Submodules, got.Count)
	}
	return got.Submodules[0]
}

func TestGitSubmodulesReportsExpectedActualAndCleanState(t *testing.T) {
	parent, s, first, _, childSource := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	if row.Name != "modules/child" && row.Name != "child" {
		// Git chooses the path as the default submodule name; keep the assertion
		// tolerant across Git versions while still requiring a concrete name.
		if strings.TrimSpace(row.Name) == "" {
			t.Fatalf("submodule name empty: %+v", row)
		}
	}
	if row.Path != "modules/child" || row.ExpectedSHA != first || row.ActualSHA != first {
		t.Fatalf("submodule row=%+v", row)
	}
	if !row.Initialized || row.Dirty || row.Mismatch || row.Status != "clean" {
		t.Fatalf("clean state=%+v", row)
	}
	if row.URL == "" || !strings.Contains(filepath.ToSlash(row.URL), filepath.Base(childSource)) {
		t.Fatalf("url=%q child=%q", row.URL, childSource)
	}
	if row.ID == "" || strings.Contains(row.ID, "/") {
		t.Fatalf("unsafe/empty ID=%q", row.ID)
	}
	if strings.Contains(mustJSON(t, row), filepath.Clean(parent)) {
		t.Fatalf("submodule row leaked parent absolute path: %+v", row)
	}
}

func TestGitSubmodulesDetectsDirtyAndCommitMismatch(t *testing.T) {
	parent, s, first, second, _ := setupGitSubmoduleRepo(t)
	child := filepath.Join(parent, "modules", "child")
	gitQuickRun(t, child, "checkout", "--detach", second)
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	row := getSingleSubmodule(t, s)
	if !row.Initialized || !row.Dirty || !row.Mismatch || row.Status != "dirty-mismatch" {
		t.Fatalf("dirty mismatch state=%+v", row)
	}
	if row.ExpectedSHA != first || row.ActualSHA != second {
		t.Fatalf("sha state=%+v", row)
	}
}

func TestGitSubmodulesDetectsUninitialized(t *testing.T) {
	parent, s, first, _, _ := setupGitSubmoduleRepo(t)
	gitQuickRun(t, parent, "submodule", "deinit", "-f", "--", "modules/child")
	row := getSingleSubmodule(t, s)
	if row.Initialized || row.Dirty || row.Mismatch || row.Status != "uninitialized" {
		t.Fatalf("uninitialized state=%+v", row)
	}
	if row.ExpectedSHA != first || row.ActualSHA != "" {
		t.Fatalf("uninitialized SHA state=%+v", row)
	}
}

func TestValidGitSubmodulePathRejectsTraversalAndAbsolute(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../outside", "a/../../outside", "/tmp/child"} {
		if got, err := validGitSubmodulePath(bad); err == nil {
			t.Fatalf("unsafe path %q accepted as %q", bad, got)
		}
	}
	for _, good := range []string{"modules/child", "vendor/lib-one"} {
		if got, err := validGitSubmodulePath(good); err != nil || got != good {
			t.Fatalf("valid path %q -> %q err=%v", good, got, err)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGitSubmoduleInitAndCheckoutExpectedCommit(t *testing.T) {
	parent, s, first, second, _ := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	gitQuickRun(t, parent, "submodule", "deinit", "-f", "--", row.Path)
	row = getSingleSubmodule(t, s)
	if row.Initialized {
		t.Fatalf("fixture remained initialized: %+v", row)
	}

	body := `{"action":"submodule_init","submodule_id":"` + row.ID + `","expected_sha":"` + first + `","confirmed":true}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("init status=%d body=%s", rr.Code, rr.Body.String())
	}
	row = getSingleSubmodule(t, s)
	if !row.Initialized || row.ActualSHA != first || row.Mismatch || row.Dirty {
		t.Fatalf("after init=%+v", row)
	}

	child := filepath.Join(parent, filepath.FromSlash(row.Path))
	gitQuickRun(t, child, "checkout", "--detach", second)
	row = getSingleSubmodule(t, s)
	if !row.Mismatch || row.Dirty {
		t.Fatalf("mismatch fixture=%+v", row)
	}
	body = `{"action":"submodule_checkout_expected","submodule_id":"` + row.ID + `","expected_sha":"` + first + `","confirmed":true}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("checkout expected status=%d body=%s", rr.Code, rr.Body.String())
	}
	row = getSingleSubmodule(t, s)
	if row.ActualSHA != first || row.Mismatch || row.Dirty {
		t.Fatalf("after checkout expected=%+v", row)
	}
}

func TestGitSubmoduleUpdateBlocksDirtyAndStaleExpectedSHA(t *testing.T) {
	parent, s, first, _, _ := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	child := filepath.Join(parent, filepath.FromSlash(row.Path))
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("dirty local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"action":"submodule_update","submodule_id":"` + row.ID + `","expected_sha":"` + first + `","confirmed":true}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "dirty_worktree") {
		t.Fatalf("dirty update status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"submodule_update_recursive","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "dirty_worktree") {
		t.Fatalf("dirty recursive status=%d body=%s", rr.Code, rr.Body.String())
	}

	gitQuickRun(t, child, "restore", "child.txt")
	stale := strings.Repeat("f", len(first))
	body = `{"action":"submodule_update","submodule_id":"` + row.ID + `","expected_sha":"` + stale + `","confirmed":true}`
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "expected commit changed") {
		t.Fatalf("stale update status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"submodule_update","submodule_id":"../../child","confirmed":true}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown ID status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitSubmoduleRecursiveUpdateAndSync(t *testing.T) {
	parent, s, first, second, _ := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	child := filepath.Join(parent, filepath.FromSlash(row.Path))
	gitQuickRun(t, child, "checkout", "--detach", second)

	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"submodule_update_recursive","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("recursive update status=%d body=%s", rr.Code, rr.Body.String())
	}
	row = getSingleSubmodule(t, s)
	if row.ActualSHA != first || row.Mismatch {
		t.Fatalf("recursive update row=%+v", row)
	}

	alt := t.TempDir()
	gitQuickRun(t, alt, "init", "--bare")
	gitQuickRun(t, parent, "config", "-f", ".gitmodules", "submodule."+row.Name+".url", alt)
	rr = callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", `{"action":"submodule_sync","confirmed":true}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("sync status=%d body=%s", rr.Code, rr.Body.String())
	}
	got := gitQuickRun(t, parent, "config", "--get", "submodule."+row.Name+".url")
	if filepath.Clean(got) != filepath.Clean(alt) {
		t.Fatalf("synced url=%q want=%q", got, alt)
	}
}

func TestGitSubmoduleMutationsRequireConfirmation(t *testing.T) {
	_, s, first, _, _ := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	for _, body := range []string{
		`{"action":"submodule_update","submodule_id":"` + row.ID + `","expected_sha":"` + first + `"}`,
		`{"action":"submodule_update_recursive"}`,
		`{"action":"submodule_sync"}`,
	} {
		rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "confirmation_required") {
			t.Fatalf("confirmation status=%d body=%s request=%s", rr.Code, rr.Body.String(), body)
		}
	}
}

func TestGitSubmoduleInitCannotBypassDirtyGuard(t *testing.T) {
	parent, s, first, _, _ := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	child := filepath.Join(parent, filepath.FromSlash(row.Path))
	if err := os.WriteFile(filepath.Join(child, "child.txt"), []byte("dirty init bypass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"action":"submodule_init","submodule_id":"` + row.ID + `","expected_sha":"` + first + `","confirmed":true}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "dirty_worktree") {
		t.Fatalf("dirty init status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(child, "child.txt")); err != nil || string(data) != "dirty init bypass\n" {
		t.Fatalf("dirty child changed=%q err=%v", data, err)
	}
}

func TestGitSubmoduleMutationRejectsEntryWithoutHeadGitlink(t *testing.T) {
	parent, s, _, _, _ := setupGitSubmoduleRepo(t)
	row := getSingleSubmodule(t, s)
	gitQuickRun(t, parent, "rm", "--cached", "-f", "--", row.Path)
	gitQuickRun(t, parent, "commit", "-m", "remove gitlink but keep gitmodules")
	row = getSingleSubmodule(t, s)
	if row.ExpectedSHA != "" {
		t.Fatalf("expected missing gitlink, row=%+v", row)
	}
	body := `{"action":"submodule_update","submodule_id":"` + row.ID + `","confirmed":true}`
	rr := callGitStatusHandler(t, s, http.MethodPost, "/api/git/status", body)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "no gitlink commit recorded") {
		t.Fatalf("missing gitlink status=%d body=%s", rr.Code, rr.Body.String())
	}
}
