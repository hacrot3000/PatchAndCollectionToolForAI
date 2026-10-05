package server

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/state"
)

const (
	projectReplacePlanMaxBytes = int64(32 << 20)
	projectReplaceMaxFiles     = 1000
	projectReplaceMaxMatches   = 10000
	projectReplacePlanTTL      = 24 * time.Hour
)

type projectReplaceRequest struct {
	Action        string `json:"action"`
	Token         string `json:"token,omitempty"`
	Query         string `json:"query,omitempty"`
	Replacement   string `json:"replacement,omitempty"`
	Regex         bool   `json:"regex,omitempty"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	WholeWord     bool   `json:"whole_word,omitempty"`
	Include       string `json:"include,omitempty"`
	Exclude       string `json:"exclude,omitempty"`
	ScopePath     string `json:"scope_path,omitempty"`
	Mode          string `json:"mode,omitempty"`
	TargetPath    string `json:"target_path,omitempty"`
	Line          int    `json:"line,omitempty"`
	Column        int    `json:"column,omitempty"`
}

type projectReplacePreviewFile struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
	Diff         string `json:"diff"`
}

type projectReplacePlanFile struct {
	Path         string `json:"path"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
	Before       []byte `json:"before"`
	After        []byte `json:"after"`
	Replacements int    `json:"replacements"`
}

type projectReplacePlan struct {
	Token             string                   `json:"token"`
	CreatedAt         string                   `json:"created_at"`
	Applied           bool                     `json:"applied"`
	Undone            bool                     `json:"undone"`
	TotalReplacements int                      `json:"total_replacements"`
	Files             []projectReplacePlanFile `json:"files"`
}

func (s *Server) projectContentReplace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectReplaceRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid replace request", http.StatusBadRequest)
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	switch req.Action {
	case "preview":
		s.projectReplacePreview(w, r, req)
	case "apply":
		s.projectReplaceApply(w, r, req.Token)
	case "undo":
		s.projectReplaceUndo(w, r, req.Token)
	default:
		http.Error(w, "unsupported replace action", http.StatusBadRequest)
	}
}

func (s *Server) projectReplacePreview(w http.ResponseWriter, r *http.Request, req projectReplaceRequest) {
	options := projectContentSearchOptions{
		Query:         req.Query,
		Regex:         req.Regex,
		CaseSensitive: req.CaseSensitive,
		WholeWord:     req.WholeWord,
		Include:       splitProjectSearchGlobs(req.Include),
		Exclude:       splitProjectSearchGlobs(req.Exclude),
	}
	if strings.TrimSpace(options.Query) == "" {
		http.Error(w, "replace query is required", http.StatusBadRequest)
		return
	}
	matcher, err := projectSearchRegexp(options)
	if err != nil {
		http.Error(w, "invalid regular expression", http.StatusBadRequest)
		return
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "file" && mode != "match" {
		http.Error(w, "replace mode must be all, file, or match", http.StatusBadRequest)
		return
	}
	if mode != "all" {
		target, cleanErr := cleanProjectRelativePath(req.TargetPath, false)
		if cleanErr != nil {
			http.Error(w, cleanErr.Error(), http.StatusBadRequest)
			return
		}
		req.TargetPath = target
	}
	if mode == "match" && (req.Line < 1 || req.Column < 1) {
		http.Error(w, "replace match requires line and column", http.StatusBadRequest)
		return
	}
	if scope := strings.TrimSpace(req.ScopePath); scope != "" && scope != "." {
		rel, cleanErr := cleanProjectRelativePath(scope, true)
		if cleanErr != nil {
			http.Error(w, cleanErr.Error(), http.StatusBadRequest)
			return
		}
		if _, resolveErr := s.resolveProjectPath(rel, true, true); resolveErr != nil {
			http.Error(w, "project replace folder unavailable", http.StatusNotFound)
			return
		}
		options.Scope = rel
	}
	root, err := s.projectRoot()
	if err != nil {
		http.Error(w, "project root unavailable", http.StatusInternalServerError)
		return
	}
	plan, previews, err := buildProjectReplacePlan(root, matcher, options, req, mode)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, errProjectReplaceLimit) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}
	if len(plan.Files) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"token": "", "files": []projectReplacePreviewFile{}, "total_replacements": 0,
		})
		return
	}
	token, err := s.saveProjectReplacePlan(&plan)
	if err != nil {
		http.Error(w, "cannot save replace preview", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token, "files": previews, "total_replacements": plan.TotalReplacements,
	})
}

