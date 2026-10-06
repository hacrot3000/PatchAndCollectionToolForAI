package server

import (
	"crypto/md5" // MD5 is exposed only for legacy compatibility/integrity comparison, not for security decisions.
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strings"
)

const maxProjectIntegrityFileBytes int64 = 4 << 30

type projectIntegrityHashResponse struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	MD5    string `json:"md5"`
}

func hashProjectRegularFile(path string) (projectIntegrityHashResponse, error) {
	file, err := os.Open(path)
	if err != nil {
		return projectIntegrityHashResponse{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return projectIntegrityHashResponse{}, err
	}
	if !info.Mode().IsRegular() {
		return projectIntegrityHashResponse{}, os.ErrInvalid
	}
	if info.Size() > maxProjectIntegrityFileBytes {
		return projectIntegrityHashResponse{}, errProjectFileTooLarge
	}
	sha := sha256.New()
	legacy := md5.New()
	if _, err := io.Copy(io.MultiWriter(sha, legacy), file); err != nil {
		return projectIntegrityHashResponse{}, err
	}
	return projectIntegrityHashResponse{
		Size: info.Size(),
		SHA256: hex.EncodeToString(sha.Sum(nil)),
		MD5: hex.EncodeToString(legacy.Sum(nil)),
	}, nil
}

func (s *Server) projectIntegrity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	virtualPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if virtualPath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	resolved, err := s.resolveProjectPath(virtualPath, false, false)
	if err != nil {
		http.Error(w, "file not found inside workspace", http.StatusNotFound)
		return
	}
	result, err := hashProjectRegularFile(resolved)
	if err != nil {
		switch {
		case err == errProjectFileTooLarge:
			http.Error(w, "file exceeds integrity hashing limit", http.StatusRequestEntityTooLarge)
		case os.IsNotExist(err):
			http.Error(w, "file not found", http.StatusNotFound)
		default:
			http.Error(w, "cannot hash project file", http.StatusBadRequest)
		}
		return
	}
	result.Path = virtualPath
	writeJSON(w, http.StatusOK, result)
}
