package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type projectTreeEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
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
