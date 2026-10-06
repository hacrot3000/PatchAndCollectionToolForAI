package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

type projectArchiveCreateRequest struct {
	Paths []string `json:"paths"`
	Output string `json:"output"`
	Format string `json:"format,omitempty"`
}

type projectArchiveDownloadRequest struct {
	Paths []string `json:"paths"`
	Name string `json:"name,omitempty"`
}

type projectArchiveSource struct {
	Virtual string
	Path string
	Info os.FileInfo
	Base string
}

func decodeProjectArchiveJSON(w http.ResponseWriter, r *http.Request, value any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return errors.New("invalid JSON")
	}
	return nil
}

func (s *Server) collectProjectArchiveSources(paths []string, outputVirtual string) ([]projectArchiveSource, error) {
	if len(paths) == 0 || len(paths) > 512 {
		return nil, errors.New("archive requires 1 to 512 selected paths")
	}
	seen := make(map[string]struct{}, len(paths))
	sources := make([]projectArchiveSource, 0, len(paths))
	for _, raw := range paths {
		virtual, err := cleanProjectRelativePath(raw, false)
		if err != nil {
			return nil, err
		}
		if virtual == strings.TrimSpace(outputVirtual) {
			return nil, errors.New("archive output cannot also be an input")
		}
		if _, exists := seen[virtual]; exists {
			continue
		}
		seen[virtual] = struct{}{}
		resolved, err := s.resolveProjectPath(virtual, true, true)
		if err != nil {
			return nil, fmt.Errorf("archive input unavailable: %s", virtual)
		}
		info, err := os.Lstat(resolved)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
			return nil, fmt.Errorf("unsupported archive input: %s", virtual)
		}
		base := pathpkg.Base(strings.ReplaceAll(virtual, "\\", "/"))
		sources = append(sources, projectArchiveSource{Virtual: virtual, Path: resolved, Info: info, Base: base})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Virtual < sources[j].Virtual })
	return sources, nil
}

func walkProjectArchiveSource(source projectArchiveSource, fn func(string, string, os.FileInfo) error) error {
	var entries int
	var total int64
	return filepath.Walk(source.Path, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink archive input is not supported")
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("special archive input is not supported")
		}
		rel, err := filepath.Rel(source.Path, current)
		if err != nil {
			return err
		}
		name := source.Base
		if rel != "." {
			name = pathpkg.Join(source.Base, filepath.ToSlash(rel))
		}
		entries++
		if entries > maxProjectArchiveEntries {
			return errProjectArchiveLimit
		}
		if info.Mode().IsRegular() {
			if info.Size() < 0 || total+info.Size() > maxProjectArchiveTotalBytes {
				return errProjectArchiveLimit
			}
			total += info.Size()
		}
		return fn(current, name, info)
	})
}

func writeProjectZip(writer *zip.Writer, sources []projectArchiveSource) error {
	for _, source := range sources {
		if err := walkProjectArchiveSource(source, func(current, name string, info os.FileInfo) error {
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(name)
			header.Method = zip.Deflate
			if info.IsDir() {
				header.Name += "/"
			}
			entry, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			file, err := os.Open(current)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(entry, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeProjectTarGz(writer io.Writer, sources []projectArchiveSource) error {
	gz := gzip.NewWriter(writer)
	tw := tar.NewWriter(gz)
	for _, source := range sources {
		if err := walkProjectArchiveSource(source, func(current, name string, info os.FileInfo) error {
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(name)
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			file, err := os.Open(current)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

func (s *Server) projectArchiveCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectArchiveCreateRequest
	if err := decodeProjectArchiveJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Output = strings.TrimSpace(req.Output)
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = archiveFormatFromPath(req.Output)
	}
	if format != "zip" && format != "tar.gz" {
		http.Error(w, "archive format must be zip or tar.gz", http.StatusBadRequest)
		return
	}
	if (format == "zip" && !strings.HasSuffix(strings.ToLower(req.Output), ".zip")) ||
		(format == "tar.gz" && archiveFormatFromPath(req.Output) != "tar.gz") {
		http.Error(w, "archive output extension does not match format", http.StatusBadRequest)
		return
	}
	rel, target, err := s.resolveHostWorkspaceDestination(req.Output)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	sources, err := s.collectProjectArchiveSources(req.Paths, rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	lease, ok := s.acquireSharedMutation(w, r, "file.archive.create", rel)
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)
	tmp, err := os.CreateTemp(filepath.Dir(target), ".taskdeck-archive-*")
	if err != nil {
		http.Error(w, "cannot create archive temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		_ = tmp.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()
	if format == "zip" {
		zw := zip.NewWriter(tmp)
		err = writeProjectZip(zw, sources)
		if closeErr := zw.Close(); err == nil {
			err = closeErr
		}
	} else {
		err = writeProjectTarGz(tmp, sources)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errProjectArchiveLimit) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "cannot create archive: "+err.Error(), status)
		return
	}
	if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
		http.Error(w, "archive output already exists", http.StatusConflict)
		return
	}
	if err := os.Rename(tmpPath, target); err != nil {
		http.Error(w, "cannot finalize archive", http.StatusInternalServerError)
		return
	}
	success = true
	s.auditSharedSuccess(r, "file.archive.create", "file", rel, map[string]any{"format": format, "inputs": len(sources)})
	writeJSON(w, http.StatusCreated, map[string]any{"path": rel, "format": format})
}

func sanitizeArchiveDownloadName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "." || value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "taskdeck-selection.zip"
	}
	if !strings.HasSuffix(strings.ToLower(value), ".zip") {
		value += ".zip"
	}
	return value
}

func (s *Server) projectArchiveDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectArchiveDownloadRequest
	if err := decodeProjectArchiveJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sources, err := s.collectProjectArchiveSources(req.Paths, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizeArchiveDownloadName(req.Name)))
	w.Header().Set("Cache-Control", "no-store")
	zw := zip.NewWriter(w)
	if err := writeProjectZip(zw, sources); err != nil {
		_ = zw.Close()
		return
	}
	_ = zw.Close()
}
