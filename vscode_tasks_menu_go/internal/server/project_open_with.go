package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func (s *Server) projectPreviewInfo(virtual string) (filePreviewResponse, string, error) {
	rel, err := cleanProjectRelativePath(virtual, false)
	if err != nil {
		return filePreviewResponse{}, "", err
	}
	resolved, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("project file unavailable")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("project file unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return filePreviewResponse{}, "", fmt.Errorf("project file unavailable")
	}
	sample := make([]byte, filePreviewSniffBytes)
	n, readErr := file.Read(sample)
	if readErr != nil && readErr != io.EOF {
		return filePreviewResponse{}, "", fmt.Errorf("project file unavailable")
	}
	sample = sample[:n]
	name := filepath.Base(resolved)
	if imageType := detectPreviewImageType(sample); imageType != "" {
		if info.Size() > filePreviewImageLimit {
			return filePreviewResponse{}, "", fmt.Errorf("image is too large to preview (limit 50 MiB)")
		}
		return filePreviewResponse{
			Kind: "image", Path: rel, ProjectPath: rel, Name: name, Size: info.Size(), ContentType: imageType,
			URL: "/api/project/preview?mode=content&path=" + url.QueryEscape(rel),
		}, resolved, nil
	}
	if info.Size() > filePreviewTextLimit {
		return filePreviewResponse{Kind: "binary", Path: rel, ProjectPath: rel, Name: name, Size: info.Size()}, resolved, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("project file unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(file, filePreviewTextLimit+1))
	if err != nil || int64(len(data)) > filePreviewTextLimit {
		return filePreviewResponse{}, "", fmt.Errorf("project file unavailable")
	}
	if isMarkdownPreviewPath(rel) {
		decoded, err := decodeMarkdownText(data)
		if err != nil {
			return filePreviewResponse{}, "", fmt.Errorf("Markdown file could not be decoded: %w", err)
		}
		return filePreviewResponse{
			Kind: "markdown", Path: rel, ProjectPath: rel, Name: name, Size: info.Size(),
			ContentType: "text/markdown; charset=utf-8", Encoding: decoded.Encoding, Content: decoded.Text,
		}, resolved, nil
	}
	if previewTextBytesValid(data) {
		return filePreviewResponse{
			Kind: "text", Path: rel, ProjectPath: rel, Name: name, Size: info.Size(),
			ContentType: "text/plain; charset=utf-8", Encoding: "utf-8",
		}, resolved, nil
	}
	return filePreviewResponse{Kind: "binary", Path: rel, ProjectPath: rel, Name: name, Size: info.Size()}, resolved, nil
}

func (s *Server) projectPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	virtual := strings.TrimSpace(r.URL.Query().Get("path"))
	info, resolved, err := s.projectPreviewInfo(virtual)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("mode") != "content" {
		writeJSON(w, http.StatusOK, info)
		return
	}
	if info.Kind != "image" {
		http.Error(w, "inline preview content is only available for images", http.StatusUnsupportedMediaType)
		return
	}
	file, err := os.Open(resolved)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != info.Size {
		http.Error(w, "project file changed; select it again", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name, stat.ModTime(), file)
}

func (s *Server) projectDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel, err := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resolved, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	file, err := os.Open(resolved)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(resolved)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(resolved), info.ModTime(), file)
}

func hostApplicationCommand(path string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "linux":
		executable, err := exec.LookPath("xdg-open")
		if err != nil {
			return nil, fmt.Errorf("xdg-open not found")
		}
		return exec.Command(executable, path), nil
	case "darwin":
		executable, err := exec.LookPath("open")
		if err != nil {
			return nil, fmt.Errorf("open command not found")
		}
		return exec.Command(executable, path), nil
	case "windows":
		executable, err := exec.LookPath("rundll32.exe")
		if err != nil {
			return nil, fmt.Errorf("rundll32.exe not found")
		}
		return exec.Command(executable, "url.dll,FileProtocolHandler", path), nil
	default:
		return nil, fmt.Errorf("host application open is unsupported on %s", runtime.GOOS)
	}
}

func (s *Server) projectOpenHostApplication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel, err := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resolved, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	command, err := hostApplicationCommand(resolved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotImplemented)
		return
	}
	if err := command.Start(); err != nil {
		http.Error(w, "cannot open host application: "+err.Error(), http.StatusBadGateway)
		return
	}
	if command.Process != nil {
		_ = command.Process.Release()
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "path": rel})
}
