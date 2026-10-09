package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGitIgnoreTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { t.Fatal(err) }
}

func suggestionByID(rows []gitIgnoreSuggestion, id string) (gitIgnoreSuggestion, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return gitIgnoreSuggestion{}, false
}

func suggestionByPattern(rows []gitIgnoreSuggestion, pattern string) (gitIgnoreSuggestion, bool) {
	for _, row := range rows {
		if row.Pattern == pattern {
			return row, true
		}
	}
	return gitIgnoreSuggestion{}, false
}

func TestGitIgnoreSuggestionsFindExactExtensionPrefixSuffixAndCommonRules(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "logs/app-debug.log", "a\n")
	writeGitIgnoreTestFile(t, workspace, "logs/app-release.log", "b\n")
	writeGitIgnoreTestFile(t, workspace, "logs/task-debug.log", "c\n")

	rows, err := s.gitIgnoreSuggestions(context.Background(), "logs/app-debug.log")
	if err != nil { t.Fatal(err) }

	if exact, ok := suggestionByID(rows, "exact-file"); !ok || exact.Pattern != "/logs/app-debug.log" || exact.MatchCount != 1 || !exact.Recommended {
		t.Fatalf("exact suggestion=%+v ok=%v", exact, ok)
	}
	if ext, ok := suggestionByID(rows, "same-extension-folder"); !ok || ext.Pattern != "/logs/*.log" || ext.MatchCount != 3 {
		t.Fatalf("folder extension suggestion=%+v ok=%v rows=%+v", ext, ok, rows)
	}
	if prefix, ok := suggestionByPattern(rows, "/logs/app-*.log"); !ok || prefix.MatchCount != 2 || prefix.Kind != "prefix" {
		t.Fatalf("prefix suggestion=%+v ok=%v rows=%+v", prefix, ok, rows)
	}
	if suffix, ok := suggestionByPattern(rows, "/logs/*-debug.log"); !ok || suffix.MatchCount != 2 || suffix.Kind != "suffix" {
		t.Fatalf("suffix suggestion=%+v ok=%v rows=%+v", suffix, ok, rows)
	}
	if common, ok := suggestionByID(rows, "common-file-rule"); !ok || common.Pattern != "*.log" || common.MatchCount != 3 || !common.Recommended {
		t.Fatalf("common suggestion=%+v ok=%v rows=%+v", common, ok, rows)
	}
}

func TestGitIgnoreSuggestionsDetectGeneratedNameFamily(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "artifacts/report-20261003.json", "a\n")
	writeGitIgnoreTestFile(t, workspace, "artifacts/report-20261004.json", "b\n")
	writeGitIgnoreTestFile(t, workspace, "artifacts/report-20261005.json", "c\n")

	rows, err := s.gitIgnoreSuggestions(context.Background(), "artifacts/report-20261003.json")
	if err != nil { t.Fatal(err) }
	family, ok := suggestionByID(rows, "normalized-family-folder")
	if !ok {
		t.Fatalf("missing normalized family suggestion: %+v", rows)
	}
	if family.Pattern != "/artifacts/report-*.json" || family.MatchCount != 3 {
		t.Fatalf("family=%+v", family)
	}
}

func TestGitIgnoreSuggestionsPrioritizeCollapsedUntrackedDirectory(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "artifacts/a.bin", "a\n")
	writeGitIgnoreTestFile(t, workspace, "artifacts/nested/b.bin", "b\n")

	rows, err := s.gitIgnoreSuggestions(context.Background(), "artifacts/")
	if err != nil { t.Fatal(err) }
	if len(rows) == 0 {
		t.Fatal("expected ignore suggestions for collapsed untracked directory")
	}
	first := rows[0]
	if first.ID != "exact-directory" || first.Pattern != "/artifacts/" || !first.Recommended {
		t.Fatalf("first suggestion must be recommended exact directory: %+v rows=%+v", first, rows)
	}
	if first.MatchCount != 2 {
		t.Fatalf("exact directory match count=%d want 2; row=%+v", first.MatchCount, first)
	}
}

