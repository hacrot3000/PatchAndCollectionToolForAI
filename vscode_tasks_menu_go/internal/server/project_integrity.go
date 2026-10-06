package server

import (
	"crypto/md5" // MD5 is exposed only for legacy compatibility/integrity comparison, not for security decisions.
	"crypto/sha256"
	"encoding/hex"
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

const (
	maxProjectIntegrityFileBytes int64 = 4 << 30
	maxProjectManifestBytes            = 4 << 20
	maxProjectManifestFiles            = 20000
	maxProjectManifestTotalBytes int64 = 16 << 30
	maxProjectManifestFailures         = 200
)

var (
	errProjectFileTooLarge = errors.New("project file exceeds integrity hashing limit")
	errProjectManifestLimit = errors.New("project integrity manifest exceeds resource limit")
)

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

type projectIntegrityRequest struct {
	Action string `json:"action"`
	Path   string `json:"path"`
}

type projectIntegrityManifestFailure struct {
	Path     string `json:"path"`
	Status   string `json:"status"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

type projectIntegrityManifestVerifyResponse struct {
	Path       string                            `json:"path"`
	OK         bool                              `json:"ok"`
	Total      int                               `json:"total"`
	Matched    int                               `json:"matched"`
	Missing    int                               `json:"missing"`
	Mismatched int                               `json:"mismatched"`
	Invalid    int                               `json:"invalid"`
	Failures   []projectIntegrityManifestFailure `json:"failures,omitempty"`
	Truncated  bool                              `json:"truncated,omitempty"`
}

func appendManifestFailure(result *projectIntegrityManifestVerifyResponse, failure projectIntegrityManifestFailure) {
	if len(result.Failures) < maxProjectManifestFailures {
		result.Failures = append(result.Failures, failure)
		return
	}
	result.Truncated = true
}

func projectManifestRelativePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") {
		return "", os.ErrInvalid
	}
	clean := pathpkg.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", os.ErrInvalid
	}
	return clean, nil
}

func (s *Server) generateProjectSHA256Manifest(virtualDir string) (string, int, int64, error) {
	resolvedDir, err := s.resolveProjectPath(virtualDir, true, true)
	if err != nil {
		return "", 0, 0, err
	}
	info, err := os.Stat(resolvedDir)
	if err != nil || !info.IsDir() {
		return "", 0, 0, os.ErrInvalid
	}
	type entry struct {
		rel  string
		hash string
		size int64
	}
	entries := make([]entry, 0, 128)
	var totalBytes int64
	err = filepath.WalkDir(resolvedDir, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == resolvedDir {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if len(entries) >= maxProjectManifestFiles || totalBytes+info.Size() > maxProjectManifestTotalBytes {
			return errProjectManifestLimit
		}
		hashed, err := hashProjectRegularFile(current)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(resolvedDir, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		entries = append(entries, entry{rel: rel, hash: hashed.SHA256, size: info.Size()})
		totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return "", 0, 0, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	var out strings.Builder
	for _, item := range entries {
		fmt.Fprintf(&out, "%s  %s\n", item.hash, item.rel)
		if out.Len() > maxProjectManifestBytes {
			return "", 0, 0, errProjectManifestLimit
		}
	}
	return out.String(), len(entries), totalBytes, nil
}

func (s *Server) verifyProjectSHA256Manifest(virtualManifest string) (projectIntegrityManifestVerifyResponse, error) {
	result := projectIntegrityManifestVerifyResponse{Path: virtualManifest}
	resolved, err := s.resolveProjectPath(virtualManifest, false, false)
	if err != nil {
		return result, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return result, err
	}
	if len(data) > maxProjectManifestBytes {
		return result, errProjectManifestLimit
	}
	base := pathpkg.Dir(strings.ReplaceAll(virtualManifest, "\\", "/"))
	seen := make(map[string]struct{})
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		result.Total++
		if result.Total > maxProjectManifestFiles || len(line) < 67 {
			result.Invalid++
			appendManifestFailure(&result, projectIntegrityManifestFailure{Path: fmt.Sprintf("line %d", lineNo+1), Status: "invalid"})
			if result.Total > maxProjectManifestFiles {
				return result, errProjectManifestLimit
			}
			continue
		}
		expected := strings.ToLower(line[:64])
		if _, err := hex.DecodeString(expected); err != nil || (line[64:66] != "  " && line[64:66] != " *") {
			result.Invalid++
			appendManifestFailure(&result, projectIntegrityManifestFailure{Path: fmt.Sprintf("line %d", lineNo+1), Status: "invalid"})
			continue
		}
		rel, err := projectManifestRelativePath(line[66:])
		if err != nil {
			result.Invalid++
			appendManifestFailure(&result, projectIntegrityManifestFailure{Path: line[66:], Status: "invalid"})
			continue
		}
		if _, exists := seen[rel]; exists {
			result.Invalid++
			appendManifestFailure(&result, projectIntegrityManifestFailure{Path: rel, Status: "duplicate"})
			continue
		}
		seen[rel] = struct{}{}
		virtualFile := rel
		if base != "." && base != "" {
			virtualFile = pathpkg.Join(base, rel)
		}
		filePath, err := s.resolveProjectPath(virtualFile, false, false)
		if err != nil {
			result.Missing++
			appendManifestFailure(&result, projectIntegrityManifestFailure{Path: rel, Status: "missing", Expected: expected})
			continue
		}
		hashed, err := hashProjectRegularFile(filePath)
		if err != nil {
			result.Missing++
			appendManifestFailure(&result, projectIntegrityManifestFailure{Path: rel, Status: "unreadable", Expected: expected})
			continue
		}
		actual := strings.ToLower(hashed.SHA256)
		if actual != expected {
			result.Mismatched++
			appendManifestFailure(&result, projectIntegrityManifestFailure{Path: rel, Status: "mismatch", Expected: expected, Actual: actual})
			continue
		}
		result.Matched++
	}
	result.OK = result.Total > 0 && result.Matched == result.Total && result.Missing == 0 && result.Mismatched == 0 && result.Invalid == 0
	return result, nil
}

func (s *Server) projectIntegrity(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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
			case errors.Is(err, errProjectFileTooLarge):
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
	case http.MethodPost:
		var req projectIntegrityRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		req.Action = strings.TrimSpace(req.Action)
		req.Path = strings.TrimSpace(req.Path)
		if req.Path == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}
		switch req.Action {
		case "manifest":
			content, files, bytes, err := s.generateProjectSHA256Manifest(req.Path)
			if err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, errProjectManifestLimit) {
					status = http.StatusRequestEntityTooLarge
				}
				http.Error(w, "cannot generate manifest: "+err.Error(), status)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"path": req.Path, "algorithm": "sha256", "files": files, "bytes": bytes, "content": content})
		case "verify_manifest":
			result, err := s.verifyProjectSHA256Manifest(req.Path)
			if err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, errProjectManifestLimit) {
					status = http.StatusRequestEntityTooLarge
				}
				http.Error(w, "cannot verify manifest: "+err.Error(), status)
				return
			}
			writeJSON(w, http.StatusOK, result)
		default:
			http.Error(w, "unsupported integrity action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
