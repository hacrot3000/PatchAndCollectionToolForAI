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
	Path string `json:"path"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *Server) filesSelection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	files := s.downloadableFilesFromText(req.Text)
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
	text = stripTerminalControlSequences(text)
	if len(text) > 128<<10 {
		// Automatic artifacts are emitted at the end of task output. Keep the
		// newest window so long-running Patch Tool sessions cannot hide the
		// ACTION REQUIRED result behind earlier console noise.
		text = text[len(text)-(128<<10):]
	}
	text = stripGitShellOutput(text)
	seen := make(map[string]struct{})
	var paths []string
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
			resolved, err := s.resolveDownloadPath(candidate)
			if err != nil {
				continue
			}
			if _, ok := seen[resolved]; ok {
				continue
			}
			seen[resolved] = struct{}{}
			paths = append(paths, resolved)
		}
	}
	files := make([]downloadableFile, 0, len(paths))
	for _, path := range paths {
		files = append(files, downloadableFile{
			Path: path,
			Name: filepath.Base(path),
			URL:  "/api/files/download?path=" + url.QueryEscape(path),
		})
	}
	return files
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
	if value == "" || strings.ContainsRune(value, '\x00') {
		return false
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return true
	}
	return strings.Contains(value, "/") && !strings.Contains(value, "://")
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
