package server

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type downloadableFile struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	PreviewKind string `json:"preview_kind,omitempty"`
}

func (s *Server) filesSelection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text    string `json:"text"`
		Context string `json:"context,omitempty"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	files := s.downloadableFilesFromSelection(req.Text, req.Context)
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) fileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requested := strings.TrimSpace(r.URL.Query().Get("path"))
	resolved, err := s.resolveDownloadPath(requested)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	f, err := os.Open(resolved)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	name := filepath.Base(resolved)
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disposition == "" {
		disposition = `attachment; filename="download"`
	}
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

func (s *Server) downloadableFilesFromText(text string) []downloadableFile {
	return s.downloadableFilesFromSelection(text, "")
}

func (s *Server) downloadableFilesFromSelection(text, context string) []downloadableFile {
	text = normalizeDownloadDetectionText(text)
	context = normalizeDownloadDetectionText(context)

	seen := make(map[string]struct{})
	var paths []string
	addResolved := func(candidate string) bool {
		candidate = trimPathCandidate(candidate)
		if !looksLikePath(candidate) {
			return false
		}
		resolved, err := s.resolveDownloadPath(candidate)
		if err != nil {
			return false
		}
		if _, ok := seen[resolved]; ok {
			return true
		}
		seen[resolved] = struct{}{}
		paths = append(paths, resolved)
		return true
	}

	unresolved := make([]string, 0)
	unresolvedSeen := make(map[string]struct{})
	for _, candidate := range downloadPathCandidates(text) {
		if addResolved(candidate) {
			continue
		}
		if _, ok := unresolvedSeen[candidate]; !ok {
			unresolvedSeen[candidate] = struct{}{}
			unresolved = append(unresolved, candidate)
		}
	}

	// The selected text has priority. Only candidates that did not resolve
	// exactly are allowed to expand to their full terminal line context.
	if context != "" {
		contextLines := strings.Split(strings.ReplaceAll(context, "\r", ""), "\n")
		for _, selected := range unresolved {
			for _, rawLine := range contextLines {
				line := strings.TrimSpace(rawLine)
				if line == "" || ignoreDownloadDetectionLine(line) {
					continue
				}
				found := false
				for _, expanded := range expandedDownloadPathCandidates(selected, line) {
					if addResolved(expanded) {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
	}

	files := make([]downloadableFile, 0, len(paths))
	for _, path := range paths {
		files = append(files, downloadableFile{
			Path:        path,
			Name:        filepath.Base(path),
			URL:         "/api/files/download?path=" + url.QueryEscape(path),
			PreviewKind: previewFileHint(path),
		})
	}
	return files
}

func normalizeDownloadDetectionText(text string) string {
	text = stripTerminalControlSequences(text)
	if len(text) > 128<<10 {
		text = text[len(text)-(128<<10):]
	}
	return stripGitShellOutput(text)
}

func downloadPathCandidates(text string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, rawLine := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || ignoreDownloadDetectionLine(line) {
			continue
		}
		candidates := []string{trimPathCandidate(line)}
		for _, field := range strings.Fields(line) {
			candidates = append(candidates, trimPathCandidate(field))
		}
		for _, candidate := range candidates {
			if !looksLikePath(candidate) {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	return out
}

func expandedDownloadPathCandidates(selected, line string) []string {
	selected = trimPathCandidate(selected)
	if selected == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, field := range strings.Fields(line) {
		token := trimPathCandidate(field)
		if token == "" || token == selected {
			continue
		}
		searchFrom := 0
		for {
			rel := strings.Index(token[searchFrom:], selected)
			if rel < 0 {
				break
			}
			idx := searchFrom + rel
			prefix := token[:idx]
			suffix := token[idx+len(selected):]
			// Expansion is intentionally left-biased. A selected suffix such as
			// "4/a5/file.txt" grows to "a4/a5/file.txt", then
			// "a3/a4/a5/file.txt", and so on. Do not guess text to the right.
			if suffix == "" {
				for _, expanded := range leftExpandedPathSuffixes(prefix, selected) {
					if _, ok := seen[expanded]; ok {
						continue
					}
					seen[expanded] = struct{}{}
					out = append(out, expanded)
				}
			}
			searchFrom = idx + 1
			if searchFrom >= len(token) {
				break
			}
		}
	}
	return out
}

func leftExpandedPathSuffixes(prefix, selected string) []string {
	if prefix == "" {
		return nil
	}
	normalized := filepath.ToSlash(prefix)
	absolute := strings.HasPrefix(normalized, "/")
	trailingSlash := strings.HasSuffix(normalized, "/")
	parts := strings.Split(strings.Trim(normalized, "/"), "/")
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	current := selected
	i := len(parts) - 1
	if !trailingSlash && i >= 0 && parts[i] != "" {
		// Selection may start in the middle of a path segment, e.g. selecting
		// "4/a5/file.txt" from "a4/a5/file.txt". Reattach the missing "a"
		// without inserting a slash before walking parent segments.
		current = parts[i] + current
		candidate := current
		if absolute && i == 0 {
			candidate = "/" + candidate
		}
		out = append(out, candidate)
		i--
	}
	for ; i >= 0; i-- {
		if parts[i] == "" {
			continue
		}
		current = parts[i] + "/" + current
		candidate := current
		if absolute && i == 0 {
			candidate = "/" + candidate
		}
		out = append(out, candidate)
	}
	return out
}

// stripGitShellOutput removes output belonging to an interactive Git command
// before generic path detection runs. The browser suppresses Git output live;
// this is the reload/reconnect fallback for scrollback that is scanned again.
// A new non-Git prompt ends suppression, while compound commands are left
// untouched so `git status && ./collector` cannot hide collector artifacts.
func stripGitShellOutput(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	out := make([]string, 0, len(lines))
	suppress := false
	for _, line := range lines {
		if command, ok := shellPromptCommand(line); ok {
			if standaloneGitShellCommand(command) {
				suppress = true
				continue
			}
			if suppress {
				suppress = false
				// Never feed the typed prompt command itself to file detection.
				continue
			}
		}
		if suppress {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func shellPromptCommand(line string) (string, bool) {
	markers := []string{"$ ", "# ", "% ", "> ", "❯ ", "➜ "}
	bestIndex, bestLen := -1, 0
	for _, marker := range markers {
		if idx := strings.LastIndex(line, marker); idx > bestIndex {
			bestIndex, bestLen = idx, len(marker)
		}
	}
	if bestIndex < 0 {
		return "", false
	}
	command := strings.TrimSpace(line[bestIndex+bestLen:])
	return command, command != ""
}

func standaloneGitShellCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.ContainsAny(command, ";|") {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	i := 0
	for i < len(fields) && shellAssignmentField(fields[i]) {
		i++
	}
	if i < len(fields) && (fields[i] == "command" || fields[i] == "builtin") {
		i++
	}
	if i < len(fields) && fields[i] == "sudo" {
		i++
		for i < len(fields) && strings.HasPrefix(fields[i], "-") {
			i++
		}
	}
	if i < len(fields) && fields[i] == "env" {
		i++
		for i < len(fields) && (strings.HasPrefix(fields[i], "-") || shellAssignmentField(fields[i])) {
			i++
		}
	}
	if i >= len(fields) {
		return false
	}
	name := strings.Trim(fields[i], "\"'")
	return name == "git" || filepath.Base(name) == "git"
}

func shellAssignmentField(value string) bool {
	if idx := strings.IndexByte(value, '='); idx > 0 {
		name := value[:idx]
		for i, r := range name {
			if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
				return false
			}
		}
		return true
	}
	return false
}

func ignoreDownloadDetectionLine(line string) bool {
	upper := strings.ToUpper(strings.TrimSpace(line))
	if strings.HasPrefix(upper, "LỆNH") || strings.HasPrefix(upper, "COMMAND") {
		return true
	}
	if strings.Contains(upper, "SKIPPED NON-PATCH CANDIDATE:") {
		return true
	}
	if strings.Contains(upper, "REQUEST ARCHIVED:") {
		return true
	}
	return false
}

func trimPathCandidate(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`<>[](){}")
	value = strings.TrimRight(value, ",;:")
	return strings.TrimSpace(value)
}

func looksLikePath(value string) bool {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.ContainsAny(value, "\r\n\t ") {
		return false
	}
	if strings.Contains(value, "://") {
		return false
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.Contains(value, "/") {
		return true
	}
	// Root-level files have no slash. Require a filename-like extension so
	// ordinary selected prose is not mistaken for a workspace file.
	base := filepath.Base(value)
	dot := strings.LastIndexByte(base, '.')
	return dot > 0 && dot < len(base)-1
}

func (s *Server) resolveDownloadPath(requested string) (string, error) {
	if requested == "" || strings.ContainsRune(requested, '\x00') {
		return "", fmt.Errorf("file not found")
	}
	workspace, err := filepath.Abs(s.Workspace)
	if err != nil {
		return "", fmt.Errorf("file not found")
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("file not found")
	}
	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspace, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("file not found")
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("file not found")
	}
	if !pathWithin(workspace, candidate) {
		return "", fmt.Errorf("file not found")
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("file not found")
	}
	return candidate, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
