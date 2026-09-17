package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxWorkspaceUpload = int64(256 << 20)

func (s *Server) fileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkspaceUpload+(2<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "invalid multipart upload", http.StatusBadRequest)
		return
	}
	src, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer src.Close()

	dir, err := s.resolveUploadDir(r.FormValue("dir"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := cleanUploadName(header.Filename)
	if name == "" {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	target := filepath.Join(dir, name)
	overwrite := r.URL.Query().Get("overwrite") == "1"
	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			http.Error(w, "target is not a regular file", http.StatusConflict)
			return
		}
		if !overwrite {
			http.Error(w, "file already exists", http.StatusConflict)
			return
		}
	} else if !os.IsNotExist(statErr) {
		http.Error(w, "target unavailable", http.StatusConflict)
		return
	}

	tmp, err := os.CreateTemp(dir, ".task-menu-upload-*")
	if err != nil {
		http.Error(w, "cannot create upload file", http.StatusInternalServerError)
		return
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	limited := io.LimitReader(src, maxWorkspaceUpload+1)
	n, copyErr := io.Copy(tmp, limited)
	if copyErr != nil || n > maxWorkspaceUpload {
		cleanup()
		if n > maxWorkspaceUpload {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "upload failed", http.StatusInternalServerError)
		}
		return
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		http.Error(w, "upload sync failed", http.StatusInternalServerError)
		return
	}
	if err := tmp.Chmod(0o644); err != nil {
		cleanup()
		http.Error(w, "upload chmod failed", http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "upload close failed", http.StatusInternalServerError)
		return
	}
	if !overwrite {
		if _, err := os.Lstat(target); err == nil {
			_ = os.Remove(tmpName)
			http.Error(w, "file already exists", http.StatusConflict)
			return
		}
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "cannot finalize upload", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, downloadableFile{
		Path: target,
		Name: name,
		URL:  "/api/files/download?path=" + url.QueryEscape(target),
	})
}

func cleanUploadName(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	name := filepath.Base(value)
	if name == "." || name == string(filepath.Separator) || name == "" || strings.ContainsRune(name, '\x00') {
		return ""
	}
	return name
}

func (s *Server) resolveUploadDir(requested string) (string, error) {
	workspace, err := filepath.Abs(s.Workspace)
	if err != nil {
		return "", fmt.Errorf("invalid workspace")
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("invalid workspace")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "."
	}
	if filepath.IsAbs(requested) || strings.ContainsRune(requested, '\x00') {
		return "", fmt.Errorf("upload directory must be inside workspace")
	}
	clean := filepath.Clean(requested)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("upload directory must be inside workspace")
	}
	candidate := filepath.Join(workspace, clean)
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil || !pathWithin(workspace, candidate) {
		return "", fmt.Errorf("upload directory not found")
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("upload directory not found")
	}
	return candidate, nil
}