func TestGitIgnoreSuggestionsDetectCommonParentDirectory(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "build/generated/output.bin", "a\n")
	writeGitIgnoreTestFile(t, workspace, "build/generated/cache.bin", "b\n")

	rows, err := s.gitIgnoreSuggestions(context.Background(), "build/generated/output.bin")
	if err != nil { t.Fatal(err) }
	common, ok := suggestionByID(rows, "common-directory")
	if !ok {
		t.Fatalf("missing common directory suggestion: %+v", rows)
	}
	if common.Pattern != "/build/" || common.MatchCount != 2 || !common.Recommended {
		t.Fatalf("common directory=%+v", common)
	}
}

func TestGitIgnoreSuggestionsDetectFullBasenameGeneratedFamily(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	for _, name := range []string{
		".envs_cache_10.9.1.20_all",
		".envs_cache_10.9.1.20_gm",
		".envs_cache_10.9.4.21_all",
		".envs_cache_10.9.4.21_gm",
	} {
		writeGitIgnoreTestFile(t, workspace, name, "cache\n")
	}

	rows, err := s.gitIgnoreSuggestions(context.Background(), ".envs_cache_10.9.1.20_all")
	if err != nil { t.Fatal(err) }

	family, ok := suggestionByPattern(rows, "/.envs_cache_*_all")
	if !ok || family.MatchCount != 2 || family.Kind != "family" {
		t.Fatalf("full basename family=%+v ok=%v rows=%+v", family, ok, rows)
	}
	broad, ok := suggestionByPattern(rows, "/.envs_cache_10.9.*")
	if !ok || broad.MatchCount != 4 {
		t.Fatalf("shared full-name prefix=%+v ok=%v rows=%+v", broad, ok, rows)
	}
}

func TestGitIgnoreSuggestionsCanBroadenExistingRelatedRule(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, ".gitignore", ".envs_cache_*_gm\n")
	writeGitIgnoreTestFile(t, workspace, ".envs_cache_10.9.1.20_all", "cache\n")
	writeGitIgnoreTestFile(t, workspace, ".envs_cache_10.9.4.21_all", "cache\n")

	rows, err := s.gitIgnoreSuggestions(context.Background(), ".envs_cache_10.9.1.20_all")
	if err != nil { t.Fatal(err) }
	item, ok := suggestionByPattern(rows, "/.envs_cache_*")
	if !ok || item.MatchCount != 4 || item.Kind != "family" || !item.Broad {
		t.Fatalf("existing family suggestion=%+v ok=%v rows=%+v", item, ok, rows)
	}
}

func TestGitIgnoreApplyAcceptsValidatedCustomPattern(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, ".envs_cache_10.9.1.20_all", "cache\n")
	writeGitIgnoreTestFile(t, workspace, ".envs_cache_10.9.4.21_all", "cache\n")

	item, added, err := s.gitIgnoreApply(
		context.Background(),
		".envs_cache_10.9.1.20_all",
		"normalized-basename-family-folder",
		"/.envs_cache_10.9.*",
	)
	if err != nil { t.Fatal(err) }
	if !added || item.Pattern != "/.envs_cache_10.9.*" || item.MatchCount != 2 {
		t.Fatalf("custom item=%+v added=%v", item, added)
	}
	data, err := os.ReadFile(filepath.Join(workspace, ".gitignore"))
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "/.envs_cache_10.9.*\n") {
		t.Fatalf(".gitignore=%q", data)
	}
}

func TestGitIgnoreCustomPatternMustMatchSelectedPath(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "scratch/cache.tmp", "cache\n")

	_, _, err := s.gitIgnoreApply(context.Background(), "scratch/cache.tmp", "exact-file", "*.log")
	if err == nil || !strings.Contains(err.Error(), "does not match the selected path") {
		t.Fatalf("expected custom-pattern mismatch error, got %v", err)
	}
}

