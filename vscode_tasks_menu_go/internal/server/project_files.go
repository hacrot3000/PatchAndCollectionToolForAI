package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	projectEditableLimit = int64(2 << 20)
	projectReadableLimit = int64(10 << 20)
	projectBinarySample  = 8 << 10
)

type projectTreeEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

type projectFileMetadata struct {
	Path    string `json:"path"`
	MtimeNS int64  `json:"mtime_ns"`
	Size    int64  `json:"size"`
}

type projectFileResponse struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	SHA256     string `json:"sha256"`
	MtimeNS    int64  `json:"mtime_ns"`
	Size       int64  `json:"size"`
	Encoding   string `json:"encoding"`
	LineEnding string `json:"line_ending"`
	ReadOnly   bool   `json:"read_only"`
	LargeFile  bool   `json:"large_file,omitempty"`
	BOM        bool   `json:"bom,omitempty"`
	Warning    string `json:"warning,omitempty"`
	HistoryWarning string `json:"history_warning,omitempty"`
}

type projectFileSaveRequest struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	ExpectedSHA256 string `json:"expected_sha256"`
	LineEnding     string `json:"line_ending,omitempty"`
	Encoding       string `json:"encoding,omitempty"`
}


func (s *Server) projectFile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.projectFileRead(w, r)
	case http.MethodPut:
		s.projectFileSave(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}


func (s *Server) projectFileSave(w http.ResponseWriter, r *http.Request) {
	var req projectFileSaveRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectEditableLimit*6+(256<<10)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON or editor payload too large", http.StatusBadRequest)
		return
	}
	rel, err := cleanProjectRelativePath(req.Path, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !utf8.ValidString(req.Content) || strings.ContainsRune(req.Content, '\x00') {
		http.Error(w, "editor content must be valid UTF-8 text without NUL bytes", http.StatusUnsupportedMediaType)
		return
	}
	if int64(len(req.Content)) > projectEditableLimit {
		http.Error(w, "editor content is too large", http.StatusRequestEntityTooLarge)
		return
	}
	req.LineEnding = strings.ToLower(strings.TrimSpace(req.LineEnding))
	switch req.LineEnding {
	case "", "preserve", "lf", "crlf":
	default:
		http.Error(w, "line_ending must be preserve, lf, or crlf", http.StatusBadRequest)
		return
	}
	req.Encoding = strings.ToLower(strings.TrimSpace(req.Encoding))
	switch req.Encoding {
	case "", "preserve", "utf-8", "utf-8-bom":
	default:
		http.Error(w, "encoding must be preserve, utf-8, or utf-8-bom", http.StatusBadRequest)
		return
	}
	if len(req.ExpectedSHA256) != sha256.Size*2 {
		http.Error(w, "expected_sha256 is required", http.StatusBadRequest)
		return
	}
	if _, err := hex.DecodeString(req.ExpectedSHA256); err != nil {
		http.Error(w, "expected_sha256 is invalid", http.StatusBadRequest)
		return
	}
	path, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	rootView, _, err := s.projectRootForVirtualPath(rel)
	if err != nil {
		http.Error(w, "project root unavailable", http.StatusInternalServerError)
		return
	}
	pinned, err := openProjectPinnedFile(rootView.Path, path)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	defer pinned.close()
	current, info, err := pinned.readCurrent(projectEditableLimit)
	if errors.Is(err, errPinnedProjectFileTooLarge) {
		http.Error(w, "project file is read-only in the editor", http.StatusForbidden)
		return
	}
	if err != nil || info == nil || !info.Mode().IsRegular() {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	if info.Mode().Perm()&0o222 == 0 {
		http.Error(w, "project file is read-only", http.StatusForbidden)
		return
	}
	if !projectTextBytesValid(current) {
		http.Error(w, "project file is not editable UTF-8 text", http.StatusUnsupportedMediaType)
		return
	}
	currentHash := sha256.Sum256(current)
	if !strings.EqualFold(hex.EncodeToString(currentHash[:]), req.ExpectedSHA256) {
		http.Error(w, "project file changed outside the editor", http.StatusConflict)
		return
	}

	currentBOM := bytes.HasPrefix(current, []byte{0xEF, 0xBB, 0xBF})
	currentText := current
	if currentBOM {
		currentText = currentText[3:]
	}
	lineEnding, normalized := applyProjectLineEndingMode(req.Content, currentText, req.LineEnding)
	nextBOM := currentBOM
	switch req.Encoding {
	case "utf-8":
		nextBOM = false
	case "utf-8-bom":
		nextBOM = true
	}
	next := []byte(normalized)
	if nextBOM {
		next = append([]byte{0xEF, 0xBB, 0xBF}, next...)
	}
	if int64(len(next)) > projectEditableLimit {
		http.Error(w, "project file is too large to save from the editor", http.StatusRequestEntityTooLarge)
		return
	}

	lease, ok := s.acquireSharedMutation(w, r, "file.write", rel)
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)

	if err := pinned.writeTemp(next, info); err != nil {
		http.Error(w, "cannot prepare editor save file", http.StatusInternalServerError)
		return
	}
	latest, _, err := pinned.readCurrent(projectEditableLimit)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusConflict)
		return
	}
	latestHash := sha256.Sum256(latest)
	if !strings.EqualFold(hex.EncodeToString(latestHash[:]), req.ExpectedSHA256) {
		http.Error(w, "project file changed outside the editor", http.StatusConflict)
		return
	}
	if err := pinned.commitTemp(); err != nil {
		http.Error(w, "cannot finalize editor save", http.StatusInternalServerError)
		return
	}
	historyWarning := ""
	if err := recordProjectFileHistory(s.Workspace, rel, latest); err != nil {
		historyWarning = "Saved file, but local history could not be recorded: " + err.Error()
		if s.Log != nil { s.Log.Printf("editor local history failed for %s: %v", rel, err) }
	}
	_, savedInfo, err := pinned.readCurrent(projectEditableLimit)
	if err != nil || savedInfo == nil {
		http.Error(w, "saved file metadata unavailable", http.StatusInternalServerError)
		return
	}
	savedHash := sha256.Sum256(next)
	s.auditSharedSuccess(r, "file.write", "file", rel, map[string]any{
		"size": savedInfo.Size(),
	})
	text := next
	if nextBOM {
		text = text[3:]
	}
	s.updateProjectSymbolIndexFile(rel, string(text))
	writeJSON(w, http.StatusOK, projectFileResponse{
		Path:       rel,
		Content:    string(text),
		SHA256:     hex.EncodeToString(savedHash[:]),
		MtimeNS:    savedInfo.ModTime().UnixNano(),
		Size:       savedInfo.Size(),
		Encoding:   "utf-8",
		LineEnding: lineEnding,
		ReadOnly:   false,
		BOM:        nextBOM,
		HistoryWarning: historyWarning,
	})
}

