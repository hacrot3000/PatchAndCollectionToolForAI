package server

import (
	"io/fs"
	"net/http"
	"path/filepath"
)

const projectHealthMaxEntries = 200000

func (s *Server) projectHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := s.projectRoot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var bytes int64
	var files, dirs, entries int
	truncated := false
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path != root && entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}
		entries++
		if entries > projectHealthMaxEntries {
			truncated = true
			return fs.SkipAll
		}
		if entry.IsDir() {
			dirs++
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	if err != nil && err != fs.SkipAll {
		http.Error(w, "project health scan failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"disk_usage_bytes": bytes,
		"file_count": files,
		"directory_count": dirs,
		"entry_count": entries,
		"truncated": truncated,
		"max_entries": projectHealthMaxEntries,
	})
}
