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
	"path/filepath"
	"strings"
)

type projectArchiveExtractRequest struct {
	Path string `json:"path"`
	Destination string `json:"destination"`
}

func extractedArchiveFileMode(mode os.FileMode) os.FileMode {
	perm := mode.Perm() & 0o777
	if perm == 0 {
		return 0o644
	}
	return perm
}

func ensureProjectArchiveDestination(root, member string, directory bool) (string, error) {
	clean, err := safeArchiveMemberPath(strings.TrimSuffix(member, "/"))
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", os.ErrInvalid
	}
	if _, err := os.Lstat(target); err == nil {
		return "", os.ErrExist
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if directory {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return "", err
		}
	} else if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return target, nil
}

func extractProjectZip(source, destination string) error {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	var entries int
	var total int64
	for _, file := range reader.File {
		entries++
		if entries > maxProjectArchiveEntries {
			return errProjectArchiveLimit
		}
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink entry is not supported: %s", file.Name)
		}
		directory := file.FileInfo().IsDir()
		if !directory {
			size := int64(file.UncompressedSize64)
			if size < 0 || total+size > maxProjectArchiveTotalBytes {
				return errProjectArchiveLimit
			}
			total += size
		}
		target, err := ensureProjectArchiveDestination(destination, file.Name, directory)
		if err != nil {
			return err
		}
		if directory {
			continue
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, extractedArchiveFileMode(file.Mode()))
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.CopyN(output, input, int64(file.UncompressedSize64))
		outputErr := output.Close()
		inputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if outputErr != nil {
			return outputErr
		}
		if inputErr != nil {
			return inputErr
		}
	}
	return nil
}

func extractProjectTarGz(source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var entries int
	var total int64
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		entries++
		if entries > maxProjectArchiveEntries {
			return errProjectArchiveLimit
		}
		directory := false
		switch header.Typeflag {
		case tar.TypeDir:
			directory = true
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || total+header.Size > maxProjectArchiveTotalBytes {
				return errProjectArchiveLimit
			}
			total += header.Size
		default:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
		target, err := ensureProjectArchiveDestination(destination, header.Name, directory)
		if err != nil {
			return err
		}
		if directory {
			continue
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, extractedArchiveFileMode(os.FileMode(header.Mode)))
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(output, tr, header.Size)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (s *Server) projectArchiveExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectArchiveExtractRequest
	if err := decodeProjectArchiveJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	req.Destination = strings.TrimSpace(req.Destination)
	format := archiveFormatFromPath(req.Path)
	if format == "" || req.Destination == "" {
		http.Error(w, "archive path and destination directory are required", http.StatusBadRequest)
		return
	}
	source, err := s.resolveProjectPath(req.Path, false, false)
	if err != nil {
		http.Error(w, "archive not found inside workspace", http.StatusNotFound)
		return
	}
	destRel, destination, err := s.resolveHostWorkspaceDestination(req.Destination)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	lease, ok := s.acquireSharedMutation(w, r, "file.archive.extract", destRel)
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)
	if err := os.Mkdir(destination, 0o755); err != nil {
		http.Error(w, "extract destination must not already exist", http.StatusConflict)
		return
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(destination)
		}
	}()
	if format == "zip" {
		err = extractProjectZip(source, destination)
	} else {
		err = extractProjectTarGz(source, destination)
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errProjectArchiveLimit) {
			status = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, os.ErrExist) {
			status = http.StatusConflict
		}
		http.Error(w, "cannot extract archive: "+err.Error(), status)
		return
	}
	success = true
	s.auditSharedSuccess(r, "file.archive.extract", "directory", destRel, map[string]any{"source": req.Path, "format": format})
	writeJSON(w, http.StatusCreated, map[string]any{"path": destRel, "format": format})
}
