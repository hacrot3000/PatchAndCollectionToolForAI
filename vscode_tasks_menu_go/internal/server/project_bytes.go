package server

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
)

const (
	projectBytesDefaultLimit = 4096
	projectBytesMaxLimit     = 64 << 10
)

type projectBytesResponse struct {
	Path       string `json:"path"`
	Offset     int64  `json:"offset"`
	Length     int    `json:"length"`
	Size       int64  `json:"size"`
	NextOffset int64  `json:"next_offset"`
	EOF        bool   `json:"eof"`
	Base64     string `json:"base64"`
}

func (s *Server) projectBytes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel, err := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	offset := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || offset < 0 {
			http.Error(w, "offset must be a non-negative integer", http.StatusBadRequest)
			return
		}
	}
	limit := projectBytesDefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > projectBytesMaxLimit {
			http.Error(w, "limit must be between 1 and 65536", http.StatusBadRequest)
			return
		}
	}
	path, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	rootView, _, err := s.projectRootForVirtualPath(rel)
	if err != nil {
		http.Error(w, "project root unavailable", http.StatusInternalServerError)
		return
	}
	pinned, err := openProjectPinnedFile(rootView.Path, path)
	if err != nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	defer pinned.close()
	data, info, err := pinned.readChunk(offset, limit)
	if err != nil || info == nil {
		http.Error(w, "project file unavailable", http.StatusNotFound)
		return
	}
	next := offset + int64(len(data))
	if next > info.Size() {
		next = info.Size()
	}
	writeJSON(w, http.StatusOK, projectBytesResponse{
		Path:       rel,
		Offset:     offset,
		Length:     len(data),
		Size:       info.Size(),
		NextOffset: next,
		EOF:        next >= info.Size(),
		Base64:     base64.StdEncoding.EncodeToString(data),
	})
}
