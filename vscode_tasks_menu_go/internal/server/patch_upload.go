package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxPatchPackageUpload = int64(256 << 20)

type patchUploadResult struct {
	Name      string `json:"name"`
	QueuePath string `json:"queue_path"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Overwrite bool   `json:"overwrite"`
}

func supportedPatchUploadName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".zip") ||
		strings.HasSuffix(lower, ".tar") ||
		strings.HasSuffix(lower, ".tgz") ||
		strings.HasSuffix(lower, ".tar.gz")
}

func (s *Server) patchUploadDir() (string, error) {
	root, err := s.projectRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "patchs")
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) {
			return "", err
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", os.ErrInvalid
	}
	return dir, nil
}

func (s *Server) patchUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPatchPackageUpload+(2<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "invalid PATCH/COLLECT upload", http.StatusBadRequest)
		return
	}
	src, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing PATCH/COLLECT package", http.StatusBadRequest)
		return
	}
	defer src.Close()

	name := cleanUploadName(header.Filename)
	if name == "" || !supportedPatchUploadName(name) {
		http.Error(w, "PATCH/COLLECT upload must be .zip, .tar, .tgz, or .tar.gz", http.StatusUnsupportedMediaType)
		return
	}
	dir, err := s.patchUploadDir()
	if err != nil {
		http.Error(w, "patchs queue directory is unavailable or unsafe", http.StatusConflict)
		return
	}
	target := filepath.Join(dir, name)
	overwrite := r.URL.Query().Get("overwrite") == "1"
	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			http.Error(w, "queue target is not a regular file", http.StatusConflict)
			return
		}
		if !overwrite {
			http.Error(w, "package already exists in Patch queue", http.StatusConflict)
			return
		}
	} else if !os.IsNotExist(statErr) {
		http.Error(w, "queue target is unavailable", http.StatusConflict)
		return
	}

	resourceID := filepath.ToSlash(filepath.Join("patchs", name))
	lease, ok := s.acquireSharedMutation(w, r, "patch.upload", resourceID)
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)

	tmp, err := os.CreateTemp(dir, ".taskdeck-patch-upload-*")
	if err != nil {
		http.Error(w, "cannot create PATCH/COLLECT upload", http.StatusInternalServerError)
		return
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	hash := sha256.New()
	limited := io.LimitReader(src, maxPatchPackageUpload+1)
	n, copyErr := io.Copy(io.MultiWriter(tmp, hash), limited)
	if copyErr != nil || n > maxPatchPackageUpload {
		cleanup()
		if n > maxPatchPackageUpload {
			http.Error(w, "PATCH/COLLECT package too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "PATCH/COLLECT upload failed", http.StatusInternalServerError)
		}
		return
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		http.Error(w, "PATCH/COLLECT upload sync failed", http.StatusInternalServerError)
		return
	}
	if err := tmp.Chmod(0o644); err != nil {
		cleanup()
		http.Error(w, "PATCH/COLLECT upload chmod failed", http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "PATCH/COLLECT upload close failed", http.StatusInternalServerError)
		return
	}
	if !overwrite {
		if _, err := os.Lstat(target); err == nil {
			_ = os.Remove(tmpName)
			http.Error(w, "package already exists in Patch queue", http.StatusConflict)
			return
		}
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		http.Error(w, "cannot publish PATCH/COLLECT package to queue", http.StatusInternalServerError)
		return
	}

	digest := hex.EncodeToString(hash.Sum(nil))
	s.auditSharedSuccess(r, "patch.upload", "patch_package", resourceID, map[string]any{
		"overwrite": overwrite,
		"size":      n,
		"sha256":    digest,
	})
	writeJSON(w, http.StatusCreated, patchUploadResult{
		Name:      name,
		QueuePath: resourceID,
		Size:      n,
		SHA256:    digest,
		Overwrite: overwrite,
	})
}
