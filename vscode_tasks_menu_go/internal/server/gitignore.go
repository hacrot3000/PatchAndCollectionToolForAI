package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type gitIgnoreSuggestion struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Pattern     string   `json:"pattern"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	MatchCount  int      `json:"match_count"`
	Samples     []string `json:"samples,omitempty"`
	Recommended bool     `json:"recommended,omitempty"`
	Broad       bool     `json:"broad,omitempty"`
}

func escapeGitIgnoreLiteral(value string) string {
	var b strings.Builder
	for _, r := range filepath.ToSlash(value) {
		switch r {
		case '\\', '*', '?', '[', ']':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func gitIgnorePatternJoin(dir, name string) string {
	if dir == "" || dir == "." {
		return "/" + name
	}
	return "/" + strings.TrimSuffix(filepath.ToSlash(dir), "/") + "/" + name
}

func escapeGitIgnoreGeneratedFamily(value string) string {
	parts := strings.Split(value, "*")
	for i := range parts {
		parts[i] = escapeGitIgnoreLiteral(parts[i])
	}
	return strings.Join(parts, "*")
}

func splitGitIgnoreName(path string) (dir, base, stem, ext string) {
	path = filepath.ToSlash(path)
	dir = filepath.ToSlash(filepath.Dir(path))
	if dir == "." {
		dir = ""
	}
	base = filepath.Base(path)
	ext = filepath.Ext(base)
	stem = strings.TrimSuffix(base, ext)
	return
}

func commonPrefixAtBoundary(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	if i < 3 {
		return ""
	}
	prefix := a[:i]
	for len(prefix) >= 3 {
		last := prefix[len(prefix)-1]
		if last == '-' || last == '_' || last == '.' || last == ' ' {
			return prefix
		}
		prefix = prefix[:len(prefix)-1]
	}
	return ""
}

func commonSuffixAtBoundary(a, b string) string {
	i, j := len(a)-1, len(b)-1
	for i >= 0 && j >= 0 && a[i] == b[j] {
		i--
		j--
	}
	suffix := a[i+1:]
	if len(suffix) < 3 {
		return ""
	}
	for len(suffix) >= 3 {
		first := suffix[0]
		if first == '-' || first == '_' || first == '.' || first == ' ' {
			return suffix
		}
		suffix = suffix[1:]
	}
	return ""
}

var (
	gitIgnoreUUIDPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	gitIgnoreHexPattern  = regexp.MustCompile(`(?i)(^|[-_.])[0-9a-f]{7,}($|[-_.])`)
	gitIgnoreDigitsPattern = regexp.MustCompile(`[0-9]{2,}`)
)

func normalizedGitIgnoreFamily(stem string) string {
	family := gitIgnoreUUIDPattern.ReplaceAllString(stem, "*")
	family = gitIgnoreHexPattern.ReplaceAllStringFunc(family, func(value string) string {
		prefix, suffix := "", ""
		if len(value) > 0 && strings.ContainsRune("-_.", rune(value[0])) {
			prefix = value[:1]
			value = value[1:]
		}
		if len(value) > 0 && strings.ContainsRune("-_.", rune(value[len(value)-1])) {
			suffix = value[len(value)-1:]
		}
		if value == "" {
			return prefix + suffix
		}
		return prefix + "*" + suffix
	})
	family = gitIgnoreDigitsPattern.ReplaceAllString(family, "*")
	for strings.Contains(family, "**") {
		family = strings.ReplaceAll(family, "**", "*")
	}
	return family
}

func commonGitIgnoreDirectorySuggestion(path string, isDir bool) (string, string, bool) {
	path = filepath.ToSlash(strings.TrimSuffix(path, "/"))
	parts := strings.Split(path, "/")
	limit := len(parts)
	if !isDir && limit > 0 {
		limit--
	}
	for i := 0; i < limit; i++ {
		lower := strings.ToLower(parts[i])
		switch lower {
		case "node_modules", ".cache", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", ".venv", "venv", ".gradle", ".terraform", "coverage", "dist", "out", "target", "build":
			folder := strings.Join(parts[:i+1], "/")
			return "/" + escapeGitIgnoreLiteral(folder) + "/", "Common generated/cache directory", true
		}
	}
	return "", "", false
}

func commonGitIgnoreFileSuggestion(path string) (string, string, bool) {
	path = filepath.ToSlash(path)
	base := filepath.Base(strings.TrimSuffix(path, "/"))
	lower := strings.ToLower(base)
	switch lower {
	case ".ds_store", "thumbs.db":
		return base, "Common operating-system metadata file", true
	case ".env", ".env.local", ".env.development.local", ".env.test.local", ".env.production.local":
		return ".env*", "Common local environment/secrets file", true
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".log", ".tmp", ".temp", ".swp", ".swo", ".bak", ".old", ".orig", ".rej", ".pyc", ".class", ".o", ".obj":
		return "*" + ext, "Common generated/temporary file type", true
	}
	if strings.HasSuffix(base, "~") {
		return "*~", "Common editor backup file", true
	}
	return "", "", false
}

func (s *Server) gitUntrackedPaths(ctx context.Context) ([]string, error) {
	out, _, _, err := s.runGit(ctx, 5*time.Second, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var outPaths []string
	for _, change := range parseGitStatusZ(out) {
		if change.Untracked {
			outPaths = append(outPaths, filepath.ToSlash(change.Path))
		}
	}
	sort.Strings(outPaths)
	return outPaths, nil
}

func pathIsUntracked(path string, paths []string) bool {
	path = strings.TrimSuffix(filepath.ToSlash(path), "/")
	for _, candidate := range paths {
		candidate = strings.TrimSuffix(filepath.ToSlash(candidate), "/")
		if candidate == path || strings.HasPrefix(candidate, path+"/") {
			return true
		}
	}
	return false
}

func (s *Server) gitIgnorePatternMatches(ctx context.Context, pattern string) ([]string, error) {
	out, _, _, err := s.runGit(ctx, 8*time.Second, "ls-files", "-z", "--others", "--ignored", "--exclude="+pattern)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(out, "\x00")
	rows := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			rows = append(rows, filepath.ToSlash(part))
		}
	}
	sort.Strings(rows)
	return rows, nil
}

func addGitIgnoreSuggestion(target map[string]gitIgnoreSuggestion, item gitIgnoreSuggestion) {
	if item.ID == "" || item.Pattern == "" {
		return
	}
	if _, exists := target[item.ID]; exists {
		return
	}
	target[item.ID] = item
}

func (s *Server) gitIgnoreSuggestions(ctx context.Context, rawPath string) ([]gitIgnoreSuggestion, error) {
	path, err := validGitRelativePath(rawPath)
	if err != nil {
		return nil, err
	}
	untracked, err := s.gitUntrackedPaths(ctx)
	if err != nil {
		return nil, err
	}
	if !pathIsUntracked(path, untracked) {
		return nil, fmt.Errorf("ignore is available only for untracked files or folders")
	}

	repoPath := filepath.Join(s.gitDirectory(ctx), filepath.FromSlash(path))
	info, statErr := os.Stat(repoPath)
	isDir := statErr == nil && info.IsDir()
	dir, base, stem, ext := splitGitIgnoreName(path)
	patterns := map[string]gitIgnoreSuggestion{}

	if isDir {
		addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
			ID: "exact-directory", Label: "Ignore this folder", Pattern: "/" + escapeGitIgnoreLiteral(strings.TrimSuffix(path, "/")) + "/",
			Kind: "exact", Description: "Ignore this exact repository-relative folder.", Recommended: true,
		})
	} else {
		addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
			ID: "exact-file", Label: "Ignore this file only", Pattern: "/" + escapeGitIgnoreLiteral(path),
			Kind: "exact", Description: "Ignore only the selected file.", Recommended: true,
		})
		if dir != "" {
			addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
				ID: "containing-directory", Label: "Ignore containing folder", Pattern: "/" + escapeGitIgnoreLiteral(dir) + "/",
				Kind: "directory", Description: "Ignore the selected file's whole containing folder.", Broad: true,
			})
		}
	}

	if base != "" {
		addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
			ID: "same-name", Label: "Ignore same filename anywhere", Pattern: escapeGitIgnoreLiteral(base),
			Kind: "similar", Description: "Ignore files with this exact basename in any folder.", Broad: true,
		})
	}
	if !isDir && ext != "" {
		escapedExt := escapeGitIgnoreLiteral(ext)
		addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
			ID: "same-extension-repo", Label: "Ignore same extension everywhere", Pattern: "*" + escapedExt,
			Kind: "extension", Description: "Ignore all untracked files with the same extension across the repository.", Broad: true,
		})
		addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
			ID: "same-extension-folder", Label: "Ignore same extension in this folder", Pattern: gitIgnorePatternJoin(dir, "*"+escapedExt),
			Kind: "extension", Description: "Ignore files with the same extension in the selected file's folder.",
		})
	}

	if !isDir && stem != "" {
		family := normalizedGitIgnoreFamily(stem)
		if family != stem && strings.Contains(family, "*") {
			addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
				ID: "normalized-family-folder", Label: "Ignore files from the same generated-name family",
				Pattern: gitIgnorePatternJoin(dir, escapeGitIgnoreGeneratedFamily(family)+escapeGitIgnoreLiteral(ext)),
				Kind: "family", Description: "Treat numeric/hash/version-like parts of the filename as variable.",
			})
		}

		for _, candidate := range untracked {
			cDir, cBase, cStem, cExt := splitGitIgnoreName(candidate)
			if cDir != dir || cBase == base || cExt != ext || cStem == "" {
				continue
			}
			if prefix := commonPrefixAtBoundary(stem, cStem); prefix != "" {
				pattern := gitIgnorePatternJoin(dir, escapeGitIgnoreLiteral(prefix)+"*"+escapeGitIgnoreLiteral(ext))
				addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
					ID: "shared-prefix:" + pattern, Label: "Ignore files with similar prefix",
					Pattern: pattern, Kind: "prefix", Description: "Ignore nearby files sharing the stable prefix " + prefix,
				})
			}
			if suffix := commonSuffixAtBoundary(stem, cStem); suffix != "" {
				pattern := gitIgnorePatternJoin(dir, "*"+escapeGitIgnoreLiteral(suffix)+escapeGitIgnoreLiteral(ext))
				addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
					ID: "shared-suffix:" + pattern, Label: "Ignore files with similar suffix",
					Pattern: pattern, Kind: "suffix", Description: "Ignore nearby files sharing the stable suffix " + suffix,
				})
			}
		}
	}

	if pattern, description, ok := commonGitIgnoreDirectorySuggestion(path, isDir); ok {
		addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
			ID: "common-directory", Label: "Ignore common generated/cache folder", Pattern: pattern, Kind: "common",
			Description: description + ".", Recommended: true,
		})
	}
	if !isDir {
		if pattern, description, ok := commonGitIgnoreFileSuggestion(path); ok {
			addGitIgnoreSuggestion(patterns, gitIgnoreSuggestion{
				ID: "common-file-rule", Label: "Use common file ignore rule", Pattern: pattern, Kind: "common",
				Description: description + ".", Recommended: true,
			})
		}
	}

	result := make([]gitIgnoreSuggestion, 0, len(patterns))
	for _, item := range patterns {
		matches, matchErr := s.gitIgnorePatternMatches(ctx, item.Pattern)
		if matchErr != nil {
			return nil, matchErr
		}
		item.MatchCount = len(matches)
		if len(matches) > 8 {
			item.Samples = append([]string(nil), matches[:8]...)
		} else {
			item.Samples = matches
		}
		switch item.Kind {
		case "exact", "directory", "common":
			// Keep exact and intentionally broad directory/common options even
			// when they currently match only the selected path.
		default:
			if item.MatchCount < 2 {
				continue
			}
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Recommended != result[j].Recommended {
			return result[i].Recommended
		}
		if result[i].Broad != result[j].Broad {
			return !result[i].Broad
		}
		if result[i].MatchCount != result[j].MatchCount {
			return result[i].MatchCount < result[j].MatchCount
		}
		return result[i].Label < result[j].Label
	})
	return result, nil
}

func appendGitIgnoreRule(repoDir, pattern string) (bool, error) {
	path := filepath.Join(repoDir, ".gitignore")
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("refusing to modify symlinked .gitignore")
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf(".gitignore is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if line == pattern {
			return false, nil
		}
	}
	prefix := ""
	if len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false, err
	}
	defer file.Close()
	if _, err := file.WriteString(prefix + pattern + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Server) gitIgnoreApply(ctx context.Context, rawPath, suggestionID string) (gitIgnoreSuggestion, bool, error) {
	suggestionID = strings.TrimSpace(suggestionID)
	if suggestionID == "" {
		return gitIgnoreSuggestion{}, false, fmt.Errorf("ignore suggestion id is required")
	}
	suggestions, err := s.gitIgnoreSuggestions(ctx, rawPath)
	if err != nil {
		return gitIgnoreSuggestion{}, false, err
	}
	for _, item := range suggestions {
		if item.ID != suggestionID {
			continue
		}
		added, err := appendGitIgnoreRule(s.gitDirectory(ctx), item.Pattern)
		return item, added, err
	}
	return gitIgnoreSuggestion{}, false, fmt.Errorf("ignore suggestion is no longer available; refresh Changes and try again")
}
