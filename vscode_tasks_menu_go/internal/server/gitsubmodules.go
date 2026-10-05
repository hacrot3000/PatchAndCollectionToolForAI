package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type gitSubmoduleRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	URL         string `json:"url,omitempty"`
	Branch      string `json:"branch,omitempty"`
	ExpectedSHA string `json:"expected_sha,omitempty"`
	ActualSHA   string `json:"actual_sha,omitempty"`
	Initialized bool   `json:"initialized"`
	Dirty       bool   `json:"dirty,omitempty"`
	Mismatch    bool   `json:"mismatch,omitempty"`
	Status      string `json:"status"`
}

type gitSubmoduleConfig struct {
	Name   string
	Path   string
	URL    string
	Branch string
}

func gitSubmoduleID(path string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(path))))
	return hex.EncodeToString(sum[:])
}

func parseGitmodulesConfig(raw string) []gitSubmoduleConfig {
	type partial struct {
		name, path, url, branch string
	}
	order := []string{}
	byName := map[string]*partial{}
	for _, record := range strings.Split(raw, "\x00") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		key, value, ok := strings.Cut(record, "\n")
		if !ok {
			key, value, ok = strings.Cut(record, " ")
		}
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		const prefix = "submodule."
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(key, prefix)
		idx := strings.LastIndex(rest, ".")
		if idx <= 0 || idx == len(rest)-1 {
			continue
		}
		name, field := rest[:idx], rest[idx+1:]
		item := byName[name]
		if item == nil {
			item = &partial{name: name}
			byName[name] = item
			order = append(order, name)
		}
		switch field {
		case "path":
			item.path = value
		case "url":
			item.url = value
		case "branch":
			item.branch = value
		}
	}
	rows := make([]gitSubmoduleConfig, 0, len(order))
	for _, name := range order {
		item := byName[name]
		if item == nil || strings.TrimSpace(item.path) == "" {
			continue
		}
		rows = append(rows, gitSubmoduleConfig{Name: item.name, Path: item.path, URL: item.url, Branch: item.branch})
	}
	return rows
}

func validGitSubmodulePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '\x00') || filepath.IsAbs(value) {
		return "", fmt.Errorf("invalid submodule path")
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid submodule path")
	}
	return filepath.ToSlash(clean), nil
}

func (s *Server) gitSubmoduleConfigs(ctx context.Context) ([]gitSubmoduleConfig, error) {
	gitmodules := filepath.Join(s.gitDirectory(ctx), ".gitmodules")
	if _, err := os.Stat(gitmodules); err != nil {
		if os.IsNotExist(err) {
			return []gitSubmoduleConfig{}, nil
		}
		return nil, err
	}
	out, stderr, _, err := s.runGit(ctx, 5*time.Second,
		"config", "-z", "--file", ".gitmodules", "--get-regexp", "^submodule\\..*\\.(path|url|branch)$")
	if err != nil {
		if strings.TrimSpace(out) == "" && strings.TrimSpace(stderr) == "" {
			return []gitSubmoduleConfig{}, nil
		}
		return nil, fmt.Errorf("%s", strings.TrimSpace(joinGitOutput(stderr, err.Error())))
	}
	rows := parseGitmodulesConfig(out)
	for i := range rows {
		pathValue, pathErr := validGitSubmodulePath(rows[i].Path)
		if pathErr != nil {
			return nil, fmt.Errorf("submodule %q has unsafe path", rows[i].Name)
		}
		rows[i].Path = pathValue
	}
	return rows, nil
}

func (s *Server) gitSubmoduleExpectedSHA(ctx context.Context, pathValue string) string {
	out, _, _, err := s.runGit(ctx, 4*time.Second, "ls-tree", "HEAD", "--", pathValue)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return ""
	}
	fields := strings.Fields(line)
	if len(fields) >= 3 && fields[0] == "160000" && fields[1] == "commit" && gitCompareCommitPattern.MatchString(fields[2]) {
		return fields[2]
	}
	return ""
}

func (s *Server) gitSubmoduleActualState(ctx context.Context, pathValue string) (sha string, dirty bool, initialized bool) {
	root := filepath.Join(s.gitDirectory(ctx), filepath.FromSlash(pathValue))
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return "", false, false
	}
	out, _, _, err := runGitInDirectory(ctx, 4*time.Second, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", false, true
	}
	sha = strings.TrimSpace(out)
	status, _, _, statusErr := runGitInDirectory(ctx, 5*time.Second, root,
		"status", "--porcelain=v1", "--untracked-files=normal")
	if statusErr == nil {
		dirty = strings.TrimSpace(status) != ""
	}
	return sha, dirty, true
}

func (s *Server) gitSubmoduleRows(ctx context.Context) ([]gitSubmoduleRow, error) {
	configs, err := s.gitSubmoduleConfigs(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]gitSubmoduleRow, 0, len(configs))
	for _, item := range configs {
		expected := s.gitSubmoduleExpectedSHA(ctx, item.Path)
		actual, dirty, initialized := s.gitSubmoduleActualState(ctx, item.Path)
		row := gitSubmoduleRow{
			ID: gitSubmoduleID(item.Path), Name: item.Name, Path: item.Path,
			URL: item.URL, Branch: item.Branch, ExpectedSHA: expected, ActualSHA: actual,
			Initialized: initialized, Dirty: dirty,
		}
		row.Mismatch = initialized && expected != "" && actual != "" && expected != actual
		switch {
		case !initialized:
			row.Status = "uninitialized"
		case dirty && row.Mismatch:
			row.Status = "dirty-mismatch"
		case dirty:
			row.Status = "dirty"
		case row.Mismatch:
			row.Status = "mismatch"
		default:
			row.Status = "clean"
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func gitSubmoduleByID(rows []gitSubmoduleRow, id string) (gitSubmoduleRow, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return gitSubmoduleRow{}, false
	}
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return gitSubmoduleRow{}, false
}

func (s *Server) gitSubmodules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.gitSubmoduleRows(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"submodules": rows, "count": len(rows)})
}