var errProjectReplaceLimit = errors.New("replace preview exceeds safety limit")

func buildProjectReplacePlan(root string, matcher *regexp.Regexp, options projectContentSearchOptions, req projectReplaceRequest, mode string) (projectReplacePlan, []projectReplacePreviewFile, error) {
	plan := projectReplacePlan{CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	previews := []projectReplacePreviewFile{}
	ignore := loadProjectRootIgnore(root)
	walkRoot := root
	if options.Scope != "" {
		walkRoot = filepath.Join(root, filepath.FromSlash(options.Scope))
	}
	var totalBytes int64
	err := filepath.WalkDir(walkRoot, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if full == walkRoot && entry.IsDir() {
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
		if mode != "all" && rel != req.TargetPath {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > projectEditableLimit || info.Mode().Perm()&0o222 == 0 {
			return nil
		}
		data, err := os.ReadFile(full)
		if err != nil || !projectTextBytesValid(data) {
			return nil
		}
		bom := bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		textBytes := data
		if bom {
			textBytes = textBytes[3:]
		}
		nextText, replacements := replaceProjectText(string(textBytes), matcher, req.Replacement, req.Regex, mode, req.Line, req.Column)
		if replacements == 0 {
			return nil
		}
		next := []byte(nextText)
		if bom {
			next = append([]byte{0xEF, 0xBB, 0xBF}, next...)
		}
		if int64(len(next)) > projectEditableLimit {
			return fmt.Errorf("%w: replacement makes %s too large", errProjectReplaceLimit, rel)
		}
		totalBytes += int64(len(data)) + int64(len(next))
		if totalBytes > projectReplacePlanMaxBytes || len(plan.Files) >= projectReplaceMaxFiles || plan.TotalReplacements+replacements > projectReplaceMaxMatches {
			return errProjectReplaceLimit
		}
		beforeHash := sha256.Sum256(data)
		afterHash := sha256.Sum256(next)
		plan.Files = append(plan.Files, projectReplacePlanFile{
			Path: rel, BeforeSHA256: hex.EncodeToString(beforeHash[:]), AfterSHA256: hex.EncodeToString(afterHash[:]),
			Before: data, After: next, Replacements: replacements,
		})
		plan.TotalReplacements += replacements
		previews = append(previews, projectReplacePreviewFile{
			Path: rel, Replacements: replacements, Diff: projectReplaceDiffPreview(rel, string(textBytes), nextText),
		})
		return nil
	})
	if err != nil {
		return projectReplacePlan{}, nil, err
	}
	return plan, previews, nil
}

func replaceProjectText(text string, matcher *regexp.Regexp, replacement string, regexMode bool, mode string, targetLine, targetColumn int) (string, int) {
	matches := matcher.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, 0
	}
	var out strings.Builder
	out.Grow(len(text))
	last := 0
	replacements := 0
	for _, loc := range matches {
		replace := mode != "match"
		if mode == "match" {
			line, column := projectReplaceLineColumn(text, loc[0])
			replace = line == targetLine && column == targetColumn && replacements == 0
		}
		if !replace {
			continue
		}
		out.WriteString(text[last:loc[0]])
		if regexMode {
			out.Write(matcher.ExpandString(nil, replacement, text, loc))
		} else {
			out.WriteString(replacement)
		}
		last = loc[1]
		replacements++
		if mode == "match" {
			break
		}
	}
	if replacements == 0 {
		return text, 0
	}
	out.WriteString(text[last:])
	return out.String(), replacements
}

func projectReplaceLineColumn(text string, byteOffset int) (int, int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(text) {
		byteOffset = len(text)
	}
	line := 1 + strings.Count(text[:byteOffset], "\n")
	lineStart := strings.LastIndex(text[:byteOffset], "\n")
	if lineStart < 0 {
		lineStart = 0
	} else {
		lineStart++
	}
	return line, projectUTF16Column(text[lineStart:], byteOffset-lineStart)
}

func projectReplaceDiffPreview(pathValue, before, after string) string {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	first := 0
	for first < len(beforeLines) && first < len(afterLines) && beforeLines[first] == afterLines[first] {
		first++
	}
	lastBefore, lastAfter := len(beforeLines)-1, len(afterLines)-1
	for lastBefore >= first && lastAfter >= first && beforeLines[lastBefore] == afterLines[lastAfter] {
		lastBefore--
		lastAfter--
	}
	start := first - 2
	if start < 0 {
		start = 0
	}
	endBefore := lastBefore + 3
	if endBefore > len(beforeLines) {
		endBefore = len(beforeLines)
	}
	endAfter := lastAfter + 3
	if endAfter > len(afterLines) {
		endAfter = len(afterLines)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n@@ line %d @@\n", pathValue, pathValue, first+1)
	appendLines := func(prefix string, lines []string, from, to int) {
		limit := to
		if limit-from > 24 {
			limit = from + 12
		}
		for i := from; i < limit; i++ {
			fmt.Fprintf(&out, "%s%s\n", prefix, lines[i])
		}
		if limit < to {
			out.WriteString(prefix + "…\n")
			for i := maxInt(limit, to-8); i < to; i++ {
				fmt.Fprintf(&out, "%s%s\n", prefix, lines[i])
			}
		}
	}
	appendLines("-", beforeLines, start, endBefore)
	appendLines("+", afterLines, start, endAfter)
	result := out.String()
	if len(result) > 16<<10 {
		result = result[:16<<10] + "\n…"
	}
	return result
}

func (s *Server) projectReplacePlanDir() (string, error) {
	dir := filepath.Join(state.Dir(s.Workspace), "project-replace")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func validProjectReplaceToken(token string) bool {
	if len(token) != 32 {
		return false
	}
	raw, err := hex.DecodeString(token)
	return err == nil && len(raw) == 16
}

func (s *Server) saveProjectReplacePlan(plan *projectReplacePlan) (string, error) {
	dir, err := s.projectReplacePlanDir()
	if err != nil {
		return "", err
	}
	if plan.Token == "" {
		raw := make([]byte, 16)
		if _, err := cryptorand.Read(raw); err != nil {
			return "", err
		}
		plan.Token = hex.EncodeToString(raw)
	}
	if !validProjectReplaceToken(plan.Token) {
		return "", fmt.Errorf("invalid replace token")
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > projectReplacePlanMaxBytes*2+(1<<20) {
		return "", errProjectReplaceLimit
	}
	path := filepath.Join(dir, plan.Token+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	s.cleanupProjectReplacePlans(dir)
	return plan.Token, nil
}

func (s *Server) loadProjectReplacePlan(token string) (projectReplacePlan, error) {
	if !validProjectReplaceToken(strings.TrimSpace(token)) {
		return projectReplacePlan{}, fmt.Errorf("invalid replace token")
	}
	dir, err := s.projectReplacePlanDir()
	if err != nil {
		return projectReplacePlan{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, token+".json"))
	if err != nil {
		return projectReplacePlan{}, fmt.Errorf("replace preview not found")
	}
	var plan projectReplacePlan
	if err := json.Unmarshal(data, &plan); err != nil || plan.Token != token {
		return projectReplacePlan{}, fmt.Errorf("invalid replace preview")
	}
	return plan, nil
}

func (s *Server) cleanupProjectReplacePlans(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-projectReplacePlanTTL)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func (s *Server) projectReplaceApply(w http.ResponseWriter, r *http.Request, token string) {
	plan, err := s.loadProjectReplacePlan(strings.TrimSpace(token))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if plan.Applied && !plan.Undone {
		http.Error(w, "replace preview was already applied", http.StatusConflict)
		return
	}
	if plan.Undone {
		http.Error(w, "replace preview was already undone", http.StatusConflict)
		return
	}
	lease, ok := s.acquireSharedMutation(w, r, "file.replace", "project")
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)
	if err := s.preflightProjectReplace(plan.Files, false); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	done := make([]projectReplacePlanFile, 0, len(plan.Files))
	for _, file := range plan.Files {
		if err := s.writeProjectReplaceCAS(file.Path, file.BeforeSHA256, file.After); err != nil {
			for i := len(done) - 1; i >= 0; i-- {
				_ = s.writeProjectReplaceCAS(done[i].Path, done[i].AfterSHA256, done[i].Before)
			}
			http.Error(w, "replace apply failed; completed files were rolled back", http.StatusConflict)
			return
		}
		done = append(done, file)
	}
	plan.Applied = true
	if _, err := s.saveProjectReplacePlan(&plan); err != nil {
		http.Error(w, "replace applied but undo metadata could not be updated", http.StatusInternalServerError)
		return
	}
	s.auditSharedSuccess(r, "file.replace", "project", "project", map[string]any{
		"files": len(plan.Files), "replacements": plan.TotalReplacements,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "token": plan.Token, "files": projectReplacePlanPaths(plan.Files), "total_replacements": plan.TotalReplacements,
	})
}

func (s *Server) projectReplaceUndo(w http.ResponseWriter, r *http.Request, token string) {
	plan, err := s.loadProjectReplacePlan(strings.TrimSpace(token))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if !plan.Applied || plan.Undone {
		http.Error(w, "replace plan is not undoable", http.StatusConflict)
		return
	}
	lease, ok := s.acquireSharedMutation(w, r, "file.replace.undo", "project")
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)
	if err := s.preflightProjectReplace(plan.Files, true); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	done := make([]projectReplacePlanFile, 0, len(plan.Files))
	for i := len(plan.Files) - 1; i >= 0; i-- {
		file := plan.Files[i]
		if err := s.writeProjectReplaceCAS(file.Path, file.AfterSHA256, file.Before); err != nil {
			for j := len(done) - 1; j >= 0; j-- {
				_ = s.writeProjectReplaceCAS(done[j].Path, done[j].BeforeSHA256, done[j].After)
			}
			http.Error(w, "replace undo failed; completed restores were rolled forward", http.StatusConflict)
			return
		}
		done = append(done, file)
	}
	plan.Undone = true
	if _, err := s.saveProjectReplacePlan(&plan); err != nil {
		http.Error(w, "replace undo completed but metadata could not be updated", http.StatusInternalServerError)
		return
	}
	s.auditSharedSuccess(r, "file.replace.undo", "project", "project", map[string]any{
		"files": len(plan.Files), "replacements": plan.TotalReplacements,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "token": plan.Token, "files": projectReplacePlanPaths(plan.Files), "total_replacements": plan.TotalReplacements,
	})
}

func (s *Server) preflightProjectReplace(files []projectReplacePlanFile, undo bool) error {
	root, err := s.projectRoot()
	if err != nil {
		return err
	}
	for _, file := range files {
		pathValue, err := s.resolveProjectPath(file.Path, false, false)
		if err != nil {
			return fmt.Errorf("%s is unavailable", file.Path)
		}
		pinned, err := openProjectPinnedFile(root, pathValue)
		if err != nil {
			return fmt.Errorf("%s is unavailable", file.Path)
		}
		current, info, readErr := pinned.readCurrent(projectEditableLimit)
		pinned.close()
		if readErr != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 == 0 {
			return fmt.Errorf("%s is no longer editable", file.Path)
		}
		sum := sha256.Sum256(current)
		expected := file.BeforeSHA256
		if undo {
			expected = file.AfterSHA256
		}
		if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
			return fmt.Errorf("%s changed after replace preview", file.Path)
		}
	}
	return nil
}

func (s *Server) writeProjectReplaceCAS(rel, expected string, next []byte) error {
	if int64(len(next)) > projectEditableLimit || !projectTextBytesValid(next) {
		return fmt.Errorf("replacement content is not editable")
	}
	root, err := s.projectRoot()
	if err != nil {
		return err
	}
	pathValue, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		return err
	}
	pinned, err := openProjectPinnedFile(root, pathValue)
	if err != nil {
		return err
	}
	defer pinned.close()
	current, info, err := pinned.readCurrent(projectEditableLimit)
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 == 0 {
		return fmt.Errorf("project file unavailable")
	}
	sum := sha256.Sum256(current)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		return fmt.Errorf("project file changed")
	}
	if err := pinned.writeTemp(next, info); err != nil {
		return err
	}
	latest, _, err := pinned.readCurrent(projectEditableLimit)
	if err != nil {
		return err
	}
	latestSum := sha256.Sum256(latest)
	if !strings.EqualFold(hex.EncodeToString(latestSum[:]), expected) {
		return fmt.Errorf("project file changed")
	}
	return pinned.commitTemp()
}

func projectReplacePlanPaths(files []projectReplacePlanFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}
