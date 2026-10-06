package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"strings"
	"time"
)

const (
	maxProjectArchiveEntries = 20000
	maxProjectArchivePreviewEntries = 10000
	maxProjectArchiveTotalBytes int64 = 16 << 30
)

var errProjectArchiveLimit = errors.New("project archive exceeds resource limit")

type projectArchiveEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64 `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

type projectArchivePreviewResponse struct {
	Path string `json:"path"`
	Format string `json:"format"`
	Entries []projectArchiveEntry `json:"entries"`
	Truncated bool `json:"truncated,omitempty"`
	Files int `json:"files"`
	Dirs int `json:"dirs"`
	Bytes int64 `json:"bytes"`
}

func archiveFormatFromPath(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return ""
	}
}

func safeArchiveMemberPath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") {
		return "", os.ErrInvalid
	}
	clean := pathpkg.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", os.ErrInvalid
	}
	return clean, nil
}

func (s *Server) projectArchivePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	virtual := strings.TrimSpace(r.URL.Query().Get("path"))
	format := archiveFormatFromPath(virtual)
	if virtual == "" || format == "" {
		http.Error(w, "path must reference a .zip, .tar.gz, or .tgz archive", http.StatusBadRequest)
		return
	}
	resolved, err := s.resolveProjectPath(virtual, false, false)
	if err != nil {
		http.Error(w, "archive not found inside workspace", http.StatusNotFound)
		return
	}
	result, err := previewProjectArchive(resolved, virtual, format)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errProjectArchiveLimit) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "cannot preview archive: "+err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func previewProjectArchive(resolved, virtual, format string) (projectArchivePreviewResponse, error) {
	result := projectArchivePreviewResponse{Path: virtual, Format: format, Entries: make([]projectArchiveEntry, 0)}
	add := func(name string, directory bool, size int64, modified time.Time) error {
		clean, err := safeArchiveMemberPath(strings.TrimSuffix(name, "/"))
		if err != nil {
			return fmt.Errorf("unsafe archive entry %q", name)
		}
		if size < 0 || result.Bytes+size > maxProjectArchiveTotalBytes || result.Files+result.Dirs >= maxProjectArchiveEntries {
			return errProjectArchiveLimit
		}
		result.Bytes += size
		kind := "file"
		if directory {
			result.Dirs++
			kind = "directory"
		} else {
			result.Files++
		}
		if len(result.Entries) < maxProjectArchivePreviewEntries {
			result.Entries = append(result.Entries, projectArchiveEntry{
				Path: clean, Type: kind, Size: size, Modified: modified.UTC().Format(time.RFC3339),
			})
		} else {
			result.Truncated = true
		}
		return nil
	}
	switch format {
	case "zip":
		reader, err := zip.OpenReader(resolved)
		if err != nil {
			return result, err
		}
		defer reader.Close()
		for _, file := range reader.File {
			if file.FileInfo().Mode()&os.ModeSymlink != 0 {
				return result, fmt.Errorf("symlink entry is not supported: %s", file.Name)
			}
			if err := add(file.Name, file.FileInfo().IsDir(), int64(file.UncompressedSize64), file.Modified); err != nil {
				return result, err
			}
		}
	case "tar.gz":
		file, err := os.Open(resolved)
		if err != nil {
			return result, err
		}
		defer file.Close()
		gz, err := gzip.NewReader(file)
		if err != nil {
			return result, err
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return result, err
			}
			switch header.Typeflag {
			case tar.TypeReg, tar.TypeRegA:
				if err := add(header.Name, false, header.Size, header.ModTime); err != nil {
					return result, err
				}
			case tar.TypeDir:
				if err := add(header.Name, true, 0, header.ModTime); err != nil {
					return result, err
				}
			default:
				return result, fmt.Errorf("unsupported archive entry type for %s", header.Name)
			}
		}
	default:
		return result, os.ErrInvalid
	}
	return result, nil
}