func projectTextBytesValid(data []byte) bool {
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

func (s *Server) projectFileRead(w http.ResponseWriter, r *http.Request) {
	rel, err := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("meta") == "1" {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			http.Error(w, "project file unavailable", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, projectFileMetadata{
			Path: rel,
			MtimeNS: info.ModTime().UnixNano(),
			Size: info.Size(),
		})
		return
	}
	rootView, _, err := s.projectRootForVirtualPath(rel)
	if err != nil {
		http.Error(w, "project root unavailable", http.StatusInternalServerError)
		return
	}
	pinned, err := openProjectPinnedFile(rootView.Path, path)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	defer pinned.close()
	data, info, err := pinned.readCurrent(projectReadableLimit)
	if errors.Is(err, errPinnedProjectFileTooLarge) {
		http.Error(w, "project file is too large for the editor", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil || info == nil || !info.Mode().IsRegular() {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	encoding := "utf-8"
	bom := bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	textData := data
	decodedReadOnly := false
	warning := ""
	if projectTextBytesValid(data) {
		if bom {
			textData = textData[3:]
		}
	} else if isMarkdownPreviewPath(rel) {
		decoded, decodeErr := decodeMarkdownText(data)
		if decodeErr != nil {
			http.Error(w, "project Markdown file could not be decoded", http.StatusUnsupportedMediaType)
			return
		}
		textData = []byte(decoded.Text)
		encoding = decoded.Encoding
		bom = decoded.BOM
		decodedReadOnly = encoding != "utf-8"
		if decodedReadOnly {
			warning = "Markdown was decoded from " + encoding + " and is opened read-only to preserve the original encoding."
		}
	} else {
		http.Error(w, "project file is not valid editable UTF-8 text", http.StatusUnsupportedMediaType)
		return
	}
	sum := sha256.Sum256(data)
	largeFile := info.Size() > projectEditableLimit
	readOnly := decodedReadOnly || info.Mode().Perm()&0o222 == 0 || largeFile
	if largeFile {
		if warning != "" {
			warning += " "
		}
		warning += "File is larger than 2 MiB and is read-only by default."
	}
	writeJSON(w, http.StatusOK, projectFileResponse{
		Path:       rel,
		Content:    string(textData),
		SHA256:     hex.EncodeToString(sum[:]),
		MtimeNS:    info.ModTime().UnixNano(),
		Size:       info.Size(),
		Encoding:   encoding,
		LineEnding: detectProjectLineEnding(textData),
		ReadOnly:   readOnly,
		LargeFile:  largeFile,
		BOM:        bom,
		Warning:    warning,
	})
}

func detectProjectLineEnding(data []byte) string {
	label, _, _ := projectLineEndingProfile(data)
	return label
}

func projectLineEndingProfile(data []byte) (label, preferred string, endings []string) {
	lfCount, crlfCount := 0, 0
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		if i > 0 && data[i-1] == '\r' {
			endings = append(endings, "\r\n")
			crlfCount++
		} else {
			endings = append(endings, "\n")
			lfCount++
		}
	}
	switch {
	case crlfCount > 0 && lfCount > 0:
		label = "mixed"
	case crlfCount > 0:
		label = "crlf"
	default:
		label = "lf"
	}
	preferred = "\n"
	if crlfCount > lfCount {
		preferred = "\r\n"
	}
	return label, preferred, endings
}

func applyProjectLineEndingMode(content string, current []byte, requested string) (string, string) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" || requested == "preserve" {
		return applyProjectLineEndings(content, current)
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	switch requested {
	case "crlf":
		return "crlf", strings.ReplaceAll(normalized, "\n", "\r\n")
	default:
		return "lf", normalized
	}
}

func applyProjectLineEndings(content string, current []byte) (string, string) {
	label, preferred, endings := projectLineEndingProfile(current)
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if !strings.Contains(normalized, "\n") {
		return label, normalized
	}
	var out strings.Builder
	out.Grow(len(normalized) + len(endings))
	endingIndex := 0
	start := 0
	for i := 0; i < len(normalized); i++ {
		if normalized[i] != '\n' {
			continue
		}
		out.WriteString(normalized[start:i])
		ending := preferred
		if endingIndex < len(endings) {
			ending = endings[endingIndex]
		}
		out.WriteString(ending)
		endingIndex++
		start = i + 1
	}
	out.WriteString(normalized[start:])
	return label, out.String()
}

func (s *Server) projectTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requested := r.URL.Query().Get("path")
	rootView, _, rootErr := s.projectRootForVirtualPath(requested)
	if rootErr != nil {
		http.Error(w, rootErr.Error(), http.StatusBadRequest)
		return
	}
	dir, err := s.resolveProjectPath(requested, true, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "project directory unavailable", http.StatusNotFound)
		return
	}
	items := make([]projectTreeEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == ".git" {
			continue
		}
		if isTaskDeckSwapName(entry.Name()) {
			continue
		}
		item, ok := s.projectTreeItem(rootView.Path, dir, entry)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type == "dir"
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) projectTreeItem(root, parent string, entry os.DirEntry) (projectTreeEntry, bool) {
	candidate := filepath.Join(parent, entry.Name())
	if entry.Type()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || !pathWithin(root, resolved) {
			return projectTreeEntry{}, false
		}
		candidate = resolved
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return projectTreeEntry{}, false
	}
	modified := info.ModTime().UTC().Format(time.RFC3339)
	switch {
	case info.IsDir():
		return projectTreeEntry{Name: entry.Name(), Type: "dir", Modified: modified}, true
	case info.Mode().IsRegular():
		return projectTreeEntry{Name: entry.Name(), Type: "file", Size: info.Size(), Modified: modified}, true
	default:
		return projectTreeEntry{}, false
	}
}