func TestGitIgnoreApplyUsesSuggestionIDAndWritesRootGitignore(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "scratch/session.tmp", "temporary\n")

	item, added, err := s.gitIgnoreApply(context.Background(), "scratch/session.tmp", "exact-file")
	if err != nil { t.Fatal(err) }
	if !added || item.Pattern != "/scratch/session.tmp" {
		t.Fatalf("item=%+v added=%v", item, added)
	}
	data, err := os.ReadFile(filepath.Join(workspace, ".gitignore"))
	if err != nil { t.Fatal(err) }
	if string(data) != "/scratch/session.tmp\n" {
		t.Fatalf(".gitignore=%q", data)
	}
	out := gitQuickRun(t, workspace, "status", "--porcelain=v1", "--untracked-files=all")
	if strings.Contains(out, "scratch/session.tmp") {
		t.Fatalf("ignored file still appears in status: %s", out)
	}

	_, added, err = s.gitIgnoreApply(context.Background(), "scratch/session.tmp", "exact-file")
	if err == nil || added || !strings.Contains(err.Error(), "untracked") {
		t.Fatalf("second apply should reject path that is now ignored: added=%v err=%v", added, err)
	}
}

func TestGitIgnoreApplyEscapesGlobCharactersInExactPath(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "scratch/a[1].tmp", "temporary\n")

	item, added, err := s.gitIgnoreApply(context.Background(), "scratch/a[1].tmp", "exact-file")
	if err != nil { t.Fatal(err) }
	if !added || item.Pattern != "/scratch/a\\[1\\].tmp" {
		t.Fatalf("item=%+v added=%v", item, added)
	}
	out := gitQuickRun(t, workspace, "status", "--porcelain=v1", "--untracked-files=all")
	if strings.Contains(out, "scratch/a[1].tmp") {
		t.Fatalf("escaped exact rule did not ignore file: %s", out)
	}
}

func TestGitIgnoreSuggestionsRejectTrackedPath(t *testing.T) {
	_, s, _ := setupGitQuickRepo(t)
	if _, err := s.gitIgnoreSuggestions(context.Background(), "tracked.txt"); err == nil || !strings.Contains(err.Error(), "untracked") {
		t.Fatalf("tracked path should not be offered for ignore: %v", err)
	}
}

func TestGitIgnoreApplyRejectsSymlinkedGitignore(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "scratch/cache.tmp", "temporary\n")
	outside := filepath.Join(t.TempDir(), "outside-ignore")
	if err := os.WriteFile(outside, []byte("keep\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(workspace, ".gitignore")); err != nil { t.Fatal(err) }

	_, _, err := s.gitIgnoreApply(context.Background(), "scratch/cache.tmp", "exact-file")
	if err == nil || !strings.Contains(err.Error(), "symlinked") {
		t.Fatalf("expected symlink refusal, got %v", err)
	}
	data, readErr := os.ReadFile(outside)
	if readErr != nil { t.Fatal(readErr) }
	if string(data) != "keep\n" {
		t.Fatalf("outside file was modified: %q", data)
	}
}

func TestGitIgnoreAPIViewAndAction(t *testing.T) {
	workspace, s, _ := setupGitQuickRepo(t)
	writeGitIgnoreTestFile(t, workspace, "notes/local.tmp", "temporary\n")
	writeGitIgnoreTestFile(t, workspace, "notes/other.tmp", "temporary\n")

	rr := callGitStatusHandler(t, s, "GET", "/api/git/status?view=ignore-suggestions&path=notes%2Flocal.tmp", "")
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"exact-file"`) || !strings.Contains(rr.Body.String(), `"suggestions"`) {
		t.Fatalf("suggestions status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, "GET", "/api/git/status?view=ignore-preview&path=notes%2Flocal.tmp&pattern=%2Fnotes%2F*.tmp", "")
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"match_count":2`) || !strings.Contains(rr.Body.String(), `"/notes/*.tmp"`) {
		t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = callGitStatusHandler(t, s, "POST", "/api/git/status", `{"action":"ignore","path":"notes/local.tmp","ignore_id":"exact-file","ignore_pattern":"/notes/*.tmp"}`)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"ok":true`) || !strings.Contains(rr.Body.String(), `"added":true`) || !strings.Contains(rr.Body.String(), `"/notes/*.tmp"`) {
		t.Fatalf("ignore action status=%d body=%s", rr.Code, rr.Body.String())
	}
}
