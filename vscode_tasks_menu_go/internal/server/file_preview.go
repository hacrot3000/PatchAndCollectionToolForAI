package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	filePreviewTextLimit  = int64(10 << 20)
	filePreviewImageLimit = int64(50 << 20)
	filePreviewSniffBytes = 8 << 10
)

type filePreviewResponse struct {
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	ProjectPath string `json:"project_path,omitempty"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	URL         string `json:"url,omitempty"`
}

func detectPreviewImageType(sample []byte) string {
	switch {
	case len(sample) >= 8 && bytes.Equal(sample[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return "image/png"
	case len(sample) >= 3 && sample[0] == 0xff && sample[1] == 0xd8 && sample[2] == 0xff:
		return "image/jpeg"
	case len(sample) >= 6 && (string(sample[:6]) == "GIF87a" || string(sample[:6]) == "GIF89a"):
		return "image/gif"
	case len(sample) >= 12 && string(sample[:4]) == "RIFF" && string(sample[8:12]) == "WEBP":
		return "image/webp"
	case len(sample) >= 2 && sample[0] == 'B' && sample[1] == 'M':
		return "image/bmp"
	default:
		return ""
	}
}

func previewFileHint(resolved string) string {
	f, err := os.Open(resolved)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	sample := make([]byte, filePreviewSniffBytes)
	n, readErr := f.Read(sample)
	if readErr != nil && readErr != io.EOF {
		return ""
	}
	sample = sample[:n]
	if detectPreviewImageType(sample) != "" {
		if info.Size() <= filePreviewImageLimit {
			return "image"
		}
		return ""
	}
	if info.Size() <= filePreviewTextLimit && previewTextBytesValid(sample) {
		return "text"
	}
	return ""
}

func previewTextBytesValid(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	return utf8.Valid(data)
}

func (s *Server) filePreviewInfo(requested string) (filePreviewResponse, string, error) {
	resolved, err := s.resolveDownloadPath(requested)
	if err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("file unavailable")
	}
	f, err := os.Open(resolved)
	if err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("file unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return filePreviewResponse{}, "", fmt.Errorf("file unavailable")
	}

	sample := make([]byte, filePreviewSniffBytes)
	n, readErr := f.Read(sample)
	if readErr != nil && readErr != io.EOF {
		return filePreviewResponse{}, "", fmt.Errorf("file unavailable")
	}
	sample = sample[:n]
	name := filepath.Base(resolved)

	if imageType := detectPreviewImageType(sample); imageType != "" {
		if info.Size() > filePreviewImageLimit {
			return filePreviewResponse{}, "", fmt.Errorf("image is too large to preview (limit 50 MiB)")
		}
		return filePreviewResponse{
			Kind: "image", Path: resolved, Name: name, Size: info.Size(), ContentType: imageType,
			URL: "/api/files/preview?mode=content&path=" + url.QueryEscape(resolved),
		}, resolved, nil
	}

	if info.Size() > filePreviewTextLimit {
		return filePreviewResponse{}, "", fmt.Errorf("text file is too large to preview (limit 10 MiB)")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("file unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(f, filePreviewTextLimit+1))
	if err != nil || int64(len(data)) > filePreviewTextLimit {
		return filePreviewResponse{}, "", fmt.Errorf("text file is too large to preview (limit 10 MiB)")
	}
	if !previewTextBytesValid(data) {
		return filePreviewResponse{}, "", fmt.Errorf("file is neither a supported image nor UTF-8 text")
	}

	workspace, err := filepath.Abs(s.Workspace)
	if err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("workspace unavailable")
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return filePreviewResponse{}, "", fmt.Errorf("workspace unavailable")
	}
	rel, err := filepath.Rel(workspace, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filePreviewResponse{}, "", fmt.Errorf("file unavailable")
	}
	return filePreviewResponse{
		Kind: "text", Path: resolved, ProjectPath: filepath.ToSlash(rel), Name: name, Size: info.Size(),
		ContentType: "text/plain; charset=utf-8",
	}, resolved, nil
}

func (s *Server) filePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requested := strings.TrimSpace(r.URL.Query().Get("path"))
	info, resolved, err := s.filePreviewInfo(requested)
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
	f, err := os.Open(resolved)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != info.Size {
		http.Error(w, "file changed; select it again", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name, stat.ModTime(), f)
}