func (s *Server) projectRoot() (string, error) {
	root, err := filepath.Abs(s.Workspace)
	if err != nil {
		return "", fmt.Errorf("invalid workspace")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("invalid workspace")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid workspace")
	}
	return root, nil
}

func (s *Server) resolveProjectPath(requested string, allowRoot, wantDir bool) (string, error) {
	rootView, rootRelative, err := s.projectRootForVirtualPath(requested)
	if err != nil {
		return "", err
	}
	root := rootView.Path
	rel, err := cleanProjectRelativePath(rootRelative, allowRoot)
	if err != nil {
		return "", err
	}
	candidate := root
	if rel != "" {
		candidate = filepath.Join(root, filepath.FromSlash(rel))
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil || !pathWithin(root, resolved) {
		return "", fmt.Errorf("project path not found")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("project path not found")
	}
	if wantDir && !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory")
	}
	if !wantDir && !info.Mode().IsRegular() {
		return "", fmt.Errorf("project path is not a regular file")
	}
	return resolved, nil
}

func cleanProjectRelativePath(requested string, allowRoot bool) (string, error) {
	requested = strings.TrimSpace(requested)
	if strings.ContainsRune(requested, '\x00') {
		return "", fmt.Errorf("invalid project path")
	}
	if requested == "" || requested == "." {
		if allowRoot {
			return "", nil
		}
		return "", fmt.Errorf("project path is required")
	}
	if projectPathLooksAbsolute(requested) {
		return "", fmt.Errorf("project path must be workspace-relative")
	}
	clean := filepath.Clean(filepath.FromSlash(requested))
	if clean == "." {
		if allowRoot {
			return "", nil
		}
		return "", fmt.Errorf("project path is required")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project path must stay inside workspace")
	}
	if isTaskDeckSwapName(filepath.Base(clean)) {
		return "", fmt.Errorf("TaskDeck editor swap paths are internal")
	}
	return filepath.ToSlash(clean), nil
}

func projectPathLooksAbsolute(value string) bool {
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") {
		return true
	}
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	return false
}
