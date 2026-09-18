package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	projectEditableLimit = int64(2 << 20)
	projectReadableLimit = int64(10 << 20)
	projectBinarySample  = 8 << 10
)

type projectTreeEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
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
	BOM        bool   `json:"bom,omitempty"`
	Warning    string `json:"warning,omitempty"`
}


func (s *Server) projectFile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.projectFileRead(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	if info.Size() > projectReadableLimit {
		http.Error(w, "project file is too large for the editor", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	sample := data
	if len(sample) > projectBinarySample {
		sample = sample[:projectBinarySample]
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		http.Error(w, "binary files are not editable as text", http.StatusUnsupportedMediaType)
		return
	}
	bom := bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	textData := data
	if bom {
		textData = textData[3:]
	}
	if !utf8.Valid(textData) {
		http.Error(w, "project file is not valid UTF-8", http.StatusUnsupportedMediaType)
		return
	}
	sum := sha256.Sum256(data)
	readOnly := info.Mode().Perm()&0o222 == 0 || info.Size() > projectEditableLimit
	warning := ""
	if info.Size() > projectEditableLimit {
		warning = "File is larger than 2 MiB and is read-only by default."
	}
	writeJSON(w, http.StatusOK, projectFileResponse{
		Path:       rel,
		Content:    string(textData),
		SHA256:     hex.EncodeToString(sum[:]),
		MtimeNS:    info.ModTime().UnixNano(),
		Size:       info.Size(),
		Encoding:   "utf-8",
		LineEnding: detectProjectLineEnding(textData),
		ReadOnly:   readOnly,
		BOM:        bom,
		Warning:    warning,
	})
}

func detectProjectLineEnding(data []byte) string {
	if bytes.Contains(data, []byte("\r\n")) {
		return "crlf"
	}
	return "lf"
}

func (s *Server) projectTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dir, err := s.resolveProjectPath(r.URL.Query().Get("path"), true, true)
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
		item, ok := s.projectTreeItem(dir, entry)
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

func (s *Server) projectTreeItem(parent string, entry os.DirEntry) (projectTreeEntry, bool) {
	candidate := filepath.Join(parent, entry.Name())
	if entry.Type()&os.ModeSymlink != 0 {
		root, err := s.projectRoot()
		if err != nil {
			return projectTreeEntry{}, false
		}
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
	switch {
	case info.IsDir():
		return projectTreeEntry{Name: entry.Name(), Type: "dir"}, true
	case info.Mode().IsRegular():
		return projectTreeEntry{Name: entry.Name(), Type: "file", Size: info.Size()}, true
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
	root, err := s.projectRoot()
	if err != nil {
		return "", err
	}
	rel, err := cleanProjectRelativePath(requested, allowRoot)
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
