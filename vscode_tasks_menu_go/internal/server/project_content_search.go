package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	projectContentSearchDefaultLimit    = 100
	projectContentSearchMaxLimit        = 200
	projectContentSearchMaxLineBytes    = 256 << 10
	projectContentSearchMaxPreviewBytes = 4 << 10
)

type projectContentSearchResult struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Preview string `json:"preview"`
}

type projectContentSearchOptions struct {
	Query         string
	Regex         bool
	CaseSensitive bool
	WholeWord     bool
	Include       []string
	Exclude       []string
	Scope         string
}

type rgJSONText struct {
	Text string `json:"text"`
}

type rgJSONMatch struct {
	Type string `json:"type"`
	Data struct {
		Path       rgJSONText `json:"path"`
		LineNumber int        `json:"line_number"`
		Lines      rgJSONText `json:"lines"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
	} `json:"data"`
}

func (s *Server) projectContentSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	options := projectContentSearchOptions{
		Query:         strings.TrimSpace(r.URL.Query().Get("q")),
		Regex:         r.URL.Query().Get("regex") == "1",
		CaseSensitive: r.URL.Query().Get("case") == "1",
		WholeWord:     r.URL.Query().Get("word") == "1",
		Include:       splitProjectSearchGlobs(r.URL.Query().Get("include")),
		Exclude:       splitProjectSearchGlobs(r.URL.Query().Get("exclude")),
	}
	if scope := strings.TrimSpace(r.URL.Query().Get("path")); scope != "" && scope != "." {
		rel, cleanErr := cleanProjectRelativePath(scope, true)
		if cleanErr != nil {
			http.Error(w, cleanErr.Error(), http.StatusBadRequest)
			return
		}
		if _, resolveErr := s.resolveProjectPath(rel, true, true); resolveErr != nil {
			http.Error(w, "project search folder unavailable", http.StatusNotFound)
			return
		}
		options.Scope = rel
	}
	limit := projectContentSearchDefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > projectContentSearchMaxLimit {
		limit = projectContentSearchMaxLimit
	}
	if options.Query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": []projectContentSearchResult{}})
		return
	}
	root, err := s.projectRoot()
	if err != nil {
		http.Error(w, "project root unavailable", http.StatusInternalServerError)
		return
	}
	results, err := searchProjectContentWithOptions(r.Context(), root, options, limit)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, "project content search failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func searchProjectContent(ctx context.Context, root, query string, limit int) ([]projectContentSearchResult, error) {
	return searchProjectContentWithOptions(ctx, root, projectContentSearchOptions{Query: query, CaseSensitive: true}, limit)
}

func searchProjectContentWithOptions(ctx context.Context, root string, options projectContentSearchOptions, limit int) ([]projectContentSearchResult, error) {
	if limit <= 0 {
		return []projectContentSearchResult{}, nil
	}
	if _, err := projectSearchRegexp(options); err != nil {
		return nil, err
	}
	if rg, err := exec.LookPath("rg"); err == nil {
		if results, err := searchProjectContentRG(ctx, rg, root, options, limit); err == nil {
			return results, nil
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
	}
	return searchProjectContentFallbackWithOptions(ctx, root, options, limit)
}

func searchProjectContentRG(parent context.Context, rg, root string, options projectContentSearchOptions, limit int) ([]projectContentSearchResult, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	args := []string{"--json", "--color", "never", "--line-number", "--column", "--glob", "!.git/**"}
	if !options.Regex {
		args = append(args, "--fixed-strings")
	}
	if options.CaseSensitive {
		args = append(args, "--case-sensitive")
	} else {
		args = append(args, "--ignore-case")
	}
	if options.WholeWord {
		args = append(args, "--word-regexp")
	}
	for _, pattern := range options.Include {
		args = append(args, "--glob", pattern)
	}
	for _, pattern := range options.Exclude {
		args = append(args, "--glob", "!"+pattern)
	}
	target := "."
	if options.Scope != "" {
		target = options.Scope
	}
	args = append(args, "--", options.Query, target)
	cmd := exec.CommandContext(ctx, rg, args...)
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	results := make([]projectContentSearchResult, 0, minInt(limit, 32))
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), projectContentSearchMaxLineBytes)
	for scanner.Scan() {
		if err := parent.Err(); err != nil {
			cancel()
			_ = cmd.Wait()
			return nil, err
		}
		var event rgJSONMatch
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Type != "match" {
			continue
		}
		rel := normalizeProjectIndexPath(strings.TrimPrefix(event.Data.Path.Text, "./"))
		if rel == "" {
			continue
		}
		lineText := strings.TrimRight(event.Data.Lines.Text, "\r\n")
		matchStart := 0
		if len(event.Data.Submatches) > 0 {
			matchStart = event.Data.Submatches[0].Start
		}
		results = append(results, projectContentSearchResult{
			Path: rel, Line: event.Data.LineNumber,
			Column: projectUTF16Column(lineText, matchStart),
			Preview: boundedProjectSearchPreview(lineText, matchStart),
		})
		if len(results) >= limit {
			cancel()
			break
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if len(results) >= limit {
		return results, nil
	}
	if scanErr != nil {
		return nil, scanErr
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 {
			return results, nil
		}
		if ctx.Err() != nil && parent.Err() == nil {
			return results, nil
		}
		return nil, waitErr
	}
	return results, nil
}

func searchProjectContentFallback(ctx context.Context, root, query string, limit int) ([]projectContentSearchResult, error) {
	return searchProjectContentFallbackWithOptions(ctx, root, projectContentSearchOptions{Query: query, CaseSensitive: true}, limit)
}

func searchProjectContentFallbackWithOptions(ctx context.Context, root string, options projectContentSearchOptions, limit int) ([]projectContentSearchResult, error) {
	matcher, err := projectSearchRegexp(options)
	if err != nil {
		return nil, err
	}
	results := make([]projectContentSearchResult, 0, minInt(limit, 32))
	ignore := loadProjectRootIgnore(root)
	walkRoot := root
	if options.Scope != "" {
		walkRoot = filepath.Join(root, filepath.FromSlash(options.Scope))
	}
	err = filepath.WalkDir(walkRoot, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if full == root {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return nil
		}
		rel = normalizeProjectIndexPath(filepath.ToSlash(rel))
		if rel == "" {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || ignore.matches(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || ignore.matches(rel, false) || !projectSearchPathAllowed(rel, options) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > projectReadableLimit {
			return nil
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil
		}
		if len(data) == 0 || !projectContentTextBytes(data) {
			return nil
		}
		for lineNumber, start := 1, 0; start <= len(data); lineNumber++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := bytes.IndexByte(data[start:], '\n')
			next := len(data)
			if end >= 0 {
				next = start + end
			}
			line := bytes.TrimSuffix(data[start:next], []byte{'\r'})
			lineText := string(line)
			for _, match := range matcher.FindAllStringIndex(lineText, -1) {
				at := match[0]
				results = append(results, projectContentSearchResult{
					Path: rel, Line: lineNumber,
					Column: projectUTF16Column(lineText, at),
					Preview: boundedProjectSearchPreview(lineText, at),
				})
				if len(results) >= limit {
					return errProjectContentSearchLimit
				}
			}
			if end < 0 {
				break
			}
			start = next + 1
		}
		return nil
	})
	if errors.Is(err, errProjectContentSearchLimit) {
		return results, nil
	}
	return results, err
}

func splitProjectSearchGlobs(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = filepath.ToSlash(strings.TrimSpace(field))
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func projectSearchRegexp(options projectContentSearchOptions) (*regexp.Regexp, error) {
	pattern := options.Query
	if !options.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if options.WholeWord {
		pattern = `\b(?:` + pattern + `)\b`
	}
	if !options.CaseSensitive {
		pattern = "(?i:" + pattern + ")"
	}
	return regexp.Compile(pattern)
}

func projectSearchGlobMatch(pattern, rel string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	rel = filepath.ToSlash(rel)
	if pattern == "" {
		return false
	}
	if ok, _ := path.Match(pattern, rel); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, path.Base(rel)); ok {
			return true
		}
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	return false
}

func projectSearchPathAllowed(rel string, options projectContentSearchOptions) bool {
	if len(options.Include) > 0 {
		matched := false
		for _, pattern := range options.Include {
			if projectSearchGlobMatch(pattern, rel) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, pattern := range options.Exclude {
		if projectSearchGlobMatch(pattern, rel) {
			return false
		}
	}
	return true
}

var errProjectContentSearchLimit = errors.New("project content search result limit reached")

type projectIgnoreRule struct {
	pattern string
	negate  bool
	dirOnly bool
	anchored bool
}

type projectIgnoreMatcher struct {
	rules []projectIgnoreRule
}

func loadProjectRootIgnore(root string) projectIgnoreMatcher {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return projectIgnoreMatcher{}
	}
	matcher := projectIgnoreMatcher{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := projectIgnoreRule{}
		if strings.HasPrefix(line, "!") {
			rule.negate = true
			line = strings.TrimPrefix(line, "!")
		}
		rule.dirOnly = strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		rule.anchored = strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		line = filepath.ToSlash(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		rule.pattern = line
		matcher.rules = append(matcher.rules, rule)
	}
	return matcher
}

func (m projectIgnoreMatcher) matches(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	ignored := false
	for _, rule := range m.rules {
		if rule.dirOnly && !isDir && !strings.Contains(rel, rule.pattern+"/") && !strings.HasPrefix(rel, rule.pattern+"/") {
			continue
		}
		if projectIgnoreRuleMatches(rule, rel) {
			ignored = !rule.negate
		}
	}
	return ignored
}

func projectIgnoreRuleMatches(rule projectIgnoreRule, rel string) bool {
	pattern := rule.pattern
	if rule.anchored || strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, rel); ok {
			return true
		}
		if rule.dirOnly && (rel == pattern || strings.HasPrefix(rel, pattern+"/")) {
			return true
		}
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if ok, _ := path.Match(pattern, part); ok {
			return true
		}
	}
	return false
}

func projectUTF16Column(line string, byteOffset int) int {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(line) {
		byteOffset = len(line)
	}
	for byteOffset > 0 && byteOffset < len(line) && (line[byteOffset]&0xC0) == 0x80 {
		byteOffset--
	}
	units := 0
	for _, r := range line[:byteOffset] {
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
	}
	return units + 1
}

func boundedProjectSearchPreview(line string, matchStart int) string {
	if len(line) <= projectContentSearchMaxPreviewBytes {
		return line
	}
	if matchStart < 0 {
		matchStart = 0
	}
	if matchStart > len(line) {
		matchStart = len(line)
	}
	start := matchStart - projectContentSearchMaxPreviewBytes/3
	if start < 0 {
		start = 0
	}
	maxStart := len(line) - projectContentSearchMaxPreviewBytes
	if start > maxStart {
		start = maxStart
	}
	for start < len(line) && start > 0 && (line[start]&0xC0) == 0x80 {
		start++
	}
	end := start + projectContentSearchMaxPreviewBytes
	if end > len(line) {
		end = len(line)
	}
	for end > start && end < len(line) && (line[end]&0xC0) == 0x80 {
		end--
	}
	preview := line[start:end]
	if start > 0 {
		preview = "…" + preview
	}
	if end < len(line) {
		preview += "…"
	}
	return preview
}

func projectContentTextBytes(data []byte) bool {
	sample := data
	if len(sample) > projectBinarySample {
		sample = sample[:projectBinarySample]
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return false
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	return utf8.Valid(data)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

